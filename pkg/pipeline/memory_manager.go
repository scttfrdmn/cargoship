// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryManagerConfig configures the memory-aware task queueing system
type MemoryManagerConfig struct {
	MemoryBudgetPercent float64       // Percentage of available memory to use (default: 0.5 = 50%)
	MinMemoryBuffer     int64         // Minimum free memory to maintain (default: 512MB)
	ProactiveGCThreshold int64        // Chunk size threshold for proactive GC (default: 50MB)
	PartSize            int64         // S3 multipart upload part size (default: 16MB)
	MonitorInterval     time.Duration // Memory monitoring interval (default: 1s)
}

// MemoryManager provides memory-aware task queueing with proactive GC
type MemoryManager struct {
	config *MemoryManagerConfig

	// State
	ctx    context.Context
	cancel context.CancelFunc

	// Memory tracking
	estimatedUsage int64 // Estimated memory in use by queued tasks (atomic)
	memoryBudget   int64 // Total memory budget (atomic)

	// Task queue
	waitingTasks   int32         // Number of tasks waiting for memory (atomic)
	taskQueue      chan *Job     // Queue for tasks waiting for memory
	releaseSignal  chan struct{} // Signal to wake up waiting tasks

	// Monitoring
	monitorWg    sync.WaitGroup
	stopMonitor  chan struct{}

	// Statistics
	totalTasksQueued   int64
	totalTasksReleased int64
	totalGCTriggered   int64
	maxMemoryUsed      int64
}

// NewMemoryManager creates a new memory-aware task queueing manager
func NewMemoryManager(ctx context.Context, config *MemoryManagerConfig) *MemoryManager {
	if config == nil {
		config = &MemoryManagerConfig{}
	}

	// Set defaults
	if config.MemoryBudgetPercent <= 0 || config.MemoryBudgetPercent > 1.0 {
		config.MemoryBudgetPercent = 0.5 // Use 50% of available memory
	}
	if config.MinMemoryBuffer <= 0 {
		config.MinMemoryBuffer = 512 << 20 // 512MB minimum buffer
	}
	if config.ProactiveGCThreshold <= 0 {
		config.ProactiveGCThreshold = 50 << 20 // 50MB
	}
	if config.PartSize <= 0 {
		config.PartSize = 16 << 20 // 16MB (S3 minimum for multipart)
	}
	if config.MonitorInterval <= 0 {
		config.MonitorInterval = time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	mgr := &MemoryManager{
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		taskQueue:     make(chan *Job, 100), // Buffer for waiting tasks
		releaseSignal: make(chan struct{}, 1),
		stopMonitor:   make(chan struct{}),
	}

	// Calculate initial memory budget
	mgr.updateMemoryBudget()

	// Start memory monitoring
	mgr.startMonitoring()

	return mgr
}

// EstimateMemoryUsage estimates memory usage for a chunk
func (m *MemoryManager) EstimateMemoryUsage(chunkSize int64) int64 {
	// For multipart uploads, we need approximately 4 × partSize in memory:
	// - 1 × partSize for reading from disk
	// - 1 × partSize for compression buffer
	// - 2 × partSize for upload buffers and AWS SDK overhead
	//
	// For chunks smaller than partSize, we still need the partSize buffer
	estimatedSize := 4 * m.config.PartSize

	// Add overhead for chunk metadata and file descriptors
	overhead := chunkSize / 100 // 1% overhead
	if overhead < 1<<20 {
		overhead = 1 << 20 // Minimum 1MB overhead
	}

	return estimatedSize + overhead
}

// CanAllocate checks if there's enough memory available for a chunk
func (m *MemoryManager) CanAllocate(chunkSize int64) bool {
	estimatedMemory := m.EstimateMemoryUsage(chunkSize)
	currentUsage := atomic.LoadInt64(&m.estimatedUsage)
	budget := atomic.LoadInt64(&m.memoryBudget)

	return currentUsage+estimatedMemory <= budget
}

// ReserveMemory reserves memory for a chunk, blocking if insufficient memory is available
func (m *MemoryManager) ReserveMemory(ctx context.Context, job *Job) error {
	estimatedMemory := m.EstimateMemoryUsage(job.Chunk.TotalSize)

	// Proactive GC for large chunks
	if job.Chunk.TotalSize > m.config.ProactiveGCThreshold {
		runtime.GC()
		atomic.AddInt64(&m.totalGCTriggered, 1)
	}

	// Try to allocate immediately
	if m.tryReserve(estimatedMemory) {
		return nil
	}

	// Memory insufficient, wait for memory to become available
	atomic.AddInt32(&m.waitingTasks, 1)
	atomic.AddInt64(&m.totalTasksQueued, 1)
	defer atomic.AddInt32(&m.waitingTasks, -1)

	// Loop until we can allocate or context is cancelled
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-m.releaseSignal:
			// Try to allocate again
			if m.tryReserve(estimatedMemory) {
				return nil
			}
			// Still can't allocate, continue waiting
		}
	}
}

// tryReserve attempts to reserve memory without blocking
func (m *MemoryManager) tryReserve(estimatedMemory int64) bool {
	for {
		currentUsage := atomic.LoadInt64(&m.estimatedUsage)
		budget := atomic.LoadInt64(&m.memoryBudget)

		if currentUsage+estimatedMemory > budget {
			return false
		}

		// Try to atomically increment usage
		if atomic.CompareAndSwapInt64(&m.estimatedUsage, currentUsage, currentUsage+estimatedMemory) {
			// Update max memory used
			for {
				maxUsed := atomic.LoadInt64(&m.maxMemoryUsed)
				newUsage := currentUsage + estimatedMemory
				if newUsage <= maxUsed {
					break
				}
				if atomic.CompareAndSwapInt64(&m.maxMemoryUsed, maxUsed, newUsage) {
					break
				}
			}
			return true
		}
		// CAS failed, retry
	}
}

// ReleaseMemory releases memory after a chunk is processed
func (m *MemoryManager) ReleaseMemory(chunkSize int64) {
	estimatedMemory := m.EstimateMemoryUsage(chunkSize)
	atomic.AddInt64(&m.estimatedUsage, -estimatedMemory)
	atomic.AddInt64(&m.totalTasksReleased, 1)

	// Signal waiting tasks
	select {
	case m.releaseSignal <- struct{}{}:
	default:
		// Channel already has a signal
	}
}

// updateMemoryBudget calculates and updates the memory budget
func (m *MemoryManager) updateMemoryBudget() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Available memory = Total system memory - allocated memory
	// For containers, respect cgroup limits if available
	availableMemory := m.getAvailableMemory()

	// Apply budget percentage
	budget := int64(float64(availableMemory) * m.config.MemoryBudgetPercent)

	// Ensure minimum buffer
	if budget > availableMemory-m.config.MinMemoryBuffer {
		budget = availableMemory - m.config.MinMemoryBuffer
	}

	atomic.StoreInt64(&m.memoryBudget, budget)
}

// getAvailableMemory returns the available system memory
func (m *MemoryManager) getAvailableMemory() int64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Heuristic: available = system total - heap allocated
	// This is a conservative estimate
	systemTotal := int64(memStats.Sys)
	heapAlloc := int64(memStats.HeapAlloc)

	available := systemTotal - heapAlloc

	// Minimum 1GB available
	if available < 1<<30 {
		available = 1 << 30
	}

	return available
}

// startMonitoring starts the memory monitoring goroutine
func (m *MemoryManager) startMonitoring() {
	m.monitorWg.Add(1)
	go m.monitorMemory()
}

// monitorMemory monitors memory usage and updates budget
func (m *MemoryManager) monitorMemory() {
	defer m.monitorWg.Done()

	ticker := time.NewTicker(m.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.stopMonitor:
			return
		case <-ticker.C:
			// Update memory budget based on current system state
			m.updateMemoryBudget()

			// Trigger signal if there are waiting tasks and memory is available
			if atomic.LoadInt32(&m.waitingTasks) > 0 {
				select {
				case m.releaseSignal <- struct{}{}:
				default:
				}
			}
		}
	}
}

// Stop stops the memory manager
func (m *MemoryManager) Stop() {
	m.cancel()

	// Stop monitoring
	select {
	case <-m.stopMonitor:
	default:
		close(m.stopMonitor)
	}

	m.monitorWg.Wait()
}

// GetStats returns memory manager statistics
func (m *MemoryManager) GetStats() MemoryManagerStats {
	return MemoryManagerStats{
		EstimatedUsage:     atomic.LoadInt64(&m.estimatedUsage),
		MemoryBudget:       atomic.LoadInt64(&m.memoryBudget),
		WaitingTasks:       atomic.LoadInt32(&m.waitingTasks),
		TotalTasksQueued:   atomic.LoadInt64(&m.totalTasksQueued),
		TotalTasksReleased: atomic.LoadInt64(&m.totalTasksReleased),
		TotalGCTriggered:   atomic.LoadInt64(&m.totalGCTriggered),
		MaxMemoryUsed:      atomic.LoadInt64(&m.maxMemoryUsed),
		BudgetPercent:      m.config.MemoryBudgetPercent,
	}
}

// MemoryManagerStats holds memory manager statistics
type MemoryManagerStats struct {
	EstimatedUsage     int64
	MemoryBudget       int64
	WaitingTasks       int32
	TotalTasksQueued   int64
	TotalTasksReleased int64
	TotalGCTriggered   int64
	MaxMemoryUsed      int64
	BudgetPercent      float64
}

// String returns a formatted string representation of stats
func (s MemoryManagerStats) String() string {
	usagePercent := float64(s.EstimatedUsage) / float64(s.MemoryBudget) * 100
	return fmt.Sprintf(
		"Memory: %d/%d MB (%.1f%%), Waiting: %d, Queued: %d, Released: %d, GC: %d, Max: %d MB",
		s.EstimatedUsage/(1<<20),
		s.MemoryBudget/(1<<20),
		usagePercent,
		s.WaitingTasks,
		s.TotalTasksQueued,
		s.TotalTasksReleased,
		s.TotalGCTriggered,
		s.MaxMemoryUsed/(1<<20),
	)
}
