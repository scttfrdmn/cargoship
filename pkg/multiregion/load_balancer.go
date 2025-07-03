// Package multiregion provides load balancing functionality for multi-region coordination
package multiregion

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// DefaultLoadBalancer implements the LoadBalancer interface
type DefaultLoadBalancer struct {
	// config holds the multi-region configuration
	config *MultiRegionConfig
	
	// logger for load balancer operations
	logger *log.Logger
	
	// regionSelector handles region selection logic (unused but kept for future use)
	_ RegionSelector
	
	// sessionAffinityMap tracks session affinity for sticky sessions
	sessionAffinityMap map[string]SessionAffinity
	
	// mu protects concurrent access to session affinity map
	mu sync.RWMutex
	
	// roundRobinCounter for round-robin load balancing
	roundRobinCounter uint64
	
	// roundRobinMutex protects round-robin counter
	roundRobinMutex sync.Mutex
	
	// random generator for weighted selection
	random *rand.Rand
	
	// randomMutex protects random generator
	randomMutex sync.Mutex
}

// SessionAffinity represents session affinity information
type SessionAffinity struct {
	// RegionName the region this session is bound to
	RegionName string
	
	// CreatedAt when the session affinity was created
	CreatedAt time.Time
	
	// LastUsed when the session affinity was last used
	LastUsed time.Time
	
	// RequestCount number of requests processed with this affinity
	RequestCount int64
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config *MultiRegionConfig, logger *log.Logger) LoadBalancer {
	return &DefaultLoadBalancer{
		config:             config,
		logger:             logger,
		sessionAffinityMap: make(map[string]SessionAffinity),
		random:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Route routes an upload request to the most appropriate region
func (lb *DefaultLoadBalancer) Route(ctx context.Context, request *UploadRequest) (*Region, error) {
	if request == nil {
		return nil, fmt.Errorf("upload request cannot be nil")
	}
	
	// Get available healthy regions
	availableRegions, err := lb.GetAvailableRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available regions: %w", err)
	}
	
	if len(availableRegions) == 0 {
		return nil, fmt.Errorf("no healthy regions available")
	}
	
	// Check for session affinity if sticky sessions are enabled
	if lb.config.LoadBalancing.StickySessions {
		if region := lb.getSessionAffinityRegion(request, availableRegions); region != nil {
			lb.logger.Debug("Using session affinity",
				"request_id", request.ID,
				"region", region.Name)
			return region, nil
		}
	}
	
	// Route based on load balancing strategy
	region, err := lb.routeByStrategy(ctx, request, availableRegions)
	if err != nil {
		return nil, fmt.Errorf("failed to route by strategy: %w", err)
	}
	
	// Create session affinity if sticky sessions are enabled
	if lb.config.LoadBalancing.StickySessions {
		lb.createSessionAffinity(request, region)
	}
	
	lb.logger.Debug("Routed request to region",
		"request_id", request.ID,
		"region", region.Name,
		"strategy", lb.config.LoadBalancing.Strategy)
	
	return region, nil
}

// GetAvailableRegions returns list of healthy regions
func (lb *DefaultLoadBalancer) GetAvailableRegions(ctx context.Context) ([]*Region, error) {
	var availableRegions []*Region
	
	for i := range lb.config.Regions {
		region := &lb.config.Regions[i]
		
		// Check if region is healthy or degraded (still usable)
		if region.Status == RegionStatusHealthy || region.Status == RegionStatusDegraded {
			// Additional capacity check
			if region.Capacity.CurrentUtilization < 95.0 {
				availableRegions = append(availableRegions, region)
			}
		}
	}
	
	return availableRegions, nil
}

// UpdateRegionStatus updates the status of a region
func (lb *DefaultLoadBalancer) UpdateRegionStatus(ctx context.Context, regionName string, status RegionStatus) error {
	if regionName == "" {
		return fmt.Errorf("region name cannot be empty")
	}
	
	// Find and update the region
	for i := range lb.config.Regions {
		if lb.config.Regions[i].Name == regionName {
			lb.config.Regions[i].Status = status
			lb.config.Regions[i].UpdatedAt = time.Now()
			
			lb.logger.Info("Updated region status",
				"region", regionName,
				"status", status)
			
			return nil
		}
	}
	
	return fmt.Errorf("region '%s' not found", regionName)
}

// routeByStrategy routes request based on configured load balancing strategy
func (lb *DefaultLoadBalancer) routeByStrategy(ctx context.Context, request *UploadRequest, regions []*Region) (*Region, error) {
	switch lb.config.LoadBalancing.Strategy {
	case LoadBalancingRoundRobin:
		return lb.routeRoundRobin(regions), nil
	case LoadBalancingWeighted:
		return lb.routeWeighted(regions), nil
	case LoadBalancingLatency:
		return lb.routeByLatency(regions), nil
	case LoadBalancingGeographic:
		return lb.routeByGeography(request, regions), nil
	default:
		return lb.routeByPriority(regions), nil
	}
}

// routeRoundRobin implements round-robin load balancing
func (lb *DefaultLoadBalancer) routeRoundRobin(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	lb.roundRobinMutex.Lock()
	defer lb.roundRobinMutex.Unlock()
	
	index := lb.roundRobinCounter % uint64(len(regions))
	lb.roundRobinCounter++
	
	return regions[index]
}

// routeWeighted implements weighted load balancing
func (lb *DefaultLoadBalancer) routeWeighted(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// Calculate total weight
	totalWeight := 0
	for _, region := range regions {
		totalWeight += region.Weight
	}
	
	if totalWeight == 0 {
		// If no weights are configured, fall back to round-robin
		return lb.routeRoundRobin(regions)
	}
	
	// Generate random number for weighted selection
	lb.randomMutex.Lock()
	target := lb.random.Intn(totalWeight)
	lb.randomMutex.Unlock()
	
	// Find region based on weighted selection
	currentWeight := 0
	for _, region := range regions {
		currentWeight += region.Weight
		if currentWeight > target {
			return region
		}
	}
	
	// Fallback to last region
	return regions[len(regions)-1]
}

// routeByLatency implements latency-based load balancing
func (lb *DefaultLoadBalancer) routeByLatency(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// TODO: Implement latency-based routing using actual latency metrics
	// For now, fall back to priority-based routing
	return lb.routeByPriority(regions)
}

// routeByGeography implements geographic load balancing
func (lb *DefaultLoadBalancer) routeByGeography(request *UploadRequest, regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// TODO: Implement geographic routing based on client location
	// This would involve:
	// 1. Determining client geographic location
	// 2. Calculating distance to each region
	// 3. Selecting closest region
	
	// For now, fall back to priority-based routing
	return lb.routeByPriority(regions)
}

// routeByPriority implements priority-based load balancing
func (lb *DefaultLoadBalancer) routeByPriority(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// Find region with highest priority (lowest priority number)
	bestRegion := regions[0]
	for _, region := range regions[1:] {
		if region.Priority < bestRegion.Priority {
			bestRegion = region
		}
	}
	
	return bestRegion
}

// getSessionAffinityRegion returns region based on session affinity
func (lb *DefaultLoadBalancer) getSessionAffinityRegion(request *UploadRequest, availableRegions []*Region) *Region {
	// Create session key based on request metadata
	sessionKey := lb.generateSessionKey(request)
	if sessionKey == "" {
		return nil
	}
	
	lb.mu.RLock()
	affinity, exists := lb.sessionAffinityMap[sessionKey]
	lb.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	// Check if session affinity has expired
	if time.Since(affinity.CreatedAt) > lb.config.LoadBalancing.SessionTTL {
		lb.mu.Lock()
		delete(lb.sessionAffinityMap, sessionKey)
		lb.mu.Unlock()
		return nil
	}
	
	// Check if the affinity region is still available
	for _, region := range availableRegions {
		if region.Name == affinity.RegionName {
			// Update last used time
			lb.mu.Lock()
			affinity.LastUsed = time.Now()
			affinity.RequestCount++
			lb.sessionAffinityMap[sessionKey] = affinity
			lb.mu.Unlock()
			
			return region
		}
	}
	
	// Affinity region is not available, remove affinity
	lb.mu.Lock()
	delete(lb.sessionAffinityMap, sessionKey)
	lb.mu.Unlock()
	
	return nil
}

// createSessionAffinity creates session affinity for a request
func (lb *DefaultLoadBalancer) createSessionAffinity(request *UploadRequest, region *Region) {
	sessionKey := lb.generateSessionKey(request)
	if sessionKey == "" {
		return
	}
	
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	lb.sessionAffinityMap[sessionKey] = SessionAffinity{
		RegionName:   region.Name,
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		RequestCount: 1,
	}
}

// generateSessionKey generates a session key for sticky sessions
func (lb *DefaultLoadBalancer) generateSessionKey(request *UploadRequest) string {
	// TODO: Implement proper session key generation
	// This could be based on:
	// 1. Client IP address
	// 2. User ID
	// 3. Session token
	// 4. Request metadata
	
	// For now, use request ID as session key
	return request.ID
}

// cleanupExpiredSessions removes expired session affinity entries
func (lb *DefaultLoadBalancer) cleanupExpiredSessions() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	now := time.Now()
	for sessionKey, affinity := range lb.sessionAffinityMap {
		if now.Sub(affinity.CreatedAt) > lb.config.LoadBalancing.SessionTTL {
			delete(lb.sessionAffinityMap, sessionKey)
		}
	}
}

// GetSessionAffinityStats returns statistics about session affinity
func (lb *DefaultLoadBalancer) GetSessionAffinityStats() map[string]interface{} {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	stats := make(map[string]interface{})
	stats["total_sessions"] = len(lb.sessionAffinityMap)
	
	// Count sessions per region
	regionCounts := make(map[string]int)
	totalRequests := int64(0)
	
	for _, affinity := range lb.sessionAffinityMap {
		regionCounts[affinity.RegionName]++
		totalRequests += affinity.RequestCount
	}
	
	stats["sessions_per_region"] = regionCounts
	stats["total_requests"] = totalRequests
	
	return stats
}

// GetLoadBalancingStats returns load balancing statistics
func (lb *DefaultLoadBalancer) GetLoadBalancingStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["strategy"] = lb.config.LoadBalancing.Strategy
	stats["sticky_sessions"] = lb.config.LoadBalancing.StickySessions
	
	if lb.config.LoadBalancing.StickySessions {
		stats["session_ttl"] = lb.config.LoadBalancing.SessionTTL
		stats["session_affinity"] = lb.GetSessionAffinityStats()
	}
	
	lb.roundRobinMutex.Lock()
	stats["round_robin_counter"] = lb.roundRobinCounter
	lb.roundRobinMutex.Unlock()
	
	return stats
}

// StartSessionCleanup starts a background goroutine to clean up expired sessions
func (lb *DefaultLoadBalancer) StartSessionCleanup(ctx context.Context) {
	if !lb.config.LoadBalancing.StickySessions {
		return
	}
	
	cleanupInterval := lb.config.LoadBalancing.SessionTTL / 4
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}
	
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lb.cleanupExpiredSessions()
		}
	}
}