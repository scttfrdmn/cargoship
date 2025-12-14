package chunking

import (
	"fmt"
	"testing"
	"time"
)

func TestNewShardRouter(t *testing.T) {
	tests := []struct {
		name    string
		config  *ShardRouterConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &ShardRouterConfig{
				Strategy:   ShardByHash,
				ShardCount: 10,
			},
			wantErr: false,
		},
		{
			name: "zero shard count",
			config: &ShardRouterConfig{
				Strategy:   ShardByHash,
				ShardCount: 0,
			},
			wantErr: true,
		},
		{
			name: "negative shard count",
			config: &ShardRouterConfig{
				Strategy:   ShardByHash,
				ShardCount: -5,
			},
			wantErr: true,
		},
		{
			name: "shard count too large",
			config: &ShardRouterConfig{
				Strategy:   ShardByHash,
				ShardCount: 1001,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, err := NewShardRouter(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewShardRouter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && router == nil {
				t.Error("NewShardRouter() returned nil router without error")
			}
		})
	}
}

func TestShardRouter_RouteByHash(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Test deterministic routing (same file → same shard)
	file := File{
		Path: "/path/to/test/file.txt",
		Size: 1024,
	}

	shard1 := router.Route(file)
	shard2 := router.Route(file)

	if shard1 != shard2 {
		t.Errorf("Hash routing not deterministic: got %d and %d for same file", shard1, shard2)
	}

	// Verify shard is within valid range
	if shard1 < 0 || shard1 >= router.GetShardCount() {
		t.Errorf("Shard ID out of range: got %d, want 0-%d", shard1, router.GetShardCount()-1)
	}
}

func TestShardRouter_RouteByHashDistribution(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Generate 10,000 files
	files := make([]File, 10000)
	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/data/file_%d.txt", i),
			Size: 1024,
		}
	}

	// Analyze distribution
	dist := router.AnalyzeDistribution(files)

	t.Logf("Distribution: Min=%d, Max=%d, Avg=%.1f, Variance=%.2f, Balance=%.4f",
		dist.MinFiles, dist.MaxFiles, dist.AvgFiles, dist.Variance, dist.BalanceQuality)

	// Hash-based routing should be well-balanced
	// With 10,000 files and 10 shards, expect ~1,000 files per shard
	// Allow ±20% variance (800-1200 files)
	expectedAvg := 1000.0
	tolerance := 0.20 * expectedAvg

	if dist.AvgFiles < expectedAvg-tolerance || dist.AvgFiles > expectedAvg+tolerance {
		t.Errorf("Average files per shard %.1f outside expected range %.1f±%.1f",
			dist.AvgFiles, expectedAvg, tolerance)
	}

	// Balance quality should be >0.9 for good hash distribution
	if dist.BalanceQuality < 0.9 {
		t.Errorf("Balance quality %.4f below threshold 0.9", dist.BalanceQuality)
	}
}

func TestShardRouter_RouteBySize(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:                ShardBySize,
		ShardCount:              10,
		SmallFileSizeThreshold:  1 << 20,   // 1MB
		MediumFileSizeThreshold: 100 << 20, // 100MB
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	tests := []struct {
		name          string
		fileSize      int64
		expectedShard int
	}{
		{
			name:          "small file (100KB)",
			fileSize:      100 * 1024,
			expectedShard: 0,
		},
		{
			name:          "small file at threshold (1MB)",
			fileSize:      1 << 20,
			expectedShard: 1, // >= threshold goes to medium
		},
		{
			name:          "medium file (10MB)",
			fileSize:      10 << 20,
			expectedShard: 1,
		},
		{
			name:          "large file (200MB)",
			fileSize:      200 << 20,
			expectedShard: 2, // Distributed across shards 2+
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := File{
				Path: "/data/file.txt",
				Size: tt.fileSize,
			}

			shard := router.Route(file)

			// For small and medium files, exact shard check
			if tt.fileSize < router.config.MediumFileSizeThreshold {
				if shard != tt.expectedShard {
					t.Errorf("Route() = %d, want %d for %s", shard, tt.expectedShard, tt.name)
				}
			} else {
				// For large files, verify it's in range 2+
				if shard < 2 || shard >= router.GetShardCount() {
					t.Errorf("Route() = %d, want 2-%d for large file", shard, router.GetShardCount()-1)
				}
			}
		})
	}
}

func TestShardRouter_RouteByType(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByType,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	tests := []struct {
		name          string
		filePath      string
		expectedShard int
	}{
		// Images → shard 0
		{
			name:          "JPEG image",
			filePath:      "/photos/vacation.jpg",
			expectedShard: 0,
		},
		{
			name:          "PNG image",
			filePath:      "/screenshots/bug.png",
			expectedShard: 0,
		},
		// Documents → shard 1
		{
			name:          "PDF document",
			filePath:      "/documents/report.pdf",
			expectedShard: 1,
		},
		{
			name:          "Excel spreadsheet",
			filePath:      "/data/sales.xlsx",
			expectedShard: 1,
		},
		// Code → shard 2
		{
			name:          "Go source file",
			filePath:      "/src/main.go",
			expectedShard: 2,
		},
		{
			name:          "Python script",
			filePath:      "/scripts/deploy.py",
			expectedShard: 2,
		},
		// Binary → shard 3
		{
			name:          "Executable",
			filePath:      "/bin/app.exe",
			expectedShard: 3,
		},
		{
			name:          "Shared library",
			filePath:      "/lib/libfoo.so",
			expectedShard: 3,
		},
		// Archive → shard 4
		{
			name:          "ZIP archive",
			filePath:      "/archives/backup.zip",
			expectedShard: 4,
		},
		{
			name:          "Gzip file",
			filePath:      "/logs/app.log.gz",
			expectedShard: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := File{
				Path: tt.filePath,
				Size: 1024,
			}

			shard := router.Route(file)

			if shard != tt.expectedShard {
				t.Errorf("Route() = %d, want %d for %s", shard, tt.expectedShard, tt.name)
			}
		})
	}
}

func TestShardRouter_RouteByDirectory(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByDirectory,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Files in same directory should go to same shard
	files := []File{
		{Path: "/project/src/main.go", Size: 1024},
		{Path: "/project/src/utils.go", Size: 2048},
		{Path: "/project/src/config.go", Size: 512},
	}

	shards := make([]int, len(files))
	for i, file := range files {
		shards[i] = router.Route(file)
	}

	// All files in /project/src should have same shard
	if shards[0] != shards[1] || shards[1] != shards[2] {
		t.Errorf("Files in same directory routed to different shards: %v", shards)
	}

	// Files in different directories should (likely) go to different shards
	file2 := File{Path: "/data/file.txt", Size: 1024}
	shard2 := router.Route(file2)

	// This isn't guaranteed due to hash collisions, but very unlikely
	// with 10 shards and 2 directories
	if shard2 == shards[0] {
		t.Logf("Warning: Hash collision detected (files in different dirs got same shard)")
	}
}

func TestShardRouter_ParseShardStrategy(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ShardStrategy
		wantErr bool
	}{
		{
			name:    "hash lowercase",
			input:   "hash",
			want:    ShardByHash,
			wantErr: false,
		},
		{
			name:    "size uppercase",
			input:   "SIZE",
			want:    ShardBySize,
			wantErr: false,
		},
		{
			name:    "type mixed case",
			input:   "Type",
			want:    ShardByType,
			wantErr: false,
		},
		{
			name:    "directory",
			input:   "directory",
			want:    ShardByDirectory,
			wantErr: false,
		},
		{
			name:    "adaptive",
			input:   "adaptive",
			want:    ShardAdaptive,
			wantErr: false,
		},
		{
			name:    "invalid strategy",
			input:   "invalid",
			want:    ShardByHash,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseShardStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseShardStrategy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseShardStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShardRouter_StrategyString(t *testing.T) {
	tests := []struct {
		strategy ShardStrategy
		want     string
	}{
		{ShardByHash, "hash"},
		{ShardBySize, "size"},
		{ShardByType, "type"},
		{ShardByDirectory, "directory"},
		{ShardAdaptive, "adaptive"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.strategy.String()
			if got != tt.want {
				t.Errorf("ShardStrategy.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShardRouter_AnalyzeDistribution(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Create test files
	files := []File{
		{Path: "/file1.txt", Size: 100},
		{Path: "/file2.txt", Size: 200},
		{Path: "/file3.txt", Size: 300},
		{Path: "/file4.txt", Size: 400},
		{Path: "/file5.txt", Size: 500},
	}

	dist := router.AnalyzeDistribution(files)

	// Verify basic stats
	if dist.TotalFiles != 5 {
		t.Errorf("TotalFiles = %d, want 5", dist.TotalFiles)
	}

	if dist.TotalSize != 1500 {
		t.Errorf("TotalSize = %d, want 1500", dist.TotalSize)
	}

	if dist.ShardCount != 5 {
		t.Errorf("ShardCount = %d, want 5", dist.ShardCount)
	}

	// Verify distribution arrays have correct length
	if len(dist.FileCounts) != 5 {
		t.Errorf("len(FileCounts) = %d, want 5", len(dist.FileCounts))
	}

	if len(dist.Sizes) != 5 {
		t.Errorf("len(Sizes) = %d, want 5", len(dist.Sizes))
	}

	// Verify all files were counted
	totalCounted := 0
	for _, count := range dist.FileCounts {
		totalCounted += count
	}
	if totalCounted != 5 {
		t.Errorf("Total files counted = %d, want 5", totalCounted)
	}

	// Verify balance quality is in valid range [0, 1]
	if dist.BalanceQuality < 0 || dist.BalanceQuality > 1 {
		t.Errorf("BalanceQuality = %.4f, want 0.0-1.0", dist.BalanceQuality)
	}

	t.Logf("Distribution: %+v", dist)
}

func TestShardRouter_EmptyFiles(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Analyze empty file list
	dist := router.AnalyzeDistribution([]File{})

	if dist.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", dist.TotalFiles)
	}

	if dist.TotalSize != 0 {
		t.Errorf("TotalSize = %d, want 0", dist.TotalSize)
	}
}

func TestShardRouter_GettersSetters(t *testing.T) {
	config := &ShardRouterConfig{
		Strategy:   ShardBySize,
		ShardCount: 15,
	}

	router, err := NewShardRouter(config)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	if router.GetShardCount() != 15 {
		t.Errorf("GetShardCount() = %d, want 15", router.GetShardCount())
	}

	if router.GetStrategy() != ShardBySize {
		t.Errorf("GetStrategy() = %v, want %v", router.GetStrategy(), ShardBySize)
	}
}

func TestShardRouter_AdaptiveStrategy(t *testing.T) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardAdaptive,
		ShardCount: 10,
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	// Adaptive strategy currently falls back to hash
	file := File{
		Path: "/data/file.txt",
		Size: 1024,
	}

	shard1 := router.Route(file)
	shard2 := router.Route(file)

	// Should be deterministic (hash-based fallback)
	if shard1 != shard2 {
		t.Errorf("Adaptive routing not deterministic: got %d and %d", shard1, shard2)
	}
}

func TestClassifyContentType(t *testing.T) {
	tests := []struct {
		ext  string
		want contentType
	}{
		{".jpg", contentTypeImage},
		{".png", contentTypeImage},
		{".pdf", contentTypeDocument},
		{".txt", contentTypeDocument},
		{".go", contentTypeCode},
		{".py", contentTypeCode},
		{".exe", contentTypeBinary},
		{".so", contentTypeBinary},
		{".zip", contentTypeArchive},
		{".gz", contentTypeArchive},
		{".unknown", contentTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := classifyContentType(tt.ext)
			if got != tt.want {
				t.Errorf("classifyContentType(%s) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

// Benchmark hash-based routing
func BenchmarkShardRouter_RouteByHash(b *testing.B) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 10,
	})
	if err != nil {
		b.Fatalf("Failed to create router: %v", err)
	}

	file := File{
		Path:    "/data/file.txt",
		Size:    1024,
		ModTime: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = router.Route(file)
	}
}

// Benchmark size-based routing
func BenchmarkShardRouter_RouteBySize(b *testing.B) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardBySize,
		ShardCount: 10,
	})
	if err != nil {
		b.Fatalf("Failed to create router: %v", err)
	}

	file := File{
		Path:    "/data/file.txt",
		Size:    50 << 20, // 50MB
		ModTime: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = router.Route(file)
	}
}

// Benchmark distribution analysis
func BenchmarkShardRouter_AnalyzeDistribution(b *testing.B) {
	router, err := NewShardRouter(&ShardRouterConfig{
		Strategy:   ShardByHash,
		ShardCount: 10,
	})
	if err != nil {
		b.Fatalf("Failed to create router: %v", err)
	}

	// Generate 10,000 files
	files := make([]File, 10000)
	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/data/file_%d.txt", i),
			Size: 1024,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = router.AnalyzeDistribution(files)
	}
}
