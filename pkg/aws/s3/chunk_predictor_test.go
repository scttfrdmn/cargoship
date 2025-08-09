package s3

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunkPredictor(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	assert.NotNil(t, cp)
	assert.Equal(t, ChunkStrategyAdaptive, cp.strategy)
	assert.NotNil(t, cp.contentAnalyzer)
	assert.NotNil(t, cp.performancePredictor)
	assert.NotNil(t, cp.networkPredictor)
	assert.Equal(t, int64(5*1024*1024), cp.minChunkSize)
	assert.Equal(t, int64(100*1024*1024), cp.maxChunkSize)
	assert.Equal(t, int64(16*1024*1024), cp.targetChunkSize)
	assert.Equal(t, 0.15, cp.adaptiveThreshold)
	assert.Equal(t, 0.8, cp.predictionAccuracy)
	assert.Equal(t, 0.1, cp.learningRate)
}

func TestChunkPredictorPredictChunkBoundaries(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyFixed, ctx)

	// Create test data
	testData := make([]byte, 50*1024*1024) // 50MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := bytes.NewReader(testData)
	predictions, err := cp.PredictChunkBoundaries(reader, int64(len(testData)), "binary")

	require.NoError(t, err)
	assert.Greater(t, len(predictions), 0)

	// Verify predictions cover the entire data
	totalSize := int64(0)
	for i, pred := range predictions {
		totalSize += pred.Size
		assert.Greater(t, pred.Size, int64(0))
		assert.LessOrEqual(t, pred.Size, cp.maxChunkSize)

		// Allow the final chunk to be smaller than minimum size (remainder case)
		isLastChunk := i == len(predictions)-1
		if !isLastChunk {
			assert.GreaterOrEqual(t, pred.Size, cp.minChunkSize)
		}

		assert.Equal(t, BoundaryFixed, pred.BoundaryReason)
		assert.Equal(t, 1.0, pred.Confidence)
		assert.NotNil(t, pred.ExpectedPerformance)
	}

	assert.Equal(t, int64(len(testData)), totalSize)
}

func TestChunkPredictorFixedStrategy(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyFixed, ctx)

	testSize := int64(35 * 1024 * 1024) // 35MB
	testData := make([]byte, testSize)
	reader := bytes.NewReader(testData)

	predictions, err := cp.PredictChunkBoundaries(reader, testSize, "text")
	require.NoError(t, err)

	// Should create 3 chunks: 16MB + 16MB + 3MB
	expectedChunks := 3
	assert.Len(t, predictions, expectedChunks)

	// First two chunks should be target size
	assert.Equal(t, cp.targetChunkSize, predictions[0].Size)
	assert.Equal(t, cp.targetChunkSize, predictions[1].Size)

	// Last chunk should be remainder
	expectedLastSize := testSize - (2 * cp.targetChunkSize)
	assert.Equal(t, expectedLastSize, predictions[2].Size)

	// All should have fixed boundary reason
	for _, pred := range predictions {
		assert.Equal(t, BoundaryFixed, pred.BoundaryReason)
		assert.Equal(t, 1.0, pred.Confidence)
	}
}

func TestChunkPredictorAdaptiveStrategy(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	testSize := int64(50 * 1024 * 1024) // 50MB
	testData := make([]byte, testSize)
	// Create compressible data pattern
	for i := range testData {
		testData[i] = byte(i % 10) // Highly repetitive = compressible
	}

	reader := bytes.NewReader(testData)
	predictions, err := cp.PredictChunkBoundaries(reader, testSize, "text")
	require.NoError(t, err)

	assert.Greater(t, len(predictions), 0)

	// Verify adaptive behavior
	for _, pred := range predictions {
		assert.Equal(t, BoundaryAdaptive, pred.BoundaryReason)
		assert.Greater(t, pred.Confidence, 0.0)
		assert.LessOrEqual(t, pred.Confidence, 1.0)
		assert.NotNil(t, pred.ExpectedPerformance)
	}
}

func TestChunkPredictorContentAwareStrategy(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyContentAware, ctx)

	testSize := int64(30 * 1024 * 1024) // 30MB
	testData := make([]byte, testSize)
	// Create data with structural breaks (null byte sequences)
	for i := int64(0); i < testSize; i++ {
		if i%(5*1024*1024) == 0 && i > 0 {
			// Insert 1KB of null bytes every 5MB
			for j := i; j < i+1024 && j < testSize; j++ {
				testData[j] = 0
			}
		} else {
			testData[i] = byte(i % 256)
		}
	}

	reader := bytes.NewReader(testData)
	predictions, err := cp.PredictChunkBoundaries(reader, testSize, "binary")
	require.NoError(t, err)

	assert.Greater(t, len(predictions), 0)

	// Should detect content boundaries
	foundContentBoundary := false
	for _, pred := range predictions {
		if pred.BoundaryReason == BoundaryContentBreak {
			foundContentBoundary = true
		}
		assert.Greater(t, pred.Confidence, 0.8) // Content-aware should be high confidence
	}

	assert.True(t, foundContentBoundary, "Should detect at least one content boundary")
}

func TestChunkPredictorPerformanceStrategy(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyPerformance, ctx)

	testSize := int64(40 * 1024 * 1024) // 40MB
	testData := make([]byte, testSize)
	reader := bytes.NewReader(testData)

	predictions, err := cp.PredictChunkBoundaries(reader, testSize, "video")
	require.NoError(t, err)

	assert.Greater(t, len(predictions), 0)

	// Performance strategy should optimize for performance
	for _, pred := range predictions {
		assert.Equal(t, BoundaryPerformanceOptimal, pred.BoundaryReason)
		assert.Equal(t, 0.95, pred.Confidence)
		assert.True(t, pred.OptimalForNetwork)
		assert.NotNil(t, pred.ExpectedPerformance)
		assert.Greater(t, pred.ExpectedPerformance.NetworkEfficiency, 0.0)
	}
}

func TestChunkPredictorHybridStrategy(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyHybrid, ctx)

	testSize := int64(25 * 1024 * 1024) // 25MB
	testData := make([]byte, testSize)
	reader := bytes.NewReader(testData)

	predictions, err := cp.PredictChunkBoundaries(reader, testSize, "text")
	require.NoError(t, err)

	assert.Greater(t, len(predictions), 0)

	// Hybrid strategy should choose the best approach
	totalSize := int64(0)
	for _, pred := range predictions {
		totalSize += pred.Size
		assert.Greater(t, pred.Confidence, 0.0)
		assert.NotNil(t, pred.ExpectedPerformance)
	}

	assert.Equal(t, testSize, totalSize)
}

func TestChunkPredictorConstrainChunkSize(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyFixed, ctx)

	// Test minimum constraint
	constrained := cp.constrainChunkSize(1024) // Too small
	assert.Equal(t, cp.minChunkSize, constrained)

	// Test maximum constraint
	constrained = cp.constrainChunkSize(200 * 1024 * 1024) // Too large
	assert.Equal(t, cp.maxChunkSize, constrained)

	// Test normal size
	normalSize := int64(20 * 1024 * 1024)
	constrained = cp.constrainChunkSize(normalSize)
	assert.Equal(t, normalSize, constrained)
}

func TestChunkPredictorCalculateOptimalChunkSize(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	analysis := &ContentAnalysis{
		ContentType:     ContentTypeText,
		Compressibility: 0.7,
	}

	networkConditions := &NetworkConditionSummary{
		BandwidthMBps:  100.0, // 100 Mbps
		LatencyMs:      50.0,  // 50ms
		PacketLossRate: 0.001,
		Stability:      0.95,
	}

	optimalSize := cp.calculateOptimalChunkSize(analysis, networkConditions)

	// Should be within bounds
	assert.GreaterOrEqual(t, optimalSize, cp.minChunkSize)
	assert.LessOrEqual(t, optimalSize, cp.maxChunkSize)

	// Text content should result in smaller chunks (multiplier 0.8)
	assert.Greater(t, optimalSize, int64(0)) // Positive size
}

func TestChunkPredictorFindNearestStructuralBreak(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyContentAware, ctx)

	breaks := []int64{1000, 5000, 10000, 15000}
	target := int64(5200)
	maxDistance := int64(500)

	// Should find break at 5000 (distance = 200)
	nearest := cp.findNearestStructuralBreak(target, breaks, maxDistance)
	assert.Equal(t, int64(5000), nearest)

	// Test with target too far from any break
	target = int64(12000)
	maxDistance = int64(100)
	nearest = cp.findNearestStructuralBreak(target, breaks, maxDistance)
	assert.Equal(t, int64(0), nearest) // No break within distance
}

func TestChunkPredictorIsOptimalForNetwork(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	conditions := &NetworkConditionSummary{
		BandwidthMBps: 10.0, // 10 Mbps
		LatencyMs:     100.0,
	}

	// Test optimal chunk size (should transfer in 5-30 seconds)
	optimalChunk := int64(50 * 1024 * 1024) // 50MB, ~5 seconds at 10 Mbps
	assert.True(t, cp.isOptimalForNetwork(optimalChunk, conditions))

	// Test too small chunk (< 5 seconds)
	tooSmall := int64(1 * 1024 * 1024) // 1MB, < 1 second
	assert.False(t, cp.isOptimalForNetwork(tooSmall, conditions))

	// Test too large chunk (> 30 seconds)
	tooLarge := int64(500 * 1024 * 1024) // 500MB, ~50 seconds
	assert.False(t, cp.isOptimalForNetwork(tooLarge, conditions))
}

func TestChunkPredictorCalculateContentBoundaries(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyContentAware, ctx)

	analysis := &ContentAnalysis{
		StructuralBreaks:  []int64{5000, 15000, 25000},
		OptimalChunkSizes: []int64{10000},
	}

	size := int64(30000)
	boundaries := cp.calculateContentBoundaries(size, analysis)

	// Should include start (0), structural breaks, and end
	expected := []int64{0, 5000, 15000, 25000, 30000}
	assert.Equal(t, expected, boundaries)
}

func TestChunkPredictorPredictCompressionForSegment(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	analysis := &ContentAnalysis{
		CompressionRatios: map[string]float64{
			"gzip": 0.3,
		},
	}

	// Test middle segment
	ratio := cp.predictCompressionForSegment(5000, 15000, analysis)
	assert.Equal(t, 0.3, ratio) // Should match base ratio

	// Test first segment (should be slightly worse)
	ratio = cp.predictCompressionForSegment(0, 1000, analysis)
	assert.Equal(t, 0.3*0.95, ratio) // 5% worse

	// Test last segment (should be slightly worse)
	ratio = cp.predictCompressionForSegment(19000, 20000, analysis)
	assert.Equal(t, 0.3*0.95, ratio) // 5% worse
}

func TestChunkPredictorCalculatePerformanceOptimalSize(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyPerformance, ctx)

	analysis := &ContentAnalysis{
		Compressibility: 0.9, // Highly compressible
	}

	networkConditions := &NetworkConditionSummary{
		BandwidthMBps: 50.0,
		LatencyMs:     25.0,
	}

	optimalSize := cp.calculatePerformanceOptimalSize(analysis, networkConditions)

	// Should be larger than network optimal due to high compressibility
	networkOptimal := cp.calculateOptimalChunkSize(analysis, networkConditions)
	assert.Greater(t, optimalSize, networkOptimal)

	// Test low compressibility
	analysis.Compressibility = 0.1
	optimalSize = cp.calculatePerformanceOptimalSize(analysis, networkConditions)
	assert.Less(t, optimalSize, networkOptimal)
}

func TestChunkPredictorScorePredictions(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyHybrid, ctx)

	predictions := []ChunkPrediction{
		{
			PredictedCompressionRatio: 0.3,
			OptimalForNetwork:         true,
			Confidence:                0.9,
		},
		{
			PredictedCompressionRatio: 0.5,
			OptimalForNetwork:         false,
			Confidence:                0.8,
		},
	}

	analysis := &ContentAnalysis{}
	networkConditions := &NetworkConditionSummary{}

	score := cp.scorePredictions(predictions, analysis, networkConditions)

	// Score should be between 0 and 1
	assert.GreaterOrEqual(t, score, 0.0)
	assert.LessOrEqual(t, score, 1.0)

	// Empty predictions should score 0
	emptyScore := cp.scorePredictions([]ChunkPrediction{}, analysis, networkConditions)
	assert.Equal(t, 0.0, emptyScore)
}

func TestChunkPredictorPredictChunkPerformance(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	prediction := &ChunkPrediction{
		Size:                      20 * 1024 * 1024, // 20MB
		PredictedCompressionRatio: 0.3,
	}

	analysis := &ContentAnalysis{
		ContentType: ContentTypeText,
	}

	performance := cp.predictChunkPerformance(prediction, analysis)

	assert.NotNil(t, performance)
	assert.Greater(t, performance.TransferTime, time.Duration(0))
	assert.Greater(t, performance.CompressionTime, time.Duration(0))
	assert.Equal(t, 0.3, performance.CompressionRatio)
	assert.GreaterOrEqual(t, performance.NetworkEfficiency, 0.0)
	assert.LessOrEqual(t, performance.NetworkEfficiency, 1.0)
	assert.Greater(t, performance.MemoryUsage, prediction.Size) // Should include overhead
	assert.Equal(t, 0.2, performance.CPUUsage)
	assert.GreaterOrEqual(t, performance.OptimalConcurrency, 1)
	assert.GreaterOrEqual(t, performance.FailureProbability, 0.0)
	assert.LessOrEqual(t, performance.FailureProbability, 1.0)
}

func TestContentAnalyzer(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test text content
	textData := []byte("This is a sample text content that should be detected as text.")
	textReader := bytes.NewReader(textData)
	analysis, err := analyzer.AnalyzeContent(textReader, int64(len(textData)), "text")

	require.NoError(t, err)
	assert.NotNil(t, analysis)
	assert.Equal(t, ContentTypeText, analysis.ContentType)
	assert.GreaterOrEqual(t, analysis.Entropy, 0.0)
	assert.LessOrEqual(t, analysis.Entropy, 1.0)
	assert.Greater(t, analysis.Compressibility, 0.0)
	assert.NotEmpty(t, analysis.CompressionRatios)
	assert.NotEmpty(t, analysis.OptimalChunkSizes)
	assert.Greater(t, analysis.AnalysisConfidence, 0.0)
	assert.NotZero(t, analysis.ProcessingTime)
}

func TestContentAnalyzerDetectContentType(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test ZIP file detection
	zipData := []byte("PK\x03\x04") // ZIP magic bytes
	contentType := analyzer.detectContentType(zipData, "")
	assert.Equal(t, ContentTypeArchive, contentType)

	// Test JPEG detection
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	contentType = analyzer.detectContentType(jpegData, "")
	assert.Equal(t, ContentTypeImage, contentType)

	// Test text detection
	textData := []byte("Hello, this is plain text content with normal characters.")
	contentType = analyzer.detectContentType(textData, "text")
	assert.Equal(t, ContentTypeText, contentType)
}

func TestContentAnalyzerIsTextContent(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test clear text content
	textData := []byte("This is clearly text content with normal ASCII characters.")
	assert.True(t, analyzer.isTextContent(textData))

	// Test binary content
	binaryData := make([]byte, 1024)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	assert.False(t, analyzer.isTextContent(binaryData))

	// Test mixed content (should be false)
	mixedData := make([]byte, 1024)
	copy(mixedData[:500], []byte(strings.Repeat("text", 125)))
	for i := 500; i < 1024; i++ {
		mixedData[i] = byte(i % 256)
	}
	assert.False(t, analyzer.isTextContent(mixedData))
}

func TestContentAnalyzerEstimateCompressibility(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test different content types
	testCases := []struct {
		contentType   ContentType
		expectedRange [2]float64 // min, max
	}{
		{ContentTypeText, [2]float64{0.6, 0.8}},
		{ContentTypeBinary, [2]float64{0.2, 0.4}},
		{ContentTypeCompressed, [2]float64{0.0, 0.2}},
		{ContentTypeImage, [2]float64{0.1, 0.3}},
		{ContentTypeVideo, [2]float64{0.0, 0.2}},
	}

	for _, tc := range testCases {
		compressibility := analyzer.estimateCompressibility(nil, tc.contentType)
		assert.GreaterOrEqual(t, compressibility, tc.expectedRange[0])
		assert.LessOrEqual(t, compressibility, tc.expectedRange[1])
	}
}

func TestContentAnalyzerPredictCompressionRatios(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test text content compression ratios
	ratios := analyzer.predictCompressionRatios(nil, ContentTypeText)

	assert.Contains(t, ratios, "gzip")
	assert.Contains(t, ratios, "brotli")
	assert.Contains(t, ratios, "zstd")

	// Text should compress well
	assert.Less(t, ratios["gzip"], 0.5)
	assert.Less(t, ratios["brotli"], ratios["gzip"]) // Brotli should be better

	// Test compressed content (should not compress much)
	compressedRatios := analyzer.predictCompressionRatios(nil, ContentTypeCompressed)
	assert.Greater(t, compressedRatios["gzip"], 0.9) // Already compressed
}

func TestContentAnalyzerCalculateOptimalChunkSizes(t *testing.T) {
	analyzer := NewContentAnalyzer()

	// Test different content types
	textSizes := analyzer.calculateOptimalChunkSizes(ContentTypeText, 0.5, 0.7)
	assert.NotEmpty(t, textSizes)
	assert.Contains(t, textSizes, int64(8*1024*1024))  // 8MB
	assert.Contains(t, textSizes, int64(16*1024*1024)) // 16MB

	videoSizes := analyzer.calculateOptimalChunkSizes(ContentTypeVideo, 0.8, 0.1)
	assert.NotEmpty(t, videoSizes)
	// Video should prefer larger chunks
	for _, size := range videoSizes {
		assert.GreaterOrEqual(t, size, int64(32*1024*1024)) // 32MB+
	}
}

func TestEntropyCalculator(t *testing.T) {
	calc := NewEntropyCalculator()

	// Test uniform data (high entropy)
	uniformData := make([]byte, 256)
	for i := range uniformData {
		uniformData[i] = byte(i)
	}
	uniformEntropy := calc.Calculate(uniformData)
	assert.Greater(t, uniformEntropy, 0.9) // Should be close to 1.0

	// Test repetitive data (low entropy)
	repetitiveData := bytes.Repeat([]byte{0x42}, 1000)
	repetitiveEntropy := calc.Calculate(repetitiveData)
	assert.Less(t, repetitiveEntropy, 0.1) // Should be close to 0.0

	// Test empty data
	emptyEntropy := calc.Calculate([]byte{})
	assert.Equal(t, 0.0, emptyEntropy)
}

func TestContentPatternDetector(t *testing.T) {
	detector := NewContentPatternDetector()

	// Create data with null byte patterns
	data := make([]byte, 2048)
	// Fill first 1024 bytes with data
	for i := 0; i < 1024; i++ {
		data[i] = byte(i % 256)
	}
	// Fill next 1024 bytes with mostly nulls (structural break)
	for i := 1024; i < 2048; i++ {
		if i%10 == 0 {
			data[i] = byte(i % 256)
		} else {
			data[i] = 0 // Null bytes
		}
	}

	patterns := detector.DetectPatterns(data)

	// Should detect at least one structural break
	assert.NotEmpty(t, patterns)
	foundStructuralBreak := false
	for _, pattern := range patterns {
		if pattern.Type == "structural_break" {
			foundStructuralBreak = true
			assert.GreaterOrEqual(t, pattern.Offset, int64(1024))
		}
	}
	assert.True(t, foundStructuralBreak)
}

func TestNetworkConditionPredictor(t *testing.T) {
	predictor := NewNetworkConditionPredictor()

	conditions := predictor.GetCurrentConditions()

	assert.NotNil(t, conditions)
	assert.Greater(t, conditions.BandwidthMBps, 0.0)
	assert.Greater(t, conditions.LatencyMs, 0.0)
	assert.GreaterOrEqual(t, conditions.PacketLossRate, 0.0)
	assert.LessOrEqual(t, conditions.PacketLossRate, 1.0)
	assert.GreaterOrEqual(t, conditions.Jitter, 0.0)
	assert.GreaterOrEqual(t, conditions.Stability, 0.0)
	assert.LessOrEqual(t, conditions.Stability, 1.0)
}

func TestChunkPredictorConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	// Test concurrent access
	done := make(chan bool, 2)

	// Goroutine 1: Predict chunks
	go func() {
		for i := 0; i < 5; i++ {
			testData := make([]byte, 10*1024*1024) // 10MB
			reader := bytes.NewReader(testData)
			_, err := cp.PredictChunkBoundaries(reader, int64(len(testData)), "binary")
			assert.NoError(t, err)
		}
		done <- true
	}()

	// Goroutine 2: Calculate optimal sizes
	go func() {
		for i := 0; i < 5; i++ {
			analysis := &ContentAnalysis{
				ContentType:     ContentTypeText,
				Compressibility: 0.5,
			}
			networkConditions := &NetworkConditionSummary{
				BandwidthMBps: 100.0,
				LatencyMs:     50.0,
			}
			size := cp.calculateOptimalChunkSize(analysis, networkConditions)
			assert.Greater(t, size, int64(0))
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}

func TestChunkPredictorEdgeCases(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyFixed, ctx)

	// Test very small file
	smallData := []byte("small")
	reader := bytes.NewReader(smallData)
	predictions, err := cp.PredictChunkBoundaries(reader, int64(len(smallData)), "text")
	require.NoError(t, err)
	assert.Len(t, predictions, 1)
	assert.Equal(t, int64(len(smallData)), predictions[0].Size)

	// Test file exactly at chunk boundary
	exactData := make([]byte, cp.targetChunkSize)
	reader = bytes.NewReader(exactData)
	predictions, err = cp.PredictChunkBoundaries(reader, int64(len(exactData)), "binary")
	require.NoError(t, err)
	assert.Len(t, predictions, 1)
	assert.Equal(t, cp.targetChunkSize, predictions[0].Size)
}

func TestChunkPredictorPerformanceMetrics(t *testing.T) {
	ctx := context.Background()
	cp := NewChunkPredictor(ChunkStrategyAdaptive, ctx)

	// Create a large prediction to test performance
	largeData := make([]byte, 100*1024*1024) // 100MB
	reader := bytes.NewReader(largeData)

	start := time.Now()
	predictions, err := cp.PredictChunkBoundaries(reader, int64(len(largeData)), "binary")
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Greater(t, len(predictions), 0)

	// Prediction should complete in reasonable time (< 1 second for 100MB)
	assert.Less(t, duration, time.Second)

	// Verify all predictions have performance expectations
	for _, pred := range predictions {
		assert.NotNil(t, pred.ExpectedPerformance)
		assert.Greater(t, pred.ExpectedPerformance.TransferTime, time.Duration(0))
		assert.Greater(t, pred.ExpectedPerformance.MemoryUsage, int64(0))
	}
}
