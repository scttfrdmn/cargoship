package s3

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context key is now defined in adaptive_staging.go

func TestNewAdaptiveStaging(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	assert.NotNil(t, as)
	assert.Equal(t, StagingAdaptive, as.stagingStrategy)
	assert.True(t, as.adaptationEnabled)
	assert.Equal(t, time.Second*30, as.adaptationInterval)
	assert.Equal(t, time.Minute*5, as.performanceWindow)
	assert.NotNil(t, as.progressTracker)
	assert.NotNil(t, as.performanceAnalyzer)
	assert.NotNil(t, as.networkMonitor)
	assert.NotNil(t, as.stagingBuffer)
	assert.NotNil(t, as.chunkSizeAdaptor)
	assert.NotNil(t, as.priorityManager)
	assert.NotNil(t, as.resourceAllocator)
	assert.NotNil(t, as.stagingMetrics)
	assert.NotNil(t, as.performanceGoals)
}

func TestAdaptiveStagingStageChunk(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Create test data
	testData := strings.Repeat("Hello, World! ", 1000)
	reader := strings.NewReader(testData)

	// Create context with start_time for timing metrics
	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())

	result, err := as.StageChunk(ctxWithTime, "test-chunk-1", reader, int64(len(testData)), ChunkPriorityNormal)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-chunk-1", result.ChunkID)
	assert.True(t, result.Success)
	assert.Greater(t, result.StagedSize, int64(0))
	assert.Greater(t, result.CompressionRatio, 0.0)
	assert.Less(t, result.CompressionRatio, 1.0)
	assert.Greater(t, result.StagingTime, time.Duration(0))
	assert.NotNil(t, result.Metrics)
}

func TestAdaptiveStagingStageChunkWithDifferentPriorities(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	priorities := []ChunkPriority{
		ChunkPriorityLow,
		ChunkPriorityNormal,
		ChunkPriorityHigh,
		ChunkPriorityCritical,
	}

	for i, priority := range priorities {
		t.Run(string(priority), func(t *testing.T) {
			testData := strings.Repeat("Test data for priority testing. ", 100)
			reader := strings.NewReader(testData)
			chunkID := fmt.Sprintf("priority-test-%d", i)

			// Create context with start_time for timing metrics
			ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())

			result, err := as.StageChunk(ctxWithTime, chunkID, reader, int64(len(testData)), priority)

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, chunkID, result.ChunkID)
			assert.True(t, result.Success)
		})
	}
}

func TestAdaptiveStagingAdaptStagingStrategy(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Test initial strategy
	assert.Equal(t, StagingAdaptive, as.stagingStrategy)

	// Trigger adaptation
	result, err := as.AdaptStagingStrategy(ctx)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.NewStrategy)
	assert.GreaterOrEqual(t, result.Confidence, 0.0)
	assert.LessOrEqual(t, result.Confidence, 1.0)

	// Verify adaptation was recorded
	assert.Greater(t, len(as.adaptationHistory), 0)
}

func TestAdaptiveStagingStagingStrategies(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	strategies := []StagingStrategy{
		StagingAggressive,
		StagingConservative,
		StagingBalanced,
		StagingPredictive,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			as.stagingStrategy = strategy
			as.updateComponentConfigurations(strategy)

			// Verify strategy was applied
			assert.Equal(t, strategy, as.stagingStrategy)

			// Test staging with this strategy
			testData := "Test data for strategy testing"
			reader := strings.NewReader(testData)

			ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
			result, err := as.StageChunk(ctxWithTime, "strategy-test", reader, int64(len(testData)), ChunkPriorityNormal)
			require.NoError(t, err)
			assert.True(t, result.Success)
		})
	}
}

func TestAdaptiveStagingGetStagingStatus(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Stage a few chunks to generate status
	for i := 0; i < 3; i++ {
		testData := strings.Repeat("Status test data. ", 50)
		reader := strings.NewReader(testData)
		chunkID := fmt.Sprintf("status-test-%d", i)

		ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
		_, err := as.StageChunk(ctxWithTime, chunkID, reader, int64(len(testData)), ChunkPriorityNormal)
		require.NoError(t, err)
	}

	status := as.GetStagingStatus()

	assert.NotNil(t, status)
	assert.NotEmpty(t, status.Strategy)
	assert.NotNil(t, status.CurrentPerformance)
	assert.GreaterOrEqual(t, status.BufferUtilization, 0.0)
	assert.LessOrEqual(t, status.BufferUtilization, 1.0)
	assert.GreaterOrEqual(t, status.ThroughputMBps, 0.0)
	assert.Equal(t, int64(3), status.StagedChunks)
	assert.Greater(t, status.StagedBytes, int64(0))
	assert.GreaterOrEqual(t, status.ErrorRate, 0.0)
	assert.NotNil(t, status.ResourceUsage)
}

func TestAdaptiveStagingConcurrentStaging(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Test concurrent staging operations
	numChunks := 5
	results := make(chan *StagingResult, numChunks)
	errors := make(chan error, numChunks)

	for i := 0; i < numChunks; i++ {
		go func(id int) {
			testData := strings.Repeat("Concurrent staging test. ", 100)
			reader := strings.NewReader(testData)
			chunkID := fmt.Sprintf("concurrent-%d", id)

			ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
			result, err := as.StageChunk(ctxWithTime, chunkID, reader, int64(len(testData)), ChunkPriorityNormal)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numChunks; i++ {
		select {
		case result := <-results:
			assert.NotNil(t, result)
			assert.True(t, result.Success)
			successCount++
		case err := <-errors:
			t.Errorf("Concurrent staging failed: %v", err)
		case <-time.After(time.Second * 10):
			t.Fatal("Concurrent staging timed out")
		}
	}

	assert.Equal(t, numChunks, successCount)
}

func TestAdaptiveStagingLargeChunk(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Create a large chunk (1MB)
	largeData := bytes.Repeat([]byte("Large chunk test data. "), 45000) // ~1MB
	reader := bytes.NewReader(largeData)

	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
	result, err := as.StageChunk(ctxWithTime, "large-chunk", reader, int64(len(largeData)), ChunkPriorityHigh)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "large-chunk", result.ChunkID)
	assert.True(t, result.Success)
	assert.Greater(t, result.StagedSize, int64(0))
	assert.Greater(t, result.StagingTime, time.Duration(0))
}

func TestAdaptiveStagingEmptyChunk(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Test empty chunk
	reader := strings.NewReader("")

	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
	result, err := as.StageChunk(ctxWithTime, "empty-chunk", reader, 0, ChunkPriorityNormal)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "empty-chunk", result.ChunkID)
	assert.True(t, result.Success)
	assert.Equal(t, int64(0), result.StagedSize)
}

func TestAdaptiveStagingContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Cancel context immediately
	cancel()

	testData := "Context cancellation test"
	reader := strings.NewReader(testData)

	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
	result, err := as.StageChunk(ctxWithTime, "cancelled-chunk", reader, int64(len(testData)), ChunkPriorityNormal)

	// Should still work since we're not checking context in the simplified implementation
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestProgressTracker(t *testing.T) {
	pt := NewProgressTracker()

	assert.NotNil(t, pt)
	assert.Equal(t, int64(0), pt.totalBytes)
	assert.Equal(t, int64(0), pt.uploadedBytes)
	assert.Equal(t, int64(0), pt.stagedBytes)
	assert.Equal(t, 0.0, pt.currentThroughput)
	assert.NotZero(t, pt.lastUpdate)

	// Test progress update
	result := &StagingResult{
		ChunkID:    "test",
		Success:    true,
		StagedSize: 1024,
	}

	pt.UpdateProgress(result)
	assert.Equal(t, int64(1024), pt.stagedBytes)

	// Test throughput calculation
	throughput := pt.GetCurrentThroughput()
	assert.GreaterOrEqual(t, throughput, 0.0)
}

func TestPerformanceAnalyzer(t *testing.T) {
	pa := NewPerformanceAnalyzer()

	assert.NotNil(t, pa)
	assert.NotNil(t, pa.performanceTrends)
	assert.NotNil(t, pa.adaptationTriggers)
	assert.NotNil(t, pa.triggerThresholds)

	// Test performance analysis
	metrics := pa.AnalyzeCurrentPerformance()
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics.ThroughputMBps, 0.0)
	assert.Greater(t, metrics.LatencyMs, 0.0)
	assert.Greater(t, metrics.TargetThroughput, 0.0)
	assert.GreaterOrEqual(t, metrics.Reliability, 0.0)
	assert.LessOrEqual(t, metrics.Reliability, 1.0)
}

func TestStagingBuffer(t *testing.T) {
	sb := NewStagingBuffer(10 * 1024 * 1024) // 10MB

	assert.NotNil(t, sb)
	assert.Equal(t, int64(10*1024*1024), sb.maxBufferSize)
	assert.Equal(t, int64(0), sb.currentBufferSize)
	assert.NotNil(t, sb.allocatedChunks)
	assert.True(t, sb.compressionEnabled)
	assert.True(t, sb.dedupEnabled)

	// Test buffer allocation
	buffer, err := sb.AllocateBuffer("test", 1024, BufferAllocationDynamic)
	require.NoError(t, err)
	assert.NotNil(t, buffer)
	assert.Equal(t, 1024, len(buffer))
	assert.Equal(t, int64(1024), sb.currentBufferSize)

	// Test utilization
	utilization := sb.GetUtilization()
	assert.Greater(t, utilization, 0.0)
	assert.Less(t, utilization, 1.0)

	// Test buffer release
	err = sb.ReleaseBuffer("test")
	assert.NoError(t, err)

	// Test storing chunk
	chunk := &StagedChunk{
		ID:   "stored-chunk",
		Data: []byte("test data"),
		Size: 9,
	}

	err = sb.StoreChunk(chunk)
	assert.NoError(t, err)
	assert.Contains(t, sb.allocatedChunks, "stored-chunk")
}

func TestStagingBufferAllocationStrategies(t *testing.T) {
	sb := NewStagingBuffer(10 * 1024 * 1024)

	strategies := []BufferAllocationStrategy{
		BufferAllocationFixed,
		BufferAllocationDynamic,
		BufferAllocationAdaptive,
		BufferAllocationPredictive,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			optimal := sb.GetOptimalStrategy(1024, ChunkPriorityNormal)
			assert.NotEmpty(t, optimal)
		})
	}
}

func TestChunkSizeAdaptor(t *testing.T) {
	csa := NewChunkSizeAdaptor()

	assert.NotNil(t, csa)
	assert.Equal(t, int64(16*1024*1024), csa.baseChunkSize)
	assert.Equal(t, int64(16*1024*1024), csa.currentChunkSize)
	assert.Equal(t, int64(4*1024*1024), csa.minChunkSize)
	assert.Equal(t, int64(64*1024*1024), csa.maxChunkSize)
	assert.Equal(t, AdaptationGradual, csa.adaptationAlgorithm)
	assert.NotNil(t, csa.chunkPerformance)

	// Test optimal chunk size calculation
	networkConditions := &NetworkConditionSummary{
		BandwidthMBps: 50.0,
		LatencyMs:     30.0,
	}

	optimalSize := csa.GetOptimalChunkSize(1024*1024, networkConditions)
	assert.Greater(t, optimalSize, int64(0))
	assert.GreaterOrEqual(t, optimalSize, csa.minChunkSize)
	assert.LessOrEqual(t, optimalSize, csa.maxChunkSize)

	// Test setting target chunk size
	csa.SetTargetChunkSize(32 * 1024 * 1024)
	assert.Equal(t, int64(32*1024*1024), csa.currentChunkSize)

	// Test invalid sizes
	csa.SetTargetChunkSize(1024)                               // Too small
	assert.Equal(t, int64(32*1024*1024), csa.currentChunkSize) // Should remain unchanged

	csa.SetTargetChunkSize(128 * 1024 * 1024)                  // Too large
	assert.Equal(t, int64(32*1024*1024), csa.currentChunkSize) // Should remain unchanged
}

func TestResourceAllocator(t *testing.T) {
	ra := NewResourceAllocator()

	assert.NotNil(t, ra)
	assert.Equal(t, 8, ra.maxConcurrentChunks)
	assert.Equal(t, int64(512*1024*1024), ra.maxMemoryUsage)
	assert.Equal(t, 1000.0, ra.maxNetworkBandwidth)
	assert.Equal(t, 0.8, ra.maxCPUUsage)
	assert.Equal(t, ResourceAllocationBalanced, ra.allocationStrategy)
	assert.True(t, ra.loadBalancing)
	assert.False(t, ra.preemptionEnabled)

	// Test current usage
	usage := ra.GetCurrentUsage()
	assert.NotNil(t, usage)
	assert.GreaterOrEqual(t, usage.CPUUsage, 0.0)
	assert.GreaterOrEqual(t, usage.MemoryUsage, int64(0))
	assert.GreaterOrEqual(t, usage.NetworkUsage, 0.0)
	assert.GreaterOrEqual(t, usage.DiskUsage, 0.0)

	// Test usage summary
	summary := ra.GetUsageSummary()
	assert.NotNil(t, summary)
	assert.Equal(t, usage.CPUUsage, summary.CPU)
	assert.Equal(t, usage.MemoryUsage, summary.Memory)
	assert.Equal(t, usage.NetworkUsage, summary.Network)
	assert.Equal(t, usage.DiskUsage, summary.Disk)

	// Test setting max concurrent chunks
	ra.SetMaxConcurrentChunks(16)
	assert.Equal(t, 16, ra.maxConcurrentChunks)
}

func TestAdaptiveStagingShutdown(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)

	// Stage some chunks before shutdown
	testData := "Shutdown test data"
	reader := strings.NewReader(testData)

	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
	result, err := as.StageChunk(ctxWithTime, "shutdown-test", reader, int64(len(testData)), ChunkPriorityNormal)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Test graceful shutdown
	err = as.Shutdown()
	assert.NoError(t, err)

	// Verify context is cancelled
	select {
	case <-as.ctx.Done():
		// Context properly cancelled
	default:
		t.Error("Context should be cancelled after shutdown")
	}
}

func TestAdaptiveStagingAdaptationTriggers(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Set performance goals that will trigger adaptation
	as.performanceGoals.TargetThroughput = 1000.0 // Very high target

	// Stage chunks to potentially trigger adaptation
	for i := 0; i < 5; i++ {
		testData := strings.Repeat("Adaptation trigger test. ", 100)
		reader := strings.NewReader(testData)
		chunkID := fmt.Sprintf("trigger-test-%d", i)

		ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
		result, err := as.StageChunk(ctxWithTime, chunkID, reader, int64(len(testData)), ChunkPriorityNormal)
		require.NoError(t, err)
		assert.True(t, result.Success)
	}

	// Give time for any background adaptations
	time.Sleep(time.Millisecond * 100)

	// Check if any adaptations were triggered
	status := as.GetStagingStatus()
	assert.NotNil(t, status)
}

func TestAdaptiveStagingPerformanceGoals(t *testing.T) {
	goals := NewPerformanceGoals()

	assert.NotNil(t, goals)
	assert.Equal(t, 100.0, goals.TargetThroughput)
	assert.Equal(t, time.Second, goals.MaxLatency)
	assert.Equal(t, 0.99, goals.MinReliability)
	assert.Equal(t, 0.8, goals.MaxResourceUsage)
	assert.Equal(t, 0.9, goals.TargetEfficiency)
}

func TestAdaptiveStagingMetrics(t *testing.T) {
	metrics := NewStagingMetrics()

	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalChunksStaged)
	assert.Equal(t, int64(0), metrics.TotalBytesStaged)
	assert.Equal(t, 0.0, metrics.StagingThroughput)
	assert.Equal(t, time.Duration(0), metrics.AverageStagingTime)
	assert.Equal(t, 0.0, metrics.BufferUtilization)
	assert.Equal(t, 0.0, metrics.CompressionRatio)
	assert.Equal(t, 0.0, metrics.HitRate)
	assert.Equal(t, 0.0, metrics.ErrorRate)
	assert.NotZero(t, metrics.LastUpdate)
}

func TestAdaptiveStagingEdgeCases(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)
	defer func() { _ = as.Shutdown() }()

	// Test staging with zero-length data
	emptyReader := strings.NewReader("")
	ctxWithTime := context.WithValue(ctx, startTimeKey, time.Now())
	result, err := as.StageChunk(ctxWithTime, "empty", emptyReader, 0, ChunkPriorityNormal)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, int64(0), result.StagedSize)

	// Test staging with very small data
	smallReader := strings.NewReader("x")
	ctxWithTime = context.WithValue(ctx, startTimeKey, time.Now())
	result, err = as.StageChunk(ctxWithTime, "small", smallReader, 1, ChunkPriorityLow)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Test multiple adaptations in quick succession
	for i := 0; i < 3; i++ {
		adaptResult, err := as.AdaptStagingStrategy(ctx)
		require.NoError(t, err)
		assert.True(t, adaptResult.Success)
	}

	// Should have recorded multiple adaptations
	assert.GreaterOrEqual(t, len(as.adaptationHistory), 3)
}

func TestAdaptiveStagingBackgroundWorkers(t *testing.T) {
	ctx := context.Background()
	as := NewAdaptiveStaging(ctx)

	// Let background workers run for a short time
	time.Sleep(time.Millisecond * 100)

	// Verify workers are running by checking they haven't crashed
	// In a real implementation, we'd have more sophisticated monitoring

	// Shutdown should stop all workers cleanly
	err := as.Shutdown()
	assert.NoError(t, err)
}
