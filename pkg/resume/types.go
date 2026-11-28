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

	// Settings are the rest of the upload's options, which a resume must
	// reproduce (#1011). Nil in a state saved before they were recorded; a
	// resume then uses the upload command's defaults.
	Settings *UploadSettings `json:"settings,omitempty"`

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

// UploadSettings records the upload options that decide what a resumed upload
// stores and how (#1011). `cargoship resume <id>` rebuilt its pipeline without
// them, so a resumed upload lost its per-file checksums, frame size and
// compression and shard settings, uploaded files its exclusions skip, dropped
// its incremental change set and dataset chain identity, and lost its writer,
// project and dedup settings.
//
// Every field is written as recorded; FileChecksums and FrameSize are written
// even when zero, so an explicit --no-file-checksums or --frame-size 0 survives.
type UploadSettings struct {
	FileChecksums    bool   `json:"file_checksums"`
	FrameSize        int64  `json:"frame_size"`
	CompressionLevel int    `json:"compression_level,omitempty"`
	ShardStrategy    string `json:"shard_strategy,omitempty"`

	ExcludePatterns  []string `json:"exclude_patterns,omitempty"`
	IncludeOnlyFiles []string `json:"include_only_files,omitempty"`

	// Incremental and dataset chain identity, needed whenever IncludeOnlyFiles
	// is: a resumed change set without them would be a "full" version holding
	// only the changed files.
	SyncType         string   `json:"sync_type,omitempty"`
	PreviousUploadID string   `json:"previous_upload_id,omitempty"`
	DeletedPaths     []string `json:"deleted_paths,omitempty"`
	DatasetID        string   `json:"dataset_id,omitempty"`
	VersionOrdinal   int      `json:"version_ordinal,omitempty"`

	WriterID            string            `json:"writer_id,omitempty"`
	ProjectID           string            `json:"project_id,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
	EnableDeduplication bool              `json:"enable_deduplication,omitempty"`

	// --auto-tier: the tier selector (nil when off) and the chunking strategy.
	TierChunkingStrategy string        `json:"tier_chunking_strategy,omitempty"`
	Tier                 *TierSettings `json:"tier,omitempty"`
}

// TierSettings records an --auto-tier upload's tier selector.
type TierSettings struct {
	HotDays         int    `json:"hot_days"`
	ColdDays        int    `json:"cold_days"`
	ArchiveDays     int    `json:"archive_days"`
	MaxTier         string `json:"max_tier,omitempty"`
	DefaultClass    string `json:"default_class,omitempty"`
	FallbackToMtime bool   `json:"fallback_to_mtime"`
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
