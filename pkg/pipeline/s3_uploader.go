// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// S3UploaderConfig configures the real S3 uploader stage
type S3UploaderConfig struct {
	Workers              int           // Number of concurrent upload workers (default: 8, matches shard count)
	PartSize             int64         // S3 multipart part size (default: 64MB)
	MaxRetries           int           // Maximum upload retry attempts
	RetryDelay           time.Duration // Delay between retries
	S3Client             *s3.Client    // AWS S3 client
	Bucket               string        // Target S3 bucket
	Prefix               string        // S3 key prefix (optional)
	StorageClass         types.StorageClass
	ServerSideEncryption types.ServerSideEncryption
	SSEKMSKeyId          string // Optional KMS key ID
}

// S3UploaderStage uploads streaming archives to real AWS S3
type S3UploaderStage struct {
	config *S3UploaderConfig
	input  <-chan *Job
	output chan<- *Job
	pool   *WorkerPool
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	stats  StageStats

	// Atomic counters
	jobsProcessed  int64
	bytesProcessed int64

	// S3 uploader
	uploader *manager.Uploader

	// Manifest tracking (Issue #157)
	pipeline *Pipeline // Reference to parent pipeline for resume capability
}

// NewS3UploaderStage creates a new real S3 uploader stage
func NewS3UploaderStage(config *S3UploaderConfig, input <-chan *Job, output chan<- *Job, pipeline *Pipeline) (*S3UploaderStage, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}
	if config.Workers <= 0 {
		config.Workers = 8 // Default to 8 workers to match shard count (Phase 4)
	}
	if config.PartSize <= 0 {
		config.PartSize = 64 * 1024 * 1024 // 64MB default
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}

	// Create AWS S3 uploader with optimized settings
	uploader := manager.NewUploader(config.S3Client, func(u *manager.Uploader) {
		u.PartSize = config.PartSize
		u.Concurrency = 4 // Internal concurrency per upload
		u.LeavePartsOnError = false
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(25 * 1024 * 1024)
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &S3UploaderStage{
		config:   config,
		input:    input,
		output:   output,
		pool:     NewWorkerPool(ctx, config.Workers),
		ctx:      ctx,
		cancel:   cancel,
		uploader: uploader,
		pipeline: pipeline, // Issue #157: Reference for resume capability
		stats: StageStats{
			Name: "s3_uploader",
		},
	}, nil
}

// Name returns the stage name
func (s *S3UploaderStage) Name() string {
	return "s3_uploader"
}

// Start starts the S3 uploader stage
func (s *S3UploaderStage) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < s.config.Workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}

	// Close output when all workers done
	go func() {
		s.wg.Wait()
		close(s.output)
	}()

	return nil
}

// Stop stops the S3 uploader stage
func (s *S3UploaderStage) Stop() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// shouldSkipUpload checks if a chunk should be skipped (already uploaded) - Issue #157
func (s *S3UploaderStage) shouldSkipUpload(ctx context.Context, job *Job) (bool, error) {
	// Check if resume mode is enabled
	if s.pipeline == nil || s.pipeline.config == nil || !s.pipeline.config.ResumeMode {
		return false, nil
	}

	// Check manifest for already uploaded chunks
	if s.pipeline.manifestBuilder != nil {
		builder := s.pipeline.manifestBuilder.(*manifest.Builder)
		manifestData := builder.Build()

		// Look for existing chunk with same ID
		for _, chunk := range manifestData.Chunks {
			if chunk.ID == job.Chunk.ID {
				// Check if UploadedAt is set (non-zero time)
				if !chunk.UploadedAt.IsZero() {
					// Chunk already uploaded
					return true, nil
				}
			}
		}
	}

	// Optional: Check S3 existence with HeadObject if SkipExisting is enabled
	if s.pipeline.config.SkipExisting {
		s3Key := job.S3Key
		if s.config.Prefix != "" {
			s3Key = s.config.Prefix + "/" + job.S3Key
		}

		_, err := s.config.S3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(s.config.Bucket),
			Key:    aws.String(s3Key),
		})

		if err == nil {
			// Object exists in S3
			return true, nil
		}
		// If HeadObject fails, continue with upload (object may not exist or permission issue)
	}

	return false, nil
}

// Process processes a single upload job (called by workers)
func (s *S3UploaderStage) Process(ctx context.Context, job *Job) error {
	startTime := time.Now()
	defer func() {
		if job.Archive != nil {
			_ = job.Archive.Close()
		}
	}()

	// Issue #157: Check if chunk should be skipped (resume mode)
	skip, err := s.shouldSkipUpload(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to check if upload should be skipped: %w", err)
	}

	if skip {
		// Chunk already uploaded - skip and mark as complete
		job.EndTime = time.Now()

		// Update statistics (but not bytes processed since we didn't actually upload)
		atomic.AddInt64(&s.jobsProcessed, 1)

		fmt.Printf("⏭️  Skipped chunk %d (already uploaded)\n", job.Chunk.ID)
		return nil
	}

	// Upload with retries
	var lastErr error
	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		// Issue #103: Track attempt number for error reporting
		job.AttemptNumber = attempt + 1

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.RetryDelay):
			}
		}

		if err := s.uploadToS3(ctx, job); err != nil {
			lastErr = err
			// Issue #103: Track error history for detailed reporting
			if job.ErrorHistory == nil {
				job.ErrorHistory = make([]error, 0, s.config.MaxRetries)
			}
			job.ErrorHistory = append(job.ErrorHistory, fmt.Errorf("attempt %d: %w", attempt+1, err))
			continue
		}

		// Success
		job.EndTime = time.Now()
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, job.ArchiveSize)

		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += job.ArchiveSize
		s.stats.TotalTime += time.Since(startTime)
		if s.stats.JobsProcessed > 0 {
			s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
		}
		s.mu.Unlock()

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", s.config.MaxRetries, lastErr)
}

// uploadToS3 performs the actual S3 upload using AWS SDK
func (s *S3UploaderStage) uploadToS3(ctx context.Context, job *Job) error {
	// Build S3 key with prefix
	s3Key := job.S3Key
	if s.config.Prefix != "" {
		s3Key = s.config.Prefix + "/" + job.S3Key
	}

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.config.Bucket),
		Key:          aws.String(s3Key),
		Body:         job.Archive,
		StorageClass: s.config.StorageClass,
		Metadata: map[string]string{
			"cargoship-chunk-id":    fmt.Sprintf("%d", job.ID),
			"cargoship-file-count":  fmt.Sprintf("%d", len(job.Chunk.Files)),
			"cargoship-chunk-size":  fmt.Sprintf("%d", job.Chunk.TotalSize),
			"cargoship-compression": "zstd",
			"cargoship-archive":     "tar",
		},
	}

	// Add server-side encryption if configured
	if s.config.ServerSideEncryption != "" {
		input.ServerSideEncryption = s.config.ServerSideEncryption
		if s.config.SSEKMSKeyId != "" {
			input.SSEKMSKeyId = aws.String(s.config.SSEKMSKeyId)
		}
	}

	// Upload using AWS SDK manager (handles multipart automatically)
	result, err := s.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("S3 upload failed for %s: %w", job.S3Key, err)
	}

	// Store upload result in job
	// Note: UploadOutput doesn't have UploadedParts, archive size is already set from archiver
	if result.Location != "" {
		job.S3Key = result.Location
	}
	if result.ETag != nil {
		// ETag indicates successful upload
		_ = result.ETag
	}

	return nil
}

// worker processes jobs from input channel
func (s *S3UploaderStage) worker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-s.input:
			if !ok {
				return
			}

			if err := s.Process(ctx, job); err != nil {
				job.Error = err
			}

			// Send to output channel
			select {
			case <-ctx.Done():
				return
			case s.output <- job:
			}
		}
	}
}

// Stats returns stage statistics
func (s *S3UploaderStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.config.Workers
	return stats
}

// GetUploadedBytes returns total bytes uploaded
func (s *S3UploaderStage) GetUploadedBytes() int64 {
	return atomic.LoadInt64(&s.bytesProcessed)
}

// GetUploadedJobs returns total jobs uploaded
func (s *S3UploaderStage) GetUploadedJobs() int64 {
	return atomic.LoadInt64(&s.jobsProcessed)
}
