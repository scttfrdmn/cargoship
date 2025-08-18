package staging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugSimilarityDetection(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.SimilarityThreshold = 0.5 // Lower threshold for testing
	config.ChunkSizeThreshold = 10   // Very low threshold for testing
	config.RollingWindowSize = 8     // Small window for better granularity
	deduplicator := NewChunkDeduplicator(config)
	
	// Create test data with high similarity (most windows should be identical)
	baseData := []byte("The quick brown fox jumps over the lazy dog and runs through the meadow")
	modifiedData := []byte("The quick brown fox jumps over the lazy cat and runs through the meadow")
	
	t.Logf("Base data length: %d", len(baseData))
	t.Logf("Modified data length: %d", len(modifiedData))
	
	// Store base chunk
	result1 := deduplicator.AnalyzeChunk(baseData, "text/plain")
	t.Logf("Result1: Action=%d", result1.RecommendedAction)
	
	// Check internal state
	deduplicator.mu.RLock()
	t.Logf("Stored chunks: %d", len(deduplicator.chunkHashes))
	t.Logf("Size index entries: %d", len(deduplicator.sizeIndex))
	t.Logf("Weak hash entries: %d", len(deduplicator.weakHashMap))
	deduplicator.mu.RUnlock()
	
	// Analyze modified chunk
	result2 := deduplicator.AnalyzeChunk(modifiedData, "text/plain")
	t.Logf("Result2: IsDuplicate=%v, SimilarityScore=%f, Action=%d", 
		result2.IsDuplicate, result2.SimilarityScore, result2.RecommendedAction)
	
	// Test shingles generation directly
	shingles1 := deduplicator.generateShingles(baseData)
	shingles2 := deduplicator.generateShingles(modifiedData)
	
	t.Logf("Shingles1 count: %d", len(shingles1))
	t.Logf("Shingles2 count: %d", len(shingles2))
	
	if len(shingles1) > 0 && len(shingles2) > 0 {
		// Debug shingle contents
		t.Logf("First few shingles1: %v", getFirstFewKeys(shingles1, 5))
		t.Logf("First few shingles2: %v", getFirstFewKeys(shingles2, 5))
		
		// Check for common shingles
		common := 0
		for hash := range shingles1 {
			if shingles2[hash] {
				common++
			}
		}
		t.Logf("Common shingles: %d", common)
		
		similarity := deduplicator.jaccardSimilarity(shingles1, shingles2)
		t.Logf("Direct Jaccard similarity: %f", similarity)
		
		// More lenient test for now
		if common > 0 {
			assert.True(t, similarity > 0, "Expected some similarity when common shingles exist")
		}
	}
}

// Helper function to get first few keys from a map
func getFirstFewKeys(m map[uint64]bool, limit int) []uint64 {
	keys := make([]uint64, 0, limit)
	count := 0
	for k := range m {
		if count >= limit {
			break
		}
		keys = append(keys, k)
		count++
	}
	return keys
}

func TestDebugWeakHashCollision(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.EnableWeakHashing = true
	deduplicator := NewChunkDeduplicator(config)
	
	baseData := []byte("test data for weak hash collision detection with sufficient length")
	similarData := []byte("test data for weak hash collision detection with sufficient size")
	
	// Check weak hashes
	weakHash1 := deduplicator.computeWeakHash(baseData)
	weakHash2 := deduplicator.computeWeakHash(similarData)
	
	t.Logf("Weak hash 1: %d", weakHash1)
	t.Logf("Weak hash 2: %d", weakHash2)
	
	// Store first chunk
	result1 := deduplicator.AnalyzeChunk(baseData, "text/plain")
	t.Logf("First chunk stored, action: %d", result1.RecommendedAction)
	
	// Check if weak hash was stored
	deduplicator.mu.RLock()
	candidates := deduplicator.weakHashMap[weakHash2]
	t.Logf("Candidates for weak hash %d: %d", weakHash2, len(candidates))
	deduplicator.mu.RUnlock()
	
	// Analyze second chunk
	result2 := deduplicator.AnalyzeChunk(similarData, "text/plain")
	t.Logf("Second chunk result: similarity=%f, action=%d", result2.SimilarityScore, result2.RecommendedAction)
}

func TestDebugSizeIndexing(t *testing.T) {
	config := DefaultDeduplicationConfig()
	config.EnableSimilarityDetection = true
	deduplicator := NewChunkDeduplicator(config)
	
	// Create data of similar sizes
	data1 := make([]byte, 1000)
	data2 := make([]byte, 1050) // Within 10% size variance
	
	for i := range data1 {
		data1[i] = byte(i % 256)
	}
	for i := range data2 {
		data2[i] = byte(i % 256)
	}
	
	// Store first chunk
	result1 := deduplicator.AnalyzeChunk(data1, "application/test")
	t.Logf("First chunk stored: %d bytes, action: %d", len(data1), result1.RecommendedAction)
	
	// Check size indexing
	deduplicator.mu.RLock()
	sizeEntries := 0
	for size, chunks := range deduplicator.sizeIndex {
		t.Logf("Size %d has %d chunks", size, len(chunks))
		sizeEntries += len(chunks)
	}
	deduplicator.mu.RUnlock()
	t.Logf("Total size index entries: %d", sizeEntries)
	
	// Analyze second chunk
	result2 := deduplicator.AnalyzeChunk(data2, "application/test")
	t.Logf("Second chunk result: similarity=%f, bytes_saved=%d", result2.SimilarityScore, result2.BytesSaved)
}