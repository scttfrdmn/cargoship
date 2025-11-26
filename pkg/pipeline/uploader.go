package pipeline

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// UploaderStage uploads archives to S3 using multipart upload
type UploaderStage struct {
	config *UploaderConfig
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
	jobsFailed     int64
	bytesProcessed int64
}

// NewUploaderStage creates a new uploader stage
func NewUploaderStage(config *UploaderConfig, input <-chan *Job, output chan<- *Job) (*UploaderStage, error) {
	if config == nil {
		return nil, fmt.Errorf("uploader config cannot be nil")
	}

	// Apply defaults
	if config.PartSize == 0 {
		config.PartSize = 5 * 1024 * 1024 // 5MB default
	}
	if config.Concurrency == 0 {
		config.Concurrency = 4
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UploaderStage{
		config: config,
		input:  input,
		output: output,
		pool:   NewWorkerPool(ctx, config.Workers),
		ctx:    ctx,
		cancel: cancel,
		stats: StageStats{
			Name: "uploader",
		},
	}, nil
}

// Name returns the stage name
func (s *UploaderStage) Name() string {
	return "uploader"
}

// Start starts the uploader stage
func (s *UploaderStage) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < s.config.Workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}

	// Goroutine to close output when all workers done
	go func() {
		s.wg.Wait()
		close(s.output)
	}()

	return nil
}

// Stop stops the uploader stage
func (s *UploaderStage) Stop() error {
	s.cancel()
	s.pool.Stop()
	s.wg.Wait()
	return nil
}

// Process processes a single upload job
func (s *UploaderStage) Process(ctx context.Context, job *Job) error {
	startTime := time.Now()
	defer func() {
		if job.Archive != nil {
			_ = job.Archive.Close()
		}
	}()

	// Upload with retries
	var lastErr error
	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.RetryDelay * time.Duration(attempt)):
			}
		}

		if err := s.uploadToS3(ctx, job); err != nil {
			lastErr = err
			atomic.AddInt64(&s.jobsFailed, 1)
			continue
		}

		// Success
		atomic.AddInt64(&s.jobsProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, job.ArchiveSize)

		s.mu.Lock()
		s.stats.TotalTime += time.Since(startTime)
		s.mu.Unlock()

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", s.config.MaxRetries, lastErr)
}

// Stats returns stage statistics
func (s *UploaderStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.JobsFailed = atomic.LoadInt64(&s.jobsFailed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.config.Workers - s.pool.AvailableWorkers()

	if stats.JobsProcessed > 0 {
		stats.AverageTime = stats.TotalTime / time.Duration(stats.JobsProcessed)
	}

	return stats
}

// worker processes jobs from the input channel
func (s *UploaderStage) worker(ctx context.Context) {
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

			// Send to output (result channel)
			select {
			case <-ctx.Done():
				return
			case s.output <- job:
			}
		}
	}
}

// uploadToS3 uploads the archive to S3 using multipart upload
func (s *UploaderStage) uploadToS3(ctx context.Context, job *Job) error {
	// Check if we should use multipart upload
	useMultipart := job.ArchiveSize > s.config.PartSize

	if useMultipart {
		return s.multipartUpload(ctx, job)
	}

	return s.simpleUpload(ctx, job)
}

// simpleUpload performs a simple single-part upload
func (s *UploaderStage) simpleUpload(ctx context.Context, job *Job) error {
	// TODO: Integrate with actual S3 client
	// For now, just read and discard the stream to simulate upload

	buf := make([]byte, 64*1024) // 64KB buffer
	var totalBytes int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := job.Archive.Read(buf)
		totalBytes += int64(n)

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error during upload: %w", err)
		}
	}

	// Update actual size
	job.ArchiveSize = totalBytes

	return nil
}

// multipartUpload performs a multipart upload for large archives
func (s *UploaderStage) multipartUpload(ctx context.Context, job *Job) error {
	// TODO: Integrate with actual S3 multipart upload API
	// For now, simulate by reading in parts

	partBuffer := make([]byte, s.config.PartSize)
	var totalBytes int64
	partNumber := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read a part
		n, err := io.ReadFull(job.Archive, partBuffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if n > 0 {
				// Upload final partial part (partNumber not incremented since we break)
				totalBytes += int64(n)
			}
			break
		}
		if err != nil {
			return fmt.Errorf("read error during multipart upload: %w", err)
		}

		// Upload part (simulated)
		partNumber++
		totalBytes += int64(n)

		// In real implementation, we would:
		// 1. Upload part to S3
		// 2. Collect ETag
		// 3. Handle concurrent part uploads
	}

	// Complete multipart upload (simulated)
	// In real implementation: CompleteMultipartUpload with collected ETags

	// Update actual size
	job.ArchiveSize = totalBytes

	return nil
}

// MultipartUploadTracker tracks progress of multipart uploads
type MultipartUploadTracker struct {
	mu            sync.Mutex
	uploadID      string
	parts         map[int]string // partNumber -> ETag
	bytesUploaded int64
	totalParts    int
}

// NewMultipartUploadTracker creates a new tracker
func NewMultipartUploadTracker(uploadID string) *MultipartUploadTracker {
	return &MultipartUploadTracker{
		uploadID: uploadID,
		parts:    make(map[int]string),
	}
}

// AddPart records a completed part
func (t *MultipartUploadTracker) AddPart(partNumber int, etag string, size int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.parts[partNumber] = etag
	t.bytesUploaded += size
	t.totalParts++
}

// GetParts returns all completed parts in order
func (t *MultipartUploadTracker) GetParts() map[int]string {
	t.mu.Lock()
	defer t.mu.Unlock()

	parts := make(map[int]string, len(t.parts))
	for k, v := range t.parts {
		parts[k] = v
	}
	return parts
}

// BytesUploaded returns total bytes uploaded
func (t *MultipartUploadTracker) BytesUploaded() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.bytesUploaded
}

// Progress returns upload progress (0.0 to 1.0)
func (t *MultipartUploadTracker) Progress() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.totalParts == 0 {
		return 0.0
	}

	return float64(len(t.parts)) / float64(t.totalParts)
}
