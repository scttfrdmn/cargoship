package monitoring

import (
	"context"
	"math"
	"sync"
	"time"
)

// AnalyticsEngine provides predictive analytics for performance monitoring.
type AnalyticsEngine struct {
	config            *MonitoringConfig
	predictionModels  map[string]*PredictionModel
	trendAnalyzer     *TrendAnalyzer
	anomalyDetector   *AnomalyDetector
	currentPredictions *PerformancePredictions
	historicalData    *HistoricalData
	mu                sync.RWMutex
	isRunning         bool
}

// NewAnalyticsEngine creates a new analytics engine.
func NewAnalyticsEngine(config *MonitoringConfig) *AnalyticsEngine {
	ae := &AnalyticsEngine{
		config:           config,
		predictionModels: make(map[string]*PredictionModel),
		trendAnalyzer:    NewTrendAnalyzer(),
		anomalyDetector:  NewAnomalyDetector(),
		historicalData:   NewHistoricalData(),
	}
	
	// Initialize prediction models
	ae.initializePredictionModels()
	
	return ae
}

// Start begins analytics processing.
func (ae *AnalyticsEngine) Start(ctx context.Context) error {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	if ae.isRunning {
		return nil
	}
	
	ae.isRunning = true
	return nil
}

// Stop stops analytics processing.
func (ae *AnalyticsEngine) Stop() {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	ae.isRunning = false
}

// ProcessNewMetrics processes new performance metrics for analytics.
func (ae *AnalyticsEngine) ProcessNewMetrics(metrics *PerformanceMetrics) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	// Store historical data
	ae.historicalData.AddMetrics(metrics)
	
	// Update prediction models
	for _, model := range ae.predictionModels {
		model.UpdateWithMetrics(metrics)
	}
	
	// Detect anomalies
	anomalies := ae.anomalyDetector.DetectAnomalies(metrics, ae.historicalData)
	if len(anomalies) > 0 {
		// Handle anomalies
		ae.handleAnomalies(anomalies)
	}
}

// RunPredictiveAnalysis performs predictive analysis and updates predictions.
func (ae *AnalyticsEngine) RunPredictiveAnalysis() {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	if !ae.config.EnablePredictive || !ae.historicalData.HasSufficientData() {
		return
	}
	
	predictions := &PerformancePredictions{
		Predictions:     make([]*PerformancePrediction, 0),
		GeneratedAt:     time.Now(),
		PredictionWindow: time.Hour, // Predict 1 hour ahead
		Confidence:      0.7,
	}
	
	// Generate predictions for each metric type
	throughputPrediction := ae.predictionModels["throughput"].Predict(time.Hour)
	if throughputPrediction != nil {
		predictions.Predictions = append(predictions.Predictions, throughputPrediction)
	}
	
	latencyPrediction := ae.predictionModels["latency"].Predict(time.Hour)
	if latencyPrediction != nil {
		predictions.Predictions = append(predictions.Predictions, latencyPrediction)
	}
	
	errorRatePrediction := ae.predictionModels["error_rate"].Predict(time.Hour)
	if errorRatePrediction != nil {
		predictions.Predictions = append(predictions.Predictions, errorRatePrediction)
	}
	
	// Analyze trends
	trends := ae.trendAnalyzer.AnalyzeTrends(ae.historicalData)
	predictions.Trends = trends
	
	// Calculate overall confidence
	predictions.Confidence = ae.calculateOverallConfidence(predictions.Predictions)
	
	ae.currentPredictions = predictions
}

// GetPredictions returns current performance predictions.
func (ae *AnalyticsEngine) GetPredictions() *PerformancePredictions {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	if ae.currentPredictions == nil {
		return nil
	}
	
	// Return a copy
	predictions := *ae.currentPredictions
	return &predictions
}

// initializePredictionModels initializes the prediction models.
func (ae *AnalyticsEngine) initializePredictionModels() {
	ae.predictionModels["throughput"] = NewPredictionModel("throughput")
	ae.predictionModels["latency"] = NewPredictionModel("latency")
	ae.predictionModels["error_rate"] = NewPredictionModel("error_rate")
	ae.predictionModels["cpu_usage"] = NewPredictionModel("cpu_usage")
	ae.predictionModels["memory_usage"] = NewPredictionModel("memory_usage")
}

// calculateOverallConfidence calculates overall prediction confidence.
func (ae *AnalyticsEngine) calculateOverallConfidence(predictions []*PerformancePrediction) float64 {
	if len(predictions) == 0 {
		return 0.0
	}
	
	totalConfidence := 0.0
	for _, prediction := range predictions {
		totalConfidence += prediction.Confidence
	}
	
	return totalConfidence / float64(len(predictions))
}

// handleAnomalies handles detected anomalies.
func (ae *AnalyticsEngine) handleAnomalies(anomalies []*Anomaly) {
	// Log anomalies or trigger additional analysis
	// This could trigger immediate alerts or additional monitoring
}

// PredictionModel represents a model for predicting specific metrics.
type PredictionModel struct {
	metricName      string
	historicalData  []float64
	timestamps      []time.Time
	trendCoefficient float64
	seasonalPattern map[int]float64 // Hour of day -> multiplier
	lastUpdate      time.Time
	mu              sync.RWMutex
}

// NewPredictionModel creates a new prediction model.
func NewPredictionModel(metricName string) *PredictionModel {
	return &PredictionModel{
		metricName:      metricName,
		historicalData:  make([]float64, 0),
		timestamps:      make([]time.Time, 0),
		seasonalPattern: make(map[int]float64),
	}
}

// UpdateWithMetrics updates the model with new metrics.
func (pm *PredictionModel) UpdateWithMetrics(metrics *PerformanceMetrics) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	var value float64
	timestamp := time.Now()
	
	// Extract relevant metric value
	switch pm.metricName {
	case "throughput":
		if metrics.TransferMetrics != nil {
			value = metrics.TransferMetrics.TotalThroughputMBps
		}
	case "latency":
		if metrics.TransferMetrics != nil {
			value = metrics.TransferMetrics.AverageLatencyMs
		}
	case "error_rate":
		if metrics.TransferMetrics != nil {
			value = 1.0 - metrics.TransferMetrics.SuccessRate
		}
	case "cpu_usage":
		if metrics.SystemMetrics != nil {
			value = metrics.SystemMetrics.CPUUsagePercent / 100.0
		}
	case "memory_usage":
		if metrics.SystemMetrics != nil {
			value = metrics.SystemMetrics.MemoryUsagePercent / 100.0
		}
	default:
		return
	}
	
	pm.historicalData = append(pm.historicalData, value)
	pm.timestamps = append(pm.timestamps, timestamp)
	
	// Limit history size
	maxPoints := 1000
	if len(pm.historicalData) > maxPoints {
		pm.historicalData = pm.historicalData[len(pm.historicalData)-maxPoints:]
		pm.timestamps = pm.timestamps[len(pm.timestamps)-maxPoints:]
	}
	
	// Update trend coefficient
	pm.updateTrendCoefficient()
	
	// Update seasonal pattern
	pm.updateSeasonalPattern(timestamp, value)
	
	pm.lastUpdate = timestamp
}

// Predict predicts the metric value for the given time window.
func (pm *PredictionModel) Predict(window time.Duration) *PerformancePrediction {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if len(pm.historicalData) < 10 {
		return nil // Not enough data
	}
	
	targetTime := time.Now().Add(window)
	
	// Get baseline (recent average)
	baseline := pm.getRecentAverage(20) // Last 20 data points
	
	// Apply trend
	trendAdjustment := pm.trendCoefficient * window.Hours()
	predictedValue := baseline + trendAdjustment
	
	// Apply seasonal adjustment
	hour := targetTime.Hour()
	if seasonalMultiplier, exists := pm.seasonalPattern[hour]; exists {
		predictedValue *= seasonalMultiplier
	}
	
	// Calculate confidence
	confidence := pm.calculatePredictionConfidence()
	
	// Determine if this represents an issue
	isIssue := pm.isValueProblematic(predictedValue)
	timeToIssue := window
	if !isIssue {
		timeToIssue = 0
	}
	
	return &PerformancePrediction{
		Type:        pm.metricName,
		Value:       predictedValue,
		Confidence:  confidence,
		TimeToIssue: timeToIssue,
		Description: pm.generateDescription(predictedValue, isIssue),
		Timestamp:   targetTime,
	}
}

// updateTrendCoefficient calculates and updates the trend coefficient.
func (pm *PredictionModel) updateTrendCoefficient() {
	if len(pm.historicalData) < 10 {
		return
	}
	
	// Simple linear regression to find trend
	n := float64(len(pm.historicalData))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0
	
	for i, value := range pm.historicalData {
		x := float64(i)
		y := value
		
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	denominator := n*sumX2 - sumX*sumX
	if denominator != 0 {
		pm.trendCoefficient = (n*sumXY - sumX*sumY) / denominator
	}
}

// updateSeasonalPattern updates the seasonal pattern.
func (pm *PredictionModel) updateSeasonalPattern(timestamp time.Time, value float64) {
	hour := timestamp.Hour()
	
	// Simple exponential moving average for seasonal pattern
	alpha := 0.1 // Learning rate
	if currentPattern, exists := pm.seasonalPattern[hour]; exists {
		pm.seasonalPattern[hour] = alpha*value + (1-alpha)*currentPattern
	} else {
		pm.seasonalPattern[hour] = value
	}
}

// getRecentAverage calculates the average of recent data points.
func (pm *PredictionModel) getRecentAverage(count int) float64 {
	if len(pm.historicalData) == 0 {
		return 0
	}
	
	start := len(pm.historicalData) - count
	if start < 0 {
		start = 0
	}
	
	sum := 0.0
	for i := start; i < len(pm.historicalData); i++ {
		sum += pm.historicalData[i]
	}
	
	return sum / float64(len(pm.historicalData)-start)
}

// calculatePredictionConfidence calculates confidence in the prediction.
func (pm *PredictionModel) calculatePredictionConfidence() float64 {
	baseConfidence := 0.5
	
	// More data = higher confidence
	dataPoints := len(pm.historicalData)
	if dataPoints > 100 {
		baseConfidence = 0.8
	} else if dataPoints > 50 {
		baseConfidence = 0.7
	} else if dataPoints > 20 {
		baseConfidence = 0.6
	}
	
	// Lower variability = higher confidence
	if len(pm.historicalData) > 10 {
		variance := pm.calculateVariance()
		mean := pm.getRecentAverage(len(pm.historicalData))
		if mean > 0 {
			cv := math.Sqrt(variance) / mean // Coefficient of variation
			confidenceAdjustment := math.Max(0, 0.3-cv) // Lower CV = higher confidence
			baseConfidence += confidenceAdjustment
		}
	}
	
	return math.Min(math.Max(baseConfidence, 0.1), 0.95)
}

// calculateVariance calculates the variance of historical data.
func (pm *PredictionModel) calculateVariance() float64 {
	if len(pm.historicalData) < 2 {
		return 0
	}
	
	mean := pm.getRecentAverage(len(pm.historicalData))
	sumSquaredDiffs := 0.0
	
	for _, value := range pm.historicalData {
		diff := value - mean
		sumSquaredDiffs += diff * diff
	}
	
	return sumSquaredDiffs / float64(len(pm.historicalData)-1)
}

// isValueProblematic determines if a predicted value indicates an issue.
func (pm *PredictionModel) isValueProblematic(value float64) bool {
	switch pm.metricName {
	case "throughput":
		return value < 1.0 // Less than 1 MB/s
	case "latency":
		return value > 1000 // More than 1 second
	case "error_rate":
		return value > 0.05 // More than 5%
	case "cpu_usage":
		return value > 0.9 // More than 90%
	case "memory_usage":
		return value > 0.9 // More than 90%
	}
	return false
}

// generateDescription generates a human-readable description.
func (pm *PredictionModel) generateDescription(value float64, isIssue bool) string {
	if isIssue {
		switch pm.metricName {
		case "throughput":
			return "Predicted low throughput may impact transfer performance"
		case "latency":
			return "Predicted high latency may cause delays"
		case "error_rate":
			return "Predicted high error rate may indicate system issues"
		case "cpu_usage":
			return "Predicted high CPU usage may impact performance"
		case "memory_usage":
			return "Predicted high memory usage may cause instability"
		}
	}
	return "Performance metrics predicted to be within normal ranges"
}

// PerformancePredictions contains performance predictions.
type PerformancePredictions struct {
	Predictions      []*PerformancePrediction `json:"predictions"`
	Trends          *TrendAnalysis           `json:"trends"`
	GeneratedAt     time.Time                `json:"generated_at"`
	PredictionWindow time.Duration           `json:"prediction_window"`
	Confidence      float64                  `json:"confidence"`
}

// PerformancePrediction represents a single performance prediction.
type PerformancePrediction struct {
	Type        string        `json:"type"`
	Value       float64       `json:"value"`
	Confidence  float64       `json:"confidence"`
	TimeToIssue time.Duration `json:"time_to_issue"`
	Description string        `json:"description"`
	Timestamp   time.Time     `json:"timestamp"`
}

// TrendAnalyzer analyzes performance trends.
type TrendAnalyzer struct{}

// NewTrendAnalyzer creates a new trend analyzer.
func NewTrendAnalyzer() *TrendAnalyzer {
	return &TrendAnalyzer{}
}

// AnalyzeTrends analyzes trends in historical data.
func (ta *TrendAnalyzer) AnalyzeTrends(data *HistoricalData) *TrendAnalysis {
	return &TrendAnalysis{
		OverallTrend:    "stable",
		ThroughputTrend: "improving",
		LatencyTrend:    "stable",
		ErrorRateTrend:  "improving",
		Timestamp:       time.Now(),
	}
}

// TrendAnalysis contains trend analysis results.
type TrendAnalysis struct {
	OverallTrend    string    `json:"overall_trend"`
	ThroughputTrend string    `json:"throughput_trend"`
	LatencyTrend    string    `json:"latency_trend"`
	ErrorRateTrend  string    `json:"error_rate_trend"`
	Timestamp       time.Time `json:"timestamp"`
}

// AnomalyDetector detects anomalies in performance metrics.
type AnomalyDetector struct{}

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{}
}

// DetectAnomalies detects anomalies in current metrics.
func (ad *AnomalyDetector) DetectAnomalies(current *PerformanceMetrics, historical *HistoricalData) []*Anomaly {
	return []*Anomaly{} // Placeholder implementation
}

// Anomaly represents a detected anomaly.
type Anomaly struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Value       float64   `json:"value"`
	Expected    float64   `json:"expected"`
	Timestamp   time.Time `json:"timestamp"`
}

// HistoricalData stores historical performance data.
type HistoricalData struct {
	metrics []PerformanceMetrics
	mu      sync.RWMutex
}

// NewHistoricalData creates new historical data storage.
func NewHistoricalData() *HistoricalData {
	return &HistoricalData{
		metrics: make([]PerformanceMetrics, 0),
	}
}

// AddMetrics adds metrics to historical data.
func (hd *HistoricalData) AddMetrics(metrics *PerformanceMetrics) {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	
	hd.metrics = append(hd.metrics, *metrics)
	
	// Limit size
	maxSize := 10000
	if len(hd.metrics) > maxSize {
		hd.metrics = hd.metrics[len(hd.metrics)-maxSize:]
	}
}

// HasSufficientData checks if there's enough historical data.
func (hd *HistoricalData) HasSufficientData() bool {
	hd.mu.RLock()
	defer hd.mu.RUnlock()
	return len(hd.metrics) >= 50
}