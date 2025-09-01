package staging

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// NewStagingBufferManager creates a new staging buffer manager.
func NewStagingBufferManager(config *StagingConfig) *StagingBufferManager {
	dedupeConfig := DefaultDeduplicationConfig()
	compressionConfig := DefaultAdaptiveCompressionConfig()
	return &StagingBufferManager{
		bufferPool:          NewBufferPool(config),
		activeBuffers:       make(map[string]*StagedChunk),
		stagingQueue:        make(chan *StagingRequest, config.StagingQueueDepth),
		memoryMonitor:       NewMemoryMonitor(config),
		deduplicator:        NewChunkDeduplicator(dedupeConfig),
		compressionSelector: NewAdaptiveCompressionSelector(compressionConfig),
		duplicateRefs:       make(map[string][]string),
		config:              config,
	}
}

// Start begins the staging buffer manager operations.
func (sbm *StagingBufferManager) Start(ctx context.Context) {
	// Start staging workers
	for i := 0; i < sbm.config.MaxConcurrentStaging; i++ {
		go sbm.stagingWorker(ctx, i)
	}

	// Start memory monitor
	go sbm.memoryMonitor.Start(ctx)

	// Start cleanup routine
	go sbm.cleanupLoop(ctx)
}

// StageChunk stages a chunk with the given boundary.
func (sbm *StagingBufferManager) StageChunk(req *StagingRequest, boundary ChunkBoundary) error {
	sbm.mu.Lock()
	defer sbm.mu.Unlock()

	// Check memory pressure
	if sbm.memoryMonitor.IsUnderPressure() {
		return &StagingError{
			Type:    "memory_pressure",
			Message: "insufficient memory for staging",
			Details: map[string]interface{}{
				"memory_usage": sbm.memoryMonitor.GetUsage(),
				"threshold":    sbm.config.MemoryPressureThreshold,
			},
		}
	}

	// Check if we have capacity
	if len(sbm.stagingQueue) >= cap(sbm.stagingQueue) {
		return &StagingError{
			Type:    "queue_full",
			Message: "staging queue is full",
			Details: map[string]interface{}{
				"queue_length": len(sbm.stagingQueue),
				"capacity":     cap(sbm.stagingQueue),
			},
		}
	}

	// Queue the staging request
	select {
	case sbm.stagingQueue <- req:
		return nil
	default:
		return &StagingError{
			Type:    "queue_full",
			Message: "could not queue staging request",
		}
	}
}

// GetStagedChunk retrieves a staged chunk for upload.
func (sbm *StagingBufferManager) GetStagedChunk(streamID string) (*StagedChunk, error) {
	sbm.mu.RLock()
	defer sbm.mu.RUnlock()

	chunk, exists := sbm.activeBuffers[streamID]
	if !exists {
		return nil, &StagingError{
			Type:    "chunk_not_found",
			Message: "staged chunk not found",
			Details: map[string]interface{}{
				"stream_id": streamID,
			},
		}
	}

	return chunk, nil
}

// ReleaseStagedChunk releases a staged chunk after upload.
func (sbm *StagingBufferManager) ReleaseStagedChunk(streamID string) {
	sbm.mu.Lock()
	defer sbm.mu.Unlock()

	if chunk, exists := sbm.activeBuffers[streamID]; exists {
		// Return buffer to pool if chunk has data
		if chunk.Data != nil {
			sbm.bufferPool.ReturnBuffer(chunk.Data)
		}

		// Clean up duplicate references
		if chunk.Hash != "" {
			sbm.removeDuplicateReference(chunk.Hash, chunk.ID)
		}

		// Remove from active buffers
		delete(sbm.activeBuffers, streamID)
	}
}

// GetActiveCount returns the number of actively staged chunks.
func (sbm *StagingBufferManager) GetActiveCount() int {
	sbm.mu.RLock()
	defer sbm.mu.RUnlock()
	return len(sbm.activeBuffers)
}

// GetQueueLength returns the current staging queue length.
func (sbm *StagingBufferManager) GetQueueLength() int {
	return len(sbm.stagingQueue)
}

// GetUtilization returns the buffer utilization percentage.
func (sbm *StagingBufferManager) GetUtilization() float64 {
	sbm.mu.RLock()
	activeCount := len(sbm.activeBuffers)
	sbm.mu.RUnlock()

	maxBuffers := sbm.config.MaxBufferSizeMB / sbm.config.TargetChunkSizeMB
	if maxBuffers == 0 {
		return 0
	}

	return float64(activeCount) / float64(maxBuffers)
}

// CleanupExpired removes expired staged chunks.
func (sbm *StagingBufferManager) CleanupExpired() {
	sbm.mu.Lock()
	defer sbm.mu.Unlock()

	now := time.Now()
	expiration := time.Minute * 10 // 10 minute expiration

	for streamID, chunk := range sbm.activeBuffers {
		if now.Sub(chunk.StagedAt) > expiration {
			// Return buffer to pool if chunk has data
			if chunk.Data != nil {
				sbm.bufferPool.ReturnBuffer(chunk.Data)
			}

			// Clean up duplicate references
			if chunk.Hash != "" {
				sbm.removeDuplicateReference(chunk.Hash, chunk.ID)
			}

			// Remove from active buffers
			delete(sbm.activeBuffers, streamID)
		}
	}
}

// AdjustBufferSizes adjusts buffer allocation based on memory pressure.
func (sbm *StagingBufferManager) AdjustBufferSizes() {
	usage := sbm.memoryMonitor.GetUsage()

	// If memory usage is high, trigger garbage collection
	if usage > sbm.config.GCTriggerThreshold {
		runtime.GC()
	}

	// Adjust buffer pool size based on memory pressure
	if usage > sbm.config.MemoryPressureThreshold {
		sbm.bufferPool.ReduceSize()
	} else if usage < sbm.config.MemoryPressureThreshold*0.5 {
		sbm.bufferPool.IncreaseSize()
	}
}

// stagingWorker processes staging requests.
func (sbm *StagingBufferManager) stagingWorker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-sbm.stagingQueue:
			sbm.processStagingRequest(req, workerID)
		}
	}
}

// processStagingRequest processes a single staging request.
func (sbm *StagingBufferManager) processStagingRequest(req *StagingRequest, workerID int) {
	// Get buffer from pool
	buffer := sbm.bufferPool.GetBuffer(req.ExpectedSize)
	if buffer == nil {
		// Failed to get buffer
		if req.Callback != nil {
			req.Callback(nil, &StagingError{
				Type:    "buffer_allocation_failed",
				Message: "could not allocate staging buffer",
			})
		}
		return
	}

	// Read data into buffer
	bytesRead, err := req.Reader.Read(buffer)
	if err != nil {
		sbm.bufferPool.ReturnBuffer(buffer)
		if req.Callback != nil {
			req.Callback(nil, err)
		}
		return
	}

	// Trim buffer to actual size
	actualData := buffer[:bytesRead]

	// Perform deduplication analysis
	dedupeResult := sbm.deduplicator.AnalyzeChunk(actualData, req.ContentType)

	// Create staged chunk
	chunk := &StagedChunk{
		ID:                  fmt.Sprintf("%s-%d-%d", req.StreamID, workerID, time.Now().UnixNano()),
		CompressedSize:      bytesRead,
		UncompressedSize:    bytesRead, // Will be updated by compression analysis
		StagedAt:            time.Now(),
		ContentType:         req.ContentType,
		IsDuplicate:         dedupeResult.IsDuplicate,
		DuplicateHash:       dedupeResult.ExistingHash,
		SimilarityScore:     dedupeResult.SimilarityScore,
		DeltaParent:         dedupeResult.DeltaParent,
		BytesSaved:          dedupeResult.BytesSaved,
		DeduplicationAction: dedupeResult.RecommendedAction,
	}

	// Handle deduplication result
	switch dedupeResult.RecommendedAction {
	case ActionStore:
		// Store full chunk data
		chunk.Data = actualData
		chunk.Hash = sbm.deduplicator.computeStrongHash(actualData)
		
	case ActionDuplicate:
		// Don't store data, just reference existing chunk
		sbm.bufferPool.ReturnBuffer(buffer) // Return unused buffer
		chunk.Data = nil
		chunk.Hash = dedupeResult.ExistingHash
		
		// Track reference
		sbm.mu.Lock()
		sbm.duplicateRefs[dedupeResult.ExistingHash] = append(sbm.duplicateRefs[dedupeResult.ExistingHash], chunk.ID)
		sbm.mu.Unlock()
		
	case ActionDeltaCompress:
		// Store delta-compressed data (simplified implementation)
		chunk.Data = actualData
		chunk.Hash = sbm.deduplicator.computeStrongHash(actualData)
		chunk.CompressedSize = int(dedupeResult.DeltaSize)
		
	case ActionSimilarityReplace:
		// Store reference to similar chunk
		chunk.Data = nil
		chunk.Hash = dedupeResult.ExistingHash
		
		// Track reference
		sbm.mu.Lock()
		sbm.duplicateRefs[dedupeResult.ExistingHash] = append(sbm.duplicateRefs[dedupeResult.ExistingHash], chunk.ID)
		sbm.mu.Unlock()
	}

	// Analyze content for additional metadata if we have data
	if chunk.Data != nil {
		sbm.analyzeChunkContent(chunk)
	}
	
	// Always select compression algorithm (even for duplicates for compatibility)
	sbm.selectOptimalCompression(chunk, req)

	// Store in active buffers
	sbm.mu.Lock()
	sbm.activeBuffers[chunk.ID] = chunk
	sbm.mu.Unlock()

	// Notify completion
	if req.Callback != nil {
		req.Callback(chunk, nil)
	}
}

// analyzeChunkContent analyzes chunk content for metadata.
func (sbm *StagingBufferManager) analyzeChunkContent(chunk *StagedChunk) {
	// Calculate entropy
	entropy := NewEntropyCalculator().CalculateEntropy(chunk.Data)
	chunk.Entropy = entropy

	// Estimate compression ratio based on entropy
	chunk.CompressionRatio = sbm.estimateCompressionRatio(entropy, chunk.ContentType)

	// Estimate compressed size
	chunk.CompressedSize = int(float64(len(chunk.Data)) * chunk.CompressionRatio)
}

// estimateCompressionRatio estimates compression ratio from entropy and content type.
func (sbm *StagingBufferManager) estimateCompressionRatio(entropy float64, contentType string) float64 {
	// Base ratio from entropy
	baseRatio := 1.0 - (entropy / 8.0)

	// Adjust by content type
	switch contentType {
	case "text":
		baseRatio *= 0.3
	case "image":
		baseRatio *= 0.9
	case "compressed":
		baseRatio *= 0.95
	case "binary":
		baseRatio *= 0.6
	default:
		baseRatio *= 0.5
	}

	// Ensure reasonable bounds
	if baseRatio < 0.1 {
		baseRatio = 0.1
	}
	if baseRatio > 0.9 {
		baseRatio = 0.9
	}

	return baseRatio
}

// selectOptimalCompression selects the optimal compression algorithm for a chunk.
func (sbm *StagingBufferManager) selectOptimalCompression(chunk *StagedChunk, req *StagingRequest) {
	// Create content profile
	contentProfile := &ContentProfile{
		ContentType:     chunk.ContentType,
		Size:            int64(len(chunk.Data)),
		Entropy:         chunk.Entropy,
		Compressibility: 1.0 - chunk.CompressionRatio,
		Patterns:        []ContentPattern{}, // Could be enhanced with pattern detection
		Metadata:        map[string]interface{}{
			"duplicate": chunk.IsDuplicate,
			"similarity_score": chunk.SimilarityScore,
		},
	}

	// Use network condition from request or create default
	networkCondition := req.NetworkCondition
	if networkCondition == nil {
		networkCondition = &NetworkCondition{
			BandwidthMBps: 10.0,  // Default bandwidth
			LatencyMs:     50.0,  // Default latency
			Reliability:   0.95,  // Default reliability
		}
	}

	// Create compression context
	context := &CompressionContext{
		FileName:          req.StreamID,
		FileExtension:     sbm.getFileExtension(req.StreamID),
		FileSize:          int64(len(chunk.Data)),
		SystemLoad:        sbm.getSystemLoad(),
		AvailableMemoryMB: sbm.getAvailableMemoryMB(),
		Priority:          req.Priority,
	}

	// Get compression decision
	decision, err := sbm.compressionSelector.SelectCompressionAlgorithm(contentProfile, networkCondition, context)
	if err != nil {
		// Fall back to default algorithm
		chunk.SelectedAlgorithm = "zstd"
		return
	}

	// Store compression decision
	chunk.SelectedAlgorithm = decision.SelectedAlgorithm
	chunk.CompressionSettings = decision.RecommendedSettings
	chunk.CompressionDecision = decision

	// Update compression ratio estimate based on selected algorithm
	if decision.PredictedPerformance != nil {
		chunk.CompressionRatio = decision.PredictedPerformance.EstimatedCompressionRatio
		chunk.CompressedSize = int(float64(len(chunk.Data)) * chunk.CompressionRatio)
	}
}

// getFileExtension extracts file extension from filename.
func (sbm *StagingBufferManager) getFileExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}

// getSystemLoad returns current system load (simplified implementation).
func (sbm *StagingBufferManager) getSystemLoad() float64 {
	// Simple approximation based on active buffers
	sbm.mu.RLock()
	activeCount := len(sbm.activeBuffers)
	sbm.mu.RUnlock()
	
	maxActive := sbm.config.MaxConcurrentStaging * 10
	if maxActive == 0 {
		maxActive = 100
	}
	
	load := float64(activeCount) / float64(maxActive)
	if load > 1.0 {
		load = 1.0
	}
	
	return load
}

// getAvailableMemoryMB returns available memory in MB (simplified implementation).
func (sbm *StagingBufferManager) getAvailableMemoryMB() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Estimate available memory based on system stats
	allocatedMB := int64(m.Sys / 1024 / 1024)
	totalEstimate := allocatedMB * 4 // Rough estimate
	availableMB := totalEstimate - allocatedMB
	
	if availableMB < 256 {
		availableMB = 256 // Minimum
	}
	
	return availableMB
}

// cleanupLoop runs periodic cleanup operations.
func (sbm *StagingBufferManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sbm.CleanupExpired()
			sbm.AdjustBufferSizes()
		}
	}
}

// BufferPool manages a pool of reusable buffers.
type BufferPool struct {
	buffers    [][]byte
	maxBuffers int
	bufferSize int
	mu         sync.Mutex
}

// NewBufferPool creates a new buffer pool.
func NewBufferPool(config *StagingConfig) *BufferPool {
	bufferSize := config.TargetChunkSizeMB * 1024 * 1024
	maxBuffers := config.MaxBufferSizeMB / config.TargetChunkSizeMB

	return &BufferPool{
		buffers:    make([][]byte, 0, maxBuffers),
		maxBuffers: maxBuffers,
		bufferSize: bufferSize,
	}
}

// GetBuffer gets a buffer from the pool or allocates a new one.
func (bp *BufferPool) GetBuffer(size int64) []byte {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Try to reuse existing buffer
	if len(bp.buffers) > 0 {
		buffer := bp.buffers[len(bp.buffers)-1]
		bp.buffers = bp.buffers[:len(bp.buffers)-1]

		// Resize if needed
		if int64(len(buffer)) < size {
			return make([]byte, size)
		}

		return buffer[:size]
	}

	// Allocate new buffer
	return make([]byte, size)
}

// ReturnBuffer returns a buffer to the pool.
func (bp *BufferPool) ReturnBuffer(buffer []byte) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Only return if we have space and buffer is reasonable size
	if len(bp.buffers) < bp.maxBuffers && len(buffer) >= bp.bufferSize/2 {
		// Reset buffer
		if cap(buffer) >= bp.bufferSize {
			bp.buffers = append(bp.buffers, buffer[:bp.bufferSize])
		}
	}
	// Otherwise let it be garbage collected
}

// ReduceSize reduces the buffer pool size.
func (bp *BufferPool) ReduceSize() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Remove half the buffers
	if len(bp.buffers) > 2 {
		newSize := len(bp.buffers) / 2
		bp.buffers = bp.buffers[:newSize]
	}
}

// IncreaseSize increases the buffer pool size.
func (bp *BufferPool) IncreaseSize() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Pre-allocate some buffers if we have room
	for len(bp.buffers) < bp.maxBuffers/2 {
		buffer := make([]byte, bp.bufferSize)
		bp.buffers = append(bp.buffers, buffer)
	}
}

// MemoryMonitor monitors system memory usage.
type MemoryMonitor struct {
	config           *StagingConfig
	currentUsage     float64
	lastUpdate       time.Time
	pressureDetected bool
	mu               sync.RWMutex
}

// NewMemoryMonitor creates a new memory monitor.
func NewMemoryMonitor(config *StagingConfig) *MemoryMonitor {
	return &MemoryMonitor{
		config:     config,
		lastUpdate: time.Now(),
	}
}

// Start begins memory monitoring.
func (mm *MemoryMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.updateMemoryUsage()
		}
	}
}

// updateMemoryUsage updates the current memory usage statistics.
func (mm *MemoryMonitor) updateMemoryUsage() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Calculate memory usage ratio
	totalMemory := memStats.Sys
	usedMemory := memStats.Alloc

	if totalMemory > 0 {
		mm.currentUsage = float64(usedMemory) / float64(totalMemory)
	}

	// Update pressure detection
	mm.pressureDetected = mm.currentUsage > mm.config.MemoryPressureThreshold
	mm.lastUpdate = time.Now()
}

// GetUsage returns the current memory usage ratio.
func (mm *MemoryMonitor) GetUsage() float64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.currentUsage
}

// IsUnderPressure returns true if the system is under memory pressure.
func (mm *MemoryMonitor) IsUnderPressure() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.pressureDetected
}

// GetDeduplicationStats returns current deduplication statistics.
func (sbm *StagingBufferManager) GetDeduplicationStats() *DeduplicationStats {
	return sbm.deduplicator.GetStats()
}

// removeDuplicateReference removes a chunk ID from duplicate references.
func (sbm *StagingBufferManager) removeDuplicateReference(hash, chunkID string) {
	if refs, exists := sbm.duplicateRefs[hash]; exists {
		// Remove the specific chunk ID
		for i, ref := range refs {
			if ref == chunkID {
				sbm.duplicateRefs[hash] = append(refs[:i], refs[i+1:]...)
				break
			}
		}
		
		// Clean up empty reference lists
		if len(sbm.duplicateRefs[hash]) == 0 {
			delete(sbm.duplicateRefs, hash)
		}
	}
}

// GetDuplicateReferenceCount returns the number of chunks referencing a hash.
func (sbm *StagingBufferManager) GetDuplicateReferenceCount(hash string) int {
	sbm.mu.RLock()
	defer sbm.mu.RUnlock()
	
	if refs, exists := sbm.duplicateRefs[hash]; exists {
		return len(refs)
	}
	return 0
}

// ClearDeduplicationCache clears the deduplication cache.
func (sbm *StagingBufferManager) ClearDeduplicationCache() {
	sbm.deduplicator.ClearCache()
}

// GetCompressionSelector returns the adaptive compression selector.
func (sbm *StagingBufferManager) GetCompressionSelector() *AdaptiveCompressionSelector {
	return sbm.compressionSelector
}

// LearnFromCompressionResult learns from actual compression results.
func (sbm *StagingBufferManager) LearnFromCompressionResult(result *CompressionResult) {
	sbm.compressionSelector.LearnFromCompressionResult(result)
}

// GetAlgorithmPerformance returns performance statistics for a compression algorithm.
func (sbm *StagingBufferManager) GetAlgorithmPerformance(algorithm string) *AlgorithmPerformance {
	return sbm.compressionSelector.GetAlgorithmPerformance(algorithm)
}

// GetCompressionProfile returns the compression profile for a content type.
func (sbm *StagingBufferManager) GetCompressionProfile(contentType string) *CompressionProfile {
	return sbm.compressionSelector.GetCompressionProfile(contentType)
}

// UpdateCompressionRule updates or adds a compression rule for a file extension.
func (sbm *StagingBufferManager) UpdateCompressionRule(extension string, rule *CompressionRule) {
	sbm.compressionSelector.UpdateCompressionRule(extension, rule)
}

// ClearCompressionHistory clears compression performance history.
func (sbm *StagingBufferManager) ClearCompressionHistory() {
	sbm.compressionSelector.ClearPerformanceHistory()
}
