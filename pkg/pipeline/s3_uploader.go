// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"go.opentelemetry.io/otel/trace"
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

	// Issue #32: Automatic storage tier selection based on file access time
	TierSelector *StorageTierSelector // If nil, uses StorageClass for all uploads

	// Advanced transporter (v0.6.2)
	// If set, uses advanced S3 transporter instead of basic manager.Uploader
	Transporter s3transport.BasicTransporter // Optional advanced transporter
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

	// Logging (Issue #155)
	logger *slog.Logger
}

// NewS3UploaderStage creates a new real S3 uploader stage
func NewS3UploaderStage(config *S3UploaderConfig, input <-chan *Job, output chan<- *Job, pipeline *Pipeline, logger *slog.Logger) (*S3UploaderStage, error) {
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

	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	// Create AWS S3 uploader with optimized settings
	uploader := manager.NewUploader(config.S3Client, func(u *manager.Uploader) {
		u.PartSize = config.PartSize
		u.Concurrency = 4 // Internal concurrency per upload
		u.LeavePartsOnError = false
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(25 * 1024 * 1024)
	})

	return &S3UploaderStage{
		config:   config,
		input:    input,
		output:   output,
		uploader: uploader,
		pipeline: pipeline, // Issue #157: Reference for resume capability
		logger:   logger,   // Issue #155: Structured logging with trace context
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
	// Create child context from parent (inherits trace context for Issue #155)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Initialize worker pool with inherited context
	s.pool = NewWorkerPool(s.ctx, s.config.Workers)

	// Start worker goroutines
	for i := 0; i < s.config.Workers; i++ {
		s.wg.Add(1)
		go s.worker(s.ctx)
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
	if s.cancel != nil {
		s.cancel()
	}
	if s.pool != nil {
		s.pool.Stop()
	}
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

	// Create job span if tracing enabled (Issue #155)
	var jobSpan trace.Span
	if s.pipeline != nil && s.pipeline.tracer != nil {
		tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
		ctx, jobSpan = tracer.StartJobSpan(ctx, job.ID, job.ShardID)
		defer jobSpan.End()

		// Add file and S3 attributes
		tracer.AddFileAttributes(jobSpan, "", atomic.LoadInt64(&job.ArchiveSize), len(job.Chunk.Files))
		tracer.AddS3Attributes(jobSpan, s.config.Bucket, job.S3Key, "")
	}

	// Issue #157: Check if chunk should be skipped (resume mode)
	skip, err := s.shouldSkipUpload(ctx, job)
	if err != nil {
		if jobSpan != nil && s.pipeline != nil && s.pipeline.tracer != nil {
			tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
			tracer.RecordError(jobSpan, err)
		}
		return fmt.Errorf("failed to check if upload should be skipped: %w", err)
	}

	if skip {
		// Chunk already uploaded - skip and mark as complete
		job.EndTime = time.Now()

		// Update statistics (but not bytes processed since we didn't actually upload)
		atomic.AddInt64(&s.jobsProcessed, 1)

		// Log with trace context (Issue #155)
		tracing.InfoWithTrace(ctx, s.logger, "skipped chunk (already uploaded)",
			slog.Int("chunk_id", job.Chunk.ID),
			slog.String("s3_key", job.S3Key),
		)

		// Record success in span
		if jobSpan != nil && s.pipeline != nil && s.pipeline.tracer != nil {
			tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
			tracer.RecordSuccess(jobSpan)
		}
		return nil
	}

	// Upload with retries
	var lastErr error
	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		// Issue #103: Track attempt number for error reporting
		job.AttemptNumber = attempt + 1

		// Create retry span if this is a retry (Issue #155)
		var retrySpan trace.Span
		retryCtx := ctx
		if attempt > 0 && s.pipeline != nil && s.pipeline.tracer != nil {
			tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
			retryCtx, retrySpan = tracer.StartRetrySpan(ctx, attempt+1)
			defer retrySpan.End()
		}

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.RetryDelay):
			}
		}

		if err := s.uploadToS3(retryCtx, job); err != nil {
			lastErr = err
			// Issue #103: Track error history for detailed reporting
			if job.ErrorHistory == nil {
				job.ErrorHistory = make([]error, 0, s.config.MaxRetries)
			}
			job.ErrorHistory = append(job.ErrorHistory, fmt.Errorf("attempt %d: %w", attempt+1, err))

			// Record error in retry span
			if retrySpan != nil && s.pipeline != nil && s.pipeline.tracer != nil {
				tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
				tracer.RecordError(retrySpan, err)
			}
			continue
		}

		// Success
		job.EndTime = time.Now()
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, atomic.LoadInt64(&job.ArchiveSize))

		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += atomic.LoadInt64(&job.ArchiveSize)
		s.stats.TotalTime += time.Since(startTime)
		if s.stats.JobsProcessed > 0 {
			s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
		}
		s.mu.Unlock()

		// Issue #158: Track uploaded key for cleanup on failure
		if s.pipeline != nil {
			s.pipeline.trackUploadedKey(job.S3Key)
		}

		// Issue #108: Update deduplication index with actual locations after successful upload
		if s.pipeline != nil && s.pipeline.dedupEnabled && s.pipeline.dedupIndex != nil {
			dedupIndex := s.pipeline.dedupIndex.(*manifest.FileDeduplicationIndex)
			for _, file := range job.Chunk.Files {
				// Check if file has content hash (stored by scanner during dedup filtering)
				if file.Metadata != nil {
					if hash, ok := file.Metadata["content_hash"]; ok {
						// Update location with actual shard/chunk/S3 key
						if err := dedupIndex.UpdateFileLocation(hash, job.ShardID, job.Chunk.ID, job.S3Key); err != nil {
							// Log warning but don't fail upload - dedup is best-effort
							tracing.WarnWithTrace(retryCtx, s.logger, "failed to update dedup location",
								slog.String("file", file.Path),
								slog.String("hash", hash),
								slog.String("error", err.Error()),
							)
						}
					}
				}
			}
		}

		// Record success in spans
		if s.pipeline != nil && s.pipeline.tracer != nil {
			tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
			if jobSpan != nil {
				tracer.RecordSuccess(jobSpan)
			}
			if retrySpan != nil {
				tracer.RecordSuccess(retrySpan)
			}
		}

		return nil
	}

	// Record final error in job span
	if jobSpan != nil && s.pipeline != nil && s.pipeline.tracer != nil {
		tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
		tracer.RecordError(jobSpan, lastErr)
	}

	return fmt.Errorf("upload failed after %d attempts: %w", s.config.MaxRetries, lastErr)
}

// uploadToS3 performs the actual S3 upload using transporter or AWS SDK
func (s *S3UploaderStage) uploadToS3(ctx context.Context, job *Job) error {
	// Build S3 key with prefix
	s3Key := job.S3Key
	if s.config.Prefix != "" {
		s3Key = s.config.Prefix + "/" + job.S3Key
	}

	// Prepare metadata
	metadata := map[string]string{
		"cargoship-chunk-id":    fmt.Sprintf("%d", job.ID),
		"cargoship-file-count":  fmt.Sprintf("%d", len(job.Chunk.Files)),
		"cargoship-chunk-size":  fmt.Sprintf("%d", job.Chunk.TotalSize),
		"cargoship-compression": "zstd",
		"cargoship-archive":     "tar",
	}

	// Choose upload path: transporter (advanced) or manager.Uploader (basic)
	if s.config.Transporter != nil {
		// Use advanced transporter (staging, adaptive, optimized)
		return s.uploadViaTransporter(ctx, s3Key, job, metadata)
	}

	// Fallback to basic manager.Uploader (backward compatibility)
	return s.uploadViaManager(ctx, s3Key, job, metadata)
}

// uploadViaTransporter uploads using advanced S3 transporter
func (s *S3UploaderStage) uploadViaTransporter(ctx context.Context, s3Key string, job *Job, metadata map[string]string) error {
	// Determine storage class: use pre-assigned tier (Issue #164), TierSelector, or default
	storageClass := awsconfig.StorageClass(s.config.StorageClass)

	// Issue #164: Check for pre-assigned tier from tier-aware chunking (v2)
	if job.Chunk.PreAssignedTier != "" {
		// Tier-aware chunking has already grouped files by tier - use pre-assigned tier
		storageClass = awsconfig.StorageClass(job.Chunk.PreAssignedTier)
	} else if s.config.TierSelector != nil && s.config.TierSelector.Enabled {
		// Fallback to v1 youngest-file strategy for backward compatibility
		// Find youngest (most recently accessed) file in chunk for tier selection
		// Conservative approach: if ANY file is hot, keep entire chunk in hot tier
		if len(job.Chunk.Files) > 0 {
			var youngestAtime time.Time
			var youngestMtime time.Time

			// Scan all files to find the most recently accessed
			for _, file := range job.Chunk.Files {
				// Parse atime from metadata (stored as RFC3339 string)
				if atimeStr, ok := file.Metadata["atime"]; ok {
					if parsedTime, err := time.Parse(time.RFC3339, atimeStr); err == nil {
						if youngestAtime.IsZero() || parsedTime.After(youngestAtime) {
							youngestAtime = parsedTime
							youngestMtime = file.ModTime
						}
					}
				}
			}

			// Use TierSelector with youngest file's access time
			selectedClass := s.config.TierSelector.SelectTier(youngestAtime, youngestMtime)
			storageClass = awsconfig.StorageClass(selectedClass)
		}
	}

	// Create transporter Archive struct
	archive := s3transport.Archive{
		Key:          s3Key,
		Reader:       job.Archive, // io.ReadCloser
		Size:         atomic.LoadInt64(&job.ArchiveSize),
		StorageClass: storageClass,
		Metadata:     metadata,
	}

	// Upload via transporter
	result, err := s.config.Transporter.Upload(ctx, archive)
	if err != nil {
		return fmt.Errorf("transporter upload failed for %s: %w", job.S3Key, err)
	}

	// Store result
	if result.Location != "" {
		job.S3Key = result.Location
	}

	return nil
}

// uploadViaManager uploads using basic AWS SDK manager.Uploader (backward compatibility)
func (s *S3UploaderStage) uploadViaManager(ctx context.Context, s3Key string, job *Job, metadata map[string]string) error {
	// Determine storage class: use pre-assigned tier (Issue #164), TierSelector, or default
	storageClass := s.config.StorageClass

	// Issue #164: Check for pre-assigned tier from tier-aware chunking (v2)
	if job.Chunk.PreAssignedTier != "" {
		// Tier-aware chunking has already grouped files by tier - use pre-assigned tier
		storageClass = job.Chunk.PreAssignedTier
	} else if s.config.TierSelector != nil && s.config.TierSelector.Enabled {
		// Fallback to v1 youngest-file strategy for backward compatibility
		// Find youngest (most recently accessed) file in chunk for tier selection
		// Conservative approach: if ANY file is hot, keep entire chunk in hot tier
		if len(job.Chunk.Files) > 0 {
			var youngestAtime time.Time
			var youngestMtime time.Time

			// Scan all files to find the most recently accessed
			for _, file := range job.Chunk.Files {
				// Parse atime from metadata (stored as RFC3339 string)
				if atimeStr, ok := file.Metadata["atime"]; ok {
					if parsedTime, err := time.Parse(time.RFC3339, atimeStr); err == nil {
						if youngestAtime.IsZero() || parsedTime.After(youngestAtime) {
							youngestAtime = parsedTime
							youngestMtime = file.ModTime
						}
					}
				}
			}

			// Use TierSelector with youngest file's access time
			storageClass = s.config.TierSelector.SelectTier(youngestAtime, youngestMtime)
		}
	}

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.config.Bucket),
		Key:          aws.String(s3Key),
		Body:         job.Archive,
		StorageClass: storageClass,
		Metadata:     metadata,
	}

	// Add server-side encryption if configured
	if s.config.ServerSideEncryption != "" {
		input.ServerSideEncryption = s.config.ServerSideEncryption
		if s.config.SSEKMSKeyId != "" {
			input.SSEKMSKeyId = aws.String(s.config.SSEKMSKeyId)
		}
	}

	// Ensure job.Archive implements io.Reader for manager.Uploader
	var reader io.Reader = job.Archive

	// Replace Body with reader to ensure interface satisfaction
	input.Body = reader

	// Upload using AWS SDK manager (handles multipart automatically)
	result, err := s.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("S3 upload failed for %s: %w", job.S3Key, err)
	}

	// Store upload result in job
	if result.Location != "" {
		job.S3Key = result.Location
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
