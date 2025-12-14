package migration

import (
	"time"
)

// MigrateRequest contains parameters for a migration operation
type MigrateRequest struct {
	// SourceArchive is the S3 URL of the traditional archive (e.g., s3://bucket/archive.tar.zst)
	SourceArchive string

	// Destination is the S3 URL prefix for the CargoHold upload (e.g., s3://bucket/dataset-sharded)
	Destination string

	// Region is the AWS region for S3 operations
	Region string

	// TempDir is the directory for temporary file extraction (default: OS temp)
	TempDir string

	// KeepTemp preserves extracted files after migration
	KeepTemp bool

	// DeleteOriginal deletes the source archive after successful migration
	DeleteOriginal bool

	// ShardCount is the number of shards for CargoHold (1-100, default: 8)
	ShardCount int

	// CompressionLevel is the zstd compression level (1-22, default: 3)
	CompressionLevel int

	// StorageClass is the S3 storage class (default: STANDARD)
	StorageClass string

	// DryRun performs validation and estimation without executing migration
	DryRun bool

	// Quiet disables progress display
	Quiet bool

	// SkipValidation skips pre-flight validation checks
	SkipValidation bool
}

// MigrateResult contains the result of a migration operation
type MigrateResult struct {
	// Success indicates if the migration completed successfully
	Success bool

	// UploadID is the unique identifier for the CargoHold upload
	UploadID string

	// FilesExtracted is the number of files extracted from the archive
	FilesExtracted int64

	// FilesUploaded is the number of files uploaded to CargoHold
	FilesUploaded int64

	// BytesExtracted is the total bytes extracted (uncompressed)
	BytesExtracted int64

	// BytesUploaded is the total bytes uploaded (compressed)
	BytesUploaded int64

	// ShardsCreated is the number of shards created
	ShardsCreated int

	// ChunksCreated is the number of chunks created
	ChunksCreated int

	// CompressionRatio is the achieved compression ratio
	CompressionRatio float64

	// Duration is the total migration time
	Duration time.Duration

	// ExtractionTime is the time spent extracting
	ExtractionTime time.Duration

	// UploadTime is the time spent uploading
	UploadTime time.Duration

	// ManifestLocation is the S3 URL of the generated manifest
	ManifestLocation string

	// TempDirectory is the temporary extraction directory used
	TempDirectory string

	// OriginalArchiveDeleted indicates if the original was deleted
	OriginalArchiveDeleted bool

	// Errors contains any non-fatal errors encountered
	Errors []error
}

// DryRunEstimate contains cost and resource estimates for a migration
type DryRunEstimate struct {
	// SourceArchiveSize is the compressed archive size
	SourceArchiveSize int64

	// EstimatedUncompressedSize is the estimated uncompressed size
	EstimatedUncompressedSize int64

	// TempDiskSpaceRequired is the disk space needed for extraction
	TempDiskSpaceRequired int64

	// AvailableDiskSpace is the available disk space in temp directory
	AvailableDiskSpace int64

	// EstimatedShards is the number of shards that will be created
	EstimatedShards int

	// EstimatedChunks is the estimated number of chunks
	EstimatedChunks int

	// EstimatedDownloadCost is the estimated AWS data transfer cost for download
	EstimatedDownloadCost float64

	// EstimatedUploadCost is the estimated AWS request cost for upload
	EstimatedUploadCost float64

	// EstimatedStorageCost is the monthly storage cost for the new format
	EstimatedStorageCost float64

	// EstimatedDuration is the estimated migration time
	EstimatedDuration time.Duration

	// CanProceed indicates if migration can proceed (sufficient disk space, etc.)
	CanProceed bool

	// Warnings contains any warnings about the migration
	Warnings []string
}

// S3Location represents a parsed S3 URL
type S3Location struct {
	// Bucket is the S3 bucket name
	Bucket string

	// Key is the S3 object key
	Key string

	// Region is the AWS region (may be empty until resolved)
	Region string

	// IsValid indicates if the location was successfully parsed
	IsValid bool
}

// MigrationPhase represents the current phase of migration
type MigrationPhase string

const (
	PhaseValidation  MigrationPhase = "validation"
	PhaseDryRun      MigrationPhase = "dry_run"
	PhaseExtraction  MigrationPhase = "extraction"
	PhaseUpload      MigrationPhase = "upload"
	PhaseCleanup     MigrationPhase = "cleanup"
	PhaseComplete    MigrationPhase = "complete"
)

// ProgressUpdate represents progress during migration
type ProgressUpdate struct {
	// Phase is the current migration phase
	Phase MigrationPhase

	// Message is a human-readable status message
	Message string

	// BytesProcessed is the number of bytes processed in current phase
	BytesProcessed int64

	// TotalBytes is the total bytes for current phase (0 if unknown)
	TotalBytes int64

	// FilesProcessed is the number of files processed in current phase
	FilesProcessed int64

	// TotalFiles is the total files for current phase (0 if unknown)
	TotalFiles int64

	// ThroughputBytesPerSec is the current throughput
	ThroughputBytesPerSec float64

	// Elapsed is the time elapsed in current phase
	Elapsed time.Duration
}
