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

func TestNewStreamingCompressor(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	assert.NotNil(t, sc)
	assert.Equal(t, CompressionGzip, sc.algorithm)
	assert.Equal(t, CompressionBalanced, sc.level)
	assert.False(t, sc.adaptiveSelection)
	assert.Equal(t, StrategyBalanced, sc.compressionStrategy)
	assert.Equal(t, 64*1024, sc.bufferSize)
	assert.Equal(t, 4, sc.maxConcurrency)
	assert.Equal(t, int64(1024*1024), sc.chunkThreshold)
	assert.NotNil(t, sc.compressionMetrics)
	assert.Len(t, sc.algorithmPerformance, 4) // none, gzip, brotli, zstd
	assert.True(t, sc.adaptationEnabled)
	assert.Equal(t, 0.1, sc.learningRate)
}

func TestStreamingCompressorCompressStream(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Create test data
	testData := strings.Repeat("Hello, World! This is test data. ", 1000)
	input := strings.NewReader(testData)
	output := &bytes.Buffer{}
	
	result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, CompressionGzip, result.Algorithm)
	assert.Equal(t, CompressionBalanced, result.Level)
	assert.Equal(t, int64(len(testData)), result.OriginalSize)
	assert.Greater(t, result.CompressedSize, int64(0))
	assert.Less(t, result.CompressedSize, result.OriginalSize)
	assert.Greater(t, result.CompressionRatio, 0.0)
	assert.Less(t, result.CompressionRatio, 1.0)
	assert.Greater(t, result.CompressionTime, time.Duration(0))
	assert.Greater(t, result.ThroughputMBps, 0.0)
	assert.True(t, result.Success)
	assert.NotZero(t, result.Timestamp)
}

func TestStreamingCompressorCompressChunk(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionBrotli, CompressionFast, ctx)
	
	// Create test chunk
	testChunk := make([]byte, 1024*1024) // 1MB
	for i := range testChunk {
		testChunk[i] = byte(i % 256)
	}
	
	result, err := sc.CompressChunk(testChunk, ContentTypeBinary)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, CompressionBrotli, result.Algorithm)
	assert.Equal(t, CompressionFast, result.Level)
	assert.Equal(t, int64(len(testChunk)), result.OriginalSize)
	assert.Greater(t, result.CompressedSize, int64(0))
	assert.True(t, result.Success)
}

func TestStreamingCompressorAdaptiveSelection(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionAuto, CompressionAutoLevel, ctx)
	
	assert.True(t, sc.adaptiveSelection)
	
	// Test with text content (should select good compression)
	textData := strings.Repeat("This is highly compressible text content. ", 1000)
	input := strings.NewReader(textData)
	output := &bytes.Buffer{}
	
	result, err := sc.CompressStream(input, output, int64(len(textData)), ContentTypeText)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	// Should select an algorithm that compresses well for text
	assert.Contains(t, []CompressionAlgorithm{CompressionBrotli, CompressionZstd, CompressionGzip}, result.Algorithm)
}

func TestStreamingCompressorDifferentAlgorithms(t *testing.T) {
	ctx := context.Background()
	testData := strings.Repeat("Test data for compression algorithms. ", 500)
	
	algorithms := []CompressionAlgorithm{
		CompressionNone,
		CompressionGzip,
		CompressionBrotli,
		CompressionZstd,
	}
	
	for _, alg := range algorithms {
		t.Run(string(alg), func(t *testing.T) {
			sc := NewStreamingCompressor(alg, CompressionBalanced, ctx)
			input := strings.NewReader(testData)
			output := &bytes.Buffer{}
			
			result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
			
			require.NoError(t, err)
			assert.Equal(t, alg, result.Algorithm)
			assert.Equal(t, int64(len(testData)), result.OriginalSize)
			assert.True(t, result.Success)
			
			if alg == CompressionNone {
				assert.Equal(t, result.OriginalSize, result.CompressedSize)
				assert.Equal(t, 1.0, result.CompressionRatio)
			} else {
				assert.Less(t, result.CompressedSize, result.OriginalSize)
				assert.Less(t, result.CompressionRatio, 1.0)
			}
		})
	}
}

func TestStreamingCompressorCompressionLevels(t *testing.T) {
	ctx := context.Background()
	testData := strings.Repeat("Compression level testing data. ", 1000)
	
	levels := []CompressionLevel{
		CompressionFast,
		CompressionBalanced,
		CompressionBest,
	}
	
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			sc := NewStreamingCompressor(CompressionGzip, level, ctx)
			input := strings.NewReader(testData)
			output := &bytes.Buffer{}
			
			result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
			
			require.NoError(t, err)
			assert.Equal(t, level, result.Level)
			assert.True(t, result.Success)
		})
	}
}

func TestStreamingCompressorCreateCompressionJob(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	testData := "Test data for job creation"
	input := strings.NewReader(testData)
	output := &bytes.Buffer{}
	
	job := sc.CreateCompressionJob(input, output, int64(len(testData)), ContentTypeText, CompressionPriorityNormal)
	
	assert.NotNil(t, job)
	assert.NotEmpty(t, job.ID)
	assert.Equal(t, input, job.Input)
	assert.Equal(t, output, job.Output)
	assert.Equal(t, CompressionGzip, job.Algorithm)
	assert.Equal(t, CompressionBalanced, job.Level)
	assert.Equal(t, int64(len(testData)), job.ExpectedSize)
	assert.Equal(t, ContentTypeText, job.ContentType)
	assert.Equal(t, CompressionPriorityNormal, job.Priority)
	assert.NotNil(t, job.Context)
	assert.NotNil(t, job.Cancel)
	assert.NotZero(t, job.StartTime)
}

func TestStreamingCompressorProcessJobAsync(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	testData := strings.Repeat("Async job test data. ", 100)
	input := strings.NewReader(testData)
	output := &bytes.Buffer{}
	
	job := sc.CreateCompressionJob(input, output, int64(len(testData)), ContentTypeText, CompressionPriorityHigh)
	
	// Set up completion callback
	completed := make(chan *CompressionResult, 1)
	job.CompletionCallback = func(result *CompressionResult) {
		completed <- result
	}
	
	// Process job asynchronously
	sc.ProcessJobAsync(job)
	
	// Wait for completion
	select {
	case result := <-completed:
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, int64(len(testData)), result.OriginalSize)
		assert.NotZero(t, job.EndTime)
	case <-time.After(time.Second * 5):
		t.Fatal("Job did not complete within timeout")
	}
}

func TestStreamingCompressorSelectOptimalAlgorithm(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Add some performance data
	sc.algorithmPerformance[CompressionGzip].AverageRatio = 0.6
	sc.algorithmPerformance[CompressionGzip].AverageThroughput = 80.0
	sc.algorithmPerformance[CompressionBrotli].AverageRatio = 0.4
	sc.algorithmPerformance[CompressionBrotli].AverageThroughput = 60.0
	sc.algorithmPerformance[CompressionZstd].AverageRatio = 0.5
	sc.algorithmPerformance[CompressionZstd].AverageThroughput = 70.0
	
	// Test with high bandwidth (should prefer speed)
	highBandwidth := &NetworkConditionSummary{
		BandwidthMBps: 100.0,
		LatencyMs:     10.0,
	}
	
	algorithm := sc.SelectOptimalAlgorithm(ContentTypeText, 1024*1024, highBandwidth)
	assert.Contains(t, []CompressionAlgorithm{CompressionGzip, CompressionBrotli, CompressionZstd}, algorithm)
	
	// Test with low bandwidth (should prefer compression)
	lowBandwidth := &NetworkConditionSummary{
		BandwidthMBps: 5.0,
		LatencyMs:     100.0,
	}
	
	algorithm = sc.SelectOptimalAlgorithm(ContentTypeText, 1024*1024, lowBandwidth)
	assert.Contains(t, []CompressionAlgorithm{CompressionGzip, CompressionBrotli, CompressionZstd}, algorithm)
}

func TestStreamingCompressorGetCompressionMetrics(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Perform some compressions to generate metrics
	for i := 0; i < 3; i++ {
		testData := strings.Repeat("Metrics test data. ", 100)
		input := strings.NewReader(testData)
		output := &bytes.Buffer{}
		
		result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
		require.NoError(t, err)
		assert.True(t, result.Success)
	}
	
	metrics := sc.GetCompressionMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(3), metrics.TotalOperations)
	assert.Greater(t, metrics.TotalBytesProcessed, int64(0))
	assert.Greater(t, metrics.TotalCompressionTime, time.Duration(0))
	assert.Greater(t, metrics.AverageRatio, 0.0)
	assert.Less(t, metrics.AverageRatio, 1.0)
	assert.Greater(t, metrics.AverageThroughput, 0.0)
	assert.Contains(t, metrics.AlgorithmUsage, CompressionGzip)
	assert.Equal(t, int64(3), metrics.AlgorithmUsage[CompressionGzip])
	assert.Equal(t, 1.0, metrics.SuccessRate)
	assert.NotZero(t, metrics.LastUpdate)
}

func TestStreamingCompressorAdaptCompressionStrategy(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	sc.compressionStrategy = StrategyAdaptive
	
	// Add performance history to trigger adaptation
	for i := 0; i < 15; i++ {
		snapshot := CompressionPerformanceSnapshot{
			Timestamp:      time.Now(),
			Algorithm:      CompressionGzip,
			InputSize:      1000,
			OutputSize:     600, // 60% compression ratio
			ThroughputMBps: 30.0, // Low throughput
		}
		sc.performanceHistory = append(sc.performanceHistory, snapshot)
	}
	
	originalAlgorithm := sc.algorithm
	sc.AdaptCompressionStrategy()
	
	// Should adapt based on low throughput
	// Might switch to different algorithm or stay the same based on conditions
	assert.Contains(t, []CompressionAlgorithm{CompressionGzip, CompressionBrotli, CompressionZstd}, sc.algorithm)
	
	// Test network-optimized strategy
	sc.compressionStrategy = StrategyNetworkOptimized
	sc.algorithm = originalAlgorithm
	sc.AdaptCompressionStrategy()
	
	assert.Contains(t, []CompressionAlgorithm{CompressionGzip, CompressionBrotli, CompressionZstd}, sc.algorithm)
}

func TestStreamingCompressorCalculateAlgorithmScore(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	performance := &AlgorithmPerformance{
		Algorithm:         CompressionGzip,
		AverageRatio:      0.6,
		AverageThroughput: 70.0,
		PerformanceByContent: map[ContentType]*ContentPerformance{
			ContentTypeText: {
				ContentType:       ContentTypeText,
				AverageRatio:      0.4,
				AverageThroughput: 80.0,
				Confidence:        0.9,
			},
		},
	}
	
	networkConditions := &NetworkConditionSummary{
		BandwidthMBps: 50.0,
		LatencyMs:     25.0,
	}
	
	score := sc.calculateAlgorithmScore(CompressionGzip, performance, ContentTypeText, 1024*1024, networkConditions)
	
	assert.GreaterOrEqual(t, score, 0.0)
	assert.LessOrEqual(t, score, 1.0)
}

func TestStreamingCompressorEstimateCPUUsage(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Test different algorithms
	gzipUsage := sc.estimateCPUUsage(CompressionGzip, CompressionBalanced)
	brotliUsage := sc.estimateCPUUsage(CompressionBrotli, CompressionBalanced)
	zstdUsage := sc.estimateCPUUsage(CompressionZstd, CompressionBalanced)
	noneUsage := sc.estimateCPUUsage(CompressionNone, CompressionBalanced)
	
	assert.Greater(t, gzipUsage, noneUsage)
	assert.Greater(t, brotliUsage, gzipUsage)
	assert.Greater(t, zstdUsage, gzipUsage)
	assert.Less(t, zstdUsage, brotliUsage)
	
	// Test different levels
	fastUsage := sc.estimateCPUUsage(CompressionGzip, CompressionFast)
	bestUsage := sc.estimateCPUUsage(CompressionGzip, CompressionBest)
	
	assert.Less(t, fastUsage, gzipUsage)
	assert.Greater(t, bestUsage, gzipUsage)
}

func TestStreamingCompressorEstimateMemoryUsage(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	inputSize := int64(10 * 1024 * 1024) // 10MB
	
	gzipMemory := sc.estimateMemoryUsage(inputSize, CompressionGzip)
	brotliMemory := sc.estimateMemoryUsage(inputSize, CompressionBrotli)
	zstdMemory := sc.estimateMemoryUsage(inputSize, CompressionZstd)
	noneMemory := sc.estimateMemoryUsage(inputSize, CompressionNone)
	
	assert.Greater(t, gzipMemory, noneMemory)
	assert.Greater(t, brotliMemory, gzipMemory)
	assert.Greater(t, zstdMemory, gzipMemory)
	assert.Less(t, zstdMemory, brotliMemory)
	
	// Test minimum memory usage
	smallMemory := sc.estimateMemoryUsage(1024, CompressionGzip)
	assert.Equal(t, int64(64*1024), smallMemory) // Should be minimum 64KB
}

func TestStreamingCompressorAssessCompressionQuality(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Test excellent quality
	excellent := sc.assessCompressionQuality(0.2, 100.0)
	assert.Equal(t, QualityExcellent, excellent)
	
	// Test good quality
	good := sc.assessCompressionQuality(0.4, 60.0)
	assert.Equal(t, QualityGood, good)
	
	// Test fair quality
	fair := sc.assessCompressionQuality(0.7, 30.0)
	assert.Equal(t, QualityFair, fair)
	
	// Test poor quality
	poor := sc.assessCompressionQuality(0.9, 10.0)
	assert.Equal(t, QualityPoor, poor)
}

func TestStreamingCompressorUpdatePerformanceMetrics(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	result := &CompressionResult{
		Algorithm:        CompressionGzip,
		Level:           CompressionBalanced,
		OriginalSize:    1000,
		CompressedSize:  600,
		CompressionRatio: 0.6,
		CompressionTime: time.Millisecond * 100,
		ThroughputMBps:  50.0,
		Success:         true,
	}
	
	// Update metrics
	sc.updatePerformanceMetrics(result, ContentTypeText)
	
	// Check overall metrics
	metrics := sc.compressionMetrics
	assert.Equal(t, int64(1), metrics.TotalOperations)
	assert.Equal(t, int64(1000), metrics.TotalBytesProcessed)
	assert.Equal(t, time.Millisecond*100, metrics.TotalCompressionTime)
	assert.Equal(t, 0.6, metrics.AverageRatio)
	assert.Equal(t, 50.0, metrics.AverageThroughput)
	assert.Equal(t, int64(1), metrics.AlgorithmUsage[CompressionGzip])
	assert.Equal(t, 1.0, metrics.SuccessRate)
	
	// Check algorithm-specific metrics
	algPerf := sc.algorithmPerformance[CompressionGzip]
	assert.Equal(t, int64(1), algPerf.TotalOperations)
	assert.Equal(t, 0.6, algPerf.AverageRatio)
	assert.Equal(t, 50.0, algPerf.AverageThroughput)
	assert.Equal(t, time.Millisecond*100, algPerf.AverageCompressionTime)
	assert.Len(t, algPerf.RecentRatios, 1)
	assert.Len(t, algPerf.RecentThroughputs, 1)
}

func TestStreamingCompressorRecordPerformanceSnapshot(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	result := &CompressionResult{
		Algorithm:        CompressionBrotli,
		Level:           CompressionFast,
		OriginalSize:    2000,
		CompressedSize:  800,
		CompressionTime: time.Millisecond * 200,
		ThroughputMBps:  75.0,
		CPUUsage:        0.3,
		MemoryUsage:     1024 * 1024,
	}
	
	initialCount := len(sc.performanceHistory)
	sc.recordPerformanceSnapshot(result, ContentTypeBinary)
	
	assert.Len(t, sc.performanceHistory, initialCount+1)
	
	snapshot := sc.performanceHistory[len(sc.performanceHistory)-1]
	assert.Equal(t, CompressionBrotli, snapshot.Algorithm)
	assert.Equal(t, CompressionFast, snapshot.Level)
	assert.Equal(t, ContentTypeBinary, snapshot.ContentType)
	assert.Equal(t, int64(2000), snapshot.InputSize)
	assert.Equal(t, int64(800), snapshot.OutputSize)
	assert.Equal(t, time.Millisecond*200, snapshot.CompressionTime)
	assert.Equal(t, 75.0, snapshot.ThroughputMBps)
	assert.Equal(t, 0.3, snapshot.CPUUsage)
	assert.Equal(t, int64(1024*1024), snapshot.MemoryUsage)
	assert.NotNil(t, snapshot.NetworkConditions)
}

func TestCompressionContentAnalyzer(t *testing.T) {
	analyzer := NewCompressionContentAnalyzer()
	
	testData := strings.Repeat("This is test data for compression analysis. ", 100)
	input := strings.NewReader(testData)
	
	analysis, err := analyzer.AnalyzeForCompression(input, int64(len(testData)), ContentTypeText)
	
	require.NoError(t, err)
	assert.NotNil(t, analysis)
	assert.Equal(t, ContentTypeText, analysis.ContentType)
	assert.GreaterOrEqual(t, analysis.Entropy, 0.0)
	assert.LessOrEqual(t, analysis.Entropy, 1.0)
	assert.GreaterOrEqual(t, analysis.RedundancyLevel, 0.0)
	assert.LessOrEqual(t, analysis.RedundancyLevel, 1.0)
	assert.NotEmpty(t, analysis.PredictedRatios)
	assert.Contains(t, analysis.PredictedRatios, CompressionGzip)
	assert.Contains(t, analysis.PredictedRatios, CompressionBrotli)
	assert.Contains(t, analysis.PredictedRatios, CompressionZstd)
	assert.Equal(t, CompressionBrotli, analysis.RecommendedAlgorithm) // Text should recommend Brotli
	assert.Greater(t, analysis.Confidence, 0.0)
	assert.Greater(t, analysis.AnalysisTime, time.Duration(0))
}

func TestCompressionContentAnalyzerDifferentContentTypes(t *testing.T) {
	analyzer := NewCompressionContentAnalyzer()
	
	testCases := []struct {
		contentType         ContentType
		expectedAlgorithm   CompressionAlgorithm
		expectedRatioRange  [2]float64 // min, max for gzip
	}{
		{ContentTypeText, CompressionBrotli, [2]float64{0.2, 0.4}},
		{ContentTypeBinary, CompressionZstd, [2]float64{0.6, 0.8}},
		{ContentTypeCompressed, CompressionNone, [2]float64{0.95, 1.0}},
	}
	
	for _, tc := range testCases {
		t.Run(string(tc.contentType), func(t *testing.T) {
			input := strings.NewReader("test data")
			analysis, err := analyzer.AnalyzeForCompression(input, 9, tc.contentType)
			
			require.NoError(t, err)
			assert.Equal(t, tc.contentType, analysis.ContentType)
			assert.Equal(t, tc.expectedAlgorithm, analysis.RecommendedAlgorithm)
			
			gzipRatio := analysis.PredictedRatios[CompressionGzip]
			assert.GreaterOrEqual(t, gzipRatio, tc.expectedRatioRange[0])
			assert.LessOrEqual(t, gzipRatio, tc.expectedRatioRange[1])
		})
	}
}

func TestStreamingCompressorConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Test concurrent compression operations
	numOperations := 5
	results := make(chan *CompressionResult, numOperations)
	
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			testData := strings.Repeat("Concurrent test data. ", 100)
			input := strings.NewReader(testData)
			output := &bytes.Buffer{}
			
			result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
			if err != nil {
				t.Errorf("Compression failed for operation %d: %v", id, err)
				return
			}
			
			results <- result
		}(i)
	}
	
	// Collect results
	for i := 0; i < numOperations; i++ {
		select {
		case result := <-results:
			assert.NotNil(t, result)
			assert.True(t, result.Success)
		case <-time.After(time.Second * 5):
			t.Fatal("Concurrent operation timed out")
		}
	}
	
	// Verify metrics reflect all operations
	metrics := sc.GetCompressionMetrics()
	assert.Equal(t, int64(numOperations), metrics.TotalOperations)
}

func TestStreamingCompressorJobCancellation(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	testData := strings.Repeat("Cancellation test data. ", 1000)
	input := strings.NewReader(testData)
	output := &bytes.Buffer{}
	
	job := sc.CreateCompressionJob(input, output, int64(len(testData)), ContentTypeText, CompressionPriorityNormal)
	
	// Cancel the job immediately
	job.Cancel()
	
	// Verify the context is cancelled
	select {
	case <-job.Context.Done():
		assert.Equal(t, context.Canceled, job.Context.Err())
	default:
		t.Fatal("Job context should be cancelled")
	}
}

func TestStreamingCompressorEdgeCases(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Test empty data
	emptyInput := strings.NewReader("")
	output := &bytes.Buffer{}
	
	result, err := sc.CompressStream(emptyInput, output, 0, ContentTypeText)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.OriginalSize)
	assert.True(t, result.Success)
	
	// Test very small data
	smallInput := strings.NewReader("a")
	output = &bytes.Buffer{}
	
	result, err = sc.CompressStream(smallInput, output, 1, ContentTypeText)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.OriginalSize)
	assert.True(t, result.Success)
}

func TestStreamingCompressorPerformanceTracking(t *testing.T) {
	ctx := context.Background()
	sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
	
	// Perform multiple operations to build performance history
	for i := 0; i < 5; i++ {
		testData := strings.Repeat("Performance tracking test. ", 200)
		input := strings.NewReader(testData)
		output := &bytes.Buffer{}
		
		result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
		require.NoError(t, err)
		assert.True(t, result.Success)
	}
	
	// Check that performance history was recorded
	assert.Len(t, sc.performanceHistory, 5)
	
	// Check algorithm performance tracking
	algPerf := sc.algorithmPerformance[CompressionGzip]
	assert.Equal(t, int64(5), algPerf.TotalOperations)
	assert.Len(t, algPerf.RecentRatios, 5)
	assert.Len(t, algPerf.RecentThroughputs, 5)
}

func TestStreamingCompressorCompressionStrategies(t *testing.T) {
	ctx := context.Background()
	
	strategies := []CompressionStrategy{
		StrategySpeed,
		StrategyRatio,
		StrategyBalanced,
		StrategyAdaptive,
		StrategyNetworkOptimized,
	}
	
	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			sc := NewStreamingCompressor(CompressionGzip, CompressionBalanced, ctx)
			sc.compressionStrategy = strategy
			
			testData := strings.Repeat("Strategy test data. ", 100)
			input := strings.NewReader(testData)
			output := &bytes.Buffer{}
			
			result, err := sc.CompressStream(input, output, int64(len(testData)), ContentTypeText)
			require.NoError(t, err)
			assert.True(t, result.Success)
			
			// Strategy should not affect basic compression functionality
			assert.Greater(t, result.ThroughputMBps, 0.0)
			assert.Less(t, result.CompressionRatio, 1.0)
		})
	}
}