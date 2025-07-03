package staging

import (
	"math"
	"sync"
)

// NewBoundaryDetector creates a new boundary detector.
func NewBoundaryDetector(config *StagingConfig) *BoundaryDetector {
	return &BoundaryDetector{
		compressionThresholds: map[string]float64{
			"text":       0.3,  // Text compresses to 30%
			"image":      0.9,  // Images barely compress
			"compressed": 0.95, // Already compressed
			"binary":     0.6,  // Moderate compression
			"document":   0.4,  // Good compression
		},
		sizeTargets: map[string]int{
			"text":       config.TargetChunkSizeMB * 1024 * 1024,     // Default size
			"image":      config.TargetChunkSizeMB * 2 * 1024 * 1024, // Larger chunks for images
			"compressed": config.TargetChunkSizeMB * 1024 * 1024,     // Default size
			"binary":     config.TargetChunkSizeMB * 1024 * 1024,     // Default size
			"document":   config.TargetChunkSizeMB * 1024 * 1024,     // Default size
		},
		alignmentRules: NewAlignmentRules(),
	}
}

// GenerateCandidates generates candidate chunk boundaries based on content analysis.
func (bd *BoundaryDetector) GenerateCandidates(profile *ContentProfile, expectedSize int64) []ChunkBoundary {
	var candidates []ChunkBoundary
	
	// Get target chunk size for this content type
	targetSize := bd.getTargetSize(profile.ContentType)
	
	// Strategy 1: File-aligned boundaries (preferred for tar archives)
	fileAlignedCandidates := bd.generateFileAlignedBoundaries(profile, targetSize)
	candidates = append(candidates, fileAlignedCandidates...)
	
	// Strategy 2: Pattern-aware boundaries
	patternAwareCandidates := bd.generatePatternAwareBoundaries(profile, targetSize)
	candidates = append(candidates, patternAwareCandidates...)
	
	// Strategy 3: Size-optimized boundaries (fallback)
	sizeOptimizedCandidates := bd.generateSizeOptimizedBoundaries(expectedSize, targetSize)
	candidates = append(candidates, sizeOptimizedCandidates...)
	
	// Remove duplicates and sort by quality
	candidates = bd.deduplicateAndRank(candidates)
	
	// Limit to reasonable number of candidates
	maxCandidates := 10
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	
	return candidates
}

// generateFileAlignedBoundaries generates boundaries aligned with file boundaries in tar archives.
func (bd *BoundaryDetector) generateFileAlignedBoundaries(profile *ContentProfile, targetSize int) []ChunkBoundary {
	var boundaries []ChunkBoundary
	
	if len(profile.FileAlignment) == 0 {
		return boundaries // No file alignment information
	}
	
	// Group files into chunks targeting the desired size
	currentChunkStart := int64(0)
	currentChunkSize := int64(0)
	
	for _, alignment := range profile.FileAlignment {
		// Calculate size if we include this file
		fileEndOffset := alignment.Offset + alignment.FileSize + 512 // Include tar header
		potentialChunkSize := fileEndOffset - currentChunkStart
		
		// If adding this file would exceed target by too much, create boundary here
		if currentChunkSize > 0 && potentialChunkSize > int64(targetSize)*2 {
			boundary := ChunkBoundary{
				StartOffset:     currentChunkStart,
				EndOffset:       alignment.Offset,
				Size:            alignment.Offset - currentChunkStart,
				AlignedWithFile: true,
				OptimalForNetwork: bd.isOptimalNetworkSize(alignment.Offset - currentChunkStart),
			}
			boundaries = append(boundaries, boundary)
			
			// Start new chunk
			currentChunkStart = alignment.Offset
			currentChunkSize = alignment.FileSize + 512
		} else {
			// Include this file in current chunk
			currentChunkSize = potentialChunkSize
		}
	}
	
	// Add final boundary if we have accumulated content
	if currentChunkSize > 0 {
		lastAlignment := profile.FileAlignment[len(profile.FileAlignment)-1]
		endOffset := lastAlignment.Offset + lastAlignment.FileSize + 512
		
		boundary := ChunkBoundary{
			StartOffset:     currentChunkStart,
			EndOffset:       endOffset,
			Size:            endOffset - currentChunkStart,
			AlignedWithFile: true,
			OptimalForNetwork: bd.isOptimalNetworkSize(endOffset - currentChunkStart),
		}
		boundaries = append(boundaries, boundary)
	}
	
	return boundaries
}

// generatePatternAwareBoundaries generates boundaries based on content patterns.
func (bd *BoundaryDetector) generatePatternAwareBoundaries(profile *ContentProfile, targetSize int) []ChunkBoundary {
	var boundaries []ChunkBoundary
	
	if len(profile.Patterns) == 0 {
		return boundaries
	}
	
	// Find good boundary points based on pattern transitions
	currentStart := int64(0)
	
	for i, pattern := range profile.Patterns {
		// Look for pattern transitions that might be good boundary points
		if i > 0 {
			prevPattern := profile.Patterns[i-1]
			
			// Boundary at transition between different pattern types
			if pattern.Type != prevPattern.Type {
				chunkSize := pattern.Offset - currentStart
				
				// If chunk is reasonable size, create boundary
				if chunkSize >= int64(targetSize/2) && chunkSize <= int64(targetSize*3) {
					boundary := ChunkBoundary{
						StartOffset:     currentStart,
						EndOffset:       pattern.Offset,
						Size:            chunkSize,
						AlignedWithFile: false,
						OptimalForNetwork: bd.isOptimalNetworkSize(chunkSize),
					}
					
					// Score based on pattern characteristics
					boundary.CompressionScore = bd.scorePatternBoundary(prevPattern, pattern)
					boundaries = append(boundaries, boundary)
					
					currentStart = pattern.Offset
				}
			}
		}
		
		// Also consider boundaries at the end of highly compressible patterns
		if pattern.Compressibility > 0.8 {
			patternEnd := pattern.Offset + pattern.Length
			chunkSize := patternEnd - currentStart
			
			if chunkSize >= int64(targetSize/2) {
				boundary := ChunkBoundary{
					StartOffset:     currentStart,
					EndOffset:       patternEnd,
					Size:            chunkSize,
					AlignedWithFile: false,
					OptimalForNetwork: bd.isOptimalNetworkSize(chunkSize),
					CompressionScore: pattern.Compressibility,
				}
				boundaries = append(boundaries, boundary)
			}
		}
	}
	
	return boundaries
}

// generateSizeOptimizedBoundaries generates boundaries optimized purely for size.
func (bd *BoundaryDetector) generateSizeOptimizedBoundaries(totalSize int64, targetSize int) []ChunkBoundary {
	var boundaries []ChunkBoundary
	
	if totalSize <= 0 {
		return boundaries
	}
	
	// Calculate number of chunks needed
	numChunks := int(math.Ceil(float64(totalSize) / float64(targetSize)))
	if numChunks < 1 {
		numChunks = 1
	}
	
	// Create evenly sized chunks
	chunkSize := totalSize / int64(numChunks)
	
	for i := 0; i < numChunks; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		
		// Last chunk gets remainder
		if i == numChunks-1 {
			end = totalSize
		}
		
		boundary := ChunkBoundary{
			StartOffset:     start,
			EndOffset:       end,
			Size:            end - start,
			AlignedWithFile: false,
			OptimalForNetwork: bd.isOptimalNetworkSize(end - start),
			CompressionScore: 0.5, // Neutral score for size-based boundaries
		}
		boundaries = append(boundaries, boundary)
	}
	
	return boundaries
}

// scorePatternBoundary scores a boundary between two patterns.
func (bd *BoundaryDetector) scorePatternBoundary(prevPattern, nextPattern ContentPattern) float64 {
	// Higher score for transitions from high to low compressibility
	compressionDelta := prevPattern.Compressibility - nextPattern.Compressibility
	compressionScore := math.Max(0, compressionDelta) // Reward positive transitions
	
	// Frequency difference (prefer boundaries at frequency changes)
	frequencyDelta := math.Abs(prevPattern.Frequency - nextPattern.Frequency)
	frequencyScore := math.Min(frequencyDelta, 1.0)
	
	// Pattern type transition score
	typeScore := 0.0
	if prevPattern.Type != nextPattern.Type {
		typeScore = 0.5 // Moderate reward for type transitions
	}
	
	// Combine scores
	return (compressionScore * 0.5) + (frequencyScore * 0.3) + (typeScore * 0.2)
}

// isOptimalNetworkSize checks if a chunk size is optimal for network transfer.
func (bd *BoundaryDetector) isOptimalNetworkSize(size int64) bool {
	minOptimal := int64(5 * 1024 * 1024)   // 5MB minimum
	maxOptimal := int64(100 * 1024 * 1024) // 100MB maximum
	
	return size >= minOptimal && size <= maxOptimal
}

// getTargetSize gets the target chunk size for the given content type.
func (bd *BoundaryDetector) getTargetSize(contentType string) int {
	if target, exists := bd.sizeTargets[contentType]; exists {
		return target
	}
	return bd.sizeTargets["binary"] // Default to binary target
}

// CalculateAlignmentScore calculates an alignment score for a boundary.
func (bd *BoundaryDetector) CalculateAlignmentScore(boundary ChunkBoundary, profile *ContentProfile) float64 {
	score := 0.0
	
	// File alignment bonus
	if boundary.AlignedWithFile {
		score += 0.4
	}
	
	// Network size optimization bonus
	if boundary.OptimalForNetwork {
		score += 0.3
	}
	
	// Size target alignment
	targetSize := float64(bd.getTargetSize(profile.ContentType))
	sizeDiff := math.Abs(float64(boundary.Size) - targetSize)
	sizeScore := 1.0 - (sizeDiff / targetSize)
	if sizeScore < 0 {
		sizeScore = 0
	}
	score += sizeScore * 0.3
	
	return math.Min(score, 1.0)
}

// deduplicateAndRank removes duplicate boundaries and ranks them by quality.
func (bd *BoundaryDetector) deduplicateAndRank(boundaries []ChunkBoundary) []ChunkBoundary {
	// Remove near-duplicates (within 1KB)
	unique := make([]ChunkBoundary, 0, len(boundaries))
	tolerance := int64(1024)
	
	for _, boundary := range boundaries {
		isDuplicate := false
		for _, existing := range unique {
			if math.Abs(float64(boundary.StartOffset-existing.StartOffset)) < float64(tolerance) &&
				math.Abs(float64(boundary.EndOffset-existing.EndOffset)) < float64(tolerance) {
				isDuplicate = true
				break
			}
		}
		
		if !isDuplicate {
			unique = append(unique, boundary)
		}
	}
	
	// Sort by quality score
	bd.rankBoundariesByQuality(unique)
	
	return unique
}

// rankBoundariesByQuality ranks boundaries by their overall quality score.
func (bd *BoundaryDetector) rankBoundariesByQuality(boundaries []ChunkBoundary) {
	// Sort by composite quality score
	for i := 0; i < len(boundaries); i++ {
		for j := i + 1; j < len(boundaries); j++ {
			scoreI := bd.calculateQualityScore(boundaries[i])
			scoreJ := bd.calculateQualityScore(boundaries[j])
			if scoreI < scoreJ {
				boundaries[i], boundaries[j] = boundaries[j], boundaries[i]
			}
		}
	}
}

// calculateQualityScore calculates an overall quality score for a boundary.
func (bd *BoundaryDetector) calculateQualityScore(boundary ChunkBoundary) float64 {
	alignmentWeight := 0.3
	compressionWeight := 0.4
	networkWeight := 0.3
	
	alignmentScore := boundary.PredictedRatio
	compressionScore := boundary.CompressionScore
	networkScore := 0.0
	if boundary.OptimalForNetwork {
		networkScore = 1.0
	}
	
	return (alignmentScore * alignmentWeight) +
		(compressionScore * compressionWeight) +
		(networkScore * networkWeight)
}

// AlignmentRules defines rules for aligning chunk boundaries.
type AlignmentRules struct {
	fileAlignmentBonus     float64
	sizeTargetTolerance    float64
	patternAlignmentBonus  float64
	compressionThreshold   float64
	mu                     sync.RWMutex
}

// NewAlignmentRules creates new alignment rules.
func NewAlignmentRules() *AlignmentRules {
	return &AlignmentRules{
		fileAlignmentBonus:    0.3,
		sizeTargetTolerance:   0.5,  // 50% tolerance on target size
		patternAlignmentBonus: 0.2,
		compressionThreshold:  0.1,  // 10% minimum compression benefit
	}
}

// EvaluateAlignment evaluates how well a boundary aligns with the rules.
func (ar *AlignmentRules) EvaluateAlignment(boundary ChunkBoundary, profile *ContentProfile) float64 {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	score := 0.0
	
	// File alignment evaluation
	if boundary.AlignedWithFile {
		score += ar.fileAlignmentBonus
	}
	
	// Pattern alignment evaluation
	score += ar.evaluatePatternAlignment(boundary, profile) * ar.patternAlignmentBonus
	
	// Compression benefit evaluation
	if boundary.CompressionScore > ar.compressionThreshold {
		score += boundary.CompressionScore * 0.3
	}
	
	return math.Min(score, 1.0)
}

// evaluatePatternAlignment evaluates alignment with content patterns.
func (ar *AlignmentRules) evaluatePatternAlignment(boundary ChunkBoundary, profile *ContentProfile) float64 {
	if len(profile.Patterns) == 0 {
		return 0.5 // Neutral score if no patterns
	}
	
	// Check if boundary aligns with pattern transitions
	alignmentScore := 0.0
	alignmentCount := 0
	
	for i := 1; i < len(profile.Patterns); i++ {
		transition := profile.Patterns[i].Offset
		
		// Check if boundary is near this transition
		distance := math.Abs(float64(boundary.EndOffset - transition))
		tolerance := 1024.0 // 1KB tolerance
		
		if distance <= tolerance {
			// Score based on how close we are to the transition
			proximityScore := 1.0 - (distance / tolerance)
			
			// Weight by pattern quality difference
			prevPattern := profile.Patterns[i-1]
			currentPattern := profile.Patterns[i]
			qualityDiff := math.Abs(prevPattern.Compressibility - currentPattern.Compressibility)
			
			alignmentScore += proximityScore * qualityDiff
			alignmentCount++
		}
	}
	
	if alignmentCount == 0 {
		return 0.5 // Neutral score if no alignments found
	}
	
	return alignmentScore / float64(alignmentCount)
}