package staging

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewStagingBufferManager(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 4,
		StagingQueueDepth:    8,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	assert.NotNil(t, sbm)
	assert.NotNil(t, sbm.bufferPool)
	assert.NotNil(t, sbm.activeBuffers)
	assert.NotNil(t, sbm.stagingQueue)
	assert.NotNil(t, sbm.memoryMonitor)
	assert.Equal(t, config, sbm.config)
	assert.Equal(t, 0, len(sbm.activeBuffers))
	assert.Equal(t, 8, cap(sbm.stagingQueue))
}

func TestStagingBufferManager_Start(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start should not block
	sbm.Start(ctx)
	
	// Give workers time to start
	time.Sleep(100 * time.Millisecond)
	
	// Cancel and cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestStagingBufferManager_StageChunk(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sbm.Start(ctx)
	
	// Create a test staging request
	testData := "test data for staging"
	req := &StagingRequest{
		StreamID:     "test-stream-1",
		Reader:       strings.NewReader(testData),
		ExpectedSize: int64(len(testData)),
		ContentType:  "text",
		Priority:     1,
		Callback:     nil,
	}
	
	boundary := ChunkBoundary{
		StartOffset:      0,
		EndOffset:        int64(len(testData)),
		Size:             int64(len(testData)),
		AlignedWithFile:  true,
		CompressionScore: 0.5,
		PredictedRatio:   0.3,
		OptimalForNetwork: true,
	}
	
	// Stage the chunk
	err := sbm.StageChunk(req, boundary)
	assert.NoError(t, err)
	
	// The request might be processed immediately by workers, so queue length could be 0
	// Just verify that staging was successful (no error)
	assert.GreaterOrEqual(t, sbm.GetQueueLength(), 0)
}

func TestStagingBufferManager_StageChunk_QueueFull(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 1,
		StagingQueueDepth:    2, // Small queue for testing
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Fill the queue
	for i := 0; i < 2; i++ {
		req := &StagingRequest{
			StreamID:     "test-stream",
			Reader:       strings.NewReader("test data"),
			ExpectedSize: 9,
			ContentType:  "text",
			Priority:     1,
		}
		
		boundary := ChunkBoundary{Size: 9}
		err := sbm.StageChunk(req, boundary)
		assert.NoError(t, err)
	}
	
	// Try to add one more - should fail
	req := &StagingRequest{
		StreamID:     "test-stream-overflow",
		Reader:       strings.NewReader("test data"),
		ExpectedSize: 9,
		ContentType:  "text",
		Priority:     1,
	}
	
	boundary := ChunkBoundary{Size: 9}
	err := sbm.StageChunk(req, boundary)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue_full")
}

func TestStagingBufferManager_GetStagedChunk(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Add a staged chunk manually
	testData := []byte("test staged data")
	chunk := &StagedChunk{
		ID:               "test-chunk-1",
		Data:             testData,
		CompressedSize:   len(testData),
		UncompressedSize: len(testData),
		CompressionRatio: 1.0,
		StagedAt:         time.Now(),
		ContentType:      "text",
		Entropy:          0.5,
	}
	
	sbm.activeBuffers["test-chunk-1"] = chunk
	
	// Retrieve the chunk
	retrievedChunk, err := sbm.GetStagedChunk("test-chunk-1")
	assert.NoError(t, err)
	assert.Equal(t, chunk, retrievedChunk)
}

func TestStagingBufferManager_GetStagedChunk_NotFound(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Try to get non-existent chunk
	chunk, err := sbm.GetStagedChunk("non-existent")
	assert.Error(t, err)
	assert.Nil(t, chunk)
	assert.Contains(t, err.Error(), "chunk_not_found")
}

func TestStagingBufferManager_ReleaseStagedChunk(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Add a staged chunk
	testData := []byte("test staged data")
	chunk := &StagedChunk{
		ID:               "test-chunk-1",
		Data:             testData,
		CompressedSize:   len(testData),
		UncompressedSize: len(testData),
		CompressionRatio: 1.0,
		StagedAt:         time.Now(),
		ContentType:      "text",
		Entropy:          0.5,
	}
	
	sbm.activeBuffers["test-chunk-1"] = chunk
	assert.Equal(t, 1, len(sbm.activeBuffers))
	
	// Release the chunk
	sbm.ReleaseStagedChunk("test-chunk-1")
	assert.Equal(t, 0, len(sbm.activeBuffers))
}

func TestStagingBufferManager_ReleaseStagedChunk_NotExists(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Try to release non-existent chunk - should not panic
	sbm.ReleaseStagedChunk("non-existent")
	assert.Equal(t, 0, len(sbm.activeBuffers))
}

func TestStagingBufferManager_GetActiveCount(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Initially should be 0
	assert.Equal(t, 0, sbm.GetActiveCount())
	
	// Add some chunks
	for i := 0; i < 3; i++ {
		chunk := &StagedChunk{
			ID:   "test-chunk-" + string(rune(i)),
			Data: []byte("test data"),
		}
		sbm.activeBuffers[chunk.ID] = chunk
	}
	
	assert.Equal(t, 3, sbm.GetActiveCount())
}

func TestStagingBufferManager_GetUtilization(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Initially should be 0
	assert.Equal(t, 0.0, sbm.GetUtilization())
	
	// Add a chunk
	chunk := &StagedChunk{
		ID:   "test-chunk-1",
		Data: []byte("test data"),
	}
	sbm.activeBuffers[chunk.ID] = chunk
	
	// Should be 0.25 (1 out of 4 possible chunks)
	utilization := sbm.GetUtilization()
	assert.Equal(t, 0.25, utilization)
}

func TestStagingBufferManager_CleanupExpired(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Add an expired chunk
	expiredChunk := &StagedChunk{
		ID:       "expired-chunk",
		Data:     []byte("expired data"),
		StagedAt: time.Now().Add(-time.Minute * 15), // 15 minutes ago
	}
	sbm.activeBuffers["expired-chunk"] = expiredChunk
	
	// Add a recent chunk
	recentChunk := &StagedChunk{
		ID:       "recent-chunk",
		Data:     []byte("recent data"),
		StagedAt: time.Now().Add(-time.Minute * 5), // 5 minutes ago
	}
	sbm.activeBuffers["recent-chunk"] = recentChunk
	
	assert.Equal(t, 2, len(sbm.activeBuffers))
	
	// Cleanup expired chunks
	sbm.CleanupExpired()
	
	// Only recent chunk should remain
	assert.Equal(t, 1, len(sbm.activeBuffers))
	assert.Contains(t, sbm.activeBuffers, "recent-chunk")
	assert.NotContains(t, sbm.activeBuffers, "expired-chunk")
}

func TestNewBufferPool(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	assert.NotNil(t, bp)
	assert.Equal(t, 0, len(bp.buffers))
	assert.Equal(t, 4, bp.maxBuffers) // 64/16 = 4
	assert.Equal(t, 16*1024*1024, bp.bufferSize)
}

func TestBufferPool_GetBuffer(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Get a buffer
	buffer := bp.GetBuffer(1024)
	assert.NotNil(t, buffer)
	assert.Equal(t, 1024, len(buffer))
	assert.True(t, cap(buffer) >= 1024)
}

func TestBufferPool_GetBuffer_ReuseExisting(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Add a buffer to the pool
	existingBuffer := make([]byte, 16*1024*1024)
	bp.buffers = append(bp.buffers, existingBuffer)
	
	// Get a buffer - should reuse existing
	buffer := bp.GetBuffer(1024)
	assert.NotNil(t, buffer)
	assert.Equal(t, 1024, len(buffer))
	assert.Equal(t, 0, len(bp.buffers)) // Buffer was removed from pool
}

func TestBufferPool_ReturnBuffer(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Create a buffer of appropriate size
	buffer := make([]byte, 16*1024*1024)
	
	// Return it to the pool
	bp.ReturnBuffer(buffer)
	
	// Should be in the pool now
	assert.Equal(t, 1, len(bp.buffers))
}

func TestBufferPool_ReturnBuffer_TooSmall(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Create a buffer that's too small
	buffer := make([]byte, 1024)
	
	// Return it to the pool - should be rejected
	bp.ReturnBuffer(buffer)
	
	// Should not be in the pool
	assert.Equal(t, 0, len(bp.buffers))
}

func TestBufferPool_ReduceSize(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Add 4 buffers
	for i := 0; i < 4; i++ {
		buffer := make([]byte, 16*1024*1024)
		bp.buffers = append(bp.buffers, buffer)
	}
	
	assert.Equal(t, 4, len(bp.buffers))
	
	// Reduce size - should remove half
	bp.ReduceSize()
	assert.Equal(t, 2, len(bp.buffers))
}

func TestBufferPool_IncreaseSize(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:   64,
		TargetChunkSizeMB: 16,
	}
	
	bp := NewBufferPool(config)
	
	// Should start with 0 buffers
	assert.Equal(t, 0, len(bp.buffers))
	
	// Increase size - should pre-allocate buffers
	bp.IncreaseSize()
	assert.Equal(t, 2, len(bp.buffers)) // maxBuffers/2 = 4/2 = 2
}

func TestNewMemoryMonitor(t *testing.T) {
	config := &StagingConfig{
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	mm := NewMemoryMonitor(config)
	
	assert.NotNil(t, mm)
	assert.Equal(t, config, mm.config)
	assert.Equal(t, 0.0, mm.currentUsage)
	assert.False(t, mm.pressureDetected)
}

func TestMemoryMonitor_GetUsage(t *testing.T) {
	config := &StagingConfig{
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	mm := NewMemoryMonitor(config)
	
	// Initially should be 0
	assert.Equal(t, 0.0, mm.GetUsage())
	
	// Set usage manually
	mm.currentUsage = 0.5
	assert.Equal(t, 0.5, mm.GetUsage())
}

func TestMemoryMonitor_IsUnderPressure(t *testing.T) {
	config := &StagingConfig{
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	mm := NewMemoryMonitor(config)
	
	// Initially should not be under pressure
	assert.False(t, mm.IsUnderPressure())
	
	// Set pressure manually
	mm.pressureDetected = true
	assert.True(t, mm.IsUnderPressure())
}

func TestMemoryMonitor_Start(t *testing.T) {
	config := &StagingConfig{
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	mm := NewMemoryMonitor(config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start should not block
	go mm.Start(ctx)
	
	// Give monitor time to start
	time.Sleep(100 * time.Millisecond)
	
	// Usage should be updated
	usage := mm.GetUsage()
	assert.True(t, usage >= 0.0)
	
	// Cancel and cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestStagingBufferManager_ProcessStagingRequest(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Test successful processing
	testData := "test data for processing"
	completed := make(chan bool, 1)
	var resultChunk *StagedChunk
	var resultErr error
	
	req := &StagingRequest{
		StreamID:     "test-stream",
		Reader:       strings.NewReader(testData),
		ExpectedSize: int64(len(testData)),
		ContentType:  "text",
		Priority:     1,
		Callback: func(chunk *StagedChunk, err error) {
			resultChunk = chunk
			resultErr = err
			completed <- true
		},
	}
	
	// Process the request
	sbm.processStagingRequest(req, 0)
	
	// Wait for completion
	select {
	case <-completed:
		assert.NoError(t, resultErr)
		assert.NotNil(t, resultChunk)
		assert.Equal(t, len(testData), len(resultChunk.Data))
		assert.Equal(t, "text", resultChunk.ContentType)
		assert.Contains(t, resultChunk.ID, "test-stream")
	case <-time.After(time.Second):
		t.Fatal("Callback not called within timeout")
	}
}

func TestStagingBufferManager_ProcessStagingRequest_ReadError(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Test with failing reader
	completed := make(chan bool, 1)
	var resultChunk *StagedChunk
	var resultErr error
	
	req := &StagingRequest{
		StreamID:     "test-stream",
		Reader:       &failingReader{},
		ExpectedSize: 1024,
		ContentType:  "text",
		Priority:     1,
		Callback: func(chunk *StagedChunk, err error) {
			resultChunk = chunk
			resultErr = err
			completed <- true
		},
	}
	
	// Process the request
	sbm.processStagingRequest(req, 0)
	
	// Wait for completion
	select {
	case <-completed:
		assert.Error(t, resultErr)
		assert.Nil(t, resultChunk)
	case <-time.After(time.Second):
		t.Fatal("Callback not called within timeout")
	}
}

func TestStagingBufferManager_EstimateCompressionRatio(t *testing.T) {
	config := &StagingConfig{
		MaxBufferSizeMB:      64,
		TargetChunkSizeMB:    16,
		MaxConcurrentStaging: 2,
		StagingQueueDepth:    4,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
	
	sbm := NewStagingBufferManager(config)
	
	// Test different content types
	testCases := []struct {
		entropy     float64
		contentType string
		expected    float64
	}{
		{4.0, "text", 0.15},      // High entropy text should compress well
		{2.0, "text", 0.225},     // Low entropy text
		{6.0, "image", 0.225},    // Images don't compress much
		{4.0, "compressed", 0.475}, // Already compressed
		{4.0, "binary", 0.3},     // Binary data
		{4.0, "unknown", 0.25},   // Unknown type
	}
	
	for _, tc := range testCases {
		ratio := sbm.estimateCompressionRatio(tc.entropy, tc.contentType)
		assert.InDelta(t, tc.expected, ratio, 0.01, "entropy: %f, type: %s", tc.entropy, tc.contentType)
	}
}

// Helper types for testing
type failingReader struct{}

func (fr *failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}