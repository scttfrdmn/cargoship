package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGlobalCongestionControllerStart(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	// Start congestion controller (should not block)
	gcc.Start(ctx)
	
	// Give it a moment to start background loops
	time.Sleep(time.Millisecond * 100)
	
	// Should not panic or error
}

func TestGlobalCongestionControllerRegisterPrefix(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	gcc.mu.RLock()
	allocation, exists := gcc.prefixAllocation["test-prefix"]
	gcc.mu.RUnlock()
	
	assert.True(t, exists)
	assert.NotNil(t, allocation)
	assert.Equal(t, "test-prefix", allocation.PrefixID)
	assert.Equal(t, 100.0, allocation.AllocatedBandwidthMBps)
	assert.Equal(t, config.GlobalCongestionWindow, allocation.CongestionWindow)
	assert.Equal(t, 0, allocation.InFlight)
	assert.Equal(t, 0.0, allocation.Utilization)
	assert.Equal(t, 1, allocation.Priority)
}

func TestGlobalCongestionControllerAllocateResources(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	upload := &ScheduledUpload{
		PrefixID:      "test-prefix",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	allocation, err := gcc.AllocateResources(upload)
	assert.NoError(t, err)
	assert.NotNil(t, allocation)
	assert.Equal(t, "test-prefix", allocation.PrefixID)
	assert.Equal(t, 1, allocation.InFlight) // Should increment in-flight count
}

func TestGlobalCongestionControllerAllocateResourcesNonExistentPrefix(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	upload := &ScheduledUpload{
		PrefixID:      "non-existent",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	_, err := gcc.AllocateResources(upload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prefix_not_registered")
}

func TestGlobalCongestionControllerCongestionWindowFull(t *testing.T) {
	config := DefaultCoordinationConfig()
	config.GlobalCongestionWindow = 2 // Small window for testing
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	// Fill up the congestion window
	upload1 := &ScheduledUpload{PrefixID: "test-prefix", Priority: 3}
	allocation1, err := gcc.AllocateResources(upload1)
	assert.NoError(t, err)
	assert.Equal(t, 1, allocation1.InFlight)
	
	upload2 := &ScheduledUpload{PrefixID: "test-prefix", Priority: 3}
	allocation2, err := gcc.AllocateResources(upload2)
	assert.NoError(t, err)
	assert.Equal(t, 2, allocation2.InFlight)
	
	// Third upload should trigger congestion window full error
	upload3 := &ScheduledUpload{PrefixID: "test-prefix", Priority: 3}
	_, err = gcc.AllocateResources(upload3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "congestion_window_full")
	assert.NotZero(t, upload3.BackoffDelay) // Should set backoff delay
}

func TestGlobalCongestionControllerUpdatePrefixPerformance(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	metrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ThroughputMBps:       75.0,
		LatencyMs:            80.0,
		ErrorRate:            0.02,
		BandwidthUtilization: 0.6,
	}
	
	gcc.UpdatePrefixPerformance("test-prefix", metrics)
	
	gcc.mu.RLock()
	allocation := gcc.prefixAllocation["test-prefix"]
	gcc.mu.RUnlock()
	
	assert.Equal(t, 0.6, allocation.Utilization)
	assert.NotZero(t, allocation.LastAdjustment)
}

func TestGlobalCongestionControllerSlowStart(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.congestionState = CongestionStateSlowStart
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	// Set a smaller initial window for slow start test
	allocation.CongestionWindow = 8  // Start below slow start threshold
	initialWindow := allocation.CongestionWindow
	initialBandwidth := allocation.AllocatedBandwidthMBps
	
	// Good performance metrics (low error rate)
	metrics := &PrefixPerformanceMetrics{
		ErrorRate: 0.005, // Less than 1%
	}
	
	gcc.applyCongestionControl(allocation, metrics)
	
	// Should increase congestion window and bandwidth
	assert.Greater(t, allocation.CongestionWindow, initialWindow)
	assert.Greater(t, allocation.AllocatedBandwidthMBps, initialBandwidth)
}

func TestGlobalCongestionControllerCongestionAvoidance(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.congestionState = CongestionStateAvoidance
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	initialWindow := allocation.CongestionWindow
	initialBandwidth := allocation.AllocatedBandwidthMBps
	
	// Good performance metrics
	metrics := &PrefixPerformanceMetrics{
		ErrorRate: 0.005, // Less than 1%
	}
	
	gcc.applyCongestionControl(allocation, metrics)
	
	// Should increase congestion window by 1 (linear increase)
	assert.Equal(t, initialWindow+1, allocation.CongestionWindow)
	assert.Greater(t, allocation.AllocatedBandwidthMBps, initialBandwidth)
}

func TestGlobalCongestionControllerCongestionDetection(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.congestionState = CongestionStateSlowStart
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	initialWindow := allocation.CongestionWindow
	initialBandwidth := allocation.AllocatedBandwidthMBps
	
	// High error rate indicating congestion
	metrics := &PrefixPerformanceMetrics{
		ErrorRate: 0.05, // 5% error rate
	}
	
	gcc.applyCongestionControl(allocation, metrics)
	
	// Should trigger congestion handling
	assert.Less(t, allocation.CongestionWindow, initialWindow)
	assert.Less(t, allocation.AllocatedBandwidthMBps, initialBandwidth)
	assert.NotEqual(t, CongestionStateSlowStart, gcc.congestionState)
}

func TestGlobalCongestionControllerRecovery(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.congestionState = CongestionStateRecovery
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	initialWindow := allocation.CongestionWindow
	
	// Very good performance metrics
	metrics := &PrefixPerformanceMetrics{
		ErrorRate: 0.001, // Very low error rate
	}
	
	gcc.applyCongestionControl(allocation, metrics)
	
	// Should gradually increase congestion window
	assert.GreaterOrEqual(t, allocation.CongestionWindow, initialWindow)
}

func TestGlobalCongestionControllerTimeoutCongestion(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	initialWindow := allocation.CongestionWindow
	initialBandwidth := allocation.AllocatedBandwidthMBps
	
	metrics := &PrefixPerformanceMetrics{
		LatencyMs: 1500, // High latency indicating timeout
	}
	
	gcc.detectCongestionEvents(allocation, metrics)
	
	// Should apply aggressive reduction for timeout
	assert.Less(t, allocation.CongestionWindow, initialWindow/2)
	assert.Less(t, allocation.AllocatedBandwidthMBps, initialBandwidth)
	assert.Equal(t, CongestionStateRecovery, gcc.congestionState)
}

func TestGlobalCongestionControllerBandwidthCongestion(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	allocation.AllocatedBandwidthMBps = 100.0
	initialWindow := allocation.CongestionWindow
	initialBandwidth := allocation.AllocatedBandwidthMBps
	
	metrics := &PrefixPerformanceMetrics{
		ThroughputMBps: 30.0, // Much less than allocated bandwidth
	}
	
	gcc.detectCongestionEvents(allocation, metrics)
	
	// Should apply moderate reduction for bandwidth congestion
	assert.Less(t, allocation.CongestionWindow, initialWindow)
	assert.Less(t, allocation.AllocatedBandwidthMBps, initialBandwidth)
}

func TestGlobalCongestionControllerCalculateBackoffDelay(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	allocation := &PrefixAllocation{
		CongestionWindow: 5,
		InFlight:         10, // More than congestion window
	}
	
	delay := gcc.calculateBackoffDelay(allocation)
	
	assert.Greater(t, delay, time.Duration(0))
	assert.LessOrEqual(t, delay, time.Second*30) // Should not exceed max delay
}

func TestGlobalCongestionControllerCalculatePriorityMultiplier(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	tests := []struct {
		priority   int
		multiplier float64
	}{
		{1, 0.5},
		{2, 0.75},
		{3, 1.0},
		{4, 1.25},
		{5, 1.5},
		{99, 1.0}, // Default case
	}
	
	for _, test := range tests {
		result := gcc.calculatePriorityMultiplier(test.priority)
		assert.Equal(t, test.multiplier, result)
	}
}

func TestGlobalCongestionControllerRebalanceAllocations(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.totalBandwidthMBps = 200.0
	
	gcc.RegisterPrefix("prefix-1", 100.0)
	gcc.RegisterPrefix("prefix-2", 100.0)
	
	// Set different utilization levels
	gcc.prefixAllocation["prefix-1"].Utilization = 0.8
	gcc.prefixAllocation["prefix-1"].Priority = 3
	gcc.prefixAllocation["prefix-2"].Utilization = 0.4
	gcc.prefixAllocation["prefix-2"].Priority = 2
	
	gcc.rebalanceAllocations()
	
	// High utilization and priority prefix should get more bandwidth
	allocation1 := gcc.prefixAllocation["prefix-1"]
	allocation2 := gcc.prefixAllocation["prefix-2"]
	
	assert.Greater(t, allocation1.AllocatedBandwidthMBps, allocation2.AllocatedBandwidthMBps)
}

func TestGlobalCongestionControllerGetMetrics(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	gcc.RegisterPrefix("prefix-1", 100.0)
	gcc.RegisterPrefix("prefix-2", 100.0)
	
	// Set some state
	gcc.prefixAllocation["prefix-1"].InFlight = 3
	gcc.prefixAllocation["prefix-1"].Utilization = 0.6
	gcc.prefixAllocation["prefix-2"].InFlight = 2
	gcc.prefixAllocation["prefix-2"].Utilization = 0.4
	
	metrics := gcc.GetMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, config.GlobalCongestionWindow, metrics.GlobalCongestionWindow)
	assert.Equal(t, 5, metrics.TotalInFlight) // 3 + 2
	assert.Equal(t, 0.5, metrics.AverageUtilization) // (0.6 + 0.4) / 2
	assert.Equal(t, gcc.congestionState, metrics.CongestionState)
}

func TestGlobalCongestionControllerBandwidthFilter(t *testing.T) {
	filter := NewBandwidthFilter(time.Second * 5)
	
	// Add some samples
	samples := []BandwidthSample{
		{Timestamp: time.Now().Add(-time.Second * 4), BandwidthMBps: 50.0},
		{Timestamp: time.Now().Add(-time.Second * 3), BandwidthMBps: 75.0},
		{Timestamp: time.Now().Add(-time.Second * 2), BandwidthMBps: 100.0},
		{Timestamp: time.Now().Add(-time.Second * 1), BandwidthMBps: 80.0},
	}
	
	for _, sample := range samples {
		filter.AddSample(sample)
	}
	
	maxBandwidth := filter.GetMaxBandwidth()
	assert.Equal(t, 100.0, maxBandwidth) // Should return the maximum
}

func TestGlobalCongestionControllerBandwidthFilterExpiry(t *testing.T) {
	filter := NewBandwidthFilter(time.Millisecond * 100)
	
	// Add old sample
	oldSample := BandwidthSample{
		Timestamp:     time.Now().Add(-time.Second),
		BandwidthMBps: 100.0,
	}
	filter.AddSample(oldSample)
	
	// Add recent sample
	recentSample := BandwidthSample{
		Timestamp:     time.Now(),
		BandwidthMBps: 50.0,
	}
	filter.AddSample(recentSample)
	
	maxBandwidth := filter.GetMaxBandwidth()
	assert.Equal(t, 50.0, maxBandwidth) // Old sample should be expired
}

func TestGlobalCongestionControllerAdaptiveParameters(t *testing.T) {
	params := NewAdaptiveParameters()
	
	assert.NotNil(t, params)
	assert.NotNil(t, params.BTLBandwidthFilter)
	
	// Test adding sample to bandwidth filter
	sample := BandwidthSample{
		Timestamp:     time.Now(),
		BandwidthMBps: 75.0,
		RTT:           time.Millisecond * 100,
		InFlight:      5,
	}
	
	params.BTLBandwidthFilter.AddSample(sample)
	maxBandwidth := params.BTLBandwidthFilter.GetMaxBandwidth()
	assert.Equal(t, 75.0, maxBandwidth)
}

func TestGlobalCongestionControllerConcurrency(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	var wg sync.WaitGroup
	
	// Register prefixes concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id)
			gcc.RegisterPrefix(prefixID, 100.0)
		}(i)
	}
	
	wg.Wait()
	
	// Allocate resources concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			upload := &ScheduledUpload{
				PrefixID:      fmt.Sprintf("prefix-%d", id%10),
				Priority:      3,
				EstimatedSize: 1024 * 1024,
			}
			_, err := gcc.AllocateResources(upload)
			// Some allocations might fail due to congestion window, which is expected
			if err != nil {
				assert.Contains(t, err.Error(), "congestion_window_full")
			}
		}(i)
	}
	
	// Update performance concurrently
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id%10)
			metrics := &PrefixPerformanceMetrics{
				PrefixID:             prefixID,
				ThroughputMBps:       float64(50 + id%50),
				LatencyMs:            float64(50 + id%100),
				ErrorRate:            float64(id%10) * 0.01,
				BandwidthUtilization: float64(id%100) * 0.01,
			}
			gcc.UpdatePrefixPerformance(prefixID, metrics)
		}(i)
	}
	
	wg.Wait()
	
	// Verify final state
	metrics := gcc.GetMetrics()
	assert.NotNil(t, metrics)
}

func TestGlobalCongestionControllerBackgroundLoops(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	// Start background loops
	gcc.Start(ctx)
	
	// Wait for loops to run
	time.Sleep(time.Millisecond * 150)
	
	// Should complete without errors or panics
}

func TestGlobalCongestionControllerPerformanceFastRecovery(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	gcc.congestionState = CongestionStateFastRecovery
	
	gcc.RegisterPrefix("test-prefix", 100.0)
	
	allocation := gcc.prefixAllocation["test-prefix"]
	
	// Very good performance metrics indicating recovery success
	metrics := &PrefixPerformanceMetrics{
		ErrorRate: 0.001, // Very low error rate
	}
	
	gcc.applyCongestionControl(allocation, metrics)
	
	// Should transition back to congestion avoidance
	assert.Equal(t, CongestionStateAvoidance, gcc.congestionState)
}

func TestUtilityFunctions(t *testing.T) {
	// Test maxInt function
	assert.Equal(t, 10, maxInt(5, 10))
	assert.Equal(t, 10, maxInt(10, 5))
	assert.Equal(t, 10, maxInt(10, 10))
	
	// Test minInt function
	assert.Equal(t, 5, minInt(5, 10))
	assert.Equal(t, 5, minInt(10, 5))
	assert.Equal(t, 5, minInt(5, 5))
}