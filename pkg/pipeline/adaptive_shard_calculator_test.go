package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAdaptiveShardCalculator_calculateFromWorkload(t *testing.T) {
	tests := []struct {
		name              string
		fileCount         int64
		rawSize           int64
		compressionRatio  float64
		cpuCores          int
		availableMemoryGB int
		expectedMin       int
		expectedMax       int
		expectWarnings    bool
	}{
		// Workload classes
		{
			name:              "tiny_workload",
			fileCount:         100,
			rawSize:           50 << 20, // 50 MB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       6,
			expectWarnings:    false,
		},
		{
			name:              "small_workload",
			fileCount:         1000,
			rawSize:           500 << 20, // 500 MB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       8,
			expectWarnings:    false,
		},
		{
			name:              "medium_workload",
			fileCount:         10_000,
			rawSize:           5 << 30, // 5 GB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       12,
			expectWarnings:    false,
		},
		{
			name:              "large_workload",
			fileCount:         50_000,
			rawSize:           50 << 30, // 50 GB
			compressionRatio:  0.5,
			cpuCores:          16,
			availableMemoryGB: 32,
			expectedMin:       8,
			expectedMax:       20,
			expectWarnings:    false,
		},
		{
			name:              "huge_workload",
			fileCount:         500_000,
			rawSize:           500 << 30, // 500 GB
			compressionRatio:  0.5,
			cpuCores:          32,
			availableMemoryGB: 64,
			expectedMin:       16,
			expectedMax:       32,
			expectWarnings:    false,
		},

		// Dominant factors
		{
			name:              "file_heavy_workload",
			fileCount:         100_000,
			rawSize:           1 << 30, // 1 GB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       6,
			expectWarnings:    true, // Load balance warning expected
			// 100k files ÷ 10k = 10 shards (file-based dominates)
			// But only ~8 chunks (500MB / 64MB), so reduced to 4 shards for balance
		},
		{
			name:              "size_heavy_workload",
			fileCount:         1000,
			rawSize:           100 << 30, // 100 GB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       5,
			expectedMax:       8,
			expectWarnings:    false,
			// 50GB compressed ÷ 10GB = 5 shards (size-based dominates)
		},

		// Resource constraints
		{
			name:              "cpu_constrained",
			fileCount:         10_000,
			rawSize:           50 << 30, // 50 GB
			compressionRatio:  0.5,
			cpuCores:          2,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       4,
			expectWarnings:    true, // CPU constraint warning expected
			// 2 cores × 2 = 4 shards max
		},
		{
			name:              "memory_constrained",
			fileCount:         10_000,
			rawSize:           50 << 30, // 50 GB
			compressionRatio:  0.5,
			cpuCores:          16,
			availableMemoryGB: 2,
			expectedMin:       4,
			expectedMax:       10,
			expectWarnings:    false, // Memory constraint doesn't trigger (workload suggests 8 shards, limit is 10)
			// 2GB ÷ 200MB = 10 shards max
		},

		// Edge cases
		{
			name:              "highly_compressible",
			fileCount:         10_000,
			rawSize:           100 << 30, // 100 GB
			compressionRatio:  0.1,       // 90% compression
			cpuCores:          16,
			availableMemoryGB: 32,
			expectedMin:       4,
			expectedMax:       8,
			expectWarnings:    false,
			// 10GB compressed → small/medium range
		},
		{
			name:              "uncompressible",
			fileCount:         10_000,
			rawSize:           10 << 30, // 10 GB
			compressionRatio:  1.0,      // No compression
			cpuCores:          16,
			availableMemoryGB: 32,
			expectedMin:       4,
			expectedMax:       12,
			expectWarnings:    false,
			// 10GB compressed → medium range
		},
		{
			name:              "very_small_workload",
			fileCount:         10,
			rawSize:           5 << 20, // 5 MB
			compressionRatio:  0.5,
			cpuCores:          8,
			availableMemoryGB: 16,
			expectedMin:       4,
			expectedMax:       4,
			expectWarnings:    false, // Gets clamped to minimum 4 shards without warnings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create calculator with test parameters
			calc := &AdaptiveShardCalculator{
				cpuCores:        tt.cpuCores,
				availableMemory: int64(tt.availableMemoryGB) << 30,
				workersPerShard: 2,
				memoryPerShard:  200 << 20, // 200MB
			}

			compressedSize := int64(float64(tt.rawSize) * tt.compressionRatio)
			shardCount, rationale, warnings := calc.calculateFromWorkload(tt.fileCount, compressedSize)

			// Verify shard count is in expected range
			if shardCount < tt.expectedMin || shardCount > tt.expectedMax {
				t.Errorf("shard count %d not in expected range [%d, %d]", shardCount, tt.expectedMin, tt.expectedMax)
			}

			// Verify shard count is within absolute bounds
			if shardCount < 4 || shardCount > 32 {
				t.Errorf("shard count %d outside absolute bounds [4, 32]", shardCount)
			}

			// Verify rationale is non-empty
			if rationale == "" {
				t.Error("rationale should not be empty")
			}

			// Verify warnings expectation
			hasWarnings := len(warnings) > 0
			if hasWarnings != tt.expectWarnings {
				t.Errorf("warnings expectation mismatch: got %v, want %v (warnings: %v)", hasWarnings, tt.expectWarnings, warnings)
			}

			t.Logf("Shard count: %d, Warnings: %v", shardCount, warnings)
		})
	}
}

func TestAdaptiveShardCalculator_classifyWorkload(t *testing.T) {
	calc := NewAdaptiveShardCalculator()

	tests := []struct {
		name           string
		compressedSize int64
		expectedClass  WorkloadClass
	}{
		{
			name:           "tiny_50mb",
			compressedSize: 50 << 20,
			expectedClass:  TinyWorkload,
		},
		{
			name:           "small_500mb",
			compressedSize: 500 << 20,
			expectedClass:  SmallWorkload,
		},
		{
			name:           "medium_5gb",
			compressedSize: 5 << 30,
			expectedClass:  MediumWorkload,
		},
		{
			name:           "large_50gb",
			compressedSize: 50 << 30,
			expectedClass:  LargeWorkload,
		},
		{
			name:           "huge_200gb",
			compressedSize: 200 << 30,
			expectedClass:  HugeWorkload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := calc.classifyWorkload(tt.compressedSize)
			if class != tt.expectedClass {
				t.Errorf("classifyWorkload() = %v, want %v", class, tt.expectedClass)
			}
		})
	}
}

func TestAdaptiveShardCalculator_CalculateOptimalShardCount_Integration(t *testing.T) {
	// Create temporary test directory with files
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []struct {
		name    string
		size    int
		content string
	}{
		{"file1.txt", 1024, "test content 1"},
		{"file2.txt", 2048, "test content 2"},
		{"file3.log", 4096, "log content 3"},
		{".hidden", 512, "hidden file"}, // Should be skipped
	}

	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		content := make([]byte, tf.size)
		copy(content, []byte(tf.content))
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	calc := NewAdaptiveShardCalculator()
	ctx := context.Background()

	result, err := calc.CalculateOptimalShardCount(ctx, tmpDir)
	if err != nil {
		t.Fatalf("CalculateOptimalShardCount() error = %v", err)
	}

	// Verify result structure
	if result.ShardCount < 4 || result.ShardCount > 32 {
		t.Errorf("ShardCount %d outside bounds [4, 32]", result.ShardCount)
	}

	if result.FileCount != 3 { // .hidden should be skipped
		t.Errorf("FileCount = %d, want 3", result.FileCount)
	}

	if result.RawSize == 0 {
		t.Error("RawSize should be > 0")
	}

	if result.CompressedSize == 0 {
		t.Error("CompressedSize should be > 0")
	}

	if result.CompressionRatio <= 0 || result.CompressionRatio > 1 {
		t.Errorf("CompressionRatio %f should be in range (0, 1]", result.CompressionRatio)
	}

	if result.Rationale == "" {
		t.Error("Rationale should not be empty")
	}

	if result.WorkloadClass == "" {
		t.Error("WorkloadClass should not be empty")
	}

	t.Logf("Result: %+v", result)
}

func TestAdaptiveShardCalculator_CalculateOptimalShardCount_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	calc := NewAdaptiveShardCalculator()
	ctx := context.Background()

	_, err := calc.CalculateOptimalShardCount(ctx, tmpDir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestAdaptiveShardCalculator_CalculateOptimalShardCount_InvalidPath(t *testing.T) {
	calc := NewAdaptiveShardCalculator()
	ctx := context.Background()

	_, err := calc.CalculateOptimalShardCount(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestAdaptiveShardCalculator_LoadBalance(t *testing.T) {
	calc := NewAdaptiveShardCalculator()

	// Very small workload that would suggest many shards but has few chunks
	compressedSize := int64(100 << 20) // 100 MB compressed
	fileCount := int64(100_000)        // Many files

	shardCount, rationale, warnings := calc.calculateFromWorkload(fileCount, compressedSize)

	// Should reduce shards due to load balance constraint
	// 100MB / 64MB = ~2 chunks total
	// Minimum 6 chunks per shard
	// So maximum ~1 shard, but enforced minimum is 4
	if shardCount > 4 {
		t.Errorf("expected load balance to limit shards to 4, got %d", shardCount)
	}

	if len(warnings) == 0 {
		t.Error("expected load balance warning")
	}

	t.Logf("ShardCount: %d, Warnings: %v", shardCount, warnings)
	t.Logf("Rationale:\n%s", rationale)
}

func TestWorkloadClass_String(t *testing.T) {
	tests := []struct {
		class WorkloadClass
		want  string
	}{
		{TinyWorkload, "Tiny"},
		{SmallWorkload, "Small"},
		{MediumWorkload, "Medium"},
		{LargeWorkload, "Large"},
		{HugeWorkload, "Huge"},
		{WorkloadClass(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.class.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAvailableMemory(t *testing.T) {
	mem := getAvailableMemory()

	// Should return at least 1 GB (enforced minimum)
	if mem < 1<<30 {
		t.Errorf("getAvailableMemory() = %d bytes, want >= 1 GB", mem)
	}

	// Should be reasonable (< 1 TB for typical systems)
	if mem > 1<<40 {
		t.Errorf("getAvailableMemory() = %d bytes, seems unreasonably high", mem)
	}

	t.Logf("Available memory: %d GB", mem/(1<<30))
}

func TestHelperFunctions(t *testing.T) {
	t.Run("max", func(t *testing.T) {
		if got := max(5, 10); got != 10 {
			t.Errorf("max(5, 10) = %d, want 10", got)
		}
		if got := max(10, 5); got != 10 {
			t.Errorf("max(10, 5) = %d, want 10", got)
		}
	})

	t.Run("clamp", func(t *testing.T) {
		if got := clamp(5, 1, 10); got != 5 {
			t.Errorf("clamp(5, 1, 10) = %d, want 5", got)
		}
		if got := clamp(0, 1, 10); got != 1 {
			t.Errorf("clamp(0, 1, 10) = %d, want 1", got)
		}
		if got := clamp(15, 1, 10); got != 10 {
			t.Errorf("clamp(15, 1, 10) = %d, want 10", got)
		}
	})
}

// Benchmark the calculation
func BenchmarkCalculateFromWorkload(b *testing.B) {
	calc := NewAdaptiveShardCalculator()
	fileCount := int64(10_000)
	compressedSize := int64(5 << 30) // 5 GB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = calc.calculateFromWorkload(fileCount, compressedSize)
	}
}

func BenchmarkClassifyWorkload(b *testing.B) {
	calc := NewAdaptiveShardCalculator()
	compressedSize := int64(5 << 30) // 5 GB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calc.classifyWorkload(compressedSize)
	}
}
