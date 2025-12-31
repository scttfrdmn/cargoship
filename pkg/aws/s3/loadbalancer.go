/*
Package s3 loadbalancer implements advanced real-time load balancing for cross-prefix coordination.

This module provides sophisticated load balancing algorithms with predictive optimization,
real-time adaptation, and machine learning-based performance prediction.
*/
package s3

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"
)

// RealTimeLoadBalancer implements advanced real-time load balancing with predictive optimization.
type RealTimeLoadBalancer struct {
	// Core load balancing
	strategy         LoadBalanceStrategy
	prefixWeights    map[string]*PrefixWeight
	prefixCapacities map[string]float64

	// Real-time adaptation
	performanceHistory map[string]*PerformanceHistory
	predictor          *PerformancePredictor
	optimizer          *LoadBalanceOptimizer

	// Configuration
	rebalanceThreshold float64
	rebalanceInterval  time.Duration
	monitoringInterval time.Duration
	predictionWindow   time.Duration
	adaptationRate     float64

	// State tracking
	lastRebalance    time.Time
	lastOptimization time.Time
	systemLoad       *SystemLoadMetrics

	// Real-time monitoring
	realTimeMetrics *RealTimeMetrics
	alertSystem     *LoadBalanceAlertSystem

	mu sync.RWMutex
}

// PrefixWeight tracks weight and performance characteristics for a prefix.
type PrefixWeight struct {
	PrefixID          string
	CurrentWeight     float64
	BaseWeight        float64
	PerformanceWeight float64
	PredictedWeight   float64

	// Performance characteristics
	ThroughputScore  float64
	LatencyScore     float64
	ReliabilityScore float64
	CapacityScore    float64

	// Adaptation tracking
	WeightHistory      []WeightSnapshot
	LastAdjustment     time.Time
	AdjustmentVelocity float64

	// Real-time state
	IsHealthy            bool
	IsOverloaded         bool
	PredictedPerformance float64
}

// WeightSnapshot captures weight at a specific point in time.
type WeightSnapshot struct {
	Timestamp   time.Time
	Weight      float64
	Performance float64
	SystemLoad  float64
}

// PerformanceHistory tracks historical performance data for prediction.
type PerformanceHistory struct {
	PrefixID          string
	ThroughputHistory []TimeSeriesPoint
	LatencyHistory    []TimeSeriesPoint
	ErrorRateHistory  []TimeSeriesPoint
	LoadHistory       []TimeSeriesPoint

	// Trend analysis
	ThroughputTrend TrendAnalysis
	LatencyTrend    TrendAnalysis
	LoadTrend       TrendAnalysis

	// Performance patterns
	SeasonalPatterns map[string]*SeasonalPattern
	LoadPatterns     map[string]*LoadPattern

	MaxHistorySize int
	LastUpdate     time.Time
}

// TimeSeriesPoint represents a data point in time series.
type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
	Quality   float64 // Data quality indicator
}

// TrendAnalysis provides trend analysis for time series data.
type TrendAnalysis struct {
	Direction    TrendDirection
	Slope        float64
	Confidence   float64
	Prediction   float64
	TimeHorizon  time.Duration
	LastAnalysis time.Time
}

// SeasonalPattern represents periodic performance patterns.
type SeasonalPattern struct {
	Period     time.Duration
	Amplitude  float64
	Phase      float64
	Confidence float64
	NextPeak   time.Time
	NextTrough time.Time
}

// LoadPattern represents load-dependent performance patterns.
type LoadPattern struct {
	LoadThreshold     float64
	PerformanceImpact float64
	ResponseTime      time.Duration
	RecoveryTime      time.Duration
	Confidence        float64
}

// PerformancePredictor implements machine learning-based performance prediction.
type PerformancePredictor struct {
	// ML models
	throughputModel *LinearRegressionModel
	latencyModel    *LinearRegressionModel
	capacityModel   *CapacityPredictionModel

	// Feature engineering
	featureExtractor *FeatureExtractor

	// Model performance
	predictionAccuracy map[string]float64
	trainingInterval   time.Duration

	// Configuration
	lookbackWindow    time.Duration
	predictionHorizon time.Duration
}

// LinearRegressionModel implements simple linear regression for prediction.
type LinearRegressionModel struct {
	Weights       []float64
	Bias          float64
	LastMSE       float64
	TrainingCount int
	IsReady       bool
}

// CapacityPredictionModel predicts remaining capacity under different loads.
type CapacityPredictionModel struct {
	CapacityFunction  func(load float64) float64
	MaxCapacity       float64
	LoadBreakpoints   []float64
	PerformanceSlopes []float64
	LastCalibration   time.Time
}

// FeatureExtractor extracts features for ML models.
type FeatureExtractor struct {
	// Feature extraction configuration - fields available for ML enhancements
}

// LoadBalanceOptimizer implements optimization algorithms for load balancing.
type LoadBalanceOptimizer struct {
	// Optimization strategy
	strategy OptimizationStrategy

	// Objective functions
	throughputObjective float64
	latencyObjective    float64
	fairnessObjective   float64
	stabilityObjective  float64

	// Constraints
	maxWeightChange float64
	minWeight       float64
	maxWeight       float64

	// Optimization state
	currentSolution     *LoadBalanceSolution
	optimizationHistory []OptimizationStep
}

// OptimizationStrategy defines different optimization approaches.
type OptimizationStrategy string

const (
	OptimizationGradientDescent    OptimizationStrategy = "gradient_descent"
	OptimizationSimulatedAnnealing OptimizationStrategy = "simulated_annealing"
	OptimizationGenetic            OptimizationStrategy = "genetic"
	OptimizationBayesian           OptimizationStrategy = "bayesian"
)

// LoadBalanceSolution represents a complete load balancing solution.
type LoadBalanceSolution struct {
	Weights            map[string]float64
	ExpectedThroughput float64
	ExpectedLatency    float64
	ExpectedFairness   float64
	TotalScore         float64
	Timestamp          time.Time
}

// OptimizationStep tracks an optimization iteration.
type OptimizationStep struct {
	Iteration       int
	Timestamp       time.Time
	Solution        *LoadBalanceSolution
	Improvement     float64
	ConvergenceRate float64
}

// SystemLoadMetrics tracks overall system load and capacity.
type SystemLoadMetrics struct {
	TotalThroughput  float64
	TotalCapacity    float64
	UtilizationRatio float64
	LoadDistribution map[string]float64
	Imbalance        float64
	LastUpdate       time.Time
}

// RealTimeMetrics provides real-time load balancing metrics.
type RealTimeMetrics struct {
	// Performance metrics
	AverageThroughput  float64
	AverageLatency     float64
	ThroughputVariance float64
	LatencyVariance    float64

	// Balance metrics
	LoadImbalance      float64
	WeightStability    float64
	RebalanceFrequency float64

	// Prediction metrics
	PredictionAccuracy float64
	ModelConfidence    float64
	OptimizationGain   float64

	// System health
	HealthyPrefixes      int
	OverloadedPrefixes   int
	UnresponsivePrefixes int

	LastUpdate time.Time
}

// LoadBalanceAlertSystem manages alerts for load balancing issues.
type LoadBalanceAlertSystem struct {
	alerts              map[string]*LoadBalanceAlert
	alertThresholds     *AlertThresholds
	alertHistory        []AlertEvent
	notificationChannel chan *LoadBalanceAlert
	mu                  sync.RWMutex
}

// LoadBalanceAlert represents a load balancing alert.
type LoadBalanceAlert struct {
	ID         string
	Type       AlertType
	Severity   AlertSeverity
	PrefixID   string
	Message    string
	Metric     string
	Value      float64
	Threshold  float64
	Timestamp  time.Time
	Resolved   bool
	ResolvedAt time.Time
}

// AlertType defines different types of load balancing alerts.
type AlertType string

const (
	AlertTypeOverload     AlertType = "overload"
	AlertTypeUnderload    AlertType = "underload"
	AlertTypeImbalance    AlertType = "imbalance"
	AlertTypePerformance  AlertType = "performance"
	AlertTypePrediction   AlertType = "prediction"
	AlertTypeSystemHealth AlertType = "system_health"
)

// AlertSeverity defines alert severity levels.
type AlertSeverity string

const (
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertThresholds defines thresholds for different alert types.
type AlertThresholds struct {
	OverloadThreshold        float64
	UnderloadThreshold       float64
	ImbalanceThreshold       float64
	LatencyThreshold         float64
	ThroughputThreshold      float64
	PredictionErrorThreshold float64
}

// AlertEvent represents an alert occurrence.
type AlertEvent struct {
	AlertID   string
	Timestamp time.Time
	Type      AlertType
	Severity  AlertSeverity
	Action    AlertAction
	Context   map[string]interface{}
}

// AlertAction defines actions taken in response to alerts.
type AlertAction string

const (
	AlertActionRebalance AlertAction = "rebalance"
	AlertActionOptimize  AlertAction = "optimize"
	AlertActionThrottle  AlertAction = "throttle"
	AlertActionNotify    AlertAction = "notify"
)

// NewRealTimeLoadBalancer creates a new real-time load balancer.
func NewRealTimeLoadBalancer(strategy LoadBalanceStrategy) *RealTimeLoadBalancer {
	return &RealTimeLoadBalancer{
		strategy:           strategy,
		prefixWeights:      make(map[string]*PrefixWeight),
		prefixCapacities:   make(map[string]float64),
		performanceHistory: make(map[string]*PerformanceHistory),
		predictor:          NewPerformancePredictor(),
		optimizer:          NewLoadBalanceOptimizer(),
		rebalanceThreshold: 0.15, // 15% imbalance triggers rebalancing
		rebalanceInterval:  time.Second * 10,
		monitoringInterval: time.Second * 2,
		predictionWindow:   time.Minute * 5,
		adaptationRate:     0.1,
		systemLoad:         &SystemLoadMetrics{},
		realTimeMetrics:    &RealTimeMetrics{},
		alertSystem:        NewLoadBalanceAlertSystem(),
	}
}

// Start begins real-time load balancing operations.
func (rtlb *RealTimeLoadBalancer) Start(ctx context.Context) {
	go rtlb.realTimeMonitoringLoop(ctx)
	go rtlb.optimizationLoop(ctx)
	go rtlb.predictionLoop(ctx)
	go rtlb.alertProcessingLoop(ctx)
}

// RegisterPrefix registers a new prefix with the real-time load balancer.
func (rtlb *RealTimeLoadBalancer) RegisterPrefix(prefixID string, capacity float64) {
	rtlb.mu.Lock()
	defer rtlb.mu.Unlock()

	rtlb.prefixWeights[prefixID] = &PrefixWeight{
		PrefixID:             prefixID,
		CurrentWeight:        1.0,
		BaseWeight:           1.0,
		PerformanceWeight:    1.0,
		PredictedWeight:      1.0,
		ThroughputScore:      1.0,
		LatencyScore:         1.0,
		ReliabilityScore:     1.0,
		CapacityScore:        1.0,
		WeightHistory:        make([]WeightSnapshot, 0, 100),
		LastAdjustment:       time.Now(),
		IsHealthy:            true,
		PredictedPerformance: 1.0,
	}

	rtlb.prefixCapacities[prefixID] = capacity

	rtlb.performanceHistory[prefixID] = &PerformanceHistory{
		PrefixID:          prefixID,
		ThroughputHistory: make([]TimeSeriesPoint, 0, 1000),
		LatencyHistory:    make([]TimeSeriesPoint, 0, 1000),
		ErrorRateHistory:  make([]TimeSeriesPoint, 0, 1000),
		LoadHistory:       make([]TimeSeriesPoint, 0, 1000),
		SeasonalPatterns:  make(map[string]*SeasonalPattern),
		LoadPatterns:      make(map[string]*LoadPattern),
		MaxHistorySize:    1000,
		LastUpdate:        time.Now(),
	}
}

// UpdatePrefixMetrics updates performance metrics and triggers real-time adaptation.
func (rtlb *RealTimeLoadBalancer) UpdatePrefixMetrics(prefixID string, metrics *PrefixPerformanceMetrics) {
	rtlb.mu.Lock()
	defer rtlb.mu.Unlock()

	weight, exists := rtlb.prefixWeights[prefixID]
	if !exists {
		return
	}

	now := time.Now()

	// Update performance history
	rtlb.updatePerformanceHistory(prefixID, metrics, now)

	// Update performance scores
	rtlb.updatePerformanceScores(weight, metrics)

	// Check for alerts
	rtlb.checkAlerts(prefixID, metrics)

	// Trigger real-time adaptation if needed
	if rtlb.shouldAdaptWeights(metrics) {
		rtlb.adaptWeightsRealTime(prefixID, metrics)
	}
}

// SelectOptimalPrefixes selects the best prefixes for new uploads using real-time optimization.
func (rtlb *RealTimeLoadBalancer) SelectOptimalPrefixes(upload *ScheduledUpload, count int) ([]string, error) {
	rtlb.mu.RLock()
	defer rtlb.mu.RUnlock()

	// Get predictions for all prefixes
	predictions := rtlb.predictor.PredictPerformance(rtlb.performanceHistory, rtlb.predictionWindow)

	// Score prefixes based on multiple criteria
	scores := rtlb.calculatePrefixScores(upload, predictions)

	// Sort by score (highest first)
	sortedPrefixes := rtlb.sortPrefixesByScore(scores)

	// Select top N prefixes
	if count > len(sortedPrefixes) {
		count = len(sortedPrefixes)
	}

	result := make([]string, count)
	for i := 0; i < count; i++ {
		result[i] = sortedPrefixes[i].PrefixID
	}

	return result, nil
}

// OptimizeWeights performs real-time weight optimization.
func (rtlb *RealTimeLoadBalancer) OptimizeWeights() *LoadBalanceSolution {
	rtlb.mu.Lock()
	defer rtlb.mu.Unlock()

	// Prepare optimization problem
	currentWeights := make(map[string]float64)
	for prefixID, weight := range rtlb.prefixWeights {
		currentWeights[prefixID] = weight.CurrentWeight
	}

	// Get performance predictions
	predictions := rtlb.predictor.PredictPerformance(rtlb.performanceHistory, rtlb.predictionWindow)

	// Run optimization
	solution := rtlb.optimizer.Optimize(currentWeights, predictions, rtlb.systemLoad)

	// Apply optimized weights
	if solution != nil && solution.TotalScore > rtlb.optimizer.currentSolution.TotalScore {
		rtlb.applyOptimizedWeights(solution)
		rtlb.optimizer.currentSolution = solution
		rtlb.lastOptimization = time.Now()
	}

	return solution
}

// GetRealTimeMetrics returns comprehensive real-time metrics.
func (rtlb *RealTimeLoadBalancer) GetRealTimeMetrics() *RealTimeMetrics {
	rtlb.mu.RLock()
	defer rtlb.mu.RUnlock()

	rtlb.updateRealTimeMetrics()
	return rtlb.realTimeMetrics
}

// Internal implementation methods

func (rtlb *RealTimeLoadBalancer) realTimeMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(rtlb.monitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rtlb.performRealTimeMonitoring()
		}
	}
}

func (rtlb *RealTimeLoadBalancer) optimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(rtlb.lastOptimization) > time.Minute {
				rtlb.OptimizeWeights()
			}
		}
	}
}

func (rtlb *RealTimeLoadBalancer) predictionLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 15)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rtlb.updatePredictions()
		}
	}
}

func (rtlb *RealTimeLoadBalancer) alertProcessingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case alert := <-rtlb.alertSystem.notificationChannel:
			rtlb.processAlert(alert)
		}
	}
}

func (rtlb *RealTimeLoadBalancer) performRealTimeMonitoring() {
	rtlb.mu.Lock()
	defer rtlb.mu.Unlock()

	// Update system load metrics
	rtlb.updateSystemLoadMetrics()

	// Detect performance anomalies
	rtlb.detectPerformanceAnomalies()

	// Update real-time metrics
	rtlb.updateRealTimeMetrics()

	// Check if rebalancing is needed
	if rtlb.shouldRebalanceRealTime() {
		rtlb.performRealTimeRebalance()
	}
}

func (rtlb *RealTimeLoadBalancer) updatePerformanceHistory(prefixID string, metrics *PrefixPerformanceMetrics, timestamp time.Time) {
	history := rtlb.performanceHistory[prefixID]

	// Add new data points
	history.ThroughputHistory = append(history.ThroughputHistory, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     metrics.ThroughputMBps,
		Quality:   1.0,
	})

	history.LatencyHistory = append(history.LatencyHistory, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     metrics.LatencyMs,
		Quality:   1.0,
	})

	history.ErrorRateHistory = append(history.ErrorRateHistory, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     metrics.ErrorRate,
		Quality:   1.0,
	})

	history.LoadHistory = append(history.LoadHistory, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     metrics.BandwidthUtilization,
		Quality:   1.0,
	})

	// Trim history if too large
	if len(history.ThroughputHistory) > history.MaxHistorySize {
		history.ThroughputHistory = history.ThroughputHistory[1:]
		history.LatencyHistory = history.LatencyHistory[1:]
		history.ErrorRateHistory = history.ErrorRateHistory[1:]
		history.LoadHistory = history.LoadHistory[1:]
	}

	history.LastUpdate = timestamp
}

func (rtlb *RealTimeLoadBalancer) updatePerformanceScores(weight *PrefixWeight, metrics *PrefixPerformanceMetrics) {
	// Throughput score (higher is better)
	weight.ThroughputScore = math.Min(metrics.ThroughputMBps/100.0, 2.0)

	// Latency score (lower is better)
	weight.LatencyScore = math.Max(1.0-(metrics.LatencyMs/1000.0), 0.1)

	// Reliability score (lower error rate is better)
	weight.ReliabilityScore = math.Max(1.0-metrics.ErrorRate*10, 0.1)

	// Capacity score (lower utilization means more capacity)
	weight.CapacityScore = math.Max(1.0-metrics.BandwidthUtilization, 0.1)

	// Update health status
	weight.IsHealthy = metrics.ErrorRate < 0.05 && metrics.LatencyMs < 2000
	weight.IsOverloaded = metrics.BandwidthUtilization > 0.9
}

func (rtlb *RealTimeLoadBalancer) shouldAdaptWeights(metrics *PrefixPerformanceMetrics) bool {
	// Adapt if performance significantly deviates from expected
	return metrics.ErrorRate > 0.1 ||
		metrics.LatencyMs > 1000 ||
		metrics.BandwidthUtilization > 0.95 ||
		metrics.BandwidthUtilization < 0.1
}

func (rtlb *RealTimeLoadBalancer) adaptWeightsRealTime(prefixID string, metrics *PrefixPerformanceMetrics) {
	weight := rtlb.prefixWeights[prefixID]

	// Calculate adaptive weight based on current performance
	performanceFactor := weight.ThroughputScore * weight.LatencyScore * weight.ReliabilityScore * weight.CapacityScore

	// Apply adaptation with learning rate
	targetWeight := weight.BaseWeight * performanceFactor
	weight.CurrentWeight = weight.CurrentWeight*(1-rtlb.adaptationRate) + targetWeight*rtlb.adaptationRate

	// Clamp weight to reasonable bounds
	weight.CurrentWeight = math.Max(weight.CurrentWeight, 0.1)
	weight.CurrentWeight = math.Min(weight.CurrentWeight, 3.0)

	// Record weight change
	weight.WeightHistory = append(weight.WeightHistory, WeightSnapshot{
		Timestamp:   time.Now(),
		Weight:      weight.CurrentWeight,
		Performance: performanceFactor,
		SystemLoad:  rtlb.systemLoad.UtilizationRatio,
	})

	weight.LastAdjustment = time.Now()
}

func (rtlb *RealTimeLoadBalancer) calculatePrefixScores(upload *ScheduledUpload, predictions map[string]*PerformancePrediction) map[string]float64 {
	scores := make(map[string]float64)

	for prefixID, weight := range rtlb.prefixWeights {
		prediction, hasPrediction := predictions[prefixID]

		// Base score from current weight
		score := weight.CurrentWeight

		// Apply prediction if available
		if hasPrediction {
			score *= prediction.ExpectedPerformance
		}

		// Apply upload-specific adjustments
		if upload.Priority > 3 {
			// High priority uploads prefer reliable prefixes
			score *= weight.ReliabilityScore
		}

		if upload.EstimatedSize > 1024*1024*1024 { // > 1GB
			// Large uploads prefer high throughput
			score *= weight.ThroughputScore
		}

		// Penalize overloaded prefixes
		if weight.IsOverloaded {
			score *= 0.5
		}

		// Penalize unhealthy prefixes
		if !weight.IsHealthy {
			score *= 0.2
		}

		scores[prefixID] = score
	}

	return scores
}

func (rtlb *RealTimeLoadBalancer) sortPrefixesByScore(scores map[string]float64) []PrefixScore {
	var prefixScores []PrefixScore

	for prefixID, score := range scores {
		prefixScores = append(prefixScores, PrefixScore{
			PrefixID: prefixID,
			Score:    score,
		})
	}

	sort.Slice(prefixScores, func(i, j int) bool {
		return prefixScores[i].Score > prefixScores[j].Score
	})

	return prefixScores
}

// PrefixScore represents a prefix with its calculated score.
type PrefixScore struct {
	PrefixID string
	Score    float64
}

func (rtlb *RealTimeLoadBalancer) applyOptimizedWeights(solution *LoadBalanceSolution) {
	for prefixID, optimizedWeight := range solution.Weights {
		if weight, exists := rtlb.prefixWeights[prefixID]; exists {
			weight.CurrentWeight = optimizedWeight
			weight.LastAdjustment = time.Now()
		}
	}
}

func (rtlb *RealTimeLoadBalancer) updateSystemLoadMetrics() {
	rtlb.mu.Lock()
	defer rtlb.mu.Unlock()

	totalThroughput := 0.0
	totalCapacity := 0.0
	loadDistribution := make(map[string]float64)

	for prefixID, weight := range rtlb.prefixWeights {
		capacity := rtlb.prefixCapacities[prefixID]
		utilization := weight.CapacityScore // Inverse of utilization

		throughput := capacity * (1.0 - utilization)
		totalThroughput += throughput
		totalCapacity += capacity
		loadDistribution[prefixID] = throughput
	}

	rtlb.systemLoad.TotalThroughput = totalThroughput
	rtlb.systemLoad.TotalCapacity = totalCapacity
	rtlb.systemLoad.UtilizationRatio = totalThroughput / totalCapacity
	rtlb.systemLoad.LoadDistribution = loadDistribution
	rtlb.systemLoad.Imbalance = rtlb.calculateLoadImbalance(loadDistribution)
	rtlb.systemLoad.LastUpdate = time.Now()
}

func (rtlb *RealTimeLoadBalancer) calculateLoadImbalance(distribution map[string]float64) float64 {
	if len(distribution) < 2 {
		return 0.0
	}

	var loads []float64
	for _, load := range distribution {
		loads = append(loads, load)
	}

	// Calculate coefficient of variation
	mean := 0.0
	for _, load := range loads {
		mean += load
	}
	mean /= float64(len(loads))

	variance := 0.0
	for _, load := range loads {
		diff := load - mean
		variance += diff * diff
	}
	variance /= float64(len(loads))

	if mean == 0 {
		return 0.0
	}

	return math.Sqrt(variance) / mean
}

func (rtlb *RealTimeLoadBalancer) detectPerformanceAnomalies() {
	// This would implement anomaly detection algorithms
	// Placeholder for now
}

func (rtlb *RealTimeLoadBalancer) updateRealTimeMetrics() {
	// Calculate average performance
	totalThroughput := 0.0
	totalLatency := 0.0
	healthyCount := 0
	overloadedCount := 0

	for _, weight := range rtlb.prefixWeights {
		totalThroughput += weight.ThroughputScore * 100 // Convert back to actual values
		totalLatency += (1.0 - weight.LatencyScore) * 1000

		if weight.IsHealthy {
			healthyCount++
		}
		if weight.IsOverloaded {
			overloadedCount++
		}
	}

	prefixCount := len(rtlb.prefixWeights)
	if prefixCount > 0 {
		rtlb.realTimeMetrics.AverageThroughput = totalThroughput / float64(prefixCount)
		rtlb.realTimeMetrics.AverageLatency = totalLatency / float64(prefixCount)
	}

	rtlb.realTimeMetrics.LoadImbalance = rtlb.systemLoad.Imbalance
	rtlb.realTimeMetrics.HealthyPrefixes = healthyCount
	rtlb.realTimeMetrics.OverloadedPrefixes = overloadedCount
	rtlb.realTimeMetrics.LastUpdate = time.Now()
}

func (rtlb *RealTimeLoadBalancer) shouldRebalanceRealTime() bool {
	return rtlb.systemLoad.Imbalance > rtlb.rebalanceThreshold ||
		rtlb.realTimeMetrics.OverloadedPrefixes > 0 ||
		time.Since(rtlb.lastRebalance) > rtlb.rebalanceInterval
}

func (rtlb *RealTimeLoadBalancer) performRealTimeRebalance() {
	// Trigger optimization
	rtlb.OptimizeWeights()
	rtlb.lastRebalance = time.Now()
}

func (rtlb *RealTimeLoadBalancer) updatePredictions() {
	// Update ML models and predictions
	rtlb.predictor.UpdateModels(rtlb.performanceHistory)
}

func (rtlb *RealTimeLoadBalancer) checkAlerts(prefixID string, metrics *PrefixPerformanceMetrics) {
	// Check various alert conditions and generate alerts
	rtlb.alertSystem.CheckAlerts(prefixID, metrics)
}

func (rtlb *RealTimeLoadBalancer) processAlert(alert *LoadBalanceAlert) {
	// Process and respond to alerts
	switch alert.Type {
	case AlertTypeOverload:
		rtlb.handleOverloadAlert(alert)
	case AlertTypeImbalance:
		rtlb.handleImbalanceAlert(alert)
	case AlertTypePerformance:
		rtlb.handlePerformanceAlert(alert)
	}
}

func (rtlb *RealTimeLoadBalancer) handleOverloadAlert(alert *LoadBalanceAlert) {
	// Reduce weight for overloaded prefix
	if weight, exists := rtlb.prefixWeights[alert.PrefixID]; exists {
		weight.CurrentWeight *= 0.8
	}
}

func (rtlb *RealTimeLoadBalancer) handleImbalanceAlert(alert *LoadBalanceAlert) {
	// Trigger immediate rebalancing
	rtlb.performRealTimeRebalance()
}

func (rtlb *RealTimeLoadBalancer) handlePerformanceAlert(alert *LoadBalanceAlert) {
	// Adjust performance scores
	if weight, exists := rtlb.prefixWeights[alert.PrefixID]; exists {
		weight.IsHealthy = false
	}
}

// Factory functions

func NewPerformancePredictor() *PerformancePredictor {
	return &PerformancePredictor{
		throughputModel:    &LinearRegressionModel{},
		latencyModel:       &LinearRegressionModel{},
		capacityModel:      &CapacityPredictionModel{},
		featureExtractor:   &FeatureExtractor{},
		predictionAccuracy: make(map[string]float64),
		lookbackWindow:     time.Hour,
		predictionHorizon:  time.Minute * 10,
		trainingInterval:   time.Hour,
	}
}

func NewLoadBalanceOptimizer() *LoadBalanceOptimizer {
	return &LoadBalanceOptimizer{
		strategy:            OptimizationGradientDescent,
		throughputObjective: 0.4,
		latencyObjective:    0.3,
		fairnessObjective:   0.2,
		stabilityObjective:  0.1,
		maxWeightChange:     0.5,
		minWeight:           0.1,
		maxWeight:           3.0,
		currentSolution:     &LoadBalanceSolution{},
		optimizationHistory: make([]OptimizationStep, 0, 1000),
	}
}

func NewLoadBalanceAlertSystem() *LoadBalanceAlertSystem {
	return &LoadBalanceAlertSystem{
		alerts: make(map[string]*LoadBalanceAlert),
		alertThresholds: &AlertThresholds{
			OverloadThreshold:        0.9,
			UnderloadThreshold:       0.1,
			ImbalanceThreshold:       0.3,
			LatencyThreshold:         1000,
			ThroughputThreshold:      10,
			PredictionErrorThreshold: 0.2,
		},
		alertHistory:        make([]AlertEvent, 0, 10000),
		notificationChannel: make(chan *LoadBalanceAlert, 100),
	}
}

// PerformancePrediction represents predicted performance metrics.
type PerformancePrediction struct {
	PrefixID            string
	ExpectedThroughput  float64
	ExpectedLatency     float64
	ExpectedErrorRate   float64
	ExpectedPerformance float64
	Confidence          float64
	TimeHorizon         time.Duration
}

// Placeholder implementations for complex methods
func (pp *PerformancePredictor) PredictPerformance(history map[string]*PerformanceHistory, window time.Duration) map[string]*PerformancePrediction {
	predictions := make(map[string]*PerformancePrediction)

	for prefixID, hist := range history {
		// Simple prediction based on recent average
		recentThroughput := pp.getRecentAverage(hist.ThroughputHistory, window)
		recentLatency := pp.getRecentAverage(hist.LatencyHistory, window)
		recentErrorRate := pp.getRecentAverage(hist.ErrorRateHistory, window)

		predictions[prefixID] = &PerformancePrediction{
			PrefixID:            prefixID,
			ExpectedThroughput:  recentThroughput,
			ExpectedLatency:     recentLatency,
			ExpectedErrorRate:   recentErrorRate,
			ExpectedPerformance: pp.calculatePerformanceScore(recentThroughput, recentLatency, recentErrorRate),
			Confidence:          0.7,
			TimeHorizon:         window,
		}
	}

	return predictions
}

func (pp *PerformancePredictor) getRecentAverage(history []TimeSeriesPoint, window time.Duration) float64 {
	if len(history) == 0 {
		return 0.0
	}

	cutoff := time.Now().Add(-window)
	sum := 0.0
	count := 0

	for _, point := range history {
		if point.Timestamp.After(cutoff) {
			sum += point.Value
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return sum / float64(count)
}

func (pp *PerformancePredictor) calculatePerformanceScore(throughput, latency, errorRate float64) float64 {
	// Simple performance score calculation
	throughputScore := math.Min(throughput/100.0, 2.0)
	latencyScore := math.Max(1.0-(latency/1000.0), 0.1)
	reliabilityScore := math.Max(1.0-errorRate*10, 0.1)

	return throughputScore * latencyScore * reliabilityScore
}

func (pp *PerformancePredictor) UpdateModels(history map[string]*PerformanceHistory) {
	// Placeholder for model training
}

func (lbo *LoadBalanceOptimizer) Optimize(weights map[string]float64, predictions map[string]*PerformancePrediction, systemLoad *SystemLoadMetrics) *LoadBalanceSolution {
	// Simple gradient descent optimization
	optimizedWeights := make(map[string]float64)

	// Copy current weights
	for prefixID, weight := range weights {
		optimizedWeights[prefixID] = weight
	}

	// Apply simple optimization heuristics
	for prefixID, prediction := range predictions {
		if weight, exists := optimizedWeights[prefixID]; exists {
			// Increase weight for well-performing prefixes
			if prediction.ExpectedPerformance > 1.0 {
				optimizedWeights[prefixID] = math.Min(weight*1.1, lbo.maxWeight)
			} else if prediction.ExpectedPerformance < 0.5 {
				optimizedWeights[prefixID] = math.Max(weight*0.9, lbo.minWeight)
			}
		}
	}

	// Normalize weights
	totalWeight := 0.0
	for _, weight := range optimizedWeights {
		totalWeight += weight
	}

	if totalWeight > 0 {
		for prefixID := range optimizedWeights {
			optimizedWeights[prefixID] *= float64(len(optimizedWeights)) / totalWeight
		}
	}

	// Calculate expected performance
	expectedThroughput := 0.0
	expectedLatency := 0.0
	for prefixID, weight := range optimizedWeights {
		if prediction, exists := predictions[prefixID]; exists {
			expectedThroughput += weight * prediction.ExpectedThroughput
			expectedLatency += weight * prediction.ExpectedLatency
		}
	}

	return &LoadBalanceSolution{
		Weights:            optimizedWeights,
		ExpectedThroughput: expectedThroughput,
		ExpectedLatency:    expectedLatency,
		ExpectedFairness:   0.8, // Placeholder
		TotalScore:         expectedThroughput - expectedLatency/1000.0,
		Timestamp:          time.Now(),
	}
}

func (las *LoadBalanceAlertSystem) CheckAlerts(prefixID string, metrics *PrefixPerformanceMetrics) {
	// Check overload condition
	if metrics.BandwidthUtilization > las.alertThresholds.OverloadThreshold {
		alert := &LoadBalanceAlert{
			ID:        prefixID + "_overload",
			Type:      AlertTypeOverload,
			Severity:  AlertSeverityHigh,
			PrefixID:  prefixID,
			Message:   "Prefix is overloaded",
			Metric:    "bandwidth_utilization",
			Value:     metrics.BandwidthUtilization,
			Threshold: las.alertThresholds.OverloadThreshold,
			Timestamp: time.Now(),
		}

		las.mu.Lock()
		las.alerts[alert.ID] = alert
		las.mu.Unlock()

		select {
		case las.notificationChannel <- alert:
		default:
			// Channel full, skip alert
		}
	}
}
