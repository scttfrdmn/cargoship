package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// WorkloadClass represents the size class of a workload
type WorkloadClass int

const (
	TinyWorkload   WorkloadClass = iota // < 100 MB
	SmallWorkload                       // 100 MB - 1 GB
	MediumWorkload                      // 1 GB - 10 GB
	LargeWorkload                       // 10 GB - 100 GB
	HugeWorkload                        // > 100 GB
)

// String returns the string representation of a workload class
func (wc WorkloadClass) String() string {
	switch wc {
	case TinyWorkload:
		return "Tiny"
	case SmallWorkload:
		return "Small"
	case MediumWorkload:
		return "Medium"
	case LargeWorkload:
		return "Large"
	case HugeWorkload:
		return "Huge"
	default:
		return "Unknown"
	}
}

// AdaptiveShardCalculator determines optimal shard count based on workload
// characteristics and system resources.
type AdaptiveShardCalculator struct {
	cpuCores        int
	availableMemory int64
	workersPerShard int   // Default: 2 workers per shard
	memoryPerShard  int64 // Estimated memory per shard: ~200MB
}

// CalculationResult contains the calculated shard count and detailed rationale
type CalculationResult struct {
	ShardCount       int
	Rationale        string
	Warnings         []string
	FileCount        int64
	RawSize          int64
	CompressedSize   int64
	CompressionRatio float64
	WorkloadClass    string
}

// NewAdaptiveShardCalculator creates a new adaptive shard calculator with system defaults
func NewAdaptiveShardCalculator() *AdaptiveShardCalculator {
	return &AdaptiveShardCalculator{
		cpuCores:        runtime.NumCPU(),
		availableMemory: getAvailableMemory(),
		workersPerShard: 2,
		memoryPerShard:  200 << 20, // 200MB per shard
	}
}

// CalculateOptimalShardCount determines the optimal shard count for a given workload
func (asc *AdaptiveShardCalculator) CalculateOptimalShardCount(
	ctx context.Context,
	sourcePath string,
) (CalculationResult, error) {
	// Step 1: Estimate workload (file count and raw size)
	fileCount, rawSize, err := EstimateWorkload(ctx, sourcePath)
	if err != nil {
		return CalculationResult{}, fmt.Errorf("failed to estimate workload: %w", err)
	}

	if fileCount == 0 {
		return CalculationResult{}, fmt.Errorf("no files found in %s", sourcePath)
	}

	// Step 2: Estimate compression ratio
	compressionRatio, err := asc.estimateCompression(ctx, sourcePath, fileCount)
	if err != nil {
		// Non-fatal: fall back to conservative estimate
		compressionRatio = 0.5 // Assume 50% compression
	}

	compressedSize := int64(float64(rawSize) * compressionRatio)

	// Step 3: Calculate optimal shard count
	shardCount, rationale, warnings := asc.calculateFromWorkload(fileCount, compressedSize)

	return CalculationResult{
		ShardCount:       shardCount,
		Rationale:        rationale,
		Warnings:         warnings,
		FileCount:        fileCount,
		RawSize:          rawSize,
		CompressedSize:   compressedSize,
		CompressionRatio: compressionRatio,
		WorkloadClass:    asc.classifyWorkload(compressedSize).String(),
	}, nil
}

// calculateFromWorkload performs the core shard count calculation
func (asc *AdaptiveShardCalculator) calculateFromWorkload(
	fileCount int64,
	compressedSize int64,
) (int, string, []string) {
	const (
		GB = 1024 * 1024 * 1024
		MB = 1024 * 1024
	)

	var warnings []string

	// Step 1: Calculate base shard count using three factors
	fileShards := int(fileCount / 10_000) // 1 shard per 10k files
	if fileShards == 0 {
		fileShards = 1
	}

	sizeShards := int(compressedSize / (10 * GB)) // 1 shard per 10 GB compressed
	if sizeShards == 0 {
		sizeShards = 1
	}

	cpuShards := asc.cpuCores / 2 // 1 shard per 2 cores
	if cpuShards == 0 {
		cpuShards = 1
	}

	// Take maximum to satisfy all constraints
	baseShards := max(max(fileShards, sizeShards), cpuShards)

	// Step 2: Classify workload and apply range constraints
	workloadClass := asc.classifyWorkload(compressedSize)
	var minShards, maxShards int

	switch workloadClass {
	case TinyWorkload: // < 100 MB
		minShards, maxShards = 4, 8
	case SmallWorkload: // 100 MB - 1 GB
		minShards, maxShards = 4, 12
	case MediumWorkload: // 1 GB - 10 GB
		minShards, maxShards = 4, 16
	case LargeWorkload: // 10 GB - 100 GB
		minShards, maxShards = 8, 24
	case HugeWorkload: // > 100 GB
		minShards, maxShards = 12, 32
	}

	// Clamp to workload-specific range
	shardCount := clamp(baseShards, minShards, maxShards)

	// Step 3: Apply resource constraints
	maxShardsByMemory := int(asc.availableMemory / asc.memoryPerShard)
	if maxShardsByMemory < shardCount {
		warnings = append(warnings, fmt.Sprintf(
			"Memory-constrained: %d GB available, reduced from %d → %d shards",
			asc.availableMemory/(1<<30), shardCount, maxShardsByMemory,
		))
		shardCount = maxShardsByMemory
	}

	maxShardsByCPU := asc.cpuCores * 2 // Max 2 shards per core
	if maxShardsByCPU < shardCount {
		warnings = append(warnings, fmt.Sprintf(
			"CPU-constrained: %d cores, reduced from %d → %d shards",
			asc.cpuCores, shardCount, maxShardsByCPU,
		))
		shardCount = maxShardsByCPU
	}

	// Cap at absolute maximum
	if shardCount > 32 {
		shardCount = 32
	}

	// Ensure minimum of 4 shards
	if shardCount < 4 {
		shardCount = 4
	}

	// Step 4: Validate load balance
	targetChunkSizeMB := 64 // Default chunk size
	estimatedChunks := compressedSize / (int64(targetChunkSizeMB) * MB)
	if estimatedChunks == 0 {
		estimatedChunks = 1
	}

	chunksPerShard := estimatedChunks / int64(shardCount)
	if chunksPerShard < 6 {
		// Reduce shard count to maintain minimum 6 chunks per shard
		optimalShards := int(estimatedChunks / 6)
		if optimalShards < 4 {
			optimalShards = 4
		}
		if optimalShards < shardCount {
			warnings = append(warnings, fmt.Sprintf(
				"Load balance: Reduced from %d → %d shards to maintain 6+ chunks per shard",
				shardCount, optimalShards,
			))
			shardCount = optimalShards
		}
	}

	// Build rationale string
	rationale := asc.buildRationale(
		fileCount,
		compressedSize,
		fileShards,
		sizeShards,
		cpuShards,
		baseShards,
		workloadClass,
		shardCount,
		estimatedChunks,
		minShards,
		maxShards,
	)

	return shardCount, rationale, warnings
}

// classifyWorkload determines the workload size class
func (asc *AdaptiveShardCalculator) classifyWorkload(compressedSize int64) WorkloadClass {
	const (
		MB100 = 100 * 1024 * 1024
		GB1   = 1024 * 1024 * 1024
		GB10  = 10 * 1024 * 1024 * 1024
		GB100 = 100 * 1024 * 1024 * 1024
	)

	switch {
	case compressedSize < MB100:
		return TinyWorkload
	case compressedSize < GB1:
		return SmallWorkload
	case compressedSize < GB10:
		return MediumWorkload
	case compressedSize < GB100:
		return LargeWorkload
	default:
		return HugeWorkload
	}
}

// estimateCompression samples files and estimates compression ratio
func (asc *AdaptiveShardCalculator) estimateCompression(
	ctx context.Context,
	sourcePath string,
	fileCount int64,
) (float64, error) {
	estimator, err := chunking.NewCompressionEstimator()
	if err != nil {
		return 0.5, err // Fall back to 50% compression
	}

	// Sample up to 20 files for compression estimation
	maxSamples := 20
	if fileCount < int64(maxSamples) {
		maxSamples = int(fileCount)
	}

	samples := make([]string, 0, maxSamples)
	sampled := 0

	// Walk directory and collect sample file paths
	err = filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil || d.IsDir() {
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		samples = append(samples, path)
		sampled++

		if sampled >= maxSamples {
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return 0.5, err
	}

	if len(samples) == 0 {
		return 0.5, fmt.Errorf("no files to sample")
	}

	// Estimate compression ratio for samples
	var totalRatio float64
	validSamples := 0

	for _, samplePath := range samples {
		ratio, err := estimator.EstimateCompressionRatio(samplePath)
		if err == nil {
			totalRatio += ratio
			validSamples++
		}
	}

	if validSamples == 0 {
		return 0.5, fmt.Errorf("failed to estimate compression for any samples")
	}

	avgRatio := totalRatio / float64(validSamples)
	return avgRatio, nil
}

// buildRationale creates a detailed explanation of the shard count decision
func (asc *AdaptiveShardCalculator) buildRationale(
	fileCount int64,
	compressedSize int64,
	fileShards int,
	sizeShards int,
	cpuShards int,
	baseShards int,
	workloadClass WorkloadClass,
	finalShards int,
	estimatedChunks int64,
	minShards int,
	maxShards int,
) string {
	const GB = 1024 * 1024 * 1024

	compressedGB := float64(compressedSize) / GB
	chunksPerShard := estimatedChunks / int64(finalShards)
	if chunksPerShard == 0 {
		chunksPerShard = 1
	}

	s3Capacity := finalShards * 3500 // ~3,500 PUT/s per S3 prefix

	rationale := fmt.Sprintf(`Workload Analysis:
  • Files: %d files
  • Estimated compressed: %.2f GB
  • Classification: %s workload

Calculation:
  • File-based: %d ÷ 10,000 = %d shards
  • Size-based: %.2f GB ÷ 10 GB = %d shards
  • CPU-based: %d cores ÷ 2 = %d shards
  • Base: %d shards (maximum)

Constraints:
  • Memory: %d GB ÷ 0.2 GB = %d shards (OK)
  • CPU: %d cores × 2 = %d shards (OK)
  • %s workload range: %d-%d shards

Load Balance:
  • Estimated chunks: ~%d (64 MB each)
  • Per shard: %d ÷ %d = %d chunks/shard

Selected: %d shards
Expected S3 capacity: ~%d PUT/s (3,500 per prefix × %d)`,
		fileCount,
		compressedGB,
		workloadClass,
		fileCount,
		fileShards,
		compressedGB,
		sizeShards,
		asc.cpuCores,
		cpuShards,
		baseShards,
		asc.availableMemory/(1<<30),
		int(asc.availableMemory/asc.memoryPerShard),
		asc.cpuCores,
		asc.cpuCores*2,
		workloadClass,
		minShards,
		maxShards,
		estimatedChunks,
		estimatedChunks,
		finalShards,
		chunksPerShard,
		finalShards,
		s3Capacity,
		finalShards,
	)

	return rationale
}

// getAvailableMemory returns the available system memory using runtime.MemStats
func getAvailableMemory() int64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Heuristic: available = system total - heap allocated
	systemTotal := int64(memStats.Sys)
	heapAlloc := int64(memStats.HeapAlloc)

	available := systemTotal - heapAlloc

	// Minimum 1GB available
	if available < 1<<30 {
		available = 1 << 30
	}

	return available
}

// Helper functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
