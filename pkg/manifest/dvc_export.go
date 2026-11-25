// Package manifest provides manifest generation and DVC interoperability.
package manifest

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	// Overwrite replaces an existing .dvc file. By default it is kept, since it
	// may be a git-tracked file carrying fields CargoShip doesn't write
	// (remote:, deps) (#916).
	Overwrite bool

	// OnSkip, if set, is called with the path of each existing .dvc file kept.
	OnSkip func(path string)
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

	// #916: every write goes through a root at outputDir, so no manifest entry
	// can place a file outside it. Entries used to be joined onto outputDir as
	// recorded, so a ".." entry overwrote a file elsewhere and an absolute one
	// built a nested <out>/Users/... tree (inside the user's data, since upload's
	// --generate-dvc-files writes into the source directory).
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", outputDir, err)
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", outputDir, err)
	}
	defer func() { _ = root.Close() }()

	written := 0
	for _, entry := range m.Files {
		if entry.ContentHash == "" {
			continue // no MD5 → skip; dvc status would be useless
		}

		rel, err := dvcRelPath(m.SourcePath, entry.Path)
		if err != nil {
			return written, err
		}
		n, err := writeDVCFile(root, rel, entry, m.UploadID, m.Bucket, opts)
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

// dvcRelPath is where an entry's .dvc file goes, relative to the output
// directory: the entry's path relative to the upload's source, slash-separated.
// An entry with a ".." segment, or nothing left after cleaning, is refused
// rather than remapped (#916).
func dvcRelPath(sourcePath, entryPath string) (string, error) {
	rel := filepath.ToSlash(RelativeToSource(sourcePath, entryPath))
	rel = strings.TrimPrefix(rel, filepath.ToSlash(filepath.VolumeName(rel)))
	rel = strings.TrimLeft(rel, "/")
	var clean []string
	for _, seg := range strings.Split(rel, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("refusing to write a .dvc file for %q: the path leaves the output directory", entryPath)
		}
		clean = append(clean, seg)
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("refusing to write a .dvc file for %q: no usable path", entryPath)
	}
	return strings.Join(clean, "/"), nil
}

// writeDVCFile creates the .dvc sidecar for one FileEntry at rel+".dvc" inside
// root, keeping an existing one unless opts.Overwrite. It returns 1 when it
// wrote a file.
func writeDVCFile(root *os.Root, rel string, entry FileEntry, uploadID, bucket string, opts *DVCGenerateOptions) (int, error) {
	dvcRel := rel + ".dvc"
	if fi, err := root.Lstat(dvcRel); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("refusing to write %s: it is a symlink", dvcRel)
		}
		if !opts.Overwrite {
			if opts.OnSkip != nil {
				opts.OnSkip(dvcRel)
			}
			return 0, nil
		}
	}
	if err := prepareParents(root, dvcRel); err != nil {
		return 0, err
	}

	out := DVCOutput{
		Path: path.Base(rel),
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

	data, err := yaml.Marshal(DVCFile{Outs: []DVCOutput{out}})
	if err != nil {
		return 0, fmt.Errorf("marshal yaml: %w", err)
	}
	if err := writeContained(root, dvcRel, data); err != nil {
		return 0, err
	}
	return 1, nil
}
