package resume

import "time"

// UploadState represents the persistent state of an upload session
// Saved locally to ~/.cargoship/state/{upload-id}.json for fast resume
type UploadState struct {
	// Upload identification
	UploadID  string    `json:"upload_id"`
	StartTime time.Time `json:"start_time"`
	LastSave  time.Time `json:"last_save_time"`

	// Source and destination
	SourceDir string `json:"source_dir"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Region    string `json:"region,omitempty"`

	// Upload configuration
	StorageClass    string `json:"storage_class,omitempty"`
	KMSKeyID        string `json:"kms_key_id,omitempty"`
	EncryptManifest bool   `json:"encrypt_manifest"`
	ChunkSizeMB     int    `json:"chunk_size_mb,omitempty"`
	ShardCount      int    `json:"shard_count,omitempty"`

	// Progress tracking
	TotalFiles     int64 `json:"total_files"`
	TotalBytes     int64 `json:"total_bytes"`
	CompletedFiles int64 `json:"completed_files"`
	CompletedBytes int64 `json:"completed_bytes"`

	// Shard-level state
	Shards []ShardState `json:"shards,omitempty"`

	// File-level hashing for change detection (optional, can be large)
	FileHashes map[string]string `json:"file_hashes,omitempty"`
}

// ShardState tracks the state of a single shard during upload
type ShardState struct {
	ShardID         int   `json:"shard_id"`
	CompletedFiles  int64 `json:"completed_files"`
	CompletedBytes  int64 `json:"completed_bytes"`
	CompletedChunks int   `json:"completed_chunks,omitempty"`

	// Track in-progress multipart uploads for resume
	MultipartUploads []MultipartState `json:"multipart_uploads,omitempty"`
}

// MultipartState tracks an S3 multipart upload for resume capability
type MultipartState struct {
	S3Key          string         `json:"s3_key"`
	UploadID       string         `json:"upload_id"`
	CompletedParts []int          `json:"completed_parts,omitempty"`
	PartETags      map[int]string `json:"part_etags,omitempty"`
}

// Progress returns the upload progress as a percentage (0-100)
func (s *UploadState) Progress() float64 {
	if s.TotalBytes == 0 {
		return 0
	}
	return (float64(s.CompletedBytes) / float64(s.TotalBytes)) * 100
}

// IsComplete returns true if the upload is 100% complete
func (s *UploadState) IsComplete() bool {
	return s.TotalFiles > 0 && s.CompletedFiles >= s.TotalFiles
}

// Age returns how long ago the upload started
func (s *UploadState) Age() time.Duration {
	return time.Since(s.StartTime)
}

// TimeSinceLastSave returns how long since the last state save
func (s *UploadState) TimeSinceLastSave() time.Duration {
	return time.Since(s.LastSave)
}
