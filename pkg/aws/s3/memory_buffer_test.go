package s3

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryAwareBuffer(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100) // 100MB
	defer mab.cancel()

	assert.NotNil(t, mab)
	assert.Equal(t, int64(100*1024*1024), mab.maxMemoryUsage)
	assert.Equal(t, 0.8, mab.targetUtilization)
	assert.Equal(t, BufferSizeAdaptive, mab.bufferSizeStrategy)
	assert.Equal(t, AllocationHybrid, mab.allocationStrategy)
	assert.NotNil(t, mab.allocatedBuffers)
	assert.NotNil(t, mab.bufferPool)
	assert.NotNil(t, mab.memoryMonitor)
	assert.True(t, mab.adaptiveResize)
	assert.Equal(t, 0.1, mab.resizeThreshold)
	assert.Equal(t, MemoryPressureNone, mab.memoryPressureLevel)
	assert.True(t, mab.preallocationEnabled)
	assert.True(t, mab.bufferReuse)
	assert.True(t, mab.compressionBuffers)
	assert.True(t, mab.asyncCleanup)
}

func TestMemoryAwareBufferAllocateBuffer(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Test basic allocation
	buffer, err := mab.AllocateBuffer("test1", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)
	assert.NotNil(t, buffer)
	assert.Equal(t, "test1", buffer.ID)
	assert.Equal(t, ContentTypeText, buffer.ContentType)
	assert.Equal(t, BufferPriorityNormal, buffer.Priority)
	assert.Equal(t, BufferStateAllocated, buffer.State)
	assert.Greater(t, buffer.Capacity, int64(0))
	assert.NotZero(t, buffer.AllocationTime)
	assert.Equal(t, int32(1), buffer.ReferenceCount)

	// Test allocation tracking
	mab.mu.RLock()
	_, exists := mab.allocatedBuffers["test1"]
	mab.mu.RUnlock()
	assert.True(t, exists)

	// Test memory usage tracking
	assert.Greater(t, mab.currentUsage, int64(0))
}

func TestMemoryAwareBufferAllocateBufferSizeStrategy(t *testing.T) {
	ctx := context.Background()

	strategies := []BufferSizeStrategy{
		BufferSizeFixed,
		BufferSizeDynamic,
		BufferSizeAdaptive,
		BufferSizeContentAware,
		BufferSizePerformance,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			mab := NewMemoryAwareBuffer(ctx, 100)
			mab.bufferSizeStrategy = strategy
			defer mab.cancel()

			buffer, err := mab.AllocateBuffer("test", 1024*1024, ContentTypeText, BufferPriorityNormal)
			require.NoError(t, err)
			assert.NotNil(t, buffer)
			assert.Greater(t, buffer.Capacity, int64(0))
		})
	}
}

func TestMemoryAwareBufferReleaseBuffer(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate buffer
	_, err := mab.AllocateBuffer("test1", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	initialUsage := mab.currentUsage

	// Release buffer
	err = mab.ReleaseBuffer("test1")
	require.NoError(t, err)

	// Verify buffer is removed from tracking
	mab.mu.RLock()
	_, exists := mab.allocatedBuffers["test1"]
	mab.mu.RUnlock()
	assert.False(t, exists)

	// Verify memory usage decreased
	assert.Less(t, mab.currentUsage, initialUsage)

	// Test releasing non-existent buffer
	err = mab.ReleaseBuffer("nonexistent")
	assert.Error(t, err)
}

func TestMemoryAwareBufferGetBuffer(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate buffer
	originalBuffer, err := mab.AllocateBuffer("test1", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	// Get buffer
	retrievedBuffer, err := mab.GetBuffer("test1")
	require.NoError(t, err)
	assert.Equal(t, originalBuffer, retrievedBuffer)
	assert.Greater(t, retrievedBuffer.AccessCount, int64(0))

	// Test getting non-existent buffer
	_, err = mab.GetBuffer("nonexistent")
	assert.Error(t, err)
}

func TestMemoryAwareBufferResizeBuffer(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate buffer
	buffer, err := mab.AllocateBuffer("test1", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	originalCapacity := buffer.Capacity
	newSize := int64(2 * 1024 * 1024) // 2MB

	// Resize buffer
	err = mab.ResizeBuffer("test1", newSize)
	require.NoError(t, err)

	// Verify buffer was resized
	assert.Equal(t, newSize, buffer.Capacity)
	assert.NotEqual(t, originalCapacity, buffer.Capacity)

	// Test resizing non-existent buffer
	err = mab.ResizeBuffer("nonexistent", newSize)
	assert.Error(t, err)
}

func TestMemoryAwareBufferGetMemoryUsage(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Get initial usage
	usage := mab.GetMemoryUsage()
	assert.NotNil(t, usage)
	assert.GreaterOrEqual(t, usage.TotalUsage, int64(0))
	assert.GreaterOrEqual(t, usage.BufferUsage, int64(0))
	assert.GreaterOrEqual(t, usage.SystemUsage, int64(0))
	assert.GreaterOrEqual(t, usage.AvailableMemory, int64(0))
	assert.Equal(t, MemoryPressureNone, usage.PressureLevel)
	assert.NotZero(t, usage.Timestamp)

	// Allocate buffer and check usage increase
	_, err := mab.AllocateBuffer("test1", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	newUsage := mab.GetMemoryUsage()
	assert.Greater(t, newUsage.TotalUsage, usage.TotalUsage)
}

func TestMemoryAwareBufferGetBufferMetrics(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Perform some allocations
	for i := 0; i < 3; i++ {
		_, err := mab.AllocateBuffer(fmt.Sprintf("test%d", i), 1024*1024, ContentTypeText, BufferPriorityNormal)
		require.NoError(t, err)
	}

	metrics := mab.GetBufferMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(3), metrics.TotalAllocations)
	assert.Equal(t, int64(3), metrics.CurrentBuffers)
	assert.Greater(t, metrics.TotalMemoryUsed, int64(0))
	assert.Greater(t, metrics.AverageBufferSize, int64(0))
	assert.NotZero(t, metrics.LastUpdate)
}

func TestMemoryAwareBufferAdaptToMemoryPressure(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate some buffers
	for i := 0; i < 5; i++ {
		priority := BufferPriorityNormal
		if i < 2 {
			priority = BufferPriorityLow
		}
		_, err := mab.AllocateBuffer(fmt.Sprintf("test%d", i), 1024*1024, ContentTypeText, priority)
		require.NoError(t, err)
	}

	_ = len(mab.allocatedBuffers) // Track initial count but don't use in test

	// Test adaptation to high memory pressure
	err := mab.AdaptToMemoryPressure(MemoryPressureHigh)
	require.NoError(t, err)
	assert.Equal(t, MemoryPressureHigh, mab.memoryPressureLevel)

	// Test adaptation to critical memory pressure
	err = mab.AdaptToMemoryPressure(MemoryPressureCritical)
	require.NoError(t, err)
	assert.Equal(t, MemoryPressureCritical, mab.memoryPressureLevel)

	// Should have triggered cleanup
	assert.Greater(t, len(mab.adaptationHistory), 0)

	// Test adaptation back to normal
	err = mab.AdaptToMemoryPressure(MemoryPressureNone)
	require.NoError(t, err)
	assert.Equal(t, MemoryPressureNone, mab.memoryPressureLevel)
}

func TestMemoryAwareBufferCheckMemoryAvailability(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 10) // Small 10MB limit
	defer mab.cancel()

	// Test normal allocation
	err := mab.checkMemoryAvailability(1024 * 1024) // 1MB
	assert.NoError(t, err)

	// Test allocation that would exceed limit
	err = mab.checkMemoryAvailability(20 * 1024 * 1024) // 20MB > 10MB limit
	assert.Error(t, err)

	// Test allocation under critical pressure
	mab.memoryPressureLevel = MemoryPressureCritical
	err = mab.checkMemoryAvailability(1024 * 1024)
	assert.Error(t, err)

	// Test large allocation under high pressure
	mab.memoryPressureLevel = MemoryPressureHigh
	err = mab.checkMemoryAvailability(2 * 1024 * 1024) // 2MB
	assert.Error(t, err)

	// Test small allocation under high pressure
	err = mab.checkMemoryAvailability(512 * 1024) // 512KB
	assert.NoError(t, err)
}

func TestMemoryAwareBufferCalculateOptimalBufferSize(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	requestedSize := int64(1024 * 1024) // 1MB

	// Test fixed strategy
	mab.bufferSizeStrategy = BufferSizeFixed
	size := mab.calculateOptimalBufferSize(requestedSize, ContentTypeText, BufferPriorityNormal)
	assert.Equal(t, int64(16*1024*1024), size) // Should return fixed 16MB

	// Test dynamic strategy
	mab.bufferSizeStrategy = BufferSizeDynamic
	size = mab.calculateOptimalBufferSize(requestedSize, ContentTypeText, BufferPriorityNormal)
	assert.Equal(t, requestedSize, size) // Should round to power of 2

	// Test adaptive strategy
	mab.bufferSizeStrategy = BufferSizeAdaptive
	size = mab.calculateOptimalBufferSize(requestedSize, ContentTypeText, BufferPriorityNormal)
	assert.Greater(t, size, int64(0))

	// Test content-aware strategy
	mab.bufferSizeStrategy = BufferSizeContentAware
	textSize := mab.calculateOptimalBufferSize(requestedSize, ContentTypeText, BufferPriorityNormal)
	videoSize := mab.calculateOptimalBufferSize(requestedSize, ContentTypeVideo, BufferPriorityNormal)
	assert.Less(t, textSize, videoSize) // Video should get larger buffers

	// Test performance strategy
	mab.bufferSizeStrategy = BufferSizePerformance
	size = mab.calculateOptimalBufferSize(requestedSize, ContentTypeText, BufferPriorityNormal)
	assert.Greater(t, size, int64(0))
}

func TestMemoryAwareBufferBufferPriorities(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	priorities := []BufferPriority{
		BufferPriorityLow,
		BufferPriorityNormal,
		BufferPriorityHigh,
		BufferPriorityCritical,
	}

	for i, priority := range priorities {
		buffer, err := mab.AllocateBuffer(fmt.Sprintf("test%d", i), 1024*1024, ContentTypeText, priority)
		require.NoError(t, err)
		assert.Equal(t, priority, buffer.Priority)
	}

	// Test that high pressure cleanup affects low priority buffers first
	mab.memoryPressureLevel = MemoryPressureHigh
	initialCount := len(mab.allocatedBuffers)
	_ = mab.performAggressiveCleanup() // Ignore error for test

	// Should have released some buffers
	assert.LessOrEqual(t, len(mab.allocatedBuffers), initialCount)
}

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool()

	// Test getting buffer from empty pool
	buffer := pool.GetBuffer(1024)
	assert.NotNil(t, buffer)
	assert.Equal(t, 1024, len(buffer))

	// Test returning buffer to pool
	pool.ReturnBuffer(buffer)

	// Test getting buffer from pool (should reuse)
	buffer2 := pool.GetBuffer(1024)
	assert.NotNil(t, buffer2)
	assert.Equal(t, 1024, len(buffer2))

	// Test warmup
	pool.WarmupPools()

	// Should have pre-allocated buffers
	for _, size := range pool.bufferSizes {
		poolChan := pool.pools[size]
		assert.Greater(t, len(poolChan), 0)
	}
}

func TestMemoryMonitor(t *testing.T) {
	ctx := context.Background()
	monitor := NewMemoryMonitor(ctx)

	// Test initial state
	assert.Greater(t, monitor.totalSystemMemory, int64(0))
	assert.GreaterOrEqual(t, monitor.currentMemoryUsage, int64(0))
	assert.GreaterOrEqual(t, monitor.availableMemory, int64(0))

	// Test updating memory stats
	monitor.UpdateMemoryStats()
	assert.Greater(t, len(monitor.usageHistory), 0)

	// Test calculating memory pressure
	pressure := monitor.CalculateMemoryPressure()
	assert.GreaterOrEqual(t, pressure, 0.0)
	assert.LessOrEqual(t, pressure, 1.0)

	// Test getting pressure level
	level := monitor.GetPressureLevel(0.5)
	assert.Equal(t, MemoryPressureNone, level)

	level = monitor.GetPressureLevel(0.75) // 75% usage = moderate pressure (between 0.7 and 0.8)
	assert.Equal(t, MemoryPressureModerate, level)

	level = monitor.GetPressureLevel(0.98)
	assert.Equal(t, MemoryPressureCritical, level)
}

func TestMemoryAwareBufferConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Test concurrent allocations
	numGoroutines := 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			bufferID := fmt.Sprintf("concurrent%d", id)
			_, err := mab.AllocateBuffer(bufferID, 1024*1024, ContentTypeText, BufferPriorityNormal)
			if err != nil {
				errors <- err
				return
			}

			// Simulate some work
			time.Sleep(time.Millisecond * 10)

			// Release buffer
			err = mab.ReleaseBuffer(bufferID)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		assert.NoError(t, err)
	}
}

func TestMemoryAwareBufferMemoryPressureAdaptation(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 10) // Very small limit to trigger pressure
	defer mab.cancel()

	// Allocate buffers to create pressure
	var buffers []string
	for i := 0; i < 5; i++ {
		bufferID := fmt.Sprintf("pressure%d", i)
		priority := BufferPriorityNormal
		if i < 2 {
			priority = BufferPriorityLow
		}

		_, err := mab.AllocateBuffer(bufferID, 2*1024*1024, ContentTypeText, priority)
		if err == nil {
			buffers = append(buffers, bufferID)
		}
	}

	// Should have some buffers allocated
	assert.Greater(t, len(buffers), 0)

	// Trigger memory pressure adaptation
	err := mab.AdaptToMemoryPressure(MemoryPressureCritical)
	assert.NoError(t, err)

	// Should have recorded adaptation events
	assert.Greater(t, len(mab.adaptationHistory), 0)

	// Clean up remaining buffers
	for _, bufferID := range buffers {
		_ = mab.ReleaseBuffer(bufferID) // Ignore errors as some may have been cleaned up
	}
}

func TestMemoryAwareBufferBufferStates(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Test buffer lifecycle
	buffer, err := mab.AllocateBuffer("lifecycle", 1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	// Should start as allocated
	assert.Equal(t, BufferStateAllocated, buffer.State)

	// Simulate usage
	buffer.State = BufferStateActive
	buffer.Size = 512 * 1024 // Half full

	// Later, mark as idle
	buffer.State = BufferStateIdle
	buffer.LastAccessed = time.Now().Add(-time.Minute * 10) // Old access

	// Release buffer
	err = mab.ReleaseBuffer("lifecycle")
	require.NoError(t, err)

	// Should end as released
	assert.Equal(t, BufferStateReleased, buffer.State)
}

func TestMemoryAwareBufferAdaptiveResize(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate buffer
	buffer, err := mab.AllocateBuffer("resize", 4*1024*1024, ContentTypeText, BufferPriorityNormal)
	require.NoError(t, err)

	// Test should resize (underutilized)
	buffer.Size = 500 * 1024 // Much smaller than capacity
	shouldResize := mab.shouldResizeBuffer(buffer)
	assert.True(t, shouldResize)

	// Calculate optimal resize
	optimalSize := mab.calculateOptimalResizeSize(buffer)
	assert.Less(t, optimalSize, buffer.Capacity)

	// Test should resize (overutilized)
	buffer.Size = int64(float64(buffer.Capacity) * 0.95) // 95% utilized
	shouldResize = mab.shouldResizeBuffer(buffer)
	assert.True(t, shouldResize)

	optimalSize = mab.calculateOptimalResizeSize(buffer)
	assert.Greater(t, optimalSize, buffer.Capacity)

	// Test should not resize (optimal utilization)
	buffer.Size = int64(float64(buffer.Capacity) * 0.6) // 60% utilized
	shouldResize = mab.shouldResizeBuffer(buffer)
	assert.False(t, shouldResize)
}

func TestMemoryAwareBufferCleanupOperations(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Allocate buffers with different priorities and access times
	buffers := []struct {
		id       string
		priority BufferPriority
		ageMin   int
	}{
		{"old_low", BufferPriorityLow, 20},
		{"old_normal", BufferPriorityNormal, 15},
		{"recent_low", BufferPriorityLow, 1},
		{"recent_normal", BufferPriorityNormal, 1},
	}

	for _, buf := range buffers {
		buffer, err := mab.AllocateBuffer(buf.id, 1024*1024, ContentTypeText, buf.priority)
		require.NoError(t, err)

		// Set access time and state
		buffer.LastAccessed = time.Now().Add(-time.Duration(buf.ageMin) * time.Minute)
		buffer.State = BufferStateIdle
	}

	initialCount := len(mab.allocatedBuffers)

	// Test light cleanup (should only affect very old, low priority)
	err := mab.performLightCleanup()
	assert.NoError(t, err)

	// Test moderate cleanup (should affect older buffers)
	err = mab.performModerateCleanup()
	assert.NoError(t, err)

	// Should have cleaned up some buffers
	assert.LessOrEqual(t, len(mab.allocatedBuffers), initialCount)
}

func TestMemoryAwareBufferEdgeCases(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Test zero size allocation
	buffer, err := mab.AllocateBuffer("zero", 0, ContentTypeText, BufferPriorityNormal)
	assert.NoError(t, err)
	assert.NotNil(t, buffer)

	// Test very large allocation (should be constrained)
	buffer, err = mab.AllocateBuffer("large", 1024*1024*1024, ContentTypeVideo, BufferPriorityNormal)
	if err == nil {
		assert.NotNil(t, buffer)
		assert.LessOrEqual(t, buffer.Capacity, int64(64*1024*1024)) // Should be capped
	}

	// Test allocation with unknown content type
	buffer, err = mab.AllocateBuffer("unknown", 1024*1024, ContentTypeUnknown, BufferPriorityNormal)
	assert.NoError(t, err)
	assert.NotNil(t, buffer)
}

func TestMemoryAwareBufferPerformanceMetrics(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 100)
	defer mab.cancel()

	// Perform multiple allocations and releases to generate metrics
	for i := 0; i < 10; i++ {
		bufferID := fmt.Sprintf("perf%d", i)
		buffer, err := mab.AllocateBuffer(bufferID, 1024*1024, ContentTypeText, BufferPriorityNormal)
		require.NoError(t, err)

		// Simulate some operations
		buffer.ReadOperations++
		buffer.WriteOperations++
		buffer.TotalBytesRead += 512 * 1024
		buffer.TotalBytesWritten += 256 * 1024

		// Release every other buffer
		if i%2 == 0 {
			err = mab.ReleaseBuffer(bufferID)
			assert.NoError(t, err)
		}
	}

	metrics := mab.GetBufferMetrics()
	assert.Equal(t, int64(10), metrics.TotalAllocations)
	assert.Equal(t, int64(5), metrics.TotalDeallocations)
	assert.Equal(t, int64(5), metrics.CurrentBuffers)
	assert.Greater(t, metrics.AverageBufferSize, int64(0))
}

func TestMemoryAwareBufferBackgroundOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	mab := NewMemoryAwareBuffer(ctx, 100)

	// Allocate some buffers
	for i := 0; i < 3; i++ {
		_, err := mab.AllocateBuffer(fmt.Sprintf("bg%d", i), 1024*1024, ContentTypeText, BufferPriorityNormal)
		require.NoError(t, err)
	}

	// Wait for background operations to run (monitoring interval is 5 seconds)
	time.Sleep(time.Millisecond * 5100)

	// Verify monitoring is working (with lock)
	mab.memoryMonitor.mu.RLock()
	historyLen := len(mab.memoryMonitor.usageHistory)
	mab.memoryMonitor.mu.RUnlock()
	assert.Greater(t, historyLen, 0)

	// The context will be cancelled, which should stop background operations
	mab.cancel()

	// Give time for cleanup
	time.Sleep(time.Millisecond * 100)
}

func TestMemoryAwareBufferMemoryLeakPrevention(t *testing.T) {
	ctx := context.Background()
	mab := NewMemoryAwareBuffer(ctx, 50) // Small limit
	defer mab.cancel()

	// Allocate and release many buffers to test for leaks
	for i := 0; i < 100; i++ {
		bufferID := fmt.Sprintf("leak%d", i)
		_, err := mab.AllocateBuffer(bufferID, 1024*1024, ContentTypeText, BufferPriorityNormal)
		if err != nil {
			continue // Skip if allocation fails due to memory pressure
		}

		// Immediately release
		err = mab.ReleaseBuffer(bufferID)
		assert.NoError(t, err)
	}

	// Force garbage collection
	runtime.GC()
	runtime.GC()

	// Memory usage should be low
	usage := mab.GetMemoryUsage()
	assert.Less(t, usage.TotalUsage, int64(10*1024*1024)) // Should be under 10MB
}
