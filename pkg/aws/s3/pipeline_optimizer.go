/*
Package s3 pipeline optimizer implements dynamic pipeline depth optimization for optimal throughput.

This module provides intelligent pipeline depth adjustment algorithms that maximize throughput
while minimizing memory usage and avoiding congestion-induced performance degradation.
*/
package s3

import (
	"context"
	"math"
	"sync"
	"time"
)

// PipelineOptimizer manages dynamic pipeline depth optimization across prefixes.
type PipelineOptimizer struct {
	strategy            PipelineOptimizationStrategy
	prefixPipelines     map[string]*PipelineMeta
	globalPipelineState *GlobalPipelineState
	performanceTracker  *PipelinePerformanceTracker
	adaptationEngine    *PipelineAdaptationEngine
	resourceManager     *PipelineResourceManager
	predictor          *PipelinePredictor
	optimizer          *DepthOptimizer
	
	// Configuration
	minPipelineDepth    int
	maxPipelineDepth    int
	adaptationRate      float64
	optimizationInterval time.Duration
	performanceWindow   time.Duration
	
	// State management
	mu                  sync.RWMutex
	isRunning          bool
	lastOptimization   time.Time
	optimizationHistory []OptimizationEvent
	
	// Memory and resource tracking
	memoryThreshold     float64
	cpuThreshold        float64
	networkThreshold    float64
}

// PipelineOptimizationStrategy defines pipeline depth optimization strategies.
type PipelineOptimizationStrategy string

const (
	PipelineOptimizationAdaptive    PipelineOptimizationStrategy = "adaptive"
	PipelineOptimizationThroughput  PipelineOptimizationStrategy = "throughput"
	PipelineOptimizationLatency     PipelineOptimizationStrategy = "latency"
	PipelineOptimizationResource    PipelineOptimizationStrategy = "resource"
	PipelineOptimizationHybrid      PipelineOptimizationStrategy = "hybrid"
)

// PipelineMeta contains metadata and state for a single prefix pipeline.
type PipelineMeta struct {
	PrefixID              string
	CurrentDepth          int
	OptimalDepth          int
	MinDepth              int
	MaxDepth              int
	LastAdjustment        time.Time
	AdjustmentHistory     []DepthAdjustment
	PerformanceMetrics    *PipelinePerformanceMetrics
	ResourceUsage         *PipelineResourceUsage
	PredictedPerformance  *PipelinePerformancePrediction
	
	// Adaptation state
	AdaptationRate        float64
	StabilityScore        float64
	PerformanceScore      float64
	ResourceScore         float64
	
	// Congestion control
	CongestionState       PipelineCongestionState
	BackoffMultiplier     float64
	ProbePhase           bool
	
	// Statistics
	TotalAdjustments      int
	SuccessfulAdjustments int
	FailedAdjustments     int
}

// PipelinePerformanceMetrics tracks performance metrics for pipeline optimization.
type PipelinePerformanceMetrics struct {
	PrefixID                string
	ActiveConnections       int
	ThroughputMBps          float64
	LatencyMs               float64
	ErrorRate               float64
	CompletionRate          float64
	QueueDepth              int
	MemoryUsageMB           float64
	CPUUsagePercent         float64
	NetworkUtilization      float64
	BandwidthEfficiency     float64
	ConcurrencyEfficiency   float64
	ResourceEfficiency      float64
	
	// Time series data
	ThroughputHistory       []TimeSeriesPoint
	LatencyHistory          []TimeSeriesPoint
	ErrorRateHistory        []TimeSeriesPoint
	DepthHistory            []TimeSeriesPoint
	
	LastUpdate              time.Time
}

// PipelineResourceUsage tracks resource consumption for pipeline operations.
type PipelineResourceUsage struct {
	MemoryAllocatedMB       float64
	MemoryAvailableMB       float64
	CPUCores                float64
	NetworkBandwidthMBps    float64
	DiskIOPS                float64
	
	// Resource limits
	MemoryLimitMB           float64
	CPULimit                float64
	NetworkLimitMBps        float64
	
	// Efficiency metrics
	MemoryEfficiency        float64
	CPUEfficiency           float64
	NetworkEfficiency       float64
	
	LastUpdate              time.Time
}

// GlobalPipelineState maintains global pipeline optimization state.
type GlobalPipelineState struct {
	TotalActivePipelines    int
	TotalDepth              int
	AverageDepth            float64
	GlobalThroughput        float64
	GlobalLatency           float64
	GlobalErrorRate         float64
	GlobalMemoryUsage       float64
	GlobalCPUUsage          float64
	
	// System-wide optimization state
	OptimizationMode        PipelineOptimizationStrategy
	AdaptationPhase         AdaptationPhase
	SystemLoad              SystemLoadLevel
	PerformanceTrend        TrendDirection
	
	// Coordination state
	RebalanceInProgress     bool
	GlobalOptimizationLock  bool
	LastGlobalOptimization  time.Time
	
	LastUpdate              time.Time
}

// DepthAdjustment represents a pipeline depth adjustment event.
type DepthAdjustment struct {
	Timestamp           time.Time
	OldDepth            int
	NewDepth            int
	Reason              AdjustmentReason
	ExpectedImprovement float64
	ActualImprovement   float64
	Success             bool
	Duration            time.Duration
}

// AdjustmentReason defines reasons for pipeline depth adjustments.
type AdjustmentReason string

const (
	ReasonThroughputIncrease  AdjustmentReason = "throughput_increase"
	ReasonLatencyDecrease     AdjustmentReason = "latency_decrease"
	ReasonResourceOptimization AdjustmentReason = "resource_optimization"
	ReasonCongestionControl   AdjustmentReason = "congestion_control"
	ReasonErrorReduction      AdjustmentReason = "error_reduction"
	ReasonPredictiveAdjustment AdjustmentReason = "predictive_adjustment"
	ReasonSystemRebalance     AdjustmentReason = "system_rebalance"
)

// PipelineCongestionState represents the congestion state of a pipeline.
type PipelineCongestionState string

const (
	PipelineCongestionNone     PipelineCongestionState = "none"
	PipelineCongestionMild     PipelineCongestionState = "mild"
	PipelineCongestionModerate PipelineCongestionState = "moderate"
	PipelineCongestionSevere   PipelineCongestionState = "severe"
)

// AdaptationPhase represents the current adaptation phase.
type AdaptationPhase string

const (
	PhaseStable     AdaptationPhase = "stable"
	PhaseExploring  AdaptationPhase = "exploring"
	PhaseOptimizing AdaptationPhase = "optimizing"
	PhaseConverging AdaptationPhase = "converging"
)

// SystemLoadLevel represents system-wide load levels.
type SystemLoadLevel string

const (
	LoadLow    SystemLoadLevel = "low"
	LoadMedium SystemLoadLevel = "medium"
	LoadHigh   SystemLoadLevel = "high"
	LoadCritical SystemLoadLevel = "critical"
)

// NewPipelineOptimizer creates a new pipeline optimizer with specified strategy.
func NewPipelineOptimizer(strategy PipelineOptimizationStrategy) *PipelineOptimizer {
	return &PipelineOptimizer{
		strategy:            strategy,
		prefixPipelines:     make(map[string]*PipelineMeta),
		globalPipelineState: &GlobalPipelineState{
			OptimizationMode: strategy,
			AdaptationPhase:  PhaseStable,
			SystemLoad:       LoadLow,
		},
		performanceTracker: NewPipelinePerformanceTracker(),
		adaptationEngine:   NewPipelineAdaptationEngine(),
		resourceManager:    NewPipelineResourceManager(),
		predictor:         NewPipelinePredictor(),
		optimizer:         NewDepthOptimizer(),
		
		minPipelineDepth:    1,
		maxPipelineDepth:    32,
		adaptationRate:      0.2,
		optimizationInterval: time.Second * 5,
		performanceWindow:   time.Minute * 2,
		
		memoryThreshold:     0.8,
		cpuThreshold:        0.8,
		networkThreshold:    0.9,
	}
}

// RegisterPipeline registers a new pipeline for optimization.
func (po *PipelineOptimizer) RegisterPipeline(prefixID string, initialDepth int, maxDepth int) {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	po.prefixPipelines[prefixID] = &PipelineMeta{
		PrefixID:             prefixID,
		CurrentDepth:         initialDepth,
		OptimalDepth:         initialDepth,
		MinDepth:             po.minPipelineDepth,
		MaxDepth:             minIntPipeline(maxDepth, po.maxPipelineDepth),
		LastAdjustment:       time.Now(),
		AdjustmentHistory:    make([]DepthAdjustment, 0, 100),
		PerformanceMetrics:   NewPipelinePerformanceMetrics(prefixID),
		ResourceUsage:        NewPipelineResourceUsage(),
		AdaptationRate:       po.adaptationRate,
		StabilityScore:       1.0,
		PerformanceScore:     1.0,
		ResourceScore:        1.0,
		CongestionState:      PipelineCongestionNone,
		BackoffMultiplier:    1.0,
	}
	
	po.updateGlobalState()
}

// UpdatePipelineMetrics updates performance metrics for a pipeline.
func (po *PipelineOptimizer) UpdatePipelineMetrics(prefixID string, metrics *PipelinePerformanceMetrics) {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	pipeline, exists := po.prefixPipelines[prefixID]
	if !exists {
		return
	}
	
	// Update current metrics
	pipeline.PerformanceMetrics = metrics
	pipeline.PerformanceMetrics.LastUpdate = time.Now()
	
	// Update historical data
	po.updateHistoricalMetrics(pipeline, metrics)
	
	// Update performance scores
	po.updatePerformanceScores(pipeline)
	
	// Update congestion state
	po.updateCongestionState(pipeline)
	
	// Update global state
	po.updateGlobalState()
	
	// Trigger optimization if needed
	if po.shouldOptimize(pipeline) {
		po.optimizePipelineDepth(pipeline)
	}
}

// OptimizeAllPipelines performs comprehensive optimization across all pipelines.
func (po *PipelineOptimizer) OptimizeAllPipelines() {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	if time.Since(po.lastOptimization) < po.optimizationInterval {
		return
	}
	
	// Update global optimization state
	po.globalPipelineState.GlobalOptimizationLock = true
	defer func() {
		po.globalPipelineState.GlobalOptimizationLock = false
		po.lastOptimization = time.Now()
	}()
	
	// Perform system-wide analysis
	po.analyzeSystemPerformance()
	
	// Generate predictions for all pipelines
	predictions := po.predictor.PredictOptimalDepths(po.prefixPipelines)
	
	// Optimize each pipeline based on global state
	for prefixID, pipeline := range po.prefixPipelines {
		if prediction, exists := predictions[prefixID]; exists {
			po.optimizePipelineWithPrediction(pipeline, prediction)
		}
	}
	
	// Perform global rebalancing if needed
	if po.shouldPerformGlobalRebalance() {
		po.performGlobalRebalance()
	}
	
	// Update global state
	po.updateGlobalState()
}

// GetOptimalDepth returns the optimal pipeline depth for a prefix.
func (po *PipelineOptimizer) GetOptimalDepth(prefixID string) int {
	po.mu.RLock()
	defer po.mu.RUnlock()
	
	if pipeline, exists := po.prefixPipelines[prefixID]; exists {
		return pipeline.OptimalDepth
	}
	
	return po.minPipelineDepth
}

// GetPipelineMetrics returns comprehensive pipeline metrics.
func (po *PipelineOptimizer) GetPipelineMetrics() *PipelineOptimizationMetrics {
	po.mu.RLock()
	defer po.mu.RUnlock()
	
	metrics := &PipelineOptimizationMetrics{
		TotalPipelines:          len(po.prefixPipelines),
		AveragePipelineDepth:    po.calculateAverageDepth(),
		OptimizationEfficiency:  po.calculateOptimizationEfficiency(),
		ResourceUtilization:     po.calculateResourceUtilization(),
		PerformanceScore:        po.calculateGlobalPerformanceScore(),
		AdaptationRate:          po.adaptationRate,
		LastOptimization:        po.lastOptimization,
	}
	
	return metrics
}

// Start begins the pipeline optimization process.
func (po *PipelineOptimizer) Start(ctx context.Context) {
	po.mu.Lock()
	po.isRunning = true
	po.mu.Unlock()
	
	go po.optimizationLoop(ctx)
	go po.monitoringLoop(ctx)
	go po.adaptationLoop(ctx)
}

// optimizationLoop runs the main optimization loop.
func (po *PipelineOptimizer) optimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(po.optimizationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			po.OptimizeAllPipelines()
		}
	}
}

// monitoringLoop continuously monitors pipeline performance.
func (po *PipelineOptimizer) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			po.performanceTracker.CollectMetrics(po.prefixPipelines)
			po.updateResourceUsage()
		}
	}
}

// adaptationLoop runs the adaptation engine.
func (po *PipelineOptimizer) adaptationLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			po.adaptationEngine.AdaptParameters(po.prefixPipelines, po.globalPipelineState)
		}
	}
}

// Helper methods

func (po *PipelineOptimizer) updateHistoricalMetrics(pipeline *PipelineMeta, metrics *PipelinePerformanceMetrics) {
	maxHistory := 200
	now := time.Now()
	
	// Update throughput history
	metrics.ThroughputHistory = append(metrics.ThroughputHistory, TimeSeriesPoint{
		Timestamp: now,
		Value:     metrics.ThroughputMBps,
	})
	if len(metrics.ThroughputHistory) > maxHistory {
		metrics.ThroughputHistory = metrics.ThroughputHistory[1:]
	}
	
	// Update latency history
	metrics.LatencyHistory = append(metrics.LatencyHistory, TimeSeriesPoint{
		Timestamp: now,
		Value:     metrics.LatencyMs,
	})
	if len(metrics.LatencyHistory) > maxHistory {
		metrics.LatencyHistory = metrics.LatencyHistory[1:]
	}
	
	// Update error rate history
	metrics.ErrorRateHistory = append(metrics.ErrorRateHistory, TimeSeriesPoint{
		Timestamp: now,
		Value:     metrics.ErrorRate,
	})
	if len(metrics.ErrorRateHistory) > maxHistory {
		metrics.ErrorRateHistory = metrics.ErrorRateHistory[1:]
	}
	
	// Update depth history
	metrics.DepthHistory = append(metrics.DepthHistory, TimeSeriesPoint{
		Timestamp: now,
		Value:     float64(pipeline.CurrentDepth),
	})
	if len(metrics.DepthHistory) > maxHistory {
		metrics.DepthHistory = metrics.DepthHistory[1:]
	}
}

func (po *PipelineOptimizer) updatePerformanceScores(pipeline *PipelineMeta) {
	metrics := pipeline.PerformanceMetrics
	
	// Calculate throughput score (normalized by expected capacity)
	expectedThroughput := po.calculateExpectedThroughput(pipeline)
	pipeline.PerformanceScore = math.Min(metrics.ThroughputMBps/expectedThroughput, 2.0)
	
	// Calculate stability score based on variance
	if len(metrics.ThroughputHistory) >= 5 {
		variance := po.calculateVariance(metrics.ThroughputHistory)
		pipeline.StabilityScore = 1.0 / (1.0 + variance)
	}
	
	// Calculate resource score
	pipeline.ResourceScore = po.calculateResourceScore(pipeline)
}

func (po *PipelineOptimizer) updateCongestionState(pipeline *PipelineMeta) {
	metrics := pipeline.PerformanceMetrics
	
	// Determine congestion based on multiple factors
	congestionScore := 0.0
	
	// Factor 1: Error rate
	if metrics.ErrorRate > 0.1 {
		congestionScore += 0.4
	} else if metrics.ErrorRate > 0.05 {
		congestionScore += 0.2
	}
	
	// Factor 2: Latency increase
	if len(metrics.LatencyHistory) >= 5 {
		recentLatency := po.calculateRecentAverage(metrics.LatencyHistory, 3)
		historicalLatency := po.calculateOverallAverage(metrics.LatencyHistory)
		if recentLatency > historicalLatency*1.5 {
			congestionScore += 0.3
		} else if recentLatency > historicalLatency*1.2 {
			congestionScore += 0.15
		}
	}
	
	// Factor 3: Throughput decrease
	if len(metrics.ThroughputHistory) >= 5 {
		recentThroughput := po.calculateRecentAverage(metrics.ThroughputHistory, 3)
		historicalThroughput := po.calculateOverallAverage(metrics.ThroughputHistory)
		if recentThroughput < historicalThroughput*0.7 {
			congestionScore += 0.3
		} else if recentThroughput < historicalThroughput*0.8 {
			congestionScore += 0.15
		}
	}
	
	// Update congestion state
	switch {
	case congestionScore >= 0.7:
		pipeline.CongestionState = PipelineCongestionSevere
	case congestionScore >= 0.4:
		pipeline.CongestionState = PipelineCongestionModerate
	case congestionScore >= 0.2:
		pipeline.CongestionState = PipelineCongestionMild
	default:
		pipeline.CongestionState = PipelineCongestionNone
	}
}

func (po *PipelineOptimizer) shouldOptimize(pipeline *PipelineMeta) bool {
	// Check if enough time has passed since last adjustment
	if time.Since(pipeline.LastAdjustment) < time.Second*30 {
		return false
	}
	
	// Check if performance has degraded significantly
	if pipeline.PerformanceScore < 0.7 {
		return true
	}
	
	// Check if congestion is detected
	if pipeline.CongestionState != PipelineCongestionNone {
		return true
	}
	
	// Check if there's potential for improvement
	if pipeline.StabilityScore > 0.8 && pipeline.PerformanceScore < 1.5 {
		return true
	}
	
	return false
}

func (po *PipelineOptimizer) optimizePipelineDepth(pipeline *PipelineMeta) {
	currentDepth := pipeline.CurrentDepth
	var newDepth int
	var reason AdjustmentReason
	
	switch po.strategy {
	case PipelineOptimizationThroughput:
		newDepth, reason = po.optimizeForThroughput(pipeline)
	case PipelineOptimizationLatency:
		newDepth, reason = po.optimizeForLatency(pipeline)
	case PipelineOptimizationResource:
		newDepth, reason = po.optimizeForResource(pipeline)
	case PipelineOptimizationHybrid:
		newDepth, reason = po.optimizeHybrid(pipeline)
	default:
		newDepth, reason = po.optimizeAdaptive(pipeline)
	}
	
	// Apply constraints
	newDepth = maxIntPipeline(pipeline.MinDepth, minIntPipeline(newDepth, pipeline.MaxDepth))
	
	if newDepth != currentDepth {
		po.applyDepthAdjustment(pipeline, newDepth, reason)
	}
}

func (po *PipelineOptimizer) optimizeAdaptive(pipeline *PipelineMeta) (int, AdjustmentReason) {
	currentDepth := pipeline.CurrentDepth
	metrics := pipeline.PerformanceMetrics
	
	// Adaptive algorithm based on current state
	switch pipeline.CongestionState {
	case PipelineCongestionSevere:
		// Aggressive reduction
		return max(pipeline.MinDepth, int(float64(currentDepth)*0.5)), ReasonCongestionControl
	case PipelineCongestionModerate:
		// Moderate reduction
		return max(pipeline.MinDepth, int(float64(currentDepth)*0.75)), ReasonCongestionControl
	case PipelineCongestionMild:
		// Small reduction
		return max(pipeline.MinDepth, currentDepth-1), ReasonCongestionControl
	default:
		// No congestion, consider increasing if performance allows
		if pipeline.PerformanceScore > 1.2 && pipeline.StabilityScore > 0.8 && metrics.ResourceEfficiency > 0.7 {
			return minIntPipeline(pipeline.MaxDepth, currentDepth+1), ReasonThroughputIncrease
		}
	}
	
	return currentDepth, ReasonThroughputIncrease
}

func (po *PipelineOptimizer) optimizeForThroughput(pipeline *PipelineMeta) (int, AdjustmentReason) {
	// Prioritize maximum throughput
	if pipeline.PerformanceScore < 1.0 && pipeline.CongestionState == PipelineCongestionNone {
		return minIntPipeline(pipeline.MaxDepth, pipeline.CurrentDepth+2), ReasonThroughputIncrease
	}
	
	if pipeline.CongestionState != PipelineCongestionNone {
		return max(pipeline.MinDepth, pipeline.CurrentDepth-1), ReasonCongestionControl
	}
	
	return pipeline.CurrentDepth, ReasonThroughputIncrease
}

func (po *PipelineOptimizer) optimizeForLatency(pipeline *PipelineMeta) (int, AdjustmentReason) {
	// Prioritize low latency
	metrics := pipeline.PerformanceMetrics
	
	if len(metrics.LatencyHistory) >= 3 {
		recentLatency := po.calculateRecentAverage(metrics.LatencyHistory, 3)
		if recentLatency > 200.0 { // High latency threshold
			return max(pipeline.MinDepth, pipeline.CurrentDepth-1), ReasonLatencyDecrease
		}
	}
	
	return pipeline.CurrentDepth, ReasonLatencyDecrease
}

func (po *PipelineOptimizer) optimizeForResource(pipeline *PipelineMeta) (int, AdjustmentReason) {
	// Prioritize resource efficiency
	if pipeline.ResourceScore < 0.7 {
		return max(pipeline.MinDepth, pipeline.CurrentDepth-1), ReasonResourceOptimization
	}
	
	return pipeline.CurrentDepth, ReasonResourceOptimization
}

func (po *PipelineOptimizer) optimizeHybrid(pipeline *PipelineMeta) (int, AdjustmentReason) {
	// Balanced optimization considering all factors
	score := (pipeline.PerformanceScore + pipeline.StabilityScore + pipeline.ResourceScore) / 3.0
	
	if score < 0.8 {
		return max(pipeline.MinDepth, pipeline.CurrentDepth-1), ReasonSystemRebalance
	} else if score > 1.2 && pipeline.CongestionState == PipelineCongestionNone {
		return minIntPipeline(pipeline.MaxDepth, pipeline.CurrentDepth+1), ReasonThroughputIncrease
	}
	
	return pipeline.CurrentDepth, ReasonSystemRebalance
}

func (po *PipelineOptimizer) applyDepthAdjustment(pipeline *PipelineMeta, newDepth int, reason AdjustmentReason) {
	oldDepth := pipeline.CurrentDepth
	
	adjustment := DepthAdjustment{
		Timestamp:           time.Now(),
		OldDepth:            oldDepth,
		NewDepth:            newDepth,
		Reason:              reason,
		ExpectedImprovement: po.calculateExpectedImprovement(pipeline, newDepth),
	}
	
	pipeline.CurrentDepth = newDepth
	pipeline.OptimalDepth = newDepth
	pipeline.LastAdjustment = time.Now()
	pipeline.AdjustmentHistory = append(pipeline.AdjustmentHistory, adjustment)
	pipeline.TotalAdjustments++
	
	// Limit history size
	if len(pipeline.AdjustmentHistory) > 50 {
		pipeline.AdjustmentHistory = pipeline.AdjustmentHistory[1:]
	}
}

// Additional helper methods

func (po *PipelineOptimizer) calculateExpectedThroughput(pipeline *PipelineMeta) float64 {
	// Base calculation on pipeline depth and historical performance
	baseRate := 10.0 // MB/s per connection
	depthFactor := math.Log(float64(pipeline.CurrentDepth + 1))
	return baseRate * depthFactor
}

func (po *PipelineOptimizer) calculateResourceScore(pipeline *PipelineMeta) float64 {
	if pipeline.ResourceUsage == nil {
		return 1.0
	}
	
	memoryScore := 1.0 - (pipeline.ResourceUsage.MemoryEfficiency)
	cpuScore := 1.0 - (pipeline.ResourceUsage.CPUEfficiency)
	networkScore := 1.0 - (pipeline.ResourceUsage.NetworkEfficiency)
	
	return (memoryScore + cpuScore + networkScore) / 3.0
}

func (po *PipelineOptimizer) calculateVariance(history []TimeSeriesPoint) float64 {
	if len(history) < 2 {
		return 0.0
	}
	
	mean := 0.0
	for _, point := range history {
		mean += point.Value
	}
	mean /= float64(len(history))
	
	variance := 0.0
	for _, point := range history {
		diff := point.Value - mean
		variance += diff * diff
	}
	variance /= float64(len(history))
	
	return variance
}

func (po *PipelineOptimizer) calculateRecentAverage(history []TimeSeriesPoint, count int) float64 {
	if len(history) == 0 {
		return 0.0
	}
	
	start := len(history) - count
	if start < 0 {
		start = 0
	}
	
	sum := 0.0
	for i := start; i < len(history); i++ {
		sum += history[i].Value
	}
	
	return sum / float64(len(history)-start)
}

func (po *PipelineOptimizer) calculateOverallAverage(history []TimeSeriesPoint) float64 {
	if len(history) == 0 {
		return 0.0
	}
	
	sum := 0.0
	for _, point := range history {
		sum += point.Value
	}
	
	return sum / float64(len(history))
}

func (po *PipelineOptimizer) calculateExpectedImprovement(pipeline *PipelineMeta, newDepth int) float64 {
	currentPerformance := pipeline.PerformanceScore
	depthRatio := float64(newDepth) / float64(pipeline.CurrentDepth)
	
	// Simplified improvement calculation
	expectedImprovement := (depthRatio - 1.0) * currentPerformance
	return math.Max(-0.5, math.Min(expectedImprovement, 0.5))
}

func (po *PipelineOptimizer) updateGlobalState() {
	state := po.globalPipelineState
	
	// Calculate global metrics
	totalDepth := 0
	totalThroughput := 0.0
	totalLatency := 0.0
	totalErrors := 0.0
	activeCount := 0
	
	for _, pipeline := range po.prefixPipelines {
		totalDepth += pipeline.CurrentDepth
		if pipeline.PerformanceMetrics != nil {
			totalThroughput += pipeline.PerformanceMetrics.ThroughputMBps
			totalLatency += pipeline.PerformanceMetrics.LatencyMs
			totalErrors += pipeline.PerformanceMetrics.ErrorRate
			activeCount++
		}
	}
	
	state.TotalActivePipelines = len(po.prefixPipelines)
	state.TotalDepth = totalDepth
	if len(po.prefixPipelines) > 0 {
		state.AverageDepth = float64(totalDepth) / float64(len(po.prefixPipelines))
	}
	
	if activeCount > 0 {
		state.GlobalThroughput = totalThroughput
		state.GlobalLatency = totalLatency / float64(activeCount)
		state.GlobalErrorRate = totalErrors / float64(activeCount)
	}
	
	// Update system load based on resource utilization
	resourceUtilization := po.calculateResourceUtilization()
	switch {
	case resourceUtilization > 0.9:
		state.SystemLoad = LoadCritical
	case resourceUtilization > 0.7:
		state.SystemLoad = LoadHigh
	case resourceUtilization > 0.4:
		state.SystemLoad = LoadMedium
	default:
		state.SystemLoad = LoadLow
	}
	
	state.LastUpdate = time.Now()
}

func (po *PipelineOptimizer) analyzeSystemPerformance() {
	// Analyze global performance trends and adjust optimization strategy
	state := po.globalPipelineState
	
	// Determine adaptation phase
	recentAdjustments := po.countRecentAdjustments(time.Minute * 5)
	if recentAdjustments > len(po.prefixPipelines)*2 {
		state.AdaptationPhase = PhaseExploring
	} else if recentAdjustments > 0 {
		state.AdaptationPhase = PhaseOptimizing
	} else {
		state.AdaptationPhase = PhaseStable
	}
	
	// Update performance trend
	if len(po.optimizationHistory) >= 5 {
		recentPerformance := po.calculateRecentPerformanceTrend()
		if recentPerformance > 0.1 {
			state.PerformanceTrend = TrendIncreasing
		} else if recentPerformance < -0.1 {
			state.PerformanceTrend = TrendDecreasing
		} else {
			state.PerformanceTrend = TrendStable
		}
	}
}

func (po *PipelineOptimizer) shouldPerformGlobalRebalance() bool {
	state := po.globalPipelineState
	
	// Rebalance if system is under high load and adaptation hasn't converged
	if state.SystemLoad >= LoadHigh && state.AdaptationPhase != PhaseStable {
		return true
	}
	
	// Rebalance if global error rate is high
	if state.GlobalErrorRate > 0.1 {
		return true
	}
	
	// Rebalance if there's significant imbalance between pipelines
	depthVariance := po.calculateDepthVariance()
	if depthVariance > 4.0 { // High variance in pipeline depths
		return true
	}
	
	return false
}

func (po *PipelineOptimizer) performGlobalRebalance() {
	// Implement global rebalancing logic
	state := po.globalPipelineState
	state.RebalanceInProgress = true
	defer func() {
		state.RebalanceInProgress = false
		state.LastGlobalOptimization = time.Now()
	}()
	
	// Calculate optimal distribution
	optimalDepths := po.optimizer.CalculateOptimalDistribution(po.prefixPipelines, state)
	
	// Apply adjustments
	for prefixID, optimalDepth := range optimalDepths {
		if pipeline, exists := po.prefixPipelines[prefixID]; exists {
			if optimalDepth != pipeline.CurrentDepth {
				po.applyDepthAdjustment(pipeline, optimalDepth, ReasonSystemRebalance)
			}
		}
	}
}

func (po *PipelineOptimizer) calculateAverageDepth() float64 {
	if len(po.prefixPipelines) == 0 {
		return 0.0
	}
	
	totalDepth := 0
	for _, pipeline := range po.prefixPipelines {
		totalDepth += pipeline.CurrentDepth
	}
	
	return float64(totalDepth) / float64(len(po.prefixPipelines))
}

func (po *PipelineOptimizer) calculateOptimizationEfficiency() float64 {
	if len(po.prefixPipelines) == 0 {
		return 1.0
	}
	
	totalEfficiency := 0.0
	for _, pipeline := range po.prefixPipelines {
		if pipeline.TotalAdjustments > 0 {
			successRate := float64(pipeline.SuccessfulAdjustments) / float64(pipeline.TotalAdjustments)
			totalEfficiency += successRate * pipeline.PerformanceScore
		} else {
			totalEfficiency += pipeline.PerformanceScore
		}
	}
	
	return totalEfficiency / float64(len(po.prefixPipelines))
}

func (po *PipelineOptimizer) calculateResourceUtilization() float64 {
	totalMemoryUsage := 0.0
	totalCPUUsage := 0.0
	count := 0
	
	for _, pipeline := range po.prefixPipelines {
		if pipeline.PerformanceMetrics != nil {
			totalMemoryUsage += pipeline.PerformanceMetrics.MemoryUsageMB
			totalCPUUsage += pipeline.PerformanceMetrics.CPUUsagePercent
			count++
		}
	}
	
	if count == 0 {
		return 0.0
	}
	
	avgMemoryUsage := totalMemoryUsage / float64(count)
	avgCPUUsage := totalCPUUsage / float64(count)
	
	// Normalize to 0-1 range (assuming 1GB memory limit and 100% CPU)
	memoryUtilization := avgMemoryUsage / 1024.0
	cpuUtilization := avgCPUUsage / 100.0
	
	return (memoryUtilization + cpuUtilization) / 2.0
}

func (po *PipelineOptimizer) calculateGlobalPerformanceScore() float64 {
	if len(po.prefixPipelines) == 0 {
		return 1.0
	}
	
	totalScore := 0.0
	for _, pipeline := range po.prefixPipelines {
		totalScore += pipeline.PerformanceScore
	}
	
	return totalScore / float64(len(po.prefixPipelines))
}

func (po *PipelineOptimizer) countRecentAdjustments(duration time.Duration) int {
	count := 0
	cutoff := time.Now().Add(-duration)
	
	for _, pipeline := range po.prefixPipelines {
		for _, adjustment := range pipeline.AdjustmentHistory {
			if adjustment.Timestamp.After(cutoff) {
				count++
			}
		}
	}
	
	return count
}

func (po *PipelineOptimizer) calculateRecentPerformanceTrend() float64 {
	if len(po.optimizationHistory) < 5 {
		return 0.0
	}
	
	// Calculate trend from recent optimization events
	recent := po.optimizationHistory[len(po.optimizationHistory)-3:]
	historical := po.optimizationHistory[:len(po.optimizationHistory)-3]
	
	recentAvg := 0.0
	historicalAvg := 0.0
	
	for _, event := range recent {
		recentAvg += event.PerformanceImprovement
	}
	recentAvg /= float64(len(recent))
	
	for _, event := range historical {
		historicalAvg += event.PerformanceImprovement
	}
	historicalAvg /= float64(len(historical))
	
	if historicalAvg == 0 {
		return 0.0
	}
	
	return (recentAvg - historicalAvg) / historicalAvg
}

func (po *PipelineOptimizer) calculateDepthVariance() float64 {
	if len(po.prefixPipelines) < 2 {
		return 0.0
	}
	
	mean := po.calculateAverageDepth()
	variance := 0.0
	
	for _, pipeline := range po.prefixPipelines {
		diff := float64(pipeline.CurrentDepth) - mean
		variance += diff * diff
	}
	variance /= float64(len(po.prefixPipelines))
	
	return variance
}

func (po *PipelineOptimizer) updateResourceUsage() {
	// Update resource usage for all pipelines
	for _, pipeline := range po.prefixPipelines {
		po.resourceManager.UpdateResourceUsage(pipeline)
	}
}

// Utility functions
func minIntPipeline(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntPipeline(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// PipelineOptimizationMetrics provides comprehensive optimization metrics.
type PipelineOptimizationMetrics struct {
	TotalPipelines          int
	AveragePipelineDepth    float64
	OptimizationEfficiency  float64
	ResourceUtilization     float64
	PerformanceScore        float64
	AdaptationRate          float64
	LastOptimization        time.Time
}

// OptimizationEvent represents an optimization event in the history.
type OptimizationEvent struct {
	Timestamp              time.Time
	EventType              string
	PerformanceImprovement float64
	ResourceImpact         float64
	Success                bool
}

// PipelinePerformancePrediction represents predicted pipeline performance.
type PipelinePerformancePrediction struct {
	PrefixID               string
	PredictedOptimalDepth  int
	ExpectedThroughput     float64
	ExpectedLatency        float64
	ExpectedResourceUsage  float64
	Confidence             float64
	PredictionHorizon      time.Duration
}

// Placeholder implementations for external components

func NewPipelinePerformanceMetrics(prefixID string) *PipelinePerformanceMetrics {
	return &PipelinePerformanceMetrics{
		PrefixID:            prefixID,
		ThroughputHistory:   make([]TimeSeriesPoint, 0, 200),
		LatencyHistory:      make([]TimeSeriesPoint, 0, 200),
		ErrorRateHistory:    make([]TimeSeriesPoint, 0, 200),
		DepthHistory:        make([]TimeSeriesPoint, 0, 200),
		LastUpdate:          time.Now(),
	}
}

func NewPipelineResourceUsage() *PipelineResourceUsage {
	return &PipelineResourceUsage{
		MemoryLimitMB:    1024.0,
		CPULimit:         4.0,
		NetworkLimitMBps: 1000.0,
		LastUpdate:       time.Now(),
	}
}

func NewPipelinePerformanceTracker() *PipelinePerformanceTracker {
	return &PipelinePerformanceTracker{}
}

func NewPipelineAdaptationEngine() *PipelineAdaptationEngine {
	return &PipelineAdaptationEngine{}
}

func NewPipelineResourceManager() *PipelineResourceManager {
	return &PipelineResourceManager{}
}

func NewPipelinePredictor() *PipelinePredictor {
	return &PipelinePredictor{}
}

func NewDepthOptimizer() *DepthOptimizer {
	return &DepthOptimizer{}
}

// Placeholder types for external components

type PipelinePerformanceTracker struct{}

func (ppt *PipelinePerformanceTracker) CollectMetrics(pipelines map[string]*PipelineMeta) {
	// Implementation would collect real-time performance metrics
}

type PipelineAdaptationEngine struct{}

func (pae *PipelineAdaptationEngine) AdaptParameters(pipelines map[string]*PipelineMeta, globalState *GlobalPipelineState) {
	// Implementation would adapt optimization parameters based on performance
}

type PipelineResourceManager struct{}

func (prm *PipelineResourceManager) UpdateResourceUsage(pipeline *PipelineMeta) {
	// Implementation would track actual resource usage
}

type PipelinePredictor struct{}

func (pp *PipelinePredictor) PredictOptimalDepths(pipelines map[string]*PipelineMeta) map[string]*PipelinePerformancePrediction {
	predictions := make(map[string]*PipelinePerformancePrediction)
	
	for prefixID, pipeline := range pipelines {
		predictions[prefixID] = &PipelinePerformancePrediction{
			PrefixID:              prefixID,
			PredictedOptimalDepth: pipeline.CurrentDepth,
			Confidence:            0.8,
			PredictionHorizon:     time.Minute * 5,
		}
	}
	
	return predictions
}

type DepthOptimizer struct{}

func (do *DepthOptimizer) CalculateOptimalDistribution(pipelines map[string]*PipelineMeta, globalState *GlobalPipelineState) map[string]int {
	distribution := make(map[string]int)
	
	for prefixID, pipeline := range pipelines {
		distribution[prefixID] = pipeline.CurrentDepth
	}
	
	return distribution
}

func (po *PipelineOptimizer) optimizePipelineWithPrediction(pipeline *PipelineMeta, prediction *PipelinePerformancePrediction) {
	if prediction.Confidence > 0.7 {
		po.applyDepthAdjustment(pipeline, prediction.PredictedOptimalDepth, ReasonPredictiveAdjustment)
	}
}