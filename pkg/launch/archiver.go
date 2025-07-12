package launch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LocalArchiver handles the actual archival of files to S3
type LocalArchiver struct {
	config     ArchiveConfig
	logger     *slog.Logger
	
	// Job management
	activeJobs map[string]*ArchiveJob
	jobQueue   chan *ArchiveJob
	
	// Concurrency control
	semaphore  chan struct{}
	
	// State
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewLocalArchiver creates a new local archiver
func NewLocalArchiver(config ArchiveConfig, logger *slog.Logger) (*LocalArchiver, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	archiver := &LocalArchiver{
		config:     config,
		logger:     logger.With("component", "local-archiver"),
		activeJobs: make(map[string]*ArchiveJob),
		jobQueue:   make(chan *ArchiveJob, 100),
		semaphore:  make(chan struct{}, config.MaxConcurrent),
		ctx:        ctx,
		cancel:     cancel,
	}
	
	// Start job processors
	for i := 0; i < config.MaxConcurrent; i++ {
		archiver.wg.Add(1)
		go archiver.jobProcessor()
	}
	
	archiver.logger.Info("Local archiver initialized", "max_concurrent", config.MaxConcurrent)
	
	return archiver, nil
}

// SubmitJob submits a new archival job
func (la *LocalArchiver) SubmitJob(candidate *ArchiveCandidate) (*ArchiveJob, error) {
	la.mu.Lock()
	defer la.mu.Unlock()
	
	// Create job from candidate
	job := &ArchiveJob{
		ID:           generateJobID(),
		Path:         candidate.Path,
		Destination:  la.buildDestination(candidate),
		StorageClass: candidate.StorageClass,
		State:        JobStatePending,
		BytesTotal:   candidate.Size,
		StartTime:    time.Now(),
		Metadata:     candidate.Metadata,
	}
	
	// Add to active jobs
	la.activeJobs[job.ID] = job
	
	// Queue for processing
	select {
	case la.jobQueue <- job:
		la.logger.Info("Archive job submitted", "job_id", job.ID, "path", job.Path)
		return job, nil
	default:
		delete(la.activeJobs, job.ID)
		return nil, fmt.Errorf("job queue is full")
	}
}

// GetJob returns a job by ID
func (la *LocalArchiver) GetJob(jobID string) (*ArchiveJob, bool) {
	la.mu.RLock()
	defer la.mu.RUnlock()
	
	job, exists := la.activeJobs[jobID]
	if exists {
		jobCopy := *job
		return &jobCopy, true
	}
	
	return nil, false
}

// GetActiveJobs returns all active jobs
func (la *LocalArchiver) GetActiveJobs() []*ArchiveJob {
	la.mu.RLock()
	defer la.mu.RUnlock()
	
	jobs := make([]*ArchiveJob, 0, len(la.activeJobs))
	for _, job := range la.activeJobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}
	
	return jobs
}

// CancelJob cancels an active job
func (la *LocalArchiver) CancelJob(jobID string) error {
	la.mu.Lock()
	defer la.mu.Unlock()
	
	job, exists := la.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}
	
	job.State = JobStateCancelled
	endTime := time.Now()
	job.EndTime = &endTime
	
	la.logger.Info("Archive job cancelled", "job_id", jobID)
	
	return nil
}

// Stop gracefully stops the archiver
func (la *LocalArchiver) Stop() error {
	la.logger.Info("Stopping local archiver")
	
	la.cancel()
	la.wg.Wait()
	
	la.logger.Info("Local archiver stopped")
	
	return nil
}

// jobProcessor processes archival jobs
func (la *LocalArchiver) jobProcessor() {
	defer la.wg.Done()
	
	for {
		select {
		case <-la.ctx.Done():
			return
		case job := <-la.jobQueue:
			la.processJob(job)
		}
	}
}

// processJob processes a single archival job
func (la *LocalArchiver) processJob(job *ArchiveJob) {
	// Acquire semaphore
	la.semaphore <- struct{}{}
	defer func() { <-la.semaphore }()
	
	la.logger.Info("Starting archive job", "job_id", job.ID, "path", job.Path)
	
	// Update job state
	la.mu.Lock()
	job.State = JobStateRunning
	la.mu.Unlock()
	
	// Perform the actual archival
	err := la.executeArchival(job)
	
	// Update final job state
	la.mu.Lock()
	defer la.mu.Unlock()
	
	endTime := time.Now()
	job.EndTime = &endTime
	
	if err != nil {
		job.State = JobStateFailed
		job.Error = err.Error()
		la.logger.Error("Archive job failed", "job_id", job.ID, "error", err)
	} else {
		job.State = JobStateCompleted
		job.Progress = 1.0
		job.BytesCompleted = job.BytesTotal
		la.logger.Info("Archive job completed", "job_id", job.ID, "bytes", job.BytesTotal)
	}
	
	// Remove from active jobs after a delay (for status queries)
	go func(jobID string) {
		time.Sleep(5 * time.Minute)
		la.mu.Lock()
		delete(la.activeJobs, jobID)
		la.mu.Unlock()
	}(job.ID)
}

// executeArchival performs the actual archival process
func (la *LocalArchiver) executeArchival(job *ArchiveJob) error {
	// This is a simplified implementation
	// In a real implementation, this would:
	// 1. Create archive (tar/zip) if needed
	// 2. Compress data
	// 3. Upload to S3 with proper storage class
	// 4. Verify upload integrity
	// 5. Update progress throughout
	
	la.logger.Info("Executing archival", "job_id", job.ID, "destination", job.Destination)
	
	// Simulate archival process with progress updates
	steps := 10
	for i := 0; i < steps; i++ {
		select {
		case <-la.ctx.Done():
			return fmt.Errorf("archival cancelled")
		case <-time.After(100 * time.Millisecond): // Simulate work
			// Update progress
			la.mu.Lock()
			if job.State == JobStateCancelled {
				la.mu.Unlock()
				return fmt.Errorf("job was cancelled")
			}
			job.Progress = float64(i+1) / float64(steps)
			job.BytesCompleted = int64(job.Progress * float64(job.BytesTotal))
			la.mu.Unlock()
		}
	}
	
	// Simulate successful completion
	return nil
}

// buildDestination builds the S3 destination path for a candidate
func (la *LocalArchiver) buildDestination(candidate *ArchiveCandidate) string {
	// Use the configured destination as base
	baseDestination := la.config.Destination
	
	// Add metadata-based path components
	if dataType, exists := candidate.Metadata["data_type"]; exists {
		baseDestination = fmt.Sprintf("%s/%s", baseDestination, dataType)
	}
	
	// Add date-based organization
	now := time.Now()
	baseDestination = fmt.Sprintf("%s/%d/%02d", baseDestination, now.Year(), now.Month())
	
	// Add filename
	filename := fmt.Sprintf("%s-%d", candidate.Path[len(candidate.Path)-20:], candidate.ModTime.Unix())
	
	return fmt.Sprintf("%s/%s", baseDestination, filename)
}

// generateJobID generates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("job-%d", time.Now().UnixNano())
}