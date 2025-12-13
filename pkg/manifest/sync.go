package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

// FindLatestManifestForSource finds the most recent manifest for a given source path from S3 (Issue #148)
// This searches through all manifests in the bucket/prefix and returns the most recent one
// that matches the source path. Returns nil if no matching manifest is found.
func FindLatestManifestForSource(ctx context.Context, s3Client *s3.Client, bucket, prefix, sourcePath string) (*Manifest, error) {
	// Get absolute path for consistent comparison
	absSourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute source path: %w", err)
	}

	// List all manifests in the bucket/prefix
	manifests, err := listAllManifests(ctx, s3Client, bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list manifests: %w", err)
	}

	// Find manifests with matching source path
	var candidates []*Manifest
	for _, manifest := range manifests {
		// Get absolute path of manifest source for comparison
		manifestAbsPath, err := filepath.Abs(manifest.SourcePath)
		if err != nil {
			// Skip if we can't get absolute path
			continue
		}

		if manifestAbsPath == absSourcePath {
			candidates = append(candidates, manifest)
		}
	}

	if len(candidates) == 0 {
		return nil, nil // No matching manifest found
	}

	// Find the most recent manifest by CreatedAt
	latest := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.CreatedAt.After(latest.CreatedAt) {
			latest = candidate
		}
	}

	return latest, nil
}

// listAllManifests lists all manifests in the specified S3 bucket/prefix
func listAllManifests(ctx context.Context, s3Client *s3.Client, bucket, prefix string) ([]*Manifest, error) {
	var manifests []*Manifest

	// List all objects under prefix/uploads/
	listPrefix := prefix
	if listPrefix != "" && !strings.HasSuffix(listPrefix, "/") {
		listPrefix += "/"
	}
	listPrefix += "uploads/"

	// Use S3 ListObjectsV2 to find all manifest files
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(listPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)

			// Check if this is a manifest file (manifest.json or manifest.json.gz)
			if !strings.HasSuffix(key, ManifestFileName) && !strings.HasSuffix(key, ManifestFileNameGZ) {
				continue
			}

			// Extract upload ID from key (format: prefix/uploads/{uploadID}/manifest.json)
			parts := strings.Split(key, "/")
			if len(parts) < 3 {
				continue
			}

			// Find "uploads" in path and get next part as upload ID
			uploadIDIdx := -1
			for i, part := range parts {
				if part == "uploads" && i+1 < len(parts) {
					uploadIDIdx = i + 1
					break
				}
			}

			if uploadIDIdx == -1 {
				continue
			}

			uploadID := parts[uploadIDIdx]

			// Download and parse the manifest
			manifest, err := DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
			if err != nil {
				// Skip manifests that fail to download or parse
				continue
			}

			manifests = append(manifests, manifest)
		}
	}

	return manifests, nil
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

// SyncPlan represents a plan for incremental sync operation (Issue #148)
type SyncPlan struct {
	SourcePath         string       // Local path being synced
	PreviousManifest   *Manifest    // Previous manifest (nil for first sync)
	Delta              *DeltaResult // Files that need to be synced
	IsFirstSync        bool         // True if no previous manifest exists
	PreviousManifestID string       // Upload ID of previous manifest
	EstimatedDataSize  int64        // Estimated total size of files to upload
}

// PrepareSyncPlan analyzes the current state and prepares a sync plan (Issue #148)
// This is the main entry point for incremental sync planning
func PrepareSyncPlan(ctx context.Context, s3Client *s3.Client, bucket, prefix, sourcePath string, opts *SyncOptions) (*SyncPlan, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}

	plan := &SyncPlan{
		SourcePath: sourcePath,
	}

	// Find latest manifest for this source path
	previousManifest, err := FindLatestManifestForSource(ctx, s3Client, bucket, prefix, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find previous manifest: %w", err)
	}

	plan.PreviousManifest = previousManifest
	plan.IsFirstSync = (previousManifest == nil)

	if previousManifest != nil {
		plan.PreviousManifestID = previousManifest.UploadID
	}

	// Scan local filesystem
	localFiles, err := ScanLocalFiles(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan local files: %w", err)
	}

	// Compute delta against previous manifest
	delta, err := ComputeDelta(localFiles, previousManifest, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to compute delta: %w", err)
	}

	plan.Delta = delta

	// Calculate estimated data size
	for _, file := range delta.GetChangedFiles() {
		plan.EstimatedDataSize += file.Size
	}

	return plan, nil
}

// Summary returns a human-readable summary of the sync plan
func (sp *SyncPlan) Summary() string {
	if sp.IsFirstSync {
		return fmt.Sprintf("First sync: uploading all files (%d files, %d bytes)",
			sp.Delta.TotalChanges(), sp.EstimatedDataSize)
	}

	return fmt.Sprintf("Incremental sync: %s (estimated size: %d bytes, previous manifest: %s)",
		sp.Delta.SummaryString(), sp.EstimatedDataSize, sp.PreviousManifestID)
}
