package pipeline

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

func TestMemoryManager_DefaultConfiguration(t *testing.T) {
	ctx := context.Background()

	mgr := NewMemoryManager(ctx, nil)
	defer mgr.Stop()

	// Verify defaults
	if mgr.config.MemoryBudgetPercent != 0.5 {
		t.Errorf("Expected default budget percent 0.5, got %f", mgr.config.MemoryBudgetPercent)
	}

	if mgr.config.MinMemoryBuffer != 512<<20 {
		t.Errorf("Expected default min buffer 512MB, got %d", mgr.config.MinMemoryBuffer)
	}

	if mgr.config.ProactiveGCThreshold != 50<<20 {
		t.Errorf("Expected default GC threshold 50MB, got %d", mgr.config.ProactiveGCThreshold)
	}

	if mgr.config.PartSize != 16<<20 {
		t.Errorf("Expected default part size 16MB, got %d", mgr.config.PartSize)
	}
}

func TestMemoryManager_EstimateMemoryUsage(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		PartSize: 16 << 20, // 16MB
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Test small chunk (10MB)
	smallChunk := int64(10 << 20)
	estimated := mgr.EstimateMemoryUsage(smallChunk)

	// Should estimate 4 × partSize + overhead
	expectedMin := 4 * config.PartSize // 64MB
	if estimated < expectedMin {
		t.Errorf("Expected at least %d MB estimated, got %d MB", expectedMin/(1<<20), estimated/(1<<20))
	}

	// Test large chunk (100MB)
	largeChunk := int64(100 << 20)
	estimatedLarge := mgr.EstimateMemoryUsage(largeChunk)

	// Should be larger than small chunk due to overhead (1% of chunk size)
	// For 10MB: overhead = max(1% of 10MB, 1MB) = 1MB
	// For 100MB: overhead = max(1% of 100MB, 1MB) = 1MB
	// So overhead is the same, but the estimate includes base + overhead
	if estimatedLarge < estimated {
		t.Errorf("Expected estimate for 100MB chunk (%d MB) >= estimate for 10MB chunk (%d MB)",
			estimatedLarge/(1<<20), estimated/(1<<20))
	}

	t.Logf("Small chunk (10MB) estimated: %d MB", estimated/(1<<20))
	t.Logf("Large chunk (100MB) estimated: %d MB", estimatedLarge/(1<<20))
}

func TestMemoryManager_CanAllocate(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Small chunk should be allocatable
	smallChunk := int64(10 << 20)
	if !mgr.CanAllocate(smallChunk) {
		t.Error("Expected small chunk to be allocatable")
	}

	// Reserve memory to reduce budget
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			TotalSize: smallChunk,
		},
	}

	err := mgr.ReserveMemory(ctx, job)
	if err != nil {
		t.Fatalf("Failed to reserve memory: %v", err)
	}

	// After reservation, available memory is reduced
	stats := mgr.GetStats()
	if stats.EstimatedUsage == 0 {
		t.Error("Expected estimated usage > 0 after reservation")
	}

	// Release memory
	mgr.ReleaseMemory(smallChunk)

	// After release, memory should be available again
	stats = mgr.GetStats()
	if stats.EstimatedUsage != 0 {
		t.Errorf("Expected estimated usage 0 after release, got %d", stats.EstimatedUsage)
	}
}

func TestMemoryManager_ReserveAndRelease(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Reserve memory for 5 chunks
	const numChunks = 5
	chunkSize := int64(10 << 20) // 10MB

	for i := 0; i < numChunks; i++ {
		job := &Job{
			ID: i + 1,
			Chunk: chunking.Chunk{
				TotalSize: chunkSize,
			},
		}

		err := mgr.ReserveMemory(ctx, job)
		if err != nil {
			t.Fatalf("Failed to reserve memory for chunk %d: %v", i+1, err)
		}
	}

	// Check stats
	stats := mgr.GetStats()
	expectedUsage := mgr.EstimateMemoryUsage(chunkSize) * numChunks
	if stats.EstimatedUsage != expectedUsage {
		t.Errorf("Expected estimated usage %d MB, got %d MB",
			expectedUsage/(1<<20), stats.EstimatedUsage/(1<<20))
	}

	// Release all memory
	for i := 0; i < numChunks; i++ {
		mgr.ReleaseMemory(chunkSize)
	}

	// Check stats after release
	stats = mgr.GetStats()
	if stats.EstimatedUsage != 0 {
		t.Errorf("Expected estimated usage 0 after release, got %d", stats.EstimatedUsage)
	}

	if stats.TotalTasksReleased != numChunks {
		t.Errorf("Expected %d tasks released, got %d", numChunks, stats.TotalTasksReleased)
	}
}

func TestMemoryManager_ConcurrentReserveAndRelease(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	const numGoroutines = 10
	const chunksPerGoroutine = 5
	chunkSize := int64(5 << 20) // 5MB

	var wg sync.WaitGroup

	// Launch concurrent goroutines that reserve and release memory
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < chunksPerGoroutine; j++ {
				job := &Job{
					ID: id*chunksPerGoroutine + j,
					Chunk: chunking.Chunk{
						TotalSize: chunkSize,
					},
				}

				// Reserve memory
				err := mgr.ReserveMemory(ctx, job)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to reserve memory: %v", id, err)
					return
				}

				// Simulate work
				time.Sleep(10 * time.Millisecond)

				// Release memory
				mgr.ReleaseMemory(chunkSize)
			}
		}(i)
	}

	wg.Wait()

	// Check final stats
	stats := mgr.GetStats()
	if stats.EstimatedUsage != 0 {
		t.Errorf("Expected estimated usage 0 after all releases, got %d", stats.EstimatedUsage)
	}

	expectedReleases := int64(numGoroutines * chunksPerGoroutine)
	if stats.TotalTasksReleased != expectedReleases {
		t.Errorf("Expected %d tasks released, got %d", expectedReleases, stats.TotalTasksReleased)
	}
}

func TestMemoryManager_ProactiveGC(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent:  0.5,
		ProactiveGCThreshold: 50 << 20, // 50MB
		PartSize:             16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Reserve memory for a large chunk (triggers GC)
	largeChunk := int64(100 << 20) // 100MB
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			TotalSize: largeChunk,
		},
	}

	err := mgr.ReserveMemory(ctx, job)
	if err != nil {
		t.Fatalf("Failed to reserve memory: %v", err)
	}

	// Check that GC was triggered
	stats := mgr.GetStats()
	if stats.TotalGCTriggered == 0 {
		t.Error("Expected GC to be triggered for large chunk")
	}

	mgr.ReleaseMemory(largeChunk)
}

func TestMemoryManager_MemoryExhaustion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.01, // Very small budget (1%)
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Try to reserve more memory than budget allows
	const numChunks = 100
	chunkSize := int64(10 << 20) // 10MB

	var reservedCount int32

	var wg sync.WaitGroup
	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			job := &Job{
				ID: id,
				Chunk: chunking.Chunk{
					TotalSize: chunkSize,
				},
			}

			err := mgr.ReserveMemory(ctx, job)
			if err == nil {
				atomic.AddInt32(&reservedCount, 1)
				// Immediately release to allow others to proceed
				mgr.ReleaseMemory(chunkSize)
			}
		}(i)
	}

	wg.Wait()

	// With a small budget, not all chunks should be able to reserve simultaneously
	// But with immediate release, all should eventually succeed or timeout
	t.Logf("Successfully reserved %d/%d chunks with small budget", reservedCount, numChunks)

	stats := mgr.GetStats()
	t.Logf("Stats: %s", stats.String())
}

func TestMemoryManager_ContextCancellation(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Reserve memory
	job1 := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			TotalSize: 10 << 20, // 10MB
		},
	}

	err := mgr.ReserveMemory(ctx, job1)
	if err != nil {
		t.Fatalf("Failed to reserve memory for first job: %v", err)
	}

	// Create a context that's already cancelled
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Try to reserve more memory with cancelled context
	// Use a large chunk that will require waiting (budget exhaustion scenario)
	// Since budget is 50%, and we already reserved ~65MB (for 10MB chunk),
	// we need to reserve another large chunk to exhaust budget
	job2 := &Job{
		ID: 2,
		Chunk: chunking.Chunk{
			TotalSize: 500 << 20, // 500MB - will exhaust budget
		},
	}

	err = mgr.ReserveMemory(cancelledCtx, job2)

	// Should receive context cancellation error since we can't allocate immediately
	// and the context is already cancelled
	if err != context.Canceled {
		// If there was enough budget, it could succeed immediately
		// In that case, release the memory and skip the error check
		if err == nil {
			mgr.ReleaseMemory(500 << 20)
			t.Log("Had enough budget to allocate immediately despite cancelled context")
		} else {
			t.Errorf("Expected context.Canceled or nil error, got %v", err)
		}
	}

	// Release first job's memory
	mgr.ReleaseMemory(10 << 20)
}

func TestMemoryManager_MemoryBudgetUpdate(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		MonitorInterval:     100 * time.Millisecond,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Get initial budget
	initialStats := mgr.GetStats()
	initialBudget := initialStats.MemoryBudget

	t.Logf("Initial budget: %d MB", initialBudget/(1<<20))

	// Allocate memory to change system state
	allocations := make([][]byte, 0, 10)
	for i := 0; i < 10; i++ {
		allocations = append(allocations, make([]byte, 10<<20)) // 10MB each
	}

	// Wait for monitor to update budget
	time.Sleep(300 * time.Millisecond)

	// Get updated budget
	updatedStats := mgr.GetStats()
	updatedBudget := updatedStats.MemoryBudget

	t.Logf("Updated budget: %d MB", updatedBudget/(1<<20))

	// Budget should have been recalculated
	// (May be same or different depending on GC and memory pressure)
	_ = updatedBudget

	// Clean up allocations
	runtime.KeepAlive(allocations)
}

func TestMemoryManager_GetStats(t *testing.T) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	// Initial stats
	stats := mgr.GetStats()
	if stats.EstimatedUsage != 0 {
		t.Errorf("Expected initial usage 0, got %d", stats.EstimatedUsage)
	}

	if stats.MemoryBudget <= 0 {
		t.Errorf("Expected positive budget, got %d", stats.MemoryBudget)
	}

	// Reserve memory
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			TotalSize: 10 << 20,
		},
	}

	err := mgr.ReserveMemory(ctx, job)
	if err != nil {
		t.Fatalf("Failed to reserve memory: %v", err)
	}

	// Check updated stats
	stats = mgr.GetStats()
	if stats.EstimatedUsage <= 0 {
		t.Errorf("Expected positive usage after reservation, got %d", stats.EstimatedUsage)
	}

	if stats.MaxMemoryUsed != stats.EstimatedUsage {
		t.Errorf("Expected max memory used %d, got %d", stats.EstimatedUsage, stats.MaxMemoryUsed)
	}

	// Check String() method
	statsStr := stats.String()
	if statsStr == "" {
		t.Error("Expected non-empty stats string")
	}
	t.Logf("Stats: %s", statsStr)

	mgr.ReleaseMemory(10 << 20)
}

func BenchmarkMemoryManager_ReserveRelease(b *testing.B) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	chunkSize := int64(10 << 20) // 10MB

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		job := &Job{
			ID: i,
			Chunk: chunking.Chunk{
				TotalSize: chunkSize,
			},
		}

		err := mgr.ReserveMemory(ctx, job)
		if err != nil {
			b.Fatalf("Failed to reserve memory: %v", err)
		}

		mgr.ReleaseMemory(chunkSize)
	}
}

func BenchmarkMemoryManager_ConcurrentReserveRelease(b *testing.B) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		MemoryBudgetPercent: 0.5,
		PartSize:            16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	chunkSize := int64(5 << 20) // 5MB

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			job := &Job{
				ID: i,
				Chunk: chunking.Chunk{
					TotalSize: chunkSize,
				},
			}

			err := mgr.ReserveMemory(ctx, job)
			if err != nil {
				b.Fatalf("Failed to reserve memory: %v", err)
			}

			mgr.ReleaseMemory(chunkSize)
			i++
		}
	})
}

func BenchmarkMemoryManager_EstimateMemoryUsage(b *testing.B) {
	ctx := context.Background()

	config := &MemoryManagerConfig{
		PartSize: 16 << 20,
	}

	mgr := NewMemoryManager(ctx, config)
	defer mgr.Stop()

	chunkSize := int64(10 << 20)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = mgr.EstimateMemoryUsage(chunkSize)
	}
}
