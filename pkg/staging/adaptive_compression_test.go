package staging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdaptiveCompressionSelector(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	selector := NewAdaptiveCompressionSelector(config)

	require.NotNil(t, selector)
	assert.Equal(t, config, selector.config)
	assert.NotNil(t, selector.compressionProfiles)
	assert.NotNil(t, selector.algorithmPerformance)
	assert.NotNil(t, selector.fileTypeRules)

	// Verify default profiles are initialized
	assert.True(t, len(selector.compressionProfiles) > 0)
	assert.True(t, len(selector.fileTypeRules) > 0)
}

func TestNewAdaptiveCompressionSelectorWithNilConfig(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	require.NotNil(t, selector)
	require.NotNil(t, selector.config)

	// Should use default config
	defaultConfig := DefaultAdaptiveCompressionConfig()
	assert.Equal(t, defaultConfig.EnableLearning, selector.config.EnableLearning)
	assert.Equal(t, defaultConfig.MinCompressionRatio, selector.config.MinCompressionRatio)
}

func TestDefaultAdaptiveCompressionConfig(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()

	require.NotNil(t, config)
	assert.True(t, config.EnableLearning)
	assert.True(t, config.EnableNetworkAdaptation)
	assert.True(t, config.EnableContextualOptimization)
	assert.True(t, config.EnableRealtimeMonitoring)
	assert.Equal(t, 0.05, config.MinCompressionRatio)
	assert.Equal(t, time.Second*30, config.MaxCompressionTime)
}

func TestSelectCompressionAlgorithm_TextContent(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType:     "text",
		Size:            1024 * 1024, // 1MB
		Entropy:         3.5,
		Compressibility: 0.7,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.95,
	}

	context := &CompressionContext{
		FileName:          "test.txt",
		FileExtension:     ".txt",
		FileSize:          1024 * 1024,
		SystemLoad:        0.3,
		AvailableMemoryMB: 2048,
		Priority:          5,
	}

	decision, err := selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	require.NoError(t, err)
	require.NotNil(t, decision)

	// Should recommend high compression for text files
	assert.Contains(t, []string{"zstd-high", "zstd"}, decision.SelectedAlgorithm)
	assert.True(t, decision.Confidence > 0.5)
	assert.NotNil(t, decision.RecommendedSettings)
	assert.True(t, len(decision.ReasoningChain) > 0)
}

func TestSelectCompressionAlgorithm_ImageContent(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType:     "image",
		Size:            5 * 1024 * 1024, // 5MB
		Entropy:         7.2,
		Compressibility: 0.05,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps: 50.0,
		LatencyMs:     20.0,
		Reliability:   0.98,
	}

	context := &CompressionContext{
		FileName:          "photo.jpg",
		FileExtension:     ".jpg",
		FileSize:          5 * 1024 * 1024,
		SystemLoad:        0.2,
		AvailableMemoryMB: 4096,
		Priority:          3,
	}

	decision, err := selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	require.NoError(t, err)
	require.NotNil(t, decision)

	// Should recommend appropriate compression for images
	// The algorithm can be any reasonable choice for images - none, fast, or balanced
	assert.Contains(t, []string{"none", "zstd-fast", "zstd"}, decision.SelectedAlgorithm)
	assert.True(t, decision.Confidence > 0.5)
}

func TestSelectCompressionAlgorithm_HighBandwidth(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType:     "binary",
		Size:            10 * 1024 * 1024, // 10MB
		Entropy:         5.0,
		Compressibility: 0.4,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps: 100.0, // High bandwidth
		LatencyMs:     10.0,
		Reliability:   0.99,
	}

	context := &CompressionContext{
		FileName:          "data.bin",
		FileExtension:     ".bin",
		FileSize:          10 * 1024 * 1024,
		SystemLoad:        0.1,
		AvailableMemoryMB: 8192,
		Priority:          2,
	}

	decision, err := selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	require.NoError(t, err)
	require.NotNil(t, decision)

	// High bandwidth should favor speed over compression ratio
	assert.Contains(t, []string{"zstd-fast", "none"}, decision.SelectedAlgorithm)
}

func TestSelectCompressionAlgorithm_LowBandwidth(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType:     "json",
		Size:            2 * 1024 * 1024, // 2MB
		Entropy:         4.0,
		Compressibility: 0.6,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps: 0.5, // Low bandwidth
		LatencyMs:     200.0,
		Reliability:   0.85,
	}

	context := &CompressionContext{
		FileName:          "config.json",
		FileExtension:     ".json",
		FileSize:          2 * 1024 * 1024,
		SystemLoad:        0.6,
		AvailableMemoryMB: 1024,
		Priority:          7,
	}

	decision, err := selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	require.NoError(t, err)
	require.NotNil(t, decision)

	// Low bandwidth should favor compression ratio over speed
	assert.Contains(t, []string{"zstd-high", "zstd"}, decision.SelectedAlgorithm)
}

func TestLearnFromCompressionResult(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	result := &CompressionResult{
		Algorithm:        "zstd",
		ContentType:      "text",
		FileSize:         1024 * 1024,
		CompressionRatio: 0.3,
		SpeedMBps:        120.0,
		MemoryUsageMB:    32.0,
		CompressionTime:  time.Millisecond * 100,
		Success:          true,
		Timestamp:        time.Now(),
	}

	// Initially no performance data
	perf := selector.GetAlgorithmPerformance("zstd")
	assert.Nil(t, perf)

	// Learn from result
	selector.LearnFromCompressionResult(result)

	// Should now have performance data
	perf = selector.GetAlgorithmPerformance("zstd")
	require.NotNil(t, perf)
	assert.Equal(t, "zstd", perf.Algorithm)
	assert.Equal(t, int64(1), perf.TotalCompressions)
	assert.Equal(t, 0.3, perf.AverageCompressionRatio)
	assert.Equal(t, 120.0, perf.AverageSpeedMBps)
	assert.Equal(t, 32.0, perf.AverageMemoryUsageMB)
	assert.Equal(t, 1.0, perf.SuccessRate)
}

func TestGetCompressionProfile(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	// Test known content type
	profile := selector.GetCompressionProfile("text")
	require.NotNil(t, profile)
	assert.Equal(t, "text", profile.ContentType)
	assert.True(t, len(profile.PreferredAlgorithms) > 0)

	// Test unknown content type (should create default)
	profile = selector.GetCompressionProfile("unknown")
	require.NotNil(t, profile)
	assert.Equal(t, "unknown", profile.ContentType)
	assert.True(t, len(profile.PreferredAlgorithms) > 0)
}

func TestUpdateCompressionRule(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	// Create new rule
	rule := &CompressionRule{
		Name:                 "Custom Test Rule",
		Priority:             200,
		RecommendedAlgorithm: "zstd-max",
		FallbackAlgorithms:   []string{"zstd-high"},
		Enabled:              true,
	}

	// Update rule
	selector.UpdateCompressionRule(".test", rule)

	// Verify rule was added
	contentProfile := &ContentProfile{
		ContentType: "application/test",
		Size:        1024,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.9,
	}
	context := &CompressionContext{
		FileExtension: ".test",
	}

	decision, err := selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	require.NoError(t, err)
	assert.NotNil(t, decision)
}

func TestClearPerformanceHistory(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	// Add some performance data
	result := &CompressionResult{
		Algorithm:        "zstd",
		ContentType:      "text",
		FileSize:         1024,
		CompressionRatio: 0.3,
		SpeedMBps:        100.0,
		MemoryUsageMB:    16.0,
		Success:          true,
		Timestamp:        time.Now(),
	}
	selector.LearnFromCompressionResult(result)

	// Verify data exists
	perf := selector.GetAlgorithmPerformance("zstd")
	require.NotNil(t, perf)
	assert.True(t, len(perf.RecentPerformance) > 0)

	// Clear history
	selector.ClearPerformanceHistory()

	// Verify history is cleared
	perf = selector.GetAlgorithmPerformance("zstd")
	require.NotNil(t, perf)
	assert.Equal(t, 0, len(perf.RecentPerformance))
}

func TestCompressionLearningEngine(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	engine := NewCompressionLearningEngine(config)

	require.NotNil(t, engine)
	assert.NotNil(t, engine.featureExtractor)

	contentProfile := &ContentProfile{
		ContentType: "text",
		Size:        1024,
		Entropy:     3.0,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.9,
	}
	context := &CompressionContext{
		SystemLoad:        0.3,
		AvailableMemoryMB: 2048,
		Priority:          5,
	}

	recommendation := engine.PredictOptimalAlgorithm(contentProfile, networkCondition, context)
	require.NotNil(t, recommendation)
	assert.NotEmpty(t, recommendation.Algorithm)
	assert.True(t, recommendation.Confidence > 0)
	assert.True(t, len(recommendation.Reasoning) > 0)
}

func TestCompressionPerformancePredictor(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	predictor := NewCompressionPerformancePredictor(config)

	require.NotNil(t, predictor)

	contentProfile := &ContentProfile{
		ContentType: "text",
		Size:        1024 * 1024,
		Entropy:     2.5,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 25.0,
		LatencyMs:     30.0,
		Reliability:   0.95,
	}

	prediction := predictor.PredictPerformance("zstd", contentProfile, networkCondition)
	require.NotNil(t, prediction)
	assert.True(t, prediction.EstimatedCompressionRatio > 0)
	assert.True(t, prediction.EstimatedSpeedMBps > 0)
	assert.True(t, prediction.EstimatedMemoryUsageMB > 0)
	assert.True(t, prediction.EstimatedCompressionTime > 0)
	assert.True(t, prediction.Confidence > 0)
}

func TestContextualCompressionOptimizer(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	optimizer := NewContextualCompressionOptimizer(config)

	require.NotNil(t, optimizer)
	assert.True(t, len(optimizer.contextRules) > 0)

	contentProfile := &ContentProfile{
		ContentType: "text",
		Size:        1024,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     30.0, // Low latency
		Reliability:   0.95,
	}
	context := &CompressionContext{
		AvailableMemoryMB: 2048, // High memory
		SystemLoad:        0.2,
		Priority:          5,
	}
	profile := &CompressionProfile{
		ContentType: "text",
	}

	recommendation := optimizer.OptimizeSelection(contentProfile, networkCondition, context, profile)
	require.NotNil(t, recommendation)
	assert.NotEmpty(t, recommendation.Algorithm)
	assert.True(t, recommendation.Weight > 0)
	assert.True(t, len(recommendation.Factors) > 0)
}

func TestRealtimeCompressionMonitor(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	monitor := NewRealtimeCompressionMonitor(config)

	require.NotNil(t, monitor)
	assert.NotNil(t, monitor.thresholds)
	assert.NotNil(t, monitor.metrics)
	assert.NotNil(t, monitor.alerts)
}

func TestNetworkCompressionAdapter(t *testing.T) {
	config := DefaultAdaptiveCompressionConfig()
	adapter := NewNetworkCompressionAdapter("ethernet", config)

	require.NotNil(t, adapter)
	assert.Equal(t, "ethernet", adapter.networkType)
	assert.NotNil(t, adapter.adaptations)
}

func TestFeatureExtractor(t *testing.T) {
	extractor := NewFeatureExtractor()
	require.NotNil(t, extractor)

	contentProfile := &ContentProfile{
		ContentType:     "text",
		Size:            1024,
		Entropy:         3.5,
		Compressibility: 0.7,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 25.0,
		LatencyMs:     40.0,
		Reliability:   0.9,
	}
	context := &CompressionContext{
		SystemLoad:        0.4,
		AvailableMemoryMB: 1024,
		Priority:          6,
	}

	features := extractor.ExtractFeatures(contentProfile, networkCondition, context)
	require.NotNil(t, features)

	// Check that key features are extracted
	assert.Equal(t, float64(1024), features["content_size"])
	assert.Equal(t, 3.5, features["content_entropy"])
	assert.Equal(t, 0.7, features["content_compressibility"])
	assert.Equal(t, 25.0, features["network_bandwidth"])
	assert.Equal(t, 40.0, features["network_latency"])
	assert.Equal(t, 0.9, features["network_reliability"])
	assert.Equal(t, 0.4, features["system_load"])
	assert.Equal(t, float64(1024), features["memory_available"])
	assert.Equal(t, float64(6), features["priority"])
}

func TestInitializeDefaultProfiles(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	// Check that default profiles are created
	profiles := []string{"text", "binary", "image", "compressed", "json"}
	for _, contentType := range profiles {
		profile := selector.GetCompressionProfile(contentType)
		require.NotNil(t, profile)
		assert.Equal(t, contentType, profile.ContentType)
		assert.True(t, len(profile.PreferredAlgorithms) > 0)
		assert.True(t, len(profile.AlgorithmEffectiveness) > 0)
	}
}

func TestGetOptimalSettings(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType: "text",
		Size:        1024,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.9,
	}

	// Test different algorithms
	algorithms := []string{"zstd-fast", "zstd", "zstd-high", "none"}
	for _, algorithm := range algorithms {
		settings := selector.getOptimalSettings(algorithm, contentProfile, networkCondition)
		require.NotNil(t, settings)
		assert.Equal(t, algorithm, settings.Algorithm)
		
		if algorithm != "none" {
			assert.True(t, settings.Level >= 0)
			assert.True(t, settings.WindowSize >= 0)
			assert.True(t, settings.ThreadCount >= 0)
		} else {
			assert.Equal(t, 0, settings.Level)
			assert.Equal(t, 0, settings.WindowSize)
			assert.Equal(t, 0, settings.ThreadCount)
		}
	}
}

func TestCalculateConfidence(t *testing.T) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType: "text",
		Size:        1024,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.95, // High reliability
	}

	confidence := selector.calculateConfidence("zstd", contentProfile, networkCondition)
	assert.True(t, confidence > 0.5)
	assert.True(t, confidence <= 0.95)
}

func BenchmarkSelectCompressionAlgorithm(b *testing.B) {
	selector := NewAdaptiveCompressionSelector(nil)

	contentProfile := &ContentProfile{
		ContentType:     "text",
		Size:            1024 * 1024,
		Entropy:         3.5,
		Compressibility: 0.7,
	}
	networkCondition := &NetworkCondition{
		BandwidthMBps: 25.0,
		LatencyMs:     40.0,
		Reliability:   0.9,
	}
	context := &CompressionContext{
		FileName:          "test.txt",
		FileExtension:     ".txt",
		FileSize:          1024 * 1024,
		SystemLoad:        0.3,
		AvailableMemoryMB: 2048,
		Priority:          5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = selector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	}
}

func BenchmarkLearnFromCompressionResult(b *testing.B) {
	selector := NewAdaptiveCompressionSelector(nil)

	result := &CompressionResult{
		Algorithm:        "zstd",
		ContentType:      "text",
		FileSize:         1024,
		CompressionRatio: 0.3,
		SpeedMBps:        100.0,
		MemoryUsageMB:    16.0,
		Success:          true,
		Timestamp:        time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.LearnFromCompressionResult(result)
	}
}