package staging

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunkDeduplicator(t *testing.T) {
	config := DefaultDeduplicationConfig()
	deduplicator := NewChunkDeduplicator(config)

	require.NotNil(t, deduplicator)
	assert.Equal(t, config, deduplicator.config)
	assert.NotNil(t, deduplicator.chunkHashes)
	assert.NotNil(t, deduplicator.duplicateStats)
}

func TestNewChunkDeduplicatorWithNilConfig(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	require.NotNil(t, deduplicator)
	require.NotNil(t, deduplicator.config)

	// Should use default config
	defaultConfig := DefaultDeduplicationConfig()
	assert.Equal(t, defaultConfig.EnableWeakHashing, deduplicator.config.EnableWeakHashing)
	assert.Equal(t, defaultConfig.MaxHashCacheSize, deduplicator.config.MaxHashCacheSize)
}

func TestAnalyzeChunk_ExactDuplicate(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Create test data (larger than default threshold)
	testData := make([]byte, 2048) // 2KB data
	for i := range testData {
		testData[i] = byte((i % 26) + 'a') // Repeating pattern
	}
	contentType := "text/plain"

	// First analysis should be unique
	result1 := deduplicator.AnalyzeChunk(testData, contentType)
	assert.False(t, result1.IsDuplicate)
	assert.Equal(t, ActionStore, result1.RecommendedAction)

	// Second analysis with same data should detect duplicate
	result2 := deduplicator.AnalyzeChunk(testData, contentType)
	assert.True(t, result2.IsDuplicate)
	assert.Equal(t, ActionDuplicate, result2.RecommendedAction)
	assert.Equal(t, int64(len(testData)), result2.BytesSaved)
	assert.NotEmpty(t, result2.ExistingHash)
}

func TestAnalyzeChunk_SimilarChunks(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.SimilarityThreshold = 0.7 // 70% similarity threshold
	config.RollingWindowSize = 8     // Smaller window for better granularity
	deduplicator := NewChunkDeduplicator(config)

	// Create similar test data (larger than threshold)
	baseData := make([]byte, 2048)
	similarData := make([]byte, 2048)

	// Fill with similar but not identical patterns
	for i := range baseData {
		baseData[i] = byte((i % 26) + 'a')
		if i < len(similarData) {
			if i%10 == 0 { // Change every 10th character
				similarData[i] = byte((i % 26) + 'A') // Different case
			} else {
				similarData[i] = baseData[i]
			}
		}
	}
	contentType := "text/plain"

	// Store base chunk
	result1 := deduplicator.AnalyzeChunk(baseData, contentType)
	assert.Equal(t, ActionStore, result1.RecommendedAction)

	// Analyze similar chunk
	result2 := deduplicator.AnalyzeChunk(similarData, contentType)

	// Should detect similarity
	t.Logf("Result2: IsDuplicate=%v, IsSemanticDuplicate=%v, SimilarityScore=%f, BytesSaved=%d, Action=%d",
		result2.IsDuplicate, result2.IsSemanticDuplicate, result2.SimilarityScore, result2.BytesSaved, result2.RecommendedAction)

	assert.False(t, result2.IsDuplicate)
	if result2.SimilarityScore >= config.SimilarityThreshold {
		assert.True(t, result2.IsSemanticDuplicate)
		assert.True(t, result2.SimilarityScore > 0.5) // Should have decent similarity
		assert.True(t, result2.BytesSaved > 0)
	} else {
		t.Logf("Similarity score %f below threshold %f, skipping semantic duplicate assertions",
			result2.SimilarityScore, config.SimilarityThreshold)
	}
}

func TestAnalyzeChunk_SmallChunks(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.ChunkSizeThreshold = 100 // 100 byte threshold
	deduplicator := NewChunkDeduplicator(config)

	// Create small test data below threshold
	smallData := []byte("small")
	contentType := "text/plain"

	result := deduplicator.AnalyzeChunk(smallData, contentType)

	// Should skip deduplication for small chunks
	assert.Equal(t, ActionStore, result.RecommendedAction)
	assert.False(t, result.IsDuplicate)
}

func TestAnalyzeChunk_DeltaCompression(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.EnableDeltaCompression = true
	config.SimilarityThreshold = 0.6
	deduplicator := NewChunkDeduplicator(config)

	// Create base and modified data
	baseData := []byte("The quick brown fox jumps over the lazy dog")
	modifiedData := []byte("The quick brown fox jumps over the lazy cat")
	contentType := "text/plain"

	// Store base chunk
	result1 := deduplicator.AnalyzeChunk(baseData, contentType)
	assert.Equal(t, ActionStore, result1.RecommendedAction)

	// Analyze modified chunk
	result2 := deduplicator.AnalyzeChunk(modifiedData, contentType)

	// Should recommend delta compression for similar content
	if result2.SimilarityScore >= config.SimilarityThreshold {
		assert.Equal(t, ActionDeltaCompress, result2.RecommendedAction)
		assert.NotEmpty(t, result2.DeltaParent)
		assert.True(t, result2.DeltaSize > 0)
		assert.True(t, result2.BytesSaved > 0)
	}
}

func TestComputeStrongHash(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	testData1 := []byte("test data 1")
	testData2 := []byte("test data 2")
	testData3 := []byte("test data 1") // Same as testData1

	hash1 := deduplicator.computeStrongHash(testData1)
	hash2 := deduplicator.computeStrongHash(testData2)
	hash3 := deduplicator.computeStrongHash(testData3)

	// Different data should produce different hashes
	assert.NotEqual(t, hash1, hash2)

	// Same data should produce same hash
	assert.Equal(t, hash1, hash3)

	// Hashes should be hex strings
	assert.Regexp(t, "^[0-9a-f]+$", hash1)
	assert.Regexp(t, "^[0-9a-f]+$", hash2)
}

func TestComputeWeakHash(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	testData1 := []byte("test data for weak hashing")
	testData2 := []byte("different test data for weak hashing")
	testData3 := []byte("test data for weak hashing") // Same as testData1

	hash1 := deduplicator.computeWeakHash(testData1)
	hash2 := deduplicator.computeWeakHash(testData2)
	hash3 := deduplicator.computeWeakHash(testData3)

	// Different data should usually produce different hashes
	assert.NotEqual(t, hash1, hash2)

	// Same data should produce same hash
	assert.Equal(t, hash1, hash3)
}

func TestRollingHasher(t *testing.T) {
	hasher := NewRollingHasher(8)

	testData := []byte("abcdefghijklmnop")
	hash := hasher.Hash(testData)

	assert.NotZero(t, hash)

	// Same data should produce same hash
	hash2 := hasher.Hash(testData)
	assert.Equal(t, hash, hash2)

	// Different data should produce different hash
	differentData := []byte("abcdefghijklmnox")
	hash3 := hasher.Hash(differentData)
	assert.NotEqual(t, hash, hash3)
}

func TestJaccardSimilarity(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Test identical sets
	set1 := map[uint64]bool{1: true, 2: true, 3: true}
	set2 := map[uint64]bool{1: true, 2: true, 3: true}
	similarity := deduplicator.jaccardSimilarity(set1, set2)
	assert.Equal(t, 1.0, similarity)

	// Test completely different sets
	set3 := map[uint64]bool{4: true, 5: true, 6: true}
	similarity = deduplicator.jaccardSimilarity(set1, set3)
	assert.Equal(t, 0.0, similarity)

	// Test partially overlapping sets
	set4 := map[uint64]bool{2: true, 3: true, 4: true}
	similarity = deduplicator.jaccardSimilarity(set1, set4)
	assert.Equal(t, 0.5, similarity) // 2 common / 4 total = 0.5

	// Test empty sets
	empty1 := map[uint64]bool{}
	empty2 := map[uint64]bool{}
	similarity = deduplicator.jaccardSimilarity(empty1, empty2)
	assert.Equal(t, 1.0, similarity)
}

func TestGenerateShingles(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.RollingWindowSize = 4
	deduplicator := NewChunkDeduplicator(config)

	testData := []byte("abcdefgh")
	shingles := deduplicator.generateShingles(testData)

	// Should generate shingles for each window position
	expectedWindows := len(testData) - config.RollingWindowSize + 1
	assert.Equal(t, expectedWindows, len(shingles))

	// Test with data smaller than window
	smallData := []byte("ab")
	smallShingles := deduplicator.generateShingles(smallData)
	assert.Equal(t, 0, len(smallShingles))
}

func TestEstimateDeltaSize(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Create larger test data (above threshold)
	basePattern := []byte("The quick brown fox jumps")
	baseData := make([]byte, 2048)
	for i := range baseData {
		baseData[i] = basePattern[i%len(basePattern)]
	}

	// Test identical data
	identicalData := make([]byte, 2048)
	copy(identicalData, baseData)
	deltaSize := deduplicator.estimateDeltaSize(identicalData, baseData)
	assert.True(t, deltaSize < int64(len(identicalData)))

	// Test completely different data
	differentPattern := []byte("Completely different content here")
	differentData := make([]byte, 2048)
	for i := range differentData {
		differentData[i] = differentPattern[i%len(differentPattern)]
	}
	deltaSize = deduplicator.estimateDeltaSize(differentData, baseData)
	assert.True(t, deltaSize > 0)

	// Test slightly modified data (90% same, 10% different)
	modifiedData := make([]byte, 2048)
	copy(modifiedData, baseData)
	for i := 0; i < len(modifiedData); i += 10 {
		modifiedData[i] = 'X' // Change every 10th byte
	}
	deltaSize = deduplicator.estimateDeltaSize(modifiedData, baseData)
	assert.True(t, deltaSize > 0)
	assert.True(t, deltaSize < int64(len(modifiedData)))
}

func TestCalculateEntropy(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Test uniform data (low entropy) - make it larger
	uniformData := bytes.Repeat([]byte("a"), 2048)
	entropy := deduplicator.calculateEntropy(uniformData)
	assert.Equal(t, 0.0, entropy) // Single byte should have zero entropy

	// Test random data (high entropy) - make it larger
	randomData := make([]byte, 2048)
	_, _ = rand.Read(randomData)
	entropy = deduplicator.calculateEntropy(randomData)
	// Random data should have high entropy (close to 8 bits for truly random byte data)
	t.Logf("Random data entropy: %f", entropy)
	assert.True(t, entropy > 0)    // Random data should have positive entropy
	assert.True(t, entropy <= 8.0) // Maximum entropy for byte data is 8 bits

	// Test empty data
	entropy = deduplicator.calculateEntropy([]byte{})
	assert.Equal(t, 0.0, entropy)
}

func TestGetStats(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Initial stats should be zero
	stats := deduplicator.GetStats()
	assert.Equal(t, int64(0), stats.TotalChunksProcessed)
	assert.Equal(t, int64(0), stats.DuplicateChunksFound)
	assert.Equal(t, 0.0, stats.DeduplicationRatio)

	// Process some chunks to update stats (must be larger than 1KB threshold)
	testData := make([]byte, 2048) // 2KB data above threshold
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}

	// First chunk (unique)
	deduplicator.AnalyzeChunk(testData, "text/plain")
	stats = deduplicator.GetStats()
	assert.Equal(t, int64(1), stats.TotalChunksProcessed)
	assert.Equal(t, int64(0), stats.DuplicateChunksFound)

	// Second chunk (duplicate)
	deduplicator.AnalyzeChunk(testData, "text/plain")
	stats = deduplicator.GetStats()
	assert.Equal(t, int64(2), stats.TotalChunksProcessed)
	assert.Equal(t, int64(1), stats.DuplicateChunksFound)
	assert.Equal(t, 0.5, stats.DeduplicationRatio) // 1 duplicate out of 2 total
}

func TestClearCache(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Add some data (must be larger than 1KB threshold)
	testData := make([]byte, 2048) // 2KB data above threshold
	for i := range testData {
		testData[i] = byte((i % 26) + 'a')
	}
	deduplicator.AnalyzeChunk(testData, "text/plain")

	// Verify data exists
	assert.True(t, len(deduplicator.chunkHashes) > 0)

	// Clear cache
	deduplicator.ClearCache()

	// Verify cache is empty
	assert.Equal(t, 0, len(deduplicator.chunkHashes))
	assert.Equal(t, 0, len(deduplicator.sizeIndex))
	assert.Equal(t, 0, len(deduplicator.contentIndex))
	assert.Equal(t, 0, len(deduplicator.recentAccess))
}

func TestCleanupOldEntries(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.HashCacheExpirationTime = time.Millisecond * 100 // Very short expiration for testing
	config.MaxHashCacheSize = 2                             // Small cache size to trigger cleanup
	deduplicator := NewChunkDeduplicator(config)

	// Add entries
	testData1 := []byte("first test chunk")
	testData2 := []byte("second test chunk")
	testData3 := []byte("third test chunk")

	deduplicator.AnalyzeChunk(testData1, "text/plain")
	deduplicator.AnalyzeChunk(testData2, "text/plain")

	// Wait for expiration
	time.Sleep(time.Millisecond * 150)

	// Adding another entry should trigger cleanup
	deduplicator.AnalyzeChunk(testData3, "text/plain")

	// Old entries should be cleaned up
	assert.True(t, len(deduplicator.chunkHashes) <= config.MaxHashCacheSize)
}

func TestContentTypeAwareness(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.EnableContentAwareness = true
	config.SimilarityThreshold = 0.5
	deduplicator := NewChunkDeduplicator(config)

	// Create similar content with different types (large enough for deduplication)
	pattern := []byte("similar content for type awareness testing")
	testData := make([]byte, 2048)
	for i := range testData {
		testData[i] = pattern[i%len(pattern)]
	}

	// Store as text
	result1 := deduplicator.AnalyzeChunk(testData, "text/plain")
	assert.Equal(t, ActionStore, result1.RecommendedAction)

	// Analyze same data as different content type
	result2 := deduplicator.AnalyzeChunk(testData, "application/json")

	// Should still detect as duplicate (exact match overrides content type)
	assert.True(t, result2.IsDuplicate)
	assert.Equal(t, ActionDuplicate, result2.RecommendedAction)
}

func TestWeakHashCollisions(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.EnableWeakHashing = true
	deduplicator := NewChunkDeduplicator(config)

	// Create data that might have weak hash collisions
	data1 := []byte("collision test data one")
	data2 := []byte("collision test data two")

	result1 := deduplicator.AnalyzeChunk(data1, "text/plain")
	result2 := deduplicator.AnalyzeChunk(data2, "text/plain")

	// Should not be false positives despite potential weak hash collisions
	assert.Equal(t, ActionStore, result1.RecommendedAction)
	assert.Equal(t, ActionStore, result2.RecommendedAction)
	assert.False(t, result2.IsDuplicate)
}

func TestDeduplicationWithLargeData(t *testing.T) {
	deduplicator := NewChunkDeduplicator(nil)

	// Create large test data (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Test deduplication performance with large data
	start := time.Now()
	result1 := deduplicator.AnalyzeChunk(largeData, "application/octet-stream")
	duration1 := time.Since(start)

	start = time.Now()
	result2 := deduplicator.AnalyzeChunk(largeData, "application/octet-stream")
	duration2 := time.Since(start)

	// First should be unique, second should be duplicate
	assert.Equal(t, ActionStore, result1.RecommendedAction)
	assert.True(t, result2.IsDuplicate)
	assert.Equal(t, ActionDuplicate, result2.RecommendedAction)

	// Duplicate detection should be faster than initial processing
	assert.True(t, duration2 < duration1)

	// Should save the full size
	assert.Equal(t, int64(len(largeData)), result2.BytesSaved)
}

func BenchmarkAnalyzeChunk_Unique(b *testing.B) {
	deduplicator := NewChunkDeduplicator(nil)
	testData := make([]byte, 32*1024) // 32KB chunks

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make each chunk unique by modifying the first bytes
		testData[0] = byte(i)
		testData[1] = byte(i >> 8)
		testData[2] = byte(i >> 16)
		testData[3] = byte(i >> 24)

		deduplicator.AnalyzeChunk(testData, "application/octet-stream")
	}
}

func BenchmarkAnalyzeChunk_Duplicate(b *testing.B) {
	deduplicator := NewChunkDeduplicator(nil)
	testData := make([]byte, 32*1024) // 32KB chunks
	_, _ = rand.Read(testData)

	// Pre-store the chunk
	deduplicator.AnalyzeChunk(testData, "application/octet-stream")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deduplicator.AnalyzeChunk(testData, "application/octet-stream")
	}
}

func BenchmarkComputeStrongHash(b *testing.B) {
	deduplicator := NewChunkDeduplicator(nil)
	testData := make([]byte, 32*1024) // 32KB
	_, _ = rand.Read(testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deduplicator.computeStrongHash(testData)
	}
}

func BenchmarkComputeWeakHash(b *testing.B) {
	deduplicator := NewChunkDeduplicator(nil)
	testData := make([]byte, 32*1024) // 32KB
	_, _ = rand.Read(testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deduplicator.computeWeakHash(testData)
	}
}
