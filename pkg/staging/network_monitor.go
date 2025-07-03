package staging

import (
	"context"
	"math"
	"sync"
	"time"
)

// NewNetworkConditionMonitor creates a new network condition monitor.
func NewNetworkConditionMonitor(config *StagingConfig) *NetworkConditionMonitor {
	return &NetworkConditionMonitor{
		currentCondition:  NewDefaultNetworkCondition(),
		conditionHistory:  make([]*NetworkCondition, 0, 100),
		trendAnalyzer:     NewNetworkTrendAnalyzer(),
		predictor:         NewNetworkPredictor(config),
		updateInterval:    time.Second * 5,  // Update every 5 seconds
		predictionWindow:  config.NetworkPredictionWindow,
	}
}

// Start begins network condition monitoring.
func (ncm *NetworkConditionMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(ncm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ncm.updateConditions()
		}
	}
}

// GetCurrentCondition returns the current network condition.
func (ncm *NetworkConditionMonitor) GetCurrentCondition() *NetworkCondition {
	ncm.mu.RLock()
	defer ncm.mu.RUnlock()
	
	// Return a copy to prevent race conditions
	condition := *ncm.currentCondition
	return &condition
}

// UpdatePredictions updates network trend predictions.
func (ncm *NetworkConditionMonitor) UpdatePredictions() {
	ncm.mu.Lock()
	defer ncm.mu.Unlock()
	
	if len(ncm.conditionHistory) < 3 {
		return // Need at least 3 data points for prediction
	}
	
	// Analyze trends
	trend := ncm.trendAnalyzer.AnalyzeTrend(ncm.conditionHistory)
	
	// Update prediction
	prediction := ncm.predictor.PredictCondition(ncm.conditionHistory, ncm.predictionWindow)
	
	// Update current condition with prediction
	ncm.currentCondition.PredictedTrend = trend
	ncm.currentCondition.Reliability = prediction.Reliability
}

// RecordTransferMetrics records actual transfer metrics for learning.
func (ncm *NetworkConditionMonitor) RecordTransferMetrics(throughputMBps, latencyMs float64) {
	ncm.mu.Lock()
	defer ncm.mu.Unlock()
	
	// Update current condition with observed metrics
	ncm.currentCondition.BandwidthMBps = throughputMBps
	ncm.currentCondition.LatencyMs = latencyMs
	ncm.currentCondition.Timestamp = time.Now()
	
	// Store in history
	condition := *ncm.currentCondition
	ncm.addToHistory(&condition)
}

// updateConditions updates network conditions through measurement.
func (ncm *NetworkConditionMonitor) updateConditions() {
	ncm.mu.Lock()
	defer ncm.mu.Unlock()
	
	// Measure current network conditions
	condition := ncm.measureNetworkCondition()
	
	// Store in history
	ncm.addToHistory(condition)
	
	// Update current condition
	ncm.currentCondition = condition
}

// measureNetworkCondition measures current network performance.
func (ncm *NetworkConditionMonitor) measureNetworkCondition() *NetworkCondition {
	// In a real implementation, this would perform actual network measurements
	// For now, we'll simulate based on historical data and trends
	
	condition := &NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   ncm.estimateBandwidth(),
		LatencyMs:       ncm.estimateLatency(),
		PacketLoss:      ncm.estimatePacketLoss(),
		Jitter:          ncm.estimateJitter(),
		CongestionLevel: ncm.estimateCongestion(),
		Reliability:     ncm.estimateReliability(),
		PredictedTrend:  TrendUnknown,
	}
	
	return condition
}

// estimateBandwidth estimates available bandwidth.
func (ncm *NetworkConditionMonitor) estimateBandwidth() float64 {
	if len(ncm.conditionHistory) == 0 {
		return 50.0 // Default 50 MB/s
	}
	
	// Calculate moving average of recent bandwidth
	recentCount := min(len(ncm.conditionHistory), 10)
	totalBandwidth := 0.0
	
	for i := len(ncm.conditionHistory) - recentCount; i < len(ncm.conditionHistory); i++ {
		totalBandwidth += ncm.conditionHistory[i].BandwidthMBps
	}
	
	return totalBandwidth / float64(recentCount)
}

// estimateLatency estimates network latency.
func (ncm *NetworkConditionMonitor) estimateLatency() float64 {
	if len(ncm.conditionHistory) == 0 {
		return 20.0 // Default 20ms
	}
	
	// Calculate moving average of recent latency
	recentCount := min(len(ncm.conditionHistory), 10)
	totalLatency := 0.0
	
	for i := len(ncm.conditionHistory) - recentCount; i < len(ncm.conditionHistory); i++ {
		totalLatency += ncm.conditionHistory[i].LatencyMs
	}
	
	return totalLatency / float64(recentCount)
}

// estimatePacketLoss estimates packet loss rate.
func (ncm *NetworkConditionMonitor) estimatePacketLoss() float64 {
	// Simplified estimation - in practice would measure actual packet loss
	if len(ncm.conditionHistory) == 0 {
		return 0.001 // Default 0.1% packet loss
	}
	
	// Calculate based on bandwidth variability
	bandwidth := ncm.estimateBandwidth()
	if bandwidth < 10.0 {
		return 0.05 // 5% loss for poor connections
	} else if bandwidth < 50.0 {
		return 0.01 // 1% loss for moderate connections
	}
	
	return 0.001 // 0.1% loss for good connections
}

// estimateJitter estimates network jitter.
func (ncm *NetworkConditionMonitor) estimateJitter() float64 {
	if len(ncm.conditionHistory) < 2 {
		return 2.0 // Default 2ms jitter
	}
	
	// Calculate latency variation
	recentCount := min(len(ncm.conditionHistory), 10)
	latencies := make([]float64, 0, recentCount)
	
	for i := len(ncm.conditionHistory) - recentCount; i < len(ncm.conditionHistory); i++ {
		latencies = append(latencies, ncm.conditionHistory[i].LatencyMs)
	}
	
	// Calculate standard deviation
	mean := 0.0
	for _, latency := range latencies {
		mean += latency
	}
	mean /= float64(len(latencies))
	
	variance := 0.0
	for _, latency := range latencies {
		diff := latency - mean
		variance += diff * diff
	}
	variance /= float64(len(latencies))
	
	return math.Sqrt(variance)
}

// estimateCongestion estimates network congestion level.
func (ncm *NetworkConditionMonitor) estimateCongestion() float64 {
	bandwidth := ncm.estimateBandwidth()
	latency := ncm.estimateLatency()
	packetLoss := ncm.estimatePacketLoss()
	
	// Combine metrics to estimate congestion
	congestion := 0.0
	
	// Low bandwidth indicates congestion
	if bandwidth < 20.0 {
		congestion += 0.4
	} else if bandwidth < 50.0 {
		congestion += 0.2
	}
	
	// High latency indicates congestion
	if latency > 100.0 {
		congestion += 0.3
	} else if latency > 50.0 {
		congestion += 0.1
	}
	
	// Packet loss indicates congestion
	if packetLoss > 0.01 {
		congestion += 0.3
	} else if packetLoss > 0.005 {
		congestion += 0.1
	}
	
	return math.Min(congestion, 1.0)
}

// estimateReliability estimates connection reliability.
func (ncm *NetworkConditionMonitor) estimateReliability() float64 {
	congestion := ncm.estimateCongestion()
	packetLoss := ncm.estimatePacketLoss()
	
	// Base reliability
	reliability := 1.0
	
	// Reduce reliability based on congestion
	reliability -= congestion * 0.3
	
	// Reduce reliability based on packet loss
	reliability -= packetLoss * 10.0 // 1% packet loss = 10% reliability reduction
	
	return math.Max(reliability, 0.1) // Minimum 10% reliability
}

// addToHistory adds a condition to the history with size management.
func (ncm *NetworkConditionMonitor) addToHistory(condition *NetworkCondition) {
	ncm.conditionHistory = append(ncm.conditionHistory, condition)
	
	// Limit history size
	maxHistory := 100
	if len(ncm.conditionHistory) > maxHistory {
		// Remove oldest entries
		copy(ncm.conditionHistory, ncm.conditionHistory[len(ncm.conditionHistory)-maxHistory:])
		ncm.conditionHistory = ncm.conditionHistory[:maxHistory]
	}
}

// NewDefaultNetworkCondition creates a default network condition.
func NewDefaultNetworkCondition() *NetworkCondition {
	return &NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   50.0,  // Default 50 MB/s
		LatencyMs:       20.0,  // Default 20ms
		PacketLoss:      0.001, // Default 0.1%
		Jitter:          2.0,   // Default 2ms
		CongestionLevel: 0.1,   // Default 10% congestion
		Reliability:     0.9,   // Default 90% reliability
		PredictedTrend:  TrendStable,
	}
}

// NetworkTrendAnalyzer analyzes network condition trends.
type NetworkTrendAnalyzer struct {
	windowSize int
	mu         sync.RWMutex
}

// NewNetworkTrendAnalyzer creates a new trend analyzer.
func NewNetworkTrendAnalyzer() *NetworkTrendAnalyzer {
	return &NetworkTrendAnalyzer{
		windowSize: 10, // Analyze last 10 measurements
	}
}

// AnalyzeTrend analyzes the trend in network conditions.
func (nta *NetworkTrendAnalyzer) AnalyzeTrend(history []*NetworkCondition) NetworkTrend {
	nta.mu.RLock()
	defer nta.mu.RUnlock()
	
	if len(history) < 3 {
		return TrendUnknown
	}
	
	// Analyze recent data
	windowSize := min(len(history), nta.windowSize)
	recent := history[len(history)-windowSize:]
	
	// Calculate trends for key metrics
	bandwidthTrend := nta.calculateMetricTrend(recent, func(c *NetworkCondition) float64 {
		return c.BandwidthMBps
	})
	
	latencyTrend := nta.calculateMetricTrend(recent, func(c *NetworkCondition) float64 {
		return -c.LatencyMs // Invert so improvement is positive
	})
	
	congestionTrend := nta.calculateMetricTrend(recent, func(c *NetworkCondition) float64 {
		return -c.CongestionLevel // Invert so improvement is positive
	})
	
	// Combine trends
	overallTrend := (bandwidthTrend + latencyTrend + congestionTrend) / 3.0
	
	// Classify trend
	if overallTrend > 0.1 {
		return TrendImproving
	} else if overallTrend < -0.1 {
		return TrendDegrading
	}
	
	// Check for volatility
	volatility := nta.calculateVolatility(recent)
	if volatility > 0.5 {
		return TrendVolatile
	}
	
	return TrendStable
}

// calculateMetricTrend calculates the trend for a specific metric.
func (nta *NetworkTrendAnalyzer) calculateMetricTrend(conditions []*NetworkCondition, metric func(*NetworkCondition) float64) float64 {
	if len(conditions) < 2 {
		return 0.0
	}
	
	// Calculate linear regression slope
	n := float64(len(conditions))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0
	
	for i, condition := range conditions {
		x := float64(i)
		y := metric(condition)
		
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	// Calculate slope (trend)
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0.0
	}
	
	slope := (n*sumXY - sumX*sumY) / denominator
	
	// Normalize slope
	if sumY != 0 {
		return slope / (sumY / n) // Normalize by average value
	}
	
	return slope
}

// calculateVolatility calculates the volatility of network conditions.
func (nta *NetworkTrendAnalyzer) calculateVolatility(conditions []*NetworkCondition) float64 {
	if len(conditions) < 2 {
		return 0.0
	}
	
	// Calculate coefficient of variation for bandwidth
	bandwidths := make([]float64, len(conditions))
	for i, condition := range conditions {
		bandwidths[i] = condition.BandwidthMBps
	}
	
	mean := 0.0
	for _, bandwidth := range bandwidths {
		mean += bandwidth
	}
	mean /= float64(len(bandwidths))
	
	if mean == 0 {
		return 0.0
	}
	
	variance := 0.0
	for _, bandwidth := range bandwidths {
		diff := bandwidth - mean
		variance += diff * diff
	}
	variance /= float64(len(bandwidths))
	
	stdDev := math.Sqrt(variance)
	
	// Coefficient of variation
	return stdDev / mean
}

// NetworkPredictor predicts future network conditions.
type NetworkPredictor struct {
	config           *StagingConfig
	predictionModels map[string]*PredictionModel
	mu               sync.RWMutex
}

// NewNetworkPredictor creates a new network predictor.
func NewNetworkPredictor(config *StagingConfig) *NetworkPredictor {
	return &NetworkPredictor{
		config: config,
		predictionModels: map[string]*PredictionModel{
			"bandwidth":  NewPredictionModel("bandwidth"),
			"latency":    NewPredictionModel("latency"),
			"congestion": NewPredictionModel("congestion"),
		},
	}
}

// PredictCondition predicts network condition for the given time window.
func (np *NetworkPredictor) PredictCondition(history []*NetworkCondition, window time.Duration) *NetworkCondition {
	np.mu.RLock()
	defer np.mu.RUnlock()
	
	if len(history) == 0 {
		return NewDefaultNetworkCondition()
	}
	
	// Get the latest condition as baseline
	latest := history[len(history)-1]
	prediction := *latest
	prediction.Timestamp = latest.Timestamp.Add(window)
	
	// Predict bandwidth
	bandwidthPrediction := np.predictionModels["bandwidth"].Predict(history, window)
	if bandwidthPrediction.Value > 0 {
		prediction.BandwidthMBps = bandwidthPrediction.Value
		prediction.Reliability = math.Min(prediction.Reliability, bandwidthPrediction.Confidence)
	}
	
	// Predict latency
	latencyPrediction := np.predictionModels["latency"].Predict(history, window)
	if latencyPrediction.Value > 0 {
		prediction.LatencyMs = latencyPrediction.Value
		prediction.Reliability = math.Min(prediction.Reliability, latencyPrediction.Confidence)
	}
	
	// Predict congestion
	congestionPrediction := np.predictionModels["congestion"].Predict(history, window)
	if congestionPrediction.Value >= 0 {
		prediction.CongestionLevel = congestionPrediction.Value
		prediction.Reliability = math.Min(prediction.Reliability, congestionPrediction.Confidence)
	}
	
	// Update derived metrics
	prediction.PacketLoss = math.Max(0.001, prediction.CongestionLevel*0.1)
	prediction.Jitter = math.Max(1.0, prediction.LatencyMs*0.1)
	
	return &prediction
}

// PredictionModel represents a model for predicting a specific network metric.
type PredictionModel struct {
	metricName string
	mu         sync.RWMutex
}

// NewPredictionModel creates a new prediction model.
func NewPredictionModel(metricName string) *PredictionModel {
	return &PredictionModel{
		metricName: metricName,
	}
}

// Predict predicts the value of the metric for the given time window.
func (pm *PredictionModel) Predict(history []*NetworkCondition, window time.Duration) *MetricPrediction {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if len(history) < 2 {
		return &MetricPrediction{
			Value:      0,
			Confidence: 0.5,
			Timestamp:  time.Now().Add(window),
		}
	}
	
	// Extract metric values
	values := pm.extractMetricValues(history)
	
	// Simple linear extrapolation
	trend := pm.calculateTrend(values)
	latest := values[len(values)-1]
	
	// Predict value based on trend
	futureValue := latest + trend*float64(window.Seconds())/60.0 // Per minute trend
	
	// Calculate confidence based on trend stability
	confidence := pm.calculateConfidence(values, trend)
	
	return &MetricPrediction{
		Value:      math.Max(0, futureValue), // Ensure non-negative
		Confidence: confidence,
		Timestamp:  time.Now().Add(window),
	}
}

// extractMetricValues extracts the relevant metric values from history.
func (pm *PredictionModel) extractMetricValues(history []*NetworkCondition) []float64 {
	values := make([]float64, len(history))
	
	for i, condition := range history {
		switch pm.metricName {
		case "bandwidth":
			values[i] = condition.BandwidthMBps
		case "latency":
			values[i] = condition.LatencyMs
		case "congestion":
			values[i] = condition.CongestionLevel
		default:
			values[i] = 0
		}
	}
	
	return values
}

// calculateTrend calculates the trend in metric values.
func (pm *PredictionModel) calculateTrend(values []float64) float64 {
	if len(values) < 2 {
		return 0.0
	}
	
	// Simple moving average trend
	windowSize := min(len(values), 5)
	if windowSize < 2 {
		return 0.0
	}
	
	recent := values[len(values)-windowSize:]
	
	// Calculate average change
	totalChange := 0.0
	for i := 1; i < len(recent); i++ {
		totalChange += recent[i] - recent[i-1]
	}
	
	return totalChange / float64(len(recent)-1)
}

// calculateConfidence calculates prediction confidence based on data stability.
func (pm *PredictionModel) calculateConfidence(values []float64, trend float64) float64 {
	if len(values) < 3 {
		return 0.5 // Low confidence with limited data
	}
	
	// Calculate variance in recent values
	windowSize := min(len(values), 10)
	recent := values[len(values)-windowSize:]
	
	mean := 0.0
	for _, value := range recent {
		mean += value
	}
	mean /= float64(len(recent))
	
	if mean == 0 {
		return 0.5
	}
	
	variance := 0.0
	for _, value := range recent {
		diff := value - mean
		variance += diff * diff
	}
	variance /= float64(len(recent))
	
	// Coefficient of variation
	cv := math.Sqrt(variance) / mean
	
	// Higher stability = higher confidence
	confidence := 1.0 - math.Min(cv, 1.0)
	
	// Boost confidence if trend is consistent
	if math.Abs(trend) < mean*0.1 { // Trend is less than 10% of mean
		confidence += 0.1
	}
	
	return math.Min(math.Max(confidence, 0.1), 0.95)
}

// MetricPrediction represents a prediction for a network metric.
type MetricPrediction struct {
	Value      float64
	Confidence float64
	Timestamp  time.Time
}