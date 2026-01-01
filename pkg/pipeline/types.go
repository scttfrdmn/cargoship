package pipeline

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/config"
)

// Job represents a unit of work flowing through the pipeline
type Job struct {
	ID          int               // Job identifier
	Chunk       chunking.Chunk    // Chunk to process
	Archive     io.ReadCloser     // Streamed archive (from archiver)
	ArchiveSize int64             // Size of archive
	S3Key       string            // S3 destination key
	Metadata    map[string]string // Additional metadata
	Error       error             // Error if job failed
	StartTime   time.Time         // When job started
	EndTime     time.Time         // When job completed

	// Phase 3.3: Compressed-aware chunking with adaptive sizing
	TargetCompressedSize int64 // Target compressed size from CompressedAwareChunker (0 = no target)
	EstimatedCompressed  int64 // Estimated compressed size from CompressionEstimator

	// Issue #103: Enhanced error reporting with shard context
	ShardID       int     // Shard identifier (e.g., 0, 1, 2, ..., 7 for 8 shards)
	ShardPrefix   string  // Shard prefix (e.g., "shard-0", "shard-1", ...)
	AttemptNumber int     // Current retry attempt (1 = first attempt, 2+ = retries)
	ErrorHistory  []error // History of errors from retry attempts

	// Issue #34 Phase 1.1: BufferedPipe tracking for pool return
	pipeReader *BufferedPipeReader // Reader side of pipe (for pool cleanup)
	pipeWriter *BufferedPipeWriter // Writer side of pipe (for pool cleanup)
	pipePool   *BufferedPipePool   // Pool to return pipe to after upload
}

// Stage represents a pipeline stage
type Stage interface {
	// Name returns the stage name for logging/metrics
	Name() string

	// Process processes a job and sends it to the next stage
	Process(ctx context.Context, job *Job) error

	// Start starts the stage workers
	Start(ctx context.Context) error

	// Stop stops the stage and waits for completion
	Stop() error

	// Stats returns stage statistics
	Stats() StageStats
}

// StageStats contains statistics for a pipeline stage
type StageStats struct {
	Name           string                 // Stage name
	JobsProcessed  int64                  // Total jobs processed
	JobsFailed     int64                  // Total jobs failed
	BytesProcessed int64                  // Total bytes processed
	TotalTime      time.Duration          // Total processing time
	AverageTime    time.Duration          // Average time per job
	ActiveWorkers  int                    // Current active workers
	QueuedJobs     int                    // Jobs waiting in queue
	Metadata       map[string]interface{} // Additional stage-specific metadata (Phase 3.2: shard distribution, etc.)
}

// Pipeline orchestrates the entire streaming pipeline
type Pipeline struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex

	// Configuration
	config *PipelineConfig

	// Stages
	scanner        *ScannerStage
	archiver       *ArchiverStage
	uploader       *UploaderStage         // Simulated uploader (for testing)
	s3Uploader     *S3UploaderStage       // Real AWS S3 uploader (single-prefix)
	directUploader *DirectUploaderStage   // Issue #166: Direct uploader (fast path for small files)

	// Phase 3: Multi-prefix parallel upload stages
	router              *PrefixRouter               // Routes jobs to per-prefix channels
	multiPrefixUploader *S3MultiPrefixUploaderStage // Per-prefix worker pools

	// Observability (Issue #155)
	tracer interface{} // *tracing.PipelineTracer for distributed tracing (type: *github.com/scttfrdmn/cargoship/pkg/observability/tracing.PipelineTracer)

	// Channels for communication between stages
	chunkChan   chan *Job
	archiveChan chan *Job
	resultChan  chan *Job

	// Phase 3: Per-prefix channels for parallel uploads
	prefixChans map[string]chan *Job // Key: "shard-N", Value: channel

	// Progress tracking
	progress *ProgressTracker

	// Errors
	errors []error

	// Manifest tracking
	manifestBuilder interface{} // *manifest.Builder for tracking files/chunks (type: *github.com/scttfrdmn/cargoship/pkg/manifest.Builder)
	manifestMu      sync.Mutex  // Protects manifest updates

	// Local state tracking (Issue #119: Enhanced resume)
	uploadState   interface{} // *resume.UploadState for local state persistence (type: *github.com/scttfrdmn/cargoship/pkg/resume.UploadState)
	uploadStateMu sync.Mutex  // Protects uploadState updates

	// Cleanup tracking (Issue #158)
	uploadedKeys   []string   // All uploaded S3 keys for cleanup on failure
	uploadedKeysMu sync.Mutex // Protects uploadedKeys list
}

// PipelineConfig contains configuration for the pipeline
type PipelineConfig struct {
	// Worker counts for each stage
	ScannerWorkers  int
	ArchiverWorkers int
	UploaderWorkers int

	// Buffer sizes for channels
	ChunkBufferSize   int
	ArchiveBufferSize int
	ResultBufferSize  int

	// S3 configuration
	S3Bucket string
	S3Prefix string
	S3Region string

	// Real S3 uploader configuration (optional)
	UseRealS3      bool                 // If true, use real AWS S3 uploader instead of simulated
	S3Client       interface{}          // *s3.Client for real uploads (type: *github.com/aws/aws-sdk-go-v2/service/s3.Client)
	S3PartSize     int64                // S3 multipart part size (default: 64MB)
	S3StorageClass string               // S3 storage class (STANDARD, INTELLIGENT_TIERING, etc.)
	TierSelector   *StorageTierSelector // Issue #32: Automatic storage tier selection (nil = disabled, uses S3StorageClass)
	S3SSEKMSKeyId  string               // Optional KMS key ID for encryption

	// Encryption configuration (Issue #163)
	KMSKeyID        string      // KMS key ID or ARN for data chunk encryption
	EncryptManifest bool        // Enable KMS envelope encryption for manifest
	KMSClient       interface{} // *kms.Client for manifest encryption (type: *github.com/aws/aws-sdk-go-v2/service/kms.Client)

	// Advanced transporter configuration (v0.6.2)
	// If set, uses advanced S3 transporters (staging, adaptive, optimized) instead of basic manager.Uploader
	// Set via NewPipelineTransporter() factory
	Transporter interface{} // s3transport.BasicTransporter for advanced uploads (type: github.com/scttfrdmn/cargoship/pkg/aws/s3.BasicTransporter)

	// Multi-prefix optimization (Phase 3)
	EnableMultiPrefix bool   // If true, use multi-prefix parallel uploads (default: true)
	WorkersPerPrefix  int    // Workers per S3 prefix (default: 2)
	UploadID          string // Unique identifier for this upload session (format: {timestamp}-{random})
	ShardCount        int    // Number of S3 prefix shards for parallel uploads (default: 8)

	// Phase 3.2: Archiver-level sharding (eliminates router bottleneck)
	EnableArchiverSharding bool // If true, archiver shards directly to per-prefix channels (no router)

	// Phase 3.3: Compressed-aware chunking with adaptive sizing and padding
	EnableCompressedAwareChunking bool    // Enable compression-aware chunking (default: true)
	EnableArchivePadding          bool    // Enable padding to reach target sizes (default: true)
	MaxPaddingRatio               float64 // Maximum padding ratio (default: 0.25 = 25%)
	ForceChunkSizeMB              int     // Override adaptive sizing (0 = adaptive, default: 0)

	// Chunking configuration
	ChunkingConfig *chunking.ChunkingConfig

	// Manifest configuration
	EnableManifest bool   // Enable manifest generation (default: true for real S3)
	SourcePath     string // Original source path for manifest

	// Incremental sync configuration (Issue #148)
	IncludeOnlyFiles []string // If set, only upload these files (for incremental sync)
	SyncType         string   // "full" or "incremental" (for manifest)
	PreviousUploadID string   // Previous upload ID (for manifest chaining)

	// Progress tracking
	EnableProgress   bool
	ProgressInterval time.Duration

	// Timeouts
	StageTimeout time.Duration

	// Resume configuration (Issue #157: Resume capability)
	ResumeMode     bool   // Enable resume capability
	ResumeUploadID string // Upload ID to resume (empty = auto-detect)
	SkipExisting   bool   // Skip chunks that exist in S3 (HeadObject check)

	// Partial manifest saving (Issue #157: Resume capability)
	EnablePartialManifest       bool          // Enable periodic manifest saves (default: true for real S3)
	PartialManifestSaveInterval time.Duration // How often to save partial manifest (default: 30s)

	// Local state persistence (Issue #119: Enhanced resume with local state)
	EnableLocalState       bool          // Enable local state file persistence (default: true)
	LocalStateSaveInterval time.Duration // How often to save local state (default: 30s)
	DisableFileHashing     bool          // Skip file hashing for change detection (faster but less safe)

	// Direct upload optimization (Issue #166: Small file optimization)
	EnableDirectUpload        bool    // Enable fast path for small files (bypasses archiving/compression)
	DirectUploadThresholdMB   int     // Max total size for direct upload mode (default: 500MB)
	DirectUploadMaxFiles      int     // Max file count for direct upload (default: 50000)
	DirectUploadAvgSizeMB     float64 // Max average file size for direct upload (default: 5MB)
	DirectUploadWorkers       int     // Worker count for direct upload (default: 256, matches s5cmd)
	ForceDirectUpload         bool    // Force direct upload regardless of thresholds (for testing)
	EnableAutoDirectUpload    bool    // Auto-enable direct upload when thresholds met (default: true)

	// Cleanup configuration (Issue #158: Automatic cleanup on failure)
	CleanupOnFailure bool // Automatically delete partial uploads on error (default: true)

	// Observability configuration (Issue #155: Distributed tracing)
	EnableTracing bool        // Enable distributed tracing with OpenTelemetry (default: false)
	Tracer        interface{} // Optional: Pre-configured tracer (type: *github.com/scttfrdmn/cargoship/pkg/observability/tracing.PipelineTracer)
}

// Progress represents current pipeline progress
type Progress struct {
	// Totals
	TotalFiles  int64
	TotalBytes  int64
	TotalChunks int

	// Completed
	FilesProcessed  int64
	BytesProcessed  int64
	ChunksCompleted int

	// In progress
	ChunksInProgress int
	BytesInFlight    int64

	// Time tracking
	StartTime    time.Time
	ElapsedTime  time.Duration
	EstimatedETA time.Duration

	// Throughput
	FilesPerSecond float64
	BytesPerSecond float64

	// Stage breakdown
	ScanTime    time.Duration
	ArchiveTime time.Duration
	UploadTime  time.Duration

	// Errors
	ErrorCount int
	LastError  error
}

// ProgressTracker tracks pipeline progress
type ProgressTracker struct {
	mu       sync.RWMutex
	progress Progress
	callback func(Progress)
}

// Result represents the final result of a pipeline execution
type Result struct {
	Success        bool
	TotalFiles     int64
	TotalBytes     int64
	ChunksCreated  int
	ChunksUploaded int
	ChunksSkipped  int // Issue #157: Number of chunks skipped (already uploaded)
	TotalChunks    int // Issue #157: Total chunks (skipped + uploaded + created)
	TotalTime      time.Duration
	Errors         []error
	Progress       Progress
	FailedJobs     []*Job   // Issue #103: Track failed jobs for detailed error reporting
	UploadedKeys   []string // Issue #158: Track all uploaded S3 keys for cleanup on failure
	UploadID       string   // Issue #158: Upload ID for cleanup operations
}

// ScannerConfig configures the scanner stage
type ScannerConfig struct {
	RootPath        string
	Workers         int
	FollowSymlinks  bool
	ExcludePatterns []string

	// Issue #148: Incremental sync file filtering
	IncludeOnlyFiles []string // If set, only scan these files (relative paths)

	// Phase 3.3: Compressed-aware chunking
	UseCompressedAwareChunking bool // Enable compression-aware chunking
	ChunkTargetSizeMB          int  // Manual override (0 = adaptive)

	// Phase 5: Chunking configuration (optional, will use defaults if nil)
	ChunkingConfig *chunking.ChunkingConfig

	// Issue #30: Magika configuration for AI file type detection (optional)
	MagikaConfig *config.MagikaConfig
}

// ArchiverConfig configures the archiver stage
type ArchiverConfig struct {
	Workers          int
	CompressionLevel int
	CompressionType  string // "gzip", "zstd", "bzip2"
	BufferSize       int

	// Multi-prefix optimization (Phase 3)
	UploadID   string // Unique identifier for this upload session (format: {timestamp}-{random})
	ShardCount int    // Number of S3 prefix shards for parallel uploads (default: 8)

	// Phase 3.3: Archive padding for uniform compressed chunk sizes
	EnablePadding        bool    // Enable zero-byte padding to reach target compressed sizes
	MaxPaddingRatio      float64 // Maximum allowed padding ratio (default: 0.25 = 25%)
	UseLowEntropyPadding bool    // Use low-entropy (zero-byte) padding (default: true for S3 optimization)
}

// UploaderConfig configures the uploader stage
type UploaderConfig struct {
	Workers     int
	PartSize    int64 // Size of each multipart upload part
	Concurrency int   // Concurrent parts per upload
	MaxRetries  int
	RetryDelay  time.Duration
}

// WorkerPool manages a pool of workers for a stage
type WorkerPool struct {
	workers   int
	semaphore chan struct{}
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(ctx context.Context, workers int) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)
	return &WorkerPool{
		workers:   workers,
		semaphore: make(chan struct{}, workers),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Submit submits work to the pool
func (p *WorkerPool) Submit(fn func(context.Context) error) error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case p.semaphore <- struct{}{}:
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer func() { <-p.semaphore }()
			_ = fn(p.ctx)
		}()
		return nil
	}
}

// Wait waits for all workers to complete
func (p *WorkerPool) Wait() {
	p.wg.Wait()
}

// Stop stops the worker pool
func (p *WorkerPool) Stop() {
	p.cancel()
	p.wg.Wait()
}

// AvailableWorkers returns the number of available workers
func (p *WorkerPool) AvailableWorkers() int {
	return len(p.semaphore)
}
