package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// FileDeduplicationIndex provides cross-shard file deduplication for uploads.
// It maintains a global hash table to detect duplicate files across all shards,
// enabling 10-30% space savings for redundant datasets.
//
// Issue #108: Cross-Shard Deduplication
type FileDeduplicationIndex struct {
	// contentHash maps SHA-256 hashes to file locations
	// This is the primary deduplication index
	contentHash map[string]*FileLocation

	// Statistics
	stats DeduplicationStats

	// Thread safety
	mu sync.RWMutex
}

// FileLocation represents where a file is stored in the archive
type FileLocation struct {
	// File hash (SHA-256)
	Hash string

	// Location in archive
	ShardID int    // Which shard contains this file
	ChunkID int    // Which chunk within the shard
	Offset  int64  // Byte offset within the chunk (for extraction)
	S3Key   string // S3 key of the chunk

	// File metadata
	Path    string // Original file path
	Size    int64  // File size in bytes
	ModTime int64  // Modification time (Unix timestamp)

	// Reference counting
	RefCount int32 // Number of times this file is referenced
}

// DeduplicationStats tracks deduplication metrics
type DeduplicationStats struct {
	TotalFiles       int64 // Total files processed
	DuplicateFiles   int64 // Files that were duplicates
	UniqueFiles      int64 // Files that were unique
	BytesSaved       int64 // Total bytes saved by deduplication
	BytesProcessed   int64 // Total bytes processed
	HashComputations int64 // Number of hash computations performed
}

// NewFileDeduplicationIndex creates a new file deduplication index
func NewFileDeduplicationIndex() *FileDeduplicationIndex {
	return &FileDeduplicationIndex{
		contentHash: make(map[string]*FileLocation),
		stats:       DeduplicationStats{},
	}
}

// AddFile attempts to add a file to the deduplication index.
// Returns:
//   - isDuplicate: true if file already exists in index
//   - location: existing location if duplicate, nil otherwise
//   - error: any error that occurred during hashing
func (d *FileDeduplicationIndex) AddFile(filePath string, shardID, chunkID int, s3Key string) (bool, *FileLocation, error) {
	// Compute file hash
	hash, size, modTime, err := d.computeFileHash(filePath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to compute hash for %s: %w", filePath, err)
	}

	// Increment hash computation counter
	atomic.AddInt64(&d.stats.HashComputations, 1)
	atomic.AddInt64(&d.stats.TotalFiles, 1)
	atomic.AddInt64(&d.stats.BytesProcessed, size)

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if file already exists
	if existing, found := d.contentHash[hash]; found {
		// Duplicate found - increment reference count
		atomic.AddInt32(&existing.RefCount, 1)
		atomic.AddInt64(&d.stats.DuplicateFiles, 1)
		atomic.AddInt64(&d.stats.BytesSaved, size)
		return true, existing, nil
	}

	// New unique file - add to index
	location := &FileLocation{
		Hash:     hash,
		ShardID:  shardID,
		ChunkID:  chunkID,
		S3Key:    s3Key,
		Path:     filePath,
		Size:     size,
		ModTime:  modTime,
		RefCount: 1,
	}

	d.contentHash[hash] = location
	atomic.AddInt64(&d.stats.UniqueFiles, 1)

	return false, nil, nil
}

// FindFile looks up a file by its SHA-256 hash
// Returns the file location if found, nil otherwise
func (d *FileDeduplicationIndex) FindFile(hash string) *FileLocation {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.contentHash[hash]
}

// FindFileByPath attempts to find a file by computing its hash
// This is useful for incremental uploads to check if a file has already been uploaded
func (d *FileDeduplicationIndex) FindFileByPath(filePath string) (*FileLocation, error) {
	hash, _, _, err := d.computeFileHash(filePath)
	if err != nil {
		return nil, err
	}
	return d.FindFile(hash), nil
}

// GetStats returns a copy of the current deduplication statistics
func (d *FileDeduplicationIndex) GetStats() DeduplicationStats {
	return DeduplicationStats{
		TotalFiles:       atomic.LoadInt64(&d.stats.TotalFiles),
		DuplicateFiles:   atomic.LoadInt64(&d.stats.DuplicateFiles),
		UniqueFiles:      atomic.LoadInt64(&d.stats.UniqueFiles),
		BytesSaved:       atomic.LoadInt64(&d.stats.BytesSaved),
		BytesProcessed:   atomic.LoadInt64(&d.stats.BytesProcessed),
		HashComputations: atomic.LoadInt64(&d.stats.HashComputations),
	}
}

// DeduplicationRatio returns the percentage of bytes saved (0.0 to 1.0)
func (d *FileDeduplicationIndex) DeduplicationRatio() float64 {
	stats := d.GetStats()
	if stats.BytesProcessed == 0 {
		return 0.0
	}
	return float64(stats.BytesSaved) / float64(stats.BytesProcessed)
}

// Size returns the number of unique files in the index
func (d *FileDeduplicationIndex) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.contentHash)
}

// Clear removes all entries from the index
func (d *FileDeduplicationIndex) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.contentHash = make(map[string]*FileLocation)
}

// computeFileHash computes the SHA-256 hash of a file
// Returns: hash (hex string), size (bytes), modTime (Unix timestamp), error
func (d *FileDeduplicationIndex) computeFileHash(filePath string) (string, int64, int64, error) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	fi, err := file.Stat()
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to stat file: %w", err)
	}

	// Compute SHA-256 hash
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", 0, 0, fmt.Errorf("failed to hash file: %w", err)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, fi.Size(), fi.ModTime().Unix(), nil
}

// ExportToManifest converts the deduplication index to manifest deduplication metadata
// This is called after upload completes to include dedup info in the manifest
func (d *FileDeduplicationIndex) ExportToManifest() *ManifestDeduplication {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := d.GetStats()

	// Convert file locations to manifest format
	fileRefs := make(map[string]FileReference, len(d.contentHash))
	for hash, loc := range d.contentHash {
		fileRefs[hash] = FileReference{
			Hash:     hash,
			ShardID:  loc.ShardID,
			ChunkID:  loc.ChunkID,
			S3Key:    loc.S3Key,
			Size:     loc.Size,
			RefCount: atomic.LoadInt32(&loc.RefCount),
		}
	}

	return &ManifestDeduplication{
		Enabled:          true,
		UniqueFiles:      stats.UniqueFiles,
		DuplicateFiles:   stats.DuplicateFiles,
		BytesSaved:       stats.BytesSaved,
		BytesProcessed:   stats.BytesProcessed,
		DeduplicationPct: d.DeduplicationRatio() * 100,
		FileReferences:   fileRefs,
	}
}

// ManifestDeduplication represents deduplication metadata stored in the manifest
type ManifestDeduplication struct {
	Enabled          bool                     `json:"enabled"`
	UniqueFiles      int64                    `json:"unique_files"`
	DuplicateFiles   int64                    `json:"duplicate_files"`
	BytesSaved       int64                    `json:"bytes_saved"`
	BytesProcessed   int64                    `json:"bytes_processed"`
	DeduplicationPct float64                  `json:"deduplication_pct"` // Percentage of bytes saved
	FileReferences   map[string]FileReference `json:"file_references"`   // Hash -> location mapping
}

// FileReference represents a file reference in the manifest deduplication metadata
type FileReference struct {
	Hash     string `json:"hash"`      // SHA-256 hash
	ShardID  int    `json:"shard_id"`  // Shard containing the file
	ChunkID  int    `json:"chunk_id"`  // Chunk containing the file
	S3Key    string `json:"s3_key"`    // S3 key of the chunk
	Size     int64  `json:"size"`      // File size in bytes
	RefCount int32  `json:"ref_count"` // Number of references to this file
}
