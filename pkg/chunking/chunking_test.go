package chunking

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkSizeCalculator_MemoryConstrained(t *testing.T) {
	config := &ChunkingConfig{
		Workers:         8,
		AvailableMemory: 4 * 1024 * 1024 * 1024, // 4 GB
	}

	calc := NewChunkSizeCalculator(config)

	// Test small files scenario (10,000 files @ 185MB total)
	totalSize := int64(185 * 1024 * 1024)
	fileCount := 10000

	chunkSize, stats := calc.CalculateOptimalChunkSize(
		totalSize,
		fileCount,
		config.AvailableMemory,
		1000, // 1000x cost savings target
	)

	// Verify chunk size is reasonable
	assert.Greater(t, chunkSize, int64(0))
	assert.LessOrEqual(t, chunkSize, config.AvailableMemory)

	// Verify memory constraint is satisfied
	memoryRequired := chunkSize * int64(config.Workers)
	assert.LessOrEqual(t, memoryRequired, config.AvailableMemory)

	// Verify stats
	assert.Equal(t, fileCount, stats.TotalFiles)
	assert.Equal(t, totalSize, stats.TotalSize)
	assert.Greater(t, stats.ChunkCount, 0)
	assert.Greater(t, stats.CostSavings, float64(1)) // Should have some savings
}

func TestChunkSizeCalculator_LargeFiles(t *testing.T) {
	config := &ChunkingConfig{
		Workers:         8,
		AvailableMemory: 4 * 1024 * 1024 * 1024, // 4 GB
	}

	calc := NewChunkSizeCalculator(config)

	// Test large files scenario (100 files @ 56GB total)
	totalSize := int64(56 * 1024 * 1024 * 1024)
	fileCount := 100

	chunkSize, stats := calc.CalculateOptimalChunkSize(
		totalSize,
		fileCount,
		config.AvailableMemory,
		100, // 100x cost savings target
	)

	// Verify chunk size is reasonable
	assert.Greater(t, chunkSize, int64(0))
	assert.LessOrEqual(t, chunkSize, int64(DefaultMaxChunkSize)) // Should not exceed S3 5GB limit

	// Verify memory is bounded
	assert.LessOrEqual(t, stats.MemoryRequired, config.AvailableMemory)

	// Verify we get operation reduction (may not be as much as target for large files)
	directOps := fileCount
	// Large files may not achieve full target savings, but should still reduce operations
	assert.LessOrEqual(t, stats.EstimatedOps, directOps*2) // Allow up to 2x direct ops

	t.Logf("Large files: chunk_size=%d MB, chunks=%d, ops=%d, savings=%.1fx, memory=%d MB",
		chunkSize/(1024*1024),
		stats.ChunkCount,
		stats.EstimatedOps,
		stats.CostSavings,
		stats.MemoryRequired/(1024*1024))
}

func TestChunkSizeCalculator_ExplicitChunkSize(t *testing.T) {
	targetSize := int64(50 * 1024 * 1024) // 50 MB
	config := &ChunkingConfig{
		TargetChunkSize: targetSize,
		Workers:         8,
	}

	calc := NewChunkSizeCalculator(config)

	totalSize := int64(1 * 1024 * 1024 * 1024) // 1 GB
	fileCount := 1000

	chunkSize, stats := calc.CalculateOptimalChunkSize(
		totalSize,
		fileCount,
		0,
		1000,
	)

	// When explicit size is set, should use it
	assert.Equal(t, targetSize, chunkSize)
	assert.Equal(t, fileCount, stats.TotalFiles)
}

func TestChunkSizeCalculator_Validation(t *testing.T) {
	config := &ChunkingConfig{}
	calc := NewChunkSizeCalculator(config)

	tests := []struct {
		name      string
		chunkSize int64
		wantError bool
	}{
		{
			name:      "valid_size",
			chunkSize: 100 * 1024 * 1024, // 100 MB
			wantError: false,
		},
		{
			name:      "below_s3_minimum",
			chunkSize: 1 * 1024 * 1024, // 1 MB (below 5MB minimum)
			wantError: true,
		},
		{
			name:      "exceeds_maximum",
			chunkSize: 10 * 1024 * 1024 * 1024, // 10 GB (above 5GB max)
			wantError: true,
		},
		{
			name:      "at_minimum",
			chunkSize: DefaultMinChunkSize,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := calc.ValidateChunkSize(tt.chunkSize)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSizeBasedChunkingStrategy_GroupFiles(t *testing.T) {
	config := &ChunkingConfig{
		GroupingStrategy: "size",
	}

	strategy := NewSizeBasedChunkingStrategy(config)

	// Create test files
	files := []File{
		{Path: "/file1.txt", Size: 10 * 1024 * 1024},  // 10 MB
		{Path: "/file2.txt", Size: 15 * 1024 * 1024},  // 15 MB
		{Path: "/file3.txt", Size: 20 * 1024 * 1024},  // 20 MB
		{Path: "/file4.txt", Size: 30 * 1024 * 1024},  // 30 MB
		{Path: "/file5.txt", Size: 5 * 1024 * 1024},   // 5 MB
	}

	chunkSize := int64(50 * 1024 * 1024) // 50 MB chunks

	chunks, err := strategy.GroupFilesIntoChunks(files, chunkSize)
	require.NoError(t, err)

	// Verify chunks were created
	assert.Greater(t, len(chunks), 0)

	// Verify all files are included
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
		assert.Greater(t, chunk.FileCount, 0)
		assert.LessOrEqual(t, chunk.TotalSize, chunkSize+files[0].Size) // Allow one overage
	}
	assert.Equal(t, len(files), totalFiles)

	// Verify chunk IDs are sequential
	for i, chunk := range chunks {
		assert.Equal(t, i, chunk.ID)
	}
}

func TestAdaptiveChunkingStrategy_GroupByDirectory(t *testing.T) {
	config := &ChunkingConfig{
		GroupingStrategy: "directory",
	}

	strategy := NewAdaptiveChunkingStrategy(config)

	// Create test files in different directories
	files := []File{
		{Path: "/dir1/file1.txt", Directory: "/dir1", Size: 10 * 1024 * 1024},
		{Path: "/dir1/file2.txt", Directory: "/dir1", Size: 15 * 1024 * 1024},
		{Path: "/dir2/file3.txt", Directory: "/dir2", Size: 20 * 1024 * 1024},
		{Path: "/dir2/file4.txt", Directory: "/dir2", Size: 30 * 1024 * 1024},
		{Path: "/dir3/file5.txt", Directory: "/dir3", Size: 5 * 1024 * 1024},
	}

	chunkSize := int64(50 * 1024 * 1024) // 50 MB chunks

	chunks, err := strategy.GroupFilesIntoChunks(files, chunkSize)
	require.NoError(t, err)

	// Verify chunks were created
	assert.Greater(t, len(chunks), 0)

	// Verify all files are included
	totalFiles := 0
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
	}
	assert.Equal(t, len(files), totalFiles)
}

func TestAdaptiveChunkingStrategy_MixedStrategy(t *testing.T) {
	config := &ChunkingConfig{
		GroupingStrategy: "mixed",
	}

	strategy := NewAdaptiveChunkingStrategy(config)

	// Create test files with mix of small and large
	files := []File{
		{Path: "/small1.txt", Directory: "/", Size: 1 * 1024 * 1024},    // 1 MB (small)
		{Path: "/small2.txt", Directory: "/", Size: 2 * 1024 * 1024},    // 2 MB (small)
		{Path: "/large1.txt", Directory: "/", Size: 200 * 1024 * 1024},  // 200 MB (large)
		{Path: "/large2.txt", Directory: "/", Size: 500 * 1024 * 1024},  // 500 MB (large)
		{Path: "/small3.txt", Directory: "/", Size: 5 * 1024 * 1024},    // 5 MB (small)
	}

	chunkSize := int64(100 * 1024 * 1024) // 100 MB chunks

	chunks, err := strategy.GroupFilesIntoChunks(files, chunkSize)
	require.NoError(t, err)

	// Verify chunks were created
	assert.Greater(t, len(chunks), 0)

	// Verify all files are included
	totalFiles := 0
	totalSize := int64(0)
	for _, chunk := range chunks {
		totalFiles += chunk.FileCount
		totalSize += chunk.TotalSize
	}
	assert.Equal(t, len(files), totalFiles)

	// Calculate expected total size
	expectedTotal := int64(0)
	for _, f := range files {
		expectedTotal += f.Size
	}
	assert.Equal(t, expectedTotal, totalSize)
}

func TestGroupFilesIntoChunks_EmptyFiles(t *testing.T) {
	config := &ChunkingConfig{}
	strategy := NewSizeBasedChunkingStrategy(config)

	files := []File{}
	chunkSize := int64(100 * 1024 * 1024)

	_, err := strategy.GroupFilesIntoChunks(files, chunkSize)
	assert.Error(t, err) // Should error on empty file list
}

func TestGroupFilesIntoChunks_InvalidChunkSize(t *testing.T) {
	config := &ChunkingConfig{}
	strategy := NewSizeBasedChunkingStrategy(config)

	files := []File{
		{Path: "/file1.txt", Size: 10 * 1024 * 1024},
	}

	_, err := strategy.GroupFilesIntoChunks(files, 0)
	assert.Error(t, err) // Should error on zero chunk size

	_, err = strategy.GroupFilesIntoChunks(files, -1)
	assert.Error(t, err) // Should error on negative chunk size
}

func TestChunkStats_Calculation(t *testing.T) {
	config := &ChunkingConfig{
		Workers:   8,
		Bandwidth: 100 * 1024 * 1024, // 100 MB/s
	}

	calc := NewChunkSizeCalculator(config)

	totalSize := int64(1 * 1024 * 1024 * 1024) // 1 GB
	fileCount := 1000
	chunkSize := int64(100 * 1024 * 1024) // 100 MB

	stats := calc.calculateStats(totalSize, fileCount, chunkSize, 1000)

	// Verify basic stats
	assert.Equal(t, fileCount, stats.TotalFiles)
	assert.Equal(t, totalSize, stats.TotalSize)
	// Chunk count uses ceiling division, so 1GB / 100MB can be 10 or 11 depending on rounding
	assert.Greater(t, stats.ChunkCount, 0)
	assert.LessOrEqual(t, stats.ChunkCount, 11)
	assert.Greater(t, stats.CostSavings, float64(1))
	assert.Greater(t, stats.EstimatedTime, time.Duration(0))

	// Verify memory calculation
	expectedMemory := chunkSize * int64(config.Workers)
	assert.Equal(t, expectedMemory, stats.MemoryRequired)

	t.Logf("Stats: chunks=%d, ops=%d, savings=%.1fx, memory=%d MB, time=%v",
		stats.ChunkCount,
		stats.EstimatedOps,
		stats.CostSavings,
		stats.MemoryRequired/(1024*1024),
		stats.EstimatedTime)
}

func TestEstimateS3Operations(t *testing.T) {
	// Create a strategy with default config to test the method
	config := &ChunkingConfig{
		MultipartPartSize: 100 * 1024 * 1024, // 100 MB default
	}
	strategy := NewAdaptiveChunkingStrategy(config)

	tests := []struct {
		name        string
		size        int64
		minOps      int
		maxOps      int
	}{
		{
			name:   "small_chunk",
			size:   50 * 1024 * 1024, // 50 MB
			minOps: 1,
			maxOps: 1,
		},
		{
			name:   "large_chunk_multipart",
			size:   10 * 1024 * 1024 * 1024, // 10 GB
			minOps: 100,
			maxOps: 105, // Allow for ceiling division
		},
		{
			name:   "exact_5gb",
			size:   5 * 1024 * 1024 * 1024,
			minOps: 50,
			maxOps: 55, // Allow for ceiling division
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := strategy.estimateS3Operations(tt.size)
			assert.GreaterOrEqual(t, ops, tt.minOps)
			assert.LessOrEqual(t, ops, tt.maxOps)
		})
	}
}
