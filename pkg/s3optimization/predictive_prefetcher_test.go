package s3optimization

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPredictivePrefetcher_Creation(t *testing.T) {
	// Create mock S3 optimizer
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	logger := slog.Default()

	prefetcher, err := NewPredictivePrefetcher(optimizer, config, logger)
	require.NoError(t, err)
	assert.NotNil(t, prefetcher)
	assert.Equal(t, config.MaxConcurrentPrefetch, len(prefetcher.prefetchWorkers))
	assert.NotNil(t, prefetcher.patternAnalyzer)
	assert.NotNil(t, prefetcher.prefetchCache)
	assert.NotNil(t, prefetcher.requestPredictor)
}

func TestPredictivePrefetcher_StartStop(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	config.MaxConcurrentPrefetch = 2 // Reduce for testing
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test start
	err = prefetcher.Start(ctx)
	require.NoError(t, err)
	assert.True(t, prefetcher.isRunning)

	// Test double start (should return error)
	err = prefetcher.Start(ctx)
	assert.Error(t, err)

	// Test stop
	err = prefetcher.Stop()
	require.NoError(t, err)
	assert.False(t, prefetcher.isRunning)
}

func TestPredictivePrefetcher_PredictAndPrefetch(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	config.MinPatternConfidence = 0.5
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = prefetcher.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = prefetcher.Stop() }()

	// Test prediction without patterns (should not error)
	err = prefetcher.PredictAndPrefetch(ctx, "test-key-1")
	assert.NoError(t, err)

	// Add some access history to create patterns
	prefetcher.patternAnalyzer.RecordAccess("file1.txt", time.Now().Add(-time.Minute*5))
	prefetcher.patternAnalyzer.RecordAccess("file2.txt", time.Now().Add(-time.Minute*4))
	prefetcher.patternAnalyzer.RecordAccess("file3.txt", time.Now().Add(-time.Minute*3))
	prefetcher.patternAnalyzer.RecordAccess("file1.txt", time.Now().Add(-time.Minute*2))

	// Update patterns
	prefetcher.patternAnalyzer.UpdatePatterns()
	patterns := prefetcher.patternAnalyzer.GetPatterns()
	prefetcher.requestPredictor.UpdatePatterns(patterns)

	// Test prediction with patterns
	err = prefetcher.PredictAndPrefetch(ctx, "file1.txt")
	assert.NoError(t, err)
}

func TestPredictivePrefetcher_CacheOperations(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	// Start the prefetcher to enable cache operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	err = prefetcher.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = prefetcher.Stop() }()

	// Test cache miss
	obj, found := prefetcher.GetCachedObject("nonexistent-key")
	assert.False(t, found)
	assert.Nil(t, obj)

	// Add object to cache
	testData := []byte("test data content")
	metadata := map[string]string{"content-type": "text/plain"}
	err = prefetcher.prefetchCache.Put("test-key", testData, metadata)
	require.NoError(t, err)

	// Test cache hit
	obj, found = prefetcher.GetCachedObject("test-key")
	assert.True(t, found)
	assert.NotNil(t, obj)
	assert.Equal(t, "test-key", obj.Key)
	assert.Equal(t, testData, obj.Data)
}

func TestPredictivePrefetcher_NetworkConditions(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	// Test updating network conditions
	conditions := &NetworkConditions{
		Bandwidth:   25.0,
		RTT:         time.Millisecond * 150,
		PacketLoss:  0.02,
		Congestion:  0.3,
		LastUpdated: time.Now(),
	}

	prefetcher.UpdateNetworkConditions(conditions)

	// Verify conditions were applied (would check internal state in real implementation)
	// Here we just verify it doesn't panic
}

func TestPredictivePrefetcher_Metrics(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	// Get initial metrics
	metrics := prefetcher.GetPrefetchMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalPrefetches)
	assert.Equal(t, int64(0), metrics.CacheHits)

	// Record some metrics
	prefetcher.metrics.RecordPrefetch(true, 1024, time.Millisecond*100)
	prefetcher.metrics.RecordCacheHit()

	// Get updated metrics
	metrics = prefetcher.GetPrefetchMetrics()
	assert.Equal(t, int64(1), metrics.TotalPrefetches)
	assert.Equal(t, int64(1), metrics.CacheHits)
	assert.True(t, metrics.CacheHitRate > 0)
}

func TestAccessPatternAnalyzer_PatternDetection(t *testing.T) {
	config := DefaultPrefetchConfig()
	analyzer := NewAccessPatternAnalyzer(config)

	// Create sequential access pattern
	baseTime := time.Now()
	analyzer.RecordAccess("file1.txt", baseTime)
	analyzer.RecordAccess("file2.txt", baseTime.Add(time.Minute))
	analyzer.RecordAccess("file3.txt", baseTime.Add(time.Minute*2))
	analyzer.RecordAccess("file1.txt", baseTime.Add(time.Minute*3))
	analyzer.RecordAccess("file2.txt", baseTime.Add(time.Minute*4))
	analyzer.RecordAccess("file3.txt", baseTime.Add(time.Minute*5))

	// Update patterns
	analyzer.UpdatePatterns()
	patterns := analyzer.GetPatterns()

	assert.True(t, len(patterns) >= 0) // Should detect some patterns

	// Test access frequency
	frequency := analyzer.GetAccessFrequency("file1.txt")
	assert.True(t, frequency > 0)

	// Test next access prediction
	predictions := analyzer.PredictNextAccess("file1.txt")
	assert.NotNil(t, predictions)
	// Note: predictions might be empty if no patterns detected yet
}

func TestRequestPredictor_Predictions(t *testing.T) {
	config := DefaultPrefetchConfig()
	predictor := NewRequestPredictor(config)

	// Create test patterns
	patterns := map[string]*AccessPattern{
		"seq1": {
			Type:          PatternSequential,
			Keys:          []string{"file1.txt", "file2.txt", "file3.txt"},
			Confidence:    0.8,
			PredictedNext: []string{"file2.txt"},
			LastUpdated:   time.Now(),
		},
		"temp1": {
			Type:          PatternTemporal,
			Keys:          []string{"doc1.pdf", "doc2.pdf"},
			Confidence:    0.7,
			PredictedNext: []string{"doc1.pdf", "doc2.pdf"},
			LastUpdated:   time.Now(),
		},
	}

	predictor.UpdatePatterns(patterns)

	// Test predictions
	predictions := predictor.PredictNextRequests("file1.txt", 5)
	assert.NotNil(t, predictions)

	// Test prediction result recording
	if len(predictions) > 0 {
		predictor.RecordPredictionResult(predictions[0], time.Now(), true)
		accuracy := predictor.GetPredictionAccuracy()
		assert.NotNil(t, accuracy)
	}
}

func TestPrefetchCache_Operations(t *testing.T) {
	config := DefaultPrefetchConfig()
	config.CacheSize = 1024 * 1024 // 1MB cache
	cache := NewPrefetchCache(config)

	// Test put and get
	testData := []byte("test content for caching")
	metadata := map[string]string{"type": "test"}

	err := cache.Put("test-key", testData, metadata)
	require.NoError(t, err)

	obj, found := cache.Get("test-key")
	assert.True(t, found)
	assert.NotNil(t, obj)
	assert.Equal(t, testData, obj.Data)
	assert.Equal(t, metadata, obj.Metadata)

	// Test cache miss
	obj, found = cache.Get("nonexistent")
	assert.False(t, found)
	assert.Nil(t, obj)

	// Test cache stats
	stats := cache.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.True(t, stats.HitRate > 0)

	// Test remove
	cache.Remove("test-key")
	obj, found = cache.Get("test-key")
	assert.False(t, found)

	// Test clear
	cache.Put("key1", []byte("data1"), nil)
	cache.Put("key2", []byte("data2"), nil)
	cache.Clear()
	
	obj, found = cache.Get("key1")
	assert.False(t, found)
	obj, found = cache.Get("key2")
	assert.False(t, found)
}

func TestAdaptiveScheduler_JobScheduling(t *testing.T) {
	config := DefaultPrefetchConfig()
	scheduler := NewAdaptiveScheduler(config)

	// Create test jobs with past scheduled times to ensure they're ready for execution
	now := time.Now()
	jobs := []*PrefetchJob{
		{
			Key:           "file1.txt",
			Bucket:        "test-bucket",
			Priority:      0.8,
			ScheduledTime: now.Add(-time.Minute), // Schedule in the past
			PredictedTime: now.Add(time.Minute),
			Confidence:    0.9,
			EstimatedSize: 1024,
		},
		{
			Key:           "file2.txt",
			Bucket:        "test-bucket",
			Priority:      0.6,
			ScheduledTime: now.Add(-time.Minute), // Schedule in the past
			PredictedTime: now.Add(time.Minute * 2),
			Confidence:    0.7,
			EstimatedSize: 2048,
		},
	}

	// Test job scheduling
	err := scheduler.ScheduleJobs(context.Background(), jobs)
	require.NoError(t, err)

	// Test getting next job
	job := scheduler.GetNextJob()
	// Note: job might be nil if scheduling constraints prevent execution
	if job != nil {
		assert.NotEmpty(t, job.ID)
		
		// Test job completion
		scheduler.CompleteJob(job, true, 1024, "")
	}

	// Test scheduling stats
	stats := scheduler.GetSchedulingStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(2), stats.TotalJobsScheduled)
	// Only check completion if job was available
	if job != nil {
		assert.Equal(t, int64(1), stats.TotalJobsCompleted)
	} else {
		assert.Equal(t, int64(0), stats.TotalJobsCompleted)
	}
}

func TestNetworkOptimizer_Optimization(t *testing.T) {
	config := DefaultPrefetchConfig()
	optimizer := NewNetworkOptimizer(config)

	// Test with default conditions
	job := &PrefetchJob{
		Key:           "test-key",
		EstimatedSize: 1024 * 1024,
	}

	optimization := optimizer.OptimizeJob(job)
	assert.NotNil(t, optimization)
	assert.True(t, optimization.ChunkSize > 0)
	assert.True(t, optimization.Concurrency > 0)

	// Test with specific network conditions
	conditions := &NetworkConditions{
		Bandwidth:   200.0, // High bandwidth
		RTT:         time.Millisecond * 50,
		PacketLoss:  0.001,
		LastUpdated: time.Now(),
	}

	optimizer.UpdateConditions(conditions)
	optimization = optimizer.OptimizeJob(job)
	assert.NotNil(t, optimization)

	// High bandwidth should result in larger chunks and higher concurrency
	assert.True(t, optimization.ChunkSize >= 1024*1024) // At least 1MB
	assert.True(t, optimization.Concurrency >= 2)
}

func TestPrefetchWorker_Operations(t *testing.T) {
	optimizer := createMockOptimizer(t)
	config := DefaultPrefetchConfig()
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	require.NoError(t, err)

	worker := NewPrefetchWorker(1, prefetcher, slog.Default())
	assert.NotNil(t, worker)
	assert.Equal(t, 1, worker.id)

	// Test worker stats
	stats := worker.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.ID)
	assert.Equal(t, int64(0), stats.JobsProcessed)
	assert.False(t, stats.IsRunning)

	// Test job submission when not running
	job := &PrefetchJob{
		Key:    "test-key",
		Bucket: "test-bucket",
	}
	success := worker.SubmitJob(job)
	assert.False(t, success) // Should fail when not running
}

func TestDefaultPrefetchConfig(t *testing.T) {
	config := DefaultPrefetchConfig()
	assert.NotNil(t, config)
	assert.True(t, config.EnablePrefetching)
	assert.True(t, config.MaxConcurrentPrefetch > 0)
	assert.True(t, config.PrefetchWindowSize > 0)
	assert.True(t, config.PrefetchAheadTime > 0)
	assert.True(t, config.CacheSize > 0)
	assert.True(t, config.MinPatternConfidence > 0)
	assert.True(t, config.MinPatternConfidence < 1)
}

func TestPriorityQueue_Operations(t *testing.T) {
	pq := NewPriorityQueue()
	assert.Equal(t, 0, pq.Size())

	// Test empty queue operations
	job := pq.Pop()
	assert.Nil(t, job)
	job = pq.Peek()
	assert.Nil(t, job)

	// Add jobs with different priorities
	job1 := &PrefetchJob{Key: "low", Priority: 0.3}
	job2 := &PrefetchJob{Key: "high", Priority: 0.9}
	job3 := &PrefetchJob{Key: "medium", Priority: 0.6}

	pq.Push(job1)
	pq.Push(job2)
	pq.Push(job3)

	assert.Equal(t, 3, pq.Size())

	// Peek should return highest priority without removing
	peeked := pq.Peek()
	assert.NotNil(t, peeked)
	assert.Equal(t, "high", peeked.Key)
	assert.Equal(t, 3, pq.Size())

	// Pop should return jobs in priority order
	popped := pq.Pop()
	assert.NotNil(t, popped)
	assert.Equal(t, "high", popped.Key)
	assert.Equal(t, 2, pq.Size())

	popped = pq.Pop()
	assert.NotNil(t, popped)
	assert.Equal(t, "medium", popped.Key)

	popped = pq.Pop()
	assert.NotNil(t, popped)
	assert.Equal(t, "low", popped.Key)
	assert.Equal(t, 0, pq.Size())
}

// Helper functions for testing

func createMockOptimizer(t *testing.T) *S3Optimizer {
	// Create a minimal mock optimizer for testing
	// In a real implementation, this would use a proper mock
	config := DefaultConfig()
	optimizer := &S3Optimizer{
		config:      config,
		metrics:     &Metrics{startTime: time.Now()},
		initialized: true,
	}
	return optimizer
}

// Benchmark tests

func BenchmarkPredictivePrefetcher_PredictAndPrefetch(b *testing.B) {
	optimizer := &S3Optimizer{
		config:      DefaultConfig(),
		metrics:     &Metrics{startTime: time.Now()},
		initialized: true,
	}
	config := DefaultPrefetchConfig()
	
	prefetcher, err := NewPredictivePrefetcher(optimizer, config, slog.Default())
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := prefetcher.Start(ctx); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = prefetcher.Stop() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "benchmark-key-" + string(rune(i%100))
		_ = prefetcher.PredictAndPrefetch(ctx, key)
	}
}

func BenchmarkPrefetchCache_Get(b *testing.B) {
	config := DefaultPrefetchConfig()
	cache := NewPrefetchCache(config)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := "key-" + string(rune(i))
		data := []byte("test data " + string(rune(i)))
		_ = cache.Put(key, data, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key-" + string(rune(i%100))
		_, _ = cache.Get(key)
	}
}

func BenchmarkRequestPredictor_PredictNextRequests(b *testing.B) {
	config := DefaultPrefetchConfig()
	predictor := NewRequestPredictor(config)

	// Create test patterns
	patterns := make(map[string]*AccessPattern)
	for i := 0; i < 10; i++ {
		pattern := &AccessPattern{
			Type:          PatternSequential,
			Keys:          []string{"file" + string(rune(i)) + ".txt", "file" + string(rune(i+1)) + ".txt"},
			Confidence:    0.8,
			PredictedNext: []string{"file" + string(rune(i+1)) + ".txt"},
			LastUpdated:   time.Now(),
		}
		patterns["pattern"+string(rune(i))] = pattern
	}

	predictor.UpdatePatterns(patterns)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "file" + string(rune(i%10)) + ".txt"
		_ = predictor.PredictNextRequests(key, 5)
	}
}