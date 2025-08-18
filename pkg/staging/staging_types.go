package staging

import "time"

// Core types for staging system

// AdvancedOptimizationConfig defines configuration for advanced staging optimization.
type AdvancedOptimizationConfig struct {
	WorkerPoolSize      int
	MaxConcurrentJobs   int
	MinConcurrency      int
	MaxConcurrency      int
	MinChunkSizeMB      int
	MaxChunkSizeMB      int
}

// DefaultAdvancedOptimizationConfig returns sensible defaults.
func DefaultAdvancedOptimizationConfig() *AdvancedOptimizationConfig {
	return &AdvancedOptimizationConfig{
		WorkerPoolSize:    4,
		MaxConcurrentJobs: 8,
		MinConcurrency:    1,
		MaxConcurrency:    16,
		MinChunkSizeMB:    5,
		MaxChunkSizeMB:    100,
	}
}

// AdvancedStagingJob represents a staging job.
type AdvancedStagingJob struct {
	ID          string
	Size        int64
	ContentType string
	Priority    int
	Deadline    time.Time
}

// JobHandle represents a handle to a submitted job.
type JobHandle struct {
	JobID       string
	SubmittedAt time.Time
	Status      JobStatus
}

// JobStatus represents the status of a job.
type JobStatus int

const (
	JobStatusQueued JobStatus = iota
	JobStatusProcessing
	JobStatusCompleted
	JobStatusFailed
)

// OptimizationState tracks the current optimization state.
type OptimizationState struct {
	TotalJobsProcessed     int64
	AverageLatencyMs       float64
	ThroughputMBps         float64
	ErrorRate              float64
	OptimizationScore      float64
	CPUUtilization         float64
	MemoryUtilization      float64
	NetworkUtilization     float64
	IOUtilization          float64
	SchedulingEfficiency   float64
	LoadBalanceEfficiency  float64
	MemoryEfficiency       float64
	PredictionAccuracy     float64
	CurrentConcurrency     int
	CurrentChunkSizeMB     int
	CurrentBufferSizeMB    int
	AdaptationCount        int64
	LastOptimization       time.Time
}

// SchedulingMetrics represents scheduling performance metrics.
type SchedulingMetrics struct {
	Efficiency float64
}

// JobProfile represents job characteristics for optimization.
type JobProfile struct {
	JobID              string
	Size               int64
	ContentType        string
	Priority           int
	Deadline           time.Time
	MemoryRequirement  int64
	NetworkRequirement float64
}

// ComprehensiveMetrics represents comprehensive system metrics.
type ComprehensiveMetrics struct {
	ThroughputScore     float64
	NormalizedLatency   float64
	ResourceEfficiency  float64
	CPUUtilization      float64
	MemoryUtilization   float64
	NetworkUtilization  float64
	IOUtilization       float64
}

// MemoryMetrics represents memory system metrics.
type MemoryMetrics struct {
	Efficiency float64
}