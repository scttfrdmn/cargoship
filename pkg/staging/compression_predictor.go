package staging

import (
	"sync"
	"time"
)

// NewCompressionRatioPredictor creates a new compression ratio predictor.
func NewCompressionRatioPredictor(config *StagingConfig) *CompressionRatioPredictor {
	return &CompressionRatioPredictor{
		compressionStats: make(map[string]*CompressionStats),
		contentPatterns:  make(map[string]float64),
		historicalData:   NewCompressionHistory(),
	}
}

// PredictRatio predicts the compression ratio for a given chunk boundary and content profile.
func (crp *CompressionRatioPredictor) PredictRatio(boundary ChunkBoundary, profile *ContentProfile) float64 {
	crp.mu.RLock()
	defer crp.mu.RUnlock()
	
	// Start with entropy-based prediction
	entropyRatio := crp.predictFromEntropy(profile.Entropy)
	
	// Adjust based on content type
	contentTypeRatio := crp.predictFromContentType(profile.ContentType)
	
	// Adjust based on detected patterns
	patternRatio := crp.predictFromPatterns(profile.Patterns)
	
	// Combine predictions with weights
	combinedRatio := (entropyRatio * 0.4) + (contentTypeRatio * 0.4) + (patternRatio * 0.2)
	
	// Apply historical learning if available
	if historical := crp.getHistoricalRatio(profile.ContentType, boundary.Size); historical > 0 {
		combinedRatio = (combinedRatio * 0.7) + (historical * 0.3)
	}
	
	// Ensure reasonable bounds
	if combinedRatio < 0.05 {
		combinedRatio = 0.05 // Minimum 5% compression
	}
	if combinedRatio > 0.95 {
		combinedRatio = 0.95 // Maximum 95% compression
	}
	
	return combinedRatio
}

// UpdateStats updates compression statistics with actual results.
func (crp *CompressionRatioPredictor) UpdateStats(algorithm string, stats *CompressionStats) {
	crp.mu.Lock()
	defer crp.mu.Unlock()
	
	crp.compressionStats[algorithm] = stats
}

// LearnFromResult learns from actual compression results.
func (crp *CompressionRatioPredictor) LearnFromResult(contentType string, size int64, actualRatio float64) {
	crp.historicalData.AddResult(contentType, size, actualRatio)
}

// predictFromEntropy predicts compression ratio based on content entropy.
func (crp *CompressionRatioPredictor) predictFromEntropy(entropy float64) float64 {
	// Shannon entropy-based compression prediction
	// Lower entropy = better compression
	
	if entropy < 1.0 {
		return 0.9 // Highly repetitive data - excellent compression
	} else if entropy < 2.0 {
		return 0.8 // Low entropy - very good compression
	} else if entropy < 4.0 {
		return 0.6 // Medium entropy - good compression
	} else if entropy < 6.0 {
		return 0.4 // High entropy - moderate compression
	} else if entropy < 7.0 {
		return 0.2 // Very high entropy - poor compression
	}
	
	return 0.05 // Near-random data - minimal compression
}

// predictFromContentType predicts compression ratio based on content type.
func (crp *CompressionRatioPredictor) predictFromContentType(contentType string) float64 {
	switch contentType {
	case "text":
		return 0.7 // Text compresses very well
	case "json":
		return 0.6 // JSON has repetitive structure
	case "xml":
		return 0.6 // XML has repetitive structure
	case "binary":
		return 0.4 // Binary data varies widely
	case "image_jpeg", "image_png":
		return 0.05 // Images already compressed
	case "pdf":
		return 0.3 // PDFs are partially compressed
	case "zip", "compressed":
		return 0.02 // Already compressed data
	case "document":
		return 0.5 // Documents usually compress well
	default:
		return 0.3 // Default conservative estimate
	}
}

// predictFromPatterns predicts compression ratio based on detected patterns.
func (crp *CompressionRatioPredictor) predictFromPatterns(patterns []ContentPattern) float64 {
	if len(patterns) == 0 {
		return 0.3 // Default ratio if no patterns
	}
	
	totalLength := int64(0)
	weightedRatio := 0.0
	
	for _, pattern := range patterns {
		patternRatio := 0.0
		
		switch pattern.Type {
		case PatternRepetitive:
			patternRatio = 0.95 // Repetitive patterns compress extremely well
		case PatternStructured:
			patternRatio = 0.7 // Structured data compresses well
		case PatternText:
			patternRatio = 0.6 // Text patterns compress well
		case PatternBinary:
			patternRatio = 0.4 // Binary patterns moderate compression
		case PatternRandom:
			patternRatio = 0.05 // Random patterns don't compress
		}
		
		// Weight by pattern length and frequency
		weight := float64(pattern.Length) * pattern.Frequency
		weightedRatio += patternRatio * weight
		totalLength += pattern.Length
	}
	
	if totalLength == 0 {
		return 0.3
	}
	
	// Normalize by total weighted length
	return weightedRatio / float64(totalLength)
}

// getHistoricalRatio gets historical compression ratio for similar content.
func (crp *CompressionRatioPredictor) getHistoricalRatio(contentType string, size int64) float64 {
	return crp.historicalData.GetAverageRatio(contentType, size)
}

// GetStats returns compression statistics for an algorithm.
func (crp *CompressionRatioPredictor) GetStats(algorithm string) *CompressionStats {
	crp.mu.RLock()
	defer crp.mu.RUnlock()
	
	if stats, exists := crp.compressionStats[algorithm]; exists {
		return stats
	}
	
	return nil
}

// PredictBestAlgorithm predicts the best compression algorithm for the given profile.
func (crp *CompressionRatioPredictor) PredictBestAlgorithm(profile *ContentProfile, networkCondition *NetworkCondition) string {
	crp.mu.RLock()
	defer crp.mu.RUnlock()
	
	// For high-bandwidth networks, favor speed over compression ratio
	if networkCondition.BandwidthMBps > 100 {
		return "zstd-fast"
	}
	
	// For low-bandwidth networks, favor compression ratio
	if networkCondition.BandwidthMBps < 10 {
		return "zstd-max"
	}
	
	// For repetitive content, use high compression
	hasRepetitive := false
	for _, pattern := range profile.Patterns {
		if pattern.Type == PatternRepetitive && pattern.Compressibility > 0.8 {
			hasRepetitive = true
			break
		}
	}
	
	if hasRepetitive {
		return "zstd-high"
	}
	
	// For already compressed content, use fast compression
	if profile.ContentType == "compressed" || profile.ContentType == "image_jpeg" || profile.ContentType == "image_png" {
		return "zstd-fast"
	}
	
	// Default balanced compression
	return "zstd"
}

// CompressionStats represents statistics for a compression algorithm.
type CompressionStats struct {
	Algorithm         string
	AverageRatio      float64
	AverageSpeed      float64 // MB/s
	TotalCompressed   int64
	TotalUncompressed int64
	CompressionCount  int
	LastUpdated       time.Time
}

// UpdateWithResult updates compression stats with a new result.
func (cs *CompressionStats) UpdateWithResult(uncompressedSize, compressedSize int64, compressionTime time.Duration) {
	ratio := float64(compressedSize) / float64(uncompressedSize)
	speedMBps := float64(uncompressedSize) / (1024 * 1024) / compressionTime.Seconds()
	
	// Update running averages
	cs.AverageRatio = (cs.AverageRatio*float64(cs.CompressionCount) + ratio) / float64(cs.CompressionCount+1)
	cs.AverageSpeed = (cs.AverageSpeed*float64(cs.CompressionCount) + speedMBps) / float64(cs.CompressionCount+1)
	
	// Update totals
	cs.TotalCompressed += compressedSize
	cs.TotalUncompressed += uncompressedSize
	cs.CompressionCount++
	cs.LastUpdated = time.Now()
}

// CompressionHistory tracks historical compression performance.
type CompressionHistory struct {
	results    map[string][]*CompressionResult
	maxResults int
	mu         sync.RWMutex
}

// NewCompressionHistory creates a new compression history tracker.
func NewCompressionHistory() *CompressionHistory {
	return &CompressionHistory{
		results:    make(map[string][]*CompressionResult),
		maxResults: 1000,
	}
}

// AddResult adds a compression result to the history.
func (ch *CompressionHistory) AddResult(contentType string, size int64, ratio float64) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	result := &CompressionResult{
		ContentType: contentType,
		Size:        size,
		Ratio:       ratio,
		Timestamp:   time.Now(),
	}
	
	if ch.results[contentType] == nil {
		ch.results[contentType] = make([]*CompressionResult, 0)
	}
	
	ch.results[contentType] = append(ch.results[contentType], result)
	
	// Limit history size per content type
	if len(ch.results[contentType]) > ch.maxResults {
		ch.results[contentType] = ch.results[contentType][1:]
	}
}

// GetAverageRatio gets the average compression ratio for similar content.
func (ch *CompressionHistory) GetAverageRatio(contentType string, size int64) float64 {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	results, exists := ch.results[contentType]
	if !exists || len(results) == 0 {
		return 0.0 // No historical data
	}
	
	// Find results for similar sized content
	tolerance := size / 5 // 20% tolerance
	similarResults := make([]*CompressionResult, 0)
	
	for _, result := range results {
		if abs64(result.Size-size) <= tolerance {
			similarResults = append(similarResults, result)
		}
	}
	
	if len(similarResults) == 0 {
		// Fall back to all results for this content type
		similarResults = results
	}
	
	// Calculate weighted average (recent results weighted higher)
	totalWeight := 0.0
	weightedSum := 0.0
	now := time.Now()
	
	for _, result := range similarResults {
		// Weight decreases with age (max 30 days)
		age := now.Sub(result.Timestamp)
		var weight float64
		if age > time.Hour*24*30 {
			weight = 0.1
		} else {
			weight = 1.0 - float64(age)/(float64(time.Hour*24*30))
		}
		
		weightedSum += result.Ratio * weight
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return 0.0
	}
	
	return weightedSum / totalWeight
}

// GetResultsForContentType returns all results for a content type.
func (ch *CompressionHistory) GetResultsForContentType(contentType string) []*CompressionResult {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	if results, exists := ch.results[contentType]; exists {
		// Return a copy to prevent race conditions
		resultsCopy := make([]*CompressionResult, len(results))
		copy(resultsCopy, results)
		return resultsCopy
	}
	
	return nil
}

// CompressionResult represents a historical compression result.
type CompressionResult struct {
	ContentType string
	Size        int64
	Ratio       float64
	Timestamp   time.Time
}

// abs64 returns the absolute value of a 64-bit integer.
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// PredictCompressionTime predicts how long compression will take.
func (crp *CompressionRatioPredictor) PredictCompressionTime(size int64, algorithm string) time.Duration {
	crp.mu.RLock()
	defer crp.mu.RUnlock()
	
	// Get stats for the algorithm
	stats := crp.compressionStats[algorithm]
	if stats == nil {
		// Use default estimates if no stats available
		return crp.getDefaultCompressionTime(size, algorithm)
	}
	
	// Calculate time based on average speed
	sizeMB := float64(size) / (1024 * 1024)
	if stats.AverageSpeed > 0 {
		timeSeconds := sizeMB / stats.AverageSpeed
		return time.Duration(timeSeconds * float64(time.Second))
	}
	
	return crp.getDefaultCompressionTime(size, algorithm)
}

// getDefaultCompressionTime returns default compression time estimates.
func (crp *CompressionRatioPredictor) getDefaultCompressionTime(size int64, algorithm string) time.Duration {
	sizeMB := float64(size) / (1024 * 1024)
	
	// Default speeds (MB/s) for different algorithms
	defaultSpeeds := map[string]float64{
		"zstd-fast": 300, // Very fast
		"zstd":      150, // Balanced
		"zstd-high": 50,  // High compression
		"zstd-max":  20,  // Maximum compression
	}
	
	speed, exists := defaultSpeeds[algorithm]
	if !exists {
		speed = 100 // Default speed
	}
	
	timeSeconds := sizeMB / speed
	return time.Duration(timeSeconds * float64(time.Second))
}

// EstimateCompressionBenefit estimates the benefit of compression vs no compression.
func (crp *CompressionRatioPredictor) EstimateCompressionBenefit(profile *ContentProfile, networkCondition *NetworkCondition, algorithm string) *CompressionBenefit {
	// Predict compression ratio
	compressionRatio := crp.PredictRatio(ChunkBoundary{CompressionScore: 0.5}, profile)
	
	// Predict compression time
	estimatedSize := int64(10 * 1024 * 1024) // Assume 10MB for estimation
	compressionTime := crp.PredictCompressionTime(estimatedSize, algorithm)
	
	// Calculate transfer time savings
	originalTransferTime := float64(estimatedSize) / (networkCondition.BandwidthMBps * 1024 * 1024)
	compressedSize := float64(estimatedSize) * compressionRatio
	compressedTransferTime := compressedSize / (networkCondition.BandwidthMBps * 1024 * 1024)
	
	transferTimeSavings := time.Duration((originalTransferTime - compressedTransferTime) * float64(time.Second))
	
	// Net benefit = transfer time savings - compression time
	netBenefit := transferTimeSavings - compressionTime
	
	return &CompressionBenefit{
		Algorithm:            algorithm,
		PredictedRatio:      compressionRatio,
		CompressionTime:     compressionTime,
		TransferTimeSavings: transferTimeSavings,
		NetBenefit:          netBenefit,
		Recommended:         netBenefit > 0,
	}
}

// CompressionBenefit represents the estimated benefit of compression.
type CompressionBenefit struct {
	Algorithm            string
	PredictedRatio      float64
	CompressionTime     time.Duration
	TransferTimeSavings time.Duration
	NetBenefit          time.Duration
	Recommended         bool
}