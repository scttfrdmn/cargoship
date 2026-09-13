// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	tmtypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"go.opentelemetry.io/otel/trace"
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
	uploader *transfermanager.Client

	// v0.6.2: Advanced transporter (optional, shared across all shards)
	transporter s3transport.BasicTransporter

	// #424: real congestion control. Shared across all shard workers (one BBR
	// prober fed by every stream); nil-safe passthrough when optimization is off.
	// Note: per-shard pacers for independent BBR/CUBIC state is a future refinement.
	pacer *s3transport.CongestionPacer

	// Manifest tracking (Issue #97)
	pipeline *Pipeline // Reference to parent pipeline for manifest tracking

	// Logging (Issue #155)
	logger *slog.Logger
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
	logger *slog.Logger,
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

	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	// Create AWS S3 uploader with optimized settings (#384: transfermanager, which
	// aborts a failed multipart upload by default and manages its own buffers).
	uploader := transfermanager.New(config.S3Client, func(o *transfermanager.Options) {
		o.PartSizeBytes = config.PartSize
		o.Concurrency = 4 // Internal concurrency per upload
		// #522: skip the SDK's default per-upload CRC32 (redundant with our SHA-256).
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})

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
		workersPerPrefix: workersPerPrefix,
		uploader:         uploader,
		perPrefixStats:   perPrefixStats,
		logger:           logger, // Issue #155: Structured logging with trace context
		stats: StageStats{
			Name: "s3_multiprefix_uploader",
		},
		pipeline: pipeline, // Store reference for manifest tracking
	}

	// v0.6.2: Use transporter if configured (shared across all shards)
	if config.Transporter != nil {
		stage.transporter = config.Transporter
	}

	// #424: build the congestion pacer. Disabled → a transparent passthrough with
	// no prober goroutine. Uses context.Background(); the prober loop is torn down
	// by Stop() → pacer.Close().
	stage.pacer = s3transport.NewCongestionPacer(context.Background(), config.CongestionControl, config.EnableOptimization)

	return stage, nil
}

// Name returns the stage name
func (s *S3MultiPrefixUploaderStage) Name() string {
	return "s3_multiprefix_uploader"
}

// Start starts the multi-prefix S3 uploader stage
func (s *S3MultiPrefixUploaderStage) Start(ctx context.Context) error {
	// Create child context from parent (inherits trace context for Issue #155)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start worker pools for each prefix
	for prefix, inputChan := range s.inputs {
		// Launch workers for this prefix
		for i := 0; i < s.workersPerPrefix; i++ {
			s.wg.Add(1)
			go s.prefixWorker(s.ctx, prefix, inputChan)
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
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	if s.pacer != nil {
		s.pacer.Close() // #424: stop the BBR prober goroutine
	}
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

		// Issue #34 Phase 1.1: Return BufferedPipe to pool after upload completes
		// This prevents memory leak by reusing pipes instead of creating new ones
		if job.pipePool != nil && job.pipeReader != nil && job.pipeWriter != nil {
			job.pipePool.Put(job.pipeReader, job.pipeWriter)
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
		job.Skipped = true // #447: so waitForCompletion counts it as skipped, not uploaded

		// Update statistics (but not bytes processed since we didn't actually upload)
		atomic.AddInt64(&s.jobsProcessed, 1)

		fmt.Printf("⏭️  Skipped chunk %d (already uploaded)\n", job.Chunk.ID)

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

			// Record error in retry span
			if retrySpan != nil && s.pipeline != nil && s.pipeline.tracer != nil {
				tracer := s.pipeline.tracer.(*tracing.PipelineTracer)
				tracer.RecordError(retrySpan, err)
			}
			continue
		}

		// Success - update statistics
		job.EndTime = time.Now()
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, atomic.LoadInt64(&job.ArchiveSize))

		// Update per-prefix stats
		if prefixStats, exists := s.perPrefixStats[prefix]; exists {
			atomic.AddInt64(&prefixStats.jobsProcessed, 1)
			atomic.AddInt64(&prefixStats.bytesProcessed, atomic.LoadInt64(&job.ArchiveSize))
		}

		// Update global stats
		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += atomic.LoadInt64(&job.ArchiveSize)
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

			// #271: record per-file content checksums captured during archiving
			// (no-op when file checksums are disabled).
			builder.SetFileChecksums(job.Chunk.ID, job.FileChecksums())

			// #436: record per-file uncompressed tar offsets captured during
			// archiving (no-op when framing is disabled).
			builder.SetFileArchiveOffsets(job.Chunk.ID, job.FileArchiveOffsets())

			// #436: the random-access frame index for this chunk, if framing was
			// active (nil for single-frame / plain-tar chunks).
			frames := job.Frames()
			// #522: for a framed chunk the whole-object hash was skipped (frames
			// cover it), so source both the true compressed size and the object
			// integrity from the frames: the per-frame CompressedSizes sum to the
			// exact object size, and the per-frame checksums are the integrity. An
			// unframed chunk uses the whole-object hasher's byte count + digest
			// (falling back to the uncompressed estimate only if it wasn't wired).
			var compressedSize int64
			var checksum string
			if len(frames) > 0 {
				builder.AddFormatFeature(manifest.FormatFeatureFrames)
				for _, f := range frames {
					compressedSize += f.CompressedSize
				}
			} else {
				compressedSize = job.ArchiveCompressedSize()
				if compressedSize == 0 {
					compressedSize = atomic.LoadInt64(&job.ArchiveSize)
				}
				checksum = job.ArchiveChecksum()
			}

			// Add chunk entry (#271/#522: whole-object SHA-256 for unframed chunks;
			// empty for framed chunks, where FrameEntry.Checksum provides integrity).
			builder.AddChunk(manifest.ChunkEntry{
				ID:               job.Chunk.ID,
				ShardID:          shardID,
				S3Key:            job.S3Key,
				FileCount:        len(job.Chunk.Files),
				FilePaths:        filePaths,
				UncompressedSize: job.Chunk.TotalSize,
				CompressedSize:   compressedSize,
				CreatedAt:        job.StartTime,
				UploadedAt:       job.EndTime,
				Checksum:         checksum,
				Frames:           frames,
			})

			// Update shard stats
			builder.UpdateShardStats(
				shardID,
				job.S3Key,
				int64(len(job.Chunk.Files)),
				job.Chunk.TotalSize,
				compressedSize,
			)

			s.pipeline.manifestMu.Unlock()

			// Issue #158: Track uploaded key for cleanup on failure
			s.pipeline.trackUploadedKey(job.S3Key)
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
func (s *S3MultiPrefixUploaderStage) uploadToS3(ctx context.Context, job *Job) error {
	// Build S3 key with prefix
	s3Key := job.S3Key
	if s.config.Prefix != "" {
		s3Key = s.config.Prefix + "/" + job.S3Key
	}

	// #271: wrap the archive stream so we hash the exact bytes uploaded to S3.
	// Both upload paths (transporter, manager) read job.Archive, so wrapping
	// here covers both. The digest is finalized once the stream is consumed and
	// is read into ChunkEntry.Checksum after a successful upload.
	// #522: skip this whole-object hash for a framed chunk — its per-frame
	// checksums already tile the entire compressed object (a strict superset), so
	// the whole-object digest is redundant. Frames also carry the true compressed
	// size, so AddChunk sources that from them below. Unframed (e.g. plain-tar /
	// incompressible) chunks still get the whole-object hash.
	if job.Archive != nil && job.archiveHasher == nil && !job.Framed {
		job.archiveHasher = newHashingReadCloser(job.Archive)
		job.Archive = job.archiveHasher
	}

	// #424: pace the stream through the real congestion controller. After the
	// hashing wrap so the checksum still covers the exact uploaded bytes; a
	// passthrough when optimization is off. Both upload paths below read
	// job.Archive, so wrapping here covers transporter and manager alike.
	if s.pacer != nil && job.Archive != nil {
		job.Archive = s.pacer.WrapReadCloser(job.Archive)
	}

	// Prepare metadata
	metadata := map[string]string{
		"cargoship-chunk-id":    fmt.Sprintf("%d", job.ID),
		"cargoship-file-count":  fmt.Sprintf("%d", len(job.Chunk.Files)),
		"cargoship-chunk-size":  fmt.Sprintf("%d", job.Chunk.TotalSize),
		"cargoship-compression": "zstd",
		"cargoship-archive":     "tar",
	}

	// Choose upload path: transporter (advanced) or the SDK transfer manager (basic)
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
		Size:     atomic.LoadInt64(&job.ArchiveSize),
		Metadata: metadata,
	}

	// Upload via transporter.
	_, err := s.transporter.Upload(ctx, archive)
	if err != nil {
		s.maybeSignalThrottle(err, job)
		return fmt.Errorf("transporter upload failed for %s: %w", job.S3Key, err)
	}

	// #273: do NOT overwrite job.S3Key with result.Location. Location is a full
	// S3 URL (scheme/host/bucket baked in); job.S3Key is the portable,
	// prefix-relative key that goes into the manifest and the cleanup list.
	// Clobbering it made manifests non-portable and broke cleanup (which builds
	// sibling keys relative to the bucket). The object was written at
	// Prefix + "/" + job.S3Key, so the relative key is correct as-is.
	return nil
}

// maybeSignalThrottle feeds an S3 server-side throttle (503 SlowDown /
// RequestLimitExceeded / ServiceUnavailable) to the congestion pacer as its loss
// signal (#424), so the estimated rate contracts and subsequent sends back off.
// Other errors are ignored — they aren't congestion.
func (s *S3MultiPrefixUploaderStage) maybeSignalThrottle(err error, job *Job) {
	if s.pacer == nil || err == nil {
		return
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return
	}
	switch apiErr.ErrorCode() {
	case "SlowDown", "RequestLimitExceeded", "ServiceUnavailable", "Throttling", "ThrottlingException":
		s.pacer.SignalThrottle(atomic.LoadInt64(&job.ArchiveSize))
	}
}

// uploadViaManager uploads using basic AWS SDK the SDK transfer manager (backward compatibility)
func (s *S3MultiPrefixUploaderStage) uploadViaManager(ctx context.Context, s3Key string, job *Job, metadata map[string]string) error {
	// Prepare upload input (#384: transfermanager types; string casts preserve
	// identical StorageClass/SSE values).
	input := &transfermanager.UploadObjectInput{
		Bucket:       aws.String(s.config.Bucket),
		Key:          aws.String(s3Key),
		Body:         job.Archive,
		StorageClass: tmtypes.StorageClass(string(s.config.StorageClass)),
		Metadata:     metadata,
	}

	// Add server-side encryption if configured
	if s.config.ServerSideEncryption != "" {
		input.ServerSideEncryption = tmtypes.ServerSideEncryption(string(s.config.ServerSideEncryption))
		if s.config.SSEKMSKeyId != "" {
			input.SSEKMSKeyID = aws.String(s.config.SSEKMSKeyId)
		}
	}

	// Ensure job.Archive implements io.Reader
	var reader io.Reader = job.Archive

	// Replace Body with reader to ensure interface satisfaction
	input.Body = reader

	// Upload (handles multipart automatically)
	_, err := s.uploader.UploadObject(ctx, input)
	if err != nil {
		s.maybeSignalThrottle(err, job)
		return fmt.Errorf("S3 upload failed for %s: %w", job.S3Key, err)
	}

	// #273: keep job.S3Key as the portable, prefix-relative key — do not
	// overwrite it with the SDK's full-URL Location. See uploadViaTransporter.
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
