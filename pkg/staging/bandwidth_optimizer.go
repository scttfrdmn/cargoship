package staging

import (
	"context"
	"math"
	"sync"
	"time"
)

// BandwidthOptimizer provides optimal bandwidth utilization with congestion avoidance.
type BandwidthOptimizer struct {
	config                  *AdaptationConfig
	congestionController    *CongestionController
	bandwidthEstimator      *BandwidthEstimator
	flowController          *FlowController
	utilizationHistory      *UtilizationHistory
	currentUtilization      *BandwidthUtilization
	optimizationCallbacks   []OptimizationCallback
	mu                      sync.RWMutex
	ctx                     context.Context
	cancel                  context.CancelFunc
	active                  bool
}

// BandwidthUtilization represents current bandwidth utilization state.
type BandwidthUtilization struct {
	Timestamp             time.Time
	AvailableBandwidthMBps float64
	UtilizedBandwidthMBps  float64
	UtilizationRatio       float64
	CongestionLevel        float64
	OptimalConcurrency     int
	OptimalChunkSizeMB     int
	EfficiencyScore        float64
	ThroughputProjection   float64
	NetworkHealth          NetworkHealthStatus
}

// NetworkHealthStatus indicates the health of network conditions.
type NetworkHealthStatus int

const (
	NetworkHealthExcellent NetworkHealthStatus = iota
	NetworkHealthGood
	NetworkHealthFair
	NetworkHealthPoor
	NetworkHealthCritical
)

// OptimizationCallback is called when bandwidth optimization adjustments are made.
type OptimizationCallback func(*BandwidthUtilization, *OptimizationRecommendation) error

// OptimizationRecommendation provides recommendations for bandwidth optimization.
type OptimizationRecommendation struct {
	Timestamp               time.Time
	RecommendedConcurrency  int
	RecommendedChunkSizeMB  int
	RecommendedCompression  string
	RecommendedFlowControl  *FlowControlSettings
	PredictedImprovement    float64
	Confidence              float64
	Reason                  string
	Priority                OptimizationPriority
}

// OptimizationPriority indicates the priority of an optimization recommendation.
type OptimizationPriority int

const (
	PriorityLow OptimizationPriority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// NewBandwidthOptimizer creates a new bandwidth optimizer.
func NewBandwidthOptimizer(config *AdaptationConfig) *BandwidthOptimizer {
	return &BandwidthOptimizer{
		config:                config,
		congestionController:  NewCongestionController(config),
		bandwidthEstimator:    NewBandwidthEstimator(),
		flowController:        NewFlowController(config),
		utilizationHistory:    NewUtilizationHistory(),
		currentUtilization:    NewDefaultBandwidthUtilization(),
		optimizationCallbacks: make([]OptimizationCallback, 0),
		active:                false,
	}
}

// Start begins bandwidth optimization.
func (bo *BandwidthOptimizer) Start(ctx context.Context) error {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	
	if bo.active {
		return nil
	}
	
	bo.ctx, bo.cancel = context.WithCancel(ctx)
	bo.active = true
	
	// Start optimization loop
	go bo.optimizationLoop(bo.ctx)
	
	// Start congestion monitoring
	go bo.congestionController.Start(bo.ctx)
	
	// Start bandwidth estimation
	go bo.bandwidthEstimator.Start(bo.ctx)
	
	return nil
}

// Stop gracefully shuts down the bandwidth optimizer.
func (bo *BandwidthOptimizer) Stop() error {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	
	if !bo.active {
		return nil
	}
	
	bo.active = false
	bo.cancel()
	
	return nil
}

// optimizationLoop runs the main bandwidth optimization loop.
func (bo *BandwidthOptimizer) optimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5) // Optimize every 5 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bo.performOptimization()
		}
	}
}

// performOptimization performs bandwidth optimization analysis and recommendations.
func (bo *BandwidthOptimizer) performOptimization() {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	
	// Update current utilization metrics
	bo.updateUtilizationMetrics()
	
	// Generate optimization recommendations
	recommendation := bo.generateOptimizationRecommendation()
	if recommendation == nil {
		return // No optimization needed
	}
	
	// Record utilization in history
	bo.utilizationHistory.RecordUtilization(bo.currentUtilization)
	
	// Notify callbacks
	bo.notifyOptimizationCallbacks(bo.currentUtilization, recommendation)
}

// updateUtilizationMetrics updates current bandwidth utilization metrics.
func (bo *BandwidthOptimizer) updateUtilizationMetrics() {
	// Get bandwidth estimation
	availableBW := bo.bandwidthEstimator.GetEstimatedBandwidth()
	utilizedBW := bo.bandwidthEstimator.GetUtilizedBandwidth()
	
	// Get congestion information
	congestionLevel := bo.congestionController.GetCongestionLevel()
	
	// Update utilization state
	bo.currentUtilization.Timestamp = time.Now()
	bo.currentUtilization.AvailableBandwidthMBps = availableBW
	bo.currentUtilization.UtilizedBandwidthMBps = utilizedBW
	bo.currentUtilization.CongestionLevel = congestionLevel
	
	// Calculate utilization ratio
	if availableBW > 0 {
		bo.currentUtilization.UtilizationRatio = utilizedBW / availableBW
	}
	
	// Calculate optimal parameters
	bo.currentUtilization.OptimalConcurrency = bo.calculateOptimalConcurrency(availableBW, congestionLevel)
	bo.currentUtilization.OptimalChunkSizeMB = bo.calculateOptimalChunkSize(availableBW, congestionLevel)
	
	// Calculate efficiency score
	bo.currentUtilization.EfficiencyScore = bo.calculateEfficiencyScore()
	
	// Project throughput
	bo.currentUtilization.ThroughputProjection = bo.projectThroughput()
	
	// Assess network health
	bo.currentUtilization.NetworkHealth = bo.assessNetworkHealth()
}

// generateOptimizationRecommendation generates optimization recommendations.
func (bo *BandwidthOptimizer) generateOptimizationRecommendation() *OptimizationRecommendation {
	utilization := bo.currentUtilization
	
	// Check if optimization is needed
	optimizationNeeded, reason, priority := bo.isOptimizationNeeded(utilization)
	if !optimizationNeeded {
		return nil
	}
	
	recommendation := &OptimizationRecommendation{
		Timestamp: time.Now(),
		Reason:    reason,
		Priority:  priority,
	}
	
	// Generate specific recommendations based on current state
	switch reason {
	case "underutilization":
		recommendation = bo.recommendForUnderutilization(recommendation, utilization)
	case "congestion":
		recommendation = bo.recommendForCongestion(recommendation, utilization)
	case "poor_efficiency":
		recommendation = bo.recommendForPoorEfficiency(recommendation, utilization)
	case "network_degradation":
		recommendation = bo.recommendForNetworkDegradation(recommendation, utilization)
	default:
		recommendation = bo.recommendGeneral(recommendation, utilization)
	}
	
	// Calculate confidence and predicted improvement
	recommendation.Confidence = bo.calculateRecommendationConfidence(recommendation, utilization)
	recommendation.PredictedImprovement = bo.predictImprovement(recommendation, utilization)
	
	return recommendation
}

// isOptimizationNeeded determines if bandwidth optimization is needed.
func (bo *BandwidthOptimizer) isOptimizationNeeded(utilization *BandwidthUtilization) (bool, string, OptimizationPriority) {
	// Check for severe congestion
	if utilization.CongestionLevel > 0.8 {
		return true, "congestion", PriorityCritical
	}
	
	// Check for network health issues
	if utilization.NetworkHealth >= NetworkHealthPoor {
		return true, "network_degradation", PriorityHigh
	}
	
	// Check for severe underutilization
	if utilization.UtilizationRatio < 0.3 && utilization.AvailableBandwidthMBps > 20 {
		return true, "underutilization", PriorityMedium
	}
	
	// Check for poor efficiency
	if utilization.EfficiencyScore < 0.5 {
		return true, "poor_efficiency", PriorityMedium
	}
	
	// Check for moderate congestion
	if utilization.CongestionLevel > 0.5 {
		return true, "congestion", PriorityMedium
	}
	
	// Check for moderate underutilization
	if utilization.UtilizationRatio < 0.6 && utilization.AvailableBandwidthMBps > 10 {
		return true, "underutilization", PriorityLow
	}
	
	return false, "", PriorityLow
}

// recommendForUnderutilization generates recommendations for bandwidth underutilization.
func (bo *BandwidthOptimizer) recommendForUnderutilization(rec *OptimizationRecommendation, util *BandwidthUtilization) *OptimizationRecommendation {
	// Increase concurrency to better utilize available bandwidth
	rec.RecommendedConcurrency = min(util.OptimalConcurrency+2, bo.config.MaxConcurrency)
	
	// Increase chunk size for better efficiency
	rec.RecommendedChunkSizeMB = min(util.OptimalChunkSizeMB+10, bo.config.MaxChunkSizeMB)
	
	// Use faster compression to reduce CPU bottleneck
	if util.AvailableBandwidthMBps > 50 {
		rec.RecommendedCompression = "zstd-fast"
	} else {
		rec.RecommendedCompression = "zstd"
	}
	
	// Aggressive flow control for better utilization
	rec.RecommendedFlowControl = &FlowControlSettings{
		WindowSize:          128,
		CongestionWindow:    20,
		SlowStartThreshold:  64,
		CongestionAlgorithm: "bbr",
	}
	
	return rec
}

// recommendForCongestion generates recommendations for congestion management.
func (bo *BandwidthOptimizer) recommendForCongestion(rec *OptimizationRecommendation, util *BandwidthUtilization) *OptimizationRecommendation {
	// Reduce concurrency to ease congestion
	rec.RecommendedConcurrency = max(util.OptimalConcurrency-1, bo.config.MinConcurrency)
	
	// Use smaller chunks for better responsiveness
	rec.RecommendedChunkSizeMB = max(util.OptimalChunkSizeMB-5, bo.config.MinChunkSizeMB)
	
	// Use higher compression to reduce network load
	rec.RecommendedCompression = "zstd-high"
	
	// Conservative flow control
	rec.RecommendedFlowControl = &FlowControlSettings{
		WindowSize:          32,
		CongestionWindow:    5,
		SlowStartThreshold:  16,
		CongestionAlgorithm: "cubic",
	}
	
	return rec
}

// recommendForPoorEfficiency generates recommendations for poor efficiency.
func (bo *BandwidthOptimizer) recommendForPoorEfficiency(rec *OptimizationRecommendation, util *BandwidthUtilization) *OptimizationRecommendation {
	// Optimize for better efficiency
	rec.RecommendedConcurrency = util.OptimalConcurrency
	rec.RecommendedChunkSizeMB = util.OptimalChunkSizeMB
	
	// Balanced compression
	rec.RecommendedCompression = "zstd"
	
	// Optimized flow control
	rec.RecommendedFlowControl = &FlowControlSettings{
		WindowSize:          64,
		CongestionWindow:    10,
		SlowStartThreshold:  32,
		CongestionAlgorithm: "bbr",
	}
	
	return rec
}

// recommendForNetworkDegradation generates recommendations for network issues.
func (bo *BandwidthOptimizer) recommendForNetworkDegradation(rec *OptimizationRecommendation, util *BandwidthUtilization) *OptimizationRecommendation {
	// Conservative approach for degraded networks
	rec.RecommendedConcurrency = max(2, bo.config.MinConcurrency)
	rec.RecommendedChunkSizeMB = max(10, bo.config.MinChunkSizeMB)
	
	// Higher compression to compensate for poor bandwidth
	rec.RecommendedCompression = "zstd-high"
	
	// Conservative flow control with error recovery
	rec.RecommendedFlowControl = &FlowControlSettings{
		WindowSize:          16,
		CongestionWindow:    3,
		SlowStartThreshold:  8,
		CongestionAlgorithm: "cubic",
	}
	
	return rec
}

// recommendGeneral generates general optimization recommendations.
func (bo *BandwidthOptimizer) recommendGeneral(rec *OptimizationRecommendation, util *BandwidthUtilization) *OptimizationRecommendation {
	rec.RecommendedConcurrency = util.OptimalConcurrency
	rec.RecommendedChunkSizeMB = util.OptimalChunkSizeMB
	rec.RecommendedCompression = "zstd"
	rec.RecommendedFlowControl = DefaultFlowControlSettings()
	
	return rec
}

// Calculation methods

// calculateOptimalConcurrency calculates optimal concurrency for current bandwidth.
func (bo *BandwidthOptimizer) calculateOptimalConcurrency(bandwidth, congestion float64) int {
	// Base concurrency on available bandwidth
	baseConcurrency := int(bandwidth / 25.0) // 25 MB/s per stream
	
	// Adjust for congestion
	congestionFactor := 1.0 - congestion*0.7
	adjustedConcurrency := int(float64(baseConcurrency) * congestionFactor)
	
	// Apply bounds
	if adjustedConcurrency < bo.config.MinConcurrency {
		adjustedConcurrency = bo.config.MinConcurrency
	}
	if adjustedConcurrency > bo.config.MaxConcurrency {
		adjustedConcurrency = bo.config.MaxConcurrency
	}
	
	return adjustedConcurrency
}

// calculateOptimalChunkSize calculates optimal chunk size for current conditions.
func (bo *BandwidthOptimizer) calculateOptimalChunkSize(bandwidth, congestion float64) int {
	// Base size on bandwidth-delay product estimate
	estimatedLatency := 50.0 // Assume 50ms base latency
	if congestion > 0.3 {
		estimatedLatency += congestion * 100.0 // Add congestion latency
	}
	
	bandwidthDelayProduct := bandwidth * (estimatedLatency / 1000.0)
	
	// Adjust for congestion
	congestionFactor := 1.0 - congestion*0.5
	optimalSize := int(bandwidthDelayProduct * congestionFactor)
	
	// Apply bounds
	if optimalSize < bo.config.MinChunkSizeMB {
		optimalSize = bo.config.MinChunkSizeMB
	}
	if optimalSize > bo.config.MaxChunkSizeMB {
		optimalSize = bo.config.MaxChunkSizeMB
	}
	
	return optimalSize
}

// calculateEfficiencyScore calculates bandwidth utilization efficiency.
func (bo *BandwidthOptimizer) calculateEfficiencyScore() float64 {
	util := bo.currentUtilization
	
	// Base efficiency on utilization ratio
	baseEfficiency := util.UtilizationRatio
	
	// Penalize for congestion
	congestionPenalty := util.CongestionLevel * 0.5
	
	// Bonus for balanced utilization (around 70-80%)
	utilizationBonus := 0.0
	if util.UtilizationRatio >= 0.7 && util.UtilizationRatio <= 0.8 {
		utilizationBonus = 0.1
	}
	
	efficiency := baseEfficiency - congestionPenalty + utilizationBonus
	
	return math.Max(0.0, math.Min(1.0, efficiency))
}

// projectThroughput projects future throughput based on current trends.
func (bo *BandwidthOptimizer) projectThroughput() float64 {
	util := bo.currentUtilization
	
	// Simple projection based on current utilization and efficiency
	projectedThroughput := util.UtilizedBandwidthMBps * util.EfficiencyScore
	
	// Adjust for congestion trends
	if util.CongestionLevel > 0.5 {
		projectedThroughput *= 0.8 // Expect degradation
	} else if util.CongestionLevel < 0.2 {
		projectedThroughput *= 1.1 // Expect improvement
	}
	
	return projectedThroughput
}

// assessNetworkHealth assesses overall network health.
func (bo *BandwidthOptimizer) assessNetworkHealth() NetworkHealthStatus {
	util := bo.currentUtilization
	
	// Score based on multiple factors
	healthScore := 1.0
	
	// Congestion impact
	healthScore -= util.CongestionLevel * 0.6
	
	// Efficiency impact
	healthScore = healthScore * util.EfficiencyScore
	
	// Utilization impact (too low or too high is bad)
	if util.UtilizationRatio < 0.3 || util.UtilizationRatio > 0.9 {
		healthScore *= 0.8
	}
	
	// Convert to health status
	if healthScore >= 0.8 {
		return NetworkHealthExcellent
	} else if healthScore >= 0.6 {
		return NetworkHealthGood
	} else if healthScore >= 0.4 {
		return NetworkHealthFair
	} else if healthScore >= 0.2 {
		return NetworkHealthPoor
	}
	
	return NetworkHealthCritical
}

// calculateRecommendationConfidence calculates confidence in a recommendation.
func (bo *BandwidthOptimizer) calculateRecommendationConfidence(rec *OptimizationRecommendation, util *BandwidthUtilization) float64 {
	confidence := 0.5 // Base confidence
	
	// Higher confidence for clear problems
	if util.CongestionLevel > 0.7 {
		confidence += 0.3
	}
	if util.UtilizationRatio < 0.4 {
		confidence += 0.2
	}
	
	// Lower confidence for marginal cases
	if util.EfficiencyScore > 0.7 {
		confidence -= 0.1
	}
	
	// Historical data improves confidence
	historyBonus := bo.utilizationHistory.GetConfidenceBonus()
	confidence += historyBonus
	
	return math.Max(0.1, math.Min(0.95, confidence))
}

// predictImprovement predicts improvement from applying recommendations.
func (bo *BandwidthOptimizer) predictImprovement(rec *OptimizationRecommendation, util *BandwidthUtilization) float64 {
	improvement := 0.0
	
	// Predict improvement based on recommendation type
	switch rec.Reason {
	case "underutilization":
		improvement = (1.0 - util.UtilizationRatio) * 0.5 // Up to 50% of unused capacity
	case "congestion":
		improvement = util.CongestionLevel * 0.3 // Up to 30% improvement from congestion relief
	case "poor_efficiency":
		improvement = (1.0 - util.EfficiencyScore) * 0.4 // Up to 40% efficiency improvement
	case "network_degradation":
		improvement = 0.2 // Conservative improvement for degraded networks
	default:
		improvement = 0.1 // General optimization
	}
	
	return math.Min(improvement, 0.5) // Cap at 50% improvement
}

// notifyOptimizationCallbacks notifies registered callbacks.
func (bo *BandwidthOptimizer) notifyOptimizationCallbacks(util *BandwidthUtilization, rec *OptimizationRecommendation) {
	for _, callback := range bo.optimizationCallbacks {
		go func(cb OptimizationCallback) {
			_ = cb(util, rec)
		}(callback)
	}
}

// Public API methods

// GetCurrentUtilization returns current bandwidth utilization.
func (bo *BandwidthOptimizer) GetCurrentUtilization() *BandwidthUtilization {
	bo.mu.RLock()
	defer bo.mu.RUnlock()
	
	// Return a copy
	utilCopy := *bo.currentUtilization
	return &utilCopy
}

// RegisterOptimizationCallback registers a callback for optimization events.
func (bo *BandwidthOptimizer) RegisterOptimizationCallback(callback OptimizationCallback) {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	
	bo.optimizationCallbacks = append(bo.optimizationCallbacks, callback)
}

// ForceOptimization forces immediate optimization evaluation.
func (bo *BandwidthOptimizer) ForceOptimization() {
	bo.performOptimization()
}

// NewDefaultBandwidthUtilization creates default bandwidth utilization state.
func NewDefaultBandwidthUtilization() *BandwidthUtilization {
	return &BandwidthUtilization{
		Timestamp:             time.Now(),
		AvailableBandwidthMBps: 50.0,
		UtilizedBandwidthMBps:  25.0,
		UtilizationRatio:       0.5,
		CongestionLevel:        0.1,
		OptimalConcurrency:     4,
		OptimalChunkSizeMB:     32,
		EfficiencyScore:        0.7,
		ThroughputProjection:   25.0,
		NetworkHealth:          NetworkHealthGood,
	}
}

// Supporting components

// CongestionController manages congestion detection and control.
type CongestionController struct {
	config            *AdaptationConfig
	congestionLevel   float64
	congestionHistory []float64
	mu                sync.RWMutex
}

// NewCongestionController creates a new congestion controller.
func NewCongestionController(config *AdaptationConfig) *CongestionController {
	return &CongestionController{
		config:            config,
		congestionLevel:   0.1,
		congestionHistory: make([]float64, 0),
	}
}

// Start begins congestion monitoring.
func (cc *CongestionController) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.updateCongestionLevel()
		}
	}
}

// updateCongestionLevel updates the current congestion level.
func (cc *CongestionController) updateCongestionLevel() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	// Simplified congestion detection
	// In practice, this would use network metrics, RTT, packet loss, etc.
	
	// Add some random variation for simulation
	variation := (math.Sin(float64(time.Now().Unix())/10.0) + 1.0) / 20.0 // ±0.1 variation
	cc.congestionLevel = math.Max(0.0, math.Min(1.0, cc.congestionLevel+variation))
	
	// Record in history
	cc.congestionHistory = append(cc.congestionHistory, cc.congestionLevel)
	if len(cc.congestionHistory) > 100 {
		cc.congestionHistory = cc.congestionHistory[1:]
	}
}

// GetCongestionLevel returns current congestion level.
func (cc *CongestionController) GetCongestionLevel() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.congestionLevel
}

// BandwidthEstimator estimates available and utilized bandwidth.
type BandwidthEstimator struct {
	estimatedBandwidth float64
	utilizedBandwidth  float64
	mu                 sync.RWMutex
}

// NewBandwidthEstimator creates a new bandwidth estimator.
func NewBandwidthEstimator() *BandwidthEstimator {
	return &BandwidthEstimator{
		estimatedBandwidth: 50.0, // Default 50 MB/s
		utilizedBandwidth:  25.0, // Default 25 MB/s utilized
	}
}

// Start begins bandwidth estimation.
func (be *BandwidthEstimator) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 3)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			be.updateBandwidthEstimates()
		}
	}
}

// updateBandwidthEstimates updates bandwidth estimates.
func (be *BandwidthEstimator) updateBandwidthEstimates() {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	// Simplified bandwidth estimation
	// In practice, this would use active probing, passive measurements, etc.
	
	// Simulate bandwidth variation
	baseVariation := math.Sin(float64(time.Now().Unix())/20.0) * 10.0
	be.estimatedBandwidth = math.Max(10.0, 50.0+baseVariation)
	
	// Utilization varies with time
	utilizationVariation := math.Cos(float64(time.Now().Unix())/15.0) * 10.0
	be.utilizedBandwidth = math.Max(5.0, math.Min(be.estimatedBandwidth*0.8, 25.0+utilizationVariation))
}

// GetEstimatedBandwidth returns estimated available bandwidth.
func (be *BandwidthEstimator) GetEstimatedBandwidth() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.estimatedBandwidth
}

// GetUtilizedBandwidth returns currently utilized bandwidth.
func (be *BandwidthEstimator) GetUtilizedBandwidth() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.utilizedBandwidth
}

// FlowController manages flow control algorithms.
type FlowController struct {
	config   *AdaptationConfig
	settings *FlowControlSettings
}

// NewFlowController creates a new flow controller.
func NewFlowController(config *AdaptationConfig) *FlowController {
	return &FlowController{
		config:   config,
		settings: DefaultFlowControlSettings(),
	}
}

// UtilizationHistory tracks bandwidth utilization history.
type UtilizationHistory struct {
	utilizations []*BandwidthUtilization
	maxHistory   int
	mu           sync.RWMutex
}

// NewUtilizationHistory creates a new utilization history tracker.
func NewUtilizationHistory() *UtilizationHistory {
	return &UtilizationHistory{
		utilizations: make([]*BandwidthUtilization, 0),
		maxHistory:   200,
	}
}

// RecordUtilization records a bandwidth utilization state.
func (uh *UtilizationHistory) RecordUtilization(util *BandwidthUtilization) {
	uh.mu.Lock()
	defer uh.mu.Unlock()
	
	uh.utilizations = append(uh.utilizations, util)
	
	// Limit history size
	if len(uh.utilizations) > uh.maxHistory {
		uh.utilizations = uh.utilizations[1:]
	}
}

// GetConfidenceBonus returns confidence bonus based on historical data.
func (uh *UtilizationHistory) GetConfidenceBonus() float64 {
	uh.mu.RLock()
	defer uh.mu.RUnlock()
	
	historyCount := len(uh.utilizations)
	if historyCount == 0 {
		return 0.0
	}
	
	// More history = higher confidence (up to 0.2 bonus)
	return math.Min(0.2, float64(historyCount)/100.0*0.2)
}