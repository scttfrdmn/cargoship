package staging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleAdvancedStagingOptimizer(t *testing.T) {
	config := DefaultAdvancedOptimizationConfig()
	optimizer := NewSimpleAdvancedStagingOptimizer(config)
	require.NotNil(t, optimizer)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Test startup
	err := optimizer.Start(ctx)
	assert.NoError(t, err)

	// Test job submission
	job := &AdvancedStagingJob{
		ID:          "simple-test-job-1",
		Size:        1024 * 1024 * 25, // 25MB
		ContentType: "application/zip",
		Priority:    5,
		Deadline:    time.Now().Add(time.Minute * 30),
	}

	handle, err := optimizer.SubmitStagingJob(job)
	assert.NoError(t, err)
	assert.NotNil(t, handle)
	assert.Equal(t, job.ID, handle.JobID)

	// Test optimization state
	state := optimizer.GetOptimizationState()
	assert.NotNil(t, state)
	assert.True(t, state.TotalJobsProcessed >= 1)
	assert.True(t, state.OptimizationScore > 0)
	assert.True(t, state.CPUUtilization >= 0 && state.CPUUtilization <= 1)

	// Give some time for processing
	time.Sleep(time.Millisecond * 100)

	// Cleanup
	err = optimizer.Stop()
	assert.NoError(t, err)
}

func TestSimpleParallelEngine(t *testing.T) {
	engine := NewSimpleParallelEngine(4)
	require.NotNil(t, engine)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err := engine.Start(ctx)
	assert.NoError(t, err)

	// Submit a few jobs
	jobs := []*AdvancedStagingJob{
		{ID: "job1", Size: 1024 * 1024},
		{ID: "job2", Size: 2 * 1024 * 1024},
		{ID: "job3", Size: 5 * 1024 * 1024},
	}

	for _, job := range jobs {
		engine.SubmitJob(job)
	}

	// Give time for processing
	time.Sleep(time.Millisecond * 200)

	err = engine.Stop()
	assert.NoError(t, err)
}

func TestSimpleScheduler(t *testing.T) {
	scheduler := NewSimpleScheduler()
	require.NotNil(t, scheduler)

	// Test job addition
	job := &AdvancedStagingJob{
		ID:       "sched-simple-test",
		Size:     1024 * 1024,
		Priority: 5,
	}

	scheduler.AddJob(job)

	// Test metrics
	metrics := scheduler.GetSchedulingMetrics()
	assert.NotNil(t, metrics)
	assert.True(t, metrics.Efficiency > 0)

	// Test optimization
	scheduler.OptimizeSchedulingParameters()
}

func TestSimpleMemoryManager(t *testing.T) {
	memManager := NewSimpleMemoryManager()
	require.NotNil(t, memManager)

	// Test buffer operations
	sizes := []int{1024, 4096, 8192, 16384}
	buffers := make([][]byte, len(sizes))

	for i, size := range sizes {
		buffer := memManager.GetBuffer(size)
		assert.NotNil(t, buffer)
		assert.True(t, len(buffer) >= size)
		buffers[i] = buffer
	}

	// Return buffers
	for _, buffer := range buffers {
		memManager.ReturnBuffer(buffer)
	}

	// Test memory pressure handling
	memManager.HandleMemoryPressure(0.9)

	// Test metrics
	metrics := memManager.GetMemoryMetrics()
	assert.NotNil(t, metrics)
	assert.True(t, metrics.Efficiency > 0)

	// Test optimization
	memManager.OptimizeMemoryAllocation()
}

func TestSimplePredictor(t *testing.T) {
	predictor := NewSimplePredictor()
	require.NotNil(t, predictor)

	// Test prediction for different job sizes
	profiles := []*JobProfile{
		{
			JobID: "small-job",
			Size:  5 * 1024 * 1024, // 5MB
		},
		{
			JobID: "medium-job",
			Size:  50 * 1024 * 1024, // 50MB
		},
		{
			JobID: "large-job",
			Size:  500 * 1024 * 1024, // 500MB
		},
	}

	for _, profile := range profiles {
		prediction := predictor.PredictOptimalParameters(profile)
		assert.NotNil(t, prediction)
		assert.True(t, prediction.OptimalConcurrency > 0)
		assert.True(t, prediction.OptimalChunkSizeMB > 0)
		assert.NotEmpty(t, prediction.OptimalCompression)
		assert.True(t, prediction.Confidence > 0)
	}

	// Test model update
	data := &ComprehensiveMetrics{
		ThroughputScore:    0.8,
		NormalizedLatency:  0.3,
		ResourceEfficiency: 0.75,
	}

	predictor.UpdateModels(data)
}

func TestSimpleAdvancedStagingIntegration(t *testing.T) {
	config := DefaultAdvancedOptimizationConfig()
	config.WorkerPoolSize = 2
	config.MaxConcurrentJobs = 4

	optimizer := NewSimpleAdvancedStagingOptimizer(config)
	require.NotNil(t, optimizer)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	err := optimizer.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = optimizer.Stop() }()

	// Submit multiple jobs with different characteristics
	jobs := []*AdvancedStagingJob{
		{ID: "small-job", Size: 1 * 1024 * 1024, Priority: 3, ContentType: "text/plain"},
		{ID: "medium-job", Size: 25 * 1024 * 1024, Priority: 5, ContentType: "application/zip"},
		{ID: "large-job", Size: 100 * 1024 * 1024, Priority: 8, ContentType: "video/mp4"},
		{ID: "urgent-job", Size: 10 * 1024 * 1024, Priority: 10, ContentType: "application/json"},
	}

	handles := make([]*JobHandle, len(jobs))
	for i, job := range jobs {
		handle, err := optimizer.SubmitStagingJob(job)
		require.NoError(t, err)
		require.NotNil(t, handle)
		handles[i] = handle
	}

	// Wait for some processing
	time.Sleep(time.Second * 2)

	// Check final state
	state := optimizer.GetOptimizationState()
	assert.NotNil(t, state)
	assert.True(t, state.TotalJobsProcessed >= int64(len(jobs)))
	assert.True(t, state.OptimizationScore > 50) // Should be reasonably good
}

func BenchmarkSimpleAdvancedStaging(b *testing.B) {
	config := DefaultAdvancedOptimizationConfig()
	optimizer := NewSimpleAdvancedStagingOptimizer(config)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err := optimizer.Start(ctx)
	require.NoError(b, err)
	defer func() { _ = optimizer.Stop() }()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		jobID := 0
		for pb.Next() {
			job := &AdvancedStagingJob{
				ID:          generateBenchJobID(jobID),
				Size:        int64(1024 * 1024 * (5 + jobID%20)), // 5-25MB
				ContentType: "application/octet-stream",
				Priority:    (jobID % 10) + 1,
			}
			jobID++

			_, err := optimizer.SubmitStagingJob(job)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkSimpleMemoryManager(b *testing.B) {
	memManager := NewSimpleMemoryManager()
	sizes := []int{1024, 4096, 8192, 16384, 32768}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			size := sizes[b.N%len(sizes)]
			buffer := memManager.GetBuffer(size)
			memManager.ReturnBuffer(buffer)
		}
	})
}

func generateBenchJobID(id int) string {
	return "bench-job-" + time.Now().Format("150405") + "-" + string(rune('0'+id%10))
}