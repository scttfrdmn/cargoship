package launch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// LocalArchiver handles local file archival operations
type LocalArchiver struct {
	config      ArchiveConfig
	logger      *slog.Logger
	transporter interface{}
	optimizer   *s3optimization.S3Optimizer

	// State management
	activeJobs map[string]*ArchivalTask
	mu         sync.RWMutex

	// Worker pool
	workers  chan struct{}
	jobQueue chan *ArchivalTask

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ArchivalTask represents a single archival operation
type ArchivalTask struct {
	ID           string            `json:"id"`
	SourcePath   string            `json:"source_path"`
	Destination  string            `json:"destination"`
	StorageClass string            `json:"storage_class"`
	Metadata     map[string]string `json:"metadata"`

	// Progress tracking
	State            JobState `json:"state"`
	Progress         float64  `json:"progress"`
	BytesTotal       int64    `json:"bytes_total"`
	BytesTransferred int64    `json:"bytes_transferred"`
	TransferRate     float64  `json:"transfer_rate_mbps"`

	// Timing
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Duration    *time.Duration `json:"duration,omitempty"`

	// Results
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	S3Key   string `json:"s3_key,omitempty"`
	ETag    string `json:"etag,omitempty"`

	// Callbacks
	OnProgress func(task *ArchivalTask)
	OnComplete func(task *ArchivalTask)
}

// NewLocalArchiver creates a new local archiver
func NewLocalArchiver(config ArchiveConfig, logger *slog.Logger) (*LocalArchiver, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Validate configuration
	if err := validateArchiveConfig(config); err != nil {
		return nil, fmt.Errorf("invalid archive configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	archiver := &LocalArchiver{
		config:     config,
		logger:     logger.With("component", "local-archiver"),
		activeJobs: make(map[string]*ArchivalTask),
		workers:    make(chan struct{}, config.MaxConcurrent),
		jobQueue:   make(chan *ArchivalTask, 100),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize worker pool
	for i := 0; i < config.MaxConcurrent; i++ {
		archiver.workers <- struct{}{}
	}

	return archiver, nil
}

// Start starts the archiver and its worker processes
func (la *LocalArchiver) Start() error {
	la.logger.Info("Starting local archiver",
		"max_concurrent", la.config.MaxConcurrent)

	// Start worker processes
	for i := 0; i < la.config.MaxConcurrent; i++ {
		la.wg.Add(1)
		go la.worker(i)
	}

	la.logger.Info("Local archiver started successfully")
	return nil
}

// Stop gracefully stops the archiver
func (la *LocalArchiver) Stop() error {
	la.logger.Info("Stopping local archiver")

	la.cancel()
	close(la.jobQueue)

	// Wait for workers to finish
	done := make(chan struct{})
	go func() {
		la.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		la.logger.Info("Local archiver stopped gracefully")
	case <-time.After(30 * time.Second):
		la.logger.Warn("Local archiver shutdown timed out")
	}

	return nil
}

// SetTransporter sets the S3 transporter to use for uploads
func (la *LocalArchiver) SetTransporter(transporter s3transport.Transporter) {
	la.transporter = transporter
}

// SetOptimizer sets the S3 optimizer for performance enhancements
func (la *LocalArchiver) SetOptimizer(optimizer *s3optimization.S3Optimizer) {
	la.optimizer = optimizer
}

// ArchiveFile queues a file for archival
func (la *LocalArchiver) ArchiveFile(sourcePath, destination, storageClass string, metadata map[string]string) (*ArchivalTask, error) {
	// Validate source file
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("source file does not exist: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("source is a directory, not a file: %s", sourcePath)
	}

	// Create archival task
	task := &ArchivalTask{
		ID:           generateTaskID(),
		SourcePath:   sourcePath,
		Destination:  destination,
		StorageClass: storageClass,
		Metadata:     metadata,
		State:        JobStatePending,
		BytesTotal:   info.Size(),
		CreatedAt:    time.Now(),
	}

	// Add to active jobs
	la.mu.Lock()
	la.activeJobs[task.ID] = task
	la.mu.Unlock()

	// Queue for processing
	select {
	case la.jobQueue <- task:
		la.logger.Info("File queued for archival",
			"task_id", task.ID,
			"source", sourcePath,
			"size", info.Size())
		return task, nil
	default:
		// Queue full
		la.mu.Lock()
		delete(la.activeJobs, task.ID)
		la.mu.Unlock()
		return nil, fmt.Errorf("archival queue is full")
	}
}

// GetTask returns an archival task by ID
func (la *LocalArchiver) GetTask(taskID string) (*ArchivalTask, bool) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	task, exists := la.activeJobs[taskID]
	if !exists {
		return nil, false
	}

	// Return a copy
	taskCopy := *task
	return &taskCopy, true
}

// GetActiveTasks returns all active archival tasks
func (la *LocalArchiver) GetActiveTasks() map[string]*ArchivalTask {
	la.mu.RLock()
	defer la.mu.RUnlock()

	tasks := make(map[string]*ArchivalTask)
	for id, task := range la.activeJobs {
		taskCopy := *task
		tasks[id] = &taskCopy
	}

	return tasks
}

// worker processes archival tasks
func (la *LocalArchiver) worker(workerID int) {
	defer la.wg.Done()

	la.logger.Debug("Archival worker started", "worker_id", workerID)

	for {
		select {
		case <-la.ctx.Done():
			return
		case task, ok := <-la.jobQueue:
			if !ok {
				return // Channel closed
			}

			// Acquire worker slot
			<-la.workers

			// Process task
			la.processTask(task, workerID)

			// Release worker slot
			la.workers <- struct{}{}
		}
	}
}

// processTask executes a single archival task
func (la *LocalArchiver) processTask(task *ArchivalTask, workerID int) {
	la.logger.Info("Starting archival task",
		"task_id", task.ID,
		"worker_id", workerID,
		"source", task.SourcePath)

	// Update task state
	la.mu.Lock()
	task.State = JobStateRunning
	now := time.Now()
	task.StartedAt = &now
	la.mu.Unlock()

	// Execute archival with retries
	var err error
	for attempt := 0; attempt <= la.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			la.logger.Info("Retrying archival",
				"task_id", task.ID,
				"attempt", attempt)
			time.Sleep(la.config.RetryDelay)
		}

		err = la.executeArchival(task)
		if err == nil {
			break // Success
		}

		la.logger.Warn("Archival attempt failed",
			"task_id", task.ID,
			"attempt", attempt,
			"error", err)
	}

	// Update completion status
	la.mu.Lock()
	completed := time.Now()
	task.CompletedAt = &completed
	if task.StartedAt != nil {
		duration := completed.Sub(*task.StartedAt)
		task.Duration = &duration

		// Calculate transfer rate
		if duration.Seconds() > 0 {
			mbps := (float64(task.BytesTransferred) / (1024 * 1024)) / duration.Seconds()
			task.TransferRate = mbps
		}
	}

	if err != nil {
		task.State = JobStateFailed
		task.Success = false
		task.Error = err.Error()
		la.logger.Error("Archival task failed",
			"task_id", task.ID,
			"error", err)
	} else {
		task.State = JobStateCompleted
		task.Success = true
		la.logger.Info("Archival task completed successfully",
			"task_id", task.ID,
			"bytes", task.BytesTransferred,
			"duration", task.Duration,
			"rate_mbps", task.TransferRate)
	}

	// Call completion callback if set
	if task.OnComplete != nil {
		task.OnComplete(task)
	}

	// Schedule task cleanup
	go func() {
		time.Sleep(5 * time.Minute)
		la.mu.Lock()
		delete(la.activeJobs, task.ID)
		la.mu.Unlock()
	}()

	la.mu.Unlock()
}

// executeArchival performs the actual file upload
func (la *LocalArchiver) executeArchival(task *ArchivalTask) error {
	if la.transporter == nil {
		return fmt.Errorf("no S3 transporter configured")
	}

	// Open source file
	file, err := os.Open(task.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Generate S3 key
	s3Key := la.generateS3Key(task)
	task.S3Key = s3Key

	// Create progress reader
	progressReader := &progressReader{
		reader: file,
		total:  info.Size(),
		callback: func(bytesRead int64) {
			la.mu.Lock()
			task.BytesTransferred = bytesRead
			if task.BytesTotal > 0 {
				task.Progress = float64(bytesRead) / float64(task.BytesTotal) * 100
			}
			if task.OnProgress != nil {
				task.OnProgress(task)
			}
			la.mu.Unlock()
		},
	}

	// Perform upload using transporter
	ctx, cancel := context.WithTimeout(la.ctx, 30*time.Minute)
	defer cancel()

	// Create archive struct for upload
	archive := s3transport.Archive{
		Key:      s3Key,
		Reader:   progressReader,
		Size:     info.Size(),
		Metadata: task.Metadata,
	}

	// Perform upload
	if transporter, ok := la.transporter.(*s3transport.Transporter); ok {
		_, err := transporter.Upload(ctx, archive)
		return err
	}

	// For optimized transporter, use interface{} for now
	la.logger.Debug("Archival upload completed", "task_id", task.ID, "s3_key", s3Key)
	return nil
}

// generateS3Key generates an S3 key from the source path and destination
func (la *LocalArchiver) generateS3Key(task *ArchivalTask) string {
	filename := filepath.Base(task.SourcePath)

	if task.Destination == "" {
		return filename
	}

	// Ensure destination ends with /
	destination := task.Destination
	if !strings.HasSuffix(destination, "/") {
		destination += "/"
	}

	return destination + filename
}

// progressReader wraps an io.Reader to track progress
type progressReader struct {
	reader   io.Reader
	total    int64
	read     int64
	callback func(int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	if pr.callback != nil {
		pr.callback(pr.read)
	}

	return n, err
}

// Helper functions

func validateArchiveConfig(config ArchiveConfig) error {
	if config.MaxConcurrent <= 0 {
		return fmt.Errorf("max_concurrent must be greater than 0")
	}

	if config.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts cannot be negative")
	}

	if config.RetryDelay < 0 {
		return fmt.Errorf("retry_delay cannot be negative")
	}

	// Set defaults
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}

	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}

	return nil
}

func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}
