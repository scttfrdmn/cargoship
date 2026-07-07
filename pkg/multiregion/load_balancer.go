// Package multiregion provides load balancing functionality for multi-region coordination
package multiregion

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- non-crypto: weighted load-balancer selection
	"strings"
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

	// latencyTracker tracks latency metrics for each region
	latencyTracker map[string]*LatencyTracker

	// latencyMutex protects latency tracker
	latencyMutex sync.RWMutex

	// geographicMap maps client locations to nearest regions
	geographicMap map[string]string

	// geoMutex protects geographic map
	geoMutex sync.RWMutex

	// performanceHistory tracks performance metrics for adaptive routing
	performanceHistory map[string]*PerformanceHistory

	// performanceMutex protects performance history
	performanceMutex sync.RWMutex
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

// LatencyTracker tracks latency metrics for a region
type LatencyTracker struct {
	// CurrentLatency current measured latency
	CurrentLatency time.Duration

	// AverageLatency rolling average latency
	AverageLatency time.Duration

	// MinLatency minimum observed latency
	MinLatency time.Duration

	// MaxLatency maximum observed latency
	MaxLatency time.Duration

	// SampleCount number of latency samples collected
	SampleCount int64

	// LastUpdated when latency was last updated
	LastUpdated time.Time
}

// PerformanceHistory tracks comprehensive performance metrics for adaptive routing
type PerformanceHistory struct {
	// SuccessRate recent success rate (0.0 - 1.0)
	SuccessRate float64

	// AverageResponseTime average response time for requests
	AverageResponseTime time.Duration

	// ThroughputMBps throughput in megabytes per second
	ThroughputMBps float64

	// ErrorRate recent error rate (0.0 - 1.0)
	ErrorRate float64

	// ResourceUtilization combined CPU/memory utilization (0.0 - 1.0)
	ResourceUtilization float64

	// Score calculated performance score (0.0 - 100.0)
	Score float64

	// LastUpdated when performance metrics were last updated
	LastUpdated time.Time

	// SampleWindow time window for performance samples
	SampleWindow time.Duration
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config *MultiRegionConfig, logger *log.Logger) LoadBalancer {
	lb := &DefaultLoadBalancer{
		config:             config,
		logger:             logger,
		sessionAffinityMap: make(map[string]SessionAffinity),
		random:             rand.New(rand.NewSource(time.Now().UnixNano())),
		latencyTracker:     make(map[string]*LatencyTracker),
		geographicMap:      make(map[string]string),
		performanceHistory: make(map[string]*PerformanceHistory),
	}

	// Initialize latency trackers and performance history for each region
	for i := range config.Regions {
		region := &config.Regions[i]
		lb.latencyTracker[region.Name] = &LatencyTracker{
			MinLatency:  time.Duration(999999999), // Large initial value
			SampleCount: 0,
		}
		lb.performanceHistory[region.Name] = &PerformanceHistory{
			SampleWindow: 5 * time.Minute,
			Score:        50.0, // Start with neutral score
		}
	}

	// Initialize geographic mapping with common locations
	lb.initializeGeographicMapping()

	return lb
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

	// Store session affinity under write lock with double-check to prevent
	// concurrent requests with the same session key from recording different regions.
	if lb.config.LoadBalancing.StickySessions {
		sessionKey := lb.generateSessionKey(request)
		if sessionKey != "" {
			lb.mu.Lock()
			if existing, exists := lb.sessionAffinityMap[sessionKey]; exists &&
				time.Since(existing.CreatedAt) <= lb.config.LoadBalancing.SessionTTL {
				// Another goroutine already stored a session; use their region
				storedName := existing.RegionName
				lb.mu.Unlock()
				for _, r := range availableRegions {
					if r.Name == storedName {
						return r, nil
					}
				}
				// Stored region gone; fall through and keep our chosen region
			} else {
				lb.sessionAffinityMap[sessionKey] = SessionAffinity{
					RegionName:   region.Name,
					CreatedAt:    time.Now(),
					LastUsed:     time.Now(),
					RequestCount: 1,
				}
				lb.mu.Unlock()
			}
		}
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
	case LoadBalancingAdaptive:
		return lb.routeAdaptive(ctx, request, regions), nil
	case LoadBalancingLeastConnections:
		return lb.routeLeastConnections(regions), nil
	case LoadBalancingResourceAware:
		return lb.routeResourceAware(regions), nil
	case LoadBalancingThroughputOptimized:
		return lb.routeThroughputOptimized(regions), nil
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

	lb.latencyMutex.RLock()
	defer lb.latencyMutex.RUnlock()

	// Find region with lowest average latency
	var bestRegion *Region
	bestLatency := time.Duration(999999999) // Large initial value

	for _, region := range regions {
		tracker, exists := lb.latencyTracker[region.Name]
		if !exists {
			// No latency data, use health check latency as fallback
			if region.Metrics.HealthCheckLatency > 0 {
				healthCheckLatency := time.Duration(region.Metrics.HealthCheckLatency) * time.Millisecond
				if healthCheckLatency < bestLatency {
					bestLatency = healthCheckLatency
					bestRegion = region
				}
			}
			continue
		}

		// Use average latency if available, otherwise use current latency
		latency := tracker.AverageLatency
		if latency == 0 {
			latency = tracker.CurrentLatency
		}

		// Skip regions with no latency data
		if latency == 0 {
			continue
		}

		if latency < bestLatency {
			bestLatency = latency
			bestRegion = region
		}
	}

	// If no region found with latency data, fall back to priority routing
	if bestRegion == nil {
		return lb.routeByPriority(regions)
	}

	return bestRegion
}

// routeByGeography implements geographic load balancing
func (lb *DefaultLoadBalancer) routeByGeography(request *UploadRequest, regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}

	// Extract client location hint from request metadata
	clientLocation := lb.extractClientLocation(request)
	if clientLocation == "" {
		// No location info available, fall back to latency-based routing
		return lb.routeByLatency(regions)
	}

	lb.geoMutex.RLock()
	preferredRegion, exists := lb.geographicMap[clientLocation]
	lb.geoMutex.RUnlock()

	if exists {
		// Check if preferred region is available
		for _, region := range regions {
			if region.Name == preferredRegion {
				lb.logger.Debug("Using geographic routing",
					"client_location", clientLocation,
					"preferred_region", preferredRegion,
					"request_id", request.ID)
				return region
			}
		}
	}

	// Preferred region not available or no mapping found
	// Use region-based proximity scoring
	bestRegion := lb.findClosestRegionByName(clientLocation, regions)
	if bestRegion != nil {
		return bestRegion
	}

	// Fall back to latency-based routing
	return lb.routeByLatency(regions)
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

// generateSessionKey generates a session key for sticky sessions (Issue #139)
// Session key generation priority:
// 1. Explicit session_id from request metadata (for multi-request workflows)
// 2. Request ID (backward compatibility, deterministic per-request affinity)
// 3. Generated secure UUID (for requests without identifiers)
func (lb *DefaultLoadBalancer) generateSessionKey(request *UploadRequest) string {
	// Priority 1: Check for explicit session ID in metadata
	// This allows clients to maintain session affinity across multiple requests
	if request.Metadata != nil {
		if sessionID, ok := request.Metadata["session_id"]; ok && sessionID != "" {
			return sessionID
		}

		// Check for user_id as alternative session identifier
		if userID, ok := request.Metadata["user_id"]; ok && userID != "" {
			return "user:" + userID
		}

		// Check for client_id as alternative session identifier
		if clientID, ok := request.Metadata["client_id"]; ok && clientID != "" {
			return "client:" + clientID
		}
	}

	// Priority 2: Use request ID (backward compatibility)
	// For most use cases, request ID provides sufficient session identification
	if request.ID != "" {
		return request.ID
	}

	// Priority 3: Generate secure random session key
	// This should rarely be used, as requests should have IDs
	// Uses cryptographically secure random UUID for unpredictable session keys
	return lb.generateSecureSessionID()
}

// generateSecureSessionID generates a cryptographically secure session ID (Issue #139)
// Uses crypto/rand for unpredictable, secure session identifiers
// Returns a UUID v4 format string (e.g., "550e8400-e29b-41d4-a716-446655440000")
func (lb *DefaultLoadBalancer) generateSecureSessionID() string {
	// Generate 16 random bytes for UUID v4
	uuid := make([]byte, 16)
	_, err := cryptorand.Read(uuid)
	if err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (should never happen)
		lb.logger.Warn("Failed to generate secure session ID, using timestamp fallback", "error", err)
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}

	// Set version (4) and variant bits according to RFC 4122
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	// Format as UUID string: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16])
}

// routeAdaptive implements adaptive load balancing based on real-time performance
func (lb *DefaultLoadBalancer) routeAdaptive(ctx context.Context, request *UploadRequest, regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}

	lb.performanceMutex.RLock()
	defer lb.performanceMutex.RUnlock()

	// Update performance scores before routing
	lb.updatePerformanceScores(regions)

	// Find region with highest performance score
	var bestRegion *Region
	bestScore := float64(-1)

	for _, region := range regions {
		history, exists := lb.performanceHistory[region.Name]
		if !exists {
			continue
		}

		// Apply request priority weighting
		score := history.Score
		if request.Priority > 5 {
			// Boost score for high priority requests
			score *= 1.2
		}

		if score > bestScore {
			bestScore = score
			bestRegion = region
		}
	}

	if bestRegion == nil {
		return lb.routeByPriority(regions)
	}

	lb.logger.Debug("Using adaptive routing",
		"request_id", request.ID,
		"selected_region", bestRegion.Name,
		"score", bestScore)

	return bestRegion
}

// routeLeastConnections routes to region with fewest active connections
func (lb *DefaultLoadBalancer) routeLeastConnections(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}

	// Find region with lowest active uploads count
	var bestRegion *Region
	lowestConnections := int64(999999)

	for _, region := range regions {
		activeConnections := region.Metrics.ActiveUploads
		if activeConnections < lowestConnections {
			lowestConnections = activeConnections
			bestRegion = region
		}
	}

	return bestRegion
}

// routeResourceAware routes based on comprehensive resource utilization
func (lb *DefaultLoadBalancer) routeResourceAware(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}

	// Calculate resource score for each region (lower is better)
	var bestRegion *Region
	bestScore := float64(999999)

	for _, region := range regions {
		// Combine CPU, memory, and capacity utilization
		cpuWeight := 0.4
		memoryWeight := 0.3
		capacityWeight := 0.3

		resourceScore := (region.Metrics.CPUUtilization * cpuWeight) +
			(region.Metrics.MemoryUtilization * memoryWeight) +
			(region.Capacity.CurrentUtilization * capacityWeight)

		// Boost score for regions with better health
		switch region.Status {
		case RegionStatusHealthy:
			resourceScore *= 0.8 // 20% boost for healthy regions
		case RegionStatusDegraded:
			resourceScore *= 1.2 // 20% penalty for degraded regions
		}

		if resourceScore < bestScore {
			bestScore = resourceScore
			bestRegion = region
		}
	}

	return bestRegion
}

// routeThroughputOptimized routes to maximize overall system throughput
func (lb *DefaultLoadBalancer) routeThroughputOptimized(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}

	// Calculate throughput potential for each region
	var bestRegion *Region
	bestThroughput := float64(0)

	for _, region := range regions {
		// Estimate potential throughput based on current metrics
		currentUtilization := region.Capacity.CurrentUtilization
		availableCapacity := 100.0 - currentUtilization

		// Factor in region health and error rate
		var healthMultiplier float64
		switch region.Status {
		case RegionStatusHealthy:
			healthMultiplier = 1.2
		case RegionStatusDegraded:
			healthMultiplier = 0.8
		default:
			healthMultiplier = 0.5
		}

		// Factor in error rate (lower error rate = higher throughput potential)
		errorMultiplier := 1.0 - (region.Metrics.ErrorRate / 100.0)

		throughputPotential := availableCapacity * healthMultiplier * errorMultiplier * float64(region.Weight)

		if throughputPotential > bestThroughput {
			bestThroughput = throughputPotential
			bestRegion = region
		}
	}

	return bestRegion
}

// Helper methods for advanced load balancing

// extractClientLocation extracts client location from request metadata
func (lb *DefaultLoadBalancer) extractClientLocation(request *UploadRequest) string {
	if request.Metadata == nil {
		return ""
	}

	// Check for various location metadata fields
	if location := request.Metadata["client_location"]; location != "" {
		return location
	}
	if country := request.Metadata["client_country"]; country != "" {
		return country
	}
	if region := request.Metadata["client_region"]; region != "" {
		return region
	}
	if ip := request.Metadata["client_ip"]; ip != "" {
		// Could implement IP geolocation here
		return lb.geolocateIP(ip)
	}

	return ""
}

// geolocateIP performs IP-based geolocation (simplified implementation)
func (lb *DefaultLoadBalancer) geolocateIP(ip string) string {
	// This is a simplified implementation
	// In production, you would use a proper IP geolocation service
	if strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "172.") {
		return "local"
	}
	// Default to unknown location
	return ""
}

// initializeGeographicMapping sets up the geographic to region mapping
func (lb *DefaultLoadBalancer) initializeGeographicMapping() {
	lb.geoMutex.Lock()
	defer lb.geoMutex.Unlock()

	// North America
	lb.geographicMap["US"] = "us-east-1"
	lb.geographicMap["USA"] = "us-east-1"
	lb.geographicMap["United States"] = "us-east-1"
	lb.geographicMap["Canada"] = "us-east-1"
	lb.geographicMap["CA"] = "us-east-1"
	lb.geographicMap["Mexico"] = "us-east-1"
	lb.geographicMap["MX"] = "us-east-1"

	// Europe
	lb.geographicMap["UK"] = "eu-west-1"
	lb.geographicMap["GB"] = "eu-west-1"
	lb.geographicMap["United Kingdom"] = "eu-west-1"
	lb.geographicMap["Germany"] = "eu-west-1"
	lb.geographicMap["DE"] = "eu-west-1"
	lb.geographicMap["France"] = "eu-west-1"
	lb.geographicMap["FR"] = "eu-west-1"
	lb.geographicMap["Italy"] = "eu-west-1"
	lb.geographicMap["IT"] = "eu-west-1"
	lb.geographicMap["Spain"] = "eu-west-1"
	lb.geographicMap["ES"] = "eu-west-1"

	// Asia Pacific
	lb.geographicMap["Japan"] = "ap-northeast-1"
	lb.geographicMap["JP"] = "ap-northeast-1"
	lb.geographicMap["Singapore"] = "ap-southeast-1"
	lb.geographicMap["SG"] = "ap-southeast-1"
	lb.geographicMap["Australia"] = "ap-southeast-2"
	lb.geographicMap["AU"] = "ap-southeast-2"
	lb.geographicMap["China"] = "ap-northeast-1"
	lb.geographicMap["CN"] = "ap-northeast-1"
	lb.geographicMap["South Korea"] = "ap-northeast-1"
	lb.geographicMap["KR"] = "ap-northeast-1"
}

// findClosestRegionByName finds closest region based on naming patterns
func (lb *DefaultLoadBalancer) findClosestRegionByName(location string, regions []*Region) *Region {
	// Simple heuristic based on region name patterns
	location = strings.ToLower(location)

	// Score regions based on geographic proximity hints in their names
	scores := make(map[*Region]int)

	for _, region := range regions {
		regionName := strings.ToLower(region.Name)
		score := 0

		// North America scoring
		if strings.Contains(location, "us") || strings.Contains(location, "america") || strings.Contains(location, "canada") {
			if strings.Contains(regionName, "us-") {
				score += 10
			}
		}

		// Europe scoring
		if strings.Contains(location, "eu") || strings.Contains(location, "europe") || strings.Contains(location, "uk") {
			if strings.Contains(regionName, "eu-") {
				score += 10
			}
		}

		// Asia Pacific scoring
		if strings.Contains(location, "ap") || strings.Contains(location, "asia") || strings.Contains(location, "pacific") {
			if strings.Contains(regionName, "ap-") {
				score += 10
			}
		}

		scores[region] = score
	}

	// Find region with highest score
	var bestRegion *Region
	bestScore := -1
	for region, score := range scores {
		if score > bestScore {
			bestScore = score
			bestRegion = region
		}
	}

	// Only return a region if we found a meaningful match (score > 0)
	if bestScore > 0 {
		return bestRegion
	}

	return nil
}

// updatePerformanceScores calculates performance scores for adaptive routing
func (lb *DefaultLoadBalancer) updatePerformanceScores(regions []*Region) {
	for _, region := range regions {
		history, exists := lb.performanceHistory[region.Name]
		if !exists {
			continue
		}

		// Calculate composite performance score (0-100)
		score := float64(0)

		// Factor 1: Health status (0-30 points)
		switch region.Status {
		case RegionStatusHealthy:
			score += 30
		case RegionStatusDegraded:
			score += 15
		case RegionStatusUnhealthy:
			score += 5
		default:
			score += 0
		}

		// Factor 2: Resource utilization (0-25 points, lower utilization = higher score)
		avgUtilization := (region.Metrics.CPUUtilization + region.Metrics.MemoryUtilization + region.Capacity.CurrentUtilization) / 3.0
		utilizationScore := 25.0 * (1.0 - (avgUtilization / 100.0))
		score += math.Max(0, utilizationScore)

		// Factor 3: Error rate (0-20 points, lower error rate = higher score)
		errorScore := 20.0 * (1.0 - (region.Metrics.ErrorRate / 100.0))
		score += math.Max(0, errorScore)

		// Factor 4: Latency (0-15 points)
		lb.latencyMutex.RLock()
		tracker, hasLatency := lb.latencyTracker[region.Name]
		lb.latencyMutex.RUnlock()

		if hasLatency && tracker.AverageLatency > 0 {
			// Lower latency = higher score (assume max acceptable latency of 1000ms)
			latencyMs := float64(tracker.AverageLatency.Milliseconds())
			latencyScore := 15.0 * math.Max(0, (1000.0-latencyMs)/1000.0)
			score += latencyScore
		} else if region.Metrics.HealthCheckLatency > 0 {
			// Use health check latency as fallback
			latencyScore := 15.0 * math.Max(0, (1000.0-float64(region.Metrics.HealthCheckLatency))/1000.0)
			score += latencyScore
		}

		// Factor 5: Connection load (0-10 points, fewer connections = higher score)
		if region.Capacity.MaxConcurrentUploads > 0 {
			loadRatio := float64(region.Metrics.ActiveUploads) / float64(region.Capacity.MaxConcurrentUploads)
			loadScore := 10.0 * (1.0 - loadRatio)
			score += math.Max(0, loadScore)
		}

		// Update performance history
		history.Score = math.Min(100.0, math.Max(0.0, score))
		history.LastUpdated = time.Now()
		lb.performanceHistory[region.Name] = history
	}
}

// UpdateLatencyMetrics updates latency tracking for a region
func (lb *DefaultLoadBalancer) UpdateLatencyMetrics(regionName string, latency time.Duration) {
	lb.latencyMutex.Lock()
	defer lb.latencyMutex.Unlock()

	tracker, exists := lb.latencyTracker[regionName]
	if !exists {
		tracker = &LatencyTracker{
			MinLatency: latency,
			MaxLatency: latency,
		}
		lb.latencyTracker[regionName] = tracker
	}

	// Update current latency
	tracker.CurrentLatency = latency
	tracker.SampleCount++
	tracker.LastUpdated = time.Now()

	// Update min/max
	if latency < tracker.MinLatency {
		tracker.MinLatency = latency
	}
	if latency > tracker.MaxLatency {
		tracker.MaxLatency = latency
	}

	// Update rolling average (simple exponential moving average)
	if tracker.AverageLatency == 0 {
		tracker.AverageLatency = latency
	} else {
		// Use alpha = 0.1 for exponential moving average
		alpha := 0.1
		tracker.AverageLatency = time.Duration(float64(tracker.AverageLatency)*(1-alpha) + float64(latency)*alpha)
	}
}

// GetLatencyStats returns latency statistics for all regions
func (lb *DefaultLoadBalancer) GetLatencyStats() map[string]LatencyTracker {
	lb.latencyMutex.RLock()
	defer lb.latencyMutex.RUnlock()

	stats := make(map[string]LatencyTracker)
	for region, tracker := range lb.latencyTracker {
		stats[region] = *tracker
	}
	return stats
}

// GetPerformanceStats returns performance statistics for all regions
func (lb *DefaultLoadBalancer) GetPerformanceStats() map[string]PerformanceHistory {
	lb.performanceMutex.RLock()
	defer lb.performanceMutex.RUnlock()

	stats := make(map[string]PerformanceHistory)
	for region, history := range lb.performanceHistory {
		stats[region] = *history
	}
	return stats
}

// AddGeographicMapping adds a new geographic location to region mapping
func (lb *DefaultLoadBalancer) AddGeographicMapping(location, region string) {
	lb.geoMutex.Lock()
	defer lb.geoMutex.Unlock()
	lb.geographicMap[location] = region
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
