// Package chunking provides intelligent file chunking strategies for CargoShip
package chunking

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// TierSelector defines the interface for storage tier selection
// This avoids a circular dependency with pkg/pipeline
type TierSelector interface {
	SelectTier(atime, mtime time.Time) types.StorageClass
}

// TierAwareChunker wraps a base chunking strategy and groups files by storage tier
// before chunking. This implements Issue #164 Strategy 1: Tier-Aware Chunking.
//
// Benefits:
//   - Homogeneous tier assignment per chunk (better cost optimization)
//   - No "tier pollution" (hot files not mixed with cold files)
//   - 30-60% better cost efficiency vs youngest-file strategy
//
// Trade-offs:
//   - Memory overhead (files buffered per tier group)
//   - Potential chunk fragmentation if tier groups are small
type TierAwareChunker struct {
	baseChunker  ChunkingStrategy
	tierSelector TierSelector
	bufferSize   int // Maximum files to buffer per tier (0 = unlimited)
}

// NewTierAwareChunker creates a new tier-aware chunker that wraps a base chunking strategy
func NewTierAwareChunker(base ChunkingStrategy, selector TierSelector, bufferSize int) *TierAwareChunker {
	if bufferSize <= 0 {
		bufferSize = 100000 // Default: 100k files per tier
	}

	return &TierAwareChunker{
		baseChunker:  base,
		tierSelector: selector,
		bufferSize:   bufferSize,
	}
}

// CalculateOptimalChunkSize delegates to the base chunker
func (t *TierAwareChunker) CalculateOptimalChunkSize(
	totalSize int64,
	fileCount int,
	availableMemory int64,
	costSavingsTarget float64,
) (chunkSize int64, stats ChunkStats) {
	return t.baseChunker.CalculateOptimalChunkSize(totalSize, fileCount, availableMemory, costSavingsTarget)
}

// GroupFilesIntoChunks groups files by storage tier first, then creates chunks per tier
//
// Algorithm:
//  1. For each file, determine storage tier using TierSelector
//  2. Group files into tier-specific lists
//  3. For each tier group, call base chunker to create chunks
//  4. Assign PreAssignedTier to each chunk
//  5. Return all chunks (across all tiers)
//
// This ensures each chunk contains files from only one storage tier,
// eliminating the "youngest-file problem" where old files pay for expensive storage.
func (t *TierAwareChunker) GroupFilesIntoChunks(
	files []File,
	chunkSize int64,
) ([]Chunk, error) {
	if len(files) == 0 {
		return []Chunk{}, nil
	}

	// Step 1: Group files by storage tier
	tierGroups := make(map[types.StorageClass][]File)

	for _, file := range files {
		// Parse atime from metadata
		atime, err := parseAtime(file.Metadata["atime"])
		if err != nil {
			// If atime parsing fails, use zero time (will fallback to mtime in selector)
			atime = time.Time{}
		}

		// Determine storage tier for this file
		tier := t.tierSelector.SelectTier(atime, file.ModTime)

		// Check buffer size limit per tier
		if t.bufferSize > 0 && len(tierGroups[tier]) >= t.bufferSize {
			return nil, fmt.Errorf("tier group buffer size exceeded for tier %s: %d files (limit: %d)",
				tier, len(tierGroups[tier]), t.bufferSize)
		}

		tierGroups[tier] = append(tierGroups[tier], file)
	}

	// Step 2: Create chunks for each tier group
	var allChunks []Chunk
	chunkIDOffset := 0

	// Sort tiers for deterministic output (STANDARD, STANDARD_IA, GLACIER, DEEP_ARCHIVE)
	tierOrder := []types.StorageClass{
		types.StorageClassStandard,
		types.StorageClassStandardIa,
		types.StorageClassIntelligentTiering,
		types.StorageClassGlacier,
		types.StorageClassGlacierIr,
		types.StorageClassDeepArchive,
	}

	for _, tier := range tierOrder {
		tierFiles, exists := tierGroups[tier]
		if !exists || len(tierFiles) == 0 {
			continue
		}

		// Use base chunker to create chunks for this tier's files
		tierChunks, err := t.baseChunker.GroupFilesIntoChunks(tierFiles, chunkSize)
		if err != nil {
			return nil, fmt.Errorf("failed to chunk files for tier %s: %w", tier, err)
		}

		// Assign tier to each chunk and renumber IDs
		for i := range tierChunks {
			tierChunks[i].ID = chunkIDOffset + i
			tierChunks[i].PreAssignedTier = tier
		}

		chunkIDOffset += len(tierChunks)
		allChunks = append(allChunks, tierChunks...)
	}

	return allChunks, nil
}

// parseAtime parses an RFC3339 timestamp from file metadata
func parseAtime(atimeStr string) (time.Time, error) {
	if atimeStr == "" {
		return time.Time{}, fmt.Errorf("empty atime string")
	}

	t, err := time.Parse(time.RFC3339, atimeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse atime %q: %w", atimeStr, err)
	}

	return t, nil
}

// GetTierDistribution returns statistics about file distribution across storage tiers
// This is useful for cost estimation and debugging
func (t *TierAwareChunker) GetTierDistribution(files []File) map[types.StorageClass]TierGroupStats {
	distribution := make(map[types.StorageClass]TierGroupStats)

	for _, file := range files {
		atime, err := parseAtime(file.Metadata["atime"])
		if err != nil {
			atime = time.Time{}
		}

		tier := t.tierSelector.SelectTier(atime, file.ModTime)

		stats := distribution[tier]
		stats.FileCount++
		stats.TotalSize += file.Size
		distribution[tier] = stats
	}

	return distribution
}

// TierGroupStats provides statistics for a single storage tier group
type TierGroupStats struct {
	FileCount int   // Number of files in this tier
	TotalSize int64 // Total size of files in bytes
}
