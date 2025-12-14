package chunking

import (
	"fmt"
	"math"
	"time"
)

const (
	// Default constraints
	DefaultMinChunkSize = 10 * 1024 * 1024       // 10 MB
	DefaultMaxChunkSize = 5 * 1024 * 1024 * 1024 // 5 GB (S3 limit)
	DefaultBandwidth    = 100 * 1024 * 1024      // 100 MB/s
	DefaultTargetTime   = 5                      // 5 seconds per chunk

	// S3 multipart minimum
	S3MultipartMinimum = 5 * 1024 * 1024 // 5 MB
)

// ChunkSizeCalculator provides methods for calculating optimal chunk sizes
type ChunkSizeCalculator struct {
	config *ChunkingConfig
}

// NewChunkSizeCalculator creates a new calculator with the given configuration
func NewChunkSizeCalculator(config *ChunkingConfig) *ChunkSizeCalculator {
	// Apply defaults
	if config.MinChunkSize == 0 {
		config.MinChunkSize = DefaultMinChunkSize
	}
	if config.MaxChunkSize == 0 {
		config.MaxChunkSize = DefaultMaxChunkSize
	}
	if config.Bandwidth == 0 {
		config.Bandwidth = DefaultBandwidth
	}
	if config.Workers == 0 {
		config.Workers = 8 // Default to 8 workers
	}
	if config.CostSavingsTarget == 0 {
		config.CostSavingsTarget = 1000 // Default to 1000x savings
	}

	return &ChunkSizeCalculator{
		config: config,
	}
}

// CalculateOptimalChunkSize determines the optimal chunk size using multi-constraint optimization
func (c *ChunkSizeCalculator) CalculateOptimalChunkSize(
	totalSize int64,
	fileCount int,
	availableMemory int64,
	costSavingsTarget float64,
) (int64, ChunkStats) {
	// If target chunk size is explicitly set, use it (with bounds checking)
	if c.config.TargetChunkSize > 0 {
		chunkSize := c.clampChunkSize(c.config.TargetChunkSize)
		return chunkSize, c.calculateStats(totalSize, fileCount, chunkSize, costSavingsTarget)
	}

	// Method 1: Memory-constrained optimal
	memoryOptimal := c.calculateMemoryConstrainedSize(availableMemory)

	// Method 2: Cost-optimal (minimize S3 operations)
	costOptimal := c.calculateCostOptimalSize(totalSize, fileCount, costSavingsTarget)

	// Method 3: Performance-optimal (maximize throughput)
	perfOptimal := c.calculatePerformanceOptimalSize()

	// Method 4: Adaptive - take the minimum that satisfies all constraints
	// This ensures we don't exceed any constraint
	chunkSize := c.selectOptimalSize(memoryOptimal, costOptimal, perfOptimal)

	// Clamp to min/max bounds
	chunkSize = c.clampChunkSize(chunkSize)

	// Calculate statistics
	stats := c.calculateStats(totalSize, fileCount, chunkSize, costSavingsTarget)

	return chunkSize, stats
}

// calculateMemoryConstrainedSize calculates optimal size based on memory constraints
// Formula: C = (M × 0.8) / P
// where M = available memory, P = parallel workers
func (c *ChunkSizeCalculator) calculateMemoryConstrainedSize(availableMemory int64) int64 {
	if availableMemory <= 0 {
		// If no memory limit specified, use a reasonable default (4 GB)
		availableMemory = 4 * 1024 * 1024 * 1024
	}

	// Use 80% of available memory to leave headroom
	usableMemory := int64(float64(availableMemory) * 0.8)

	// Divide by number of workers
	chunkSize := usableMemory / int64(c.config.Workers)

	return chunkSize
}

// calculateCostOptimalSize calculates optimal size to achieve cost savings target
// Formula: C = (S × savings_target) / N
// where S = total size, N = number of files
func (c *ChunkSizeCalculator) calculateCostOptimalSize(
	totalSize int64,
	fileCount int,
	costSavingsTarget float64,
) int64 {
	if fileCount == 0 {
		return c.config.MinChunkSize
	}

	// Calculate chunk size to achieve target operation reduction
	// If we have N files and want to reduce to N/target operations,
	// each chunk should contain target files worth of data
	averageFileSize := totalSize / int64(fileCount)
	chunkSize := int64(float64(averageFileSize) * costSavingsTarget)

	// Alternative calculation: total_size / (files / target)
	targetOps := float64(fileCount) / costSavingsTarget
	if targetOps < 1 {
		targetOps = 1
	}
	alternativeSize := totalSize / int64(targetOps)

	// Use the smaller of the two approaches (more conservative)
	if alternativeSize < chunkSize {
		chunkSize = alternativeSize
	}

	return chunkSize
}

// calculatePerformanceOptimalSize calculates optimal size for maximum throughput
// Formula: C = BANDWIDTH × TARGET_TIME
func (c *ChunkSizeCalculator) calculatePerformanceOptimalSize() int64 {
	// Calculate how much data can be processed in the target time
	chunkSize := c.config.Bandwidth * DefaultTargetTime
	return chunkSize
}

// selectOptimalSize selects the best chunk size from multiple candidates
// Uses the minimum that satisfies all constraints (most conservative)
func (c *ChunkSizeCalculator) selectOptimalSize(memoryOptimal, costOptimal, perfOptimal int64) int64 {
	// Start with the minimum of all three
	candidates := []int64{memoryOptimal, costOptimal, perfOptimal}

	minSize := candidates[0]
	for _, size := range candidates[1:] {
		if size > 0 && size < minSize {
			minSize = size
		}
	}

	// Ensure it's at least the practical minimum
	if minSize < c.config.MinChunkSize {
		minSize = c.config.MinChunkSize
	}

	return minSize
}

// clampChunkSize ensures chunk size is within min/max bounds
func (c *ChunkSizeCalculator) clampChunkSize(size int64) int64 {
	if size < c.config.MinChunkSize {
		return c.config.MinChunkSize
	}
	if size > c.config.MaxChunkSize {
		return c.config.MaxChunkSize
	}
	// Ensure it meets S3 multipart minimum
	if size < S3MultipartMinimum {
		return S3MultipartMinimum
	}
	return size
}

// calculateStats generates statistics about the chunking decision
func (c *ChunkSizeCalculator) calculateStats(
	totalSize int64,
	fileCount int,
	chunkSize int64,
	costSavingsTarget float64,
) ChunkStats {
	// Calculate number of chunks
	chunkCount := int(math.Ceil(float64(totalSize) / float64(chunkSize)))
	if chunkCount == 0 {
		chunkCount = 1
	}

	// Calculate average, min, max chunk sizes
	averageChunkSize := totalSize / int64(chunkCount)
	lastChunkSize := totalSize % chunkSize
	if lastChunkSize == 0 {
		lastChunkSize = chunkSize
	}

	minChunkSize := lastChunkSize
	maxChunkSize := chunkSize

	// Estimate S3 operations (each chunk may need multipart upload)
	// Assume each chunk < 5GB uses 1 operation, >= 5GB uses multipart
	estimatedOps := chunkCount
	if chunkSize >= DefaultMaxChunkSize {
		// Multipart upload: divide by 100MB parts
		partsPerChunk := int(math.Ceil(float64(chunkSize) / (100 * 1024 * 1024)))
		estimatedOps = chunkCount * partsPerChunk
	}

	// Calculate cost savings
	directOps := fileCount // Direct upload would be 1 op per file
	actualSavings := float64(directOps) / float64(estimatedOps)

	// Calculate memory required (chunk_size × workers)
	memoryRequired := chunkSize * int64(c.config.Workers)

	// Estimate processing time
	// Time = (total_size / bandwidth) / workers + overhead
	uploadTime := float64(totalSize) / float64(c.config.Bandwidth)
	parallelUploadTime := uploadTime / float64(c.config.Workers)
	overheadPerChunk := 0.5 // 500ms overhead per chunk
	totalOverhead := float64(chunkCount) * overheadPerChunk
	estimatedTime := time.Duration(parallelUploadTime+totalOverhead) * time.Second

	return ChunkStats{
		TotalFiles:       fileCount,
		TotalSize:        totalSize,
		ChunkCount:       chunkCount,
		AverageChunkSize: averageChunkSize,
		MinChunkSize:     minChunkSize,
		MaxChunkSize:     maxChunkSize,
		EstimatedOps:     estimatedOps,
		CostSavings:      actualSavings,
		MemoryRequired:   memoryRequired,
		EstimatedTime:    estimatedTime,
	}
}

// ValidateChunkSize validates that a chunk size is acceptable
func (c *ChunkSizeCalculator) ValidateChunkSize(chunkSize int64) error {
	if chunkSize < S3MultipartMinimum {
		return fmt.Errorf("chunk size %d is below S3 minimum %d", chunkSize, S3MultipartMinimum)
	}
	if chunkSize < c.config.MinChunkSize {
		return fmt.Errorf("chunk size %d is below configured minimum %d", chunkSize, c.config.MinChunkSize)
	}
	if chunkSize > c.config.MaxChunkSize {
		return fmt.Errorf("chunk size %d exceeds configured maximum %d", chunkSize, c.config.MaxChunkSize)
	}
	return nil
}
