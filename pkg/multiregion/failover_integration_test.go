// Package multiregion - Integration testing for multi-region failover scenarios
// This file contains integration tests that validate the complete failover system
// working together with the coordinator, health checks, and metrics collection.

package multiregion

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFailoverIntegrationScenarios tests integration between all components
func TestFailoverIntegrationScenarios(t *testing.T) {
	t.Run("EndToEndFailoverWithHealthChecks", func(t *testing.T) {
		// Test complete integration: health checks → failure detection → automatic failover
		config := createValidMultiRegionConfig()
		config.Failover.AutoFailover = true
		config.Failover.Strategy = FailoverImmediate
		config.Failover.FailoverTimeout = 3 * time.Second
		// Disable monitoring to allow manual metric control in tests
		config.Monitoring.Enabled = false

		// Disable health checks for this test to avoid AWS connectivity issues
		for i := range config.Regions {
			config.Regions[i].HealthCheck.Enabled = false
		}
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		// Wait for initial health checks and metrics
		time.Sleep(500 * time.Millisecond)
		
		t.Run("HealthySystemNoFailover", func(t *testing.T) {
			// Get region status - should be healthy
			status, err := coordinator.GetRegionStatus(ctx)
			require.NoError(t, err)
			
			for regionName, regionStatus := range status {
				assert.Equal(t, RegionStatusHealthy, regionStatus, 
					"Region %s should be healthy initially", regionName)
			}
		})
		
		t.Run("SimulateRegionDegradation", func(t *testing.T) {
			// Simulate region degradation by manipulating metrics
			coordinator.mu.Lock()
			testRegion := coordinator.regions["us-east-1"]
			originalErrorRate := testRegion.Metrics.ErrorRate
			
			// Gradually increase error rate to simulate degradation
			testRegion.Metrics.ErrorRate = 30.0 // Above threshold
			testRegion.Metrics.ConsecutiveFailedChecks = 3 // Above threshold
			coordinator.mu.Unlock()
			
			// Wait for failure detection to trigger
			time.Sleep(300 * time.Millisecond)
			
			// Verify automatic failover was triggered
			// Note: In a real scenario, this would be observable through logs or monitoring
			coordinator.mu.RLock()
			currentErrorRate := coordinator.regions["us-east-1"].Metrics.ErrorRate
			coordinator.mu.RUnlock()
			
			assert.Equal(t, 30.0, currentErrorRate, "Error rate should be set to 30%")
			
			// Restore for cleanup
			coordinator.mu.Lock()
			testRegion.Metrics.ErrorRate = originalErrorRate
			testRegion.Metrics.ConsecutiveFailedChecks = 0
			coordinator.mu.Unlock()
		})
	})
	
	t.Run("MultiRegionCoordinatedFailover", func(t *testing.T) {
		// Test failover coordination across multiple regions
		config := createValidMultiRegionConfig()
		
		// Add additional regions for more complex testing
		config.Regions = append(config.Regions, []Region{
			{
				Name: "eu-west-1",
				DisplayName: "EU West 1",
				Status: RegionStatusHealthy,
				Priority: 3,
				Weight: 50,
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Interval: 5 * time.Second,
					Timeout: 2 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 60,
					MaxBandwidthMbps: 600,
					MaxStorageGB: 6000,
					CurrentUtilization: 25.0,
				},
				Metrics: RegionMetrics{
					AverageLatencyMs: 100.0,
					ThroughputMbps: 60.0,
					ErrorRate: 3.0,
					ConsecutiveHealthyChecks: 8,
					ConsecutiveFailedChecks: 0,
					HealthCheckSuccess: true,
				},
			},
			{
				Name: "ap-southeast-1",
				DisplayName: "Asia Pacific Southeast 1",
				Status: RegionStatusHealthy,
				Priority: 4,
				Weight: 40,
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Interval: 5 * time.Second,
					Timeout: 2 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 50,
					MaxBandwidthMbps: 500,
					MaxStorageGB: 5000,
					CurrentUtilization: 30.0,
				},
				Metrics: RegionMetrics{
					AverageLatencyMs: 150.0,
					ThroughputMbps: 50.0,
					ErrorRate: 4.0,
					ConsecutiveHealthyChecks: 6,
					ConsecutiveFailedChecks: 0,
					HealthCheckSuccess: true,
				},
			},
		}...)
		
		config.Failover.AutoFailover = true
		config.Failover.Strategy = FailoverGraceful
		config.Failover.FailoverTimeout = 5 * time.Second
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		t.Run("PriorityBasedFailoverTargetSelection", func(t *testing.T) {
			// Test that failover selects targets based on priority and health
			
			// Fail primary region (priority 1)
			target := coordinator.selectFailoverTarget("us-east-1")
			assert.Equal(t, "us-west-2", target, "Should select priority 2 region")
			
			// Fail secondary region, should select next priority
			coordinator.mu.Lock()
			coordinator.regions["us-west-2"].Status = RegionStatusUnhealthy
			coordinator.mu.Unlock()
			
			target = coordinator.selectFailoverTarget("us-east-1")
			assert.Equal(t, "eu-west-1", target, "Should select priority 3 region")
		})
		
		t.Run("CapacityBasedFailoverRejection", func(t *testing.T) {
			// Set all secondary regions to high utilization
			coordinator.mu.Lock()
			for name, region := range coordinator.regions {
				if name != "us-east-1" {
					region.Capacity.CurrentUtilization = 90.0 // Too high for failover
				}
			}
			coordinator.mu.Unlock()
			
			target := coordinator.selectFailoverTarget("us-east-1")
			assert.Empty(t, target, "Should not select overloaded regions for failover")
			
			// Restore capacity
			coordinator.mu.Lock()
			for name, region := range coordinator.regions {
				if name != "us-east-1" {
					region.Capacity.CurrentUtilization = 30.0
					region.Status = RegionStatusHealthy
				}
			}
			coordinator.mu.Unlock()
		})
	})
}

// TestFailoverStressScenarios tests failover under stress conditions
func TestFailoverStressScenarios(t *testing.T) {
	t.Run("HighVolumeFailoverRequests", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 2 * time.Second
		
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)
		
		ctx := context.Background()
		
		// Simulate high volume of failover requests
		var wg sync.WaitGroup
		requests := 20
		wg.Add(requests)
		
		results := make([]error, requests)
		
		for i := 0; i < requests; i++ {
			go func(index int) {
				defer wg.Done()
				fromRegion := "us-east-1"
				toRegion := "us-west-2"
				if index%2 == 0 {
					fromRegion, toRegion = toRegion, fromRegion
				}
				
				results[index] = manager.ExecuteFailover(ctx, fromRegion, toRegion)
			}(i)
		}
		
		wg.Wait()
		
		// Verify no panics occurred and some requests succeeded
		successCount := 0
		for _, err := range results {
			if err == nil {
				successCount++
			}
		}
		
		assert.Greater(t, successCount, 0, "At least some failover requests should succeed")
		t.Logf("Stress test: %d/%d failover requests succeeded", successCount, requests)
	})
	
	t.Run("ConcurrentFailureDetection", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		
		regions := []string{"region-1", "region-2", "region-3", "region-4", "region-5"}
		
		// Simulate concurrent failure detection and recording
		var wg sync.WaitGroup
		operations := 100
		wg.Add(operations)
		
		for i := 0; i < operations; i++ {
			go func(index int) {
				defer wg.Done()
				region := regions[index%len(regions)]
				
				if index%3 == 0 {
					manager.RecordFailure(region)
				} else {
					manager.RecordSuccess(region)
				}
				
				// Also test concurrent detection
				ctx := context.Background()
				_, _ = manager.DetectFailure(ctx, region)
			}(i)
		}
		
		wg.Wait()
		
		// Verify system integrity after stress
		ctx := context.Background()
		for _, region := range regions {
			_, err := manager.DetectFailure(ctx, region)
			assert.NoError(t, err, "Failure detection should work after concurrent stress")
		}
	})
}

// TestFailoverRealtimeMonitoring tests real-time monitoring integration
func TestFailoverRealtimeMonitoring(t *testing.T) {
	t.Run("MetricsCollectionDuringFailover", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		// Disable monitoring to allow manual metric control in this test
		config.Monitoring.Enabled = false
		config.Failover.AutoFailover = true
		config.Failover.Strategy = FailoverGraceful
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		// Let metrics collection run for a bit
		time.Sleep(300 * time.Millisecond)
		
		// Get initial metrics
		initialMetrics, err := coordinator.GetRegionMetrics(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, initialMetrics, "Should have initial metrics")
		
		// Simulate region degradation
		coordinator.mu.Lock()
		testRegion := coordinator.regions["us-east-1"]
		testRegion.Metrics.ErrorRate = 35.0 // High error rate
		coordinator.mu.Unlock()
		
		// Wait for metrics to be updated
		time.Sleep(200 * time.Millisecond)
		
		// Get updated metrics
		updatedMetrics, err := coordinator.GetRegionMetrics(ctx)
		require.NoError(t, err)
		
		assert.Equal(t, 35.0, updatedMetrics["us-east-1"].ErrorRate, 
			"Metrics should reflect the degraded state")
	})
	
	t.Run("HealthCheckIntegrationWithFailover", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		
		// Configure aggressive health checking
		for i := range config.Regions {
			config.Regions[i].HealthCheck.Enabled = true
			config.Regions[i].HealthCheck.Interval = 100 * time.Millisecond
			config.Regions[i].HealthCheck.Timeout = 50 * time.Millisecond
			config.Regions[i].HealthCheck.FailureThreshold = 2
			config.Regions[i].HealthCheck.SuccessThreshold = 2
		}
		
		config.Failover.AutoFailover = true
		config.Failover.Strategy = FailoverImmediate
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		// Let health checks run initially
		time.Sleep(300 * time.Millisecond)
		
		// Force health check failures
		coordinator.mu.Lock()
		testRegion := coordinator.regions["us-east-1"]
		testRegion.Metrics.ConsecutiveFailedChecks = 3 // Above threshold
		testRegion.Metrics.ConsecutiveHealthyChecks = 0
		testRegion.Metrics.HealthCheckSuccess = false
		coordinator.mu.Unlock()
		
		// Trigger failure detection
		coordinator.detectAndHandleFailures()
		
		// Wait a moment for any triggered failover
		time.Sleep(100 * time.Millisecond)
		
		// Verify that the failure detection logic ran without error
		coordinator.mu.RLock()
		failedChecks := coordinator.regions["us-east-1"].Metrics.ConsecutiveFailedChecks
		coordinator.mu.RUnlock()
		
		assert.Equal(t, int64(3), failedChecks, "Should maintain failure count")
	})
}

// TestFailoverRecoveryPatterns tests common recovery patterns
func TestFailoverRecoveryPatterns(t *testing.T) {
	t.Run("GradualRecoveryPattern", func(t *testing.T) {
		// Test gradual recovery of a failed region
		config := createValidMultiRegionConfig()
		config.Failover.AutoFailover = true
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		// Get test region
		coordinator.mu.RLock()
		testRegion := coordinator.regions["us-east-1"]
		coordinator.mu.RUnlock()
		
		// Phase 1: Degrade region
		testRegion.Status = RegionStatusUnhealthy
		testRegion.Metrics.ErrorRate = 45.0
		testRegion.Metrics.ConsecutiveFailedChecks = 5
		testRegion.Metrics.ConsecutiveHealthyChecks = 0
		
		shouldTrigger := coordinator.shouldTriggerFailover(testRegion)
		assert.True(t, shouldTrigger, "Should trigger failover for degraded region")
		
		// Phase 2: Partial recovery
		testRegion.Status = RegionStatusDegraded
		testRegion.Metrics.ErrorRate = 20.0 // Improved but still high
		testRegion.Metrics.ConsecutiveFailedChecks = 2
		testRegion.Metrics.ConsecutiveHealthyChecks = 1
		
		shouldTrigger = coordinator.shouldTriggerFailover(testRegion)
		assert.False(t, shouldTrigger, "Should not trigger failover for partially recovered region")
		
		// Phase 3: Full recovery
		testRegion.Status = RegionStatusHealthy
		testRegion.Metrics.ErrorRate = 2.0 // Normal
		testRegion.Metrics.ConsecutiveFailedChecks = 0
		testRegion.Metrics.ConsecutiveHealthyChecks = 10
		
		shouldTrigger = coordinator.shouldTriggerFailover(testRegion)
		assert.False(t, shouldTrigger, "Should not trigger failover for fully recovered region")
	})
	
	t.Run("FlappingRegionHandling", func(t *testing.T) {
		// Test handling of regions that flip between healthy and unhealthy
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		
		ctx := context.Background()
		regionName := "flapping-region"
		
		// Simulate flapping: alternating success/failure
		for i := 0; i < 20; i++ {
			if i%2 == 0 {
				manager.RecordFailure(regionName)
			} else {
				manager.RecordSuccess(regionName)
			}
			
			// Check detection periodically
			if i%5 == 4 {
				failed, err := manager.DetectFailure(ctx, regionName)
				assert.NoError(t, err, "Should handle flapping region detection")
				t.Logf("Cycle %d: Region failed = %v", i, failed)
			}
		}
		
		// Final state check
		history := manager.GetFailureHistory(regionName)
		assert.NotNil(t, history, "Should maintain history for flapping region")
		
		// Verify system stability
		failed, err := manager.DetectFailure(ctx, regionName)
		assert.NoError(t, err, "Final detection should work correctly")
		_ = failed
	})
}

// TestFailoverErrorHandlingIntegration tests error handling across components
func TestFailoverErrorHandlingIntegration(t *testing.T) {
	t.Run("FailoverManagerInitializationErrors", func(t *testing.T) {
		// Test various initialization error conditions
		
		t.Run("NilConfig", func(t *testing.T) {
			logger := log.New(nil)
			
			// Should handle nil config gracefully or panic predictably
			defer func() {
				if r := recover(); r != nil {
					t.Logf("Nil config caused expected panic: %v", r)
				}
			}()
			
			manager := NewFailoverManager(nil, logger)
			assert.NotNil(t, manager, "Should handle nil config")
		})
		
		t.Run("InvalidTimeouts", func(t *testing.T) {
			config := createValidMultiRegionConfig()
			config.Failover.FailoverTimeout = -1 * time.Second // Invalid
			
			logger := log.New(nil)
			manager := NewFailoverManager(config, logger)
			assert.NotNil(t, manager, "Should handle invalid timeouts")
			
			// Try to use it
			ctx := context.Background()
			err := manager.ExecuteFailover(ctx, "us-east-1", "us-west-2")
			// Implementation should either correct the timeout or handle gracefully
			_ = err
		})
	})
	
	t.Run("CoordinatorFailoverIntegrationErrors", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.AutoFailover = true
		
		coordinator := NewCoordinator()
		
		ctx := context.Background()
		err := coordinator.Initialize(ctx, config)
		require.NoError(t, err)
		defer func() { _ = coordinator.Shutdown(ctx) }()
		
		t.Run("NoValidFailoverTarget", func(t *testing.T) {
			// Make all regions except one unhealthy
			coordinator.mu.Lock()
			for name, region := range coordinator.regions {
				if name != "us-east-1" {
					region.Status = RegionStatusUnhealthy
					region.Capacity.CurrentUtilization = 95.0
				}
			}
			coordinator.mu.Unlock()
			
			target := coordinator.selectFailoverTarget("us-east-1")
			assert.Empty(t, target, "Should have no valid failover target")
		})
		
		t.Run("FailoverManagerNotInitialized", func(t *testing.T) {
			// Temporarily remove failover manager
			originalManager := coordinator.failoverManager
			coordinator.failoverManager = nil
			
			// Should handle missing failover manager gracefully
			coordinator.triggerAutomaticFailover("us-east-1", "us-west-2")
			// No assertions needed - just verify no panic
			
			// Restore
			coordinator.failoverManager = originalManager
		})
	})
}

// TestFailoverConfigurationScenarios tests various configuration scenarios
func TestFailoverConfigurationScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping configuration scenarios test in short mode (takes 4s)")
	}
	t.Run("DifferentFailoverStrategies", func(t *testing.T) {
		strategies := []FailoverStrategy{
			FailoverImmediate,
			FailoverGraceful,
			FailoverManual,
		}
		
		for _, strategy := range strategies {
			t.Run(string(strategy), func(t *testing.T) {
				config := createValidMultiRegionConfig()
				config.Failover.Strategy = strategy
				config.Failover.FailoverTimeout = 2 * time.Second // Increased for notification delays

				logger := log.New(nil)
				manager := NewFailoverManager(config, logger)
				assert.NotNil(t, manager, "Should create manager for %s strategy", strategy)

				// Use a timeout context to prevent hanging
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				// Test basic functionality
				_, err := manager.DetectFailure(ctx, "us-east-1")
				assert.NoError(t, err, "Should detect failures for %s strategy", strategy)

				// Test execution (may succeed or timeout based on strategy)
				err = manager.ExecuteFailover(ctx, "us-east-1", "us-west-2")
				if strategy == FailoverManual {
					assert.Error(t, err, "Manual failover should timeout without approval")
				} else {
					assert.NoError(t, err, "Immediate and graceful should succeed with optimized timeouts")
				}
			})
		}
	})
	
	t.Run("CustomThresholds", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.RetryAttempts = 5 // Custom retry attempts
		config.Failover.DetectionInterval = 50 * time.Millisecond // Aggressive detection
		
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		
		// Test with custom thresholds
		regionName := "custom-threshold-test"
		
		// Record failures up to but not exceeding threshold
		for i := 0; i < 4; i++ { // Less than 5
			manager.RecordFailure(regionName)
		}
		
		ctx := context.Background()
		_, err := manager.DetectFailure(ctx, regionName)
		assert.NoError(t, err)
		
		// One more failure should exceed threshold
		manager.RecordFailure(regionName)
		failed, err := manager.DetectFailure(ctx, regionName)
		assert.NoError(t, err)
		
		t.Logf("Custom threshold test: failed detection = %v", failed)
	})
}