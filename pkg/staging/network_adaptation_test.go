package staging

import (
	"context"
	"testing"
	"time"
)

func TestNewNetworkAdaptationEngine(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	if engine == nil {
		t.Fatal("Expected non-nil NetworkAdaptationEngine")
		return
	}

	if engine.config != config {
		t.Error("Expected config to be set correctly")
	}
}

func TestNetworkAdaptationEngine_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network adaptation test in short mode")
	}

	config := DefaultAdaptationConfig()

	// Use cancellable context for proper cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cleanup happens

	engine := NewNetworkAdaptationEngine(ctx, config)

	// Start in a goroutine since it blocks
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = engine.Start()
	}()

	// Give it a moment to start up
	time.Sleep(50 * time.Millisecond)

	// Stop the engine by cancelling context
	cancel()

	// Wait for goroutine to finish or timeout
	select {
	case <-done:
		// Good - goroutine stopped
	case <-time.After(time.Second):
		t.Error("NetworkAdaptationEngine goroutine did not stop within timeout")
	}
}

func TestNetworkAdaptationEngine_DoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network adaptation test in short mode")
	}

	config := DefaultAdaptationConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cleanup

	engine := NewNetworkAdaptationEngine(ctx, config)

	// Start first instance
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		_ = engine.Start()
	}()

	// Give it a moment to start up
	time.Sleep(50 * time.Millisecond)

	// Try to start again - should not panic and should handle gracefully
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		_ = engine.Start() // This should handle already-started state
	}()

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	// Cancel to stop both
	cancel()

	// Wait for both to finish
	select {
	case <-done1:
		// First goroutine stopped
	case <-time.After(time.Second):
		t.Error("First NetworkAdaptationEngine goroutine did not stop")
	}

	select {
	case <-done2:
		// Second goroutine stopped
	case <-time.After(time.Second):
		t.Error("Second NetworkAdaptationEngine goroutine did not stop")
	}
}

func TestNetworkAdaptationEngine_GetCurrentAdaptation(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	state := engine.GetCurrentAdaptation()

	// State might be nil initially, which is acceptable
	if state != nil {
		// If state is not nil, verify it has expected fields
		if state.ChunkSizeMB <= 0 {
			t.Error("Expected positive chunk size")
		}
	}
}

func TestNetworkAdaptationEngine_ForceAdaptation(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	// Start in a goroutine since it blocks
	go func() { _ = engine.Start() }()

	// Give it a moment to start up
	time.Sleep(100 * time.Millisecond)

	// Force adaptation
	engine.ForceAdaptation()

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)
}

func TestNetworkAdaptationEngine_GetCurrentAdaptationAfterStart(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	// Start in a goroutine since it blocks
	go func() { _ = engine.Start() }()

	// Give it a moment to start up
	time.Sleep(100 * time.Millisecond)

	state := engine.GetCurrentAdaptation()

	if state == nil {
		t.Error("Expected non-nil adaptation state after start")
	}
}

func TestDefaultAdaptationConfig(t *testing.T) {
	config := DefaultAdaptationConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
		return
	}

	if config.AdaptationInterval <= 0 {
		t.Error("Expected positive adaptation interval")
	}

	if config.MaxChunkSizeMB <= 0 {
		t.Error("Expected positive max chunk size")
	}

	if config.MaxConcurrency <= 0 {
		t.Error("Expected positive max concurrency")
	}
}

func TestAdaptationState_Fields(t *testing.T) {
	state := &AdaptationState{
		ChunkSizeMB:      10,
		Concurrency:      5,
		CompressionLevel: "zstd",
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 100,
			LatencyMs:     50,
		},
	}

	if state.ChunkSizeMB != 10 {
		t.Error("Expected chunk size to be set")
	}

	if state.Concurrency != 5 {
		t.Error("Expected concurrency to be set")
	}

	if state.CompressionLevel != "zstd" {
		t.Error("Expected compression level to be set")
	}

	if state.NetworkCondition == nil {
		t.Error("Expected network condition to be set")
	}
}

// Test NetworkAdaptationEngine scoreCompression function
func TestNetworkAdaptationEngine_scoreCompression(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	condition := &NetworkCondition{
		BandwidthMBps: 100,
		LatencyMs:     50,
		PacketLoss:    0.01,
	}

	tests := []struct {
		name        string
		compression string
		expected    float64
	}{
		{
			name:        "optimal compression",
			compression: "zstd",
			expected:    1.0,
		},
		{
			name:        "similar compression",
			compression: "zstd-fast",
			expected:    0.75, // Should be close but not perfect
		},
		{
			name:        "invalid compression",
			compression: "invalid",
			expected:    0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := engine.scoreCompression(tt.compression, condition)

			if tt.name == "optimal compression" && score != tt.expected {
				t.Errorf("Expected score %f, got %f", tt.expected, score)
			}

			if tt.name == "invalid compression" && score != tt.expected {
				t.Errorf("Expected score %f, got %f", tt.expected, score)
			}

			if score < 0 || score > 1 {
				t.Errorf("Expected score between 0 and 1, got %f", score)
			}
		})
	}
}

// Test NetworkAdaptationEngine predictPerformanceImprovement function
func TestNetworkAdaptationEngine_predictPerformanceImprovement(t *testing.T) {
	config := DefaultAdaptationConfig()
	ctx := context.Background()
	engine := NewNetworkAdaptationEngine(ctx, config)

	oldState := &AdaptationState{
		ChunkSizeMB:      10,
		Concurrency:      5,
		CompressionLevel: "zstd",
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 100,
			LatencyMs:     50,
		},
		PerformanceMetrics: &PerformanceMetrics{
			AverageThroughputMBps: 50,
			ErrorRate:             0.01,
		},
	}

	newState := &AdaptationState{
		ChunkSizeMB:      20,
		Concurrency:      10,
		CompressionLevel: "zstd-fast",
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 100,
			LatencyMs:     50,
		},
	}

	improvement := engine.predictPerformanceImprovement(oldState, newState)

	if improvement < 0 {
		t.Error("Expected non-negative improvement")
	}

	// Test with nil old state
	nilOldState := &AdaptationState{
		PerformanceMetrics: nil,
	}

	improvement = engine.predictPerformanceImprovement(nilOldState, newState)

	if improvement != 0.1 {
		t.Errorf("Expected improvement of 0.1 for nil old state, got %f", improvement)
	}
}

func TestNetworkTrend_Values(t *testing.T) {
	trend := TrendImproving

	if trend != TrendImproving {
		t.Error("Expected trend to be set")
	}

	// Test other trend values
	if TrendDegrading == TrendImproving {
		t.Error("Expected different trend values")
	}
}

func TestNetworkCondition_Fields(t *testing.T) {
	condition := &NetworkCondition{
		BandwidthMBps:   100,
		LatencyMs:       50,
		PacketLoss:      0.01,
		Jitter:          0.02,
		CongestionLevel: 0.3,
		Reliability:     0.95,
	}

	if condition.BandwidthMBps != 100 {
		t.Error("Expected bandwidth to be set")
	}

	if condition.LatencyMs != 50 {
		t.Error("Expected latency to be set")
	}

	if condition.PacketLoss != 0.01 {
		t.Error("Expected packet loss to be set")
	}

	if condition.Jitter != 0.02 {
		t.Error("Expected jitter to be set")
	}

	if condition.CongestionLevel != 0.3 {
		t.Error("Expected congestion to be set")
	}

	if condition.Reliability != 0.95 {
		t.Error("Expected reliability to be set")
	}
}
