/*
Tests for predictive adaptation engine - comprehensive test suite for network prediction and adaptation.
*/
package s3

import (
	"context"
	"testing"
	"time"
)

func TestNewPredictiveAdaptationEngine(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{} // Mock adjuster
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	if engine == nil {
		t.Fatal("Expected non-nil predictive adaptation engine")
	}
	
	if engine.predictionHorizon != time.Minute*5 {
		t.Errorf("Expected prediction horizon of 5 minutes, got %v", engine.predictionHorizon)
	}
	
	if engine.adaptationStrategy != AdaptationHybrid {
		t.Errorf("Expected hybrid adaptation strategy, got %v", engine.adaptationStrategy)
	}
	
	if engine.confidenceThreshold != 0.7 {
		t.Errorf("Expected confidence threshold of 0.7, got %f", engine.confidenceThreshold)
	}
}

func TestPredictiveAdaptationEngineStartStop(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test starting
	err := engine.StartPredictiveAdaptation()
	if err != nil {
		t.Fatalf("Failed to start predictive adaptation: %v", err)
	}
	
	if !engine.isRunning {
		t.Error("Expected engine to be running after start")
	}
	
	// Test starting again (should fail)
	err = engine.StartPredictiveAdaptation()
	if err == nil {
		t.Error("Expected error when starting already running engine")
	}
	
	// Test stopping
	err = engine.StopPredictiveAdaptation()
	if err != nil {
		t.Fatalf("Failed to stop predictive adaptation: %v", err)
	}
	
	if engine.isRunning {
		t.Error("Expected engine to be stopped after stop")
	}
	
	// Test stopping again (should fail)
	err = engine.StopPredictiveAdaptation()
	if err == nil {
		t.Error("Expected error when stopping already stopped engine")
	}
}

func TestPredictNetworkConditions(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	prediction, err := engine.PredictNetworkConditions(time.Minute * 5)
	if err != nil {
		t.Fatalf("Failed to predict network conditions: %v", err)
	}
	
	if prediction == nil {
		t.Fatal("Expected non-nil prediction")
	}
	
	if prediction.PredictionHorizon != time.Minute*5 {
		t.Errorf("Expected prediction horizon of 5 minutes, got %v", prediction.PredictionHorizon)
	}
	
	if prediction.PredictedBandwidth < 0 {
		t.Errorf("Expected non-negative bandwidth prediction, got %f", prediction.PredictedBandwidth)
	}
	
	if prediction.PredictedLatency < 0 {
		t.Errorf("Expected non-negative latency prediction, got %f", prediction.PredictedLatency)
	}
	
	if prediction.Confidence < 0 || prediction.Confidence > 1 {
		t.Errorf("Expected confidence between 0 and 1, got %f", prediction.Confidence)
	}
	
	if prediction.UncertaintyBounds == nil {
		t.Error("Expected uncertainty bounds to be present")
	}
}

func TestEvaluateAdaptationNeed(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Create test prediction and current conditions
	prediction := &NetworkPrediction{
		Timestamp:           time.Now(),
		PredictionHorizon:   time.Minute * 5,
		PredictedBandwidth:  30.0, // Significant decrease
		PredictedLatency:    50.0, // Significant increase
		PredictedStability:  0.7,
		PredictedQuality:    0.6,
		Confidence:          0.8, // Above threshold
	}
	
	currentConditions := &RealTimeNetworkConditions{
		Timestamp:           time.Now(),
		BandwidthMBps:       50.0,
		LatencyMs:           30.0,
		ConnectionStability: 0.9,
		NetworkQuality:      0.8,
		Confidence:          0.85,
	}
	
	decision, err := engine.EvaluateAdaptationNeed(prediction, currentConditions)
	if err != nil {
		t.Fatalf("Failed to evaluate adaptation need: %v", err)
	}
	
	if decision == nil {
		t.Fatal("Expected adaptation decision for significant changes")
	}
	
	if decision.ExpectedImprovement <= 0 {
		t.Errorf("Expected positive improvement score, got %f", decision.ExpectedImprovement)
	}
	
	if decision.ExecutionStatus != AdaptationPending {
		t.Errorf("Expected pending status, got %v", decision.ExecutionStatus)
	}
	
	if len(decision.ParameterChanges) == 0 {
		t.Error("Expected parameter changes for adaptation")
	}
}

func TestEvaluateAdaptationNeedLowConfidence(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Create prediction with low confidence
	prediction := &NetworkPrediction{
		Confidence: 0.5, // Below threshold
	}
	
	currentConditions := &RealTimeNetworkConditions{}
	
	decision, err := engine.EvaluateAdaptationNeed(prediction, currentConditions)
	
	if err == nil {
		t.Error("Expected error for low confidence prediction")
	}
	
	if decision != nil {
		t.Error("Expected no decision for low confidence prediction")
	}
}

func TestExecuteAdaptation(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	decision := &AdaptationDecision{
		Timestamp:           time.Now(),
		AdaptationReason:    "Test adaptation",
		ParameterChanges:    map[string]interface{}{"chunk_size": 1024 * 1024 * 4},
		ExpectedImprovement: 0.2,
		RiskAssessment:      0.3, // Low risk
		DecisionConfidence:  0.8,
		ExecutionStatus:     AdaptationPending,
	}
	
	err := engine.ExecuteAdaptation(decision)
	if err != nil {
		t.Fatalf("Failed to execute adaptation: %v", err)
	}
	
	if decision.ExecutionStatus != AdaptationCompleted {
		t.Errorf("Expected completed status, got %v", decision.ExecutionStatus)
	}
}

func TestExecuteAdaptationHighRisk(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	decision := &AdaptationDecision{
		RiskAssessment:  0.8, // High risk
		ParameterChanges: map[string]interface{}{},
		ExecutionStatus: AdaptationPending,
	}
	
	err := engine.ExecuteAdaptation(decision)
	if err == nil {
		t.Error("Expected error for high risk adaptation")
	}
}

func TestValidateAndAdjustPrediction(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test prediction with out-of-bounds values
	prediction := &NetworkPrediction{
		PredictedBandwidth:  -10.0,  // Invalid negative
		PredictedLatency:    2000.0, // Too high
		PredictedStability:  1.5,    // Above 1.0
		PredictedQuality:    -0.5,   // Below 0.0
	}
	
	adjusted := engine.validateAndAdjustPrediction(prediction)
	
	if adjusted.PredictedBandwidth < 0 {
		t.Errorf("Expected non-negative bandwidth, got %f", adjusted.PredictedBandwidth)
	}
	
	if adjusted.PredictedLatency > 1000 {
		t.Errorf("Expected latency <= 1000ms, got %f", adjusted.PredictedLatency)
	}
	
	if adjusted.PredictedStability > 1.0 {
		t.Errorf("Expected stability <= 1.0, got %f", adjusted.PredictedStability)
	}
	
	if adjusted.PredictedQuality < 0 {
		t.Errorf("Expected quality >= 0.0, got %f", adjusted.PredictedQuality)
	}
}

func TestAnalyzePredictedChanges(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	prediction := &NetworkPrediction{
		PredictedBandwidth:  30.0,
		PredictedLatency:    50.0,
		PredictedStability:  0.7,
		PredictedQuality:    0.6,
	}
	
	current := &RealTimeNetworkConditions{
		BandwidthMBps:       50.0,
		LatencyMs:           30.0,
		ConnectionStability: 0.9,
		NetworkQuality:      0.8,
	}
	
	changes := engine.analyzePredictedChanges(prediction, current)
	
	if changes["bandwidth_change"] != -20.0 {
		t.Errorf("Expected bandwidth change of -20.0, got %f", changes["bandwidth_change"])
	}
	
	if changes["latency_change"] != 20.0 {
		t.Errorf("Expected latency change of 20.0, got %f", changes["latency_change"])
	}
	
	expectedBandwidthRelative := -20.0 / 50.0
	if changes["bandwidth_relative"] != expectedBandwidthRelative {
		t.Errorf("Expected bandwidth relative change of %f, got %f", expectedBandwidthRelative, changes["bandwidth_relative"])
	}
}

func TestCalculateAdaptationScore(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test with significant changes
	changes := map[string]float64{
		"bandwidth_relative": -0.4, // 40% decrease
		"latency_relative":   0.6,  // 60% increase
		"stability_change":   -0.2, // 20% decrease
		"quality_change":     -0.2, // 20% decrease
	}
	
	score := engine.calculateAdaptationScore(changes)
	
	if score <= 0 {
		t.Errorf("Expected positive adaptation score for significant changes, got %f", score)
	}
	
	// Test with minimal changes
	minimalChanges := map[string]float64{
		"bandwidth_relative": 0.01,
		"latency_relative":   0.01,
		"stability_change":   0.01,
		"quality_change":     0.01,
	}
	
	minimalScore := engine.calculateAdaptationScore(minimalChanges)
	
	if minimalScore >= score {
		t.Errorf("Expected lower score for minimal changes: minimal=%f, significant=%f", minimalScore, score)
	}
}

func TestGenerateAdaptationReason(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test bandwidth decrease
	changes := map[string]float64{
		"bandwidth_change": -20.0,
		"latency_change":   5.0,
	}
	
	reason := engine.generateAdaptationReason(changes)
	
	if reason != "Predicted bandwidth decrease" {
		t.Errorf("Expected bandwidth decrease reason, got %s", reason)
	}
	
	// Test latency increase
	changes = map[string]float64{
		"bandwidth_change": -5.0,
		"latency_change":   25.0,
	}
	
	reason = engine.generateAdaptationReason(changes)
	
	if reason != "Predicted latency increase" {
		t.Errorf("Expected latency increase reason, got %s", reason)
	}
}

func TestGenerateParameterChanges(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test bandwidth increase scenario
	changes := map[string]float64{
		"bandwidth_change": 15.0, // Significant increase
		"latency_change":   -15.0, // Significant decrease
		"stability_change": 0.15,  // Improvement
	}
	
	paramChanges := engine.generateParameterChanges(changes)
	
	if chunkSize, exists := paramChanges["chunk_size"]; exists {
		if chunkSize != 1024*1024*8 {
			t.Errorf("Expected chunk size of 8MB for bandwidth increase, got %v", chunkSize)
		}
	} else {
		t.Error("Expected chunk_size parameter change for bandwidth increase")
	}
	
	if concurrency, exists := paramChanges["max_concurrency"]; exists {
		if concurrency != 8 {
			t.Errorf("Expected max concurrency of 8 for latency decrease, got %v", concurrency)
		}
	} else {
		t.Error("Expected max_concurrency parameter change for latency decrease")
	}
}

func TestAssessAdaptationRisk(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test high risk changes
	highRiskChanges := map[string]float64{
		"bandwidth_change": -40.0,
		"latency_change":   50.0,
		"stability_change": -0.5,
		"quality_change":   -0.4,
	}
	
	highRisk := engine.assessAdaptationRisk(highRiskChanges)
	
	if highRisk <= 0.5 {
		t.Errorf("Expected high risk (>0.5) for significant changes, got %f", highRisk)
	}
	
	// Test low risk changes
	lowRiskChanges := map[string]float64{
		"bandwidth_change": -2.0,
		"latency_change":   1.0,
		"stability_change": -0.05,
		"quality_change":   -0.02,
	}
	
	lowRisk := engine.assessAdaptationRisk(lowRiskChanges)
	
	if lowRisk >= highRisk {
		t.Errorf("Expected lower risk for minor changes: low=%f, high=%f", lowRisk, highRisk)
	}
}

func TestShouldExecuteAdaptation(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Test proactive strategy
	engine.adaptationStrategy = AdaptationProactive
	decision := &AdaptationDecision{
		DecisionConfidence: 0.7,
		RiskAssessment:     0.5,
	}
	
	if !engine.shouldExecuteAdaptation(decision) {
		t.Error("Expected proactive strategy to execute with confidence > 0.6")
	}
	
	// Test risk-aware strategy
	engine.adaptationStrategy = AdaptationRiskAware
	decision.DecisionConfidence = 0.9
	decision.RiskAssessment = 0.2
	
	if !engine.shouldExecuteAdaptation(decision) {
		t.Error("Expected risk-aware strategy to execute with high confidence and low risk")
	}
	
	// Test risk-aware with high risk
	decision.RiskAssessment = 0.5
	
	if engine.shouldExecuteAdaptation(decision) {
		t.Error("Expected risk-aware strategy to reject high risk adaptation")
	}
}

func TestNetworkFeatureExtractor(t *testing.T) {
	extractor := NewNetworkFeatureExtractor()
	
	features, err := extractor.ExtractFeatures()
	if err != nil {
		t.Fatalf("Failed to extract features: %v", err)
	}
	
	if len(features) == 0 {
		t.Error("Expected non-empty feature set")
	}
	
	expectedFeatures := []string{
		"bandwidth_trend",
		"latency_trend",
		"stability_score",
		"quality_score",
		"time_of_day",
		"day_of_week",
		"historical_variance",
	}
	
	for _, feature := range expectedFeatures {
		if _, exists := features[feature]; !exists {
			t.Errorf("Expected feature %s to be present", feature)
		}
	}
	
	// Validate time_of_day is in valid range
	if timeOfDay := features["time_of_day"]; timeOfDay < 0 || timeOfDay > 1 {
		t.Errorf("Expected time_of_day in range [0,1], got %f", timeOfDay)
	}
}

func TestPredictionModelEnsemble(t *testing.T) {
	ensemble := NewPredictionModelEnsemble()
	
	features := map[string]float64{
		"bandwidth_trend": 0.1,
		"latency_trend":   -0.05,
		"stability_score": 0.9,
		"quality_score":   0.8,
	}
	
	prediction, err := ensemble.PredictConditions(features, time.Minute*5)
	if err != nil {
		t.Fatalf("Failed to predict conditions: %v", err)
	}
	
	if prediction == nil {
		t.Fatal("Expected non-nil prediction")
	}
	
	if prediction.PredictionMethod != PredictionEnsemble {
		t.Errorf("Expected ensemble prediction method, got %v", prediction.PredictionMethod)
	}
	
	if prediction.UncertaintyBounds == nil {
		t.Error("Expected uncertainty bounds to be present")
	}
	
	if prediction.FeatureImportance == nil {
		t.Error("Expected feature importance to be present")
	}
}

func TestNetworkAnomalyDetector(t *testing.T) {
	detector := NewNetworkAnomalyDetector()
	
	// Test normal prediction (high confidence)
	normalPrediction := &NetworkPrediction{
		Confidence: 0.9,
	}
	
	if detector.DetectAnomaly(normalPrediction) {
		t.Error("Expected no anomaly for high confidence prediction")
	}
	
	// Test anomalous prediction (low confidence)
	anomalousPrediction := &NetworkPrediction{
		Confidence: 0.8, // Below threshold of 0.95
	}
	
	if !detector.DetectAnomaly(anomalousPrediction) {
		t.Error("Expected anomaly for low confidence prediction")
	}
}

func TestPredictionAccuracyTracker(t *testing.T) {
	tracker := NewPredictionAccuracyTracker()
	
	prediction := &NetworkPrediction{
		Timestamp:          time.Now(),
		PredictedBandwidth: 50.0,
		PredictedLatency:   30.0,
		Confidence:         0.8,
	}
	
	tracker.RecordPrediction(prediction)
	
	metrics := tracker.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}
	
	if metrics.PredictionCount != 1 {
		t.Errorf("Expected prediction count of 1, got %d", metrics.PredictionCount)
	}
	
	if metrics.OverallAccuracy <= 0 {
		t.Errorf("Expected positive overall accuracy, got %f", metrics.OverallAccuracy)
	}
}

func TestAdaptationMetrics(t *testing.T) {
	metrics := NewPredictiveAdaptationMetrics()
	
	decision := &AdaptationDecision{
		Timestamp:           time.Now(),
		AdaptationReason:    "Test adaptation",
		ExpectedImprovement: 0.2,
		RiskAssessment:      0.3,
		ExecutionStatus:     AdaptationCompleted,
	}
	
	metrics.RecordAdaptation(decision)
	
	if metrics.TotalAdaptations != 1 {
		t.Errorf("Expected total adaptations of 1, got %d", metrics.TotalAdaptations)
	}
	
	if metrics.SuccessfulAdaptations != 1 {
		t.Errorf("Expected successful adaptations of 1, got %d", metrics.SuccessfulAdaptations)
	}
	
	if len(metrics.AdaptationHistory) != 1 {
		t.Errorf("Expected adaptation history length of 1, got %d", len(metrics.AdaptationHistory))
	}
	
	summary := metrics.AdaptationHistory[0]
	if !summary.Success {
		t.Error("Expected successful adaptation in history")
	}
	
	if summary.AdaptationType != "Test adaptation" {
		t.Errorf("Expected adaptation type 'Test adaptation', got %s", summary.AdaptationType)
	}
}

func TestPredictiveAdaptationEngineIntegration(t *testing.T) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	// Start the engine
	err := engine.StartPredictiveAdaptation()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() {
		_ = engine.StopPredictiveAdaptation()
	}()
	
	// Generate prediction
	prediction, err := engine.PredictNetworkConditions(time.Minute * 5)
	if err != nil {
		t.Fatalf("Failed to predict conditions: %v", err)
	}
	
	// Create current conditions that would trigger adaptation
	currentConditions := &RealTimeNetworkConditions{
		BandwidthMBps:       prediction.PredictedBandwidth + 20, // Different from prediction
		LatencyMs:           prediction.PredictedLatency - 10,   // Different from prediction
		ConnectionStability: 0.9,
		NetworkQuality:      0.8,
		Confidence:          0.85,
	}
	
	// Evaluate adaptation need
	decision, err := engine.EvaluateAdaptationNeed(prediction, currentConditions)
	if err != nil {
		t.Fatalf("Failed to evaluate adaptation need: %v", err)
	}
	
	// Execute adaptation if needed
	if decision != nil && engine.shouldExecuteAdaptation(decision) {
		err = engine.ExecuteAdaptation(decision)
		if err != nil {
			t.Fatalf("Failed to execute adaptation: %v", err)
		}
		
		if decision.ExecutionStatus != AdaptationCompleted {
			t.Errorf("Expected completed adaptation, got %v", decision.ExecutionStatus)
		}
	}
	
	// Check metrics
	accuracy := engine.GetPredictionAccuracy()
	if accuracy == nil {
		t.Error("Expected non-nil prediction accuracy metrics")
	}
	
	adaptationMetrics := engine.GetAdaptationMetrics()
	if adaptationMetrics == nil {
		t.Error("Expected non-nil adaptation metrics")
	}
}

// Benchmark tests
func BenchmarkPredictNetworkConditions(b *testing.B) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.PredictNetworkConditions(time.Minute * 5)
	}
}

func BenchmarkEvaluateAdaptationNeed(b *testing.B) {
	ctx := context.Background()
	parameterAdjuster := &DynamicParameterAdjuster{}
	engine := NewPredictiveAdaptationEngine(ctx, parameterAdjuster)
	
	prediction := &NetworkPrediction{
		PredictedBandwidth: 30.0,
		PredictedLatency:   50.0,
		PredictedStability: 0.7,
		PredictedQuality:   0.6,
		Confidence:         0.8,
	}
	
	currentConditions := &RealTimeNetworkConditions{
		BandwidthMBps:       50.0,
		LatencyMs:           30.0,
		ConnectionStability: 0.9,
		NetworkQuality:      0.8,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.EvaluateAdaptationNeed(prediction, currentConditions)
	}
}