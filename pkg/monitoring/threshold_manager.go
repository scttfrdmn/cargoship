package monitoring

import (
	"context"
	"math"
	"sync"
	"time"
)

// ThresholdManager manages dynamic performance thresholds.
type ThresholdManager struct {
	config            *MonitoringConfig
	currentThresholds *DynamicThresholds
	baselineMetrics   *BaselineMetrics
	adaptationEngine  *ThresholdAdaptationEngine
	mu                sync.RWMutex
	isRunning         bool
}

// NewThresholdManager creates a new threshold manager.
func NewThresholdManager(config *MonitoringConfig) *ThresholdManager {
	tm := &ThresholdManager{
		config:           config,
		currentThresholds: NewDynamicThresholds(config.DefaultThresholds),
		baselineMetrics:   NewBaselineMetrics(),
		adaptationEngine:  NewThresholdAdaptationEngine(),
	}
	
	return tm
}

// Start begins threshold management.
func (tm *ThresholdManager) Start(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.isRunning {
		return nil
	}
	
	tm.isRunning = true
	go tm.adaptationLoop(ctx)
	return nil
}

// Stop stops threshold management.
func (tm *ThresholdManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.isRunning = false
}

// GetCurrentThresholds returns the current dynamic thresholds.
func (tm *ThresholdManager) GetCurrentThresholds() *DynamicThresholds {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// Return a copy
	thresholds := *tm.currentThresholds
	return &thresholds
}

// SetThreshold manually sets a specific threshold.
func (tm *ThresholdManager) SetThreshold(metric string, threshold *AlertThreshold) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	switch metric {
	case "throughput":
		tm.currentThresholds.MinThroughputMBps = threshold.Value
	case "latency":
		tm.currentThresholds.MaxLatencyMs = threshold.Value
	case "error_rate":
		tm.currentThresholds.MaxErrorRate = threshold.Value
	case "cpu_usage":
		tm.currentThresholds.MaxCPUUsage = threshold.Value
	case "memory_usage":
		tm.currentThresholds.MaxMemoryUsage = threshold.Value
	case "packet_loss":
		tm.currentThresholds.MaxPacketLoss = threshold.Value
	case "s3_error_rate":
		tm.currentThresholds.MaxS3ErrorRate = threshold.Value
	}
	
	return nil
}

// UpdateBaseline updates the performance baseline with new metrics.
func (tm *ThresholdManager) UpdateBaseline(metrics *PerformanceMetrics) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.baselineMetrics.UpdateWithMetrics(metrics)
	
	// Trigger threshold adaptation based on new baseline
	tm.adaptThresholds()
}

// adaptationLoop runs the threshold adaptation process.
func (tm *ThresholdManager) adaptationLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 5) // Adapt every 5 minutes
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.mu.Lock()
			tm.adaptThresholds()
			tm.mu.Unlock()
		}
	}
}

// adaptThresholds adapts thresholds based on current baseline and trends.
func (tm *ThresholdManager) adaptThresholds() {
	if !tm.baselineMetrics.HasSufficientData() {
		return
	}
	
	adaptation := tm.adaptationEngine.CalculateAdaptation(tm.baselineMetrics, tm.currentThresholds)
	
	// Apply adaptations with safety bounds
	tm.currentThresholds.MinThroughputMBps = tm.applySafeBounds(
		tm.currentThresholds.MinThroughputMBps * adaptation.ThroughputMultiplier,
		tm.config.DefaultThresholds.MinThroughputMBps * 0.1, // 10% of default minimum
		tm.config.DefaultThresholds.MinThroughputMBps * 2.0, // 200% of default maximum
	)
	
	tm.currentThresholds.MaxLatencyMs = tm.applySafeBounds(
		tm.currentThresholds.MaxLatencyMs * adaptation.LatencyMultiplier,
		tm.config.DefaultThresholds.MaxLatencyMs * 0.5, // 50% of default minimum
		tm.config.DefaultThresholds.MaxLatencyMs * 3.0, // 300% of default maximum
	)
	
	tm.currentThresholds.MaxErrorRate = tm.applySafeBounds(
		tm.currentThresholds.MaxErrorRate * adaptation.ErrorRateMultiplier,
		0.001, // Minimum 0.1% error rate
		0.2,   // Maximum 20% error rate
	)
	
	tm.currentThresholds.LastAdaptation = time.Now()
}

// applySafeBounds ensures threshold values stay within safe bounds.
func (tm *ThresholdManager) applySafeBounds(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// DynamicThresholds contains dynamically adapted alert thresholds.
type DynamicThresholds struct {
	MinThroughputMBps float64   `json:"min_throughput_mbps"`
	MaxLatencyMs      float64   `json:"max_latency_ms"`
	MaxErrorRate      float64   `json:"max_error_rate"`
	MaxCPUUsage       float64   `json:"max_cpu_usage"`
	MaxMemoryUsage    float64   `json:"max_memory_usage"`
	MaxDiskUsage      float64   `json:"max_disk_usage"`
	MaxPacketLoss     float64   `json:"max_packet_loss"`
	MaxS3ErrorRate    float64   `json:"max_s3_error_rate"`
	LastAdaptation    time.Time `json:"last_adaptation"`
	AdaptationCount   int       `json:"adaptation_count"`
}

// NewDynamicThresholds creates dynamic thresholds from default thresholds.
func NewDynamicThresholds(defaults *DefaultThresholds) *DynamicThresholds {
	return &DynamicThresholds{
		MinThroughputMBps: defaults.MinThroughputMBps,
		MaxLatencyMs:      defaults.MaxLatencyMs,
		MaxErrorRate:      defaults.MaxErrorRate,
		MaxCPUUsage:       defaults.MaxCPUUsage,
		MaxMemoryUsage:    defaults.MaxMemoryUsage,
		MaxDiskUsage:      defaults.MaxDiskUsage,
		MaxPacketLoss:     defaults.MaxPacketLoss,
		MaxS3ErrorRate:    float64(defaults.MaxS3Errors) / 100.0, // Convert count to rate
		LastAdaptation:    time.Now(),
		AdaptationCount:   0,
	}
}

// BaselineMetrics tracks baseline performance metrics for threshold adaptation.
type BaselineMetrics struct {
	ThroughputHistory    []float64  `json:"throughput_history"`
	LatencyHistory       []float64  `json:"latency_history"`
	ErrorRateHistory     []float64  `json:"error_rate_history"`
	CPUUsageHistory      []float64  `json:"cpu_usage_history"`
	MemoryUsageHistory   []float64  `json:"memory_usage_history"`
	
	// Statistical measures
	AvgThroughput        float64    `json:"avg_throughput"`
	AvgLatency           float64    `json:"avg_latency"`
	AvgErrorRate         float64    `json:"avg_error_rate"`
	AvgCPUUsage          float64    `json:"avg_cpu_usage"`
	AvgMemoryUsage       float64    `json:"avg_memory_usage"`
	
	// Variability measures
	ThroughputStdDev     float64    `json:"throughput_std_dev"`
	LatencyStdDev        float64    `json:"latency_std_dev"`
	ErrorRateStdDev      float64    `json:"error_rate_std_dev"`
	
	LastUpdated          time.Time  `json:"last_updated"`
	SampleCount          int        `json:"sample_count"`
	
	maxHistory           int
}

// NewBaselineMetrics creates new baseline metrics tracker.
func NewBaselineMetrics() *BaselineMetrics {
	return &BaselineMetrics{
		ThroughputHistory:  make([]float64, 0),
		LatencyHistory:     make([]float64, 0),
		ErrorRateHistory:   make([]float64, 0),
		CPUUsageHistory:    make([]float64, 0),
		MemoryUsageHistory: make([]float64, 0),
		maxHistory:         1000, // Keep last 1000 samples
	}
}

// UpdateWithMetrics updates baseline metrics with new performance data.
func (bm *BaselineMetrics) UpdateWithMetrics(metrics *PerformanceMetrics) {
	// Update throughput
	if metrics.TransferMetrics != nil {
		bm.addToHistory(&bm.ThroughputHistory, metrics.TransferMetrics.TotalThroughputMBps)
		bm.addToHistory(&bm.LatencyHistory, metrics.TransferMetrics.AverageLatencyMs)
		bm.addToHistory(&bm.ErrorRateHistory, 1.0-metrics.TransferMetrics.SuccessRate)
	}
	
	// Update system metrics
	if metrics.SystemMetrics != nil {
		bm.addToHistory(&bm.CPUUsageHistory, metrics.SystemMetrics.CPUUsagePercent/100.0)
		bm.addToHistory(&bm.MemoryUsageHistory, metrics.SystemMetrics.MemoryUsagePercent/100.0)
	}
	
	// Recalculate statistics
	bm.calculateStatistics()
	bm.LastUpdated = time.Now()
	bm.SampleCount++
}

// addToHistory adds a value to a history slice with size management.
func (bm *BaselineMetrics) addToHistory(history *[]float64, value float64) {
	*history = append(*history, value)
	
	// Limit history size
	if len(*history) > bm.maxHistory {
		*history = (*history)[len(*history)-bm.maxHistory:]
	}
}

// calculateStatistics calculates statistical measures from history.
func (bm *BaselineMetrics) calculateStatistics() {
	bm.AvgThroughput = bm.calculateMean(bm.ThroughputHistory)
	bm.AvgLatency = bm.calculateMean(bm.LatencyHistory)
	bm.AvgErrorRate = bm.calculateMean(bm.ErrorRateHistory)
	bm.AvgCPUUsage = bm.calculateMean(bm.CPUUsageHistory)
	bm.AvgMemoryUsage = bm.calculateMean(bm.MemoryUsageHistory)
	
	bm.ThroughputStdDev = bm.calculateStdDev(bm.ThroughputHistory, bm.AvgThroughput)
	bm.LatencyStdDev = bm.calculateStdDev(bm.LatencyHistory, bm.AvgLatency)
	bm.ErrorRateStdDev = bm.calculateStdDev(bm.ErrorRateHistory, bm.AvgErrorRate)
}

// calculateMean calculates the mean of a slice of values.
func (bm *BaselineMetrics) calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

// calculateStdDev calculates the standard deviation.
func (bm *BaselineMetrics) calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	
	sumSquaredDiffs := 0.0
	for _, value := range values {
		diff := value - mean
		sumSquaredDiffs += diff * diff
	}
	
	variance := sumSquaredDiffs / float64(len(values)-1)
	return math.Sqrt(variance)
}

// HasSufficientData checks if there's enough data for meaningful adaptation.
func (bm *BaselineMetrics) HasSufficientData() bool {
	return len(bm.ThroughputHistory) >= 10 && 
		   len(bm.LatencyHistory) >= 10 && 
		   bm.SampleCount >= 20
}

// ThresholdAdaptationEngine calculates threshold adaptations.
type ThresholdAdaptationEngine struct {
	adaptationRates map[string]float64
}

// NewThresholdAdaptationEngine creates a new adaptation engine.
func NewThresholdAdaptationEngine() *ThresholdAdaptationEngine {
	return &ThresholdAdaptationEngine{
		adaptationRates: map[string]float64{
			"throughput":  0.1, // 10% adaptation rate
			"latency":     0.1,
			"error_rate":  0.05, // 5% adaptation rate (more conservative)
		},
	}
}

// CalculateAdaptation calculates threshold adaptation multipliers.
func (tae *ThresholdAdaptationEngine) CalculateAdaptation(baseline *BaselineMetrics, current *DynamicThresholds) *ThresholdAdaptation {
	adaptation := &ThresholdAdaptation{
		ThroughputMultiplier: 1.0,
		LatencyMultiplier:    1.0,
		ErrorRateMultiplier:  1.0,
		Timestamp:            time.Now(),
	}
	
	// Adapt throughput threshold based on baseline performance
	if baseline.AvgThroughput > 0 {
		// If average throughput is much higher than current threshold, raise threshold
		ratio := baseline.AvgThroughput / current.MinThroughputMBps
		if ratio > 1.2 { // 20% better performance
			adaptation.ThroughputMultiplier = 1.0 + (ratio-1.0)*tae.adaptationRates["throughput"]
		} else if ratio < 0.8 { // 20% worse performance
			adaptation.ThroughputMultiplier = 1.0 + (ratio-1.0)*tae.adaptationRates["throughput"]
		}
	}
	
	// Adapt latency threshold based on baseline + variability
	if baseline.AvgLatency > 0 {
		// Set threshold to mean + 2*stddev (95% of normal values)
		expectedLatency := baseline.AvgLatency + 2*baseline.LatencyStdDev
		ratio := expectedLatency / current.MaxLatencyMs
		if math.Abs(ratio-1.0) > 0.2 { // 20% difference
			adaptation.LatencyMultiplier = 1.0 + (ratio-1.0)*tae.adaptationRates["latency"]
		}
	}
	
	// Adapt error rate threshold conservatively
	if baseline.AvgErrorRate >= 0 {
		// Set threshold to mean + 3*stddev (99.7% of normal values)
		expectedErrorRate := baseline.AvgErrorRate + 3*baseline.ErrorRateStdDev
		if expectedErrorRate > 0 && current.MaxErrorRate > 0 {
			ratio := expectedErrorRate / current.MaxErrorRate
			if ratio > 1.5 || ratio < 0.5 { // Only adapt for significant changes
				adaptation.ErrorRateMultiplier = 1.0 + (ratio-1.0)*tae.adaptationRates["error_rate"]
			}
		}
	}
	
	return adaptation
}

// ThresholdAdaptation contains adaptation multipliers for thresholds.
type ThresholdAdaptation struct {
	ThroughputMultiplier float64   `json:"throughput_multiplier"`
	LatencyMultiplier    float64   `json:"latency_multiplier"`
	ErrorRateMultiplier  float64   `json:"error_rate_multiplier"`
	Timestamp            time.Time `json:"timestamp"`
	Reasoning            []string  `json:"reasoning"`
}

// AlertThreshold represents a configurable alert threshold.
type AlertThreshold struct {
	Value       float64   `json:"value"`
	Enabled     bool      `json:"enabled"`
	LastUpdated time.Time `json:"last_updated"`
	Source      string    `json:"source"` // "manual", "adaptive", "default"
}