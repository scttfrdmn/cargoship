package manifest

import (
	"path/filepath"
	"time"
)

// Manifest represents the complete metadata for an uploaded dataset
type Manifest struct {
	// Version of the manifest format (for future compatibility)
	Version string `json:"version"`

	// Upload metadata
	UploadID    string    `json:"upload_id"`    // Unique upload session ID (timestamp-random)
	CreatedAt   time.Time `json:"created_at"`   // When upload started
	CompletedAt time.Time `json:"completed_at"` // When upload finished

	// Source information
	SourcePath string `json:"source_path"` // Original local path that was uploaded
	Hostname   string `json:"hostname"`    // Machine that performed the upload

	// Sync information (Issue #148)
	PreviousManifestID string `json:"previous_manifest_id,omitempty"` // For version chain in incremental syncs
	SyncType           string `json:"sync_type,omitempty"`            // "full" or "incremental" (empty = full for backwards compat)

	// S3 location
	Bucket string `json:"bucket"` // S3 bucket name
	Prefix string `json:"prefix"` // S3 prefix (key prefix)
	Region string `json:"region"` // AWS region

	// Statistics
	TotalFiles  int64 `json:"total_files"`  // Total number of files
	TotalBytes  int64 `json:"total_bytes"`  // Total uncompressed size
	TotalChunks int   `json:"total_chunks"` // Total number of chunks (archives)
	ShardCount  int   `json:"shard_count"`  // Number of S3 prefix shards

	// Compression
	CompressionType  string  `json:"compression_type"`  // "zstd", "gzip", etc.
	CompressionLevel int     `json:"compression_level"` // Compression level used
	CompressionRatio float64 `json:"compression_ratio"` // Actual compression ratio achieved

	// Encryption (Issue #163)
	Encryption *EncryptionMetadata `json:"encryption,omitempty"` // Encryption configuration if enabled

	// Deduplication (Issue #108)
	Deduplication *ManifestDeduplication `json:"deduplication,omitempty"` // Deduplication metadata if enabled

	// Files - array of all files with their locations
	Files []FileEntry `json:"files"`

	// Chunks - array of all chunks with their metadata
	Chunks []ChunkEntry `json:"chunks"`

	// Shards - array of shard information
	Shards []ShardEntry `json:"shards"`
}

// EncryptionMetadata represents encryption configuration for the manifest and data (Issue #163)
type EncryptionMetadata struct {
	// Enabled indicates if encryption is active
	Enabled bool `json:"enabled"`

	// Data encryption (S3 server-side encryption)
	DataKMSKeyID string `json:"data_kms_key_id,omitempty"` // KMS key ID/ARN for data chunks

	// Manifest encryption (envelope encryption)
	ManifestEncrypted bool   `json:"manifest_encrypted"`           // Whether manifest itself is encrypted
	ManifestKMSKeyID  string `json:"manifest_kms_key_id,omitempty"` // KMS key ID/ARN for manifest
	Algorithm         string `json:"algorithm,omitempty"`            // Encryption algorithm (e.g., "AES-256-GCM")
	EncryptedDEK      string `json:"encrypted_dek,omitempty"`        // Base64-encoded encrypted data encryption key
}

// FileEntry represents a single file in the manifest
type FileEntry struct {
	// File information
	Path    string    `json:"path"`     // Relative path from source root
	Size    int64     `json:"size"`     // File size in bytes
	ModTime time.Time `json:"mod_time"` // Last modification time

	// Location in archive
	ChunkID int    `json:"chunk_id"` // Which chunk contains this file
	ShardID int    `json:"shard_id"` // Which shard the chunk is in
	S3Key   string `json:"s3_key"`   // Full S3 key for the chunk

	// File splitting information (Phase 5)
	Offset     int64 `json:"offset,omitempty"`      // Start offset for partial files (0 = full file)
	Length     int64 `json:"length,omitempty"`      // Length of this part (0 = full file)
	PartIndex  int   `json:"part_index,omitempty"`  // Part index for split files (0 = not split)
	TotalParts int   `json:"total_parts,omitempty"` // Total parts if split (0 or 1 = not split)

	// Optional metadata
	Checksum string            `json:"checksum,omitempty"` // SHA256 checksum (optional)
	Metadata map[string]string `json:"metadata,omitempty"` // Additional metadata

	// Deduplication (Issue #108)
	IsDuplicate       bool   `json:"is_duplicate,omitempty"`        // True if this file is a duplicate
	DuplicateOfHash   string `json:"duplicate_of_hash,omitempty"`   // Hash of the original file
	OriginalChunkID   int    `json:"original_chunk_id,omitempty"`   // Chunk ID of the original file
	OriginalShardID   int    `json:"original_shard_id,omitempty"`   // Shard ID of the original file
	OriginalS3Key     string `json:"original_s3_key,omitempty"`     // S3 key of the original file
	DeduplicationRefs int32  `json:"deduplication_refs,omitempty"`  // Number of duplicates referencing this file
}

// ChunkEntry represents a single chunk (tar.zst archive) in the manifest
type ChunkEntry struct {
	// Chunk identification
	ID      int    `json:"id"`       // Chunk ID
	ShardID int    `json:"shard_id"` // Which shard this chunk belongs to
	S3Key   string `json:"s3_key"`   // Full S3 key (e.g., "uploads/20251206-abc123/shard-0/chunk-0.tar.zst")

	// Contents
	FileCount        int      `json:"file_count"`        // Number of files in this chunk
	FilePaths        []string `json:"file_paths"`        // Paths of files in this chunk (for quick lookup)
	UncompressedSize int64    `json:"uncompressed_size"` // Total uncompressed size
	CompressedSize   int64    `json:"compressed_size"`   // Actual compressed size in S3

	// Timestamps
	CreatedAt  time.Time `json:"created_at"`  // When chunk was created
	UploadedAt time.Time `json:"uploaded_at"` // When upload to S3 completed

	// Checksums
	Checksum string `json:"checksum,omitempty"` // SHA256 of compressed archive
}

// ShardEntry represents a shard (S3 prefix) in the manifest
type ShardEntry struct {
	// Shard identification
	ID     int    `json:"id"`     // Shard ID (0-7 for default 8 shards)
	Prefix string `json:"prefix"` // S3 prefix path (e.g., "uploads/20251206-abc123/shard-0")

	// Statistics
	ChunkCount       int   `json:"chunk_count"`       // Number of chunks in this shard
	FileCount        int64 `json:"file_count"`        // Total files across all chunks
	UncompressedSize int64 `json:"uncompressed_size"` // Total uncompressed size
	CompressedSize   int64 `json:"compressed_size"`   // Total compressed size

	// S3 keys
	ChunkKeys []string `json:"chunk_keys"` // All chunk S3 keys in this shard
}

// ManifestQuery provides query capabilities for the manifest
type ManifestQuery struct {
	manifest  *Manifest
	fileIndex map[string]*FileEntry // O(1) lookup index for files by path
}

// NewManifestQuery creates a new query interface for a manifest
func NewManifestQuery(m *Manifest) *ManifestQuery {
	// Build file index for O(1) lookups
	fileIndex := make(map[string]*FileEntry, len(m.Files))
	for i := range m.Files {
		fileIndex[m.Files[i].Path] = &m.Files[i]
	}

	return &ManifestQuery{
		manifest:  m,
		fileIndex: fileIndex,
	}
}

// FindFile finds a file by exact path using O(1) hash map lookup
func (mq *ManifestQuery) FindFile(path string) *FileEntry {
	return mq.fileIndex[path]
}

// ListFiles returns all files, optionally filtered by glob pattern
// Pattern matching uses filepath.Match syntax:
//   - "*" matches any sequence of non-separator characters
//   - "?" matches any single non-separator character
//   - "[...]" matches any character in the set
//
// Examples:
//   - "*.log" matches all files ending in .log
//   - "data/*.csv" matches all CSV files in data directory
//   - "**/*.json" would require custom implementation (not supported by filepath.Match)
func (mq *ManifestQuery) ListFiles(pattern string) []FileEntry {
	if pattern == "" {
		return mq.manifest.Files
	}

	// Filter files by glob pattern
	var matches []FileEntry
	for _, file := range mq.manifest.Files {
		// Try matching against full path first
		matched, err := filepath.Match(pattern, file.Path)
		if err != nil {
			// Invalid pattern - skip this file
			continue
		}
		if matched {
			matches = append(matches, file)
			continue
		}

		// Also try matching against basename for convenience
		// This allows "*.log" to match "dir/file.log"
		basename := filepath.Base(file.Path)
		matched, err = filepath.Match(pattern, basename)
		if err == nil && matched {
			matches = append(matches, file)
		}
	}
	return matches
}

// FilesInShard returns all files in a specific shard
func (mq *ManifestQuery) FilesInShard(shardID int) []FileEntry {
	var files []FileEntry
	for _, file := range mq.manifest.Files {
		if file.ShardID == shardID {
			files = append(files, file)
		}
	}
	return files
}

// FilesInChunk returns all files in a specific chunk
func (mq *ManifestQuery) FilesInChunk(chunkID int) []FileEntry {
	var files []FileEntry
	for _, file := range mq.manifest.Files {
		if file.ChunkID == chunkID {
			files = append(files, file)
		}
	}
	return files
}

// GetSummary returns a summary of the manifest
func (mq *ManifestQuery) GetSummary() ManifestSummary {
	return ManifestSummary{
		TotalFiles:       mq.manifest.TotalFiles,
		TotalBytes:       mq.manifest.TotalBytes,
		TotalChunks:      mq.manifest.TotalChunks,
		ShardCount:       mq.manifest.ShardCount,
		CompressionRatio: mq.manifest.CompressionRatio,
		UploadID:         mq.manifest.UploadID,
		CreatedAt:        mq.manifest.CreatedAt,
		CompletedAt:      mq.manifest.CompletedAt,
	}
}

// ManifestSummary provides a high-level summary
type ManifestSummary struct {
	TotalFiles       int64
	TotalBytes       int64
	TotalChunks      int
	ShardCount       int
	CompressionRatio float64
	UploadID         string
	CreatedAt        time.Time
	CompletedAt      time.Time
}

// GetShard returns shard metadata for the specified shard ID (Issue #90)
// Returns nil if shard ID is invalid
func (mq *ManifestQuery) GetShard(shardID int) *ShardEntry {
	for i := range mq.manifest.Shards {
		if mq.manifest.Shards[i].ID == shardID {
			return &mq.manifest.Shards[i]
		}
	}
	return nil
}

// CountFiles returns the total number of files in the manifest (Issue #90)
func (mq *ManifestQuery) CountFiles() int64 {
	return mq.manifest.TotalFiles
}

// TotalSize returns the total uncompressed size of all files in bytes (Issue #90)
func (mq *ManifestQuery) TotalSize() int64 {
	return mq.manifest.TotalBytes
}
