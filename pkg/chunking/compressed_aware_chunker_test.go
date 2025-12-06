package chunking

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompressedAwareChunker(t *testing.T) {
	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)
	require.NotNil(t, chunker)
	assert.NotNil(t, chunker.estimator)
	assert.NotNil(t, chunker.targetCalc)
}

func TestCompressedAwareChunker_CreateChunks_EmptyFiles(t *testing.T) {
	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	chunks, err := chunker.CreateChunks([]File{})
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestCompressedAwareChunker_CreateChunks_SmallWorkload(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 10 files of 10MB each (100MB total, compresses to ~10-20MB)
	files := make([]File, 10)
	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".txt")
		content := strings.Repeat("test data ", 1024*1024) // ~10MB compressible data
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	chunks, err := chunker.CreateChunks(files)
	require.NoError(t, err)

	// Small workload should use 10MB chunks
	assert.GreaterOrEqual(t, len(chunks), 1, "Should create at least 1 chunk")
	assert.Equal(t, 10, chunker.GetLastOptimalChunkSize(), "Should use 10MB chunks for small workload")

	// Verify all files are included
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
	}
	assert.Equal(t, 10, totalFiles, "All files should be included")
}

func TestCompressedAwareChunker_CreateChunks_LargeWorkload(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 100 files of 200MB each (20GB total, compresses to ~2-4GB)
	files := make([]File, 100)
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(tmpDir, "large"+string(rune('0'+i))+".txt")

		// Create file but don't actually write 200MB (just simulate size)
		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    200 * 1024 * 1024, // 200MB simulated
			ModTime: time.Now(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	// Note: This will estimate based on actual file (tiny), so total estimated size will be small
	// But we can still test the chunking logic
	chunks, err := chunker.CreateChunks(files)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(chunks), 1, "Should create at least 1 chunk")

	// Verify all files are included
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
	}
	assert.Equal(t, 100, totalFiles, "All files should be included")
}

func TestCompressedAwareChunker_CreateChunksWithMetadata_SmallWorkload(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 5 files with compressible content
	files := make([]File, 5)
	for i := 0; i < 5; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".txt")
		content := strings.Repeat("compressible data ", 500000) // ~10MB
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	result, err := chunker.CreateChunksWithMetadata(files)
	require.NoError(t, err)

	// Validate result structure
	assert.GreaterOrEqual(t, len(result.Chunks), 1, "Should create at least 1 chunk")
	assert.Equal(t, len(result.Chunks), len(result.ChunkMetadata), "Should have metadata for each chunk")
	assert.Equal(t, 5, result.TotalFiles)
	assert.Greater(t, result.TotalRawSize, int64(0))
	assert.Greater(t, result.TotalEstimatedCompressedSize, int64(0))
	assert.Less(t, result.AverageCompressionRatio, 1.0, "Should compress well")
	assert.NotEmpty(t, result.Rationale)

	// Validate chunk metadata
	for i, metadata := range result.ChunkMetadata {
		assert.Equal(t, i, metadata.ChunkID)
		assert.Greater(t, metadata.FileCount, 0)
		assert.Greater(t, metadata.TotalRawSize, int64(0))
		assert.Greater(t, metadata.EstimatedCompressedSize, int64(0))
		assert.Less(t, metadata.CompressionRatio, 1.0, "Each chunk should compress")
	}

	t.Logf("Chunking Result:")
	t.Logf("  Total Files: %d", result.TotalFiles)
	t.Logf("  Total Raw Size: %d MB", result.TotalRawSize/(1024*1024))
	t.Logf("  Estimated Compressed: %d MB", result.TotalEstimatedCompressedSize/(1024*1024))
	t.Logf("  Average Compression: %.2f%%", (1-result.AverageCompressionRatio)*100)
	t.Logf("  Optimal Chunk Size: %d MB", result.OptimalChunkSizeMB)
	t.Logf("  Rationale: %s", result.Rationale)
	t.Logf("  Load Balanced: %v", result.LoadBalanced)
	t.Logf("  Load Balance Message: %s", result.LoadBalanceMessage)
}

func TestCompressedAwareChunker_CreateChunksWithMetadata_MixedCompressibility(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mixed files: some highly compressible, some not
	files := make([]File, 6)

	// 3 text files (highly compressible)
	for i := 0; i < 3; i++ {
		filePath := filepath.Join(tmpDir, "text"+string(rune('0'+i))+".txt")
		content := strings.Repeat("this is highly compressible text ", 300000) // ~10MB
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	// 3 "random" files (less compressible - we'll use varied text to simulate)
	for i := 3; i < 6; i++ {
		filePath := filepath.Join(tmpDir, "data"+string(rune('0'+i-3))+".bin")
		// Simulate less compressible data with more entropy
		content := ""
		for j := 0; j < 10000; j++ {
			content += string(rune(j%256)) + string(rune((j*7)%256)) + string(rune((j*13)%256))
		}
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	result, err := chunker.CreateChunksWithMetadata(files)
	require.NoError(t, err)

	assert.Equal(t, 6, result.TotalFiles)
	assert.Greater(t, result.TotalRawSize, int64(0))

	// Log compression ratios for different file types
	t.Logf("Mixed Compressibility Test:")
	t.Logf("  Total Files: %d", result.TotalFiles)
	t.Logf("  Average Compression Ratio: %.2f", result.AverageCompressionRatio)

	for i, metadata := range result.ChunkMetadata {
		t.Logf("  Chunk %d: %d files, %.2f compression ratio",
			i, metadata.FileCount, metadata.CompressionRatio)
	}
}

func TestCompressedAwareChunker_CreateChunks_SingleLargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one very large file (simulated)
	filePath := filepath.Join(tmpDir, "huge.txt")
	content := strings.Repeat("x", 1024) // Small actual content
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	files := []File{
		{
			Path:    filePath,
			Size:    100 * 1024 * 1024 * 1024, // 100GB simulated
			ModTime: time.Now(),
		},
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	chunks, err := chunker.CreateChunks(files)
	require.NoError(t, err)

	// Should create exactly 1 chunk with 1 file
	assert.Equal(t, 1, len(chunks))
	assert.Equal(t, 1, chunks[0].FileCount)
	assert.Equal(t, files[0].Path, chunks[0].Files[0].Path)
}

func TestCompressedAwareChunker_CreateChunks_ManySmallFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 50 small files (1KB each = 50KB total)
	files := make([]File, 50)
	for i := 0; i < 50; i++ {
		filePath := filepath.Join(tmpDir, "small"+string(rune('0'+i%10))+".txt")
		content := strings.Repeat("a", 1024) // 1KB
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	chunks, err := chunker.CreateChunks(files)
	require.NoError(t, err)

	// Very small total size should result in 1 chunk with all files
	assert.GreaterOrEqual(t, len(chunks), 1)

	// Verify all files are included
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
	}
	assert.Equal(t, 50, totalFiles)
}

func TestCompressedAwareChunker_GetLastOptimalChunkSize(t *testing.T) {
	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	// Initial value should be 0
	assert.Equal(t, 0, chunker.GetLastOptimalChunkSize())

	// Create some chunks
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := strings.Repeat("test ", 1000000)
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	fileInfo, err := os.Stat(filePath)
	require.NoError(t, err)

	files := []File{{
		Path:    filePath,
		Size:    fileInfo.Size(),
		ModTime: fileInfo.ModTime(),
	}}

	_, err = chunker.CreateChunks(files)
	require.NoError(t, err)

	// Should now have a calculated optimal chunk size
	optimalSize := chunker.GetLastOptimalChunkSize()
	assert.Greater(t, optimalSize, 0)
	t.Logf("Optimal chunk size: %d MB", optimalSize)
}

func TestCompressedAwareChunker_GetCompressionStats(t *testing.T) {
	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	// Create files with different extensions
	extensions := []string{".txt", ".json", ".log"}
	files := make([]File, 3)

	for i, ext := range extensions {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i))+ext)
		content := strings.Repeat("test content ", 10000)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	// Create chunks (will populate cache)
	_, err = chunker.CreateChunks(files)
	require.NoError(t, err)

	// Get compression stats
	stats := chunker.GetCompressionStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "cached_extensions")
	assert.Contains(t, stats, "cache")

	cachedExtensions := stats["cached_extensions"].(int)
	assert.Equal(t, 3, cachedExtensions, "Should have cached 3 extensions")

	cache := stats["cache"].(map[string]float64)
	assert.Contains(t, cache, ".txt")
	assert.Contains(t, cache, ".json")
	assert.Contains(t, cache, ".log")

	t.Logf("Compression Stats: %+v", stats)
}

func TestCompressedAwareChunker_ClearCompressionCache(t *testing.T) {
	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := strings.Repeat("test ", 10000)
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	fileInfo, err := os.Stat(filePath)
	require.NoError(t, err)

	files := []File{{
		Path:    filePath,
		Size:    fileInfo.Size(),
		ModTime: fileInfo.ModTime(),
	}}

	// Create chunks (populates cache)
	_, err = chunker.CreateChunks(files)
	require.NoError(t, err)

	stats := chunker.GetCompressionStats()
	assert.Equal(t, 1, stats["cached_extensions"].(int))

	// Clear cache
	chunker.ClearCompressionCache()

	stats = chunker.GetCompressionStats()
	assert.Equal(t, 0, stats["cached_extensions"].(int))
}

func TestCompressedAwareChunker_ChunkSizeUniformity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 20 files with varying raw sizes but similar compressed sizes
	files := make([]File, 20)
	for i := 0; i < 20; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i%10))+".txt")
		// All files have similar content (will compress similarly)
		content := strings.Repeat("uniform compressible content ", 100000) // ~3MB each
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(t, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(t, err)

	result, err := chunker.CreateChunksWithMetadata(files)
	require.NoError(t, err)

	// Check that chunks have relatively uniform estimated compressed sizes
	if len(result.ChunkMetadata) > 1 {
		sizes := make([]int64, len(result.ChunkMetadata))
		for i, metadata := range result.ChunkMetadata {
			sizes[i] = metadata.EstimatedCompressedSize
		}

		// Calculate variance in chunk sizes
		var sum int64
		for _, size := range sizes {
			sum += size
		}
		avgSize := sum / int64(len(sizes))

		maxDeviation := int64(0)
		for _, size := range sizes {
			deviation := size - avgSize
			if deviation < 0 {
				deviation = -deviation
			}
			if deviation > maxDeviation {
				maxDeviation = deviation
			}
		}

		// Max deviation should be less than 50% of average (reasonably uniform)
		percentDeviation := float64(maxDeviation) / float64(avgSize) * 100
		assert.Less(t, percentDeviation, 50.0, "Chunk sizes should be relatively uniform")

		t.Logf("Chunk Size Uniformity:")
		t.Logf("  Average Chunk Size: %d MB", avgSize/(1024*1024))
		t.Logf("  Max Deviation: %d MB (%.1f%%)", maxDeviation/(1024*1024), percentDeviation)
		for i, size := range sizes {
			t.Logf("  Chunk %d: %d MB", i, size/(1024*1024))
		}
	}
}

// Benchmark tests
func BenchmarkCompressedAwareChunker_CreateChunks_100Files(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 100 files
	files := make([]File, 100)
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i%10))+".txt")
		content := strings.Repeat("benchmark test ", 10000)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(b, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chunker.CreateChunks(files)
		require.NoError(b, err)
	}
}

func BenchmarkCompressedAwareChunker_CreateChunksWithMetadata_100Files(b *testing.B) {
	tmpDir := b.TempDir()

	files := make([]File, 100)
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i%10))+".txt")
		content := strings.Repeat("benchmark test ", 10000)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(b, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chunker.CreateChunksWithMetadata(files)
		require.NoError(b, err)
	}
}

func BenchmarkCompressedAwareChunker_CreateChunks_CachedExtensions(b *testing.B) {
	tmpDir := b.TempDir()

	// All files have same extension (will benefit from caching)
	files := make([]File, 50)
	for i := 0; i < 50; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('0'+i%10))+".txt")
		content := strings.Repeat("test ", 10000)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err)

		fileInfo, err := os.Stat(filePath)
		require.NoError(b, err)

		files[i] = File{
			Path:    filePath,
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime(),
		}
	}

	chunker, err := NewCompressedAwareChunker()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chunker.CreateChunks(files)
		require.NoError(b, err)
	}
}
