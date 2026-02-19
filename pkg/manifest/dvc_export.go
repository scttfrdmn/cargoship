// Package manifest provides manifest generation and DVC interoperability.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DVCFile is the top-level structure of a DVC sidecar .dvc file.
//
// DVC v3+ format:
//
//	outs:
//	  - path: data.csv
//	    md5:  a304afb96060aad90176268345e10355
//	    size: 1024000
//	    meta:
//	      cloud_bucket: mybucket
//	      cloud_key: uploads/20251206-abc/shard-0/chunk-0.tar.zst
type DVCFile struct {
	Outs []DVCOutput `yaml:"outs"`
}

// DVCOutput describes a single tracked output entry inside a .dvc file.
type DVCOutput struct {
	// Path is the file path relative to the location of the .dvc file.
	Path string `yaml:"path"`

	// MD5 is the DVC-compatible MD5 hex digest of the tracked file.
	// Required for dvc status to compare workspace content against the record.
	MD5 string `yaml:"md5,omitempty"`

	// Size is the uncompressed file size in bytes.
	Size int64 `yaml:"size,omitempty"`

	// Meta carries CargoShip-specific cloud location metadata.
	// DVC ignores unknown meta fields but passes them through, so they
	// survive round-trips through dvc status / dvc push / dvc pull.
	Meta *DVCMeta `yaml:"meta,omitempty"`
}

// DVCMeta holds CargoShip cloud-storage provenance embedded in a .dvc file.
type DVCMeta struct {
	// CloudBucket is the S3 bucket that holds the CargoShip archive chunk.
	CloudBucket string `yaml:"cloud_bucket,omitempty"`

	// CloudKey is the S3 object key of the archive chunk containing this file.
	CloudKey string `yaml:"cloud_key,omitempty"`

	// UploadID is the CargoShip upload session that produced this .dvc file.
	UploadID string `yaml:"upload_id,omitempty"`
}

// DVCGenerateOptions configures .dvc file generation.
type DVCGenerateOptions struct {
	// CacheDir is the local DVC cache directory (e.g., ".dvc/cache").
	// When non-empty it is recorded in the manifest's DVCCompatibility block.
	// It does not affect the generated .dvc files.
	CacheDir string
}

// GenerateDVCFiles emits one DVC sidecar .dvc file per FileEntry in the
// manifest into outputDir, preserving the relative directory structure of the
// source tree.
//
// Each .dvc file is named <filename>.dvc and placed in the same subdirectory
// as the file it describes would occupy under outputDir, so that the generated
// tree mirrors the original source layout and can be committed to a DVC
// project alongside the data files.
//
// Only FileEntry values with a non-empty ContentHash are written; entries
// without a hash are silently skipped because dvc status requires the md5
// field to function.
//
// The function returns the number of .dvc files written.
func (m *Manifest) GenerateDVCFiles(outputDir string, opts *DVCGenerateOptions) (int, error) {
	if opts == nil {
		opts = &DVCGenerateOptions{}
	}

	written := 0
	for _, entry := range m.Files {
		if entry.ContentHash == "" {
			continue // no MD5 → skip; dvc status would be useless
		}

		n, err := writeDVCFile(outputDir, entry, m.UploadID, m.Bucket)
		if err != nil {
			return written, fmt.Errorf("write .dvc file for %s: %w", entry.Path, err)
		}
		written += n
	}

	// Record that .dvc files were generated in the manifest's compatibility block.
	if m.DVCCompatibility == nil {
		m.DVCCompatibility = &DVCCompatibility{}
	}
	m.DVCCompatibility.DVCFilesGenerated = true
	if opts.CacheDir != "" {
		m.DVCCompatibility.CacheDir = opts.CacheDir
	}

	return written, nil
}

// writeDVCFile creates the .dvc sidecar for a single FileEntry.
// It returns 1 on success so the caller can count written files easily.
func writeDVCFile(outputDir string, entry FileEntry, uploadID, bucket string) (int, error) {
	// Construct output path: <outputDir>/<entry-dir>/<basename>.dvc
	entryDir := filepath.Dir(entry.Path)
	dvcDir := filepath.Join(outputDir, filepath.FromSlash(entryDir))
	if err := os.MkdirAll(dvcDir, 0o755); err != nil {
		return 0, fmt.Errorf("create directory %s: %w", dvcDir, err)
	}

	basename := filepath.Base(entry.Path)
	dvcPath := filepath.Join(dvcDir, basename+".dvc")

	out := DVCOutput{
		Path: basename,
		MD5:  entry.ContentHash,
		Size: entry.Size,
	}

	// Attach cloud provenance when we have enough information.
	if bucket != "" || entry.S3Key != "" || uploadID != "" {
		out.Meta = &DVCMeta{
			CloudBucket: bucket,
			CloudKey:    entry.S3Key,
			UploadID:    uploadID,
		}
	}

	f := DVCFile{Outs: []DVCOutput{out}}

	data, err := yaml.Marshal(f)
	if err != nil {
		return 0, fmt.Errorf("marshal yaml: %w", err)
	}

	if err := os.WriteFile(dvcPath, data, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dvcPath, err)
	}
	return 1, nil
}
