package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func TestAnalyzeShardBalance_Balanced(t *testing.T) {
	// Create a manifest with balanced shards (8 shards, 1GB each)
	m := &manifest.Manifest{
		Version:   "1.0",
		UploadID:  "test-upload",
		CreatedAt: time.Now(),
		ShardCount: 8,
		Shards: []manifest.ShardEntry{
			{ID: 0, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 1, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 2, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 3, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 4, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 5, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 6, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
			{ID: 7, FileCount: 1000, UncompressedSize: 1024 * 1024 * 1024, ChunkCount: 10},
		},
	}

	balance, err := AnalyzeShardBalance(m, 2.0)
	if err != nil {
		t.Fatalf("Failed to analyze balance: %v", err)
	}

	// Verify balanced state
	if !balance.IsBalanced {
		t.Errorf("Expected balanced state, got imbalanced (ratio: %.2f)", balance.ImbalanceRatio)
	}

	if balance.ImbalanceRatio > 1.1 {
		t.Errorf("Expected imbalance ratio ~1.0, got %.2f", balance.ImbalanceRatio)
	}

	// Verify statistics
	expectedTotal := int64(8 * 1024 * 1024 * 1024)
	if balance.TotalSize != expectedTotal {
		t.Errorf("Expected total size %d, got %d", expectedTotal, balance.TotalSize)
	}

	expectedAvg := float64(expectedTotal) / 8
	if balance.AverageSize != expectedAvg {
		t.Errorf("Expected average size %.0f, got %.0f", expectedAvg, balance.AverageSize)
	}
}

func TestAnalyzeShardBalance_Imbalanced(t *testing.T) {
	// Create a manifest with severely imbalanced shards
	// Shard 0 has 90% of data, others share 10%
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		CreatedAt:  time.Now(),
		ShardCount: 8,
		Shards: []manifest.ShardEntry{
			{ID: 0, FileCount: 9000, UncompressedSize: 9 * 1024 * 1024 * 1024, ChunkCount: 90}, // 9GB
			{ID: 1, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},         // ~146MB
			{ID: 2, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
			{ID: 3, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
			{ID: 4, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
			{ID: 5, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
			{ID: 6, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
			{ID: 7, FileCount: 143, UncompressedSize: 146 * 1024 * 1024, ChunkCount: 2},
		},
	}

	balance, err := AnalyzeShardBalance(m, 2.0)
	if err != nil {
		t.Fatalf("Failed to analyze balance: %v", err)
	}

	// Verify imbalanced state
	if balance.IsBalanced {
		t.Error("Expected imbalanced state, got balanced")
	}

	// Imbalance ratio should be significantly > 2.0
	if balance.ImbalanceRatio < 2.0 {
		t.Errorf("Expected imbalance ratio > 2.0, got %.2f", balance.ImbalanceRatio)
	}

	// Verify largest shard is identified
	if balance.MaxSize != 9*1024*1024*1024 {
		t.Errorf("Expected max size 9GB, got %d", balance.MaxSize)
	}

	// Verify shard stats show overloaded status
	hasOverloaded := false
	for _, stat := range balance.ShardStats {
		if stat.Status == "overloaded" {
			hasOverloaded = true
			break
		}
	}
	if !hasOverloaded {
		t.Error("Expected at least one shard to be marked overloaded")
	}
}

func TestAnalyzeShardBalance_EmptyManifest(t *testing.T) {
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		ShardCount: 0,
		Shards:     []manifest.ShardEntry{},
	}

	_, err := AnalyzeShardBalance(m, 2.0)
	if err == nil {
		t.Error("Expected error for manifest with no shards")
	}
}

func TestAnalyzeShardBalance_NilManifest(t *testing.T) {
	_, err := AnalyzeShardBalance(nil, 2.0)
	if err == nil {
		t.Error("Expected error for nil manifest")
	}
}

func TestRebalanceShards_AlreadyBalanced(t *testing.T) {
	// Create a balanced manifest
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		CreatedAt:  time.Now(),
		ShardCount: 4,
		Shards: []manifest.ShardEntry{
			{ID: 0, FileCount: 250, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 1, FileCount: 250, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 2, FileCount: 250, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 3, FileCount: 250, UncompressedSize: 1024 * 1024 * 1024},
		},
	}

	ctx := context.Background()
	config := DefaultRebalanceConfig()

	result, err := RebalanceShards(ctx, m, config)
	if err != nil {
		t.Fatalf("Failed to rebalance: %v", err)
	}

	// Should succeed without making changes
	if !result.Success {
		t.Error("Expected success for already balanced shards")
	}

	if result.FilesReassigned != 0 {
		t.Errorf("Expected 0 files reassigned, got %d", result.FilesReassigned)
	}

	if len(result.ShardsAffected) != 0 {
		t.Errorf("Expected 0 shards affected, got %d", len(result.ShardsAffected))
	}
}

func TestRebalanceShards_DryRun(t *testing.T) {
	// Create an imbalanced manifest
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		CreatedAt:  time.Now(),
		ShardCount: 4,
		Shards: []manifest.ShardEntry{
			{ID: 0, FileCount: 800, UncompressedSize: 8 * 1024 * 1024 * 1024}, // 8GB
			{ID: 1, FileCount: 200, UncompressedSize: 1 * 1024 * 1024 * 1024}, // 1GB
			{ID: 2, FileCount: 200, UncompressedSize: 1 * 1024 * 1024 * 1024},
			{ID: 3, FileCount: 200, UncompressedSize: 1 * 1024 * 1024 * 1024},
		},
	}

	ctx := context.Background()
	config := &RebalanceConfig{
		ImbalanceThreshold: 2.0,
		DryRun:             true,
	}

	result, err := RebalanceShards(ctx, m, config)
	if err != nil {
		t.Fatalf("Failed dry run: %v", err)
	}

	// Dry run should succeed without making changes
	if !result.Success {
		t.Error("Expected success for dry run")
	}

	if result.FilesReassigned != 0 {
		t.Errorf("Dry run should not reassign files, got %d", result.FilesReassigned)
	}

	// Initial and final balance should be same in dry run
	if result.InitialBalance.ImbalanceRatio != result.FinalBalance.ImbalanceRatio {
		t.Error("Dry run should not change balance")
	}
}

func TestShardStats_StatusCalculation(t *testing.T) {
	testCases := []struct {
		name      string
		sizeRatio float64
		threshold float64
		expected  string
	}{
		{"Overloaded", 5.0, 2.0, "overloaded"}, // 5x ratio will be > 2.0 after averaging
		{"Balanced", 1.5, 2.0, "balanced"},
		{"Underloaded", 0.4, 2.0, "underloaded"},
		{"Exactly at threshold", 2.0, 2.0, "balanced"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create manifest with specific ratio
			avgSize := 1024 * 1024 * 1024 // 1GB average
			shardSize := int64(float64(avgSize) * tc.sizeRatio)

			m := &manifest.Manifest{
				Version:    "1.0",
				UploadID:   "test",
				ShardCount: 4,
				Shards: []manifest.ShardEntry{
					{ID: 0, UncompressedSize: shardSize},
					{ID: 1, UncompressedSize: int64(avgSize)},
					{ID: 2, UncompressedSize: int64(avgSize)},
					{ID: 3, UncompressedSize: int64(avgSize)},
				},
			}

			balance, err := AnalyzeShardBalance(m, tc.threshold)
			if err != nil {
				t.Fatalf("Failed to analyze: %v", err)
			}

			// Find shard 0 in stats
			var shard0 *ShardStats
			for i := range balance.ShardStats {
				if balance.ShardStats[i].ShardID == 0 {
					shard0 = &balance.ShardStats[i]
					break
				}
			}

			if shard0 == nil {
				t.Fatal("Could not find shard 0 in stats")
			}

			if shard0.Status != tc.expected {
				t.Errorf("Expected status %s, got %s (ratio: %.2f, threshold: %.2f)",
					tc.expected, shard0.Status, tc.sizeRatio, tc.threshold)
			}
		})
	}
}

func TestDefaultRebalanceConfig(t *testing.T) {
	config := DefaultRebalanceConfig()

	if config.ImbalanceThreshold != 2.0 {
		t.Errorf("Expected threshold 2.0, got %.2f", config.ImbalanceThreshold)
	}

	if config.MinShardSize != 100*1024*1024 {
		t.Errorf("Expected min shard size 100MB, got %d", config.MinShardSize)
	}

	if config.DryRun {
		t.Error("Expected DryRun to be false by default")
	}
}

func TestCreateRebalancePlan_NoImbalance(t *testing.T) {
	// Create a balanced manifest
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		ShardCount: 4,
		Shards: []manifest.ShardEntry{
			{ID: 0, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 1, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 2, UncompressedSize: 1024 * 1024 * 1024},
			{ID: 3, UncompressedSize: 1024 * 1024 * 1024},
		},
		Files: []manifest.FileEntry{},
	}

	balance, err := AnalyzeShardBalance(m, 2.0)
	if err != nil {
		t.Fatalf("Failed to analyze balance: %v", err)
	}

	plan, err := createRebalancePlan(m, balance)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// No moves should be needed for balanced shards
	if len(plan.Moves) != 0 {
		t.Errorf("Expected 0 moves for balanced shards, got %d", len(plan.Moves))
	}
}

func TestCreateRebalancePlan_WithImbalance(t *testing.T) {
	// Create an imbalanced manifest with actual files
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		ShardCount: 4,
		Shards: []manifest.ShardEntry{
			{ID: 0, UncompressedSize: 8 * 1024 * 1024 * 1024}, // 8GB
			{ID: 1, UncompressedSize: 1 * 1024 * 1024 * 1024}, // 1GB
			{ID: 2, UncompressedSize: 1 * 1024 * 1024 * 1024},
			{ID: 3, UncompressedSize: 1 * 1024 * 1024 * 1024},
		},
		Files: []manifest.FileEntry{
			// Shard 0 has large files
			{Path: "file1.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0},
			{Path: "file2.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0},
			{Path: "file3.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0},
			{Path: "file4.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0},
			// Other shards have smaller files
			{Path: "file5.dat", Size: 1 * 1024 * 1024 * 1024, ShardID: 1, ChunkID: 1},
			{Path: "file6.dat", Size: 1 * 1024 * 1024 * 1024, ShardID: 2, ChunkID: 2},
			{Path: "file7.dat", Size: 1 * 1024 * 1024 * 1024, ShardID: 3, ChunkID: 3},
		},
	}

	balance, err := AnalyzeShardBalance(m, 2.0)
	if err != nil {
		t.Fatalf("Failed to analyze balance: %v", err)
	}

	plan, err := createRebalancePlan(m, balance)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Should have moves to rebalance
	if len(plan.Moves) == 0 {
		t.Error("Expected moves for imbalanced shards")
	}

	// All moves should be from shard 0 (overloaded)
	for _, move := range plan.Moves {
		if move.SourceShard != 0 {
			t.Errorf("Expected moves from shard 0, got move from shard %d", move.SourceShard)
		}
		if move.TargetShard == 0 {
			t.Error("Should not move files to the same shard")
		}
	}

	// Total bytes moved should be reasonable
	if plan.TotalBytes == 0 {
		t.Error("Expected non-zero bytes to be moved")
	}
}

func TestCreateRebalancePlan_SkipDuplicates(t *testing.T) {
	// Create manifest with duplicate files
	m := &manifest.Manifest{
		Version:    "1.0",
		UploadID:   "test-upload",
		ShardCount: 2,
		Shards: []manifest.ShardEntry{
			{ID: 0, UncompressedSize: 5 * 1024 * 1024 * 1024},
			{ID: 1, UncompressedSize: 1 * 1024 * 1024 * 1024},
		},
		Files: []manifest.FileEntry{
			// Original file
			{Path: "file1.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0, IsDuplicate: false},
			// Duplicate (should not be considered for moves)
			{Path: "file1_dup.dat", Size: 2 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0, IsDuplicate: true},
			// Another file
			{Path: "file2.dat", Size: 3 * 1024 * 1024 * 1024, ShardID: 0, ChunkID: 0, IsDuplicate: false},
			{Path: "file3.dat", Size: 1 * 1024 * 1024 * 1024, ShardID: 1, ChunkID: 1, IsDuplicate: false},
		},
	}

	balance, err := AnalyzeShardBalance(m, 2.0)
	if err != nil {
		t.Fatalf("Failed to analyze balance: %v", err)
	}

	plan, err := createRebalancePlan(m, balance)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Check that duplicate file is not in the moves
	for _, move := range plan.Moves {
		if move.File.IsDuplicate {
			t.Error("Duplicate files should not be moved")
		}
	}
}
