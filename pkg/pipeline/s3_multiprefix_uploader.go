// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// S3MultiPrefixUploaderStage uploads to S3 using per-prefix worker pools for parallel uploads.
// This achieves 5-8x throughput improvement by leveraging S3's partition-level parallelism.
//
// Architecture:
//   - Map of per-prefix input channels (shard-0, shard-1, ..., shard-N)
//   - Dedicated worker pool for each prefix (WorkersPerPrefix workers)
//   - Each worker pool uploads to its assigned S3 prefix concurrently
//
// Phase 3.1: Multi-Prefix Parallel Upload
type S3MultiPrefixUploaderStage struct {
	config *S3UploaderConfig
	inputs map[string]<-chan *Job // Key: "shard-N", Value: input channel
	output chan<- *Job
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	stats  StageStats

	// Per-prefix worker pools
	workersPerPrefix int

	// Atomic counters
	jobsProcessed  int64
	bytesProcessed int64

	// Per-prefix statistics
	perPrefixStats map[string]*PrefixStats

	// S3 uploader (shared across all workers)
	uploader *manager.Uploader

	// v0.6.2: Advanced transporter (optional, shared across all shards)
	// Future enhancement: Create per-shard transporter instances for independent BBR/CUBIC state
	transporter s3transport.BasicTransporter

	// Manifest tracking (Issue #97)
	pipeline *Pipeline // Reference to parent pipeline for manifest tracking
}

// PrefixStats tracks statistics for a specific S3 prefix
type PrefixStats struct {
	jobsProcessed  int64
	bytesProcessed int64
	activeWorkers  int
}

// NewS3MultiPrefixUploaderStage creates a new multi-prefix S3 uploader stage
func NewS3MultiPrefixUploaderStage(
	config *S3UploaderConfig,
	inputs map[string]<-chan *Job,
	output chan<- *Job,
	workersPerPrefix int,
	pipeline *Pipeline,
) (*S3MultiPrefixUploaderStage, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no input channels provided")
	}
	if workersPerPrefix <= 0 {
		workersPerPrefix = 2 // Default: 2 workers per prefix
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

	// Create AWS S3 uploader with optimized settings (backward compatibility)
	uploader := manager.NewUploader(config.S3Client, func(u *manager.Uploader) {
		u.PartSize = config.PartSize
		u.Concurrency = 4 // Internal concurrency per upload
		u.LeavePartsOnError = false
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(25 * 1024 * 1024)
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize per-prefix stats
	perPrefixStats := make(map[string]*PrefixStats)
	for prefix := range inputs {
		perPrefixStats[prefix] = &PrefixStats{
			activeWorkers: workersPerPrefix,
		}
	}

	stage := &S3MultiPrefixUploaderStage{
		config:           config,
		inputs:           inputs,
		output:           output,
		ctx:              ctx,
		cancel:           cancel,
		workersPerPrefix: workersPerPrefix,
		uploader:         uploader,
		perPrefixStats:   perPrefixStats,
		stats: StageStats{
			Name: "s3_multiprefix_uploader",
		},
		pipeline: pipeline, // Store reference for manifest tracking
	}

	// v0.6.2: Use transporter if configured (shared across all shards)
	if config.Transporter != nil {
		stage.transporter = config.Transporter
	}

	return stage, nil
}

// Name returns the stage name
func (s *S3MultiPrefixUploaderStage) Name() string {
	return "s3_multiprefix_uploader"
}

// Start starts the multi-prefix S3 uploader stage
func (s *S3MultiPrefixUploaderStage) Start(ctx context.Context) error {
	// Start worker pools for each prefix
	for prefix, inputChan := range s.inputs {
		// Launch workers for this prefix
		for i := 0; i < s.workersPerPrefix; i++ {
			s.wg.Add(1)
			go s.prefixWorker(ctx, prefix, inputChan)
		}
	}

	// Close output when all workers done
	go func() {
		s.wg.Wait()
		close(s.output)
	}()

	return nil
}

// Stop stops the multi-prefix S3 uploader stage
func (s *S3MultiPrefixUploaderStage) Stop() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// prefixWorker processes jobs for a specific S3 prefix
func (s *S3MultiPrefixUploaderStage) prefixWorker(ctx context.Context, prefix string, input <-chan *Job) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-input:
			if !ok {
				// Input channel closed
				return
			}

			if err := s.processJob(ctx, job, prefix); err != nil {
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

// processJob processes a single upload job
// shouldSkipUpload checks if a chunk should be skipped (already uploaded) - Issue #157
func (s *S3MultiPrefixUploaderStage) shouldSkipUpload(ctx context.Context, job *Job) (bool, error) {
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

func (s *S3MultiPrefixUploaderStage) processJob(ctx context.Context, job *Job, prefix string) error {
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
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.RetryDelay):
			}
		}

		if err := s.uploadToS3(ctx, job); err != nil {
			lastErr = err
			continue
		}

		// Success - update statistics
		job.EndTime = time.Now()
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, job.ArchiveSize)

		// Update per-prefix stats
		if prefixStats, exists := s.perPrefixStats[prefix]; exists {
			atomic.AddInt64(&prefixStats.jobsProcessed, 1)
			atomic.AddInt64(&prefixStats.bytesProcessed, job.ArchiveSize)
		}

		// Update global stats
		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += job.ArchiveSize
		s.stats.TotalTime += time.Since(startTime)
		if s.stats.JobsProcessed > 0 {
			s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
		}
		s.mu.Unlock()

		// Track chunk in manifest (Issue #97)
		if s.pipeline != nil && s.pipeline.manifestBuilder != nil {
			builder := s.pipeline.manifestBuilder.(*manifest.Builder)

			// Extract shard ID from S3 key (format: "prefix/uploads/uploadID/shard-N/chunk-M.tar.zst")
			shardID := extractShardIDFromS3Key(job.S3Key)

			// Extract file paths from chunk
			filePaths := make([]string, len(job.Chunk.Files))
			for i, f := range job.Chunk.Files {
				filePaths[i] = f.Path
			}

			s.pipeline.manifestMu.Lock()

			// Update file entries with S3 key and shard ID
			builder.UpdateFileS3Keys(job.Chunk.ID, shardID, job.S3Key)

			// Add chunk entry
			builder.AddChunk(manifest.ChunkEntry{
				ID:               job.Chunk.ID,
				ShardID:          shardID,
				S3Key:            job.S3Key,
				FileCount:        len(job.Chunk.Files),
				FilePaths:        filePaths,
				UncompressedSize: job.Chunk.TotalSize,
				CompressedSize:   job.ArchiveSize,
				CreatedAt:        job.StartTime,
				UploadedAt:       job.EndTime,
			})

			// Update shard stats
			builder.UpdateShardStats(
				shardID,
				job.S3Key,
				int64(len(job.Chunk.Files)),
				job.Chunk.TotalSize,
				job.ArchiveSize,
			)

			s.pipeline.manifestMu.Unlock()

			// Issue #158: Track uploaded key for cleanup on failure
			s.pipeline.trackUploadedKey(job.S3Key)
		}

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", s.config.MaxRetries, lastErr)
}

// uploadToS3 performs the actual S3 upload using transporter or AWS SDK
func (s *S3MultiPrefixUploaderStage) uploadToS3(ctx context.Context, job *Job) error {
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
	if s.transporter != nil {
		return s.uploadViaTransporter(ctx, s3Key, job, metadata)
	}
	return s.uploadViaManager(ctx, s3Key, job, metadata)
}

// uploadViaTransporter uploads using advanced S3 transporter
func (s *S3MultiPrefixUploaderStage) uploadViaTransporter(ctx context.Context, s3Key string, job *Job, metadata map[string]string) error {
	// Create transporter Archive struct
	archive := s3transport.Archive{
		Key:      s3Key,
		Reader:   job.Archive,
		Size:     job.ArchiveSize,
		Metadata: metadata,
	}

	// Upload via transporter
	result, err := s.transporter.Upload(ctx, archive)
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
func (s *S3MultiPrefixUploaderStage) uploadViaManager(ctx context.Context, s3Key string, job *Job, metadata map[string]string) error {
	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:       aws.String(s.config.Bucket),
		Key:          aws.String(s3Key),
		Body:         job.Archive,
		StorageClass: s.config.StorageClass,
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

// Stats returns stage statistics
func (s *S3MultiPrefixUploaderStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = len(s.inputs) * s.workersPerPrefix
	return stats
}

// GetPerPrefixStats returns statistics for each S3 prefix
func (s *S3MultiPrefixUploaderStage) GetPerPrefixStats() map[string]PrefixStats {
	result := make(map[string]PrefixStats)
	for prefix, stats := range s.perPrefixStats {
		result[prefix] = PrefixStats{
			jobsProcessed:  atomic.LoadInt64(&stats.jobsProcessed),
			bytesProcessed: atomic.LoadInt64(&stats.bytesProcessed),
			activeWorkers:  stats.activeWorkers,
		}
	}
	return result
}

// GetUploadedBytes returns total bytes uploaded
func (s *S3MultiPrefixUploaderStage) GetUploadedBytes() int64 {
	return atomic.LoadInt64(&s.bytesProcessed)
}

// GetUploadedJobs returns total jobs uploaded
func (s *S3MultiPrefixUploaderStage) GetUploadedJobs() int64 {
	return atomic.LoadInt64(&s.jobsProcessed)
}

// extractShardIDFromS3Key extracts shard ID from S3 key
// Key format: "prefix/uploads/uploadID/shard-N/chunk-M.tar.zst"
func extractShardIDFromS3Key(s3Key string) int {
	parts := strings.Split(s3Key, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "shard-") {
			if id, err := strconv.Atoi(strings.TrimPrefix(part, "shard-")); err == nil {
				return id
			}
		}
	}
	return 0 // Default to shard 0 if not found
}
