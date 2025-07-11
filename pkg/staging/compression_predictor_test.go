package staging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCompressionRatioPredictor(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	assert.NotNil(t, predictor)
	assert.NotNil(t, predictor.compressionStats)
	assert.NotNil(t, predictor.contentPatterns)
	assert.NotNil(t, predictor.historicalData)
}

func TestCompressionRatioPredictor_UpdateStats(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	stats := &CompressionStats{
		Algorithm:         "zstd",
		AverageRatio:      0.6,
		AverageSpeed:      50.0,
		TotalCompressed:   1000,
		TotalUncompressed: 2000,
		CompressionCount:  10,
		LastUpdated:       time.Now(),
	}
	
	predictor.UpdateStats("zstd", stats)
	
	retrievedStats := predictor.GetStats("zstd")
	assert.NotNil(t, retrievedStats)
	assert.Equal(t, "zstd", retrievedStats.Algorithm)
	assert.Equal(t, 0.6, retrievedStats.AverageRatio)
}

func TestCompressionRatioPredictor_GetStats(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	// Test getting non-existent stats
	stats := predictor.GetStats("nonexistent")
	assert.Nil(t, stats)
	
	// Add stats and retrieve
	testStats := &CompressionStats{
		Algorithm:    "gzip",
		AverageRatio: 0.7,
	}
	predictor.UpdateStats("gzip", testStats)
	
	retrievedStats := predictor.GetStats("gzip")
	assert.NotNil(t, retrievedStats)
	assert.Equal(t, "gzip", retrievedStats.Algorithm)
}

func TestCompressionRatioPredictor_LearnFromResult(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	// This should not panic and should update the historical data
	predictor.LearnFromResult("text", 1024, 0.7)
	
	// Verify the historical data was added by checking if we can get an average
	avgRatio := predictor.getHistoricalRatio("text", 1024)
	assert.GreaterOrEqual(t, avgRatio, 0.0)
}

func TestCompressionRatioPredictor_PredictBestAlgorithm(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	profile := &ContentProfile{
		ContentType: "text",
		Entropy:     2.0,
		Patterns: []ContentPattern{
			{
				Type:           PatternRepetitive,
				Compressibility: 0.9,
			},
		},
	}
	
	testCases := []struct {
		name              string
		networkCondition  *NetworkCondition
		expectedAlgorithm string
	}{
		{
			name: "high bandwidth network",
			networkCondition: &NetworkCondition{
				BandwidthMBps: 150.0,
			},
			expectedAlgorithm: "zstd-fast",
		},
		{
			name: "low bandwidth network",
			networkCondition: &NetworkCondition{
				BandwidthMBps: 5.0,
			},
			expectedAlgorithm: "zstd-max",
		},
		{
			name: "medium bandwidth with repetitive content",
			networkCondition: &NetworkCondition{
				BandwidthMBps: 50.0,
			},
			expectedAlgorithm: "zstd-high",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			algorithm := predictor.PredictBestAlgorithm(profile, tc.networkCondition)
			assert.Equal(t, tc.expectedAlgorithm, algorithm)
		})
	}
}

func TestCompressionStats_UpdateWithResult(t *testing.T) {
	stats := &CompressionStats{
		Algorithm:        "zstd",
		AverageRatio:     0.5,
		AverageSpeed:     30.0,
		CompressionCount: 1,
	}
	
	// Update with new result
	uncompressedSize := int64(2048)
	compressedSize := int64(1024)
	compressionTime := time.Millisecond * 100
	
	stats.UpdateWithResult(uncompressedSize, compressedSize, compressionTime)
	
	expectedRatio := float64(compressedSize) / float64(uncompressedSize) // 0.5
	expectedSpeed := float64(uncompressedSize) / (1024 * 1024) / compressionTime.Seconds()
	
	// Should average with previous values
	expectedAvgRatio := (0.5 + expectedRatio) / 2
	expectedAvgSpeed := (30.0 + expectedSpeed) / 2
	
	assert.Equal(t, 2, stats.CompressionCount)
	assert.InDelta(t, expectedAvgRatio, stats.AverageRatio, 0.01)
	assert.InDelta(t, expectedAvgSpeed, stats.AverageSpeed, 0.01)
	assert.Equal(t, compressedSize, stats.TotalCompressed)
	assert.Equal(t, uncompressedSize, stats.TotalUncompressed)
}

func TestNewCompressionHistory(t *testing.T) {
	history := NewCompressionHistory()
	
	assert.NotNil(t, history)
	assert.NotNil(t, history.results)
	assert.Equal(t, 1000, history.maxResults)
}

func TestCompressionHistory_AddResult(t *testing.T) {
	history := NewCompressionHistory()
	
	history.AddResult("text", 1024, 0.7)
	history.AddResult("text", 2048, 0.6)
	history.AddResult("binary", 1024, 0.3)
	
	// Verify results were added
	assert.Contains(t, history.results, "text")
	assert.Contains(t, history.results, "binary")
	assert.Len(t, history.results["text"], 2)
	assert.Len(t, history.results["binary"], 1)
}

func TestCompressionHistory_GetResultsForContentType(t *testing.T) {
	history := NewCompressionHistory()
	
	history.AddResult("text", 1024, 0.7)
	history.AddResult("text", 2048, 0.6)
	history.AddResult("binary", 1024, 0.3)
	
	textResults := history.GetResultsForContentType("text")
	assert.Len(t, textResults, 2)
	
	binaryResults := history.GetResultsForContentType("binary")
	assert.Len(t, binaryResults, 1)
	
	nonExistentResults := history.GetResultsForContentType("nonexistent")
	assert.Len(t, nonExistentResults, 0)
}

func TestCompressionRatioPredictor_PredictCompressionTime(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	// Add some stats for a known algorithm
	stats := &CompressionStats{
		Algorithm:    "zstd",
		AverageSpeed: 50.0, // 50 MB/s
	}
	predictor.UpdateStats("zstd", stats)
	
	size := int64(10 * 1024 * 1024) // 10MB
	duration := predictor.PredictCompressionTime(size, "zstd")
	
	// Should predict time based on size and speed
	assert.Greater(t, duration, time.Duration(0))
	
	// Test with unknown algorithm - should return default
	unknownDuration := predictor.PredictCompressionTime(size, "unknown")
	assert.Greater(t, unknownDuration, time.Duration(0))
}

func TestCompressionRatioPredictor_EstimateCompressionBenefit(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	profile := &ContentProfile{
		ContentType: "text",
		Entropy:     2.0,
	}
	
	networkCondition := &NetworkCondition{
		BandwidthMBps: 50.0,
	}
	
	benefit := predictor.EstimateCompressionBenefit(profile, networkCondition, "zstd")
	
	// Should return a benefit estimate
	assert.NotNil(t, benefit)
	assert.Equal(t, "zstd", benefit.Algorithm)
	assert.GreaterOrEqual(t, benefit.PredictedRatio, 0.0)
	assert.LessOrEqual(t, benefit.PredictedRatio, 1.0)
}

func TestAbs64(t *testing.T) {
	testCases := []struct {
		input    int64
		expected int64
	}{
		{42, 42},
		{-42, 42},
		{0, 0},
		{-1, 1},
		{1, 1},
	}
	
	for _, tc := range testCases {
		result := abs64(tc.input)
		assert.Equal(t, tc.expected, result)
	}
}

func TestCompressionRatioPredictor_PredictFromContentType(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	testCases := []struct {
		contentType    string
		expectedRatio  float64
		description    string
	}{
		{"text", 0.7, "text should compress very well"},
		{"json", 0.6, "json should compress well"},
		{"xml", 0.6, "xml should compress well"},
		{"binary", 0.4, "binary should compress moderately"},
		{"image_jpeg", 0.05, "jpeg should compress poorly"},
		{"image_png", 0.05, "png should compress poorly"},
		{"pdf", 0.3, "pdf should compress somewhat"},
		{"zip", 0.02, "zip should compress very poorly"},
		{"compressed", 0.02, "compressed data should compress very poorly"},
		{"unknown", 0.3, "unknown should get default ratio"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ratio := predictor.predictFromContentType(tc.contentType)
			assert.Equal(t, tc.expectedRatio, ratio)
		})
	}
}

func TestCompressionRatioPredictor_PredictFromEntropy(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	testCases := []struct {
		entropy       float64
		expectedRatio float64
		description   string
	}{
		{0.5, 0.9, "very low entropy should predict excellent compression"},
		{1.5, 0.8, "low entropy should predict very good compression"},
		{3.0, 0.6, "medium entropy should predict good compression"},
		{5.0, 0.4, "high entropy should predict moderate compression"},
		{6.5, 0.2, "very high entropy should predict poor compression"},
		{8.0, 0.05, "near-random entropy should predict minimal compression"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ratio := predictor.predictFromEntropy(tc.entropy)
			assert.Equal(t, tc.expectedRatio, ratio)
		})
	}
}

func TestCompressionRatioPredictor_PredictRatio(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	boundary := ChunkBoundary{
		Size: 1024 * 1024, // 1MB
	}
	
	profile := &ContentProfile{
		ContentType: "text",
		Entropy:     2.0,
		Patterns: []ContentPattern{
			{
				Type:           PatternRepetitive,
				Compressibility: 0.8,
			},
		},
	}
	
	// Test without historical data
	ratio := predictor.PredictRatio(boundary, profile)
	assert.GreaterOrEqual(t, ratio, 0.05)
	assert.LessOrEqual(t, ratio, 0.95)
	
	// Add historical data and test again
	predictor.LearnFromResult("text", 1024*1024, 0.8)
	ratioWithHistory := predictor.PredictRatio(boundary, profile)
	assert.GreaterOrEqual(t, ratioWithHistory, 0.05)
	assert.LessOrEqual(t, ratioWithHistory, 0.95)
}

func TestCompressionRatioPredictor_PredictFromPatterns(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewCompressionRatioPredictor(config)
	
	// Test with various pattern types
	patterns := []ContentPattern{
		{
			Type:           PatternRepetitive,
			Compressibility: 0.9,
			Frequency:      0.8,
		},
		{
			Type:           PatternStructured,
			Compressibility: 0.6,
			Frequency:      0.5,
		},
	}
	
	ratio := predictor.predictFromPatterns(patterns)
	
	// Should return a weighted average based on compressibility and frequency
	assert.GreaterOrEqual(t, ratio, 0.0)
	assert.LessOrEqual(t, ratio, 1.0)
}