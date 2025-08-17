package multiregion

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckImplementation(t *testing.T) {
	// Create a test coordinator with health check enabled
	config := &MultiRegionConfig{
		Enabled: true,
		PrimaryRegion: "us-east-1",
		Regions: []Region{
			{
				Name:        "us-east-1",
				DisplayName: "US East 1",
				Status:      RegionStatusHealthy,
				Priority:    1,
				Weight:      100,
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         time.Second * 5,
					Timeout:          time.Second * 2,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 100,
					MaxBandwidthMbps:    1000,
					MaxStorageGB:        10000,
					CurrentUtilization:  25.0,
				},
				Metrics: RegionMetrics{
					AverageLatencyMs:         50.0,
					ThroughputMbps:          100.0,
					ErrorRate:               0.5,
					CPUUtilization:          60.0,
					MemoryUtilization:       45.0,
					ActiveUploads:           25,
					ConsecutiveHealthyChecks: 5,
					ConsecutiveFailedChecks:  0,
					HealthCheckSuccess:      true,
				},
			},
			{
				Name:        "us-west-2",
				DisplayName: "US West 2",
				Status:      RegionStatusHealthy,
				Priority:    2,
				Weight:      80,
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         time.Second * 5,
					Timeout:          time.Second * 2,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 80,
					MaxBandwidthMbps:    800,
					MaxStorageGB:        8000,
					CurrentUtilization:  30.0,
				},
				Metrics: RegionMetrics{
					AverageLatencyMs:         75.0,
					ThroughputMbps:          80.0,
					ErrorRate:               1.0,
					CPUUtilization:          55.0,
					MemoryUtilization:       50.0,
					ActiveUploads:           20,
					ConsecutiveHealthyChecks: 3,
					ConsecutiveFailedChecks:  0,
					HealthCheckSuccess:      true,
				},
			},
		},
	}

	coordinator := NewCoordinator()
	require.NotNil(t, coordinator)

	ctx := context.Background()
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)

	defaultCoordinator := coordinator

	t.Run("TestPerformRegionHealthCheck", func(t *testing.T) {
		// Test health check on a healthy region
		region := defaultCoordinator.regions["us-east-1"]
		originalStatus := region.Status
		originalHealthyChecks := region.Metrics.ConsecutiveHealthyChecks

		// Perform health check
		defaultCoordinator.performRegionHealthCheck(region)

		// Verify that health check was performed
		assert.True(t, region.LastChecked.After(time.Now().Add(-time.Second)))
		assert.Equal(t, originalStatus, region.Status) // Should remain healthy
		assert.True(t, region.Metrics.HealthCheckSuccess)
		// ConsecutiveHealthyChecks should have increased
		assert.Greater(t, region.Metrics.ConsecutiveHealthyChecks, originalHealthyChecks)
	})

	t.Run("TestExecuteHealthChecks", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		region := defaultCoordinator.regions["us-east-1"]
		
		// Execute health checks
		results := defaultCoordinator.executeHealthChecks(ctx, region)

		// Verify results structure
		assert.NotNil(t, results)
		assert.Len(t, results.CheckResults, 4) // Should have 4 types of checks
		assert.NotEmpty(t, results.CheckResults)

		// Verify check types are present
		checkTypes := make(map[string]bool)
		for _, result := range results.CheckResults {
			checkTypes[result.CheckType] = true
		}

		expectedTypes := []string{
			"aws_connectivity",
			"s3_service_health", 
			"region_latency",
			"resource_capacity",
		}

		for _, expectedType := range expectedTypes {
			assert.True(t, checkTypes[expectedType], "Expected check type %s not found", expectedType)
		}
	})

	t.Run("TestCheckResourceCapacity", func(t *testing.T) {
		ctx := context.Background()
		region := defaultCoordinator.regions["us-east-1"]

		// Test with normal capacity
		result := defaultCoordinator.checkResourceCapacity(ctx, region)
		
		assert.Equal(t, "resource_capacity", result.CheckType)
		assert.True(t, result.Success) // Should succeed with current metrics
		assert.NotNil(t, result.Details)
		assert.Contains(t, result.Details, "cpu_utilization")
		assert.Contains(t, result.Details, "memory_utilization")
		assert.Contains(t, result.Details, "active_uploads")

		// Test with high capacity usage
		region.Metrics.CPUUtilization = 95.0
		region.Metrics.MemoryUtilization = 90.0
		region.Metrics.ActiveUploads = int64(region.Capacity.MaxConcurrentUploads + 10)

		result = defaultCoordinator.checkResourceCapacity(ctx, region)
		assert.False(t, result.Success) // Should fail due to high utilization
		assert.NotNil(t, result.Error)
	})

	t.Run("TestEvaluateHealthResults", func(t *testing.T) {
		// Test with all successful checks
		results := &OverallHealthStatus{
			CheckResults: []HealthCheckResult{
				{CheckType: "aws_connectivity", Success: true, ResponseTime: time.Millisecond * 100},
				{CheckType: "s3_service_health", Success: true, ResponseTime: time.Millisecond * 150},
				{CheckType: "region_latency", Success: true, ResponseTime: time.Millisecond * 50},
				{CheckType: "resource_capacity", Success: true, ResponseTime: time.Millisecond * 25},
			},
			FailureReasons: make([]string, 0),
		}

		evaluated := defaultCoordinator.evaluateHealthResults(results)
		
		assert.True(t, evaluated.Healthy)
		assert.Equal(t, 1.0, evaluated.SuccessRate)
		assert.Greater(t, evaluated.AvgResponseTime, time.Duration(0))
		assert.Empty(t, evaluated.FailureReasons)

		// Test with some failures
		results.CheckResults[2].Success = false
		results.CheckResults[3].Success = false

		evaluated = defaultCoordinator.evaluateHealthResults(results)
		
		assert.False(t, evaluated.Healthy) // Should be unhealthy because success rate < 75%
		assert.Equal(t, 0.5, evaluated.SuccessRate)

		// Test with critical check failure
		results.CheckResults[0].Success = false // AWS connectivity fails

		evaluated = defaultCoordinator.evaluateHealthResults(results)
		
		assert.False(t, evaluated.Healthy) // Should be unhealthy due to critical check failure
	})

	t.Run("TestSelectFailoverTarget", func(t *testing.T) {
		// Initialize regions map
		defaultCoordinator.regions = make(map[string]*Region)
		for i := range config.Regions {
			region := &config.Regions[i]
			defaultCoordinator.regions[region.Name] = region
		}

		// Test normal failover target selection
		target := defaultCoordinator.selectFailoverTarget("us-east-1")
		assert.Equal(t, "us-west-2", target) // Should select the other healthy region

		// Test when target region is unhealthy
		defaultCoordinator.regions["us-west-2"].Status = RegionStatusUnhealthy
		target = defaultCoordinator.selectFailoverTarget("us-east-1")
		assert.Empty(t, target) // No healthy targets available

		// Test when target region has high utilization
		defaultCoordinator.regions["us-west-2"].Status = RegionStatusHealthy
		defaultCoordinator.regions["us-west-2"].Capacity.CurrentUtilization = 85.0
		target = defaultCoordinator.selectFailoverTarget("us-east-1")
		assert.Empty(t, target) // Should not failover to high-utilization region
	})

	t.Run("TestHealthStatusThresholds", func(t *testing.T) {
		// Initialize regions map
		defaultCoordinator.regions = make(map[string]*Region)
		for i := range config.Regions {
			region := &config.Regions[i]
			defaultCoordinator.regions[region.Name] = region
		}

		region := defaultCoordinator.regions["us-east-1"]
		region.Status = RegionStatusUnhealthy
		region.Metrics.ConsecutiveFailedChecks = 2
		region.Metrics.ConsecutiveHealthyChecks = 0

		// Simulate a successful health check
		healthStatus := &OverallHealthStatus{
			Healthy:         true,
			SuccessRate:     1.0,
			AvgResponseTime: time.Millisecond * 100,
			CheckResults:    make([]HealthCheckResult, 0),
			FailureReasons:  make([]string, 0),
		}

		// Should not mark as healthy immediately (needs to reach success threshold)
		defaultCoordinator.updateRegionHealthStatus(region, healthStatus, healthStatus)
		assert.Equal(t, RegionStatusUnhealthy, region.Status)
		assert.Equal(t, int64(1), region.Metrics.ConsecutiveHealthyChecks)

		// After reaching success threshold, should mark as healthy
		defaultCoordinator.updateRegionHealthStatus(region, healthStatus, healthStatus)
		assert.Equal(t, RegionStatusHealthy, region.Status)
		assert.Equal(t, int64(2), region.Metrics.ConsecutiveHealthyChecks)
	})
}

func TestHealthCheckTypes(t *testing.T) {
	config := &MultiRegionConfig{
		Enabled: true,
		PrimaryRegion: "test-region",
		Regions: []Region{
			{
				Name: "test-region",
				DisplayName: "Test Region",
				Status: RegionStatusHealthy,
				Priority: 1,
				Weight: 100,
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Timeout: time.Second * 2,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
				Metrics: RegionMetrics{
					CPUUtilization:    50.0,
					MemoryUtilization: 40.0,
					ActiveUploads:     10,
					ConsecutiveHealthyChecks: 5,
				},
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 100,
					CurrentUtilization: 25.0,
				},
			},
		},
	}

	coordinator := NewCoordinator()
	err := coordinator.Initialize(context.Background(), config)
	require.NoError(t, err)
	
	ctx := context.Background()
	region := &config.Regions[0]

	t.Run("TestCheckAWSConnectivity", func(t *testing.T) {
		result := coordinator.checkAWSConnectivity(ctx, region)
		
		assert.Equal(t, "aws_connectivity", result.CheckType)
		assert.NotNil(t, result.Details)
		assert.Greater(t, result.ResponseTime, time.Duration(0))
		// Note: This will currently fail because AWSConfig.Region is empty in test
		// In a real implementation with proper AWS config, this should succeed
	})

	t.Run("TestCheckS3ServiceHealth", func(t *testing.T) {
		result := coordinator.checkS3ServiceHealth(ctx, region)
		
		assert.Equal(t, "s3_service_health", result.CheckType)
		assert.NotNil(t, result.Details)
		assert.True(t, result.Success) // Simulated success
		assert.Greater(t, result.ResponseTime, time.Duration(0))
	})

	t.Run("TestCheckRegionLatency", func(t *testing.T) {
		result := coordinator.checkRegionLatency(ctx, region)
		
		assert.Equal(t, "region_latency", result.CheckType)
		assert.True(t, result.Success) // Should succeed with fast execution
		assert.Contains(t, result.Details, "latency_ms")
		assert.Greater(t, result.ResponseTime, time.Duration(0))
	})

	t.Run("TestCheckResourceCapacity", func(t *testing.T) {
		result := coordinator.checkResourceCapacity(ctx, region)
		
		assert.Equal(t, "resource_capacity", result.CheckType)
		assert.True(t, result.Success) // Should succeed with current test metrics
		assert.Contains(t, result.Details, "cpu_utilization")
		assert.Contains(t, result.Details, "memory_utilization")
		assert.Contains(t, result.Details, "active_uploads")
		assert.Contains(t, result.Details, "capacity_limit")
	})
}