package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPipelineCoordinator(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	
	coordinator := NewPipelineCoordinator(ctx, config)
	
	assert.NotNil(t, coordinator)
	assert.Equal(t, config, coordinator.config)
	assert.NotNil(t, coordinator.scheduler)
	assert.NotNil(t, coordinator.congestionControl)
	assert.NotNil(t, coordinator.metrics)
	assert.False(t, coordinator.active)
}

func TestNewPipelineCoordinatorWithNilConfig(t *testing.T) {
	ctx := context.Background()
	
	coordinator := NewPipelineCoordinator(ctx, nil)
	
	assert.NotNil(t, coordinator)
	assert.NotNil(t, coordinator.config)
	assert.Equal(t, DefaultCoordinationConfig().PipelineDepth, coordinator.config.PipelineDepth)
}

func TestDefaultCoordinationConfig(t *testing.T) {
	config := DefaultCoordinationConfig()
	
	assert.Equal(t, 16, config.PipelineDepth)
	assert.Equal(t, 32, config.GlobalCongestionWindow)
	assert.Equal(t, "adaptive", config.Strategy)
	assert.Equal(t, 16, config.MaxActivePrefixes)
	assert.Equal(t, "fair_share", config.BandwidthStrategy)
	assert.Equal(t, time.Second*2, config.UpdateInterval)
	assert.True(t, config.EnableAdvancedFlowControl)
}

func TestPipelineCoordinatorStartStop(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	// Test starting coordinator
	err := coordinator.Start()
	assert.NoError(t, err)
	assert.True(t, coordinator.active)
	
	// Test starting already active coordinator
	err = coordinator.Start()
	assert.NoError(t, err) // Should be idempotent
	
	// Test stopping coordinator
	err = coordinator.Stop()
	assert.NoError(t, err)
	assert.False(t, coordinator.active)
	
	// Test stopping already stopped coordinator
	err = coordinator.Stop()
	assert.NoError(t, err) // Should be idempotent
}

func TestPipelineCoordinatorRegisterPrefix(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	// Test registering prefix when coordinator is not active
	err := coordinator.RegisterPrefix("test-prefix", 100.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator_inactive")
	
	// Start coordinator and test registration
	err = coordinator.Start()
	require.NoError(t, err)
	
	err = coordinator.RegisterPrefix("test-prefix", 100.0)
	assert.NoError(t, err)
	
	// Verify prefix channel was created
	coordinator.mu.RLock()
	channel, exists := coordinator.prefixChannels["test-prefix"]
	coordinator.mu.RUnlock()
	
	assert.True(t, exists)
	assert.NotNil(t, channel)
	assert.Equal(t, config.PipelineDepth, cap(channel))
	
	_ = coordinator.Stop()
}

func TestPipelineCoordinatorScheduleUpload(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	// Test scheduling when coordinator is not active
	err := coordinator.ScheduleUpload(upload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator_inactive")
	
	// Start coordinator and register prefix
	err = coordinator.Start()
	require.NoError(t, err)
	
	err = coordinator.RegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)
	
	// Test successful scheduling
	err = coordinator.ScheduleUpload(upload)
	assert.NoError(t, err)
	assert.Equal(t, "test-prefix", upload.PrefixID)
	assert.NotZero(t, upload.ScheduledAt)
	assert.NotZero(t, upload.BandwidthAllocation)
	assert.NotZero(t, upload.CongestionWindow)
	
	_ = coordinator.Stop()
}

func TestPipelineCoordinatorGetMetrics(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	metrics := coordinator.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, 1.0, metrics.LoadBalanceEfficiency)
	assert.Equal(t, 0, metrics.ActivePrefixes)
}

func TestPipelineCoordinatorUpdatePrefixMetrics(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	err := coordinator.Start()
	require.NoError(t, err)
	
	err = coordinator.RegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)
	
	metrics := &PrefixPerformanceMetrics{
		PrefixID:       "test-prefix",
		ActiveUploads:  5,
		ThroughputMBps: 50.0,
		LatencyMs:      100.0,
		ErrorRate:      0.01,
	}
	
	// This should not panic or error
	coordinator.UpdatePrefixMetrics("test-prefix", metrics)
	
	_ = coordinator.Stop()
}

func TestPipelineCoordinatorConcurrency(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	coordinator := NewPipelineCoordinator(ctx, config)
	
	err := coordinator.Start()
	require.NoError(t, err)
	
	// Register multiple prefixes concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id)
			err := coordinator.RegisterPrefix(prefixID, 100.0)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
	
	// Verify all prefixes were registered
	coordinator.mu.RLock()
	assert.Equal(t, 10, len(coordinator.prefixChannels))
	coordinator.mu.RUnlock()
	
	// Schedule uploads concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			upload := &ScheduledUpload{
				ArchivePath:   fmt.Sprintf("/test/archive-%d.tar", id),
				Priority:      3,
				EstimatedSize: 1024 * 1024,
			}
			err := coordinator.ScheduleUpload(upload)
			// Some uploads might fail due to queue full, which is expected
			if err != nil {
				assert.Contains(t, err.Error(), "prefix_queue_full")
			}
		}(i)
	}
	wg.Wait()
	
	_ = coordinator.Stop()
}

func TestCoordinationError(t *testing.T) {
	// Test basic error
	err := &CoordinationError{
		Type:    "test_error",
		Message: "test message",
	}
	assert.Equal(t, "test_error: test message", err.Error())
	
	// Test error with prefix ID
	err = &CoordinationError{
		Type:     "test_error",
		Message:  "test message",
		PrefixID: "test-prefix",
	}
	assert.Equal(t, "test_error [test-prefix]: test message", err.Error())
}

func TestNewTransferScheduler(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.prefixMetrics)
	assert.NotNil(t, scheduler.networkProfile)
	assert.NotNil(t, scheduler.globalState)
	assert.NotNil(t, scheduler.loadBalancer)
	assert.Equal(t, config, scheduler.config)
}

func TestNewGlobalCongestionController(t *testing.T) {
	config := DefaultCoordinationConfig()
	gcc := NewGlobalCongestionController(config)
	
	assert.NotNil(t, gcc)
	assert.Equal(t, config.GlobalCongestionWindow, gcc.globalCongestionWindow)
	assert.Equal(t, config.GlobalCongestionWindow/2, gcc.slowStartThreshold)
	assert.NotNil(t, gcc.prefixAllocation)
	assert.Equal(t, CongestionStateSlowStart, gcc.congestionState)
	assert.NotNil(t, gcc.adaptiveParameters)
}

func TestNewNetworkProfile(t *testing.T) {
	profile := NewNetworkProfile()
	
	assert.NotNil(t, profile)
	assert.Equal(t, 100.0, profile.EstimatedBandwidthMBps)
	assert.Equal(t, time.Millisecond*50, profile.BaselineRTT)
	assert.Equal(t, 0.1, profile.NetworkVariance)
	assert.Equal(t, 0.8, profile.CongestionThreshold)
	assert.Equal(t, 50, profile.MaxHistorySize)
	assert.Equal(t, TrendUnknown, profile.BandwidthTrend)
	assert.Equal(t, TrendUnknown, profile.LatencyTrend)
	assert.Equal(t, 0.5, profile.LearningConfidence)
}

func TestNewGlobalTransferState(t *testing.T) {
	state := NewGlobalTransferState()
	
	assert.NotNil(t, state)
	assert.NotNil(t, state.ActivePrefixes)
	assert.Equal(t, 0, state.TotalActiveUploads)
	assert.Equal(t, 0.0, state.GlobalThroughputMBps)
	assert.Equal(t, 0.0, state.GlobalErrorRate)
	assert.Equal(t, 1.0, state.LoadBalanceEfficiency)
}

func TestNewPrefixLoadBalancer(t *testing.T) {
	balancer := NewPrefixLoadBalancer(LoadBalanceAdaptive)
	
	assert.NotNil(t, balancer)
	assert.Equal(t, LoadBalanceAdaptive, balancer.strategy)
	assert.NotNil(t, balancer.prefixWeights)
	assert.NotNil(t, balancer.prefixCapacities)
	assert.Equal(t, 0.2, balancer.rebalanceThreshold)
	assert.Equal(t, time.Second*30, balancer.rebalanceInterval)
}

func TestNewAdaptiveParameters(t *testing.T) {
	params := NewAdaptiveParameters()
	
	assert.NotNil(t, params)
	assert.Equal(t, 0.1, params.LearningRate)
	assert.Equal(t, 0.05, params.BandwidthProbingRate)
	assert.Equal(t, 0.8, params.CongestionSensitivity)
	assert.Equal(t, 1.2, params.RecoveryAggressiveness)
	assert.NotNil(t, params.BTLBandwidthFilter)
	assert.Equal(t, time.Millisecond*10, params.RTTMin)
	assert.Equal(t, time.Second*8, params.CycleLength)
}

func TestNewBandwidthFilter(t *testing.T) {
	filter := NewBandwidthFilter(time.Second * 10)
	
	assert.NotNil(t, filter)
	assert.NotNil(t, filter.samples)
	assert.Equal(t, time.Second*10, filter.maxWindow)
	assert.Equal(t, 0.0, filter.currentMax)
}

func TestNewCoordinationMetrics(t *testing.T) {
	metrics := NewCoordinationMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, 0.0, metrics.CoordinationOverheadPercent)
	assert.Equal(t, 1.0, metrics.LoadBalanceEfficiency)
	assert.Equal(t, 0.0, metrics.PipelineUtilization)
	assert.Equal(t, 1.0, metrics.ImprovementFactor)
}

func TestPipelineCoordinatorMetricsCollection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	
	config := DefaultCoordinationConfig()
	config.UpdateInterval = time.Millisecond * 100 // Fast update for testing
	coordinator := NewPipelineCoordinator(ctx, config)
	
	err := coordinator.Start()
	require.NoError(t, err)
	
	// Register a prefix
	err = coordinator.RegisterPrefix("test-prefix", 100.0)
	require.NoError(t, err)
	
	// Wait for a few metrics collection cycles
	time.Sleep(time.Millisecond * 300)
	
	metrics := coordinator.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, 1, metrics.ActivePrefixes) // Should reflect registered prefix
	
	_ = coordinator.Stop()
}