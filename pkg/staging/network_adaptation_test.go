package staging

import (
	"context"
	"testing"
	"time"
)

func TestNewNetworkAdaptationEngine(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	if engine == nil {
		t.Fatal("Expected non-nil NetworkAdaptationEngine")
	}
	
	if engine.config != config {
		t.Error("Expected config to be set correctly")
	}
	
	if engine.conditionMonitor == nil {
		t.Error("Expected conditionMonitor to be initialized")
	}
	
	if engine.transferController == nil {
		t.Error("Expected transferController to be initialized")
	}
	
	if engine.bandwidthOptimizer == nil {
		t.Error("Expected bandwidthOptimizer to be initialized")
	}
	
	if engine.adaptationHistory == nil {
		t.Error("Expected adaptationHistory to be initialized")
	}
}

func TestNetworkAdaptationEngine_StartStop(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	// Test start
	err := engine.Start()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	
	if !engine.active {
		t.Error("Expected engine to be active after start")
	}
	
	// Test stop
	err = engine.Stop()
	if err != nil {
		t.Fatalf("Failed to stop engine: %v", err)
	}
	
	if engine.active {
		t.Error("Expected engine to be inactive after stop")
	}
}

func TestNetworkAdaptationEngine_DoubleStart(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	// First start should succeed
	err := engine.Start()
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	
	// Second start should also succeed (idempotent)
	err = engine.Start()
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}
}

func TestNetworkAdaptationEngine_StopWithoutStart(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	// Stop without start should succeed
	err := engine.Stop()
	if err != nil {
		t.Fatalf("Stop without start failed: %v", err)
	}
}

func TestNetworkAdaptationEngine_GetCurrentAdaptation(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	// Should return nil when not started
	adaptation := engine.GetCurrentAdaptation()
	if adaptation != nil {
		t.Error("Expected nil adaptation when engine not started")
	}
	
	// Start engine
	err := engine.Start()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() { _ = engine.Stop() }()
	
	// Should return current adaptation
	adaptation = engine.GetCurrentAdaptation()
	if adaptation == nil {
		t.Error("Expected non-nil adaptation when engine started")
	}
}

func TestNetworkAdaptationEngine_RegisterAdaptationCallback(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	callbackCalled := false
	callback := func(oldState, newState *AdaptationState) error {
		callbackCalled = true
		return nil
	}
	
	engine.RegisterAdaptationCallback(callback)
	
	// Verify callback was registered
	if len(engine.adaptationCallbacks) != 1 {
		t.Error("Expected callback to be registered")
	}
	
	// Test callback is called during adaptation
	err := engine.Start()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() { _ = engine.Stop() }()
	
	// Force an adaptation to trigger callback
	engine.ForceAdaptation()
	
	// Wait a moment for the callback to be called
	time.Sleep(100 * time.Millisecond)
	
	if !callbackCalled {
		t.Error("Expected callback to be called during adaptation")
	}
}

func TestNetworkAdaptationEngine_ForceAdaptation(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	err := engine.Start()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() { _ = engine.Stop() }()
	
	initialAdaptation := engine.GetCurrentAdaptation()
	if initialAdaptation == nil {
		t.Fatal("Expected initial adaptation")
	}
	
	// Force adaptation should trigger immediate adaptation
	engine.ForceAdaptation()
	
	// Wait a moment for adaptation to complete
	time.Sleep(100 * time.Millisecond)
	
	newAdaptation := engine.GetCurrentAdaptation()
	if newAdaptation == nil {
		t.Fatal("Expected adaptation after force")
	}
	
	// Should have updated timestamp
	if !newAdaptation.Timestamp.After(initialAdaptation.Timestamp) {
		t.Error("Expected adaptation timestamp to be updated")
	}
}

func TestNetworkAdaptationEngine_GetCurrentAdaptationAfterStart(t *testing.T) {
	ctx := context.Background()
	config := DefaultAdaptationConfig()
	engine := NewNetworkAdaptationEngine(ctx, config)
	
	err := engine.Start()
	if err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer func() { _ = engine.Stop() }()
	
	// Wait a moment for the engine to initialize
	time.Sleep(100 * time.Millisecond)
	
	// Verify condition was updated
	adaptation := engine.GetCurrentAdaptation()
	if adaptation == nil {
		t.Fatal("Expected adaptation after engine start")
	}
	
	// Should have basic fields set
	if adaptation.ChunkSizeMB <= 0 {
		t.Error("Expected positive chunk size")
	}
	
	if adaptation.Concurrency <= 0 {
		t.Error("Expected positive concurrency")
	}
}

func TestDefaultAdaptationConfig(t *testing.T) {
	config := DefaultAdaptationConfig()
	
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	
	// Test default values
	if config.MonitoringInterval != time.Second*2 {
		t.Error("Expected default monitoring interval to be 2 seconds")
	}
	
	if config.AdaptationInterval != time.Second*5 {
		t.Error("Expected default adaptation interval to be 5 seconds")
	}
	
	if config.BandwidthChangeThreshold != 0.1 {
		t.Error("Expected default bandwidth change threshold to be 0.1")
	}
	
	if config.LatencyChangeThreshold != 0.2 {
		t.Error("Expected default latency change threshold to be 0.2")
	}
	
	if config.MinChunkSizeMB != 5 {
		t.Error("Expected default min chunk size to be 5 MB")
	}
	
	if config.MaxChunkSizeMB != 100 {
		t.Error("Expected default max chunk size to be 100 MB")
	}
	
	if config.MinConcurrency != 1 {
		t.Error("Expected default min concurrency to be 1")
	}
	
	if config.MaxConcurrency != 8 {
		t.Error("Expected default max concurrency to be 8")
	}
}

func TestAdaptationState_Fields(t *testing.T) {
	state := &AdaptationState{
		ChunkSizeMB:      32,
		Concurrency:      4,
		CompressionLevel: "zstd",
		BufferSizeMB:     256,
	}
	
	// Test that fields are accessible and have expected values
	if state.ChunkSizeMB != 32 {
		t.Error("Expected chunk size to be set correctly")
	}
	
	if state.Concurrency != 4 {
		t.Error("Expected concurrency to be set correctly")
	}
	
	if state.CompressionLevel != "zstd" {
		t.Error("Expected compression level to be set correctly")
	}
	
	if state.BufferSizeMB != 256 {
		t.Error("Expected buffer size to be set correctly")
	}
}

func TestAdaptationHistory_RecordAdaptation(t *testing.T) {
	history := NewAdaptationHistory()
	
	oldState := &AdaptationState{
		ChunkSizeMB: 32,
		Concurrency: 4,
	}
	
	newState := &AdaptationState{
		ChunkSizeMB: 64,
		Concurrency: 8,
	}
	
	history.RecordAdaptation(oldState, newState, "bandwidth_increase")
	
	// Verify history was recorded (we can't access internal fields directly)
	recent := history.GetRecentAdaptations(1)
	if len(recent) != 1 {
		t.Error("Expected one record in recent adaptations")
	}
	
	if recent[0].Reason != "bandwidth_increase" {
		t.Error("Expected reason to be recorded correctly")
	}
}

func TestAdaptationHistory_GetRecentAdaptations(t *testing.T) {
	history := NewAdaptationHistory()
	
	// Add multiple records
	for i := 0; i < 5; i++ {
		oldState := &AdaptationState{ChunkSizeMB: 32}
		newState := &AdaptationState{ChunkSizeMB: 64}
		history.RecordAdaptation(oldState, newState, "test")
		// Add small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}
	
	recent := history.GetRecentAdaptations(3)
	if len(recent) != 3 {
		t.Errorf("Expected 3 recent adaptations, got %d", len(recent))
	}
	
	// Should be in reverse chronological order (most recent first)
	for i := 1; i < len(recent); i++ {
		if recent[i].Timestamp.After(recent[i-1].Timestamp) {
			t.Error("Expected recent adaptations to be in reverse chronological order")
		}
	}
}

func TestAdaptationHistory_Basic(t *testing.T) {
	history := NewAdaptationHistory()
	
	if history == nil {
		t.Fatal("Expected non-nil AdaptationHistory")
	}
	
	// Should be able to get recent adaptations even when empty
	recent := history.GetRecentAdaptations(5)
	if recent == nil {
		t.Error("Expected non-nil slice even when empty")
	}
	
	if len(recent) != 0 {
		t.Error("Expected empty slice for new history")
	}
}

func TestNetworkTrend_Values(t *testing.T) {
	// Test that trend constants are defined
	trends := []NetworkTrend{
		TrendUnknown,
		TrendImproving,
		TrendDegrading,
		TrendStable,
		TrendVolatile,
	}
	
	for i, trend := range trends {
		if int(trend) != i {
			t.Errorf("Expected trend %d to have value %d, got %d", i, i, int(trend))
		}
	}
}

func TestNetworkCondition_Fields(t *testing.T) {
	condition := &NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   100.0,
		LatencyMs:       10.0,
		PacketLoss:      0.001,
		Jitter:          1.0,
		CongestionLevel: 0.1,
		Reliability:     0.99,
		PredictedTrend:  TrendImproving,
	}
	
	// Test that fields are accessible and have expected values
	if condition.BandwidthMBps != 100.0 {
		t.Error("Expected bandwidth to be set correctly")
	}
	
	if condition.LatencyMs != 10.0 {
		t.Error("Expected latency to be set correctly")
	}
	
	if condition.PredictedTrend != TrendImproving {
		t.Error("Expected trend to be set correctly")
	}
}

