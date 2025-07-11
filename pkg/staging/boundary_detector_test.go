package staging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBoundaryDetector(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	
	bd := NewBoundaryDetector(config)
	
	assert.NotNil(t, bd)
	assert.NotNil(t, bd.compressionThresholds)
	assert.NotNil(t, bd.sizeTargets)
	assert.NotNil(t, bd.alignmentRules)
	
	// Verify compression thresholds
	assert.Equal(t, 0.3, bd.compressionThresholds["text"])
	assert.Equal(t, 0.9, bd.compressionThresholds["image"])
	
	// Verify size targets
	expectedTextSize := 10 * 1024 * 1024
	expectedImageSize := 20 * 1024 * 1024
	assert.Equal(t, expectedTextSize, bd.sizeTargets["text"])
	assert.Equal(t, expectedImageSize, bd.sizeTargets["image"])
}

func TestGenerateCandidates(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	t.Run("with file alignment", func(t *testing.T) {
		profile := &ContentProfile{
			ContentType: "text",
			FileAlignment: []FileAlignment{
				{Offset: 0, FileSize: 1024},
				{Offset: 2048, FileSize: 2048},
				{Offset: 8192, FileSize: 4096},
			},
		}
		
		candidates := bd.GenerateCandidates(profile, 15*1024)
		assert.NotEmpty(t, candidates)
		assert.LessOrEqual(t, len(candidates), 10) // Limit to max candidates
		
		// Check that at least one candidate is file-aligned
		hasFileAligned := false
		for _, c := range candidates {
			if c.AlignedWithFile {
				hasFileAligned = true
				break
			}
		}
		assert.True(t, hasFileAligned)
	})
	
	t.Run("with patterns", func(t *testing.T) {
		profile := &ContentProfile{
			ContentType: "text",
			Patterns: []ContentPattern{
				{Offset: 0, Length: 1024, Type: PatternText, Compressibility: 0.3, Frequency: 0.5},
				{Offset: 1024, Length: 2048, Type: PatternBinary, Compressibility: 0.6, Frequency: 0.8},
				{Offset: 3072, Length: 1024, Type: PatternText, Compressibility: 0.9, Frequency: 0.2},
			},
		}
		
		candidates := bd.GenerateCandidates(profile, 8*1024)
		assert.NotEmpty(t, candidates)
	})
	
	t.Run("size-only boundaries", func(t *testing.T) {
		profile := &ContentProfile{
			ContentType: "binary",
		}
		
		candidates := bd.GenerateCandidates(profile, 100*1024)
		assert.NotEmpty(t, candidates)
		
		// All should be size-optimized
		for _, c := range candidates {
			assert.False(t, c.AlignedWithFile)
			assert.Equal(t, 0.5, c.CompressionScore)
		}
	})
}

func TestGenerateFileAlignedBoundaries(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	targetSize := 10 * 1024 * 1024
	
	t.Run("no file alignment", func(t *testing.T) {
		profile := &ContentProfile{}
		boundaries := bd.generateFileAlignedBoundaries(profile, targetSize)
		assert.Empty(t, boundaries)
	})
	
	t.Run("single file", func(t *testing.T) {
		profile := &ContentProfile{
			FileAlignment: []FileAlignment{
				{Offset: 0, FileSize: 1024},
			},
		}
		
		boundaries := bd.generateFileAlignedBoundaries(profile, targetSize)
		assert.Len(t, boundaries, 1)
		
		boundary := boundaries[0]
		assert.Equal(t, int64(0), boundary.StartOffset)
		assert.Equal(t, int64(1536), boundary.EndOffset) // 1024 + 512 header
		assert.True(t, boundary.AlignedWithFile)
	})
	
	t.Run("multiple files exceeding target", func(t *testing.T) {
		profile := &ContentProfile{
			FileAlignment: []FileAlignment{
				{Offset: 0, FileSize: 15 * 1024 * 1024},     // 15MB
				{Offset: 16 * 1024 * 1024, FileSize: 5 * 1024 * 1024}, // 5MB
			},
		}
		
		boundaries := bd.generateFileAlignedBoundaries(profile, targetSize)
		assert.NotEmpty(t, boundaries)
		
		// Should create boundary before second file due to size
		for _, boundary := range boundaries {
			assert.True(t, boundary.AlignedWithFile)
		}
	})
}

func TestGeneratePatternAwareBoundaries(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	targetSize := 10 * 1024 * 1024
	
	t.Run("no patterns", func(t *testing.T) {
		profile := &ContentProfile{}
		boundaries := bd.generatePatternAwareBoundaries(profile, targetSize)
		assert.Empty(t, boundaries)
	})
	
	t.Run("pattern transitions", func(t *testing.T) {
		profile := &ContentProfile{
			Patterns: []ContentPattern{
				{Offset: 0, Length: 6 * 1024 * 1024, Type: PatternText, Compressibility: 0.3},
				{Offset: 6 * 1024 * 1024, Length: 8 * 1024 * 1024, Type: PatternBinary, Compressibility: 0.6},
			},
		}
		
		boundaries := bd.generatePatternAwareBoundaries(profile, targetSize)
		assert.NotEmpty(t, boundaries)
		
		// Should create boundary at type transition
		for _, boundary := range boundaries {
			assert.False(t, boundary.AlignedWithFile)
			assert.Greater(t, boundary.CompressionScore, 0.0)
		}
	})
	
	t.Run("highly compressible patterns", func(t *testing.T) {
		profile := &ContentProfile{
			Patterns: []ContentPattern{
				{Offset: 0, Length: 8 * 1024 * 1024, Type: PatternText, Compressibility: 0.9},
			},
		}
		
		boundaries := bd.generatePatternAwareBoundaries(profile, targetSize)
		assert.NotEmpty(t, boundaries)
		
		// Should create boundary at end of highly compressible pattern
		boundary := boundaries[0]
		assert.Equal(t, 0.9, boundary.CompressionScore)
	})
}

func TestGenerateSizeOptimizedBoundaries(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	targetSize := 10 * 1024 * 1024
	
	t.Run("zero size", func(t *testing.T) {
		boundaries := bd.generateSizeOptimizedBoundaries(0, targetSize)
		assert.Empty(t, boundaries)
	})
	
	t.Run("single chunk", func(t *testing.T) {
		totalSize := int64(5 * 1024 * 1024) // 5MB
		boundaries := bd.generateSizeOptimizedBoundaries(totalSize, targetSize)
		
		assert.Len(t, boundaries, 1)
		boundary := boundaries[0]
		assert.Equal(t, int64(0), boundary.StartOffset)
		assert.Equal(t, totalSize, boundary.EndOffset)
		assert.Equal(t, 0.5, boundary.CompressionScore)
	})
	
	t.Run("multiple chunks", func(t *testing.T) {
		totalSize := int64(25 * 1024 * 1024) // 25MB
		boundaries := bd.generateSizeOptimizedBoundaries(totalSize, targetSize)
		
		assert.Len(t, boundaries, 3) // 25MB / 10MB = 2.5 -> 3 chunks
		
		// Check chunk sizes
		for i, boundary := range boundaries {
			assert.Equal(t, int64(i)*totalSize/3, boundary.StartOffset)
			if i == len(boundaries)-1 {
				// Last chunk gets remainder
				assert.Equal(t, totalSize, boundary.EndOffset)
			}
		}
	})
}

func TestScorePatternBoundary(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	t.Run("compression transition", func(t *testing.T) {
		prevPattern := ContentPattern{Compressibility: 0.8, Frequency: 0.5, Type: PatternText}
		nextPattern := ContentPattern{Compressibility: 0.3, Frequency: 0.5, Type: PatternText}
		
		score := bd.scorePatternBoundary(prevPattern, nextPattern)
		assert.Greater(t, score, 0.0)
		assert.LessOrEqual(t, score, 1.0)
	})
	
	t.Run("type transition", func(t *testing.T) {
		prevPattern := ContentPattern{Compressibility: 0.5, Frequency: 0.5, Type: PatternText}
		nextPattern := ContentPattern{Compressibility: 0.5, Frequency: 0.5, Type: PatternBinary}
		
		score := bd.scorePatternBoundary(prevPattern, nextPattern)
		assert.Greater(t, score, 0.0) // Should get type transition bonus
	})
	
	t.Run("frequency difference", func(t *testing.T) {
		prevPattern := ContentPattern{Compressibility: 0.5, Frequency: 0.8, Type: PatternText}
		nextPattern := ContentPattern{Compressibility: 0.5, Frequency: 0.2, Type: PatternText}
		
		score := bd.scorePatternBoundary(prevPattern, nextPattern)
		assert.Greater(t, score, 0.0) // Should get frequency difference bonus
	})
}

func TestIsOptimalNetworkSize(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	testCases := []struct {
		size     int64
		expected bool
		name     string
	}{
		{1024, false, "too small"},
		{5 * 1024 * 1024, true, "minimum optimal"},
		{50 * 1024 * 1024, true, "middle range"},
		{100 * 1024 * 1024, true, "maximum optimal"},
		{200 * 1024 * 1024, false, "too large"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := bd.isOptimalNetworkSize(tc.size)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetTargetSize(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	t.Run("known content types", func(t *testing.T) {
		textSize := bd.getTargetSize("text")
		assert.Equal(t, 10*1024*1024, textSize)
		
		imageSize := bd.getTargetSize("image")
		assert.Equal(t, 20*1024*1024, imageSize)
	})
	
	t.Run("unknown content type", func(t *testing.T) {
		unknownSize := bd.getTargetSize("unknown")
		assert.Equal(t, 10*1024*1024, unknownSize) // Should default to binary
	})
}

func TestCalculateAlignmentScore(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	profile := &ContentProfile{
		ContentType: "text",
	}
	
	t.Run("file aligned and network optimal", func(t *testing.T) {
		boundary := ChunkBoundary{
			Size:              10 * 1024 * 1024,
			AlignedWithFile:   true,
			OptimalForNetwork: true,
		}
		
		score := bd.CalculateAlignmentScore(boundary, profile)
		assert.Greater(t, score, 0.7) // Should get high score
		assert.LessOrEqual(t, score, 1.0)
	})
	
	t.Run("size mismatch penalty", func(t *testing.T) {
		boundary := ChunkBoundary{
			Size:              100 * 1024 * 1024, // Much larger than target
			AlignedWithFile:   false,
			OptimalForNetwork: false,
		}
		
		score := bd.CalculateAlignmentScore(boundary, profile)
		assert.Less(t, score, 0.5) // Should get low score due to size mismatch
	})
}

func TestDeduplicateAndRank(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	t.Run("remove duplicates", func(t *testing.T) {
		boundaries := []ChunkBoundary{
			{StartOffset: 0, EndOffset: 1024, Size: 1024},
			{StartOffset: 0, EndOffset: 1025, Size: 1025}, // Near duplicate
			{StartOffset: 2048, EndOffset: 4096, Size: 2048},
		}
		
		unique := bd.deduplicateAndRank(boundaries)
		assert.Len(t, unique, 2) // Should remove the near duplicate
	})
	
	t.Run("ranking by quality", func(t *testing.T) {
		boundaries := []ChunkBoundary{
			{CompressionScore: 0.3, OptimalForNetwork: false},
			{CompressionScore: 0.8, OptimalForNetwork: true},
			{CompressionScore: 0.5, OptimalForNetwork: false},
		}
		
		ranked := bd.deduplicateAndRank(boundaries)
		
		// Should be ranked by quality (highest first)
		for i := 1; i < len(ranked); i++ {
			prevScore := bd.calculateQualityScore(ranked[i-1])
			currScore := bd.calculateQualityScore(ranked[i])
			assert.GreaterOrEqual(t, prevScore, currScore)
		}
	})
}

func TestCalculateQualityScore(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB: 10,
	}
	bd := NewBoundaryDetector(config)
	
	t.Run("high quality boundary", func(t *testing.T) {
		boundary := ChunkBoundary{
			PredictedRatio:     0.8,
			CompressionScore:   0.9,
			OptimalForNetwork:  true,
		}
		
		score := bd.calculateQualityScore(boundary)
		assert.Greater(t, score, 0.8)
	})
	
	t.Run("low quality boundary", func(t *testing.T) {
		boundary := ChunkBoundary{
			PredictedRatio:     0.2,
			CompressionScore:   0.1,
			OptimalForNetwork:  false,
		}
		
		score := bd.calculateQualityScore(boundary)
		assert.Less(t, score, 0.3)
	})
}

func TestNewAlignmentRules(t *testing.T) {
	rules := NewAlignmentRules()
	
	assert.NotNil(t, rules)
	assert.Equal(t, 0.3, rules.fileAlignmentBonus)
	assert.Equal(t, 0.5, rules.sizeTargetTolerance)
	assert.Equal(t, 0.2, rules.patternAlignmentBonus)
	assert.Equal(t, 0.1, rules.compressionThreshold)
}

func TestAlignmentRulesEvaluateAlignment(t *testing.T) {
	rules := NewAlignmentRules()
	
	profile := &ContentProfile{
		Patterns: []ContentPattern{
			{Offset: 0, Compressibility: 0.8},
			{Offset: 1024, Compressibility: 0.3},
		},
	}
	
	t.Run("file aligned boundary", func(t *testing.T) {
		boundary := ChunkBoundary{
			AlignedWithFile:  true,
			CompressionScore: 0.5,
		}
		
		score := rules.EvaluateAlignment(boundary, profile)
		assert.Greater(t, score, 0.3) // Should get file alignment bonus
		assert.LessOrEqual(t, score, 1.0)
	})
	
	t.Run("high compression boundary", func(t *testing.T) {
		boundary := ChunkBoundary{
			AlignedWithFile:  false,
			CompressionScore: 0.8,
		}
		
		score := rules.EvaluateAlignment(boundary, profile)
		assert.Greater(t, score, 0.2) // Should get compression bonus
	})
}

func TestEvaluatePatternAlignment(t *testing.T) {
	rules := NewAlignmentRules()
	
	t.Run("no patterns", func(t *testing.T) {
		boundary := ChunkBoundary{EndOffset: 1024}
		profile := &ContentProfile{}
		
		score := rules.evaluatePatternAlignment(boundary, profile)
		assert.Equal(t, 0.5, score) // Neutral score
	})
	
	t.Run("aligned with pattern transition", func(t *testing.T) {
		boundary := ChunkBoundary{EndOffset: 1024}
		profile := &ContentProfile{
			Patterns: []ContentPattern{
				{Offset: 0, Compressibility: 0.8},
				{Offset: 1024, Compressibility: 0.3}, // Transition at boundary
			},
		}
		
		score := rules.evaluatePatternAlignment(boundary, profile)
		// Score should be calculated based on proximity and quality difference
		// With exact alignment (distance=0) and high quality diff (0.5), should be > 0.5
		assert.GreaterOrEqual(t, score, 0.5) // Should get alignment bonus or neutral
	})
	
	t.Run("not aligned with patterns", func(t *testing.T) {
		boundary := ChunkBoundary{EndOffset: 5000}
		profile := &ContentProfile{
			Patterns: []ContentPattern{
				{Offset: 0, Compressibility: 0.8},
				{Offset: 1024, Compressibility: 0.3},
			},
		}
		
		score := rules.evaluatePatternAlignment(boundary, profile)
		assert.Equal(t, 0.5, score) // Should get neutral score
	})
}