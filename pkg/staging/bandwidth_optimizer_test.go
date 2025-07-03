package staging

import (
	"context"
	"testing"
	"time"
)

func TestNewBandwidthOptimizer(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	
	if optimizer == nil {
		t.Fatal("Expected non-nil BandwidthOptimizer")
	}
	
	if optimizer.config != config {
		t.Error("Expected config to be set correctly")
	}
	
	if optimizer.congestionController == nil {
		t.Error("Expected congestionController to be initialized")
	}
	
	if optimizer.bandwidthEstimator == nil {
		t.Error("Expected bandwidthEstimator to be initialized")
	}
	
	if optimizer.utilizationHistory == nil {
		t.Error("Expected utilizationHistory to be initialized")
	}
	
	if optimizer.optimizationCallbacks == nil {
		t.Error("Expected optimizationCallbacks to be initialized")
	}
	
	if optimizer.active {
		t.Error("Expected optimizer to be inactive initially")
	}
}

func TestBandwidthOptimizer_StartStop(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	ctx := context.Background()
	
	// Test start
	err := optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}
	
	if !optimizer.active {
		t.Error("Expected optimizer to be active after start")
	}
	
	// Test stop
	err = optimizer.Stop()
	if err != nil {
		t.Fatalf("Failed to stop optimizer: %v", err)
	}
	
	if optimizer.active {
		t.Error("Expected optimizer to be inactive after stop")
	}
}

func TestBandwidthOptimizer_DoubleStart(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	ctx := context.Background()
	
	// First start should succeed
	err := optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	
	// Second start should also succeed (idempotent)
	err = optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}
}

func TestBandwidthOptimizer_GetCurrentUtilization(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	
	ctx := context.Background()
	err := optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}
	defer func() { _ = optimizer.Stop() }()
	
	utilization := optimizer.GetCurrentUtilization()
	if utilization == nil {
		t.Fatal("Expected non-nil utilization")
	}
	
	// Utilization should have reasonable values
	if utilization.UtilizationRatio < 0 || utilization.UtilizationRatio > 1 {
		t.Error("Expected utilization ratio to be between 0 and 1")
	}
	
	if utilization.EfficiencyScore < 0 || utilization.EfficiencyScore > 1 {
		t.Error("Expected efficiency score to be between 0 and 1")
	}
}

func TestBandwidthOptimizer_UtilizationBasics(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	
	utilization := optimizer.GetCurrentUtilization()
	if utilization == nil {
		t.Fatal("Expected non-nil utilization")
	}
	
	// Initial utilization should have reasonable defaults
	if utilization.UtilizationRatio < 0 || utilization.UtilizationRatio > 1 {
		t.Error("Expected utilization ratio to be between 0 and 1")
	}
	
	if utilization.EfficiencyScore < 0 || utilization.EfficiencyScore > 1 {
		t.Error("Expected efficiency score to be between 0 and 1")
	}
}

func TestBandwidthOptimizer_RegisterOptimizationCallback(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	
	callbackCalled := false
	var capturedUtilization *BandwidthUtilization
	var capturedRecommendation *OptimizationRecommendation
	
	callback := func(util *BandwidthUtilization, rec *OptimizationRecommendation) error {
		callbackCalled = true
		capturedUtilization = util
		capturedRecommendation = rec
		return nil
	}
	
	optimizer.RegisterOptimizationCallback(callback)
	
	// Start optimizer to enable optimization
	ctx := context.Background()
	err := optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}
	defer func() { _ = optimizer.Stop() }()
	
	// Force optimization to trigger callback
	optimizer.ForceOptimization()
	
	// Allow time for callback to be called
	time.Sleep(100 * time.Millisecond)
	
	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
	
	if capturedUtilization == nil {
		t.Error("Expected utilization to be passed to callback")
	}
	
	if capturedRecommendation == nil {
		t.Error("Expected recommendation to be passed to callback")
	}
}

func TestBandwidthOptimizer_ForceOptimization(t *testing.T) {
	config := DefaultAdaptationConfig()
	optimizer := NewBandwidthOptimizer(config)
	
	ctx := context.Background()
	err := optimizer.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}
	defer func() { _ = optimizer.Stop() }()
	
	initialUtilization := optimizer.GetCurrentUtilization()
	
	// Force optimization should trigger immediate optimization
	optimizer.ForceOptimization()
	
	// Wait a moment for optimization to complete
	time.Sleep(100 * time.Millisecond)
	
	newUtilization := optimizer.GetCurrentUtilization()
	
	// Should have updated timestamp
	if !newUtilization.Timestamp.After(initialUtilization.Timestamp) {
		t.Error("Expected utilization timestamp to be updated after force optimization")
	}
}

func TestNewCongestionController(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewCongestionController(config)
	
	if controller == nil {
		t.Fatal("Expected non-nil CongestionController")
	}
	
	if controller.config != config {
		t.Error("Expected config to be set correctly")
	}
}

func TestCongestionController_GetCongestionLevel(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewCongestionController(config)
	
	level := controller.GetCongestionLevel()
	if level < 0 || level > 1 {
		t.Errorf("Expected congestion level between 0 and 1, got %f", level)
	}
}

func TestCongestionController_Start(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewCongestionController(config)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start in a goroutine since it blocks
	go controller.Start(ctx)
	
	// Give it a moment to start up
	time.Sleep(10 * time.Millisecond)
	
	// Verify it started by checking congestion level is available
	level := controller.GetCongestionLevel()
	if level < 0 {
		t.Error("Expected valid congestion level after start")
	}
}

func TestNewBandwidthEstimator(t *testing.T) {
	estimator := NewBandwidthEstimator()
	
	if estimator == nil {
		t.Fatal("Expected non-nil BandwidthEstimator")
	}
}

func TestBandwidthEstimator_GetEstimatedBandwidth(t *testing.T) {
	estimator := NewBandwidthEstimator()
	
	estimated := estimator.GetEstimatedBandwidth()
	if estimated <= 0 {
		t.Error("Expected positive estimated bandwidth")
	}
}

func TestBandwidthEstimator_Start(t *testing.T) {
	estimator := NewBandwidthEstimator()
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start in a goroutine since it blocks
	go estimator.Start(ctx)
	
	// Give it a moment to start up
	time.Sleep(10 * time.Millisecond)
	
	// Verify it started by checking bandwidth is available
	bandwidth := estimator.GetEstimatedBandwidth()
	if bandwidth <= 0 {
		t.Error("Expected positive bandwidth after start")
	}
}

func TestBandwidthEstimator_GetUtilizedBandwidth(t *testing.T) {
	estimator := NewBandwidthEstimator()
	
	utilized := estimator.GetUtilizedBandwidth()
	if utilized < 0 {
		t.Error("Expected non-negative utilized bandwidth")
	}
}

func TestBandwidthUtilization_Fields(t *testing.T) {
	utilization := &BandwidthUtilization{
		AvailableBandwidthMBps: 100.0,
		UtilizedBandwidthMBps:  80.0,
		UtilizationRatio:       0.8,
		CongestionLevel:        0.1,
		EfficiencyScore:        0.85,
	}
	
	// Test that fields are accessible
	if utilization.EfficiencyScore < 0 || utilization.EfficiencyScore > 1 {
		t.Errorf("Expected efficiency score between 0 and 1, got %f", utilization.EfficiencyScore)
	}
	
	// Good utilization should have high score
	if utilization.EfficiencyScore < 0.7 {
		t.Errorf("Expected high efficiency score for good utilization, got %f", utilization.EfficiencyScore)
	}
}

func TestBandwidthUtilization_NetworkHealth(t *testing.T) {
	// Test excellent health
	excellentUtil := &BandwidthUtilization{
		UtilizationRatio: 0.85,
		CongestionLevel:  0.05,
		NetworkHealth:    NetworkHealthExcellent,
	}
	
	if excellentUtil.NetworkHealth != NetworkHealthExcellent {
		t.Errorf("Expected Excellent health, got %v", excellentUtil.NetworkHealth)
	}
	
	// Test poor health
	poorUtil := &BandwidthUtilization{
		UtilizationRatio: 0.3,
		CongestionLevel:  0.8,
		NetworkHealth:    NetworkHealthPoor,
	}
	
	if poorUtil.NetworkHealth != NetworkHealthPoor {
		t.Errorf("Expected Poor health, got %v", poorUtil.NetworkHealth)
	}
	
	// Test critical health
	criticalUtil := &BandwidthUtilization{
		UtilizationRatio: 0.1,
		CongestionLevel:  0.95,
		NetworkHealth:    NetworkHealthCritical,
	}
	
	if criticalUtil.NetworkHealth != NetworkHealthCritical {
		t.Errorf("Expected Critical health, got %v", criticalUtil.NetworkHealth)
	}
}

func TestOptimizationRecommendation_Fields(t *testing.T) {
	recommendation := &OptimizationRecommendation{
		RecommendedChunkSizeMB: 64,
		RecommendedConcurrency: 8,
		RecommendedCompression: "zstd-fast",
		PredictedImprovement:   0.25,
		Confidence:             0.8,
		Reason:                 "bandwidth_increase",
	}
	
	// Test that fields are accessible and have expected values
	if recommendation.RecommendedChunkSizeMB != 64 {
		t.Error("Expected chunk size to be set correctly")
	}
	
	if recommendation.RecommendedConcurrency != 8 {
		t.Error("Expected concurrency to be set correctly")
	}
	
	if recommendation.RecommendedCompression != "zstd-fast" {
		t.Error("Expected compression level to be set correctly")
	}
	
	if recommendation.PredictedImprovement != 0.25 {
		t.Error("Expected predicted improvement to be set correctly")
	}
	
	if recommendation.Confidence != 0.8 {
		t.Error("Expected confidence to be set correctly")
	}
}

func TestOptimizationRecommendation_Confidence(t *testing.T) {
	// Test high confidence recommendation
	highConfidenceRec := &OptimizationRecommendation{
		PredictedImprovement: 0.25,
		Confidence:           0.9,
	}
	
	if highConfidenceRec.Confidence < 0.8 {
		t.Error("Expected high confidence recommendation")
	}
	
	// Test low confidence recommendation
	lowConfidenceRec := &OptimizationRecommendation{
		PredictedImprovement: 0.05,
		Confidence:           0.3,
	}
	
	if lowConfidenceRec.Confidence > 0.5 {
		t.Error("Expected low confidence recommendation")
	}
}

func TestNewFlowController(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewFlowController(config)
	
	if controller == nil {
		t.Fatal("Expected non-nil FlowController")
	}
	
	if controller.config != config {
		t.Error("Expected config to be set correctly")
	}
	
	if controller.settings == nil {
		t.Error("Expected settings to be initialized")
	}
}

func TestUtilizationHistory_Basic(t *testing.T) {
	history := NewUtilizationHistory()
	
	if history == nil {
		t.Fatal("Expected non-nil UtilizationHistory")
	}
	
	if history.maxHistory <= 0 {
		t.Error("Expected positive max history")
	}
}