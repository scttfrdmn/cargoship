/*
Package s3 memory buffer implements memory-aware chunk buffering for optimal resource utilization.

This module provides intelligent buffer management with dynamic allocation, memory pressure detection,
and adaptive buffer sizing based on available system resources and performance requirements.
*/
package s3

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryAwareBuffer implements intelligent chunk buffering with dynamic memory management.
type MemoryAwareBuffer struct {
	// Buffer configuration
	maxMemoryUsage      int64
	targetUtilization   float64
	bufferSizeStrategy  BufferSizeStrategy
	allocationStrategy  AllocationStrategy
	
	// Memory management
	currentUsage        int64
	allocatedBuffers    map[string]*ChunkBuffer
	bufferPool          *BufferPool
	memoryMonitor       *MemoryMonitor
	
	// Dynamic adaptation
	adaptiveResize      bool
	resizeThreshold     float64
	memoryPressureLevel MemoryPressureLevel
	// lastAdaptation      time.Time // TODO: Add usage for adaptation timing
	adaptationHistory   []AdaptationEvent
	
	// Performance optimization
	preallocationEnabled bool
	bufferReuse         bool
	compressionBuffers  bool
	asyncCleanup        bool
	
	// Monitoring and metrics
	bufferMetrics       *BufferMetrics
	performanceTracker  *BufferPerformanceTracker
	allocationTracker   *AllocationTracker
	
	// Concurrency control
	mu                  sync.RWMutex
	allocationMutex     sync.Mutex
	cleanupMutex        sync.Mutex
	
	ctx                 context.Context
	cancel              context.CancelFunc
}

// BufferSizeStrategy defines different buffer sizing strategies.
type BufferSizeStrategy string

const (
	BufferSizeFixed        BufferSizeStrategy = "fixed"
	BufferSizeDynamic      BufferSizeStrategy = "dynamic"
	BufferSizeAdaptive     BufferSizeStrategy = "adaptive"
	BufferSizeContentAware BufferSizeStrategy = "content_aware"
	BufferSizePerformance  BufferSizeStrategy = "performance"
)

// AllocationStrategy defines buffer allocation approaches.
type AllocationStrategy string

const (
	AllocationEager       AllocationStrategy = "eager"
	AllocationLazy        AllocationStrategy = "lazy"
	AllocationPredictive  AllocationStrategy = "predictive"
	AllocationHybrid      AllocationStrategy = "hybrid"
)

// MemoryPressureLevel indicates current memory pressure.
type MemoryPressureLevel string

const (
	MemoryPressureNone     MemoryPressureLevel = "none"
	MemoryPressureLow      MemoryPressureLevel = "low"
	MemoryPressureModerate MemoryPressureLevel = "moderate"
	MemoryPressureHigh     MemoryPressureLevel = "high"
	MemoryPressureCritical MemoryPressureLevel = "critical"
)

// ChunkBuffer represents a buffer for chunk data with metadata.
type ChunkBuffer struct {
	ID                  string
	Data                []byte
	Size                int64
	Capacity            int64
	ContentType         ContentType
	CompressionLevel    CompressionLevel
	Priority            BufferPriority
	
	// State tracking
	State               BufferState
	AllocationTime      time.Time
	LastAccessed        time.Time
	AccessCount         int64
	
	// Performance tracking
	ReadOperations      int64
	WriteOperations     int64
	TotalBytesRead      int64
	TotalBytesWritten   int64
	
	// Memory management
	PoolAllocated       bool
	ReferenceCount      int32
	CleanupCallback     func(*ChunkBuffer)
	
	mu                  sync.RWMutex
}

// BufferPriority defines buffer priority levels for allocation decisions.
type BufferPriority string

const (
	BufferPriorityLow      BufferPriority = "low"
	BufferPriorityNormal   BufferPriority = "normal"
	BufferPriorityHigh     BufferPriority = "high"
	BufferPriorityCritical BufferPriority = "critical"
)

// BufferState represents the current state of a buffer.
type BufferState string

const (
	BufferStateAllocated   BufferState = "allocated"
	BufferStateActive      BufferState = "active"
	BufferStateIdle        BufferState = "idle"
	BufferStateReleasing   BufferState = "releasing"
	BufferStateReleased    BufferState = "released"
)

// BufferPool manages a pool of reusable buffers.
type BufferPool struct {
	// Pool configuration
	minPoolSize         int
	maxPoolSize         int
	bufferSizes         []int64
	
	// Pool management
	pools               map[int64]chan []byte
	allocationCount     map[int64]int64
	hitRate             map[int64]float64
	
	// Performance optimization
	warmupEnabled       bool
	backgroundCleanup   bool
	adaptiveSizing      bool
	
	mu                  sync.RWMutex
}

// MemoryMonitor tracks system memory usage and pressure.
type MemoryMonitor struct {
	// Monitoring configuration
	monitoringInterval  time.Duration
	pressureThresholds  map[MemoryPressureLevel]float64
	
	// Current state
	currentMemoryUsage  int64
	availableMemory     int64
	totalSystemMemory   int64
	pressureLevel       MemoryPressureLevel
	
	// Historical tracking
	usageHistory        []MemoryUsageSnapshot
	pressureHistory     []MemoryPressureEvent
	
	// Adaptive monitoring
	adaptiveInterval    bool
	dynamicThresholds   bool
	
	mu                  sync.RWMutex
	ctx                 context.Context
}

// BufferMetrics tracks buffer performance and usage statistics.
type BufferMetrics struct {
	// Allocation metrics
	TotalAllocations    int64
	TotalDeallocations  int64
	CurrentBuffers      int64
	TotalMemoryUsed     int64
	PeakMemoryUsage     int64
	
	// Performance metrics
	AllocationTime      time.Duration
	DeallocationTime    time.Duration
	AverageBufferSize   int64
	BufferReuseRate     float64
	MemoryEfficiency    float64
	
	// Pool metrics
	PoolHitRate         float64
	PoolMissRate        float64
	PoolUtilization     float64
	
	// Pressure metrics
	PressureEvents      int64
	AdaptationEvents    int64
	CleanupEvents       int64
	
	LastUpdate          time.Time
}

// AdaptationEvent records a buffer adaptation event.
type AdaptationEvent struct {
	Timestamp           time.Time
	EventType           AdaptationType
	PressureLevel       MemoryPressureLevel
	MemoryUsageBefore   int64
	MemoryUsageAfter    int64
	BuffersAffected     int
	PerformanceImpact   float64
	Success             bool
}

// AdaptationType defines types of buffer adaptations.
type AdaptationType string

const (
	AdaptationResize       AdaptationType = "resize"
	AdaptationRelease      AdaptationType = "release"
	AdaptationPreallocate  AdaptationType = "preallocate"
	AdaptationRebalance    AdaptationType = "rebalance"
	AdaptationCleanup      AdaptationType = "cleanup"
)

// MemoryUsageSnapshot captures memory usage at a point in time.
type MemoryUsageSnapshot struct {
	Timestamp           time.Time
	TotalUsage          int64
	BufferUsage         int64
	SystemUsage         int64
	AvailableMemory     int64
	PressureLevel       MemoryPressureLevel
}

// MemoryPressureEvent records a memory pressure event.
type MemoryPressureEvent struct {
	Timestamp           time.Time
	PreviousLevel       MemoryPressureLevel
	NewLevel            MemoryPressureLevel
	TriggerUsage        int64
	Response            string
	ResponseTime        time.Duration
}

// NewMemoryAwareBuffer creates a new memory-aware buffer manager.
func NewMemoryAwareBuffer(maxMemoryMB int64, ctx context.Context) *MemoryAwareBuffer {
	bufferCtx, cancel := context.WithCancel(ctx)
	
	mab := &MemoryAwareBuffer{
		maxMemoryUsage:      maxMemoryMB * 1024 * 1024, // Convert MB to bytes
		targetUtilization:   0.8,
		bufferSizeStrategy:  BufferSizeAdaptive,
		allocationStrategy:  AllocationHybrid,
		
		allocatedBuffers:    make(map[string]*ChunkBuffer),
		bufferPool:          NewBufferPool(),
		memoryMonitor:       NewMemoryMonitor(bufferCtx),
		
		adaptiveResize:      true,
		resizeThreshold:     0.1,
		memoryPressureLevel: MemoryPressureNone,
		adaptationHistory:   make([]AdaptationEvent, 0, 100),
		
		preallocationEnabled: true,
		bufferReuse:         true,
		compressionBuffers:  true,
		asyncCleanup:        true,
		
		bufferMetrics:       NewBufferMetrics(),
		performanceTracker:  NewBufferPerformanceTracker(),
		allocationTracker:   NewAllocationTracker(),
		
		ctx:                 bufferCtx,
		cancel:              cancel,
	}
	
	// Start monitoring and management goroutines
	go mab.memoryMonitoringLoop()
	go mab.adaptationLoop()
	go mab.cleanupLoop()
	
	return mab
}

// AllocateBuffer allocates a new buffer with the specified size and properties.
func (mab *MemoryAwareBuffer) AllocateBuffer(id string, size int64, contentType ContentType, priority BufferPriority) (*ChunkBuffer, error) {
	mab.allocationMutex.Lock()
	defer mab.allocationMutex.Unlock()
	
	// Check memory pressure and availability
	if err := mab.checkMemoryAvailability(size); err != nil {
		return nil, err
	}
	
	// Determine optimal buffer size
	optimalSize := mab.calculateOptimalBufferSize(size, contentType, priority)
	
	// Try to get buffer from pool first
	var buffer *ChunkBuffer
	if mab.bufferReuse {
		if poolBuffer := mab.bufferPool.GetBuffer(optimalSize); poolBuffer != nil {
			buffer = mab.createBufferFromPool(id, poolBuffer, contentType, priority)
		}
	}
	
	// Allocate new buffer if pool miss
	if buffer == nil {
		data := make([]byte, optimalSize)
		buffer = &ChunkBuffer{
			ID:               id,
			Data:             data,
			Size:             0,
			Capacity:         optimalSize,
			ContentType:      contentType,
			Priority:         priority,
			State:            BufferStateAllocated,
			AllocationTime:   time.Now(),
			LastAccessed:     time.Now(),
			PoolAllocated:    false,
			ReferenceCount:   1,
		}
	}
	
	// Track allocation
	mab.mu.Lock()
	mab.allocatedBuffers[id] = buffer
	atomic.AddInt64(&mab.currentUsage, optimalSize)
	mab.mu.Unlock()
	
	// Update metrics
	mab.updateAllocationMetrics(buffer)
	
	return buffer, nil
}

// ReleaseBuffer releases a buffer back to the pool or deallocates it.
func (mab *MemoryAwareBuffer) ReleaseBuffer(id string) error {
	mab.mu.Lock()
	buffer, exists := mab.allocatedBuffers[id]
	if !exists {
		mab.mu.Unlock()
		return fmt.Errorf("buffer %s not found", id)
	}
	
	delete(mab.allocatedBuffers, id)
	mab.mu.Unlock()
	
	return mab.releaseBufferInternal(buffer)
}

// GetBuffer retrieves an existing buffer by ID.
func (mab *MemoryAwareBuffer) GetBuffer(id string) (*ChunkBuffer, error) {
	mab.mu.RLock()
	buffer, exists := mab.allocatedBuffers[id]
	mab.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("buffer %s not found", id)
	}
	
	// Update access tracking
	buffer.mu.Lock()
	buffer.LastAccessed = time.Now()
	buffer.AccessCount++
	buffer.mu.Unlock()
	
	return buffer, nil
}

// ResizeBuffer dynamically resizes a buffer based on usage patterns.
func (mab *MemoryAwareBuffer) ResizeBuffer(id string, newSize int64) error {
	mab.mu.Lock()
	buffer, exists := mab.allocatedBuffers[id]
	mab.mu.Unlock()
	
	if !exists {
		return fmt.Errorf("buffer %s not found", id)
	}
	
	return mab.resizeBufferInternal(buffer, newSize)
}

// GetMemoryUsage returns current memory usage statistics.
func (mab *MemoryAwareBuffer) GetMemoryUsage() *MemoryUsageSnapshot {
	mab.mu.RLock()
	defer mab.mu.RUnlock()
	
	return &MemoryUsageSnapshot{
		Timestamp:       time.Now(),
		TotalUsage:      atomic.LoadInt64(&mab.currentUsage),
		BufferUsage:     mab.calculateBufferUsage(),
		SystemUsage:     mab.memoryMonitor.currentMemoryUsage,
		AvailableMemory: mab.memoryMonitor.availableMemory,
		PressureLevel:   mab.memoryPressureLevel,
	}
}

// GetBufferMetrics returns comprehensive buffer metrics.
func (mab *MemoryAwareBuffer) GetBufferMetrics() *BufferMetrics {
	mab.mu.RLock()
	defer mab.mu.RUnlock()
	
	metrics := *mab.bufferMetrics
	metrics.CurrentBuffers = int64(len(mab.allocatedBuffers))
	metrics.TotalMemoryUsed = atomic.LoadInt64(&mab.currentUsage)
	metrics.LastUpdate = time.Now()
	
	return &metrics
}

// AdaptToMemoryPressure adapts buffer usage based on current memory pressure.
func (mab *MemoryAwareBuffer) AdaptToMemoryPressure(pressureLevel MemoryPressureLevel) error {
	mab.mu.Lock()
	defer mab.mu.Unlock()
	
	oldPressure := mab.memoryPressureLevel
	mab.memoryPressureLevel = pressureLevel
	
	switch pressureLevel {
	case MemoryPressureHigh, MemoryPressureCritical:
		return mab.performAggressiveCleanup()
	case MemoryPressureModerate:
		return mab.performModerateCleanup()
	case MemoryPressureLow:
		return mab.performLightCleanup()
	case MemoryPressureNone:
		return mab.enablePreallocation()
	}
	
	// Record adaptation event
	mab.recordAdaptationEvent(AdaptationRebalance, oldPressure, pressureLevel, 0, 0, 0, true)
	
	return nil
}

// Internal methods

func (mab *MemoryAwareBuffer) checkMemoryAvailability(size int64) error {
	currentUsage := atomic.LoadInt64(&mab.currentUsage)
	
	// Check against maximum memory usage
	if currentUsage+size > mab.maxMemoryUsage {
		return fmt.Errorf("allocation would exceed maximum memory usage (%d + %d > %d)", 
			currentUsage, size, mab.maxMemoryUsage)
	}
	
	// Check memory pressure level
	switch mab.memoryPressureLevel {
	case MemoryPressureCritical:
		return fmt.Errorf("allocation blocked due to critical memory pressure")
	case MemoryPressureHigh:
		if size > 1024*1024 { // Block large allocations under high pressure
			return fmt.Errorf("large allocation blocked due to high memory pressure")
		}
	}
	
	return nil
}

func (mab *MemoryAwareBuffer) calculateOptimalBufferSize(requestedSize int64, contentType ContentType, priority BufferPriority) int64 {
	switch mab.bufferSizeStrategy {
	case BufferSizeFixed:
		return mab.getFixedBufferSize()
	case BufferSizeDynamic:
		return mab.calculateDynamicSize(requestedSize)
	case BufferSizeAdaptive:
		return mab.calculateAdaptiveSize(requestedSize, contentType, priority)
	case BufferSizeContentAware:
		return mab.calculateContentAwareSize(requestedSize, contentType)
	case BufferSizePerformance:
		return mab.calculatePerformanceOptimizedSize(requestedSize, priority)
	default:
		return requestedSize
	}
}

func (mab *MemoryAwareBuffer) getFixedBufferSize() int64 {
	return 16 * 1024 * 1024 // 16MB fixed size
}

func (mab *MemoryAwareBuffer) calculateDynamicSize(requestedSize int64) int64 {
	// Round up to nearest power of 2 for better memory alignment
	size := int64(1)
	for size < requestedSize {
		size <<= 1
	}
	
	// Cap at reasonable maximum
	if size > 64*1024*1024 {
		size = 64 * 1024 * 1024 // 64MB max
	}
	
	return size
}

func (mab *MemoryAwareBuffer) calculateAdaptiveSize(requestedSize int64, contentType ContentType, priority BufferPriority) int64 {
	baseSize := mab.calculateDynamicSize(requestedSize)
	
	// Adjust based on content type
	switch contentType {
	case ContentTypeText:
		baseSize = int64(float64(baseSize) * 0.8) // Text compresses well, smaller buffer
	case ContentTypeVideo, ContentTypeAudio:
		baseSize = int64(float64(baseSize) * 1.5) // Media benefits from larger buffers
	case ContentTypeCompressed:
		baseSize = requestedSize // Already compressed, exact size
	}
	
	// Adjust based on priority
	switch priority {
	case BufferPriorityCritical:
		baseSize = int64(float64(baseSize) * 1.2)
	case BufferPriorityLow:
		baseSize = int64(float64(baseSize) * 0.9)
	}
	
	// Apply memory pressure adjustment
	switch mab.memoryPressureLevel {
	case MemoryPressureHigh:
		baseSize = int64(float64(baseSize) * 0.7)
	case MemoryPressureCritical:
		baseSize = int64(float64(baseSize) * 0.5)
	}
	
	return baseSize
}

func (mab *MemoryAwareBuffer) calculateContentAwareSize(requestedSize int64, contentType ContentType) int64 {
	// Content-specific optimization
	multiplier := map[ContentType]float64{
		ContentTypeText:       0.6,  // High compression ratio
		ContentTypeBinary:     1.0,  // Standard
		ContentTypeImage:      1.2,  // May benefit from larger chunks
		ContentTypeVideo:      1.8,  // Large streaming buffers
		ContentTypeAudio:      1.3,  // Medium streaming buffers
		ContentTypeCompressed: 1.0,  // No additional compression
		ContentTypeArchive:    1.1,  // Slightly larger for efficiency
	}
	
	factor := multiplier[contentType]
	if factor == 0 {
		factor = 1.0 // Default
	}
	
	return int64(float64(requestedSize) * factor)
}

func (mab *MemoryAwareBuffer) calculatePerformanceOptimizedSize(requestedSize int64, priority BufferPriority) int64 {
	// Optimize for performance based on current system state
	currentUsage := float64(atomic.LoadInt64(&mab.currentUsage)) / float64(mab.maxMemoryUsage)
	
	if currentUsage < 0.5 {
		// Low usage, can afford larger buffers for better performance
		return int64(float64(requestedSize) * 1.5)
	} else if currentUsage > 0.8 {
		// High usage, conserve memory
		return requestedSize
	}
	
	// Medium usage, slight optimization
	return int64(float64(requestedSize) * 1.2)
}

func (mab *MemoryAwareBuffer) createBufferFromPool(id string, data []byte, contentType ContentType, priority BufferPriority) *ChunkBuffer {
	return &ChunkBuffer{
		ID:               id,
		Data:             data,
		Size:             0,
		Capacity:         int64(len(data)),
		ContentType:      contentType,
		Priority:         priority,
		State:            BufferStateAllocated,
		AllocationTime:   time.Now(),
		LastAccessed:     time.Now(),
		PoolAllocated:    true,
		ReferenceCount:   1,
	}
}

func (mab *MemoryAwareBuffer) releaseBufferInternal(buffer *ChunkBuffer) error {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	
	// Decrement reference count
	if atomic.AddInt32(&buffer.ReferenceCount, -1) > 0 {
		return nil // Still has references
	}
	
	buffer.State = BufferStateReleasing
	
	// Update memory usage
	atomic.AddInt64(&mab.currentUsage, -buffer.Capacity)
	
	// Return to pool or deallocate
	if mab.bufferReuse && buffer.PoolAllocated {
		mab.bufferPool.ReturnBuffer(buffer.Data)
	} else {
		// Mark for garbage collection
		buffer.Data = nil
	}
	
	buffer.State = BufferStateReleased
	
	// Update metrics
	mab.updateDeallocationMetrics(buffer)
	
	// Call cleanup callback if set
	if buffer.CleanupCallback != nil {
		buffer.CleanupCallback(buffer)
	}
	
	return nil
}

func (mab *MemoryAwareBuffer) resizeBufferInternal(buffer *ChunkBuffer, newSize int64) error {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	
	oldSize := buffer.Capacity
	sizeDiff := newSize - oldSize
	
	// Check if resize is possible
	if err := mab.checkMemoryAvailability(sizeDiff); err != nil {
		return err
	}
	
	// Create new buffer data
	newData := make([]byte, newSize)
	copy(newData, buffer.Data[:minInt64Buffer(int64(len(buffer.Data)), newSize)])
	
	// Update buffer
	buffer.Data = newData
	buffer.Capacity = newSize
	
	// Update memory tracking
	atomic.AddInt64(&mab.currentUsage, sizeDiff)
	
	// Record adaptation event
	mab.recordAdaptationEvent(AdaptationResize, mab.memoryPressureLevel, mab.memoryPressureLevel, oldSize, newSize, 1, true)
	
	return nil
}

func (mab *MemoryAwareBuffer) calculateBufferUsage() int64 {
	total := int64(0)
	for _, buffer := range mab.allocatedBuffers {
		total += buffer.Capacity
	}
	return total
}

func (mab *MemoryAwareBuffer) performAggressiveCleanup() error {
	// Release low priority buffers
	released := 0
	for id, buffer := range mab.allocatedBuffers {
		if buffer.Priority == BufferPriorityLow && buffer.State == BufferStateIdle {
			delete(mab.allocatedBuffers, id)
			_ = mab.releaseBufferInternal(buffer) // Ignore error during cleanup
			released++
		}
	}
	
	// Force garbage collection
	runtime.GC()
	
	mab.recordAdaptationEvent(AdaptationCleanup, MemoryPressureHigh, mab.memoryPressureLevel, 0, 0, released, true)
	return nil
}

func (mab *MemoryAwareBuffer) performModerateCleanup() error {
	// Release idle buffers that haven't been accessed recently
	threshold := time.Now().Add(-time.Minute * 5)
	released := 0
	
	for id, buffer := range mab.allocatedBuffers {
		if buffer.LastAccessed.Before(threshold) && buffer.State == BufferStateIdle {
			delete(mab.allocatedBuffers, id)
			_ = mab.releaseBufferInternal(buffer) // Ignore error during cleanup
			released++
		}
	}
	
	mab.recordAdaptationEvent(AdaptationCleanup, MemoryPressureModerate, mab.memoryPressureLevel, 0, 0, released, true)
	return nil
}

func (mab *MemoryAwareBuffer) performLightCleanup() error {
	// Only release buffers that haven't been accessed in a long time
	threshold := time.Now().Add(-time.Minute * 15)
	released := 0
	
	for id, buffer := range mab.allocatedBuffers {
		if buffer.LastAccessed.Before(threshold) && buffer.Priority == BufferPriorityLow {
			delete(mab.allocatedBuffers, id)
			_ = mab.releaseBufferInternal(buffer) // Ignore error during cleanup
			released++
		}
	}
	
	mab.recordAdaptationEvent(AdaptationCleanup, MemoryPressureLow, mab.memoryPressureLevel, 0, 0, released, true)
	return nil
}

func (mab *MemoryAwareBuffer) enablePreallocation() error {
	if !mab.preallocationEnabled {
		return nil
	}
	
	// Pre-allocate common buffer sizes if memory is available
	currentUsage := float64(atomic.LoadInt64(&mab.currentUsage)) / float64(mab.maxMemoryUsage)
	if currentUsage < 0.5 {
		mab.bufferPool.WarmupPools()
	}
	
	return nil
}

func (mab *MemoryAwareBuffer) updateAllocationMetrics(buffer *ChunkBuffer) {
	metrics := mab.bufferMetrics
	metrics.TotalAllocations++
	
	if buffer.Capacity > metrics.PeakMemoryUsage {
		metrics.PeakMemoryUsage = buffer.Capacity
	}
	
	// Update average buffer size
	if metrics.TotalAllocations > 0 {
		metrics.AverageBufferSize = (metrics.AverageBufferSize*(metrics.TotalAllocations-1) + buffer.Capacity) / metrics.TotalAllocations
	}
}

func (mab *MemoryAwareBuffer) updateDeallocationMetrics(buffer *ChunkBuffer) {
	metrics := mab.bufferMetrics
	metrics.TotalDeallocations++
	
	// Update reuse rate
	if buffer.PoolAllocated {
		totalOps := metrics.TotalAllocations
		if totalOps > 0 {
			metrics.BufferReuseRate = float64(metrics.TotalDeallocations) / float64(totalOps)
		}
	}
}

func (mab *MemoryAwareBuffer) recordAdaptationEvent(eventType AdaptationType, oldPressure, newPressure MemoryPressureLevel, before, after int64, affected int, success bool) {
	event := AdaptationEvent{
		Timestamp:         time.Now(),
		EventType:         eventType,
		PressureLevel:     newPressure,
		MemoryUsageBefore: before,
		MemoryUsageAfter:  after,
		BuffersAffected:   affected,
		Success:           success,
	}
	
	mab.adaptationHistory = append(mab.adaptationHistory, event)
	
	// Limit history size
	if len(mab.adaptationHistory) > 100 {
		mab.adaptationHistory = mab.adaptationHistory[1:]
	}
	
	mab.bufferMetrics.AdaptationEvents++
}

// Background loops

func (mab *MemoryAwareBuffer) memoryMonitoringLoop() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	
	for {
		select {
		case <-mab.ctx.Done():
			return
		case <-ticker.C:
			mab.memoryMonitor.UpdateMemoryStats()
			mab.checkMemoryPressure()
		}
	}
}

func (mab *MemoryAwareBuffer) adaptationLoop() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	
	for {
		select {
		case <-mab.ctx.Done():
			return
		case <-ticker.C:
			if mab.adaptiveResize {
				mab.performAdaptiveResize()
			}
		}
	}
}

func (mab *MemoryAwareBuffer) cleanupLoop() {
	ticker := time.NewTicker(time.Minute * 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-mab.ctx.Done():
			return
		case <-ticker.C:
			if mab.asyncCleanup {
				mab.performPeriodicCleanup()
			}
		}
	}
}

func (mab *MemoryAwareBuffer) checkMemoryPressure() {
	usage := mab.memoryMonitor.CalculateMemoryPressure()
	newPressure := mab.memoryMonitor.GetPressureLevel(usage)
	
	if newPressure != mab.memoryPressureLevel {
		_ = mab.AdaptToMemoryPressure(newPressure) // Ignore error during monitoring
	}
}

func (mab *MemoryAwareBuffer) performAdaptiveResize() {
	// Analyze buffer usage patterns and resize accordingly
	mab.mu.RLock()
	defer mab.mu.RUnlock()
	
	for _, buffer := range mab.allocatedBuffers {
		if mab.shouldResizeBuffer(buffer) {
			optimalSize := mab.calculateOptimalResizeSize(buffer)
			if optimalSize != buffer.Capacity {
				go func(buf *ChunkBuffer, size int64) {
					_ = mab.resizeBufferInternal(buf, size) // Ignore error during auto-resize
				}(buffer, optimalSize)
			}
		}
	}
}

func (mab *MemoryAwareBuffer) shouldResizeBuffer(buffer *ChunkBuffer) bool {
	// Check if buffer size is significantly different from usage
	utilizationRatio := float64(buffer.Size) / float64(buffer.Capacity)
	return utilizationRatio < 0.3 || utilizationRatio > 0.9
}

func (mab *MemoryAwareBuffer) calculateOptimalResizeSize(buffer *ChunkBuffer) int64 {
	utilizationRatio := float64(buffer.Size) / float64(buffer.Capacity)
	
	if utilizationRatio < 0.3 {
		// Buffer is underutilized, shrink it
		return int64(float64(buffer.Size) * 1.3)
	} else if utilizationRatio > 0.9 {
		// Buffer is overutilized, grow it
		return int64(float64(buffer.Capacity) * 1.5)
	}
	
	return buffer.Capacity
}

func (mab *MemoryAwareBuffer) performPeriodicCleanup() {
	// Regular cleanup of unused buffers
	threshold := time.Now().Add(-time.Minute * 10)
	
	mab.cleanupMutex.Lock()
	defer mab.cleanupMutex.Unlock()
	
	for id, buffer := range mab.allocatedBuffers {
		if buffer.LastAccessed.Before(threshold) && buffer.State == BufferStateIdle {
			delete(mab.allocatedBuffers, id)
			_ = mab.releaseBufferInternal(buffer) // Ignore error during cleanup
		}
	}
}

// Placeholder implementations for external components

func NewBufferPool() *BufferPool {
	pool := &BufferPool{
		minPoolSize:     10,
		maxPoolSize:     100,
		bufferSizes:     []int64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216}, // Powers of 2
		pools:           make(map[int64]chan []byte),
		allocationCount: make(map[int64]int64),
		hitRate:         make(map[int64]float64),
		warmupEnabled:   true,
		backgroundCleanup: true,
		adaptiveSizing:  true,
	}
	
	// Initialize pools for each size
	for _, size := range pool.bufferSizes {
		pool.pools[size] = make(chan []byte, pool.maxPoolSize)
	}
	
	return pool
}

func NewMemoryMonitor(ctx context.Context) *MemoryMonitor {
	monitor := &MemoryMonitor{
		monitoringInterval: time.Second * 5,
		pressureThresholds: map[MemoryPressureLevel]float64{
			MemoryPressureNone:     0.6,
			MemoryPressureLow:      0.7,
			MemoryPressureModerate: 0.8,
			MemoryPressureHigh:     0.9,
			MemoryPressureCritical: 0.95,
		},
		usageHistory:    make([]MemoryUsageSnapshot, 0, 1000),
		pressureHistory: make([]MemoryPressureEvent, 0, 100),
		adaptiveInterval: true,
		dynamicThresholds: false,
		ctx:             ctx,
	}
	
	// Initialize with current system memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	monitor.totalSystemMemory = int64(m.Sys)
	monitor.currentMemoryUsage = int64(m.Alloc)
	monitor.availableMemory = monitor.totalSystemMemory - monitor.currentMemoryUsage
	
	return monitor
}

func NewBufferMetrics() *BufferMetrics {
	return &BufferMetrics{
		LastUpdate: time.Now(),
	}
}

func NewBufferPerformanceTracker() *BufferPerformanceTracker {
	return &BufferPerformanceTracker{}
}

func NewAllocationTracker() *AllocationTracker {
	return &AllocationTracker{}
}

// Buffer pool methods

func (bp *BufferPool) GetBuffer(size int64) []byte {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	
	// Find the smallest buffer size that can accommodate the request
	var poolSize int64
	for _, s := range bp.bufferSizes {
		if s >= size {
			poolSize = s
			break
		}
	}
	
	if poolSize == 0 {
		return nil // Size too large for pool
	}
	
	// Try to get from pool
	pool := bp.pools[poolSize]
	select {
	case buffer := <-pool:
		bp.allocationCount[poolSize]++
		return buffer
	default:
		// Pool empty, create new buffer
		return make([]byte, poolSize)
	}
}

func (bp *BufferPool) ReturnBuffer(buffer []byte) {
	if buffer == nil {
		return
	}
	
	size := int64(len(buffer))
	
	bp.mu.RLock()
	pool, exists := bp.pools[size]
	bp.mu.RUnlock()
	
	if !exists {
		return // Size not in pool
	}
	
	// Clear buffer data for security
	for i := range buffer {
		buffer[i] = 0
	}
	
	// Return to pool if not full
	select {
	case pool <- buffer:
		// Successfully returned to pool
	default:
		// Pool full, let GC handle it
	}
}

func (bp *BufferPool) WarmupPools() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	
	if !bp.warmupEnabled {
		return
	}
	
	// Pre-allocate buffers in each pool
	for _, size := range bp.bufferSizes {
		pool := bp.pools[size]
		for len(pool) < bp.minPoolSize {
			buffer := make([]byte, size)
			select {
			case pool <- buffer:
				// Added to pool
			default:
				// Pool full, can't add more buffers
				return
			}
		}
	}
}

// Memory monitor methods

func (mm *MemoryMonitor) UpdateMemoryStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	mm.currentMemoryUsage = int64(m.Alloc)
	mm.availableMemory = mm.totalSystemMemory - mm.currentMemoryUsage
	
	// Record snapshot
	snapshot := MemoryUsageSnapshot{
		Timestamp:       time.Now(),
		TotalUsage:      mm.currentMemoryUsage,
		SystemUsage:     int64(m.Sys),
		AvailableMemory: mm.availableMemory,
		PressureLevel:   mm.pressureLevel,
	}
	
	mm.usageHistory = append(mm.usageHistory, snapshot)
	
	// Limit history size
	if len(mm.usageHistory) > 1000 {
		mm.usageHistory = mm.usageHistory[1:]
	}
}

func (mm *MemoryMonitor) CalculateMemoryPressure() float64 {
	if mm.totalSystemMemory == 0 {
		return 0.0
	}
	
	return float64(mm.currentMemoryUsage) / float64(mm.totalSystemMemory)
}

func (mm *MemoryMonitor) GetPressureLevel(usage float64) MemoryPressureLevel {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	for level, threshold := range mm.pressureThresholds {
		if usage <= threshold {
			return level
		}
	}
	
	return MemoryPressureCritical
}

// Utility function
func minInt64Buffer(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// Placeholder types
type BufferPerformanceTracker struct{}
type AllocationTracker struct{}