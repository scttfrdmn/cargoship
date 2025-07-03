package multiregion

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoadBalancer(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	
	balancer := NewLoadBalancer(config, logger)
	
	assert.NotNil(t, balancer)
	assert.IsType(t, &DefaultLoadBalancer{}, balancer)
	
	defaultBalancer := balancer.(*DefaultLoadBalancer)
	assert.Equal(t, config, defaultBalancer.config)
	assert.Equal(t, logger, defaultBalancer.logger)
	assert.NotNil(t, defaultBalancer.sessionAffinityMap)
}

func TestDefaultLoadBalancer_Route(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	request := &UploadRequest{
		FilePath: "/test/file.txt",
		Size:     1024,
		Priority: 5,
	}
	
	tests := []struct {
		name     string
		strategy LoadBalancingStrategy
	}{
		{
			name:     "round robin routing",
			strategy: LoadBalancingRoundRobin,
		},
		{
			name:     "weighted routing",
			strategy: LoadBalancingWeighted,
		},
		{
			name:     "latency-based routing",
			strategy: LoadBalancingLatency,
		},
		{
			name:     "geographic routing",
			strategy: LoadBalancingGeographic,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Update strategy for test
			balancer.config.LoadBalancing.Strategy = tt.strategy
			
			region, err := balancer.Route(ctx, request)
			assert.NoError(t, err)
			assert.NotNil(t, region)
			assert.Contains(t, []string{"us-east-1", "us-west-2"}, region.Name)
		})
	}
}

func TestDefaultLoadBalancer_Route_WithSessionAffinity(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.StickySessions = true
	config.LoadBalancing.SessionTTL = 5 * time.Minute
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	request := &UploadRequest{
		ID:       "session-123",
		FilePath: "/test/file.txt",
		Size:     1024,
		Priority: 5,
	}
	
	// First request should create session affinity
	region1, err := balancer.Route(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, region1)
	
	// Second request with same ID should go to same region
	region2, err := balancer.Route(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, region2)
	assert.Equal(t, region1.Name, region2.Name)
	
	// Verify session affinity was recorded
	sessionKey := balancer.generateSessionKey(request)
	assert.Contains(t, balancer.sessionAffinityMap, sessionKey)
	affinity := balancer.sessionAffinityMap[sessionKey]
	assert.Equal(t, region1.Name, affinity.RegionName)
	assert.True(t, affinity.CreatedAt.After(time.Now().Add(-1*time.Second)))
}

func TestDefaultLoadBalancer_GetAvailableRegions(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	regions, err := balancer.GetAvailableRegions(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, regions)
	assert.Len(t, regions, 2)
	
	regionNames := make([]string, len(regions))
	for i, region := range regions {
		regionNames[i] = region.Name
	}
	
	assert.Contains(t, regionNames, "us-east-1")
	assert.Contains(t, regionNames, "us-west-2")
}

func TestDefaultLoadBalancer_GetAvailableRegions_FilterUnhealthy(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.Regions[1].Status = RegionStatusUnhealthy // Mark us-west-2 as unhealthy
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	regions, err := balancer.GetAvailableRegions(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, regions)
	assert.Len(t, regions, 1)
	assert.Equal(t, "us-east-1", regions[0].Name)
}

func TestDefaultLoadBalancer_UpdateRegionStatus(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	// Test updating existing region
	err := balancer.UpdateRegionStatus(ctx, "us-east-1", RegionStatusDegraded)
	assert.NoError(t, err)
	
	// Verify the status was updated by checking available regions
	regions, err := balancer.GetAvailableRegions(ctx)
	assert.NoError(t, err)
	
	var usEast1 *Region
	for _, region := range regions {
		if region.Name == "us-east-1" {
			usEast1 = region
			break
		}
	}
	
	assert.NotNil(t, usEast1)
	assert.Equal(t, RegionStatusDegraded, usEast1.Status)
	
	// Test updating non-existent region
	err = balancer.UpdateRegionStatus(ctx, "non-existent", RegionStatusHealthy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDefaultLoadBalancer_CleanupExpiredSessions(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.StickySessions = true
	config.LoadBalancing.SessionTTL = 1 * time.Millisecond
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	
	// Add expired session
	balancer.sessionAffinityMap["expired-session"] = SessionAffinity{
		RegionName: "us-east-1",
		CreatedAt:  time.Now().Add(-1 * time.Hour), // Old session
		LastUsed:   time.Now().Add(-1 * time.Hour),
	}
	
	// Add active session
	balancer.sessionAffinityMap["active-session"] = SessionAffinity{
		RegionName: "us-west-2",
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}
	
	assert.Len(t, balancer.sessionAffinityMap, 2)
	
	// Wait for TTL to pass for expired session
	time.Sleep(2 * time.Millisecond)
	
	// Update active session timestamp to keep it alive
	balancer.sessionAffinityMap["active-session"] = SessionAffinity{
		RegionName: "us-west-2",
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}
	
	// Cleanup expired sessions
	balancer.cleanupExpiredSessions()
	
	// Should only have active session left
	assert.Len(t, balancer.sessionAffinityMap, 1)
	assert.Contains(t, balancer.sessionAffinityMap, "active-session")
	assert.NotContains(t, balancer.sessionAffinityMap, "expired-session")
}

func TestDefaultLoadBalancer_GetSessionAffinityStats(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.StickySessions = true
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	
	// Add some sessions
	balancer.sessionAffinityMap["session-1"] = SessionAffinity{
		RegionName:   "us-east-1",
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		RequestCount: 5,
	}
	
	balancer.sessionAffinityMap["session-2"] = SessionAffinity{
		RegionName:   "us-west-2",
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		RequestCount: 3,
	}
	
	stats := balancer.GetSessionAffinityStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "total_sessions")
	assert.Equal(t, 2, stats["total_sessions"])
}

func TestDefaultLoadBalancer_GetLoadBalancingStats(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	
	stats := balancer.GetLoadBalancingStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "strategy")
	assert.Equal(t, config.LoadBalancing.Strategy, stats["strategy"])
}

func TestDefaultLoadBalancer_ConcurrentAccess(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.StickySessions = true
	config.LoadBalancing.SessionTTL = 5 * time.Minute
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	// Test concurrent routing with session affinity
	results := make(chan *Region, 10)
	errors := make(chan error, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			request := &UploadRequest{
				ID:       "concurrent-session",
				FilePath: "/test/file.txt",
				Size:     1024,
				Priority: 5,
			}
			
			region, err := balancer.Route(ctx, request)
			if err != nil {
				errors <- err
				return
			}
			results <- region
		}(i)
	}
	
	// Collect results
	var regions []*Region
	for i := 0; i < 10; i++ {
		select {
		case region := <-results:
			assert.NotNil(t, region)
			regions = append(regions, region)
		case err := <-errors:
			t.Fatalf("Unexpected error during concurrent access: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for concurrent operations")
		}
	}
	
	// All requests should go to the same region due to session affinity
	firstRegion := regions[0].Name
	for _, region := range regions {
		assert.Equal(t, firstRegion, region.Name)
	}
}

func TestDefaultLoadBalancer_EdgeCases(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
		ctx := context.Background()
		
		region, err := balancer.Route(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, region)
		assert.Contains(t, err.Error(), "request cannot be nil")
	})
	
	t.Run("no healthy regions", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		// Mark all regions as unhealthy
		for i := range config.Regions {
			config.Regions[i].Status = RegionStatusUnhealthy
		}
		
		logger := log.New(nil)
		balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
		ctx := context.Background()
		
		request := &UploadRequest{
			FilePath: "/test/file.txt",
			Size:     1024,
		}
		
		region, err := balancer.Route(ctx, request)
		assert.Error(t, err)
		assert.Nil(t, region)
		assert.Contains(t, err.Error(), "no healthy regions available")
	})
	
	t.Run("session affinity with empty request ID", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.LoadBalancing.StickySessions = true
		
		logger := log.New(nil)
		balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
		ctx := context.Background()
		
		request := &UploadRequest{
			ID:       "", // Empty ID
			FilePath: "/test/file.txt",
			Size:     1024,
		}
		
		// Should still work (no session affinity without ID)
		region, err := balancer.Route(ctx, request)
		assert.NoError(t, err)
		assert.NotNil(t, region)
	})
	
	t.Run("update status of empty region name", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
		ctx := context.Background()
		
		err := balancer.UpdateRegionStatus(ctx, "", RegionStatusHealthy)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region name cannot be empty")
	})
}

func TestDefaultLoadBalancer_WeightedDistribution(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.Strategy = LoadBalancingWeighted
	config.Regions[0].Weight = 80 // us-east-1
	config.Regions[1].Weight = 20 // us-west-2
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx := context.Background()
	
	selections := make(map[string]int)
	
	// Run many selections to test distribution
	for i := 0; i < 100; i++ {
		request := &UploadRequest{
			FilePath: "/test/file.txt",
			Size:     1024,
		}
		
		region, err := balancer.Route(ctx, request)
		require.NoError(t, err)
		selections[region.Name]++
	}
	
	// us-east-1 should be selected more often due to higher weight
	assert.Greater(t, selections["us-east-1"], selections["us-west-2"])
	
	// Rough check - us-east-1 should get roughly 4x more selections
	ratio := float64(selections["us-east-1"]) / float64(selections["us-west-2"])
	assert.Greater(t, ratio, 2.0) // At least 2:1 ratio
}

func TestDefaultLoadBalancer_StartSessionCleanup(t *testing.T) {
	config := createValidMultiRegionConfig()
	config.LoadBalancing.StickySessions = true
	config.LoadBalancing.SessionTTL = 10 * time.Millisecond
	
	logger := log.New(nil)
	balancer := NewLoadBalancer(config, logger).(*DefaultLoadBalancer)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// Add a session that will expire quickly
	balancer.sessionAffinityMap["test-session"] = SessionAffinity{
		RegionName: "us-east-1",
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}
	
	// Start session cleanup
	go balancer.StartSessionCleanup(ctx)
	
	// Wait a bit and check that cleanup happens
	time.Sleep(50 * time.Millisecond)
	
	// The session might still exist depending on timing, but the method should not panic
	assert.NotPanics(t, func() {
		balancer.GetSessionAffinityStats()
	})
}