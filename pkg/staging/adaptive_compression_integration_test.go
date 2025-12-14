package staging

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingBufferManager_WithAdaptiveCompression(t *testing.T) {
	config := DefaultStagingConfig()
	config.MaxBufferSizeMB = 64
	config.TargetChunkSizeMB = 16

	manager := NewStagingBufferManager(config)
	require.NotNil(t, manager.compressionSelector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the manager
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 100) // Let workers start
}

func TestStagingBufferManager_AdaptiveCompressionForText(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create text data
	textData := []byte(`This is sample text data that should compress very well with adaptive compression algorithms.
The quick brown fox jumps over the lazy dog. This sentence is repeated to create patterns.
The quick brown fox jumps over the lazy dog. This sentence is repeated to create patterns.
The quick brown fox jumps over the lazy dog. This sentence is repeated to create patterns.`)

	reader := bytes.NewReader(textData)

	// Create network condition favoring compression ratio
	networkCondition := &NetworkCondition{
		BandwidthMBps: 2.0, // Low bandwidth
		LatencyMs:     100.0,
		Reliability:   0.9,
	}

	req := &StagingRequest{
		StreamID:         "text-file.txt",
		Reader:           reader,
		ExpectedSize:     int64(len(textData)),
		ContentType:      "text/plain",
		NetworkCondition: networkCondition,
		Priority:         5,
	}

	var chunk *StagedChunk
	var err error
	req.Callback = func(c *StagedChunk, e error) {
		chunk = c
		err = e
	}

	err = manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err)
	require.NotNil(t, chunk)

	// Should recommend high compression for text with low bandwidth
	assert.Contains(t, []string{"zstd-high", "zstd"}, chunk.SelectedAlgorithm)
	assert.NotNil(t, chunk.CompressionSettings)
	assert.NotNil(t, chunk.CompressionDecision)
	assert.True(t, chunk.CompressionDecision.Confidence > 0.5)
}

func TestStagingBufferManager_AdaptiveCompressionForImages(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create image-like data (high entropy)
	imageData := make([]byte, 2048)
	for i := range imageData {
		imageData[i] = byte(i*7 + i*3 + 17) // High entropy pattern
	}

	reader := bytes.NewReader(imageData)

	// Create network condition favoring speed
	networkCondition := &NetworkCondition{
		BandwidthMBps: 100.0, // High bandwidth
		LatencyMs:     10.0,
		Reliability:   0.99,
	}

	req := &StagingRequest{
		StreamID:         "photo.jpg",
		Reader:           reader,
		ExpectedSize:     int64(len(imageData)),
		ContentType:      "image/jpeg",
		NetworkCondition: networkCondition,
		Priority:         3,
	}

	var chunk *StagedChunk
	var err error
	req.Callback = func(c *StagedChunk, e error) {
		chunk = c
		err = e
	}

	err = manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err)
	require.NotNil(t, chunk)

	// Should recommend no compression or fast compression for images with high bandwidth
	assert.Contains(t, []string{"none", "zstd-fast"}, chunk.SelectedAlgorithm)
	assert.NotNil(t, chunk.CompressionDecision)
}

func TestStagingBufferManager_AdaptiveCompressionLearning(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Initially no performance data
	perf := manager.GetAlgorithmPerformance("zstd")
	assert.Nil(t, perf)

	// Simulate compression result
	result := &CompressionResult{
		Algorithm:        "zstd",
		ContentType:      "text/plain",
		FileSize:         1024,
		CompressionRatio: 0.3,
		SpeedMBps:        120.0,
		MemoryUsageMB:    32.0,
		CompressionTime:  time.Millisecond * 100,
		Success:          true,
		Timestamp:        time.Now(),
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 10.0,
		},
		Context: &CompressionContext{
			Priority: 5,
		},
	}

	// Learn from result
	manager.LearnFromCompressionResult(result)

	// Should now have performance data
	perf = manager.GetAlgorithmPerformance("zstd")
	require.NotNil(t, perf)
	assert.Equal(t, "zstd", perf.Algorithm)
	assert.Equal(t, int64(1), perf.TotalCompressions)
	assert.Equal(t, 0.3, perf.AverageCompressionRatio)
	assert.Equal(t, 120.0, perf.AverageSpeedMBps)
}

func TestStagingBufferManager_CompressionProfileManagement(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	// Test getting compression profiles
	textProfile := manager.GetCompressionProfile("text")
	require.NotNil(t, textProfile)
	assert.Equal(t, "text", textProfile.ContentType)
	assert.True(t, len(textProfile.PreferredAlgorithms) > 0)

	imageProfile := manager.GetCompressionProfile("image")
	require.NotNil(t, imageProfile)
	assert.Equal(t, "image", imageProfile.ContentType)

	// Test unknown content type (should create default)
	unknownProfile := manager.GetCompressionProfile("unknown")
	require.NotNil(t, unknownProfile)
	assert.Equal(t, "unknown", unknownProfile.ContentType)
}

func TestStagingBufferManager_CompressionRuleManagement(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create custom rule
	rule := &CompressionRule{
		Name:                 "Custom Test Rule",
		Priority:             200,
		RecommendedAlgorithm: "zstd-max",
		FallbackAlgorithms:   []string{"zstd-high"},
		Enabled:              true,
	}

	// Update rule
	manager.UpdateCompressionRule(".custom", rule)

	// Test that the rule affects compression selection
	testData := []byte("test data for custom rule")
	reader := bytes.NewReader(testData)

	req := &StagingRequest{
		StreamID:     "test.custom",
		Reader:       reader,
		ExpectedSize: int64(len(testData)),
		ContentType:  "application/custom",
		Priority:     5,
	}

	var chunk *StagedChunk
	var err error
	req.Callback = func(c *StagedChunk, e error) {
		chunk = c
		err = e
	}

	err = manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.NotEmpty(t, chunk.SelectedAlgorithm)
}

func TestStagingBufferManager_CompressionWithDeduplication(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create identical test data
	testData := make([]byte, 2048)
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}

	reader1 := bytes.NewReader(testData)
	reader2 := bytes.NewReader(testData)

	// Stage first chunk
	req1 := &StagingRequest{
		StreamID:     "file1.txt",
		Reader:       reader1,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
		Priority:     5,
	}

	var chunk1 *StagedChunk
	var err1 error
	req1.Callback = func(c *StagedChunk, e error) {
		chunk1 = c
		err1 = e
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err1)
	require.NotNil(t, chunk1)
	assert.False(t, chunk1.IsDuplicate)
	assert.NotEmpty(t, chunk1.SelectedAlgorithm)

	// Stage duplicate chunk
	req2 := &StagingRequest{
		StreamID:     "file2.txt",
		Reader:       reader2,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
		Priority:     5,
	}

	var chunk2 *StagedChunk
	var err2 error
	req2.Callback = func(c *StagedChunk, e error) {
		chunk2 = c
		err2 = e
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err2)
	require.NotNil(t, chunk2)
	assert.True(t, chunk2.IsDuplicate)
	// Duplicate chunks should still have compression algorithm selected
	assert.NotEmpty(t, chunk2.SelectedAlgorithm)
}

func TestStagingBufferManager_CompressionHistoryManagement(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	// Add some performance data
	result1 := &CompressionResult{
		Algorithm:        "zstd",
		ContentType:      "text",
		FileSize:         1024,
		CompressionRatio: 0.3,
		SpeedMBps:        100.0,
		MemoryUsageMB:    16.0,
		Success:          true,
		Timestamp:        time.Now(),
	}

	result2 := &CompressionResult{
		Algorithm:        "zstd-fast",
		ContentType:      "binary",
		FileSize:         2048,
		CompressionRatio: 0.6,
		SpeedMBps:        300.0,
		MemoryUsageMB:    8.0,
		Success:          true,
		Timestamp:        time.Now(),
	}

	manager.LearnFromCompressionResult(result1)
	manager.LearnFromCompressionResult(result2)

	// Verify data exists
	perf1 := manager.GetAlgorithmPerformance("zstd")
	perf2 := manager.GetAlgorithmPerformance("zstd-fast")
	require.NotNil(t, perf1)
	require.NotNil(t, perf2)
	assert.True(t, len(perf1.RecentPerformance) > 0)
	assert.True(t, len(perf2.RecentPerformance) > 0)

	// Clear history
	manager.ClearCompressionHistory()

	// Verify history is cleared but algorithms still exist
	perf1 = manager.GetAlgorithmPerformance("zstd")
	perf2 = manager.GetAlgorithmPerformance("zstd-fast")
	require.NotNil(t, perf1)
	require.NotNil(t, perf2)
	assert.Equal(t, 0, len(perf1.RecentPerformance))
	assert.Equal(t, 0, len(perf2.RecentPerformance))
}

func TestStagingBufferManager_ContextualCompressionOptimization(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Test high priority task (should favor speed)
	testData := []byte("high priority task data for compression testing")
	reader := bytes.NewReader(testData)

	req := &StagingRequest{
		StreamID:     "urgent.txt",
		Reader:       reader,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
		Priority:     9, // High priority
	}

	var chunk *StagedChunk
	var err error
	req.Callback = func(c *StagedChunk, e error) {
		chunk = c
		err = e
	}

	err = manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err)
	require.NotNil(t, chunk)

	// High priority should tend toward faster compression
	assert.NotEmpty(t, chunk.SelectedAlgorithm)
	assert.NotNil(t, chunk.CompressionDecision)

	// Decision should have reasoning chain
	assert.True(t, len(chunk.CompressionDecision.ReasoningChain) > 0)
}

func TestStagingBufferManager_NetworkConditionAdaptation(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	testData := []byte("network condition adaptation test data for compression")

	// Test unreliable network (should favor faster compression)
	reader1 := bytes.NewReader(testData)
	networkCondition1 := &NetworkCondition{
		BandwidthMBps: 10.0,
		LatencyMs:     50.0,
		Reliability:   0.7, // Poor reliability
	}

	req1 := &StagingRequest{
		StreamID:         "unreliable.txt",
		Reader:           reader1,
		ExpectedSize:     int64(len(testData)),
		ContentType:      "text/plain",
		NetworkCondition: networkCondition1,
		Priority:         5,
	}

	var chunk1 *StagedChunk
	var err1 error
	req1.Callback = func(c *StagedChunk, e error) {
		chunk1 = c
		err1 = e
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err1)
	require.NotNil(t, chunk1)

	// Test reliable, high-bandwidth network (can use higher compression)
	reader2 := bytes.NewReader(testData)
	networkCondition2 := &NetworkCondition{
		BandwidthMBps: 1.0, // Very low bandwidth
		LatencyMs:     20.0,
		Reliability:   0.99, // Excellent reliability
	}

	req2 := &StagingRequest{
		StreamID:         "reliable.txt",
		Reader:           reader2,
		ExpectedSize:     int64(len(testData)),
		ContentType:      "text/plain",
		NetworkCondition: networkCondition2,
		Priority:         5,
	}

	var chunk2 *StagedChunk
	var err2 error
	req2.Callback = func(c *StagedChunk, e error) {
		chunk2 = c
		err2 = e
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NoError(t, err2)
	require.NotNil(t, chunk2)

	// Both should have valid compression decisions
	assert.NotEmpty(t, chunk1.SelectedAlgorithm)
	assert.NotEmpty(t, chunk2.SelectedAlgorithm)
	assert.NotNil(t, chunk1.CompressionDecision)
	assert.NotNil(t, chunk2.CompressionDecision)
}

func BenchmarkStagingBufferManager_AdaptiveCompressionSelection(b *testing.B) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	testData := make([]byte, 32*1024) // 32KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		chunkID := 0
		for pb.Next() {
			reader := bytes.NewReader(testData)
			req := &StagingRequest{
				StreamID:     "bench-" + string(rune(chunkID)),
				Reader:       reader,
				ExpectedSize: int64(len(testData)),
				ContentType:  "application/octet-stream",
				NetworkCondition: &NetworkCondition{
					BandwidthMBps: 25.0,
					LatencyMs:     40.0,
					Reliability:   0.9,
				},
				Priority: 5,
			}
			req.Callback = func(*StagedChunk, error) {}

			_ = manager.StageChunk(req, ChunkBoundary{})
			chunkID++
		}
	})
}
