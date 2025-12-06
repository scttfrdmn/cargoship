// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// S3Uploader is an interface for uploading to S3 (allows mocking)
type S3Uploader interface {
	PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)

	// Multipart upload methods (required for streaming large objects)
	CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

// DirectUploaderConfig configures the direct uploader stage
type DirectUploaderConfig struct {
	S3Client    S3Uploader      // AWS S3 client (interface for testability)
	Bucket      string          // Target S3 bucket
	Prefix      string          // S3 key prefix (optional)
	Workers     int             // Number of concurrent upload workers
	MaxRetries  int             // Maximum upload retry attempts
	RetryDelay  time.Duration   // Delay between retries
	WorkerPool  *AdaptiveWorkerPool // Optional: Use adaptive worker pool
}

// DirectUploaderStage uploads files directly to S3 without archiving or compression
// This provides maximum speed at the cost of more S3 operations
type DirectUploaderStage struct {
	config *DirectUploaderConfig
	input  <-chan *Job
	output chan<- *Job
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	stats  StageStats

	// Atomic counters
	jobsProcessed  int64
	filesProcessed int64
	bytesProcessed int64

	// Worker pool
	pool *AdaptiveWorkerPool
}

// NewDirectUploaderStage creates a new direct uploader stage
func NewDirectUploaderStage(config *DirectUploaderConfig, input <-chan *Job, output chan<- *Job) (*DirectUploaderStage, error) {
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
		config.Workers = 256 // Default to 256 workers (match s5cmd)
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	stage := &DirectUploaderStage{
		config: config,
		input:  input,
		output: output,
		ctx:    ctx,
		cancel: cancel,
		stats: StageStats{
			Name: "direct_uploader",
		},
	}

	// Create or use provided worker pool
	if config.WorkerPool != nil {
		stage.pool = config.WorkerPool
	} else {
		// Create adaptive worker pool with defaults
		poolConfig := &AdaptiveWorkerPoolConfig{
			MaxWorkers:     config.Workers,
			EnableAdaptive: true,
		}
		stage.pool = NewAdaptiveWorkerPool(ctx, poolConfig)
	}

	return stage, nil
}

// Name returns the stage name
func (s *DirectUploaderStage) Name() string {
	return "direct_uploader"
}

// Start starts the direct uploader stage
func (s *DirectUploaderStage) Start(ctx context.Context) error {
	// Start main dispatcher goroutine
	s.wg.Add(1)
	go s.dispatcher(ctx)

	return nil
}

// Stop stops the direct uploader stage
func (s *DirectUploaderStage) Stop() error {
	s.cancel()
	s.wg.Wait()
	s.pool.Stop()
	return nil
}

// dispatcher reads jobs and submits files to worker pool
func (s *DirectUploaderStage) dispatcher(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.output)

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-s.input:
			if !ok {
				// Input closed, wait for all workers to finish
				s.pool.Wait()
				return
			}

			// Process each file in the chunk independently
			var fileWg sync.WaitGroup
			for _, file := range job.Chunk.Files {
				// Create a copy of job for each file to avoid race conditions
				fileCopy := file
				fileWg.Add(1)
				s.submitFileUpload(ctx, fileCopy, &fileWg)
			}

			// Wait for all files in this job to complete
			fileWg.Wait()

			// Mark job as processed and send to output
			atomic.AddInt64(&s.jobsProcessed, 1)

			// Send job to output (non-blocking)
			select {
			case s.output <- job:
			case <-ctx.Done():
				return
			}
		}
	}
}

// submitFileUpload submits a single file upload to the worker pool
func (s *DirectUploaderStage) submitFileUpload(ctx context.Context, file chunking.File, wg *sync.WaitGroup) {
	err := s.pool.Submit(func(workerCtx context.Context) error {
		defer wg.Done()
		return s.uploadFile(workerCtx, file)
	})

	if err != nil {
		// If submission failed, still decrement WaitGroup
		wg.Done()
		// Log error but continue processing other files
		s.mu.Lock()
		s.stats.JobsFailed++
		s.mu.Unlock()
	}
}

// uploadFile uploads a single file to S3
func (s *DirectUploaderStage) uploadFile(ctx context.Context, file chunking.File) error {
	startTime := time.Now()

	// Build S3 key
	s3Key := s.buildS3Key(file.Path)

	// Open file
	f, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", file.Path, err)
	}
	defer func() {
		_ = f.Close()
	}()

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

		// Seek to beginning for retry
		if _, err := f.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to seek file: %w", err)
		}

		// Upload to S3
		_, err := s.config.S3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(s.config.Bucket),
			Key:    aws.String(s3Key),
			Body:   f,
			Metadata: map[string]string{
				"cargoship-mode":      "direct",
				"cargoship-file-path": file.Path,
				"cargoship-file-size": fmt.Sprintf("%d", file.Size),
			},
		})

		if err != nil {
			lastErr = err
			continue
		}

		// Success
		atomic.AddInt64(&s.filesProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, file.Size)
		s.pool.AddBytes(file.Size) // For throughput monitoring

		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += file.Size
		s.stats.TotalTime += time.Since(startTime)
		if s.stats.JobsProcessed > 0 {
			s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
		}
		s.mu.Unlock()

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts for %s: %w", s.config.MaxRetries, file.Path, lastErr)
}

// buildS3Key constructs the S3 key from file path
func (s *DirectUploaderStage) buildS3Key(filePath string) string {
	// Get relative path from file path
	relPath := filepath.Base(filePath)

	// Add prefix if configured
	if s.config.Prefix != "" {
		return filepath.Join(s.config.Prefix, relPath)
	}

	return relPath
}

// Process processes a single job (not used in direct mode, kept for interface compatibility)
func (s *DirectUploaderStage) Process(ctx context.Context, job *Job) error {
	// Direct uploader processes files individually in dispatcher
	// This method is kept for Stage interface compatibility
	return nil
}

// Stats returns stage statistics
func (s *DirectUploaderStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.pool.GetWorkerCount()

	// Add custom metadata
	stats.Metadata = map[string]interface{}{
		"files_uploaded":  atomic.LoadInt64(&s.filesProcessed),
		"worker_count":    s.pool.GetWorkerCount(),
		"throughput_mbps": float64(s.pool.GetThroughput()) / (1024 * 1024),
	}

	return stats
}

// GetUploadedFiles returns total files uploaded
func (s *DirectUploaderStage) GetUploadedFiles() int64 {
	return atomic.LoadInt64(&s.filesProcessed)
}

// GetUploadedBytes returns total bytes uploaded
func (s *DirectUploaderStage) GetUploadedBytes() int64 {
	return atomic.LoadInt64(&s.bytesProcessed)
}

// GetWorkerCount returns current worker count
func (s *DirectUploaderStage) GetWorkerCount() int {
	return s.pool.GetWorkerCount()
}
