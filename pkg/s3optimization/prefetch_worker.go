package s3optimization

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PrefetchWorker executes prefetch operations in parallel.
type PrefetchWorker struct {
	id               int
	prefetcher       *PredictivePrefetcher
	logger           *slog.Logger
	isRunning        bool
	stopChan         chan struct{}
	jobChan          chan *PrefetchJob
	wg               sync.WaitGroup
	
	// Performance metrics
	jobsProcessed    int64
	totalBytes       int64
	totalDuration    time.Duration
	successCount     int64
	errorCount       int64
}

// NetworkOptimizer optimizes prefetch operations based on network conditions.
type NetworkOptimizer struct {
	config              *PrefetchConfig
	currentCondition    *NetworkCondition
	conditionHistory    []*NetworkCondition
	adaptationEngine    *NetworkAdaptationEngine
	bandwidthEstimator  *BandwidthEstimator
	mu                  sync.RWMutex
}

// NetworkCondition represents current network conditions.
type NetworkCondition struct {
	Bandwidth       float64
	Latency         time.Duration
	PacketLoss      float64
	Jitter          time.Duration
	Reliability     float64
	Timestamp       time.Time
	QualityScore    float64
}

// NetworkAdaptationEngine adapts network behavior based on conditions.
type NetworkAdaptationEngine struct {
	adaptationRules []*AdaptationRule
	learningRate    float64
	history         []*AdaptationResult
}

// AdaptationRule defines a network adaptation rule.
type AdaptationRule struct {
	Name        string
	Condition   func(*NetworkCondition) bool
	Action      func(*PrefetchJob) *JobOptimization
	Priority    float64
	Enabled     bool
}

// JobOptimization contains optimizations to apply to a job.
type JobOptimization struct {
	ChunkSize       int64
	Concurrency     int
	RetryStrategy   string
	TimeoutAdjust   time.Duration
	CompressionHint string
}

// AdaptationResult tracks the result of applying an adaptation.
type AdaptationResult struct {
	Rule        *AdaptationRule
	Job         *PrefetchJob
	Improvement float64
	Timestamp   time.Time
}

// BandwidthEstimator estimates available bandwidth.
type BandwidthEstimator struct {
	measurements []BandwidthMeasurement
	windowSize   int
	mu           sync.RWMutex
}

// BandwidthMeasurement represents a bandwidth measurement.
type BandwidthMeasurement struct {
	Bandwidth float64
	Timestamp time.Time
	Accuracy  float64
}

// NewPrefetchWorker creates a new prefetch worker.
func NewPrefetchWorker(id int, prefetcher *PredictivePrefetcher, logger *slog.Logger) *PrefetchWorker {
	return &PrefetchWorker{
		id:         id,
		prefetcher: prefetcher,
		logger:     logger.With("worker_id", id),
		stopChan:   make(chan struct{}),
		jobChan:    make(chan *PrefetchJob, 10), // Buffered channel
	}
}

// Start starts the prefetch worker.
func (pw *PrefetchWorker) Start(ctx context.Context) {
	pw.isRunning = true
	pw.wg.Add(1)
	
	go func() {
		defer pw.wg.Done()
		pw.workerLoop(ctx)
	}()
	
	pw.logger.Debug("prefetch worker started")
}

// Stop stops the prefetch worker.
func (pw *PrefetchWorker) Stop() {
	if !pw.isRunning {
		return
	}
	
	close(pw.stopChan)
	pw.wg.Wait()
	pw.isRunning = false
	
	pw.logger.Debug("prefetch worker stopped")
}

// SubmitJob submits a job to the worker.
func (pw *PrefetchWorker) SubmitJob(job *PrefetchJob) bool {
	if !pw.isRunning {
		return false
	}
	
	select {
	case pw.jobChan <- job:
		return true
	default:
		return false // Channel full
	}
}

// GetStats returns worker statistics.
func (pw *PrefetchWorker) GetStats() *WorkerStats {
	return &WorkerStats{
		ID:            pw.id,
		JobsProcessed: pw.jobsProcessed,
		TotalBytes:    pw.totalBytes,
		TotalDuration: pw.totalDuration,
		SuccessCount:  pw.successCount,
		ErrorCount:    pw.errorCount,
		IsRunning:     pw.isRunning,
	}
}

// workerLoop is the main worker loop.
func (pw *PrefetchWorker) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Millisecond * 100) // Check for jobs every 100ms
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-pw.stopChan:
			return
		case job := <-pw.jobChan:
			pw.processJob(ctx, job)
		case <-ticker.C:
			// Check scheduler for new jobs
			if len(pw.jobChan) < cap(pw.jobChan)/2 { // Don't overwhelm the channel
				if job := pw.prefetcher.adaptiveScheduler.GetNextJob(); job != nil {
					pw.jobChan <- job
				}
			}
		}
	}
}

// processJob processes a single prefetch job.
func (pw *PrefetchWorker) processJob(ctx context.Context, job *PrefetchJob) {
	startTime := time.Now()
	pw.jobsProcessed++
	
	pw.logger.Debug("processing prefetch job",
		"job_id", job.ID,
		"key", job.Key,
		"priority", job.Priority)
	
	// Apply network optimizations
	optimization := pw.prefetcher.networkOptimizer.OptimizeJob(job)
	
	// Execute the prefetch
	success, bytesTransferred, err := pw.executePrefetch(ctx, job, optimization)
	
	duration := time.Since(startTime)
	pw.totalDuration += duration
	
	if success {
		pw.successCount++
		pw.totalBytes += bytesTransferred
		pw.logger.Debug("prefetch job completed successfully",
			"job_id", job.ID,
			"duration", duration,
			"bytes", bytesTransferred)
	} else {
		pw.errorCount++
		errorMsg := ""
		if err != nil {
			errorMsg = err.Error()
		}
		
		pw.logger.Warn("prefetch job failed",
			"job_id", job.ID,
			"error", errorMsg,
			"duration", duration)
		
		// Retry logic
		if job.Retries < job.MaxRetries {
			pw.prefetcher.adaptiveScheduler.RetryJob(job, errorMsg)
			return
		}
	}
	
	// Complete the job
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}
	pw.prefetcher.adaptiveScheduler.CompleteJob(job, success, bytesTransferred, errorMsg)
}

// executePrefetch executes the actual prefetch operation.
func (pw *PrefetchWorker) executePrefetch(ctx context.Context, job *PrefetchJob, optimization *JobOptimization) (bool, int64, error) {
	// Create S3 GetObject input
	input := &s3.GetObjectInput{
		Bucket: &job.Bucket,
		Key:    &job.Key,
	}
	
	// Apply timeout from optimization
	fetchCtx := ctx
	if optimization.TimeoutAdjust > 0 {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(ctx, optimization.TimeoutAdjust)
		defer cancel()
	}
	
	// Fetch object using optimized S3 client
	result, err := pw.prefetcher.optimizer.GetObjectOptimized(fetchCtx, input)
	if err != nil {
		return false, 0, fmt.Errorf("failed to fetch object: %w", err)
	}
	defer result.Body.Close()
	
	// Read object data
	data := make([]byte, job.EstimatedSize)
	bytesRead, err := result.Body.Read(data)
	if err != nil && err.Error() != "EOF" {
		return false, 0, fmt.Errorf("failed to read object data: %w", err)
	}
	
	// Trim data to actual size
	actualData := data[:bytesRead]
	
	// Store in prefetch cache
	metadata := make(map[string]string)
	if result.ContentType != nil {
		metadata["content-type"] = *result.ContentType
	}
	if result.ContentLength != nil {
		metadata["content-length"] = fmt.Sprintf("%d", *result.ContentLength)
	}
	
	err = pw.prefetcher.prefetchCache.Put(job.Key, actualData, metadata)
	if err != nil {
		return false, int64(bytesRead), fmt.Errorf("failed to cache object: %w", err)
	}
	
	return true, int64(bytesRead), nil
}

// NewNetworkOptimizer creates a new network optimizer.
func NewNetworkOptimizer(config *PrefetchConfig) *NetworkOptimizer {
	no := &NetworkOptimizer{
		config:             config,
		conditionHistory:   make([]*NetworkCondition, 0),
		adaptationEngine:   NewNetworkAdaptationEngine(config),
		bandwidthEstimator: NewBandwidthEstimator(),
	}
	
	no.initializeAdaptationRules()
	return no
}

// UpdateConditions updates network conditions.
func (no *NetworkOptimizer) UpdateConditions(conditions *NetworkConditions) {
	no.mu.Lock()
	defer no.mu.Unlock()
	
	// Convert to internal format
	condition := &NetworkCondition{
		Bandwidth:    conditions.Bandwidth,
		Latency:      conditions.RTT,
		PacketLoss:   conditions.PacketLoss,
		Jitter:       conditions.Jitter,
		Reliability:  1.0 - conditions.PacketLoss/100.0, // Convert packet loss to reliability
		Timestamp:    conditions.LastUpdated,
		QualityScore: no.calculateQualityScore(conditions),
	}
	
	no.currentCondition = condition
	no.addToHistory(condition)
	
	// Update bandwidth estimator
	no.bandwidthEstimator.AddMeasurement(conditions.Bandwidth, conditions.LastUpdated)
}

// OptimizeJob optimizes a prefetch job based on network conditions.
func (no *NetworkOptimizer) OptimizeJob(job *PrefetchJob) *JobOptimization {
	no.mu.RLock()
	defer no.mu.RUnlock()
	
	if no.currentCondition == nil {
		return no.getDefaultOptimization()
	}
	
	return no.adaptationEngine.OptimizeJob(job, no.currentCondition)
}

// GetCurrentCondition returns the current network condition.
func (no *NetworkOptimizer) GetCurrentCondition() *NetworkCondition {
	no.mu.RLock()
	defer no.mu.RUnlock()
	
	if no.currentCondition == nil {
		return &NetworkCondition{
			Bandwidth:    50.0,
			Latency:      time.Millisecond * 20,
			PacketLoss:   0.001,
			Reliability:  0.9,
			QualityScore: 0.8,
			Timestamp:    time.Now(),
		}
	}
	
	return no.currentCondition
}

// GetUtilization returns network utilization.
func (no *NetworkOptimizer) GetUtilization() float64 {
	no.mu.RLock()
	defer no.mu.RUnlock()
	
	if no.currentCondition == nil {
		return 0.5
	}
	
	// Calculate utilization based on bandwidth and quality
	utilization := no.currentCondition.Bandwidth / 100.0 // Normalize to 100 Mbps
	if utilization > 1.0 {
		utilization = 1.0
	}
	
	// Adjust for quality
	utilization *= no.currentCondition.QualityScore
	
	return utilization
}

// initializeAdaptationRules initializes network adaptation rules.
func (no *NetworkOptimizer) initializeAdaptationRules() {
	// High bandwidth rule
	no.adaptationEngine.AddRule(&AdaptationRule{
		Name: "HighBandwidth",
		Condition: func(nc *NetworkCondition) bool {
			return nc.Bandwidth > 100.0
		},
		Action: func(job *PrefetchJob) *JobOptimization {
			return &JobOptimization{
				ChunkSize:     1024 * 1024 * 8, // 8MB chunks
				Concurrency:   4,
				TimeoutAdjust: time.Second * 30,
			}
		},
		Priority: 0.8,
		Enabled:  true,
	})
	
	// Low bandwidth rule
	no.adaptationEngine.AddRule(&AdaptationRule{
		Name: "LowBandwidth",
		Condition: func(nc *NetworkCondition) bool {
			return nc.Bandwidth < 10.0
		},
		Action: func(job *PrefetchJob) *JobOptimization {
			return &JobOptimization{
				ChunkSize:     1024 * 1024 / 2, // 512KB chunks
				Concurrency:   1,
				TimeoutAdjust: time.Minute * 2,
			}
		},
		Priority: 0.9,
		Enabled:  true,
	})
	
	// High latency rule
	no.adaptationEngine.AddRule(&AdaptationRule{
		Name: "HighLatency",
		Condition: func(nc *NetworkCondition) bool {
			return nc.Latency > time.Millisecond*200
		},
		Action: func(job *PrefetchJob) *JobOptimization {
			return &JobOptimization{
				ChunkSize:     1024 * 1024 * 4, // 4MB chunks
				Concurrency:   2,
				RetryStrategy: "exponential",
				TimeoutAdjust: time.Minute * 3,
			}
		},
		Priority: 0.7,
		Enabled:  true,
	})
}

// calculateQualityScore calculates a network quality score.
func (no *NetworkOptimizer) calculateQualityScore(conditions *NetworkConditions) float64 {
	score := 1.0
	
	// Bandwidth factor (normalized to 100 Mbps)
	bandwidthScore := conditions.Bandwidth / 100.0
	if bandwidthScore > 1.0 {
		bandwidthScore = 1.0
	}
	score *= bandwidthScore
	
	// Latency factor (penalty for high latency)
	latencyPenalty := float64(conditions.RTT.Milliseconds()) / 1000.0
	if latencyPenalty > 1.0 {
		latencyPenalty = 1.0
	}
	score *= (1.0 - latencyPenalty)
	
	// Packet loss penalty
	score *= (1.0 - conditions.PacketLoss/100.0)
	
	// Ensure score is in valid range
	if score < 0.0 {
		score = 0.0
	} else if score > 1.0 {
		score = 1.0
	}
	
	return score
}

// addToHistory adds a condition to the history.
func (no *NetworkOptimizer) addToHistory(condition *NetworkCondition) {
	no.conditionHistory = append(no.conditionHistory, condition)
	
	// Limit history size
	maxHistory := 100
	if len(no.conditionHistory) > maxHistory {
		no.conditionHistory = no.conditionHistory[len(no.conditionHistory)-maxHistory:]
	}
}

// getDefaultOptimization returns default optimization settings.
func (no *NetworkOptimizer) getDefaultOptimization() *JobOptimization {
	return &JobOptimization{
		ChunkSize:     1024 * 1024 * 2, // 2MB chunks
		Concurrency:   2,
		RetryStrategy: "linear",
		TimeoutAdjust: time.Minute,
	}
}

// NewNetworkAdaptationEngine creates a new network adaptation engine.
func NewNetworkAdaptationEngine(config *PrefetchConfig) *NetworkAdaptationEngine {
	return &NetworkAdaptationEngine{
		adaptationRules: make([]*AdaptationRule, 0),
		learningRate:    config.LearningRate,
		history:         make([]*AdaptationResult, 0),
	}
}

// AddRule adds an adaptation rule.
func (nae *NetworkAdaptationEngine) AddRule(rule *AdaptationRule) {
	nae.adaptationRules = append(nae.adaptationRules, rule)
}

// OptimizeJob optimizes a job based on network conditions.
func (nae *NetworkAdaptationEngine) OptimizeJob(job *PrefetchJob, condition *NetworkCondition) *JobOptimization {
	// Find applicable rules
	var applicableRules []*AdaptationRule
	for _, rule := range nae.adaptationRules {
		if rule.Enabled && rule.Condition(condition) {
			applicableRules = append(applicableRules, rule)
		}
	}
	
	if len(applicableRules) == 0 {
		// Return default optimization
		return &JobOptimization{
			ChunkSize:     1024 * 1024 * 2,
			Concurrency:   2,
			TimeoutAdjust: time.Minute,
		}
	}
	
	// Sort rules by priority
	for i := 0; i < len(applicableRules)-1; i++ {
		for j := i + 1; j < len(applicableRules); j++ {
			if applicableRules[i].Priority < applicableRules[j].Priority {
				applicableRules[i], applicableRules[j] = applicableRules[j], applicableRules[i]
			}
		}
	}
	
	// Apply highest priority rule
	optimization := applicableRules[0].Action(job)
	
	// Record adaptation result for learning
	result := &AdaptationResult{
		Rule:      applicableRules[0],
		Job:       job,
		Timestamp: time.Now(),
	}
	nae.history = append(nae.history, result)
	
	return optimization
}

// NewBandwidthEstimator creates a new bandwidth estimator.
func NewBandwidthEstimator() *BandwidthEstimator {
	return &BandwidthEstimator{
		measurements: make([]BandwidthMeasurement, 0),
		windowSize:   20, // Keep last 20 measurements
	}
}

// AddMeasurement adds a bandwidth measurement.
func (be *BandwidthEstimator) AddMeasurement(bandwidth float64, timestamp time.Time) {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	measurement := BandwidthMeasurement{
		Bandwidth: bandwidth,
		Timestamp: timestamp,
		Accuracy:  0.8, // Default accuracy
	}
	
	be.measurements = append(be.measurements, measurement)
	
	// Limit size
	if len(be.measurements) > be.windowSize {
		be.measurements = be.measurements[len(be.measurements)-be.windowSize:]
	}
}

// EstimateBandwidth estimates current bandwidth.
func (be *BandwidthEstimator) EstimateBandwidth() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	
	if len(be.measurements) == 0 {
		return 50.0 // Default bandwidth
	}
	
	// Weighted average with recent measurements having higher weight
	total := 0.0
	weight := 0.0
	
	for i, measurement := range be.measurements {
		// Recent measurements get higher weight
		w := float64(i+1) / float64(len(be.measurements))
		total += measurement.Bandwidth * w
		weight += w
	}
	
	return total / weight
}

// WorkerStats contains worker performance statistics.
type WorkerStats struct {
	ID            int           `json:"id"`
	JobsProcessed int64         `json:"jobs_processed"`
	TotalBytes    int64         `json:"total_bytes"`
	TotalDuration time.Duration `json:"total_duration"`
	SuccessCount  int64         `json:"success_count"`
	ErrorCount    int64         `json:"error_count"`
	IsRunning     bool          `json:"is_running"`
}