package pipeline

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"

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
	Plan            *RebalancePlan     // Detailed rebalancing plan
	Error           error              // Error if rebalancing failed
}

// RebalancePlan represents a plan for redistributing files between shards
type RebalancePlan struct {
	Moves          []FileMove      // Individual file movements
	TargetBalance  ShardBalance    // Expected balance after rebalancing
	TotalBytes     int64           // Total bytes to be moved
	ChunksAffected int             // Number of chunks that will be modified
}

// FileMove represents moving a single file from one shard to another
type FileMove struct {
	File          manifest.FileEntry // File to move
	SourceShard   int                // Current shard ID
	TargetShard   int                // Destination shard ID
	SourceChunk   int                // Current chunk ID
	TargetChunk   int                // New chunk ID (TBD during execution)
}

// extractedFile holds file data extracted from a chunk
//
//nolint:unused // Will be used in full rebalancing execution implementation
type extractedFile struct {
	Entry manifest.FileEntry // File metadata
	Data  []byte             // File contents
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
		ImbalanceThreshold: 2.0,               // Rebalance if max > 2x average
		MinShardSize:       100 * 1024 * 1024, // 100MB minimum
		DryRun:             false,
	}
}

// createRebalancePlan generates a plan for redistributing files between shards
func createRebalancePlan(m *manifest.Manifest, balance *ShardBalance) (*RebalancePlan, error) {
	avgSize := balance.AverageSize

	// Build shard info map for quick lookups
	shardSizes := make(map[int]int64)
	for _, shard := range m.Shards {
		shardSizes[shard.ID] = shard.UncompressedSize
	}

	// Create file-to-shard mapping
	filesByShard := make(map[int][]manifest.FileEntry)
	for _, file := range m.Files {
		// Skip duplicate files (they don't take up space)
		if file.IsDuplicate {
			continue
		}
		filesByShard[file.ShardID] = append(filesByShard[file.ShardID], file)
	}

	// Sort files within each shard by size (ascending) for greedy selection
	for shardID := range filesByShard {
		files := filesByShard[shardID]
		sort.Slice(files, func(i, j int) bool {
			return files[i].Size < files[j].Size
		})
	}

	// Identify overloaded and underloaded shards
	var overloaded, underloaded []int
	for _, stat := range balance.ShardStats {
		switch stat.Status {
		case "overloaded":
			overloaded = append(overloaded, stat.ShardID)
		case "underloaded":
			underloaded = append(underloaded, stat.ShardID)
		}
	}

	// Sort overloaded shards by size (descending - tackle worst first)
	sort.Slice(overloaded, func(i, j int) bool {
		return shardSizes[overloaded[i]] > shardSizes[overloaded[j]]
	})

	// Sort underloaded shards by size (ascending - fill smallest first)
	sort.Slice(underloaded, func(i, j int) bool {
		return shardSizes[underloaded[i]] < shardSizes[underloaded[j]]
	})

	// Greedy redistribution algorithm
	var moves []FileMove
	var totalBytes int64
	chunksAffected := make(map[int]bool)

	for _, srcShard := range overloaded {
		// Calculate how much to move from this shard
		excessSize := shardSizes[srcShard] - int64(avgSize)
		if excessSize <= 0 {
			continue
		}

		// Select files to move (smallest first for easier distribution)
		files := filesByShard[srcShard]
		var moved int64

		for _, file := range files {
			if moved >= excessSize {
				break // Moved enough from this shard
			}

			// Find best target shard (most underloaded)
			targetShard := -1
			for _, tgtShard := range underloaded {
				deficit := int64(avgSize) - shardSizes[tgtShard]
				if deficit > 0 && file.Size <= deficit {
					targetShard = tgtShard
					break
				}
			}

			if targetShard == -1 {
				// No suitable target found, try any underloaded shard
				for _, tgtShard := range underloaded {
					if shardSizes[tgtShard] < shardSizes[srcShard] {
						targetShard = tgtShard
						break
					}
				}
			}

			if targetShard == -1 {
				// Still no target - skip this file
				continue
			}

			// Create move
			moves = append(moves, FileMove{
				File:        file,
				SourceShard: srcShard,
				TargetShard: targetShard,
				SourceChunk: file.ChunkID,
				TargetChunk: -1, // Will be assigned during execution
			})

			// Update virtual sizes
			shardSizes[srcShard] -= file.Size
			shardSizes[targetShard] += file.Size
			moved += file.Size
			totalBytes += file.Size

			// Track affected chunks
			chunksAffected[file.ChunkID] = true
		}
	}

	// Calculate projected balance after moves
	// This is a simplified estimate - actual balance depends on execution
	projectedStats := make([]ShardStats, len(balance.ShardStats))
	copy(projectedStats, balance.ShardStats)

	for i := range projectedStats {
		projectedStats[i].UncompressedSize = shardSizes[projectedStats[i].ShardID]
		if avgSize > 0 {
			projectedStats[i].SizeRatio = float64(projectedStats[i].UncompressedSize) / avgSize
		}
	}

	targetBalance := *balance
	targetBalance.ShardStats = projectedStats

	plan := &RebalancePlan{
		Moves:          moves,
		TargetBalance:  targetBalance,
		TotalBytes:     totalBytes,
		ChunksAffected: len(chunksAffected),
	}

	return plan, nil
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

	// Create rebalancing plan
	plan, err := createRebalancePlan(m, initialBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to create rebalance plan: %w", err)
	}

	result.Plan = plan
	result.FilesReassigned = len(plan.Moves)

	// Track which shards are affected
	shardsAffected := make(map[int]bool)
	for _, move := range plan.Moves {
		shardsAffected[move.SourceShard] = true
		shardsAffected[move.TargetShard] = true
	}
	for shardID := range shardsAffected {
		result.ShardsAffected = append(result.ShardsAffected, shardID)
	}

	// If dry run, return plan without executing
	if config.DryRun {
		result.Success = true
		result.FinalBalance = plan.TargetBalance
		result.Recommendation = fmt.Sprintf(
			"DRY RUN: Would rebalance %d files (%.2f GB) from %d overloaded shards to %d underloaded shards. "+
				"Projected imbalance: %.2fx → %.2fx",
			len(plan.Moves),
			float64(plan.TotalBytes)/(1024*1024*1024),
			len(shardsAffected)/2, // Approximate
			len(shardsAffected)/2,
			initialBalance.ImbalanceRatio,
			plan.TargetBalance.ImbalanceRatio)
		return result, nil
	}

	// Execute rebalancing
	err = executeRebalancing(ctx, m, plan, result)
	if err != nil {
		result.Error = err
		return result, fmt.Errorf("rebalancing execution failed: %w", err)
	}

	// Analyze final balance
	finalBalance, err := AnalyzeShardBalance(m, config.ImbalanceThreshold)
	if err != nil {
		return result, fmt.Errorf("failed to analyze final balance: %w", err)
	}

	result.Success = true
	result.FinalBalance = *finalBalance
	result.Recommendation = fmt.Sprintf(
		"Rebalancing complete: moved %d files (%.2f GB). Imbalance: %.2fx → %.2fx",
		len(plan.Moves),
		float64(plan.TotalBytes)/(1024*1024*1024),
		result.InitialBalance.ImbalanceRatio,
		finalBalance.ImbalanceRatio)

	return result, nil
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
		var statusIcon string
		switch stat.Status {
		case "overloaded":
			statusIcon = "⚠️ "
		case "underloaded":
			statusIcon = "📥"
		default:
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

// downloadAndExtractFiles downloads a chunk from S3 and extracts specific files
//
//nolint:unused // Will be used in full rebalancing execution implementation
func downloadAndExtractFiles(ctx context.Context, s3Client *s3.Client, bucket string, chunkKey string, filesToExtract []manifest.FileEntry, compressionType string) ([]extractedFile, error) {
	// Build file lookup map
	fileMap := make(map[string]manifest.FileEntry)
	for _, file := range filesToExtract {
		fileMap[file.Path] = file
	}

	// Download chunk from S3
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(chunkKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download chunk %s: %w", chunkKey, err)
	}
	defer func() {
		_ = result.Body.Close()
	}()

	// Decompress based on compression type
	var decompressor io.Reader
	switch compressionType {
	case "zstd":
		decoder, err := zstd.NewReader(result.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		defer decoder.Close()
		decompressor = decoder
	case "gzip", "gz":
		return nil, fmt.Errorf("gzip compression not yet supported for rebalancing")
	case "none", "":
		decompressor = result.Body
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compressionType)
	}

	// Extract files from tar archive
	tarReader := tar.NewReader(decompressor)
	var extracted []extractedFile

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Check if this file should be extracted
		fileEntry, shouldExtract := fileMap[header.Name]
		if !shouldExtract {
			continue // Skip files not in our list
		}

		// Only extract regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Read file contents into memory
		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", header.Name, err)
		}

		extracted = append(extracted, extractedFile{
			Entry: fileEntry,
			Data:  data,
		})

		// Remove from map to track what's been found
		delete(fileMap, header.Name)
	}

	// Check if all files were found
	if len(fileMap) > 0 {
		missingFiles := make([]string, 0, len(fileMap))
		for path := range fileMap {
			missingFiles = append(missingFiles, path)
		}
		return nil, fmt.Errorf("failed to find %d files in chunk: %v", len(fileMap), missingFiles)
	}

	return extracted, nil
}

// executeRebalancing performs the actual file redistribution
func executeRebalancing(ctx context.Context, m *manifest.Manifest, plan *RebalancePlan, result *RebalanceResult) error {
	// NOTE: Full implementation requires:
	// 1. S3 client for downloading/uploading chunks
	// 2. Integration with existing chunking/archiving pipeline
	// 3. Temporary storage for extracted files
	// 4. Chunk creation with proper compression
	// 5. Manifest updates
	// 6. Old chunk cleanup
	//
	// This is a complex operation that should be done carefully to avoid data loss.
	// For now, returning an error to indicate this needs further implementation.

	return fmt.Errorf("rebalancing execution not yet fully implemented - requires S3 client configuration and pipeline integration")
}
