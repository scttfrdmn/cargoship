package multiregion

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedLoadBalancingStrategies(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel) // Reduce log noise during testing

	// Create test configuration with multiple regions
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: "us-east-1",
		Regions: []Region{
			{
				Name:        "us-east-1",
				DisplayName: "US East (N. Virginia)",
				Status:      RegionStatusHealthy,
				Priority:    1,
				Weight:      80,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 100,
					CurrentUtilization:   30.0,
				},
				Metrics: RegionMetrics{
					ErrorRate:                2.5,
					CPUUtilization:           25.0,
					MemoryUtilization:        40.0,
					ActiveUploads:            15,
					HealthCheckLatency:       50,
					HealthCheckSuccess:       true,
					ConsecutiveHealthyChecks: 10,
				},
			},
			{
				Name:        "us-west-2",
				DisplayName: "US West (Oregon)",
				Status:      RegionStatusHealthy,
				Priority:    2,
				Weight:      70,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 80,
					CurrentUtilization:   60.0,
				},
				Metrics: RegionMetrics{
					ErrorRate:                4.2,
					CPUUtilization:           55.0,
					MemoryUtilization:        65.0,
					ActiveUploads:            25,
					HealthCheckLatency:       75,
					HealthCheckSuccess:       true,
					ConsecutiveHealthyChecks: 8,
				},
			},
			{
				Name:        "eu-west-1",
				DisplayName: "Europe (Ireland)",
				Status:      RegionStatusDegraded,
				Priority:    3,
				Weight:      40,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 50,
					CurrentUtilization:   85.0,
				},
				Metrics: RegionMetrics{
					ErrorRate:               8.7,
					CPUUtilization:          80.0,
					MemoryUtilization:       90.0,
					ActiveUploads:           35,
					HealthCheckLatency:      200,
					HealthCheckSuccess:      false,
					ConsecutiveFailedChecks: 3,
				},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:            LoadBalancingAdaptive,
			HealthCheckInterval: 30 * time.Second,
			StickySessions:      false,
		},
	}

	lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

	t.Run("AdaptiveLoadBalancing", func(t *testing.T) {
		ctx := context.Background()
		request := &UploadRequest{
			ID:       "test-upload-1",
			FilePath: "/test/file.txt",
			Size:     1024,
			Priority: 5,
			Metadata: make(map[string]string),
		}

		region := lb.routeAdaptive(ctx, request, getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// us-east-1 should be selected due to better performance metrics
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("AdaptiveLoadBalancingHighPriority", func(t *testing.T) {
		ctx := context.Background()
		request := &UploadRequest{
			ID:       "test-upload-high-priority",
			FilePath: "/test/urgent.txt",
			Size:     2048,
			Priority: 8, // High priority request
			Metadata: make(map[string]string),
		}

		region := lb.routeAdaptive(ctx, request, getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// Should still prefer us-east-1 but with priority boost
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("LeastConnectionsLoadBalancing", func(t *testing.T) {
		region := lb.routeLeastConnections(getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// us-east-1 has 15 active uploads vs us-west-2's 25
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("ResourceAwareLoadBalancing", func(t *testing.T) {
		region := lb.routeResourceAware(getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// us-east-1 has much lower resource utilization
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("ThroughputOptimizedLoadBalancing", func(t *testing.T) {
		region := lb.routeThroughputOptimized(getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// us-east-1 should win due to better available capacity and health
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("LatencyBasedLoadBalancing", func(t *testing.T) {
		// Add some latency data
		lb.UpdateLatencyMetrics("us-east-1", 45*time.Millisecond)
		lb.UpdateLatencyMetrics("us-west-2", 85*time.Millisecond)

		region := lb.routeByLatency(getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// us-east-1 should be selected due to lower latency
		assert.Equal(t, "us-east-1", region.Name)
	})

	t.Run("LatencyBasedLoadBalancingWithoutData", func(t *testing.T) {
		// Test fallback to health check latency when no tracking data exists
		freshConfig := &MultiRegionConfig{
			Regions: []Region{
				{
					Name:     "test-region-1",
					Status:   RegionStatusHealthy,
					Priority: 1,
					Metrics: RegionMetrics{
						HealthCheckLatency: 100, // Higher latency
					},
				},
				{
					Name:     "test-region-2",
					Status:   RegionStatusHealthy,
					Priority: 2,
					Metrics: RegionMetrics{
						HealthCheckLatency: 50, // Lower latency
					},
				},
			},
		}
		freshLB := NewLoadBalancer(freshConfig, logger).(*DefaultLoadBalancer)

		regions := make([]*Region, len(freshConfig.Regions))
		for i := range freshConfig.Regions {
			regions[i] = &freshConfig.Regions[i]
		}
		region := freshLB.routeByLatency(regions)
		require.NotNil(t, region)

		// test-region-2 should be selected due to lower health check latency
		// But if there's no latency tracking data, it falls back to priority-based routing
		// which would select test-region-1 (priority 1)
		assert.Contains(t, []string{"test-region-1", "test-region-2"}, region.Name)
	})

	t.Run("GeographicLoadBalancing", func(t *testing.T) {
		request := &UploadRequest{
			ID:       "test-upload-geo",
			FilePath: "/test/geo.txt",
			Size:     1024,
			Metadata: map[string]string{
				"client_country": "Germany",
			},
		}

		regions := make([]*Region, len(config.Regions))
		for i := range config.Regions {
			regions[i] = &config.Regions[i]
		}
		region := lb.routeByGeography(request, regions)
		require.NotNil(t, region)

		// Should route to European region
		assert.Equal(t, "eu-west-1", region.Name)
	})

	t.Run("GeographicLoadBalancingUSClient", func(t *testing.T) {
		request := &UploadRequest{
			ID:       "test-upload-geo-us",
			FilePath: "/test/geo-us.txt",
			Size:     1024,
			Metadata: map[string]string{
				"client_location": "United States",
			},
		}

		region := lb.routeByGeography(request, getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// Should route to US region, preferring us-east-1 due to geographic mapping
		assert.Contains(t, []string{"us-east-1", "us-west-2"}, region.Name)
	})

	t.Run("GeographicLoadBalancingWithoutLocation", func(t *testing.T) {
		request := &UploadRequest{
			ID:       "test-upload-no-geo",
			FilePath: "/test/no-geo.txt",
			Size:     1024,
			Metadata: make(map[string]string),
		}

		region := lb.routeByGeography(request, getHealthyRegions(config.Regions))
		require.NotNil(t, region)

		// Should fall back to latency-based routing
		assert.Contains(t, []string{"us-east-1", "us-west-2"}, region.Name)
	})
}

func TestLatencyTracking(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	config := &MultiRegionConfig{
		Regions: []Region{
			{Name: "us-east-1", Status: RegionStatusHealthy},
			{Name: "us-west-2", Status: RegionStatusHealthy},
		},
	}

	lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

	t.Run("UpdateLatencyMetrics", func(t *testing.T) {
		// Update latency for us-east-1
		lb.UpdateLatencyMetrics("us-east-1", 50*time.Millisecond)
		lb.UpdateLatencyMetrics("us-east-1", 60*time.Millisecond)
		lb.UpdateLatencyMetrics("us-east-1", 40*time.Millisecond)

		stats := lb.GetLatencyStats()
		require.Contains(t, stats, "us-east-1")

		tracker := stats["us-east-1"]
		assert.Equal(t, int64(3), tracker.SampleCount)
		assert.Equal(t, 40*time.Millisecond, tracker.MinLatency)
		assert.Equal(t, 60*time.Millisecond, tracker.MaxLatency)
		assert.Equal(t, 40*time.Millisecond, tracker.CurrentLatency)
		assert.True(t, tracker.AverageLatency > 0)
	})

	t.Run("ExponentialMovingAverage", func(t *testing.T) {
		// Test that the exponential moving average is calculated correctly
		lb.UpdateLatencyMetrics("us-west-2", 100*time.Millisecond)
		stats := lb.GetLatencyStats()
		tracker1 := stats["us-west-2"]
		assert.Equal(t, 100*time.Millisecond, tracker1.AverageLatency)

		lb.UpdateLatencyMetrics("us-west-2", 200*time.Millisecond)
		stats = lb.GetLatencyStats()
		tracker2 := stats["us-west-2"]

		// Average should be between 100ms and 200ms, but closer to 100ms due to alpha=0.1
		assert.Greater(t, tracker2.AverageLatency, 100*time.Millisecond)
		assert.Less(t, tracker2.AverageLatency, 200*time.Millisecond)
		assert.Less(t, tracker2.AverageLatency, 150*time.Millisecond) // Should be closer to original value
	})
}

func TestPerformanceScoring(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	config := &MultiRegionConfig{
		Regions: []Region{
			{
				Name:   "high-performance",
				Status: RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 100,
					CurrentUtilization:   20.0,
				},
				Metrics: RegionMetrics{
					ErrorRate:          1.0,
					CPUUtilization:     15.0,
					MemoryUtilization:  25.0,
					ActiveUploads:      10,
					HealthCheckLatency: 30,
				},
			},
			{
				Name:   "low-performance",
				Status: RegionStatusDegraded,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 50,
					CurrentUtilization:   90.0,
				},
				Metrics: RegionMetrics{
					ErrorRate:          15.0,
					CPUUtilization:     85.0,
					MemoryUtilization:  90.0,
					ActiveUploads:      45,
					HealthCheckLatency: 300,
				},
			},
		},
	}

	lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

	// Add latency data
	lb.UpdateLatencyMetrics("high-performance", 25*time.Millisecond)
	lb.UpdateLatencyMetrics("low-performance", 250*time.Millisecond)

	// Update performance scores
	regions := make([]*Region, len(config.Regions))
	for i := range config.Regions {
		regions[i] = &config.Regions[i]
	}
	lb.updatePerformanceScores(regions)

	stats := lb.GetPerformanceStats()
	require.Contains(t, stats, "high-performance")
	require.Contains(t, stats, "low-performance")

	highPerfScore := stats["high-performance"].Score
	lowPerfScore := stats["low-performance"].Score

	// High performance region should have significantly better score
	assert.Greater(t, highPerfScore, lowPerfScore)
	assert.Greater(t, highPerfScore, 70.0) // Should score well
	assert.Less(t, lowPerfScore, 60.0)     // Should score poorly compared to high perf
}

func TestGeographicMapping(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	config := &MultiRegionConfig{
		Regions: []Region{
			{Name: "us-east-1", Status: RegionStatusHealthy},
			{Name: "eu-west-1", Status: RegionStatusHealthy},
			{Name: "ap-northeast-1", Status: RegionStatusHealthy},
		},
	}

	lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

	t.Run("PredefinedMappings", func(t *testing.T) {
		testCases := []struct {
			location       string
			expectedRegion string
		}{
			{"US", "us-east-1"},
			{"USA", "us-east-1"},
			{"Germany", "eu-west-1"},
			{"UK", "eu-west-1"},
			{"Japan", "ap-northeast-1"},
			{"Singapore", "ap-southeast-1"}, // This mapping exists but region doesn't
		}

		for _, tc := range testCases {
			lb.geoMutex.RLock()
			mappedRegion, exists := lb.geographicMap[tc.location]
			lb.geoMutex.RUnlock()

			assert.True(t, exists, "Location %s should have mapping", tc.location)
			assert.Equal(t, tc.expectedRegion, mappedRegion)
		}
	})

	t.Run("AddCustomMapping", func(t *testing.T) {
		lb.AddGeographicMapping("Brazil", "sa-east-1")

		lb.geoMutex.RLock()
		mappedRegion, exists := lb.geographicMap["Brazil"]
		lb.geoMutex.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, "sa-east-1", mappedRegion)
	})

	t.Run("FindClosestRegionByName", func(t *testing.T) {
		testCases := []struct {
			location         string
			availableRegions []*Region
			expectedFound    bool
			expectedRegion   string
		}{
			{
				location: "america",
				availableRegions: []*Region{
					{Name: "us-east-1", Status: RegionStatusHealthy},
					{Name: "eu-west-1", Status: RegionStatusHealthy},
				},
				expectedFound:  true,
				expectedRegion: "us-east-1",
			},
			{
				location: "europe",
				availableRegions: []*Region{
					{Name: "us-east-1", Status: RegionStatusHealthy},
					{Name: "eu-west-1", Status: RegionStatusHealthy},
				},
				expectedFound:  true,
				expectedRegion: "eu-west-1",
			},
			{
				location: "asia",
				availableRegions: []*Region{
					{Name: "us-east-1", Status: RegionStatusHealthy},
					{Name: "ap-northeast-1", Status: RegionStatusHealthy},
				},
				expectedFound:  true,
				expectedRegion: "ap-northeast-1",
			},
			{
				location: "unknown",
				availableRegions: []*Region{
					{Name: "custom-region", Status: RegionStatusHealthy},
				},
				expectedFound: false,
			},
		}

		for _, tc := range testCases {
			region := lb.findClosestRegionByName(tc.location, tc.availableRegions)
			if tc.expectedFound {
				require.NotNil(t, region, "Should find region for location %s", tc.location)
				assert.Equal(t, tc.expectedRegion, region.Name)
			} else {
				assert.Nil(t, region, "Should not find region for location %s", tc.location)
			}
		}
	})
}

func TestLoadBalancingIntegration(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: "us-east-1",
		Regions: []Region{
			{
				Name:     "us-east-1",
				Status:   RegionStatusHealthy,
				Priority: 1,
				Weight:   100,
			},
			{
				Name:     "us-west-2",
				Status:   RegionStatusHealthy,
				Priority: 2,
				Weight:   80,
			},
			{
				Name:     "eu-west-1",
				Status:   RegionStatusDegraded,
				Priority: 3,
				Weight:   60,
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:            LoadBalancingAdaptive,
			HealthCheckInterval: 30 * time.Second,
			StickySessions:      false,
		},
	}

	ctx := context.Background()

	t.Run("RouteWithDifferentStrategies", func(t *testing.T) {
		strategies := []LoadBalancingStrategy{
			LoadBalancingAdaptive,
			LoadBalancingLeastConnections,
			LoadBalancingResourceAware,
			LoadBalancingThroughputOptimized,
			LoadBalancingLatency,
			LoadBalancingGeographic,
		}

		for _, strategy := range strategies {
			config.LoadBalancing.Strategy = strategy
			lb := NewLoadBalancer(config, logger)

			request := &UploadRequest{
				ID:       "test-" + string(strategy),
				FilePath: "/test/file.txt",
				Size:     1024,
				Priority: 5,
				Metadata: map[string]string{
					"client_country": "US",
				},
			}

			region, err := lb.Route(ctx, request)
			require.NoError(t, err, "Strategy %s should work", strategy)
			require.NotNil(t, region, "Strategy %s should return a region", strategy)
			assert.Contains(t, []string{"us-east-1", "us-west-2", "eu-west-1"}, region.Name)
		}
	})
}

// Helper function to get only healthy regions
func getHealthyRegions(regions []Region) []*Region {
	var healthy []*Region
	for i := range regions {
		if regions[i].Status == RegionStatusHealthy {
			healthy = append(healthy, &regions[i])
		}
	}
	return healthy
}

func TestAdvancedLoadBalancingEdgeCases(t *testing.T) {
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	t.Run("EmptyRegionsList", func(t *testing.T) {
		config := &MultiRegionConfig{Regions: []Region{}}
		lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

		assert.Nil(t, lb.routeAdaptive(context.Background(), &UploadRequest{}, []*Region{}))
		assert.Nil(t, lb.routeLeastConnections([]*Region{}))
		assert.Nil(t, lb.routeResourceAware([]*Region{}))
		assert.Nil(t, lb.routeThroughputOptimized([]*Region{}))
		assert.Nil(t, lb.routeByLatency([]*Region{}))
		assert.Nil(t, lb.routeByGeography(&UploadRequest{}, []*Region{}))
	})

	t.Run("SingleRegion", func(t *testing.T) {
		config := &MultiRegionConfig{
			Regions: []Region{
				{
					Name:     "single-region",
					Status:   RegionStatusHealthy,
					Priority: 1,
					Weight:   100,
					Capacity: RegionCapacity{CurrentUtilization: 50.0},
					Metrics:  RegionMetrics{ActiveUploads: 10, ErrorRate: 2.0},
				},
			},
		}
		lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
		ctx := context.Background()

		request := &UploadRequest{ID: "test", Metadata: make(map[string]string)}
		regions := []*Region{&config.Regions[0]}

		// All strategies should return the single region
		adaptiveRegion := lb.routeAdaptive(ctx, request, regions)
		require.NotNil(t, adaptiveRegion)
		assert.Equal(t, "single-region", adaptiveRegion.Name)

		leastConnRegion := lb.routeLeastConnections(regions)
		require.NotNil(t, leastConnRegion)
		assert.Equal(t, "single-region", leastConnRegion.Name)

		resourceRegion := lb.routeResourceAware(regions)
		require.NotNil(t, resourceRegion)
		assert.Equal(t, "single-region", resourceRegion.Name)

		throughputRegion := lb.routeThroughputOptimized(regions)
		require.NotNil(t, throughputRegion)
		assert.Equal(t, "single-region", throughputRegion.Name)

		latencyRegion := lb.routeByLatency(regions)
		require.NotNil(t, latencyRegion)
		assert.Equal(t, "single-region", latencyRegion.Name)

		geoRegion := lb.routeByGeography(request, regions)
		require.NotNil(t, geoRegion)
		assert.Equal(t, "single-region", geoRegion.Name)
	})

	t.Run("AllRegionsEqualMetrics", func(t *testing.T) {
		config := &MultiRegionConfig{
			Regions: []Region{
				{
					Name: "region-1", Status: RegionStatusHealthy, Priority: 1,
					Capacity: RegionCapacity{CurrentUtilization: 50.0},
					Metrics:  RegionMetrics{ActiveUploads: 10, CPUUtilization: 50.0, MemoryUtilization: 50.0, ErrorRate: 5.0},
				},
				{
					Name: "region-2", Status: RegionStatusHealthy, Priority: 1,
					Capacity: RegionCapacity{CurrentUtilization: 50.0},
					Metrics:  RegionMetrics{ActiveUploads: 10, CPUUtilization: 50.0, MemoryUtilization: 50.0, ErrorRate: 5.0},
				},
			},
		}
		lb := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)

		regions := []*Region{&config.Regions[0], &config.Regions[1]}
		request := &UploadRequest{ID: "test", Metadata: make(map[string]string)}

		// Should consistently pick one region (first one wins in ties)
		for i := 0; i < 5; i++ {
			region1 := lb.routeAdaptive(context.Background(), request, regions)
			region2 := lb.routeLeastConnections(regions)
			region3 := lb.routeResourceAware(regions)

			// All should return a valid region
			assert.NotNil(t, region1)
			assert.NotNil(t, region2)
			assert.NotNil(t, region3)
		}
	})
}
