package s3

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
)

func TestEnhancedCongestionControlWithCommunication(t *testing.T) {
	testutil.SkipIfShort(t, "congestion control involves background goroutines")

	// Disable parallel execution to prevent test interference
	// This test creates goroutines and shared state that can race with other tests
	t.Setenv("_TEST_ISOLATION", "true") // Dummy env var to prevent parallel execution

	testutil.WithLeakCheck(t, testutil.DefaultLeakCheckOptions(), func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel() // Ensure context is cancelled for cleanup

		config := DefaultCoordinationConfig()

		// Create global congestion controller
		gcc := NewGlobalCongestionController(config)

		// Create communication system
		commConfig := DefaultCommunicationConfig()
		communicator := NewCrossPrefixCommunicator(ctx, commConfig)

		err := communicator.Start()
		require.NoError(t, err)
		defer func() {
			if err := communicator.Stop(); err != nil {
				t.Logf("Warning: communicator stop error: %v", err)
			}
		}()

		// Integrate congestion controller with communicator
		gcc.SetCommunicator(communicator)

		// Register test prefixes
		prefixes := []string{"prefix-1", "prefix-2", "prefix-3"}
		for _, prefixID := range prefixes {
			err := communicator.RegisterPrefix(prefixID)
			require.NoError(t, err)

			err = gcc.CoordinatedRegisterPrefix(prefixID, 100.0)
			require.NoError(t, err)
		}

		// Verify BBR mode initialization
		metrics := gcc.GetEnhancedMetrics()
		assert.Equal(t, BBRModeStartup, metrics.BBRMode)
		assert.True(t, metrics.CrossPrefixActive)
		assert.Equal(t, 3, len(gcc.prefixAllocation))

		// Explicit cleanup before defer calls
		cancel()
		time.Sleep(100 * time.Millisecond) // Allow goroutines to cleanup (increased from 50ms)
	})
}

func TestBBRCongestionControlAlgorithms(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	// Set up communicator
	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	// Register a test prefix
	err := gcc.CoordinatedRegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)

	allocation := gcc.prefixAllocation["test-prefix"]
	require.NotNil(t, allocation)

	// Test BBR startup phase
	metrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ActiveUploads:        5,
		ThroughputMBps:       50.0,
		LatencyMs:            100.0,
		ErrorRate:            0.001, // Very low error rate
		BandwidthUtilization: 0.8,
	}

	initialBandwidth := allocation.AllocatedBandwidthMBps

	gcc.ApplyBBRCongestionControl(allocation, metrics)

	// Verify the BBR algorithm executed (bandwidth allocation should change)
	assert.NotEqual(t, allocation.AllocatedBandwidthMBps, initialBandwidth)

	// Verify BBR mode is maintained or transitions properly
	enhancedMetrics := gcc.GetEnhancedMetrics()
	assert.True(t, enhancedMetrics.BBRMode >= BBRModeStartup && enhancedMetrics.BBRMode <= BBRModeProbeRTT)

	// Verify RTT estimate is updated
	assert.Greater(t, enhancedMetrics.GlobalRTTEstimate, time.Duration(0))
}

func TestBBRModeTransitions(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	err := gcc.CoordinatedRegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)

	allocation := gcc.prefixAllocation["test-prefix"]

	// Start in startup mode
	assert.Equal(t, BBRModeStartup, gcc.bbrMode)

	// Simulate high bandwidth detection to trigger drain mode
	metrics := &PrefixPerformanceMetrics{
		PrefixID:       "test-prefix",
		ThroughputMBps: 80.0, // High throughput
		ErrorRate:      0.001,
		LatencyMs:      50.0,
	}

	gcc.ApplyBBRCongestionControl(allocation, metrics)

	// Should transition to drain mode when bandwidth threshold is reached
	if gcc.adaptiveParameters != nil && gcc.adaptiveParameters.BTLBandwidthFilter != nil {
		maxBW := gcc.adaptiveParameters.BTLBandwidthFilter.GetMaxBandwidth()
		if maxBW > 0 && metrics.ThroughputMBps > maxBW*0.75 {
			assert.Equal(t, BBRModeDrain, gcc.bbrMode)
		}
	}
}

func TestCrossPrefixBandwidthCoordination(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	// Register multiple prefixes with different utilizations
	prefixes := []struct {
		id          string
		capacity    float64
		utilization float64
	}{
		{"high-util", 100.0, 0.95}, // Overutilized
		{"low-util", 100.0, 0.30},  // Underutilized
		{"med-util", 100.0, 0.60},  // Normal
	}

	for _, prefix := range prefixes {
		err := gcc.CoordinatedRegisterPrefix(prefix.id, prefix.capacity)
		require.NoError(t, err)

		// Set utilization
		allocation := gcc.prefixAllocation[prefix.id]
		allocation.Utilization = prefix.utilization
	}

	// Create coordinator for testing
	coordinator := NewCrossPrefixCongestionCoordinator(gcc, communicator)

	// Test bandwidth redistribution
	initialHighUtil := gcc.prefixAllocation["high-util"].AllocatedBandwidthMBps
	initialLowUtil := gcc.prefixAllocation["low-util"].AllocatedBandwidthMBps

	gcc.redistributeBandwidthFairly(coordinator)

	// High utilization prefix should get more bandwidth
	// Low utilization prefix should give some bandwidth
	newHighUtil := gcc.prefixAllocation["high-util"].AllocatedBandwidthMBps
	newLowUtil := gcc.prefixAllocation["low-util"].AllocatedBandwidthMBps

	assert.Greater(t, newHighUtil, initialHighUtil)
	assert.Less(t, newLowUtil, initialLowUtil)
}

func TestFairnessIndexCalculation(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	// Test with equal allocations (should have high fairness)
	for i := 0; i < 3; i++ {
		prefixID := fmt.Sprintf("prefix-%d", i)
		err := gcc.CoordinatedRegisterPrefix(prefixID, 100.0)
		require.NoError(t, err)

		allocation := gcc.prefixAllocation[prefixID]
		allocation.AllocatedBandwidthMBps = 50.0
		allocation.Utilization = 0.8
	}

	fairnessIndex := gcc.calculateFairnessIndex()
	assert.Greater(t, fairnessIndex, 0.9) // Should be high for equal allocations

	// Test with unequal allocations (should have lower fairness)
	gcc.prefixAllocation["prefix-0"].AllocatedBandwidthMBps = 20.0
	gcc.prefixAllocation["prefix-1"].AllocatedBandwidthMBps = 50.0
	gcc.prefixAllocation["prefix-2"].AllocatedBandwidthMBps = 80.0

	fairnessIndex = gcc.calculateFairnessIndex()
	assert.Less(t, fairnessIndex, 0.9) // Should be lower for unequal allocations
}

func TestCongestionEventTracking(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	coordinator := NewCrossPrefixCongestionCoordinator(gcc, communicator)

	// Simulate congestion alert message
	congestionMsg := &CongestionCoordinationMessage{
		Type:            CongestionMsgCongestionAlert,
		SourcePrefixID:  "test-prefix",
		CongestionLevel: 0.7, // High congestion
		Timestamp:       time.Now(),
	}

	initialEvents := len(gcc.congestionEventHistory)

	gcc.handleCongestionAlert(coordinator, congestionMsg)

	// Should record the congestion event
	assert.Equal(t, initialEvents+1, len(gcc.congestionEventHistory))

	lastEvent := gcc.congestionEventHistory[len(gcc.congestionEventHistory)-1]
	assert.Equal(t, CongestionEventBandwidth, lastEvent.EventType)
	assert.Equal(t, "test-prefix", lastEvent.PrefixID)
	assert.Equal(t, 0.7, lastEvent.LossRate)
}

func TestBandwidthFilterIntegration(t *testing.T) {
	bf := NewBandwidthFilter(time.Second * 5) // Shorter window for testing

	now := time.Now()

	// Add samples with increasing bandwidth
	for i := 0; i < 5; i++ {
		sample := BandwidthSample{
			Timestamp:     now.Add(time.Duration(i) * time.Second),
			BandwidthMBps: float64(50 + i*10),
			RTT:           time.Millisecond * 100,
			InFlight:      5,
		}
		bf.AddSample(sample)
	}

	maxBandwidth := bf.GetMaxBandwidth()
	assert.Equal(t, 90.0, maxBandwidth) // Should be the highest sample

	// Add old samples that should be filtered out (outside 5 second window)
	oldSample := BandwidthSample{
		Timestamp:     now.Add(-time.Second * 10), // 10 seconds ago, outside 5 second window
		BandwidthMBps: 200.0,                      // Higher but old
		RTT:           time.Millisecond * 100,
		InFlight:      5,
	}
	bf.AddSample(oldSample)

	// Should still be 90.0 since old sample is filtered out
	maxBandwidth = bf.GetMaxBandwidth()
	assert.Equal(t, 90.0, maxBandwidth)
}

func TestDeliveryRateEstimator(t *testing.T) {
	estimator := NewDeliveryRateEstimator()

	assert.NotNil(t, estimator)
	assert.Equal(t, 0.0, estimator.currentRate)
	assert.Equal(t, 0.0, estimator.maxDeliveryRate)
	assert.Equal(t, time.Millisecond*50, estimator.rttEstimate)
}

func TestEnhancedMetricsReporting(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	// Register a prefix and update metrics
	err := gcc.CoordinatedRegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)

	metrics := gcc.GetEnhancedMetrics()

	// Verify enhanced metrics structure
	assert.NotNil(t, metrics)
	assert.Equal(t, BBRModeStartup, metrics.BBRMode)
	assert.True(t, metrics.CrossPrefixActive)
	assert.Equal(t, 0, metrics.CongestionEvents)
	assert.Equal(t, 2.77, metrics.PacingGain)
	assert.Equal(t, 2.0, metrics.CWNDGain)

	// Verify it includes base congestion metrics
	assert.Equal(t, config.GlobalCongestionWindow, metrics.GlobalCongestionWindow)
	assert.Equal(t, config.GlobalCongestionWindow/2, metrics.SlowStartThreshold)
}

func TestCrossPrefixCongestionCoordinator(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	coordinator := NewCrossPrefixCongestionCoordinator(gcc, communicator)

	assert.NotNil(t, coordinator)
	assert.Equal(t, gcc, coordinator.gcc)
	assert.Equal(t, communicator, coordinator.communicator)
	assert.Equal(t, 0.8, coordinator.globalTarget)
	assert.Equal(t, 0.3, coordinator.fairnessWeight)
	assert.NotNil(t, coordinator.coordMessages)
	assert.NotNil(t, coordinator.prefixStates)
}

func TestSystemUtilizationCalculation(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)

	commConfig := DefaultCommunicationConfig()
	communicator := NewCrossPrefixCommunicator(ctx, commConfig)
	_ = communicator.Start()
	defer func() { _ = communicator.Stop() }()

	gcc.SetCommunicator(communicator)

	// Register prefixes with known allocations and utilizations
	prefixes := []struct {
		id          string
		capacity    float64
		utilization float64
	}{
		{"prefix-1", 100.0, 0.8},
		{"prefix-2", 100.0, 0.6},
		{"prefix-3", 100.0, 0.9},
	}

	for _, prefix := range prefixes {
		err := gcc.CoordinatedRegisterPrefix(prefix.id, prefix.capacity)
		require.NoError(t, err)

		allocation := gcc.prefixAllocation[prefix.id]
		allocation.AllocatedBandwidthMBps = prefix.capacity
		allocation.Utilization = prefix.utilization
	}

	totalUtilization := gcc.calculateTotalUtilization()

	// Expected: (100*0.8 + 100*0.6 + 100*0.9) / (100 + 100 + 100) = 230/300 = 0.7667
	expected := (100*0.8 + 100*0.6 + 100*0.9) / (100 + 100 + 100)
	assert.InDelta(t, expected, totalUtilization, 0.01)
}
