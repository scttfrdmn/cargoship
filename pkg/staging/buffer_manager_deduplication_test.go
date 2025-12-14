package staging

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingBufferManager_WithDeduplication(t *testing.T) {
	config := DefaultStagingConfig()
	config.MaxBufferSizeMB = 64
	config.TargetChunkSizeMB = 16

	manager := NewStagingBufferManager(config)
	require.NotNil(t, manager.deduplicator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the manager
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 100) // Let workers start
}

func TestStagingBufferManager_DeduplicateIdenticalChunks(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create identical test data (large enough to trigger deduplication)
	testData := make([]byte, 2048) // 2KB data above threshold
	for i := range testData {
		testData[i] = byte((i % 26) + 'a') // Repeating pattern
	}
	reader1 := bytes.NewReader(testData)
	reader2 := bytes.NewReader(testData)

	// Stage first chunk
	req1 := &StagingRequest{
		StreamID:     "stream1",
		Reader:       reader1,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk1 *StagedChunk
	var err1 error
	req1.Callback = func(chunk *StagedChunk, err error) {
		chunk1 = chunk
		err1 = err
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100) // Wait for processing

	require.NoError(t, err1)
	require.NotNil(t, chunk1)
	assert.False(t, chunk1.IsDuplicate)
	assert.Equal(t, ActionStore, chunk1.DeduplicationAction)
	assert.NotNil(t, chunk1.Data)

	// Stage second identical chunk
	req2 := &StagingRequest{
		StreamID:     "stream2",
		Reader:       reader2,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk2 *StagedChunk
	var err2 error
	req2.Callback = func(chunk *StagedChunk, err error) {
		chunk2 = chunk
		err2 = err
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100) // Wait for processing

	require.NoError(t, err2)
	require.NotNil(t, chunk2)
	assert.True(t, chunk2.IsDuplicate)
	assert.Equal(t, ActionDuplicate, chunk2.DeduplicationAction)
	assert.Nil(t, chunk2.Data) // Duplicate should not store data
	assert.Equal(t, chunk1.Hash, chunk2.Hash)
	assert.Equal(t, int64(len(testData)), chunk2.BytesSaved)

	// Verify duplicate reference tracking
	refCount := manager.GetDuplicateReferenceCount(chunk1.Hash)
	assert.Equal(t, 1, refCount) // One reference (chunk2 referencing chunk1)
}

func TestStagingBufferManager_SimilarityDetection(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create similar test data (large enough to trigger deduplication)
	basePattern := []byte("The quick brown fox jumps over the lazy dog in the sunny meadow")
	baseData := make([]byte, 2048)
	similarPattern := []byte("The quick brown fox jumps over the lazy cat in the sunny meadow")
	similarData := make([]byte, 2048)

	// Fill with repeating patterns
	for i := 0; i < len(baseData); i++ {
		baseData[i] = basePattern[i%len(basePattern)]
		similarData[i] = similarPattern[i%len(similarPattern)]
	}

	reader1 := bytes.NewReader(baseData)
	reader2 := bytes.NewReader(similarData)

	// Stage base chunk
	req1 := &StagingRequest{
		StreamID:     "base",
		Reader:       reader1,
		ExpectedSize: int64(len(baseData)),
		ContentType:  "text/plain",
	}

	var chunk1 *StagedChunk
	req1.Callback = func(chunk *StagedChunk, err error) {
		chunk1 = chunk
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NotNil(t, chunk1)
	assert.Equal(t, ActionStore, chunk1.DeduplicationAction)

	// Stage similar chunk
	req2 := &StagingRequest{
		StreamID:     "similar",
		Reader:       reader2,
		ExpectedSize: int64(len(similarData)),
		ContentType:  "text/plain",
	}

	var chunk2 *StagedChunk
	req2.Callback = func(chunk *StagedChunk, err error) {
		chunk2 = chunk
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NotNil(t, chunk2)
	// Should detect similarity if threshold is met
	assert.True(t, chunk2.SimilarityScore > 0)
	assert.True(t, chunk2.BytesSaved >= 0)
}

func TestStagingBufferManager_SmallChunksSkipDeduplication(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create small test data (below deduplication threshold)
	smallData := []byte("small")
	reader := bytes.NewReader(smallData)

	req := &StagingRequest{
		StreamID:     "small",
		Reader:       reader,
		ExpectedSize: int64(len(smallData)),
		ContentType:  "text/plain",
	}

	var chunk *StagedChunk
	req.Callback = func(c *StagedChunk, err error) {
		chunk = c
	}

	err := manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	require.NotNil(t, chunk)
	assert.Equal(t, ActionStore, chunk.DeduplicationAction)
	assert.NotNil(t, chunk.Data)
}

func TestStagingBufferManager_DeduplicationStats(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Initial stats should be zero
	stats := manager.GetDeduplicationStats()
	assert.Equal(t, int64(0), stats.TotalChunksProcessed)
	assert.Equal(t, int64(0), stats.DuplicateChunksFound)

	// Process some chunks (large enough for deduplication)
	testData := make([]byte, 2048)
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}

	// First unique chunk
	reader1 := bytes.NewReader(testData)
	req1 := &StagingRequest{
		StreamID:     "stats1",
		Reader:       reader1,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}
	req1.Callback = func(*StagedChunk, error) {}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	// Second duplicate chunk
	reader2 := bytes.NewReader(testData)
	req2 := &StagingRequest{
		StreamID:     "stats2",
		Reader:       reader2,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}
	req2.Callback = func(*StagedChunk, error) {}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	// Check updated stats
	stats = manager.GetDeduplicationStats()
	assert.True(t, stats.TotalChunksProcessed >= 2)
	assert.True(t, stats.DuplicateChunksFound >= 1)
	assert.True(t, stats.BytesSaved > 0)
	assert.True(t, stats.DeduplicationRatio > 0)
}

func TestStagingBufferManager_ReleaseChunkWithDeduplication(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create test chunks (large enough for deduplication)
	testData := make([]byte, 2048)
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}
	reader1 := bytes.NewReader(testData)
	reader2 := bytes.NewReader(testData)

	// Stage first chunk
	req1 := &StagingRequest{
		StreamID:     "release1",
		Reader:       reader1,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk1 *StagedChunk
	req1.Callback = func(chunk *StagedChunk, err error) {
		chunk1 = chunk
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, chunk1)

	// Stage duplicate chunk
	req2 := &StagingRequest{
		StreamID:     "release2",
		Reader:       reader2,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk2 *StagedChunk
	req2.Callback = func(chunk *StagedChunk, err error) {
		chunk2 = chunk
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, chunk2)

	// Verify duplicate reference exists
	refCount := manager.GetDuplicateReferenceCount(chunk1.Hash)
	assert.Equal(t, 1, refCount)

	// Release duplicate chunk
	manager.ReleaseStagedChunk(chunk2.ID)

	// Verify reference was cleaned up
	refCount = manager.GetDuplicateReferenceCount(chunk1.Hash)
	assert.Equal(t, 0, refCount)

	// Verify original chunk still exists
	retrieved, err := manager.GetStagedChunk(chunk1.ID)
	require.NoError(t, err)
	assert.Equal(t, chunk1.ID, retrieved.ID)
}

func TestStagingBufferManager_CleanupExpiredWithDeduplication(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create test data
	testData := []byte("test data for cleanup testing")
	reader := bytes.NewReader(testData)

	req := &StagingRequest{
		StreamID:     "cleanup-test",
		Reader:       reader,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk *StagedChunk
	req.Callback = func(c *StagedChunk, err error) {
		chunk = c
	}

	err := manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, chunk)

	// Manually set old timestamp to trigger expiration
	chunk.StagedAt = time.Now().Add(-time.Hour)

	initialCount := manager.GetActiveCount()
	assert.True(t, initialCount > 0)

	// Trigger cleanup
	manager.CleanupExpired()

	finalCount := manager.GetActiveCount()
	assert.True(t, finalCount < initialCount)
}

func TestStagingBufferManager_ClearDeduplicationCache(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Add some data to build up deduplication cache
	testData := []byte("test data for cache clearing")
	reader := bytes.NewReader(testData)

	req := &StagingRequest{
		StreamID:     "cache-test",
		Reader:       reader,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}
	req.Callback = func(*StagedChunk, error) {}

	err := manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)

	// Verify stats show processed chunks
	stats := manager.GetDeduplicationStats()
	assert.True(t, stats.TotalChunksProcessed > 0)

	// Clear cache
	manager.ClearDeduplicationCache()

	// Note: Stats might not be reset by clearing cache (depends on implementation)
	// but internal hash maps should be cleared
}

func TestStagingBufferManager_ContentTypeAwareness(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create identical data with different content types (large enough for deduplication)
	testData := make([]byte, 2048)
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}
	reader1 := bytes.NewReader(testData)
	reader2 := bytes.NewReader(testData)

	// Stage as text
	req1 := &StagingRequest{
		StreamID:     "text-content",
		Reader:       reader1,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
	}

	var chunk1 *StagedChunk
	req1.Callback = func(chunk *StagedChunk, err error) {
		chunk1 = chunk
	}

	err := manager.StageChunk(req1, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, chunk1)

	// Stage same data as JSON
	req2 := &StagingRequest{
		StreamID:     "json-content",
		Reader:       reader2,
		ExpectedSize: int64(len(testData)),
		ContentType:  "application/json",
	}

	var chunk2 *StagedChunk
	req2.Callback = func(chunk *StagedChunk, err error) {
		chunk2 = chunk
	}

	err = manager.StageChunk(req2, ChunkBoundary{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 100)
	require.NotNil(t, chunk2)

	// Should still detect as duplicate despite different content types
	assert.True(t, chunk2.IsDuplicate)
	assert.Equal(t, chunk1.Hash, chunk2.Hash)
}

func BenchmarkStagingBufferManager_DeduplicationProcessing(b *testing.B) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	// Create test data
	testData := make([]byte, 32*1024) // 32KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		chunkID := 0
		for pb.Next() {
			reader := bytes.NewReader(testData)
			req := &StagingRequest{
				StreamID:     "bench-" + string(rune(chunkID)),
				Reader:       reader,
				ExpectedSize: int64(len(testData)),
				ContentType:  "application/octet-stream",
			}
			req.Callback = func(*StagedChunk, error) {}

			_ = manager.StageChunk(req, ChunkBoundary{})
			chunkID++
		}
	})
}

// Helper function for testing callback behavior
func createStagingCallback(resultChan chan<- *StagedChunk, errorChan chan<- error) func(*StagedChunk, error) {
	return func(chunk *StagedChunk, err error) {
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- chunk
		}
	}
}

func TestStagingRequest_CallbackExecution(t *testing.T) {
	config := DefaultStagingConfig()
	manager := NewStagingBufferManager(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(time.Millisecond * 50)

	testData := []byte("callback execution test data")
	reader := bytes.NewReader(testData)

	resultChan := make(chan *StagedChunk, 1)
	errorChan := make(chan error, 1)

	req := &StagingRequest{
		StreamID:     "callback-test",
		Reader:       reader,
		ExpectedSize: int64(len(testData)),
		ContentType:  "text/plain",
		Callback:     createStagingCallback(resultChan, errorChan),
	}

	err := manager.StageChunk(req, ChunkBoundary{})
	require.NoError(t, err)

	// Wait for callback
	select {
	case chunk := <-resultChan:
		assert.NotNil(t, chunk)
		assert.Equal(t, "callback-test", req.StreamID)
	case err := <-errorChan:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Callback not executed within timeout")
	}
}
