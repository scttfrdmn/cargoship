package pipeline

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// RebalanceConfig configures shard rebalancing
type RebalanceConfig struct {
	// ImbalanceThreshold is the size variance ratio to trigger rebalancing
	// A value of 2.0 means rebalance if largest shard is >2x the average
	ImbalanceThreshold float64

	// MinShardSize is the minimum size (bytes) for a shard to participate in rebalancing
	// Shards smaller than this are always considered for receiving files
	MinShardSize int64

	// DryRun if true, only analyze and report without making changes
	DryRun bool
}

// ShardBalance represents the balance state of shards
type ShardBalance struct {
	TotalSize      int64                // Total size across all shards
	AverageSize    float64              // Average size per shard
	MaxSize        int64                // Size of largest shard
	MinSize        int64                // Size of smallest shard
	SizeVariance   float64              // Max/Avg ratio (imbalance factor)
	IsBalanced     bool                 // True if within threshold
	ShardStats     []ShardStats         // Per-shard statistics
	ImbalanceRatio float64              // Actual imbalance ratio detected
	Recommendation string               // Human-readable recommendation
}

// ShardStats contains statistics for a single shard
type ShardStats struct {
	ShardID          int     // Shard identifier
	FileCount        int64   // Number of files
	UncompressedSize int64   // Total uncompressed size
	CompressedSize   int64   // Total compressed size
	ChunkCount       int     // Number of chunks
	SizeRatio        float64 // Size relative to average (1.0 = average)
	Status           string  // "underloaded", "balanced", "overloaded"
}

// RebalanceResult represents the outcome of a rebalancing operation
type RebalanceResult struct {
	Success         bool               // Whether rebalancing succeeded
	InitialBalance  ShardBalance       // Balance before rebalancing
	FinalBalance    ShardBalance       // Balance after rebalancing
	FilesReassigned int                // Number of files moved between shards
	ShardsAffected  []int              // Shard IDs that were modified
	NewManifest     *manifest.Manifest // Updated manifest
	Recommendation  string             // Human-readable recommendation
	Error           error              // Error if rebalancing failed
}

// AnalyzeShardBalance analyzes the current shard distribution for imbalance
func AnalyzeShardBalance(m *manifest.Manifest, threshold float64) (*ShardBalance, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest cannot be nil")
	}

	if len(m.Shards) == 0 {
		return nil, fmt.Errorf("manifest has no shards")
	}

	// Calculate total size and gather per-shard stats
	var totalSize int64
	shardStats := make([]ShardStats, len(m.Shards))

	for i, shard := range m.Shards {
		shardStats[i] = ShardStats{
			ShardID:          shard.ID,
			FileCount:        shard.FileCount,
			UncompressedSize: shard.UncompressedSize,
			CompressedSize:   shard.CompressedSize,
			ChunkCount:       shard.ChunkCount,
		}
		totalSize += shard.UncompressedSize
	}

	// Calculate average size
	avgSize := float64(totalSize) / float64(len(m.Shards))

	// Find min and max sizes
	var minSize, maxSize int64 = math.MaxInt64, 0
	for _, shard := range m.Shards {
		if shard.UncompressedSize < minSize {
			minSize = shard.UncompressedSize
		}
		if shard.UncompressedSize > maxSize {
			maxSize = shard.UncompressedSize
		}
	}

	// Calculate size ratios and status for each shard
	for i := range shardStats {
		if avgSize > 0 {
			shardStats[i].SizeRatio = float64(shardStats[i].UncompressedSize) / avgSize
		} else {
			shardStats[i].SizeRatio = 0
		}

		// Determine status based on threshold
		if shardStats[i].SizeRatio > threshold {
			shardStats[i].Status = "overloaded"
		} else if shardStats[i].SizeRatio < (1.0 / threshold) {
			shardStats[i].Status = "underloaded"
		} else {
			shardStats[i].Status = "balanced"
		}
	}

	// Calculate imbalance ratio (max/avg)
	var imbalanceRatio float64
	if avgSize > 0 {
		imbalanceRatio = float64(maxSize) / avgSize
	}

	// Determine if balanced
	isBalanced := imbalanceRatio <= threshold

	// Generate recommendation
	var recommendation string
	if isBalanced {
		recommendation = fmt.Sprintf("Shards are well-balanced (%.2fx max/avg ratio)", imbalanceRatio)
	} else {
		recommendation = fmt.Sprintf("Rebalancing recommended: largest shard is %.2fx average (threshold: %.2fx)",
			imbalanceRatio, threshold)
	}

	// Sort shard stats by size (descending) for display
	sort.Slice(shardStats, func(i, j int) bool {
		return shardStats[i].UncompressedSize > shardStats[j].UncompressedSize
	})

	return &ShardBalance{
		TotalSize:      totalSize,
		AverageSize:    avgSize,
		MaxSize:        maxSize,
		MinSize:        minSize,
		SizeVariance:   imbalanceRatio,
		IsBalanced:     isBalanced,
		ShardStats:     shardStats,
		ImbalanceRatio: imbalanceRatio,
		Recommendation: recommendation,
	}, nil
}

// DefaultRebalanceConfig returns default rebalancing configuration
func DefaultRebalanceConfig() *RebalanceConfig {
	return &RebalanceConfig{
		ImbalanceThreshold: 2.0,        // Rebalance if max > 2x average
		MinShardSize:       100 * 1024 * 1024, // 100MB minimum
		DryRun:             false,
	}
}

// RebalanceShards analyzes and rebalances shard distribution
// Returns a RebalanceResult with before/after state and modified manifest
func RebalanceShards(ctx context.Context, m *manifest.Manifest, config *RebalanceConfig) (*RebalanceResult, error) {
	if config == nil {
		config = DefaultRebalanceConfig()
	}

	// Analyze initial balance
	initialBalance, err := AnalyzeShardBalance(m, config.ImbalanceThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze shard balance: %w", err)
	}

	result := &RebalanceResult{
		InitialBalance: *initialBalance,
	}

	// Check if rebalancing is needed
	if initialBalance.IsBalanced {
		result.Success = true
		result.FinalBalance = *initialBalance
		result.Recommendation = "No rebalancing needed - shards are already balanced"
		return result, nil
	}

	// If dry run, return analysis without making changes
	if config.DryRun {
		result.Success = true
		result.FinalBalance = *initialBalance
		result.Recommendation = fmt.Sprintf(
			"DRY RUN: Would rebalance %d files from overloaded shards (imbalance: %.2fx)",
			0, initialBalance.ImbalanceRatio)
		return result, nil
	}

	// TODO: Implement actual rebalancing algorithm
	// This will be implemented in subsequent phases:
	// 1. Identify files to move from overloaded shards
	// 2. Redistribute files to underloaded shards
	// 3. Re-upload affected chunks
	// 4. Update manifest with new distribution

	return result, fmt.Errorf("rebalancing implementation not yet complete")
}

// PrintShardBalance prints a human-readable summary of shard balance
func PrintShardBalance(balance *ShardBalance) {
	fmt.Printf("\n📊 Shard Balance Analysis\n")
	fmt.Printf("═════════════════════════════════════════════\n\n")

	fmt.Printf("Total Size:     %.2f GB\n", float64(balance.TotalSize)/(1024*1024*1024))
	fmt.Printf("Average Size:   %.2f GB per shard\n", balance.AverageSize/(1024*1024*1024))
	fmt.Printf("Max Size:       %.2f GB\n", float64(balance.MaxSize)/(1024*1024*1024))
	fmt.Printf("Min Size:       %.2f GB\n", float64(balance.MinSize)/(1024*1024*1024))
	fmt.Printf("Imbalance:      %.2fx (max/avg ratio)\n", balance.ImbalanceRatio)
	fmt.Printf("Status:         ")

	if balance.IsBalanced {
		fmt.Printf("✅ BALANCED\n")
	} else {
		fmt.Printf("⚠️  IMBALANCED\n")
	}

	fmt.Printf("\n💡 %s\n\n", balance.Recommendation)

	fmt.Printf("Per-Shard Breakdown:\n")
	fmt.Printf("─────────────────────────────────────────────\n")
	fmt.Printf("%-8s %-12s %-10s %-8s %s\n", "Shard", "Size (GB)", "Files", "Ratio", "Status")
	fmt.Printf("─────────────────────────────────────────────\n")

	for _, stat := range balance.ShardStats {
		statusIcon := "  "
		if stat.Status == "overloaded" {
			statusIcon = "⚠️ "
		} else if stat.Status == "underloaded" {
			statusIcon = "📥"
		} else {
			statusIcon = "✓ "
		}

		fmt.Printf("%-8d %-12.2f %-10d %-8.2fx %s%s\n",
			stat.ShardID,
			float64(stat.UncompressedSize)/(1024*1024*1024),
			stat.FileCount,
			stat.SizeRatio,
			statusIcon,
			stat.Status,
		)
	}

	fmt.Printf("─────────────────────────────────────────────\n\n")
}
