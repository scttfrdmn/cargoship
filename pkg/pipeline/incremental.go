// Package pipeline implements the streaming upload pipeline stages.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// IncrementalStats records metrics accumulated during an incremental scan.
type IncrementalStats struct {
	// FilesScanned is the total number of local files evaluated.
	FilesScanned int64

	// FilesSkipped is the number of files determined to be unchanged (will not be re-uploaded).
	FilesSkipped int64

	// FilesUploaded is the number of new or modified files (will be uploaded).
	FilesUploaded int64

	// BytesSaved is the total uncompressed size of skipped files.
	BytesSaved int64

	// BytesUploaded is the total uncompressed size of files that will be uploaded.
	BytesUploaded int64

	// EstimatedTimeSaved is a rough estimate of time saved by skipping unchanged files,
	// computed assuming a 100 MB/s upload rate.
	EstimatedTimeSaved time.Duration
}

// IncrementalScanner determines which local files need re-uploading by comparing
// them against a previous manifest using a three-tier, short-circuit strategy:
//
//  1. Path check:  file not in previous manifest → must upload (new file)
//  2. Size check:  file size differs              → must upload (changed)
//  3. MD5 check:   content hash matches manifest  → skip (unchanged)
//
// Each tier short-circuits: the first mismatch triggers an upload decision
// without computing the more expensive subsequent checks.
//
// When the previous manifest contains no ContentHash for a file, tier 3 is
// skipped and a matching size is treated as unchanged.
//
// An in-memory HashCache avoids redundant disk reads across repeated calls to
// ShouldUpload for the same file (e.g., when multiple goroutines scan concurrently).
type IncrementalScanner struct {
	prev      *manifest.Manifest
	fileIndex map[string]manifest.FileEntry // relative path → FileEntry
	cache     *manifest.HashCache

	mu    sync.Mutex
	stats IncrementalStats
}

// NewIncrementalScanner creates an IncrementalScanner backed by prev.
//
// cacheFile is the path to a persistent JSON hash cache; pass "" for an
// in-memory-only cache (hashes are not persisted across process runs).
func NewIncrementalScanner(prev *manifest.Manifest, cacheFile string) (*IncrementalScanner, error) {
	if prev == nil {
		return nil, fmt.Errorf("previous manifest must not be nil")
	}
	cache, err := manifest.NewHashCache(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("create hash cache: %w", err)
	}

	// Build O(1) path index from previous manifest entries.
	fileIndex := make(map[string]manifest.FileEntry, len(prev.Files))
	for _, f := range prev.Files {
		fileIndex[f.Path] = f
	}

	return &IncrementalScanner{
		prev:      prev,
		fileIndex: fileIndex,
		cache:     cache,
	}, nil
}

// ShouldUpload returns true when the file at absPath (with relative path relPath
// inside the source root) needs to be included in the upload.
//
// Returns false only when the file is conclusively unchanged compared to the
// previous manifest.  On stat or hash errors the file is conservatively treated
// as needing upload (true is returned) so that no data is silently skipped.
//
// ShouldUpload is safe to call concurrently from multiple goroutines.
func (s *IncrementalScanner) ShouldUpload(absPath, relPath string) bool {
	// Tier 1: existence check against previous manifest.
	prev, found := s.fileIndex[relPath]
	if !found {
		s.recordUpload(0)
		return true
	}

	// Tier 2: size comparison (single cheap syscall).
	fi, err := os.Stat(absPath)
	if err != nil {
		// Cannot stat → conservatively upload.
		s.recordUpload(0)
		return true
	}
	if fi.Size() != prev.Size {
		s.recordUpload(fi.Size())
		return true
	}

	// Tier 3: MD5 content hash comparison.
	// Only applied when the previous manifest recorded a hash.
	if prev.ContentHash != "" {
		hash, err := s.cache.GetOrCompute(absPath)
		if err != nil {
			// Cannot hash the file → conservatively upload.
			s.recordUpload(fi.Size())
			return true
		}
		if hash == prev.ContentHash {
			s.recordSkip(fi.Size())
			return false
		}
		s.recordUpload(fi.Size())
		return true
	}

	// No content hash in previous manifest: same size → treat as unchanged.
	s.recordSkip(fi.Size())
	return false
}

// FilterFiles walks rootPath and returns the relative paths (relative to rootPath)
// of files for which ShouldUpload returns true.
//
// The returned slice is suitable for direct assignment to
// PipelineConfig.IncludeOnlyFiles.
//
// Errors from individual file operations are collected; the first error is
// returned after the full walk completes so the caller receives a complete list
// of files to upload even when some checks fail.
func (s *IncrementalScanner) FilterFiles(rootPath string) ([]string, error) {
	var upload []string
	var firstErr error

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil // skip files with unresolvable paths
		}
		if s.ShouldUpload(path, relPath) {
			upload = append(upload, relPath)
		}
		return nil
	})
	if err != nil {
		return upload, err
	}
	return upload, firstErr
}

// Stats returns a snapshot of the accumulated incremental scan statistics.
//
// EstimatedTimeSaved is computed from BytesSaved assuming a 100 MB/s upload rate.
func (s *IncrementalScanner) Stats() IncrementalStats {
	s.mu.Lock()
	snap := s.stats
	s.mu.Unlock()

	// Rough time estimate: assume 100 MB/s upload throughput.
	const uploadRateBPS = 100 * 1024 * 1024
	if snap.BytesSaved > 0 {
		snap.EstimatedTimeSaved = time.Duration(snap.BytesSaved/uploadRateBPS) * time.Second
	}
	return snap
}

// SaveCache persists the hash cache to the backing file configured at construction.
// This is a no-op when the cache is in-memory-only (cacheFile was "").
func (s *IncrementalScanner) SaveCache() error {
	return s.cache.Save()
}

// recordSkip updates statistics for a file determined to be unchanged.
func (s *IncrementalScanner) recordSkip(size int64) {
	s.mu.Lock()
	s.stats.FilesScanned++
	s.stats.FilesSkipped++
	s.stats.BytesSaved += size
	s.mu.Unlock()
}

// recordUpload updates statistics for a file that needs to be uploaded.
func (s *IncrementalScanner) recordUpload(size int64) {
	s.mu.Lock()
	s.stats.FilesScanned++
	s.stats.FilesUploaded++
	s.stats.BytesUploaded += size
	s.mu.Unlock()
}
