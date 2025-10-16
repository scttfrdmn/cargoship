package ioutils

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// TestNumaSupported verifies NUMA detection on the system
func TestNumaSupported(t *testing.T) {
	supported := NumaSupported()

	// On Linux with multiple NUMA nodes, this should be true
	// On other platforms or single-node systems, this should be false
	if runtime.GOOS == "linux" {
		t.Logf("NUMA supported: %v (this may vary by hardware)", supported)
	} else {
		if supported {
			t.Error("NUMA should not be supported on non-Linux platforms")
		}
	}
}

// TestGetNumaInfo verifies NUMA information retrieval
func TestGetNumaInfo(t *testing.T) {
	info := GetNumaInfo()

	// Verify basic sanity checks
	if info.NodeCount < 1 {
		t.Errorf("Expected at least 1 NUMA node, got %d", info.NodeCount)
	}

	if info.Enabled && info.NodeCount == 1 {
		t.Error("NUMA should not be enabled with only 1 node")
	}

	if !info.Enabled && info.NodeCount != 1 {
		t.Errorf("NUMA disabled but got %d nodes, expected 1", info.NodeCount)
	}

	t.Logf("NUMA Info: Enabled=%v, Nodes=%d, CPUs=%d",
		info.Enabled, info.NodeCount, info.CPUCount)
}

// TestGetCurrentNumaNode verifies current NUMA node detection
func TestGetCurrentNumaNode(t *testing.T) {
	node, err := GetCurrentNumaNode()
	if err != nil && NumaSupported() {
		t.Errorf("Failed to get current NUMA node: %v", err)
	}

	info := GetNumaInfo()
	if node < 0 || node >= info.NodeCount {
		t.Errorf("Invalid NUMA node %d, expected 0-%d", node, info.NodeCount-1)
	}

	t.Logf("Current NUMA node: %d", node)
}

// TestGetCurrentNumaNode_Consistency verifies node detection consistency
func TestGetCurrentNumaNode_Consistency(t *testing.T) {
	// Call multiple times and verify we get valid nodes
	for i := 0; i < 10; i++ {
		node, err := GetCurrentNumaNode()
		if err != nil && NumaSupported() {
			t.Errorf("Iteration %d: Failed to get current NUMA node: %v", i, err)
		}

		info := GetNumaInfo()
		if node < 0 || node >= info.NodeCount {
			t.Errorf("Iteration %d: Invalid NUMA node %d", i, node)
		}
	}
}

// TestAllocateNumaBuffer verifies NUMA buffer allocation
func TestAllocateNumaBuffer(t *testing.T) {
	sizes := []int{1024, 64 * 1024, 1024 * 1024}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			buf, err := AllocateNumaBuffer(size)
			if err != nil {
				t.Fatalf("Failed to allocate NUMA buffer of size %d: %v", size, err)
			}

			if buf == nil {
				t.Fatal("Got nil buffer")
			}

			if len(buf.Data) != size {
				t.Errorf("Expected buffer size %d, got %d", size, len(buf.Data))
			}

			info := GetNumaInfo()
			if buf.Node < 0 || buf.Node >= info.NodeCount {
				t.Errorf("Invalid NUMA node %d for buffer", buf.Node)
			}

			// Verify buffer is usable
			for i := range buf.Data {
				buf.Data[i] = byte(i % 256)
			}

			// Verify data integrity
			for i := range buf.Data {
				if buf.Data[i] != byte(i%256) {
					t.Errorf("Data corruption at index %d", i)
					break
				}
			}
		})
	}
}

// TestNumaBufferPool_Basic verifies basic buffer pool operations
func TestNumaBufferPool_Basic(t *testing.T) {
	bufferSize := 64 * 1024
	pool := NewNumaBufferPool(bufferSize)

	if pool.GetBufferSize() != bufferSize {
		t.Errorf("Expected buffer size %d, got %d", bufferSize, pool.GetBufferSize())
	}

	// Get a buffer
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Got nil buffer from pool")
	}

	if len(buf.Data) != bufferSize {
		t.Errorf("Expected buffer size %d, got %d", bufferSize, len(buf.Data))
	}

	// Write some data
	for i := range buf.Data {
		buf.Data[i] = 0xFF
	}

	// Return buffer to pool
	pool.Put(buf)

	// Get another buffer (should be cleared)
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Got nil buffer from pool")
	}

	// Verify buffer was cleared
	for i := range buf2.Data {
		if buf2.Data[i] != 0 {
			t.Errorf("Buffer not cleared at index %d", i)
			break
		}
	}

	pool.Put(buf2)
}

// TestNumaBufferPool_Concurrent verifies concurrent buffer pool operations
func TestNumaBufferPool_Concurrent(t *testing.T) {
	bufferSize := 64 * 1024
	pool := NewNumaBufferPool(bufferSize)

	numGoroutines := 100
	opsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch concurrent workers
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < opsPerGoroutine; j++ {
				// Get buffer
				buf := pool.Get()
				if buf == nil {
					t.Errorf("Worker %d: Got nil buffer", workerID)
					return
				}

				if len(buf.Data) != bufferSize {
					t.Errorf("Worker %d: Wrong buffer size", workerID)
					return
				}

				// Use buffer
				pattern := byte(workerID % 256)
				for k := range buf.Data {
					buf.Data[k] = pattern
				}

				// Verify data
				for k := range buf.Data {
					if buf.Data[k] != pattern {
						t.Errorf("Worker %d: Data corruption", workerID)
						return
					}
				}

				// Return buffer
				pool.Put(buf)
			}
		}(i)
	}

	wg.Wait()
}

// TestNumaBufferPool_MultipleNodes verifies buffer allocation across NUMA nodes
func TestNumaBufferPool_MultipleNodes(t *testing.T) {
	if !NumaSupported() {
		t.Skip("NUMA not supported on this system")
	}

	bufferSize := 64 * 1024
	pool := NewNumaBufferPool(bufferSize)

	info := GetNumaInfo()
	if info.NodeCount < 2 {
		t.Skip("System has fewer than 2 NUMA nodes")
	}

	// Collect buffers and check node distribution
	nodeCount := make(map[int]int)
	buffers := make([]*NumaBuffer, 100)

	for i := range buffers {
		buf := pool.Get()
		buffers[i] = buf
		nodeCount[buf.Node]++
	}

	// Return all buffers
	for _, buf := range buffers {
		pool.Put(buf)
	}

	// With a multi-node system, we should see some distribution
	// (though not guaranteed due to scheduler behavior)
	t.Logf("Buffer distribution across NUMA nodes: %v", nodeCount)

	if len(nodeCount) == 0 {
		t.Error("No buffers allocated on any NUMA node")
	}
}

// TestNumaBufferPool_NilPut verifies Put handles nil buffers gracefully
func TestNumaBufferPool_NilPut(t *testing.T) {
	bufferSize := 64 * 1024
	pool := NewNumaBufferPool(bufferSize)

	// Should not panic
	pool.Put(nil)
}

// TestNumaBufferPool_Reuse verifies buffer reuse from pool
func TestNumaBufferPool_Reuse(t *testing.T) {
	bufferSize := 1024
	pool := NewNumaBufferPool(bufferSize)

	// Get and modify a buffer
	buf1 := pool.Get()
	buf1.Data[0] = 0xAA
	buf1Addr := &buf1.Data[0]

	// Return it
	pool.Put(buf1)

	// Get another buffer - might be the same one
	buf2 := pool.Get()
	buf2Addr := &buf2.Data[0]

	// Check if reused (addresses match)
	if buf1Addr == buf2Addr {
		// Buffer was reused - verify it was cleared
		if buf2.Data[0] != 0 {
			t.Error("Reused buffer was not cleared")
		}
		t.Log("Buffer was successfully reused from pool")
	} else {
		t.Log("Got a new buffer from pool")
	}

	pool.Put(buf2)
}

// BenchmarkAllocateNumaBuffer benchmarks NUMA buffer allocation
func BenchmarkAllocateNumaBuffer(b *testing.B) {
	sizes := []int{4096, 64 * 1024, 1024 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf, err := AllocateNumaBuffer(size)
				if err != nil {
					b.Fatalf("Allocation failed: %v", err)
				}
				// Touch the memory to ensure it's allocated
				_ = buf.Data[0]
			}
		})
	}
}

// BenchmarkNumaBufferPool benchmarks buffer pool operations
func BenchmarkNumaBufferPool(b *testing.B) {
	sizes := []int{4096, 64 * 1024, 1024 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			pool := NewNumaBufferPool(size)
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf := pool.Get()
				// Touch the memory
				_ = buf.Data[0]
				pool.Put(buf)
			}
		})
	}
}

// BenchmarkNumaBufferPool_Parallel benchmarks parallel buffer pool operations
func BenchmarkNumaBufferPool_Parallel(b *testing.B) {
	sizes := []int{4096, 64 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			pool := NewNumaBufferPool(size)
			b.SetBytes(int64(size))
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					buf := pool.Get()
					// Touch the memory
					_ = buf.Data[0]
					pool.Put(buf)
				}
			})
		})
	}
}

// BenchmarkStandardAllocation benchmarks standard Go allocation for comparison
func BenchmarkStandardAllocation(b *testing.B) {
	sizes := []int{4096, 64 * 1024, 1024 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf := make([]byte, size)
				// Touch the memory
				_ = buf[0]
			}
		})
	}
}
