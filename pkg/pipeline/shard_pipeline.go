// Package pipeline provides streaming pipeline for CargoShip
package pipeline

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/klauspost/compress/zstd"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// ShardPipelineConfig configures a single shard pipeline
type ShardPipelineConfig struct {
	// Shard identification
	ShardID   int    // Shard identifier (0-indexed)
	ShardName string // Shard name (e.g., "shard-00000")

	// S3 configuration
	S3Client S3Uploader // AWS S3 client interface
	Bucket   string     // Target S3 bucket
	Prefix   string     // S3 key prefix (optional)

	// Compression configuration
	CompressionLevel zstd.EncoderLevel // Zstd compression level (default: SpeedDefault)

	// Memory management
	MemoryManager *MemoryManager // Memory manager for bounded usage

	// Retry configuration
	MaxRetries int           // Maximum upload retry attempts (default: 3)
	RetryDelay time.Duration // Delay between retries (default: 1s)

	// Performance tuning
	FileQueueBuffer int // File queue buffer size (default: 1000)

	// Multipart upload configuration
	UseMultipartUpload bool  // Enable S3 multipart upload (default: true for AWS compatibility)
	PartSize           int64 // Part size for multipart upload (default: 50 MB, min: 5 MB)

	// Adaptive worker pool (Issue #84)
	WorkerPool       *AdaptiveWorkerPool // Optional: Use existing worker pool
	MinWorkers       int                 // Minimum workers for adaptive scaling (default: 2)
	MaxWorkers       int                 // Maximum workers for adaptive scaling (default: auto-calculated)
	EnableAdaptive   bool                // Enable adaptive worker scaling (default: true)
}

// ShardPipeline handles streaming tar → zstd → S3 for a single shard
// This is zero-disk: files are streamed through tar, compressed, and uploaded without touching disk
type ShardPipeline struct {
	config *ShardPipelineConfig
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex

	// Streaming components
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	tarWriter  *tar.Writer
	encoder    *zstd.Encoder

	// State
	started    atomic.Bool
	closed     atomic.Bool
	fileQueue  chan chunking.File // Files waiting to be added
	done       chan struct{}      // Signal when upload completes
	uploadErr  error              // Upload error (if any)
	uploadSize int64              // Final uploaded size

	// Statistics
	filesAdded     int64 // Total files added to archive
	bytesProcessed int64 // Total bytes processed
	startTime      time.Time
	endTime        time.Time

	// Synchronization
	wg sync.WaitGroup

	// Worker pool for adaptive parallelism (Issue #84)
	pool *AdaptiveWorkerPool
}

// NewShardPipeline creates a new shard pipeline
func NewShardPipeline(ctx context.Context, config *ShardPipelineConfig) (*ShardPipeline, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}
	if config.ShardName == "" {
		return nil, fmt.Errorf("shard name cannot be empty")
	}

	// Set defaults
	if config.CompressionLevel == 0 {
		config.CompressionLevel = zstd.SpeedDefault
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}
	if config.FileQueueBuffer <= 0 {
		config.FileQueueBuffer = 1000 // Default: 1000 files per shard
	}
	// Multipart upload is enabled by default for AWS compatibility
	config.UseMultipartUpload = true
	if config.PartSize <= 0 {
		config.PartSize = 50 << 20 // Default: 50 MB parts
	}
	// Enforce S3 minimum part size (5 MB)
	if config.PartSize < (5 << 20) {
		config.PartSize = 5 << 20
	}

	ctx, cancel := context.WithCancel(ctx)

	sp := &ShardPipeline{
		config:    config,
		ctx:       ctx,
		cancel:    cancel,
		fileQueue: make(chan chunking.File, config.FileQueueBuffer),
		done:      make(chan struct{}),
	}

	// Create or use provided worker pool (Issue #84)
	if config.WorkerPool != nil {
		sp.pool = config.WorkerPool
	} else {
		// Calculate intelligent worker limits based on shard characteristics
		minWorkers := config.MinWorkers
		if minWorkers <= 0 {
			minWorkers = 2
		}

		maxWorkers := config.MaxWorkers
		if maxWorkers <= 0 {
			// Auto-calculate based on part size as proxy for file size
			// Small parts (small files) → fewer workers (CPU-bound compression)
			// Large parts (large files) → more workers (I/O-bound upload)
			if config.PartSize < (10 << 20) { // < 10 MB parts (small files)
				maxWorkers = 16 // CPU-bound compression
			} else if config.PartSize < (50 << 20) { // < 50 MB parts (medium files)
				maxWorkers = 64 // Mixed workload
			} else { // >= 50 MB parts (large files)
				maxWorkers = 256 // I/O-bound upload
			}
		}

		// Create adaptive worker pool
		poolConfig := &AdaptiveWorkerPoolConfig{
			MinWorkers:     minWorkers,
			MaxWorkers:     maxWorkers,
			EnableAdaptive: config.EnableAdaptive,
		}
		sp.pool = NewAdaptiveWorkerPool(ctx, poolConfig)
	}

	return sp, nil
}

// Start starts the streaming pipeline: tar → zstd → S3
func (sp *ShardPipeline) Start() error {
	if !sp.started.CompareAndSwap(false, true) {
		return fmt.Errorf("pipeline already started")
	}

	sp.startTime = time.Now()

	// Create pipe for streaming
	sp.pipeReader, sp.pipeWriter = io.Pipe()

	// Create zstd encoder writing to pipe
	var err error
	sp.encoder, err = zstd.NewWriter(sp.pipeWriter,
		zstd.WithEncoderLevel(sp.config.CompressionLevel),
	)
	if err != nil {
		_ = sp.pipeWriter.Close()
		_ = sp.pipeReader.Close()
		return fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	// Create tar writer writing to zstd encoder
	sp.tarWriter = tar.NewWriter(sp.encoder)

	// Start goroutines
	sp.wg.Add(2)
	go sp.fileAdder()     // Adds files to tar archive
	go sp.uploader()      // Uploads compressed archive to S3

	return nil
}

// AddFile queues a file to be added to the archive
func (sp *ShardPipeline) AddFile(file chunking.File) error {
	if sp.closed.Load() {
		return fmt.Errorf("pipeline is closed")
	}

	select {
	case sp.fileQueue <- file:
		return nil
	case <-sp.ctx.Done():
		return sp.ctx.Err()
	}
}

// Close closes the pipeline and waits for upload to complete
func (sp *ShardPipeline) Close() error {
	if !sp.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	sp.endTime = time.Now()

	// Close file queue to signal no more files
	close(sp.fileQueue)

	// Wait for all goroutines to finish
	sp.wg.Wait()

	// Wait for upload to complete
	<-sp.done

	// Stop worker pool
	if sp.pool != nil {
		sp.pool.Stop()
	}

	return sp.uploadErr
}

// fileAdder reads files from queue and adds them to tar archive
func (sp *ShardPipeline) fileAdder() {
	defer sp.wg.Done()
	defer func() {
		// Close tar writer (flushes any buffered data)
		if err := sp.tarWriter.Close(); err != nil {
			sp.setUploadError(fmt.Errorf("tar close error: %w", err))
		}

		// Close zstd encoder (flushes compressed data)
		if err := sp.encoder.Close(); err != nil {
			sp.setUploadError(fmt.Errorf("encoder close error: %w", err))
		}

		// Close pipe writer (signals EOF to reader)
		if err := sp.pipeWriter.Close(); err != nil {
			sp.setUploadError(fmt.Errorf("pipe writer close error: %w", err))
		}
	}()

	for {
		select {
		case <-sp.ctx.Done():
			return
		case file, ok := <-sp.fileQueue:
			if !ok {
				// Queue closed, no more files
				return
			}

			if err := sp.addFileToTar(file); err != nil {
				sp.setUploadError(fmt.Errorf("failed to add file %s: %w", file.Path, err))
				return
			}

			atomic.AddInt64(&sp.filesAdded, 1)
			atomic.AddInt64(&sp.bytesProcessed, file.Size)

			// Track throughput for adaptive worker scaling (Issue #84)
			if sp.pool != nil {
				sp.pool.AddBytes(file.Size)
			}
		}
	}
}

// addFileToTar adds a single file to the tar archive
func (sp *ShardPipeline) addFileToTar(file chunking.File) error {
	// Open file
	f, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Get file info
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Create tar header
	header := &tar.Header{
		Name:    file.Path,
		Size:    file.Size,
		Mode:    int64(fi.Mode()),
		ModTime: file.ModTime,
	}

	// Write header
	if err := sp.tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	// Copy file data to tar (zero-copy optimized)
	if _, err := ioutils.CopyOptimized(sp.tarWriter, f); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	return nil
}

// uploader reads from pipe and uploads to S3 (multipart or single PutObject)
func (sp *ShardPipeline) uploader() {
	defer sp.wg.Done()
	defer close(sp.done)
	defer func() {
		_ = sp.pipeReader.Close()
	}()

	// Build S3 key
	s3Key := sp.buildS3Key()

	// Reserve memory if memory manager available
	if sp.config.MemoryManager != nil {
		// Estimate memory usage based on part size
		estimatedMemory := sp.config.PartSize
		if err := sp.config.MemoryManager.ReserveMemory(sp.ctx, &Job{
			Chunk: chunking.Chunk{TotalSize: estimatedMemory},
		}); err != nil {
			sp.setUploadError(fmt.Errorf("failed to reserve memory: %w", err))
			return
		}
		defer sp.config.MemoryManager.ReleaseMemory(estimatedMemory)
	}

	// Use multipart upload (required for AWS compatibility)
	if sp.config.UseMultipartUpload {
		if err := sp.uploadMultipart(s3Key); err != nil {
			sp.setUploadError(err)
		}
	} else {
		// Fallback to PutObject (for testing/local only)
		if err := sp.uploadSingle(s3Key); err != nil {
			sp.setUploadError(err)
		}
	}
}

// uploadMultipart uploads to S3 using multipart upload API (for AWS compatibility)
func (sp *ShardPipeline) uploadMultipart(s3Key string) error {
	// Create multipart upload
	createResp, err := sp.config.S3Client.CreateMultipartUpload(sp.ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(sp.config.Bucket),
		Key:    aws.String(s3Key),
		Metadata: map[string]string{
			"cargoship-mode":     "sharded",
			"cargoship-shard-id": fmt.Sprintf("%d", sp.config.ShardID),
			"cargoship-shard":    sp.config.ShardName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create multipart upload: %w", err)
	}

	uploadID := *createResp.UploadId
	var completedParts []s3Types.CompletedPart
	partNumber := int32(1)
	buffer := make([]byte, sp.config.PartSize)
	var totalUploaded int64

	// Upload parts
	for {
		// Read part data
		n, readErr := io.ReadFull(sp.pipeReader, buffer)
		if n == 0 && readErr == io.EOF {
			// No more data
			break
		}

		// Last part can be smaller than PartSize
		partData := buffer[:n]
		if len(partData) == 0 {
			break
		}

		// Upload part with retries
		var etag string
		var lastErr error
		for attempt := 0; attempt < sp.config.MaxRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-sp.ctx.Done():
					_, _ = sp.config.S3Client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
						Bucket:   aws.String(sp.config.Bucket),
						Key:      aws.String(s3Key),
						UploadId: aws.String(uploadID),
					})
					return sp.ctx.Err()
				case <-time.After(sp.config.RetryDelay):
				}
			}

			uploadResp, err := sp.config.S3Client.UploadPart(sp.ctx, &s3.UploadPartInput{
				Bucket:     aws.String(sp.config.Bucket),
				Key:        aws.String(s3Key),
				UploadId:   aws.String(uploadID),
				PartNumber: aws.Int32(partNumber),
				Body:       bytes.NewReader(partData),
			})

			if err != nil {
				lastErr = err
				continue
			}

			etag = *uploadResp.ETag
			break
		}

		if lastErr != nil {
			// Abort multipart upload on failure
			_, _ = sp.config.S3Client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(sp.config.Bucket),
				Key:      aws.String(s3Key),
				UploadId: aws.String(uploadID),
			})
			return fmt.Errorf("failed to upload part %d after %d attempts: %w", partNumber, sp.config.MaxRetries, lastErr)
		}

		// Record completed part
		completedParts = append(completedParts, s3Types.CompletedPart{
			ETag:       aws.String(etag),
			PartNumber: aws.Int32(partNumber),
		})

		totalUploaded += int64(len(partData))
		partNumber++

		// Check if we've reached EOF
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	// Complete multipart upload
	_, err = sp.config.S3Client.CompleteMultipartUpload(sp.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(sp.config.Bucket),
		Key:      aws.String(s3Key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3Types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})

	if err != nil {
		// Abort on completion failure
		_, _ = sp.config.S3Client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(sp.config.Bucket),
			Key:      aws.String(s3Key),
			UploadId: aws.String(uploadID),
		})
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	// Update upload size
	sp.mu.Lock()
	sp.uploadSize = totalUploaded
	sp.mu.Unlock()

	return nil
}

// uploadSingle uploads to S3 using PutObject (fallback for testing/local only)
func (sp *ShardPipeline) uploadSingle(s3Key string) error {
	// Buffer entire stream (not recommended for production)
	data, err := io.ReadAll(sp.pipeReader)
	if err != nil {
		return fmt.Errorf("failed to read pipe: %w", err)
	}

	// Upload with retries
	var lastErr error
	for attempt := 0; attempt < sp.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-sp.ctx.Done():
				return sp.ctx.Err()
			case <-time.After(sp.config.RetryDelay):
			}
		}

		_, err := sp.config.S3Client.PutObject(sp.ctx, &s3.PutObjectInput{
			Bucket: aws.String(sp.config.Bucket),
			Key:    aws.String(s3Key),
			Body:   bytes.NewReader(data),
			Metadata: map[string]string{
				"cargoship-mode":     "sharded",
				"cargoship-shard-id": fmt.Sprintf("%d", sp.config.ShardID),
				"cargoship-shard":    sp.config.ShardName,
			},
		})

		if err != nil {
			lastErr = err
			continue
		}

		// Update upload size
		sp.mu.Lock()
		sp.uploadSize = int64(len(data))
		sp.mu.Unlock()

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", sp.config.MaxRetries, lastErr)
}

// buildS3Key constructs the S3 key for this shard
func (sp *ShardPipeline) buildS3Key() string {
	key := fmt.Sprintf("%s.tar.zst", sp.config.ShardName)
	if sp.config.Prefix != "" {
		return fmt.Sprintf("%s/%s", sp.config.Prefix, key)
	}
	return key
}

// setUploadError sets the upload error (thread-safe)
func (sp *ShardPipeline) setUploadError(err error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.uploadErr == nil {
		sp.uploadErr = err
	}
}

// GetStats returns pipeline statistics
func (sp *ShardPipeline) GetStats() ShardPipelineStats {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	duration := time.Since(sp.startTime)
	if !sp.endTime.IsZero() {
		duration = sp.endTime.Sub(sp.startTime)
	}

	// Get worker count if pool exists (Issue #84)
	workerCount := 0
	if sp.pool != nil {
		workerCount = sp.pool.GetWorkerCount()
	}

	return ShardPipelineStats{
		ShardID:        sp.config.ShardID,
		ShardName:      sp.config.ShardName,
		FilesAdded:     atomic.LoadInt64(&sp.filesAdded),
		BytesProcessed: atomic.LoadInt64(&sp.bytesProcessed),
		UploadSize:     sp.uploadSize,
		Duration:       duration,
		Completed:      sp.closed.Load(),
		Error:          sp.uploadErr,
		WorkerCount:    workerCount,
	}
}

// ShardPipelineStats contains statistics for a shard pipeline
type ShardPipelineStats struct {
	ShardID        int           // Shard identifier
	ShardName      string        // Shard name
	FilesAdded     int64         // Total files added to archive
	BytesProcessed int64         // Total bytes processed (uncompressed)
	UploadSize     int64         // Final uploaded size (compressed)
	Duration       time.Duration // Total processing time
	Completed      bool          // Whether pipeline has completed
	Error          error         // Upload error (if any)
	WorkerCount    int           // Current worker count (Issue #84)
}

// String returns a formatted string representation of stats
func (s ShardPipelineStats) String() string {
	status := "in-progress"
	if s.Completed {
		if s.Error != nil {
			status = fmt.Sprintf("failed: %v", s.Error)
		} else {
			status = "completed"
		}
	}

	compressionRatio := 0.0
	if s.BytesProcessed > 0 && s.UploadSize > 0 {
		compressionRatio = float64(s.UploadSize) / float64(s.BytesProcessed)
	}

	// Include worker count if available (Issue #84)
	if s.WorkerCount > 0 {
		return fmt.Sprintf("Shard %s: %d files, %d MB processed, %d MB uploaded (%.1f%% compression), %d workers, %s, %s",
			s.ShardName,
			s.FilesAdded,
			s.BytesProcessed/(1<<20),
			s.UploadSize/(1<<20),
			(1-compressionRatio)*100,
			s.WorkerCount,
			s.Duration.Round(time.Millisecond),
			status,
		)
	}

	return fmt.Sprintf("Shard %s: %d files, %d MB processed, %d MB uploaded (%.1f%% compression), %s, %s",
		s.ShardName,
		s.FilesAdded,
		s.BytesProcessed/(1<<20),
		s.UploadSize/(1<<20),
		(1-compressionRatio)*100,
		s.Duration.Round(time.Millisecond),
		status,
	)
}
