package s3optimization

import (
	"context"
	"sort"
	"sync"
	"time"
)

// AdaptiveScheduler intelligently schedules prefetch operations.
type AdaptiveScheduler struct {
	config               *PrefetchConfig
	jobQueue             *PriorityQueue
	scheduledJobs        map[string]*PrefetchJob
	completedJobs        map[string]*JobResult
	networkConditions    *NetworkConditions
	systemLoad           *SystemLoadInfo
	
	// Adaptive parameters
	windowSizeMultiplier float64
	timingMultiplier     float64
	aggressivenessLevel  float64
	
	// Scheduling statistics
	totalJobsScheduled   int64
	totalJobsCompleted   int64
	averageJobDuration   time.Duration
	
	mu                   sync.RWMutex
}

// PrefetchJob represents a prefetch job to be scheduled.
type PrefetchJob struct {
	ID            string
	Key           string
	Bucket        string
	Priority      float64
	ScheduledTime time.Time
	PredictedTime time.Time
	Confidence    float64
	EstimatedSize int64
	Retries       int
	MaxRetries    int
	
	// Execution tracking
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	Success       bool
	ErrorMessage  string
}

// JobResult represents the result of a completed prefetch job.
type JobResult struct {
	Job           *PrefetchJob
	Success       bool
	Duration      time.Duration
	BytesTransferred int64
	ErrorMessage  string
	CompletedAt   time.Time
}

// PriorityQueue implements a priority queue for prefetch jobs.
type PriorityQueue struct {
	jobs []*PrefetchJob
	mu   sync.RWMutex
}

// SystemLoadInfo contains system load information.
type SystemLoadInfo struct {
	CPUUsage         float64
	MemoryUsage      float64
	NetworkUtilization float64
	DiskIOUtilization float64
	LastUpdated      time.Time
}

// NewAdaptiveScheduler creates a new adaptive scheduler.
func NewAdaptiveScheduler(config *PrefetchConfig) *AdaptiveScheduler {
	return &AdaptiveScheduler{
		config:               config,
		jobQueue:             NewPriorityQueue(),
		scheduledJobs:        make(map[string]*PrefetchJob),
		completedJobs:        make(map[string]*JobResult),
		windowSizeMultiplier: 1.0,
		timingMultiplier:     1.0,
		aggressivenessLevel:  1.0,
	}
}

// ScheduleJobs schedules a batch of prefetch jobs.
func (as *AdaptiveScheduler) ScheduleJobs(ctx context.Context, jobs []*PrefetchJob) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	// Apply adaptive scheduling logic
	adaptedJobs := as.adaptJobScheduling(jobs)
	
	// Add jobs to queue and tracking
	for _, job := range adaptedJobs {
		job.ID = as.generateJobID()
		job.MaxRetries = 3 // Default retry limit
		
		as.jobQueue.Push(job)
		as.scheduledJobs[job.ID] = job
		as.totalJobsScheduled++
	}
	
	return nil
}

// GetNextJob gets the next job to execute based on priority and timing.
func (as *AdaptiveScheduler) GetNextJob() *PrefetchJob {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	// Check if any jobs are ready for execution
	now := time.Now()
	
	for {
		job := as.jobQueue.Peek()
		if job == nil {
			return nil // No jobs available
		}
		
		// Check if job is ready to execute
		if as.isJobReadyForExecution(job, now) {
			as.jobQueue.Pop()
			job.StartTime = now
			return job
		}
		
		// If the highest priority job isn't ready, no jobs are ready
		break
	}
	
	return nil
}

// CompleteJob marks a job as completed and records its result.
func (as *AdaptiveScheduler) CompleteJob(job *PrefetchJob, success bool, bytesTransferred int64, errorMessage string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	job.EndTime = time.Now()
	job.Duration = job.EndTime.Sub(job.StartTime)
	job.Success = success
	job.ErrorMessage = errorMessage
	
	// Record job result
	result := &JobResult{
		Job:              job,
		Success:          success,
		Duration:         job.Duration,
		BytesTransferred: bytesTransferred,
		ErrorMessage:     errorMessage,
		CompletedAt:      job.EndTime,
	}
	
	as.completedJobs[job.ID] = result
	delete(as.scheduledJobs, job.ID)
	
	as.totalJobsCompleted++
	as.updateAverageJobDuration(job.Duration)
	
	// Learn from job completion for future scheduling
	as.learnFromJobCompletion(result)
}

// RetryJob reschedules a failed job for retry.
func (as *AdaptiveScheduler) RetryJob(job *PrefetchJob, errorMessage string) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	if job.Retries >= job.MaxRetries {
		// Mark as permanently failed
		as.CompleteJob(job, false, 0, "max retries exceeded: "+errorMessage)
		return nil
	}
	
	// Increment retry count and reschedule
	job.Retries++
	job.ErrorMessage = errorMessage
	
	// Apply exponential backoff
	backoffDelay := time.Duration(job.Retries*job.Retries) * time.Second * 10
	job.ScheduledTime = time.Now().Add(backoffDelay)
	
	// Reduce priority slightly for retries
	job.Priority *= 0.9
	
	// Re-add to queue
	as.jobQueue.Push(job)
	
	return nil
}

// IsScheduled checks if a key is already scheduled for prefetching.
func (as *AdaptiveScheduler) IsScheduled(key string) bool {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	for _, job := range as.scheduledJobs {
		if job.Key == key {
			return true
		}
	}
	
	return false
}

// UpdateNetworkConditions updates network conditions for adaptive scheduling.
func (as *AdaptiveScheduler) UpdateNetworkConditions(conditions *NetworkConditions) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	as.networkConditions = conditions
	as.adaptToNetworkConditions()
}

// UpdateSystemLoad updates system load information.
func (as *AdaptiveScheduler) UpdateSystemLoad(loadInfo *SystemLoadInfo) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	as.systemLoad = loadInfo
	as.adaptToSystemLoad()
}

// SetWindowSizeMultiplier sets the window size multiplier.
func (as *AdaptiveScheduler) SetWindowSizeMultiplier(multiplier float64) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.windowSizeMultiplier = multiplier
}

// GetWindowSizeMultiplier gets the current window size multiplier.
func (as *AdaptiveScheduler) GetWindowSizeMultiplier() float64 {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.windowSizeMultiplier
}

// SetTimingMultiplier sets the timing multiplier.
func (as *AdaptiveScheduler) SetTimingMultiplier(multiplier float64) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.timingMultiplier = multiplier
}

// GetTimingMultiplier gets the current timing multiplier.
func (as *AdaptiveScheduler) GetTimingMultiplier() float64 {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.timingMultiplier
}

// AdaptAggressiveness adapts the aggressiveness level.
func (as *AdaptiveScheduler) AdaptAggressiveness(multiplier float64) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	as.aggressivenessLevel *= multiplier
	
	// Clamp to reasonable bounds
	if as.aggressivenessLevel < 0.1 {
		as.aggressivenessLevel = 0.1
	} else if as.aggressivenessLevel > 3.0 {
		as.aggressivenessLevel = 3.0
	}
}

// GetSchedulingStats returns scheduling statistics.
func (as *AdaptiveScheduler) GetSchedulingStats() *SchedulingStats {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	successRate := 0.0
	if as.totalJobsCompleted > 0 {
		successfulJobs := int64(0)
		for _, result := range as.completedJobs {
			if result.Success {
				successfulJobs++
			}
		}
		successRate = float64(successfulJobs) / float64(as.totalJobsCompleted)
	}
	
	return &SchedulingStats{
		TotalJobsScheduled: as.totalJobsScheduled,
		TotalJobsCompleted: as.totalJobsCompleted,
		JobsInQueue:        int64(as.jobQueue.Size()),
		SuccessRate:        successRate,
		AverageJobDuration: as.averageJobDuration,
		AggressivenessLevel: as.aggressivenessLevel,
	}
}

// adaptJobScheduling applies adaptive logic to job scheduling.
func (as *AdaptiveScheduler) adaptJobScheduling(jobs []*PrefetchJob) []*PrefetchJob {
	// Sort jobs by priority
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Priority > jobs[j].Priority
	})
	
	// Apply window size multiplier
	maxJobs := int(float64(as.config.PrefetchWindowSize) * as.windowSizeMultiplier)
	if len(jobs) > maxJobs {
		jobs = jobs[:maxJobs]
	}
	
	// Adjust timing based on timing multiplier
	for _, job := range jobs {
		timeAdjustment := time.Duration(float64(time.Until(job.PredictedTime)) * as.timingMultiplier)
		job.ScheduledTime = time.Now().Add(timeAdjustment)
		
		// Apply aggressiveness level to priority
		job.Priority *= as.aggressivenessLevel
	}
	
	return jobs
}

// isJobReadyForExecution checks if a job is ready for execution.
func (as *AdaptiveScheduler) isJobReadyForExecution(job *PrefetchJob, now time.Time) bool {
	// Check if scheduled time has passed
	if now.Before(job.ScheduledTime) {
		return false
	}
	
	// Check system load constraints
	if as.systemLoad != nil {
		if as.systemLoad.CPUUsage > 90.0 || as.systemLoad.MemoryUsage > 95.0 {
			return false // System too busy
		}
	}
	
	// Check network conditions
	if as.networkConditions != nil {
		if as.networkConditions.Bandwidth < 1.0 { // Less than 1 Mbps
			return false // Network too slow
		}
	}
	
	return true
}

// adaptToNetworkConditions adapts scheduling based on network conditions.
func (as *AdaptiveScheduler) adaptToNetworkConditions() {
	if as.networkConditions == nil {
		return
	}
	
	// Adjust aggressiveness based on bandwidth
	if as.networkConditions.Bandwidth > 100.0 { // High bandwidth
		as.aggressivenessLevel = 1.5
	} else if as.networkConditions.Bandwidth < 10.0 { // Low bandwidth
		as.aggressivenessLevel = 0.5
	} else {
		as.aggressivenessLevel = 1.0
	}
	
	// Adjust timing based on latency
	if as.networkConditions.RTT > time.Millisecond*200 { // High latency
		as.timingMultiplier = 1.5 // Start prefetching earlier
	} else {
		as.timingMultiplier = 1.0
	}
}

// adaptToSystemLoad adapts scheduling based on system load.
func (as *AdaptiveScheduler) adaptToSystemLoad() {
	if as.systemLoad == nil {
		return
	}
	
	// Reduce aggressiveness under high system load
	avgLoad := (as.systemLoad.CPUUsage + as.systemLoad.MemoryUsage) / 2.0
	
	if avgLoad > 80.0 {
		as.aggressivenessLevel *= 0.5 // Reduce prefetching
	} else if avgLoad < 30.0 {
		as.aggressivenessLevel *= 1.2 // Increase prefetching
	}
	
	// Clamp aggressiveness
	if as.aggressivenessLevel < 0.1 {
		as.aggressivenessLevel = 0.1
	} else if as.aggressivenessLevel > 3.0 {
		as.aggressivenessLevel = 3.0
	}
}

// learnFromJobCompletion learns from job completion to improve future scheduling.
func (as *AdaptiveScheduler) learnFromJobCompletion(result *JobResult) {
	// Adjust scheduling parameters based on job success/failure
	if result.Success {
		// Successful job - slightly increase aggressiveness
		as.aggressivenessLevel *= 1.01
	} else {
		// Failed job - slightly decrease aggressiveness
		as.aggressivenessLevel *= 0.99
	}
	
	// Learn from timing accuracy
	if result.Job.PredictedTime.IsZero() {
		return
	}
	
	actualAccessTime := result.CompletedAt
	predictedTime := result.Job.PredictedTime
	timingError := actualAccessTime.Sub(predictedTime)
	
	if timingError < 0 {
		timingError = -timingError
	}
	
	// Adjust timing multiplier based on prediction accuracy
	if timingError < time.Minute*5 { // Very accurate
		as.timingMultiplier *= 1.01
	} else if timingError > time.Minute*30 { // Poor accuracy
		as.timingMultiplier *= 0.99
	}
	
	// Clamp timing multiplier
	if as.timingMultiplier < 0.5 {
		as.timingMultiplier = 0.5
	} else if as.timingMultiplier > 2.0 {
		as.timingMultiplier = 2.0
	}
}

// updateAverageJobDuration updates the average job duration.
func (as *AdaptiveScheduler) updateAverageJobDuration(duration time.Duration) {
	if as.totalJobsCompleted == 1 {
		as.averageJobDuration = duration
	} else {
		// Exponential moving average
		alpha := 0.1
		newAvg := time.Duration(alpha*float64(duration) + (1-alpha)*float64(as.averageJobDuration))
		as.averageJobDuration = newAvg
	}
}

// generateJobID generates a unique job ID.
func (as *AdaptiveScheduler) generateJobID() string {
	return "job_" + string(rune(time.Now().UnixNano()))
}

// NewPriorityQueue creates a new priority queue.
func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		jobs: make([]*PrefetchJob, 0),
	}
}

// Push adds a job to the priority queue.
func (pq *PriorityQueue) Push(job *PrefetchJob) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	pq.jobs = append(pq.jobs, job)
	pq.heapifyUp(len(pq.jobs) - 1)
}

// Pop removes and returns the highest priority job.
func (pq *PriorityQueue) Pop() *PrefetchJob {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	if len(pq.jobs) == 0 {
		return nil
	}
	
	job := pq.jobs[0]
	lastIndex := len(pq.jobs) - 1
	pq.jobs[0] = pq.jobs[lastIndex]
	pq.jobs = pq.jobs[:lastIndex]
	
	if len(pq.jobs) > 0 {
		pq.heapifyDown(0)
	}
	
	return job
}

// Peek returns the highest priority job without removing it.
func (pq *PriorityQueue) Peek() *PrefetchJob {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	if len(pq.jobs) == 0 {
		return nil
	}
	
	return pq.jobs[0]
}

// Size returns the number of jobs in the queue.
func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.jobs)
}

// heapifyUp maintains heap property upward.
func (pq *PriorityQueue) heapifyUp(index int) {
	for index > 0 {
		parentIndex := (index - 1) / 2
		if pq.jobs[index].Priority <= pq.jobs[parentIndex].Priority {
			break
		}
		
		pq.jobs[index], pq.jobs[parentIndex] = pq.jobs[parentIndex], pq.jobs[index]
		index = parentIndex
	}
}

// heapifyDown maintains heap property downward.
func (pq *PriorityQueue) heapifyDown(index int) {
	for {
		leftChild := 2*index + 1
		rightChild := 2*index + 2
		largest := index
		
		if leftChild < len(pq.jobs) && pq.jobs[leftChild].Priority > pq.jobs[largest].Priority {
			largest = leftChild
		}
		
		if rightChild < len(pq.jobs) && pq.jobs[rightChild].Priority > pq.jobs[largest].Priority {
			largest = rightChild
		}
		
		if largest == index {
			break
		}
		
		pq.jobs[index], pq.jobs[largest] = pq.jobs[largest], pq.jobs[index]
		index = largest
	}
}

// SchedulingStats contains scheduling performance statistics.
type SchedulingStats struct {
	TotalJobsScheduled  int64         `json:"total_jobs_scheduled"`
	TotalJobsCompleted  int64         `json:"total_jobs_completed"`
	JobsInQueue         int64         `json:"jobs_in_queue"`
	SuccessRate         float64       `json:"success_rate"`
	AverageJobDuration  time.Duration `json:"average_job_duration"`
	AggressivenessLevel float64       `json:"aggressiveness_level"`
}