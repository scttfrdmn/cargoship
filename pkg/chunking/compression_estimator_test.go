package chunking

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompressionEstimator(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)
	require.NotNil(t, ce)
	assert.Equal(t, int64(4096), ce.sampleSize)
	assert.NotNil(t, ce.cache)
	assert.NotNil(t, ce.encoder)
}

func TestCompressionEstimator_EstimateRatio_HighlyCompressible(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	// Create highly compressible file (repeated text)
	tmpDir := t.TempDir()
	textFile := filepath.Join(tmpDir, "test.txt")
	content := strings.Repeat("This is a test file with repeated content. ", 200)
	err = os.WriteFile(textFile, []byte(content), 0644)
	require.NoError(t, err)

	// Estimate compression ratio
	ratio, err := ce.EstimateCompressionRatio(textFile)
	require.NoError(t, err)

	// Text should compress very well (ratio < 0.3 means >70% compression)
	assert.Less(t, ratio, 0.3, "Expected high compression ratio for repeated text")
}

func TestCompressionEstimator_EstimateRatio_Incompressible(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	// Create incompressible file (random data)
	tmpDir := t.TempDir()
	randomFile := filepath.Join(tmpDir, "random.bin")
	randomData := make([]byte, 8192)
	_, err = rand.Read(randomData)
	require.NoError(t, err)
	err = os.WriteFile(randomFile, randomData, 0644)
	require.NoError(t, err)

	// Estimate compression ratio
	ratio, err := ce.EstimateCompressionRatio(randomFile)
	require.NoError(t, err)

	// Random data should compress poorly (ratio > 0.9 means <10% compression)
	assert.Greater(t, ratio, 0.9, "Expected poor compression ratio for random data")
}

func TestCompressionEstimator_EstimateRatio_Caching(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	// Create two .txt files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	content := strings.Repeat("test ", 1000)
	err = os.WriteFile(file1, []byte(content), 0644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte(content), 0644)
	require.NoError(t, err)

	// First call should compute and cache
	ratio1, err := ce.EstimateCompressionRatio(file1)
	require.NoError(t, err)

	// Check cache
	stats := ce.GetCacheStats()
	assert.Equal(t, 1, stats["cached_extensions"].(int))

	// Second call with same extension should use cache
	ratio2, err := ce.EstimateCompressionRatio(file2)
	require.NoError(t, err)

	// Ratios should be identical (from cache)
	assert.Equal(t, ratio1, ratio2, "Expected cached ratio to be reused")
}

func TestCompressionEstimator_EstimateCompressedSize(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	textFile := filepath.Join(tmpDir, "test.txt")
	content := strings.Repeat("test content ", 500)
	err = os.WriteFile(textFile, []byte(content), 0644)
	require.NoError(t, err)

	// Get file size
	fileInfo, err := os.Stat(textFile)
	require.NoError(t, err)
	originalSize := fileInfo.Size()

	// Estimate compressed size
	compressedSize, err := ce.EstimateCompressedSize(textFile, originalSize)
	require.NoError(t, err)

	// Compressed size should be significantly smaller than original
	assert.Less(t, compressedSize, originalSize)
	assert.Less(t, compressedSize, originalSize/2, "Expected at least 50% compression")
}

func TestCompressionEstimator_EstimateCompressedSize_EmptyFile(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	err = os.WriteFile(emptyFile, []byte{}, 0644)
	require.NoError(t, err)

	// Estimate compressed size for empty file
	compressedSize, err := ce.EstimateCompressedSize(emptyFile, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), compressedSize)
}

func TestCompressionEstimator_EstimateRatio_SmallFile(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	smallFile := filepath.Join(tmpDir, "small.txt")
	content := "small"
	err = os.WriteFile(smallFile, []byte(content), 0644)
	require.NoError(t, err)

	// Estimate compression ratio for file smaller than sample size
	ratio, err := ce.EstimateCompressionRatio(smallFile)
	require.NoError(t, err)

	// Should handle small files gracefully
	// Note: compression overhead can make small files larger (ratio > 1.0)
	assert.Greater(t, ratio, 0.0)
	// Allow ratio > 1.0 for very small files due to compression header overhead
}

func TestCompressionEstimator_ClearCache(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	textFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(textFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Populate cache
	_, err = ce.EstimateCompressionRatio(textFile)
	require.NoError(t, err)

	stats := ce.GetCacheStats()
	assert.Equal(t, 1, stats["cached_extensions"].(int))

	// Clear cache
	ce.ClearCache()

	stats = ce.GetCacheStats()
	assert.Equal(t, 0, stats["cached_extensions"].(int))
}

func TestCompressionEstimator_GetCacheStats(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	// Create files with different extensions
	txtFile := filepath.Join(tmpDir, "test.txt")
	jsonFile := filepath.Join(tmpDir, "test.json")
	binFile := filepath.Join(tmpDir, "test.bin")

	err = os.WriteFile(txtFile, []byte("text content"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(jsonFile, []byte(`{"key": "value"}`), 0644)
	require.NoError(t, err)
	err = os.WriteFile(binFile, []byte{0, 1, 2, 3, 4}, 0644)
	require.NoError(t, err)

	// Estimate all files
	_, err = ce.EstimateCompressionRatio(txtFile)
	require.NoError(t, err)
	_, err = ce.EstimateCompressionRatio(jsonFile)
	require.NoError(t, err)
	_, err = ce.EstimateCompressionRatio(binFile)
	require.NoError(t, err)

	// Check cache stats
	stats := ce.GetCacheStats()
	assert.Equal(t, 3, stats["cached_extensions"].(int))

	cache := stats["cache"].(map[string]float64)
	assert.Contains(t, cache, ".txt")
	assert.Contains(t, cache, ".json")
	assert.Contains(t, cache, ".bin")
}

func TestCompressionEstimator_EstimateRatio_NonexistentFile(t *testing.T) {
	ce, err := NewCompressionEstimator()
	require.NoError(t, err)

	// Try to estimate ratio for nonexistent file
	ratio, err := ce.EstimateCompressionRatio("/nonexistent/file.txt")
	assert.Error(t, err)
	assert.Equal(t, 1.0, ratio, "Should return 1.0 (no compression) on error")
}

// Benchmark tests
func BenchmarkCompressionEstimator_EstimateRatio_CacheMiss(b *testing.B) {
	ce, err := NewCompressionEstimator()
	require.NoError(b, err)

	tmpDir := b.TempDir()
	content := strings.Repeat("benchmark test content ", 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		testFile := filepath.Join(tmpDir, "test-"+strconv.Itoa(i)+".unique")
		err := os.WriteFile(testFile, []byte(content), 0644)
		require.NoError(b, err)
		b.StartTimer()

		_, err = ce.EstimateCompressionRatio(testFile)
		require.NoError(b, err)
	}
}

func BenchmarkCompressionEstimator_EstimateRatio_CacheHit(b *testing.B) {
	ce, err := NewCompressionEstimator()
	require.NoError(b, err)

	tmpDir := b.TempDir()
	content := strings.Repeat("benchmark test content ", 500)

	// Create multiple files with same extension
	files := make([]string, 100)
	for i := 0; i < 100; i++ {
		files[i] = filepath.Join(tmpDir, "test-"+strconv.Itoa(i)+".txt")
		err := os.WriteFile(files[i], []byte(content), 0644)
		require.NoError(b, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ce.EstimateCompressionRatio(files[i%100])
		require.NoError(b, err)
	}
}

func BenchmarkCompressionEstimator_EstimateCompressedSize(b *testing.B) {
	ce, err := NewCompressionEstimator()
	require.NoError(b, err)

	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := strings.Repeat("test content ", 500)
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(b, err)

	fileInfo, err := os.Stat(testFile)
	require.NoError(b, err)
	fileSize := fileInfo.Size()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ce.EstimateCompressedSize(testFile, fileSize)
		require.NoError(b, err)
	}
}
