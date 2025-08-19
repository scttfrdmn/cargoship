package s3optimization

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// PredictivePrefetcher implements intelligent prefetching based on access patterns.
type PredictivePrefetcher struct {
	optimizer          *S3Optimizer
	patternAnalyzer    *AccessPatternAnalyzer
	prefetchCache      *PrefetchCache
	requestPredictor   *RequestPredictor
	adaptiveScheduler  *AdaptiveScheduler
	networkOptimizer   *NetworkOptimizer
	config            *PrefetchConfig
	logger            *slog.Logger
	
	// State management
	isRunning         bool
	cancelFunc        context.CancelFunc
	prefetchWorkers   []*PrefetchWorker
	mu                sync.RWMutex
	
	// Performance tracking
	metrics           *PrefetchMetrics
	lastOptimization  time.Time
}

// PrefetchConfig configures the predictive prefetching system.
type PrefetchConfig struct {
	// Prefetch behavior
	EnablePrefetching     bool          `yaml:"enable_prefetching" json:"enable_prefetching"`
	MaxConcurrentPrefetch int           `yaml:"max_concurrent_prefetch" json:"max_concurrent_prefetch"`
	PrefetchWindowSize    int           `yaml:"prefetch_window_size" json:"prefetch_window_size"`
	PrefetchAheadTime     time.Duration `yaml:"prefetch_ahead_time" json:"prefetch_ahead_time"`
	
	// Pattern analysis
	AnalysisWindow        time.Duration `yaml:"analysis_window" json:"analysis_window"`
	MinPatternConfidence  float64       `yaml:"min_pattern_confidence" json:"min_pattern_confidence"`
	PatternUpdateInterval time.Duration `yaml:"pattern_update_interval" json:"pattern_update_interval"`
	
	// Cache management
	CacheSize             int64         `yaml:"cache_size" json:"cache_size"`
	CacheTTL              time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
	EvictionPolicy        string        `yaml:"eviction_policy" json:"eviction_policy"`
	
	// Adaptive optimization
	EnableAdaptiveScheduling bool    `yaml:"enable_adaptive_scheduling" json:"enable_adaptive_scheduling"`
	LearningRate            float64 `yaml:"learning_rate" json:"learning_rate"`
	OptimizationInterval    time.Duration `yaml:"optimization_interval" json:"optimization_interval"`
	
	// Network optimization
	BandwidthThreshold     float64 `yaml:"bandwidth_threshold" json:"bandwidth_threshold"`
	LatencyThreshold       time.Duration `yaml:"latency_threshold" json:"latency_threshold"`
	NetworkAdaptationRate  float64 `yaml:"network_adaptation_rate" json:"network_adaptation_rate"`
}

// DefaultPrefetchConfig returns sensible defaults for predictive prefetching.
func DefaultPrefetchConfig() *PrefetchConfig {
	return &PrefetchConfig{
		EnablePrefetching:         true,
		MaxConcurrentPrefetch:     5,
		PrefetchWindowSize:        20,
		PrefetchAheadTime:         time.Minute * 2,
		AnalysisWindow:            time.Hour * 6,
		MinPatternConfidence:      0.7,
		PatternUpdateInterval:     time.Minute * 5,
		CacheSize:                 1024 * 1024 * 1024, // 1GB
		CacheTTL:                  time.Hour * 2,
		EvictionPolicy:            "lru",
		EnableAdaptiveScheduling:  true,
		LearningRate:             0.1,
		OptimizationInterval:     time.Minute * 10,
		BandwidthThreshold:       50.0, // 50 Mbps
		LatencyThreshold:         time.Millisecond * 100,
		NetworkAdaptationRate:    0.2,
	}
}

// NewPredictivePrefetcher creates a new predictive prefetcher.
func NewPredictivePrefetcher(optimizer *S3Optimizer, config *PrefetchConfig, logger *slog.Logger) (*PredictivePrefetcher, error) {
	if optimizer == nil {
		return nil, fmt.Errorf("S3 optimizer cannot be nil")
	}
	
	if config == nil {
		config = DefaultPrefetchConfig()
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	prefetcher := &PredictivePrefetcher{
		optimizer:         optimizer,
		patternAnalyzer:   NewAccessPatternAnalyzer(config),
		prefetchCache:     NewPrefetchCache(config),
		requestPredictor:  NewRequestPredictor(config),
		adaptiveScheduler: NewAdaptiveScheduler(config),
		networkOptimizer:  NewNetworkOptimizer(config),
		config:           config,
		logger:           logger,
		metrics:          NewPrefetchMetrics(),
		prefetchWorkers:  make([]*PrefetchWorker, config.MaxConcurrentPrefetch),
	}
	
	// Initialize prefetch workers
	for i := 0; i < config.MaxConcurrentPrefetch; i++ {
		prefetcher.prefetchWorkers[i] = NewPrefetchWorker(i, prefetcher, logger)
	}
	
	return prefetcher, nil
}

// Start begins predictive prefetching operations.
func (pp *PredictivePrefetcher) Start(ctx context.Context) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	
	if pp.isRunning {
		return fmt.Errorf("predictive prefetcher is already running")
	}
	
	// Create cancellable context
	prefetchCtx, cancel := context.WithCancel(ctx)
	pp.cancelFunc = cancel
	
	// Start background services
	go pp.patternAnalysisLoop(prefetchCtx)
	go pp.adaptiveOptimizationLoop(prefetchCtx)
	go pp.networkMonitoringLoop(prefetchCtx)
	
	// Start prefetch workers
	for _, worker := range pp.prefetchWorkers {
		go worker.Start(prefetchCtx)
	}
	
	pp.isRunning = true
	pp.logger.Info("predictive prefetcher started successfully",
		"max_concurrent", pp.config.MaxConcurrentPrefetch,
		"window_size", pp.config.PrefetchWindowSize,
		"ahead_time", pp.config.PrefetchAheadTime)
	
	return nil
}

// Stop gracefully stops predictive prefetching.
func (pp *PredictivePrefetcher) Stop() error {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	
	if !pp.isRunning {
		return nil
	}
	
	// Cancel all operations
	if pp.cancelFunc != nil {
		pp.cancelFunc()
	}
	
	// Wait for workers to finish
	var wg sync.WaitGroup
	for _, worker := range pp.prefetchWorkers {
		wg.Add(1)
		go func(w *PrefetchWorker) {
			defer wg.Done()
			w.Stop()
		}(worker)
	}
	wg.Wait()
	
	pp.isRunning = false
	pp.logger.Info("predictive prefetcher stopped")
	
	return nil
}

// PredictAndPrefetch analyzes access patterns and triggers prefetching.
func (pp *PredictivePrefetcher) PredictAndPrefetch(ctx context.Context, requestKey string) error {
	if !pp.isRunning || !pp.config.EnablePrefetching {
		return nil
	}
	
	// Record the access for pattern analysis
	pp.patternAnalyzer.RecordAccess(requestKey, time.Now())
	
	// Get predictions based on current access
	predictions := pp.requestPredictor.PredictNextRequests(requestKey, pp.config.PrefetchWindowSize)
	if len(predictions) == 0 {
		return nil
	}
	
	// Filter predictions by confidence threshold
	filteredPredictions := make([]*RequestPrediction, 0, len(predictions))
	for _, prediction := range predictions {
		if prediction.Confidence >= pp.config.MinPatternConfidence {
			filteredPredictions = append(filteredPredictions, prediction)
		}
	}
	
	if len(filteredPredictions) == 0 {
		return nil
	}
	
	// Schedule prefetch operations
	return pp.schedulePrefetch(ctx, filteredPredictions)
}

// GetCachedObject retrieves an object from the prefetch cache.
func (pp *PredictivePrefetcher) GetCachedObject(key string) (*CachedObject, bool) {
	if !pp.isRunning {
		return nil, false
	}
	
	obj, found := pp.prefetchCache.Get(key)
	if found {
		pp.metrics.RecordCacheHit()
		pp.logger.Debug("prefetch cache hit", "key", key)
	} else {
		pp.metrics.RecordCacheMiss()
	}
	
	return obj, found
}

// GetPrefetchMetrics returns current prefetch metrics.
func (pp *PredictivePrefetcher) GetPrefetchMetrics() *PrefetchMetrics {
	return pp.metrics.GetSnapshot()
}

// UpdateNetworkConditions updates network conditions for optimization.
func (pp *PredictivePrefetcher) UpdateNetworkConditions(conditions *NetworkConditions) {
	if !pp.isRunning {
		return
	}
	
	pp.networkOptimizer.UpdateConditions(conditions)
	
	// Adjust prefetch strategy based on network conditions
	pp.adaptPrefetchStrategy(conditions)
}

// schedulePrefetch schedules prefetch operations for predicted requests.
func (pp *PredictivePrefetcher) schedulePrefetch(ctx context.Context, predictions []*RequestPrediction) error {
	// Sort predictions by confidence and predicted time
	sort.Slice(predictions, func(i, j int) bool {
		if predictions[i].Confidence != predictions[j].Confidence {
			return predictions[i].Confidence > predictions[j].Confidence
		}
		return predictions[i].PredictedTime.Before(predictions[j].PredictedTime)
	})
	
	// Schedule with adaptive scheduler
	jobs := make([]*PrefetchJob, 0, len(predictions))
	for _, prediction := range predictions {
		// Check if already cached
		if _, cached := pp.prefetchCache.Get(prediction.Key); cached {
			continue
		}
		
		// Check if already scheduled
		if pp.adaptiveScheduler.IsScheduled(prediction.Key) {
			continue
		}
		
		job := &PrefetchJob{
			Key:            prediction.Key,
			Bucket:         prediction.Bucket,
			Priority:       pp.calculatePriority(prediction),
			ScheduledTime:  time.Now(),
			PredictedTime:  prediction.PredictedTime,
			Confidence:     prediction.Confidence,
			EstimatedSize:  prediction.EstimatedSize,
		}
		
		jobs = append(jobs, job)
	}
	
	if len(jobs) == 0 {
		return nil
	}
	
	// Schedule jobs with adaptive scheduler
	return pp.adaptiveScheduler.ScheduleJobs(ctx, jobs)
}

// calculatePriority calculates the priority for a prefetch job.
func (pp *PredictivePrefetcher) calculatePriority(prediction *RequestPrediction) float64 {
	// Base priority from confidence
	priority := prediction.Confidence
	
	// Adjust based on predicted access time (sooner = higher priority)
	timeToAccess := time.Until(prediction.PredictedTime)
	if timeToAccess <= pp.config.PrefetchAheadTime {
		priority *= 1.5 // Increase priority for imminent accesses
	}
	
	// Adjust based on access frequency
	frequency := pp.patternAnalyzer.GetAccessFrequency(prediction.Key)
	if frequency > 0 {
		priority *= (1.0 + frequency*0.5) // Higher frequency = higher priority
	}
	
	// Network condition adjustment
	networkCondition := pp.networkOptimizer.GetCurrentCondition()
	if networkCondition.Bandwidth < pp.config.BandwidthThreshold {
		priority *= 0.8 // Lower priority in poor network conditions
	}
	
	return priority
}

// adaptPrefetchStrategy adapts the prefetch strategy based on network conditions.
func (pp *PredictivePrefetcher) adaptPrefetchStrategy(conditions *NetworkConditions) {
	// Adjust prefetch window size based on bandwidth
	if conditions.Bandwidth < pp.config.BandwidthThreshold {
		// Reduce prefetch window in poor network conditions
		pp.adaptiveScheduler.SetWindowSizeMultiplier(0.5)
	} else if conditions.Bandwidth > pp.config.BandwidthThreshold*2 {
		// Increase prefetch window in good network conditions
		pp.adaptiveScheduler.SetWindowSizeMultiplier(1.5)
	} else {
		pp.adaptiveScheduler.SetWindowSizeMultiplier(1.0)
	}
	
	// Adjust prefetch timing based on latency
	if conditions.RTT > pp.config.LatencyThreshold {
		// Start prefetching earlier in high-latency conditions
		pp.adaptiveScheduler.SetTimingMultiplier(1.3)
	} else {
		pp.adaptiveScheduler.SetTimingMultiplier(1.0)
	}
	
	pp.logger.Debug("adapted prefetch strategy",
		"bandwidth", conditions.Bandwidth,
		"rtt", conditions.RTT,
		"window_multiplier", pp.adaptiveScheduler.GetWindowSizeMultiplier(),
		"timing_multiplier", pp.adaptiveScheduler.GetTimingMultiplier())
}

// patternAnalysisLoop runs pattern analysis in the background.
func (pp *PredictivePrefetcher) patternAnalysisLoop(ctx context.Context) {
	ticker := time.NewTicker(pp.config.PatternUpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.updatePatterns()
		}
	}
}

// adaptiveOptimizationLoop runs adaptive optimization in the background.
func (pp *PredictivePrefetcher) adaptiveOptimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(pp.config.OptimizationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.performAdaptiveOptimization()
		}
	}
}

// networkMonitoringLoop monitors network conditions.
func (pp *PredictivePrefetcher) networkMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 10) // Monitor every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.monitorNetworkConditions()
		}
	}
}

// updatePatterns updates access patterns and predictions.
func (pp *PredictivePrefetcher) updatePatterns() {
	// Update pattern analyzer
	pp.patternAnalyzer.UpdatePatterns()
	
	// Update request predictor with new patterns
	patterns := pp.patternAnalyzer.GetPatterns()
	pp.requestPredictor.UpdatePatterns(patterns)
	
	// Log pattern update
	patternCount := len(patterns)
	if patternCount > 0 {
		pp.logger.Debug("updated access patterns", "pattern_count", patternCount)
	}
}

// performAdaptiveOptimization performs adaptive optimization.
func (pp *PredictivePrefetcher) performAdaptiveOptimization() {
	if !pp.config.EnableAdaptiveScheduling {
		return
	}
	
	// Get current metrics
	metrics := pp.metrics.GetSnapshot()
	
	// Calculate optimization parameters
	cacheHitRate := metrics.CacheHitRate
	networkUtilization := pp.networkOptimizer.GetUtilization()
	
	// Adapt cache size based on hit rate
	if cacheHitRate > 0.8 && networkUtilization < 0.7 {
		// Increase cache size for high hit rate and low network utilization
		pp.prefetchCache.AdaptSize(1.1)
	} else if cacheHitRate < 0.5 {
		// Decrease cache size for low hit rate
		pp.prefetchCache.AdaptSize(0.9)
	}
	
	// Adapt prefetch aggressiveness
	if metrics.PrefetchAccuracy > 0.8 {
		// Increase prefetch window for high accuracy
		pp.adaptiveScheduler.AdaptAggressiveness(1.1)
	} else if metrics.PrefetchAccuracy < 0.6 {
		// Decrease prefetch window for low accuracy
		pp.adaptiveScheduler.AdaptAggressiveness(0.9)
	}
	
	pp.lastOptimization = time.Now()
	pp.logger.Debug("performed adaptive optimization",
		"cache_hit_rate", cacheHitRate,
		"prefetch_accuracy", metrics.PrefetchAccuracy,
		"network_utilization", networkUtilization)
}

// monitorNetworkConditions monitors and updates network conditions.
func (pp *PredictivePrefetcher) monitorNetworkConditions() {
	// Get current network conditions from optimizer
	optimizerMetrics := pp.optimizer.GetPerformanceMetrics()
	
	// Create network conditions from metrics
	conditions := &NetworkConditions{
		Bandwidth:   optimizerMetrics.ThroughputMbps,
		RTT:         optimizerMetrics.AverageLatency,
		PacketLoss:  calculatePacketLoss(optimizerMetrics),
		Congestion:  calculateCongestion(optimizerMetrics),
		Jitter:      optimizerMetrics.AverageLatency / 10, // Estimate jitter as 10% of RTT
		LastUpdated: time.Now(),
	}
	
	// Update network optimizer
	pp.networkOptimizer.UpdateConditions(conditions)
	
	// Record network metrics
	pp.metrics.RecordNetworkConditions(conditions)
}

// calculatePacketLoss estimates packet loss from metrics.
func calculatePacketLoss(metrics *PerformanceMetrics) float64 {
	if metrics.TotalRequests == 0 {
		return 0
	}
	return float64(metrics.FailedRequests) / float64(metrics.TotalRequests) * 100
}

// calculateCongestion estimates network congestion from metrics.
func calculateCongestion(metrics *PerformanceMetrics) float64 {
	// Estimate congestion based on latency and throughput
	latencyMs := float64(metrics.AverageLatency.Milliseconds())
	
	// Higher latency = higher congestion
	congestion := latencyMs / 1000.0 // Normalize to 0-1 range
	if congestion > 1.0 {
		congestion = 1.0
	}
	
	return congestion * 100 // Convert to percentage
}

// PrefetchMetrics tracks prefetch performance metrics.
type PrefetchMetrics struct {
	TotalPrefetches      int64     `json:"total_prefetches"`
	SuccessfulPrefetches int64     `json:"successful_prefetches"`
	FailedPrefetches     int64     `json:"failed_prefetches"`
	CacheHits           int64     `json:"cache_hits"`
	CacheMisses         int64     `json:"cache_misses"`
	TotalBytesPrefeched int64     `json:"total_bytes_prefetched"`
	AvgPrefetchTime     time.Duration `json:"avg_prefetch_time"`
	
	// Accuracy metrics
	PrefetchAccuracy    float64   `json:"prefetch_accuracy"`    // % of prefetches that were used
	CacheHitRate       float64   `json:"cache_hit_rate"`       // % of requests served from cache
	NetworkSavings     float64   `json:"network_savings"`      // % bandwidth saved
	
	// Network conditions
	LastNetworkUpdate   time.Time `json:"last_network_update"`
	AverageBandwidth   float64   `json:"average_bandwidth"`
	AverageLatency     time.Duration `json:"average_latency"`
	
	mu                 sync.RWMutex
	startTime          time.Time
}

// NewPrefetchMetrics creates new prefetch metrics.
func NewPrefetchMetrics() *PrefetchMetrics {
	return &PrefetchMetrics{
		startTime: time.Now(),
	}
}

// RecordPrefetch records a prefetch operation.
func (pm *PrefetchMetrics) RecordPrefetch(success bool, bytes int64, duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.TotalPrefetches++
	pm.TotalBytesPrefeched += bytes
	
	if success {
		pm.SuccessfulPrefetches++
	} else {
		pm.FailedPrefetches++
	}
	
	// Update average prefetch time
	if pm.TotalPrefetches > 0 {
		pm.AvgPrefetchTime = time.Duration(
			(int64(pm.AvgPrefetchTime)*pm.TotalPrefetches + int64(duration)) / (pm.TotalPrefetches + 1))
	}
}

// RecordCacheHit records a cache hit.
func (pm *PrefetchMetrics) RecordCacheHit() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.CacheHits++
	pm.updateCacheHitRate()
}

// RecordCacheMiss records a cache miss.
func (pm *PrefetchMetrics) RecordCacheMiss() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.CacheMisses++
	pm.updateCacheHitRate()
}

// RecordNetworkConditions records network conditions.
func (pm *PrefetchMetrics) RecordNetworkConditions(conditions *NetworkConditions) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.LastNetworkUpdate = conditions.LastUpdated
	
	// Update running averages
	alpha := 0.1 // Exponential moving average factor
	pm.AverageBandwidth = alpha*conditions.Bandwidth + (1-alpha)*pm.AverageBandwidth
	
	newLatency := time.Duration(alpha*float64(conditions.RTT) + (1-alpha)*float64(pm.AverageLatency))
	pm.AverageLatency = newLatency
}

// updateCacheHitRate updates the cache hit rate.
func (pm *PrefetchMetrics) updateCacheHitRate() {
	totalCacheRequests := pm.CacheHits + pm.CacheMisses
	if totalCacheRequests > 0 {
		pm.CacheHitRate = float64(pm.CacheHits) / float64(totalCacheRequests)
	}
}

// GetSnapshot returns a snapshot of current metrics.
func (pm *PrefetchMetrics) GetSnapshot() *PrefetchMetrics {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	// Create a copy without the mutex
	return &PrefetchMetrics{
		TotalPrefetches:      pm.TotalPrefetches,
		SuccessfulPrefetches: pm.SuccessfulPrefetches,
		FailedPrefetches:     pm.FailedPrefetches,
		CacheHits:           pm.CacheHits,
		CacheMisses:         pm.CacheMisses,
		TotalBytesPrefeched: pm.TotalBytesPrefeched,
		AvgPrefetchTime:     pm.AvgPrefetchTime,
		PrefetchAccuracy:    pm.PrefetchAccuracy,
		CacheHitRate:       pm.CacheHitRate,
		NetworkSavings:     pm.NetworkSavings,
		LastNetworkUpdate:   pm.LastNetworkUpdate,
		AverageBandwidth:   pm.AverageBandwidth,
		AverageLatency:     pm.AverageLatency,
		startTime:          pm.startTime,
	}
}