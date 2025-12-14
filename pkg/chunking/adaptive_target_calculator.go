package chunking

import (
	"fmt"
)

// AdaptiveTargetCalculator calculates optimal chunk size based on workload characteristics
// and AWS S3 cost optimization principles.
type AdaptiveTargetCalculator struct {
	// Thresholds for workload size classification (in bytes)
	smallWorkloadThreshold  int64 // Default: 1GB
	mediumWorkloadThreshold int64 // Default: 10GB

	// Target chunk sizes for each workload class (in MB)
	smallWorkloadChunkSizeMB  int // Default: 10MB (maximize load balance)
	mediumWorkloadChunkSizeMB int // Default: 20MB (balance cost and performance)
	largeWorkloadChunkSizeMB  int // Default: 64MB (minimize API cost)

	// Shard configuration for load balancing validation
	shardCount        int // Number of S3 prefixes/shards
	minChunksPerShard int // Minimum chunks per shard for good balance
}

// NewAdaptiveTargetCalculator creates a new adaptive target calculator with default settings
func NewAdaptiveTargetCalculator() *AdaptiveTargetCalculator {
	return &AdaptiveTargetCalculator{
		smallWorkloadThreshold:    1 * 1024 * 1024 * 1024,  // 1GB
		mediumWorkloadThreshold:   10 * 1024 * 1024 * 1024, // 10GB
		smallWorkloadChunkSizeMB:  10,                      // 10MB for <1GB
		mediumWorkloadChunkSizeMB: 20,                      // 20MB for 1-10GB
		largeWorkloadChunkSizeMB:  64,                      // 64MB for >10GB
		shardCount:                8,                       // Default 8 shards
		minChunksPerShard:         6,                       // Minimum 6 chunks per shard
	}
}

// NewAdaptiveTargetCalculatorWithConfig creates a calculator with custom configuration
func NewAdaptiveTargetCalculatorWithConfig(
	smallThresholdGB int,
	mediumThresholdGB int,
	smallChunkMB int,
	mediumChunkMB int,
	largeChunkMB int,
	shardCount int,
) *AdaptiveTargetCalculator {
	return &AdaptiveTargetCalculator{
		smallWorkloadThreshold:    int64(smallThresholdGB) * 1024 * 1024 * 1024,
		mediumWorkloadThreshold:   int64(mediumThresholdGB) * 1024 * 1024 * 1024,
		smallWorkloadChunkSizeMB:  smallChunkMB,
		mediumWorkloadChunkSizeMB: mediumChunkMB,
		largeWorkloadChunkSizeMB:  largeChunkMB,
		shardCount:                shardCount,
		minChunksPerShard:         6,
	}
}

// CalculateOptimalChunkSize determines the optimal chunk size based on estimated total compressed size
// Returns chunk size in MB
func (atc *AdaptiveTargetCalculator) CalculateOptimalChunkSize(estimatedTotalCompressedSize int64) int {
	// Classify workload size
	if estimatedTotalCompressedSize < atc.smallWorkloadThreshold {
		// Small workload (<1GB): Use small chunks for maximum load balance
		return atc.smallWorkloadChunkSizeMB
	} else if estimatedTotalCompressedSize <= atc.mediumWorkloadThreshold {
		// Medium workload (1GB-10GB): Use medium chunks (balanced approach)
		return atc.mediumWorkloadChunkSizeMB
	} else {
		// Large workload (>10GB): Use large chunks for minimum API cost
		return atc.largeWorkloadChunkSizeMB
	}
}

// CalculateOptimalChunkSizeWithRationale returns both the chunk size and explanation
func (atc *AdaptiveTargetCalculator) CalculateOptimalChunkSizeWithRationale(
	estimatedTotalCompressedSize int64,
) (chunkSizeMB int, rationale string) {
	chunkSizeMB = atc.CalculateOptimalChunkSize(estimatedTotalCompressedSize)

	totalGB := float64(estimatedTotalCompressedSize) / (1024 * 1024 * 1024)
	estimatedChunks := estimatedTotalCompressedSize / (int64(chunkSizeMB) * 1024 * 1024)
	chunksPerShard := estimatedChunks / int64(atc.shardCount)

	if estimatedTotalCompressedSize < atc.smallWorkloadThreshold {
		rationale = fmt.Sprintf(
			"Small workload (%.2f GB compressed): Using %dMB chunks for maximum load balance. "+
				"Expected ~%d chunks (~%d per shard). "+
				"API cost: negligible.",
			totalGB, chunkSizeMB, estimatedChunks, chunksPerShard,
		)
	} else if estimatedTotalCompressedSize < atc.mediumWorkloadThreshold {
		apiCostPer1000 := 0.005
		estimatedAPICost := float64(estimatedChunks) * apiCostPer1000 / 1000.0
		rationale = fmt.Sprintf(
			"Medium workload (%.2f GB compressed): Using %dMB chunks for balanced cost/performance. "+
				"Expected ~%d chunks (~%d per shard). "+
				"API cost: ~$%.4f.",
			totalGB, chunkSizeMB, estimatedChunks, chunksPerShard, estimatedAPICost,
		)
	} else {
		apiCostPer1000 := 0.005
		estimatedAPICost := float64(estimatedChunks) * apiCostPer1000 / 1000.0
		savingsVs20MB := (estimatedTotalCompressedSize/(20*1024*1024) - estimatedChunks) * int64(apiCostPer1000) / 1000
		rationale = fmt.Sprintf(
			"Large workload (%.2f GB compressed): Using %dMB chunks for minimum API cost. "+
				"Expected ~%d chunks (~%d per shard). "+
				"API cost: ~$%.4f (saves ~$%.4f vs 20MB chunks).",
			totalGB, chunkSizeMB, estimatedChunks, chunksPerShard, estimatedAPICost, float64(savingsVs20MB)/1000.0,
		)
	}

	return chunkSizeMB, rationale
}

// ValidateLoadBalancing checks if the calculated chunk size will provide good load balancing
// Returns true if load balancing is acceptable, false otherwise with warning message
func (atc *AdaptiveTargetCalculator) ValidateLoadBalancing(
	estimatedTotalCompressedSize int64,
	chunkSizeMB int,
) (bool, string) {
	estimatedChunks := estimatedTotalCompressedSize / (int64(chunkSizeMB) * 1024 * 1024)
	chunksPerShard := estimatedChunks / int64(atc.shardCount)

	minChunks := int64(atc.shardCount * atc.minChunksPerShard)

	if estimatedChunks < minChunks {
		return false, fmt.Sprintf(
			"WARNING: Only %d chunks for %d shards (~%d per shard). "+
				"Recommend at least %d total chunks (%d per shard) for good load balance. "+
				"Consider reducing chunk size or increasing workload size.",
			estimatedChunks, atc.shardCount, chunksPerShard,
			minChunks, atc.minChunksPerShard,
		)
	}

	if chunksPerShard < int64(atc.minChunksPerShard) {
		return false, fmt.Sprintf(
			"WARNING: Only ~%d chunks per shard (total: %d chunks, %d shards). "+
				"Recommend at least %d chunks per shard for good load balance.",
			chunksPerShard, estimatedChunks, atc.shardCount, atc.minChunksPerShard,
		)
	}

	return true, fmt.Sprintf(
		"Load balancing OK: %d chunks for %d shards (~%d per shard).",
		estimatedChunks, atc.shardCount, chunksPerShard,
	)
}

// EstimateAPICost calculates estimated AWS S3 API cost for a workload
// Returns cost in USD
func (atc *AdaptiveTargetCalculator) EstimateAPICost(
	estimatedTotalCompressedSize int64,
	chunkSizeMB int,
) float64 {
	estimatedChunks := estimatedTotalCompressedSize / (int64(chunkSizeMB) * 1024 * 1024)
	apiCostPer1000 := 0.005 // $0.005 per 1000 PUT requests (2025 AWS pricing)
	return float64(estimatedChunks) * apiCostPer1000 / 1000.0
}

// CompareAPICosts compares API costs between different chunk sizes
func (atc *AdaptiveTargetCalculator) CompareAPICosts(estimatedTotalCompressedSize int64) string {
	cost10MB := atc.EstimateAPICost(estimatedTotalCompressedSize, 10)
	cost20MB := atc.EstimateAPICost(estimatedTotalCompressedSize, 20)
	cost64MB := atc.EstimateAPICost(estimatedTotalCompressedSize, 64)

	optimal := atc.CalculateOptimalChunkSize(estimatedTotalCompressedSize)
	optimalCost := atc.EstimateAPICost(estimatedTotalCompressedSize, optimal)

	totalGB := float64(estimatedTotalCompressedSize) / (1024 * 1024 * 1024)

	return fmt.Sprintf(
		"API Cost Comparison for %.2f GB workload:\n"+
			"  10MB chunks: $%.4f\n"+
			"  20MB chunks: $%.4f (%.0f%% savings vs 10MB)\n"+
			"  64MB chunks: $%.4f (%.0f%% savings vs 20MB)\n"+
			"  Optimal (%dMB): $%.4f",
		totalGB,
		cost10MB,
		cost20MB, (1-cost20MB/cost10MB)*100,
		cost64MB, (1-cost64MB/cost20MB)*100,
		optimal, optimalCost,
	)
}

// GetConfig returns the current configuration
func (atc *AdaptiveTargetCalculator) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"small_workload_threshold_gb":   float64(atc.smallWorkloadThreshold) / (1024 * 1024 * 1024),
		"medium_workload_threshold_gb":  float64(atc.mediumWorkloadThreshold) / (1024 * 1024 * 1024),
		"small_workload_chunk_size_mb":  atc.smallWorkloadChunkSizeMB,
		"medium_workload_chunk_size_mb": atc.mediumWorkloadChunkSizeMB,
		"large_workload_chunk_size_mb":  atc.largeWorkloadChunkSizeMB,
		"shard_count":                   atc.shardCount,
		"min_chunks_per_shard":          atc.minChunksPerShard,
	}
}
