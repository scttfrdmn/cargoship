// Package multiregion provides region selection logic for multi-region coordination
package multiregion

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// DefaultRegionSelector implements the RegionSelector interface
type DefaultRegionSelector struct {
	// config holds the multi-region configuration
	config *MultiRegionConfig
	
	// logger for selector operations
	logger *log.Logger
	
	// regionMetrics stores cached region metrics
	regionMetrics map[string]RegionMetrics
	
	// mu protects concurrent access to region metrics
	mu sync.RWMutex
}

// NewRegionSelector creates a new region selector
func NewRegionSelector(config *MultiRegionConfig, logger *log.Logger) RegionSelector {
	return &DefaultRegionSelector{
		config:        config,
		logger:        logger,
		regionMetrics: make(map[string]RegionMetrics),
	}
}

// SelectRegion selects the best region for an upload request
func (s *DefaultRegionSelector) SelectRegion(ctx context.Context, request *UploadRequest) (*Region, error) {
	if request == nil {
		return nil, fmt.Errorf("upload request cannot be nil")
	}
	
	// Get available healthy regions
	availableRegions := s.getAvailableRegions()
	if len(availableRegions) == 0 {
		return nil, fmt.Errorf("no healthy regions available")
	}
	
	// Check if preferred region is specified and available
	if request.PreferredRegion != "" {
		for _, region := range availableRegions {
			if region.Name == request.PreferredRegion && region.Status == RegionStatusHealthy {
				s.logger.Debug("Using preferred region",
					"region", region.Name,
					"request_id", request.ID)
				return region, nil
			}
		}
		
		s.logger.Warn("Preferred region not available, selecting alternative",
			"preferred_region", request.PreferredRegion,
			"request_id", request.ID)
	}
	
	// Select region based on load balancing strategy
	return s.selectRegionByStrategy(ctx, request, availableRegions)
}

// SelectRegions selects multiple regions for redundant uploads
func (s *DefaultRegionSelector) SelectRegions(ctx context.Context, request *UploadRequest, count int) ([]*Region, error) {
	if request == nil {
		return nil, fmt.Errorf("upload request cannot be nil")
	}
	
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive")
	}
	
	// Get available healthy regions
	availableRegions := s.getAvailableRegions()
	if len(availableRegions) == 0 {
		return nil, fmt.Errorf("no healthy regions available")
	}
	
	// Limit count to available regions
	if count > len(availableRegions) {
		count = len(availableRegions)
	}
	
	// Select regions based on strategy
	selectedRegions := make([]*Region, 0, count)
	
	// Always include preferred region if available
	if request.PreferredRegion != "" {
		for _, region := range availableRegions {
			if region.Name == request.PreferredRegion && region.Status == RegionStatusHealthy {
				selectedRegions = append(selectedRegions, region)
				count--
				break
			}
		}
	}
	
	// Select remaining regions
	remainingRegions := s.selectBestRegions(ctx, request, availableRegions, count)
	
	// Avoid duplicates
	regionSet := make(map[string]bool)
	for _, region := range selectedRegions {
		regionSet[region.Name] = true
	}
	
	for _, region := range remainingRegions {
		if !regionSet[region.Name] {
			selectedRegions = append(selectedRegions, region)
			regionSet[region.Name] = true
		}
	}
	
	return selectedRegions, nil
}

// UpdateRegionMetrics updates metrics for a region
func (s *DefaultRegionSelector) UpdateRegionMetrics(ctx context.Context, regionName string, metrics RegionMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.regionMetrics[regionName] = metrics
	
	s.logger.Debug("Updated region metrics",
		"region", regionName,
		"latency_ms", metrics.AverageLatencyMs,
		"throughput_mbps", metrics.ThroughputMbps,
		"error_rate", metrics.ErrorRate)
	
	return nil
}

// getAvailableRegions returns list of healthy regions
func (s *DefaultRegionSelector) getAvailableRegions() []*Region {
	var availableRegions []*Region
	
	for i := range s.config.Regions {
		region := &s.config.Regions[i]
		if region.Status == RegionStatusHealthy || region.Status == RegionStatusDegraded {
			availableRegions = append(availableRegions, region)
		}
	}
	
	return availableRegions
}

// selectRegionByStrategy selects region based on load balancing strategy
func (s *DefaultRegionSelector) selectRegionByStrategy(ctx context.Context, request *UploadRequest, regions []*Region) (*Region, error) {
	switch s.config.LoadBalancing.Strategy {
	case LoadBalancingRoundRobin:
		return s.selectRoundRobin(regions), nil
	case LoadBalancingWeighted:
		return s.selectWeighted(regions), nil
	case LoadBalancingLatency:
		return s.selectByLatency(regions), nil
	case LoadBalancingGeographic:
		return s.selectByGeography(request, regions), nil
	default:
		return s.selectByPriority(regions), nil
	}
}

// selectRoundRobin selects region using round-robin algorithm
func (s *DefaultRegionSelector) selectRoundRobin(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// Simple round-robin based on current time
	// In a real implementation, this would maintain state
	index := int(time.Now().Unix()) % len(regions)
	return regions[index]
}

// selectWeighted selects region based on weights
func (s *DefaultRegionSelector) selectWeighted(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// Calculate total weight
	totalWeight := 0
	for _, region := range regions {
		totalWeight += region.Weight
	}
	
	if totalWeight == 0 {
		// If no weights are set, use priority-based selection
		return s.selectByPriority(regions)
	}
	
	// Simple weighted selection based on current time
	// In a real implementation, this would use proper weighted random selection
	target := int(time.Now().Unix()) % totalWeight
	currentWeight := 0
	
	for _, region := range regions {
		currentWeight += region.Weight
		if currentWeight > target {
			return region
		}
	}
	
	return regions[0]
}

// selectByLatency selects region with lowest latency
func (s *DefaultRegionSelector) selectByLatency(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	bestRegion := regions[0]
	bestLatency := float64(^uint(0) >> 1) // Max float64
	
	for _, region := range regions {
		if metrics, exists := s.regionMetrics[region.Name]; exists {
			if metrics.AverageLatencyMs < bestLatency {
				bestLatency = metrics.AverageLatencyMs
				bestRegion = region
			}
		}
	}
	
	return bestRegion
}

// selectByGeography selects region based on geographic proximity
func (s *DefaultRegionSelector) selectByGeography(request *UploadRequest, regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// TODO: Implement geographic selection based on client location
	// For now, fall back to priority-based selection
	return s.selectByPriority(regions)
}

// selectByPriority selects region with highest priority
func (s *DefaultRegionSelector) selectByPriority(regions []*Region) *Region {
	if len(regions) == 0 {
		return nil
	}
	
	// Sort regions by priority (lower number = higher priority)
	sortedRegions := make([]*Region, len(regions))
	copy(sortedRegions, regions)
	
	sort.Slice(sortedRegions, func(i, j int) bool {
		return sortedRegions[i].Priority < sortedRegions[j].Priority
	})
	
	return sortedRegions[0]
}

// selectBestRegions selects the best regions for redundant uploads
func (s *DefaultRegionSelector) selectBestRegions(ctx context.Context, request *UploadRequest, regions []*Region, count int) []*Region {
	if len(regions) == 0 || count <= 0 {
		return nil
	}
	
	// Create a copy of regions to avoid modifying original
	candidateRegions := make([]*Region, len(regions))
	copy(candidateRegions, regions)
	
	// Sort by preference based on strategy
	switch s.config.LoadBalancing.Strategy {
	case LoadBalancingLatency:
		s.sortRegionsByLatency(candidateRegions)
	case LoadBalancingWeighted:
		s.sortRegionsByWeight(candidateRegions)
	default:
		s.sortRegionsByPriority(candidateRegions)
	}
	
	// Select top regions
	if count > len(candidateRegions) {
		count = len(candidateRegions)
	}
	
	return candidateRegions[:count]
}

// sortRegionsByLatency sorts regions by average latency (ascending)
func (s *DefaultRegionSelector) sortRegionsByLatency(regions []*Region) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	sort.Slice(regions, func(i, j int) bool {
		metricsI, existsI := s.regionMetrics[regions[i].Name]
		metricsJ, existsJ := s.regionMetrics[regions[j].Name]
		
		if !existsI && !existsJ {
			return regions[i].Priority < regions[j].Priority
		}
		if !existsI {
			return false
		}
		if !existsJ {
			return true
		}
		
		return metricsI.AverageLatencyMs < metricsJ.AverageLatencyMs
	})
}

// sortRegionsByWeight sorts regions by weight (descending)
func (s *DefaultRegionSelector) sortRegionsByWeight(regions []*Region) {
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Weight == regions[j].Weight {
			return regions[i].Priority < regions[j].Priority
		}
		return regions[i].Weight > regions[j].Weight
	})
}

// sortRegionsByPriority sorts regions by priority (ascending)
func (s *DefaultRegionSelector) sortRegionsByPriority(regions []*Region) {
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Priority < regions[j].Priority
	})
}


