package pipeline

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdaptiveWorkerPool_BasicOperation(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     16,
		EnableAdaptive: false, // Disable adaptive for deterministic test
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	// Submit some work
	var completed int32
	for i := 0; i < 10; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&completed, 1)
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	pool.Wait()

	if completed != 10 {
		t.Errorf("Expected 10 completed tasks, got %d", completed)
	}

	if pool.GetWorkerCount() != 4 {
		t.Errorf("Expected 4 workers, got %d", pool.GetWorkerCount())
	}
}

func TestAdaptiveWorkerPool_DefaultConfiguration(t *testing.T) {
	ctx := context.Background()
	pool := NewAdaptiveWorkerPool(ctx, nil) // Use defaults
	defer pool.Stop()

	// Check defaults
	if pool.initialWorkers != runtime.NumCPU() {
		t.Errorf("Expected initialWorkers=%d, got %d", runtime.NumCPU(), pool.initialWorkers)
	}

	if pool.maxWorkers != 256 {
		t.Errorf("Expected maxWorkers=256, got %d", pool.maxWorkers)
	}

	if pool.minWorkers != 2 {
		t.Errorf("Expected minWorkers=2, got %d", pool.minWorkers)
	}

	if pool.monitorPeriod != 4*time.Second {
		t.Errorf("Expected monitorPeriod=4s, got %v", pool.monitorPeriod)
	}

	if pool.scalingFactor != runtime.GOMAXPROCS(0) {
		t.Errorf("Expected scalingFactor=%d, got %d", runtime.GOMAXPROCS(0), pool.scalingFactor)
	}

	if !pool.enableAdaptive {
		t.Error("Expected adaptive scaling to be enabled by default")
	}
}

func TestAdaptiveWorkerPool_AdaptiveScaling(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 2,
		MaxWorkers:     16,
		MonitorPeriod:  100 * time.Millisecond, // Fast monitoring for test
		ScalingFactor:  2,
		EnableAdaptive: true,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	initialWorkers := pool.GetWorkerCount()
	if initialWorkers != 2 {
		t.Errorf("Expected 2 initial workers, got %d", initialWorkers)
	}

	// Submit work that generates throughput
	var completed int32
	for i := 0; i < 100; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			pool.AddBytes(1024 * 1024) // Simulate 1MB processed
			atomic.AddInt32(&completed, 1)
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	// Wait for adaptive scaling to kick in
	time.Sleep(500 * time.Millisecond)

	// Check that workers have scaled up
	currentWorkers := pool.GetWorkerCount()
	if currentWorkers <= initialWorkers {
		t.Errorf("Expected workers to scale up from %d, got %d", initialWorkers, currentWorkers)
	}

	t.Logf("Workers scaled from %d to %d", initialWorkers, currentWorkers)

	pool.Wait()

	if completed != 100 {
		t.Errorf("Expected 100 completed tasks, got %d", completed)
	}
}

func TestAdaptiveWorkerPool_ThroughputMonitoring(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     16,
		MonitorPeriod:  100 * time.Millisecond,
		EnableAdaptive: true,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	// Submit work that generates measurable throughput
	for i := 0; i < 50; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			pool.AddBytes(10 * 1024 * 1024) // 10MB per task
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	// Wait for throughput to accumulate
	time.Sleep(200 * time.Millisecond)

	totalBytes := pool.GetTotalBytes()
	if totalBytes == 0 {
		t.Error("Expected non-zero bytes processed")
	}

	t.Logf("Total bytes processed: %d", totalBytes)

	pool.Wait()
}

func TestAdaptiveWorkerPool_MaxWorkerLimit(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 2,
		MaxWorkers:     8,
		MonitorPeriod:  50 * time.Millisecond,
		ScalingFactor:  4,
		EnableAdaptive: true,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	// Submit long-running work to trigger scaling
	for i := 0; i < 100; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			pool.AddBytes(1024 * 1024) // 1MB per task
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	// Wait for scaling to reach maximum
	time.Sleep(300 * time.Millisecond)

	workers := pool.GetWorkerCount()
	if workers > 8 {
		t.Errorf("Expected max 8 workers, got %d", workers)
	}

	t.Logf("Workers at max limit: %d", workers)

	pool.Wait()
}

func TestAdaptiveWorkerPool_PlateauDetection(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 2,
		MaxWorkers:     64,
		MonitorPeriod:  100 * time.Millisecond,
		ScalingFactor:  2,
		EnableAdaptive: true,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	// Submit work with consistent throughput (should plateau)
	workersAtStart := pool.GetWorkerCount()

	for i := 0; i < 50; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			pool.AddBytes(1024 * 1024) // Consistent 1MB per task
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	// Wait for monitoring cycles (3 cycles × 100ms = 300ms)
	time.Sleep(400 * time.Millisecond)

	workersAfterPlateau := pool.GetWorkerCount()

	// After plateau detection (2 retries), scaling should stop
	// Workers should have increased but not reached maximum
	if workersAfterPlateau <= workersAtStart {
		t.Logf("Note: Workers did not scale up (may be expected if throughput plateaued immediately)")
	}

	if workersAfterPlateau >= 64 {
		t.Errorf("Expected workers to stop scaling before max (64), got %d", workersAfterPlateau)
	}

	t.Logf("Workers: start=%d, after plateau=%d", workersAtStart, workersAfterPlateau)

	pool.Wait()
}

func TestAdaptiveWorkerPool_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     16,
		EnableAdaptive: false,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)

	// Submit some work
	var completed int32
	for i := 0; i < 20; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				atomic.AddInt32(&completed, 1)
				return nil
			}
		})
	}

	// Cancel context after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	pool.Stop()

	// Some tasks should have completed, but not all due to cancellation
	completedCount := atomic.LoadInt32(&completed)
	t.Logf("Completed %d tasks before cancellation", completedCount)

	if completedCount >= 20 {
		t.Error("Expected some tasks to be cancelled")
	}
}

func TestAdaptiveWorkerPool_ConcurrentSubmit(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 8,
		MaxWorkers:     32,
		EnableAdaptive: false,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	// Submit work from multiple goroutines concurrently
	var completed int32
	const numGoroutines = 10
	const tasksPerGoroutine = 20

	var submitWg sync.WaitGroup
	submitWg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer submitWg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				_ = pool.Submit(func(ctx context.Context) error {
					time.Sleep(5 * time.Millisecond)
					atomic.AddInt32(&completed, 1)
					return nil
				})
			}
		}()
	}

	// Wait for all submits to complete before waiting for workers
	submitWg.Wait()
	pool.Wait()

	expectedTotal := int32(numGoroutines * tasksPerGoroutine)
	if completed != expectedTotal {
		t.Errorf("Expected %d completed tasks, got %d", expectedTotal, completed)
	}
}

func TestAdaptiveWorkerPool_BytesCounting(t *testing.T) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     16,
		EnableAdaptive: false,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	const bytesPerTask = 1024 * 1024 // 1MB
	const numTasks = 50

	for i := 0; i < numTasks; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			pool.AddBytes(bytesPerTask)
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to submit work: %v", err)
		}
	}

	pool.Wait()

	expectedBytes := int64(numTasks * bytesPerTask)
	actualBytes := pool.GetTotalBytes()

	if actualBytes != expectedBytes {
		t.Errorf("Expected %d bytes, got %d", expectedBytes, actualBytes)
	}
}

func BenchmarkAdaptiveWorkerPool_Submit(b *testing.B) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: runtime.NumCPU(),
		MaxWorkers:     256,
		EnableAdaptive: false, // Disable for consistent benchmark
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			// Minimal work
			return nil
		})
	}

	pool.Wait()
}

func BenchmarkAdaptiveWorkerPool_SubmitWithWork(b *testing.B) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: runtime.NumCPU(),
		MaxWorkers:     256,
		EnableAdaptive: false,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			// Simulate some work
			time.Sleep(100 * time.Microsecond)
			pool.AddBytes(1024)
			return nil
		})
	}

	pool.Wait()
}

func BenchmarkAdaptiveWorkerPool_AdaptiveScaling(b *testing.B) {
	ctx := context.Background()
	config := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     256,
		MonitorPeriod:  100 * time.Millisecond,
		ScalingFactor:  runtime.GOMAXPROCS(0),
		EnableAdaptive: true,
	}

	pool := NewAdaptiveWorkerPool(ctx, config)
	defer pool.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			time.Sleep(100 * time.Microsecond)
			pool.AddBytes(1024 * 1024) // 1MB
			return nil
		})
	}

	pool.Wait()

	b.Logf("Final worker count: %d", pool.GetWorkerCount())
	b.Logf("Total bytes processed: %d", pool.GetTotalBytes())
}
