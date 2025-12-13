package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncType constants for manifest sync types (Issue #148)
const (
	SyncTypeFull        = "full"
	SyncTypeIncremental = "incremental"
)

// FileInfo represents local filesystem file information for sync comparison
type FileInfo struct {
	Path    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// DeltaResult represents the result of comparing local files against a manifest
type DeltaResult struct {
	New      []FileInfo // Files that don't exist in previous manifest
	Modified []FileInfo // Files that changed (size or mtime)
	Deleted  []string   // Files in manifest but not in local filesystem
	Same     []FileInfo // Files that haven't changed
}

// SyncOptions configures delta detection behavior (Issue #148)
type SyncOptions struct {
	// UseChecksum enables SHA256 checksum comparison (slower but guaranteed accuracy)
	UseChecksum bool

	// TrackDeletes includes deleted files in delta result
	TrackDeletes bool

	// IgnorePatterns specifies glob patterns to ignore (like .gitignore)
	IgnorePatterns []string
}

// ComputeDelta compares local filesystem state against a previous manifest (Issue #148)
// Returns files that are new, modified, deleted, or unchanged
func ComputeDelta(localFiles []FileInfo, previousManifest *Manifest, opts *SyncOptions) (*DeltaResult, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}

	result := &DeltaResult{
		New:      make([]FileInfo, 0),
		Modified: make([]FileInfo, 0),
		Deleted:  make([]string, 0),
		Same:     make([]FileInfo, 0),
	}

	// Build index of previous manifest files for O(1) lookups
	manifestFiles := make(map[string]FileEntry)
	if previousManifest != nil {
		for _, file := range previousManifest.Files {
			manifestFiles[file.Path] = file
		}
	}

	// Compare local files against manifest
	for _, localFile := range localFiles {
		// Skip directories
		if localFile.IsDir {
			continue
		}

		manifestFile, existsInManifest := manifestFiles[localFile.Path]

		if !existsInManifest {
			// New file - doesn't exist in previous manifest
			result.New = append(result.New, localFile)
		} else {
			// File exists in manifest - check if changed
			if hasChanged(localFile, manifestFile, opts) {
				result.Modified = append(result.Modified, localFile)
			} else {
				result.Same = append(result.Same, localFile)
			}

			// Mark as seen
			delete(manifestFiles, localFile.Path)
		}
	}

	// Track deleted files if requested
	if opts.TrackDeletes {
		for path := range manifestFiles {
			result.Deleted = append(result.Deleted, path)
		}
	}

	return result, nil
}

// hasChanged determines if a local file has changed compared to manifest entry
func hasChanged(local FileInfo, manifest FileEntry, opts *SyncOptions) bool {
	// Size change always indicates modification
	if local.Size != manifest.Size {
		return true
	}

	// ModTime change indicates possible modification
	// Use After() to avoid false positives from clock skew
	if local.ModTime.After(manifest.ModTime) {
		return true
	}

	// TODO: Implement checksum comparison when opts.UseChecksum is true
	// This would require computing SHA256 of local file and comparing
	// to manifest.Checksum (when available)

	return false
}

// ScanLocalFiles scans a directory and returns file information for sync comparison (Issue #148)
func ScanLocalFiles(rootPath string) ([]FileInfo, error) {
	var files []FileInfo

	// Get absolute path for consistent comparisons
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log error but continue walking
			return nil
		}

		// Get relative path from root
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip root directory itself
		if relPath == "." {
			return nil
		}

		files = append(files, FileInfo{
			Path:    relPath,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	return files, nil
}

// FindLatestManifestBySource finds the most recent manifest for a given source path (Issue #148)
// This is used to locate the previous manifest for incremental sync
func (mq *ManifestQuery) FindLatestManifestBySource() *Manifest {
	// For now, just return the manifest we're querying
	// In the future, this would query S3 for all manifests and find the latest
	return mq.manifest
}

// SummaryString returns a human-readable summary of delta results
func (d *DeltaResult) SummaryString() string {
	return fmt.Sprintf("New: %d, Modified: %d, Deleted: %d, Same: %d",
		len(d.New), len(d.Modified), len(d.Deleted), len(d.Same))
}

// TotalChanges returns the count of files that need to be synced (new + modified)
func (d *DeltaResult) TotalChanges() int {
	return len(d.New) + len(d.Modified)
}

// HasChanges returns true if there are any files to sync
func (d *DeltaResult) HasChanges() bool {
	return d.TotalChanges() > 0
}

// GetChangedFiles returns all files that need to be uploaded (new + modified)
func (d *DeltaResult) GetChangedFiles() []FileInfo {
	changed := make([]FileInfo, 0, len(d.New)+len(d.Modified))
	changed = append(changed, d.New...)
	changed = append(changed, d.Modified...)
	return changed
}
