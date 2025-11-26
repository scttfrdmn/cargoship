package pipeline

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// Job represents a unit of work flowing through the pipeline
type Job struct {
	ID          int              // Job identifier
	Chunk       chunking.Chunk   // Chunk to process
	Archive     io.ReadCloser    // Streamed archive (from archiver)
	ArchiveSize int64            // Size of archive
	S3Key       string           // S3 destination key
	Metadata    map[string]string // Additional metadata
	Error       error            // Error if job failed
	StartTime   time.Time        // When job started
	EndTime     time.Time        // When job completed
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
	Name          string        // Stage name
	JobsProcessed int64         // Total jobs processed
	JobsFailed    int64         // Total jobs failed
	BytesProcessed int64        // Total bytes processed
	TotalTime     time.Duration // Total processing time
	AverageTime   time.Duration // Average time per job
	ActiveWorkers int           // Current active workers
	QueuedJobs    int           // Jobs waiting in queue
}

// Pipeline orchestrates the entire streaming pipeline
type Pipeline struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex

	// Configuration
	config *PipelineConfig

	// Stages
	scanner  *ScannerStage
	archiver *ArchiverStage
	uploader *UploaderStage     // Simulated uploader (for testing)
	s3Uploader *S3UploaderStage // Real AWS S3 uploader

	// Channels for communication between stages
	chunkChan   chan *Job
	archiveChan chan *Job
	resultChan  chan *Job

	// Progress tracking
	progress *ProgressTracker

	// Errors
	errors []error
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
	UseRealS3     bool        // If true, use real AWS S3 uploader instead of simulated
	S3Client      interface{} // *s3.Client for real uploads (type: *github.com/aws/aws-sdk-go-v2/service/s3.Client)
	S3PartSize    int64       // S3 multipart part size (default: 64MB)
	S3StorageClass string     // S3 storage class (STANDARD, INTELLIGENT_TIERING, etc.)
	S3SSEKMSKeyId string      // Optional KMS key ID for encryption

	// Multi-prefix optimization (Phase 3)
	UploadID   string // Unique identifier for this upload session (format: {timestamp}-{random})
	ShardCount int    // Number of S3 prefix shards for parallel uploads (default: 8)

	// Chunking configuration
	ChunkingConfig *chunking.ChunkingConfig

	// Progress tracking
	EnableProgress bool
	ProgressInterval time.Duration

	// Timeouts
	StageTimeout time.Duration
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
	StartTime   time.Time
	ElapsedTime time.Duration
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
	TotalTime      time.Duration
	Errors         []error
	Progress       Progress
}

// ScannerConfig configures the scanner stage
type ScannerConfig struct {
	RootPath      string
	Workers       int
	FollowSymlinks bool
	ExcludePatterns []string
}

// ArchiverConfig configures the archiver stage
type ArchiverConfig struct {
	Workers        int
	CompressionLevel int
	CompressionType  string // "gzip", "zstd", "bzip2"
	BufferSize     int

	// Multi-prefix optimization (Phase 3)
	UploadID   string // Unique identifier for this upload session (format: {timestamp}-{random})
	ShardCount int    // Number of S3 prefix shards for parallel uploads (default: 8)
}

// UploaderConfig configures the uploader stage
type UploaderConfig struct {
	Workers       int
	PartSize      int64 // Size of each multipart upload part
	Concurrency   int   // Concurrent parts per upload
	MaxRetries    int
	RetryDelay    time.Duration
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
