package multiregion

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCoordinator(t *testing.T) {
	coordinator := NewCoordinator()
	assert.NotNil(t, coordinator)
	assert.NotNil(t, coordinator.regions)
	assert.NotNil(t, coordinator.logger)
	assert.False(t, coordinator.initialized)
}

func TestDefaultCoordinator_Initialize(t *testing.T) {
	tests := []struct {
		name        string
		config      *MultiRegionConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "configuration cannot be nil",
		},
		{
			name: "disabled multi-region",
			config: &MultiRegionConfig{
				Enabled: false,
			},
			expectError: true,
			errorMsg:    "multi-region support is disabled",
		},
		{
			name: "no regions configured",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{},
			},
			expectError: true,
			errorMsg:    "at least one region must be configured",
		},
		{
			name: "empty primary region",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{Name: "us-east-1", Priority: 1, Weight: 50},
				},
				PrimaryRegion: "",
			},
			expectError: true,
			errorMsg:    "primary region must be specified",
		},
		{
			name: "primary region not in list",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{Name: "us-east-1", Priority: 1, Weight: 50},
				},
				PrimaryRegion: "us-west-2",
			},
			expectError: true,
			errorMsg:    "primary region 'us-west-2' not found in regions list",
		},
		{
			name: "valid configuration",
			config: createValidMultiRegionConfig(),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := NewCoordinator()
			ctx := context.Background()
			
			err := coordinator.Initialize(ctx, tt.config)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.False(t, coordinator.initialized)
			} else {
				assert.NoError(t, err)
				assert.True(t, coordinator.initialized)
				assert.NotNil(t, coordinator.config)
				assert.NotNil(t, coordinator.ctx)
				assert.NotNil(t, coordinator.cancel)
				assert.Equal(t, len(tt.config.Regions), len(coordinator.regions))
			}
		})
	}
}

func TestDefaultCoordinator_Initialize_DoubleInit(t *testing.T) {
	coordinator := NewCoordinator()
	ctx := context.Background()
	config := createValidMultiRegionConfig()
	
	// First initialization should succeed
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	assert.True(t, coordinator.initialized)
	
	// Second initialization should fail
	err = coordinator.Initialize(ctx, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator already initialized")
}

func TestDefaultCoordinator_Upload(t *testing.T) {
	coordinator := NewCoordinator()
	ctx := context.Background()
	
	// Test upload without initialization
	request := &UploadRequest{
		FilePath: "/test/file.txt",
		Size:     1024,
		Priority: 5,
	}
	
	result, err := coordinator.Upload(ctx, request)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "coordinator not initialized")
	
	// Initialize coordinator
	config := createValidMultiRegionConfig()
	err = coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	
	// Test upload with nil request
	result, err = coordinator.Upload(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "upload request cannot be nil")
	
	// Test valid upload
	result, err = coordinator.Upload(ctx, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.RequestID)
	assert.NotEmpty(t, result.Region)
	assert.True(t, result.Success)
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.Equal(t, request.Size, result.BytesTransferred)
}

func TestDefaultCoordinator_GetRegionStatus(t *testing.T) {
	coordinator := NewCoordinator()
	ctx := context.Background()
	
	// Test without initialization
	status, err := coordinator.GetRegionStatus(ctx)
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "coordinator not initialized")
	
	// Initialize coordinator
	config := createValidMultiRegionConfig()
	err = coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	
	// Test with initialization
	status, err = coordinator.GetRegionStatus(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, len(config.Regions), len(status))
	
	for _, region := range config.Regions {
		assert.Contains(t, status, region.Name)
	}
}

func TestDefaultCoordinator_GetRegionMetrics(t *testing.T) {
	coordinator := NewCoordinator()
	ctx := context.Background()
	
	// Test without initialization
	metrics, err := coordinator.GetRegionMetrics(ctx)
	assert.Error(t, err)
	assert.Nil(t, metrics)
	assert.Contains(t, err.Error(), "coordinator not initialized")
	
	// Initialize coordinator
	config := createValidMultiRegionConfig()
	err = coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	
	// Test with initialization
	metrics, err = coordinator.GetRegionMetrics(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, len(config.Regions), len(metrics))
	
	for _, region := range config.Regions {
		assert.Contains(t, metrics, region.Name)
	}
}

func TestDefaultCoordinator_Shutdown(t *testing.T) {
	coordinator := NewCoordinator()
	ctx := context.Background()
	
	// Test shutdown without initialization
	err := coordinator.Shutdown(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "coordinator not initialized")
	
	// Initialize coordinator
	config := createValidMultiRegionConfig()
	err = coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	
	// Test successful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	err = coordinator.Shutdown(shutdownCtx)
	assert.NoError(t, err)
	assert.False(t, coordinator.initialized)
}

func TestDefaultCoordinator_ValidateConfig(t *testing.T) {
	coordinator := NewCoordinator()
	
	tests := []struct {
		name        string
		config      *MultiRegionConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid region priority",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{Name: "us-east-1", Priority: 0, Weight: 50}, // Invalid priority
				},
				PrimaryRegion: "us-east-1",
			},
			expectError: true,
			errorMsg:    "region priority must be at least 1",
		},
		{
			name: "invalid region weight",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{Name: "us-east-1", Priority: 1, Weight: 150}, // Invalid weight
				},
				PrimaryRegion: "us-east-1",
			},
			expectError: true,
			errorMsg:    "region weight must be between 0 and 100",
		},
		{
			name: "invalid max concurrent uploads",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{
						Name:     "us-east-1",
						Priority: 1,
						Weight:   50,
						Capacity: RegionCapacity{MaxConcurrentUploads: 0}, // Invalid
					},
				},
				PrimaryRegion: "us-east-1",
			},
			expectError: true,
			errorMsg:    "max concurrent uploads must be at least 1",
		},
		{
			name: "invalid health check interval",
			config: &MultiRegionConfig{
				Enabled: true,
				Regions: []Region{
					{
						Name:     "us-east-1",
						Priority: 1,
						Weight:   50,
						Capacity: RegionCapacity{MaxConcurrentUploads: 10},
						HealthCheck: HealthCheckConfig{
							Enabled:  true,
							Interval: -1 * time.Second, // Invalid
						},
					},
				},
				PrimaryRegion: "us-east-1",
			},
			expectError: true,
			errorMsg:    "health check interval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := coordinator.validateConfig(tt.config)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultCoordinator_SimulateNetworkDelay(t *testing.T) {
	coordinator := NewCoordinator()
	
	region := &Region{
		Name:     "us-east-1",
		Priority: 1,
	}
	
	delay := coordinator.simulateNetworkDelay(region)
	assert.Greater(t, delay, time.Duration(0))
	assert.Less(t, delay, 100*time.Millisecond) // Should be reasonable
}

func TestDefaultCoordinator_CalculateUploadDuration(t *testing.T) {
	coordinator := NewCoordinator()
	
	request := &UploadRequest{
		Size: 10 * 1024 * 1024, // 10MB
	}
	
	region := &Region{
		Name: "us-east-1",
	}
	
	duration := coordinator.calculateUploadDuration(request, region)
	assert.Greater(t, duration, time.Duration(0))
	assert.Less(t, duration, 10*time.Second) // Should be reasonable for 10MB
}

func TestDefaultCoordinator_ShouldSimulateFailure(t *testing.T) {
	coordinator := NewCoordinator()
	
	// Initialize regions map for the test
	coordinator.regions = make(map[string]*Region)
	
	region := &Region{
		Name: "us-east-1",
		Metrics: RegionMetrics{
			ErrorRate: 5.0, // Low error rate
		},
	}
	
	coordinator.regions[region.Name] = region
	
	// With low error rate, failure should be rare (but we can't guarantee it won't happen)
	failure := coordinator.shouldSimulateFailure(region)
	assert.IsType(t, bool(false), failure)
	
	// High error rate region
	highErrorRegion := &Region{
		Name: "us-west-2",
		Metrics: RegionMetrics{
			ErrorRate: 50.0, // High error rate
		},
	}
	
	coordinator.regions[highErrorRegion.Name] = highErrorRegion
	
	// Test multiple times to see if it can return true
	failureOccurred := false
	for i := 0; i < 100; i++ {
		if coordinator.shouldSimulateFailure(highErrorRegion) {
			failureOccurred = true
			break
		}
	}
	// With 50% error rate, we should see at least one failure in 100 attempts
	assert.True(t, failureOccurred)
}

func TestDefaultCoordinator_RecordRegionFailure(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.regions = make(map[string]*Region)
	
	region := &Region{
		Name: "us-east-1",
		Status: RegionStatusHealthy,
		Metrics: RegionMetrics{
			SuccessfulUploads: 10,
			FailedUploads:     0,
			ErrorRate:         0,
		},
	}
	
	coordinator.regions[region.Name] = region
	
	// Record a failure
	coordinator.recordRegionFailure(region.Name, assert.AnError)
	
	assert.Equal(t, int64(1), region.Metrics.FailedUploads)
	// Error rate calculation: failedUploads / (successfulUploads + failedUploads) * 100
	expectedErrorRate := float64(1) / float64(10+1) * 100 // 1/11 * 100 = 9.09...
	assert.InDelta(t, expectedErrorRate, region.Metrics.ErrorRate, 0.01)
	assert.Equal(t, RegionStatusHealthy, region.Status) // Should still be healthy
	
	// Record enough failures to mark as degraded
	for i := 0; i < 5; i++ {
		coordinator.recordRegionFailure(region.Name, assert.AnError)
	}
	
	assert.Equal(t, RegionStatusDegraded, region.Status)
	assert.Greater(t, region.Metrics.ErrorRate, 25.0)
}

func TestDefaultCoordinator_UpdateRegionMetrics(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.regions = make(map[string]*Region)
	
	region := &Region{
		Name: "us-east-1",
		Metrics: RegionMetrics{
			SuccessfulUploads: 0,
			FailedUploads:     0,
		},
	}
	
	coordinator.regions[region.Name] = region
	
	result := &UploadResult{
		Success:          true,
		Duration:         100 * time.Millisecond,
		BytesTransferred: 1024,
	}
	
	coordinator.updateRegionMetrics(region.Name, result)
	
	assert.Equal(t, int64(1), region.Metrics.SuccessfulUploads)
	assert.Equal(t, float64(100), region.Metrics.AverageLatencyMs)
	assert.Equal(t, float64(0), region.Metrics.ErrorRate)
	assert.True(t, region.Metrics.LastUpdated.After(time.Now().Add(-1*time.Second)))
}

func TestDefaultCoordinator_GetAlternativeRegions(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.regions = make(map[string]*Region)
	
	regions := []*Region{
		{Name: "us-east-1", Status: RegionStatusHealthy},
		{Name: "us-west-2", Status: RegionStatusHealthy},
		{Name: "eu-west-1", Status: RegionStatusUnhealthy},
		{Name: "ap-south-1", Status: RegionStatusHealthy},
	}
	
	for _, region := range regions {
		coordinator.regions[region.Name] = region
	}
	
	alternatives := coordinator.getAlternativeRegions("us-east-1")
	
	assert.Len(t, alternatives, 2) // us-west-2 and ap-south-1
	
	regionNames := make([]string, len(alternatives))
	for i, region := range alternatives {
		regionNames[i] = region.Name
	}
	
	assert.Contains(t, regionNames, "us-west-2")
	assert.Contains(t, regionNames, "ap-south-1")
	assert.NotContains(t, regionNames, "us-east-1") // Excluded
	assert.NotContains(t, regionNames, "eu-west-1") // Unhealthy
}

// Helper function to create a valid multi-region configuration for testing
func createValidMultiRegionConfig() *MultiRegionConfig {
	return &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: "us-east-1",
		Regions: []Region{
			{
				Name:     "us-east-1",
				Priority: 1,
				Weight:   50,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
			{
				Name:     "us-west-2",
				Priority: 2,
				Weight:   30,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 8,
					MaxBandwidthMbps:     800,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       LoadBalancingRoundRobin,
			StickySessions: false,
		},
		Failover: FailoverConfig{
			AutoFailover:      true,
			Strategy:          FailoverGraceful,
			DetectionInterval: 15 * time.Second,
			FailoverTimeout:   30 * time.Second,
			RetryAttempts:     2,
		},
		Monitoring: MonitoringConfig{
			Enabled:         true,
			MetricsInterval: 60 * time.Second,
		},
	}
}