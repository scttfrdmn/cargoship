// Package s3 provides adaptive multipart upload optimization for CargoShip
package s3

import (
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AdaptiveUploader provides intelligent multipart sizing based on network conditions
type AdaptiveUploader struct {
	client          *s3.Client
	networkMonitor  *NetworkMonitor
	uploadHistory   *UploadHistory
	config          AdaptiveConfig
	mutex           sync.RWMutex
}

// AdaptiveConfig configures adaptive upload behavior
type AdaptiveConfig struct {
	// MinChunkSize is the minimum chunk size in bytes (default: 5MB)
	MinChunkSize int64
	
	// MaxChunkSize is the maximum chunk size in bytes (default: 100MB)
	MaxChunkSize int64
	
	// InitialChunkSize is the starting chunk size (default: 8MB)
	InitialChunkSize int64
	
	// MaxConcurrency is the maximum concurrent uploads (default: 10)
	MaxConcurrency int
	
	// AdaptationInterval how often to adjust sizing (default: 30s)
	AdaptationInterval time.Duration
	
	// NetworkSamples number of samples for network condition averaging
	NetworkSamples int
	
	// EnableContentTypeOptimization adjusts based on file type
	EnableContentTypeOptimization bool
}

// NetworkMonitor tracks network performance metrics
type NetworkMonitor struct {
	bandwidth     float64       // MB/s
	latency       time.Duration // Round-trip time
	samples       []NetworkSample
	maxSamples    int
	mutex         sync.RWMutex
	lastUpdate    time.Time
}

// NetworkSample represents a single network measurement
type NetworkSample struct {
	Timestamp   time.Time
	Bandwidth   float64       // MB/s
	Latency     time.Duration
	ChunkSize   int64
	Success     bool
	Error       error
}

// UploadHistory tracks upload performance for learning
type UploadHistory struct {
	sessions    []UploadSession
	maxSessions int
	mutex       sync.RWMutex
}

// UploadSession contains metrics for a complete upload session
type UploadSession struct {
	StartTime     time.Time
	EndTime       time.Time
	TotalSize     int64
	ChunkSizes    []int64
	Throughputs   []float64 // MB/s per chunk
	Concurrency   int
	Success       bool
	OptimalChunk  int64
	OptimalConcurrency int
	ContentType   string
	NetworkCondition string // "poor", "fair", "good", "excellent"
}

// NewAdaptiveUploader creates a new adaptive uploader
func NewAdaptiveUploader(client *s3.Client, config AdaptiveConfig) *AdaptiveUploader {
	if config.MinChunkSize <= 0 {
		config.MinChunkSize = 5 * 1024 * 1024 // 5MB minimum for S3
	}
	if config.MaxChunkSize <= 0 {
		config.MaxChunkSize = 100 * 1024 * 1024 // 100MB maximum
	}
	if config.InitialChunkSize <= 0 {
		config.InitialChunkSize = 8 * 1024 * 1024 // 8MB default
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}
	if config.AdaptationInterval <= 0 {
		config.AdaptationInterval = 30 * time.Second
	}
	if config.NetworkSamples <= 0 {
		config.NetworkSamples = 10
	}

	return &AdaptiveUploader{
		client: client,
		networkMonitor: &NetworkMonitor{
			maxSamples: config.NetworkSamples,
			samples:    make([]NetworkSample, 0, config.NetworkSamples),
		},
		uploadHistory: &UploadHistory{
			maxSessions: 50, // Keep last 50 sessions
		},
		config: config,
	}
}

// CalculateOptimalChunkSize determines the best chunk size for current conditions
func (a *AdaptiveUploader) CalculateOptimalChunkSize(fileSize int64, contentType string) int64 {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	// Start with base calculation
	chunkSize := a.calculateBaseChunkSize(fileSize)
	
	// Adjust based on network conditions
	chunkSize = a.adjustForNetworkConditions(chunkSize)
	
	// Adjust based on content type if enabled
	if a.config.EnableContentTypeOptimization {
		chunkSize = a.adjustForContentType(chunkSize, contentType)
	}
	
	// Apply historical learning
	chunkSize = a.adjustForHistory(chunkSize, contentType)
	
	// Ensure within bounds
	return a.clampChunkSize(chunkSize)
}

// calculateBaseChunkSize determines initial chunk size based on file size
func (a *AdaptiveUploader) calculateBaseChunkSize(fileSize int64) int64 {
	switch {
	case fileSize < 100*1024*1024: // < 100MB
		return 8 * 1024 * 1024 // 8MB chunks
	case fileSize < 1024*1024*1024: // < 1GB
		return 16 * 1024 * 1024 // 16MB chunks
	case fileSize < 10*1024*1024*1024: // < 10GB
		return 32 * 1024 * 1024 // 32MB chunks
	default: // >= 10GB
		return 64 * 1024 * 1024 // 64MB chunks
	}
}

// adjustForNetworkConditions modifies chunk size based on network performance
func (a *AdaptiveUploader) adjustForNetworkConditions(baseSize int64) int64 {
	bandwidth := a.networkMonitor.GetAverageBandwidth()
	latency := a.networkMonitor.GetAverageLatency()
	
	// If no network data, return base size
	if bandwidth <= 0 {
		return baseSize
	}
	
	// Adjust based on bandwidth
	var bandwidthMultiplier float64
	switch {
	case bandwidth < 1.0: // < 1 MB/s (poor connection)
		bandwidthMultiplier = 0.5 // Smaller chunks for poor connections
	case bandwidth < 5.0: // < 5 MB/s (fair connection)
		bandwidthMultiplier = 0.75
	case bandwidth < 25.0: // < 25 MB/s (good connection)
		bandwidthMultiplier = 1.0
	default: // >= 25 MB/s (excellent connection)
		bandwidthMultiplier = 1.5 // Larger chunks for fast connections
	}
	
	// Adjust based on latency
	var latencyMultiplier float64
	switch {
	case latency > 500*time.Millisecond: // High latency
		latencyMultiplier = 1.5 // Larger chunks to reduce round trips
	case latency > 200*time.Millisecond: // Medium latency
		latencyMultiplier = 1.2
	default: // Low latency
		latencyMultiplier = 1.0
	}
	
	adjusted := float64(baseSize) * bandwidthMultiplier * latencyMultiplier
	return int64(adjusted)
}

// adjustForContentType optimizes based on file type characteristics
func (a *AdaptiveUploader) adjustForContentType(baseSize int64, contentType string) int64 {
	multiplier := 1.0
	
	switch contentType {
	case "application/zip", "application/x-tar":
		// Already compressed archives - larger chunks
		multiplier = 1.3
	case "video/mp4", "video/mov":
		// Video files - larger chunks for efficiency
		multiplier = 1.4
	case "image/jpeg", "image/png":
		// Images - moderate chunks
		multiplier = 1.1
	case "text/plain", "application/json":
		// Text files compress well - smaller chunks for parallel compression
		multiplier = 0.8
	case "application/octet-stream":
		// Binary data - use default
		multiplier = 1.0
	}
	
	return int64(float64(baseSize) * multiplier)
}

// adjustForHistory applies learning from previous uploads
func (a *AdaptiveUploader) adjustForHistory(baseSize int64, contentType string) int64 {
	a.uploadHistory.mutex.RLock()
	defer a.uploadHistory.mutex.RUnlock()
	
	// Find similar uploads (same content type)
	var similarSessions []UploadSession
	for _, session := range a.uploadHistory.sessions {
		if session.ContentType == contentType && session.Success {
			similarSessions = append(similarSessions, session)
		}
	}
	
	if len(similarSessions) == 0 {
		return baseSize
	}
	
	// Calculate average optimal chunk size from history
	var totalOptimal int64
	for _, session := range similarSessions {
		totalOptimal += session.OptimalChunk
	}
	
	historicalOptimal := totalOptimal / int64(len(similarSessions))
	
	// Blend historical data with current calculation (70% current, 30% historical)
	blended := int64(0.7*float64(baseSize) + 0.3*float64(historicalOptimal))
	
	return blended
}

// clampChunkSize ensures chunk size is within configured bounds
func (a *AdaptiveUploader) clampChunkSize(size int64) int64 {
	if size < a.config.MinChunkSize {
		return a.config.MinChunkSize
	}
	if size > a.config.MaxChunkSize {
		return a.config.MaxChunkSize
	}
	return size
}

// CalculateOptimalConcurrency determines the best concurrency level
func (a *AdaptiveUploader) CalculateOptimalConcurrency(fileSize int64, chunkSize int64) int {
	// Calculate total number of chunks
	numChunks := int(math.Ceil(float64(fileSize) / float64(chunkSize)))
	
	// Base concurrency on network conditions
	bandwidth := a.networkMonitor.GetAverageBandwidth()
	
	var baseConcurrency int
	switch {
	case bandwidth < 1.0: // Poor connection
		baseConcurrency = 2
	case bandwidth < 5.0: // Fair connection
		baseConcurrency = 4
	case bandwidth < 25.0: // Good connection
		baseConcurrency = 8
	default: // Excellent connection
		baseConcurrency = a.config.MaxConcurrency
	}
	
	// Don't exceed number of chunks
	if baseConcurrency > numChunks {
		baseConcurrency = numChunks
	}
	
	// Apply historical learning
	optimalFromHistory := a.getOptimalConcurrencyFromHistory()
	if optimalFromHistory > 0 {
		// Blend with historical data
		blended := int(0.7*float64(baseConcurrency) + 0.3*float64(optimalFromHistory))
		if blended > 0 && blended <= a.config.MaxConcurrency {
			baseConcurrency = blended
		}
	}
	
	return baseConcurrency
}

// RecordNetworkSample adds a network performance measurement
func (a *AdaptiveUploader) RecordNetworkSample(sample NetworkSample) {
	a.networkMonitor.mutex.Lock()
	defer a.networkMonitor.mutex.Unlock()
	
	// Add sample
	a.networkMonitor.samples = append(a.networkMonitor.samples, sample)
	
	// Remove old samples if we exceed max
	if len(a.networkMonitor.samples) > a.networkMonitor.maxSamples {
		a.networkMonitor.samples = a.networkMonitor.samples[1:]
	}
	
	a.networkMonitor.lastUpdate = time.Now()
	
	// Update averages
	a.updateNetworkAverages()
}

// updateNetworkAverages recalculates network performance averages
func (a *AdaptiveUploader) updateNetworkAverages() {
	samples := a.networkMonitor.samples
	if len(samples) == 0 {
		return
	}
	
	var totalBandwidth float64
	var totalLatency time.Duration
	var successCount int
	
	for _, sample := range samples {
		if sample.Success {
			totalBandwidth += sample.Bandwidth
			totalLatency += sample.Latency
			successCount++
		}
	}
	
	if successCount > 0 {
		a.networkMonitor.bandwidth = totalBandwidth / float64(successCount)
		a.networkMonitor.latency = totalLatency / time.Duration(successCount)
	}
}

// RecordUploadSession adds a completed upload session to history
func (a *AdaptiveUploader) RecordUploadSession(session UploadSession) {
	a.uploadHistory.mutex.Lock()
	defer a.uploadHistory.mutex.Unlock()
	
	// Calculate optimal values for this session
	session.OptimalChunk = a.calculateSessionOptimalChunk(session)
	session.OptimalConcurrency = a.calculateSessionOptimalConcurrency(session)
	session.NetworkCondition = a.classifyNetworkCondition(session)
	
	// Add session
	a.uploadHistory.sessions = append(a.uploadHistory.sessions, session)
	
	// Remove old sessions if we exceed max
	if len(a.uploadHistory.sessions) > a.uploadHistory.maxSessions {
		a.uploadHistory.sessions = a.uploadHistory.sessions[1:]
	}
	
	slog.Debug("recorded upload session", 
		"duration", session.EndTime.Sub(session.StartTime),
		"size", session.TotalSize,
		"optimal_chunk", session.OptimalChunk,
		"optimal_concurrency", session.OptimalConcurrency,
		"network_condition", session.NetworkCondition)
}

// calculateSessionOptimalChunk determines the best chunk size from a session
func (a *AdaptiveUploader) calculateSessionOptimalChunk(session UploadSession) int64 {
	if len(session.ChunkSizes) == 0 || len(session.Throughputs) == 0 {
		return a.config.InitialChunkSize
	}
	
	// Find chunk size with highest throughput
	maxThroughput := 0.0
	optimalChunk := session.ChunkSizes[0]
	
	for i, throughput := range session.Throughputs {
		if i < len(session.ChunkSizes) && throughput > maxThroughput {
			maxThroughput = throughput
			optimalChunk = session.ChunkSizes[i]
		}
	}
	
	return optimalChunk
}

// calculateSessionOptimalConcurrency determines best concurrency from session
func (a *AdaptiveUploader) calculateSessionOptimalConcurrency(session UploadSession) int {
	// This is simplified - in practice would analyze concurrent performance
	if session.Success {
		return session.Concurrency
	}
	return max(1, session.Concurrency-1) // Reduce if failed
}

// classifyNetworkCondition categorizes network performance
func (a *AdaptiveUploader) classifyNetworkCondition(session UploadSession) string {
	if len(session.Throughputs) == 0 {
		return "unknown"
	}
	
	// Calculate average throughput
	var totalThroughput float64
	for _, throughput := range session.Throughputs {
		totalThroughput += throughput
	}
	avgThroughput := totalThroughput / float64(len(session.Throughputs))
	
	switch {
	case avgThroughput < 1.0:
		return "poor"
	case avgThroughput < 5.0:
		return "fair"
	case avgThroughput < 25.0:
		return "good"
	default:
		return "excellent"
	}
}

// getOptimalConcurrencyFromHistory retrieves best concurrency from past sessions
func (a *AdaptiveUploader) getOptimalConcurrencyFromHistory() int {
	a.uploadHistory.mutex.RLock()
	defer a.uploadHistory.mutex.RUnlock()
	
	if len(a.uploadHistory.sessions) == 0 {
		return 0
	}
	
	// Calculate weighted average of recent successful sessions
	var totalWeightedConcurrency float64
	var totalWeight float64
	
	now := time.Now()
	for _, session := range a.uploadHistory.sessions {
		if !session.Success {
			continue
		}
		
		// Weight recent sessions more heavily
		age := now.Sub(session.EndTime)
		weight := math.Exp(-age.Hours() / 24.0) // Exponential decay over days
		
		totalWeightedConcurrency += float64(session.OptimalConcurrency) * weight
		totalWeight += weight
	}
	
	if totalWeight > 0 {
		return int(totalWeightedConcurrency / totalWeight)
	}
	
	return 0
}

// GetNetworkCondition returns current network condition assessment
func (a *AdaptiveUploader) GetNetworkCondition() string {
	bandwidth := a.networkMonitor.GetAverageBandwidth()
	
	switch {
	case bandwidth < 1.0:
		return "poor"
	case bandwidth < 5.0:
		return "fair"
	case bandwidth < 25.0:
		return "good"
	default:
		return "excellent"
	}
}

// GetAverageBandwidth returns the current average bandwidth
func (nm *NetworkMonitor) GetAverageBandwidth() float64 {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()
	return nm.bandwidth
}

// GetAverageLatency returns the current average latency
func (nm *NetworkMonitor) GetAverageLatency() time.Duration {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()
	return nm.latency
}

// GetRecommendations provides optimization recommendations
func (a *AdaptiveUploader) GetRecommendations(fileSize int64, contentType string) *UploadRecommendations {
	chunkSize := a.CalculateOptimalChunkSize(fileSize, contentType)
	concurrency := a.CalculateOptimalConcurrency(fileSize, chunkSize)
	
	return &UploadRecommendations{
		OptimalChunkSize:    chunkSize,
		OptimalConcurrency:  concurrency,
		NetworkCondition:    a.GetNetworkCondition(),
		EstimatedDuration:   a.estimateUploadDuration(fileSize, chunkSize, concurrency),
		ConfidenceLevel:     a.calculateConfidence(),
		Reasoning:          a.generateReasoningText(fileSize, contentType, chunkSize, concurrency),
	}
}

// UploadRecommendations contains optimization suggestions
type UploadRecommendations struct {
	OptimalChunkSize   int64         `json:"optimal_chunk_size"`
	OptimalConcurrency int           `json:"optimal_concurrency"`
	NetworkCondition   string        `json:"network_condition"`
	EstimatedDuration  time.Duration `json:"estimated_duration"`
	ConfidenceLevel    float64       `json:"confidence_level"`
	Reasoning          string        `json:"reasoning"`
}

// estimateUploadDuration provides time estimation
func (a *AdaptiveUploader) estimateUploadDuration(fileSize, chunkSize int64, concurrency int) time.Duration {
	bandwidth := a.networkMonitor.GetAverageBandwidth()
	if bandwidth <= 0 {
		bandwidth = 5.0 // Default assumption: 5 MB/s
	}
	
	// Account for concurrency efficiency (not linear)
	effectiveBandwidth := bandwidth * (0.7 + 0.3/float64(concurrency))
	
	// Estimate time in seconds
	estimatedSeconds := float64(fileSize) / (1024 * 1024) / effectiveBandwidth
	
	return time.Duration(estimatedSeconds * float64(time.Second))
}

// calculateConfidence determines confidence in recommendations
func (a *AdaptiveUploader) calculateConfidence() float64 {
	// Base confidence on amount of historical data
	sampleCount := len(a.networkMonitor.samples)
	sessionCount := len(a.uploadHistory.sessions)
	
	networkConfidence := math.Min(float64(sampleCount)/10.0, 1.0)
	historyConfidence := math.Min(float64(sessionCount)/20.0, 1.0)
	
	return (networkConfidence + historyConfidence) / 2.0
}

// generateReasoningText explains the optimization choices
func (a *AdaptiveUploader) generateReasoningText(fileSize int64, contentType string, chunkSize int64, concurrency int) string {
	condition := a.GetNetworkCondition()
	
	return fmt.Sprintf("Based on %s network conditions (%.1f MB/s avg), %s content type, and %.1f MB file size. Optimized for %d concurrent %.1f MB chunks.",
		condition,
		a.networkMonitor.GetAverageBandwidth(),
		contentType,
		float64(fileSize)/(1024*1024),
		concurrency,
		float64(chunkSize)/(1024*1024))
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}