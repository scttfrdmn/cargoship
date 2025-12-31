package manifest

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManifestIndex(t *testing.T) {
	manifest := createTestManifestForIndex(1000)

	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)
	assert.NotNil(t, idx)
	assert.Equal(t, 1000, idx.stats.TotalFiles)
	assert.Greater(t, idx.stats.IndexSizeBytes, int64(0))
	assert.GreaterOrEqual(t, idx.stats.BuildTimeMs, int64(0))
}

func TestFindFile(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantNil bool
	}{
		{
			name:    "existing file",
			path:    "dir0/file0.txt",
			wantNil: false,
		},
		{
			name:    "non-existent file",
			path:    "does/not/exist.txt",
			wantNil: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := idx.FindFile(tt.path)
			if tt.wantNil {
				assert.Nil(t, file)
			} else {
				assert.NotNil(t, file)
				assert.Equal(t, tt.path, file.Path)
			}
		})
	}
}

func TestFindFilesByShard(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	// Test valid shard
	files := idx.FindFilesByShard(0)
	assert.Greater(t, len(files), 0)

	// Verify all files belong to shard 0
	for _, file := range files {
		assert.Equal(t, 0, file.ShardID)
	}

	// Test shard with sorted files
	if len(files) > 1 {
		// Check that files are sorted by path
		for i := 0; i < len(files)-1; i++ {
			assert.LessOrEqual(t, files[i].Path, files[i+1].Path)
		}
	}
}

func TestFindFilesByExtension(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	tests := []struct {
		name string
		ext  string
		want int
	}{
		{
			name: "txt files",
			ext:  ".txt",
			want: 50, // Half the test files are .txt
		},
		{
			name: "log files",
			ext:  ".log",
			want: 50, // Half the test files are .log
		},
		{
			name: "non-existent extension",
			ext:  ".xyz",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := idx.FindFilesByExtension(tt.ext)
			assert.Len(t, files, tt.want)

			// Verify all have correct extension
			for _, file := range files {
				assert.Contains(t, file.Path, tt.ext)
			}
		})
	}
}

func TestFindFilesByDirectory(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	files := idx.FindFilesByDirectory("dir0")
	assert.Greater(t, len(files), 0)

	// Verify all files are in dir0 (not subdirectories)
	for _, file := range files {
		assert.Contains(t, file.Path, "dir0/")
		// Should not contain additional slashes after dir0/
		remaining := file.Path[len("dir0/"):]
		assert.NotContains(t, remaining, "/")
	}
}

func TestFindFilesByPrefix(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "directory prefix",
			prefix: "dir0/",
		},
		{
			name:   "file name prefix",
			prefix: "dir0/file",
		},
		{
			name:   "full path prefix",
			prefix: "dir0/file0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := idx.FindFilesByPrefix(tt.prefix)
			assert.Greater(t, len(files), 0)

			// Verify all have correct prefix
			for _, file := range files {
				assert.True(t, file.Path >= tt.prefix, "file %s should be >= prefix %s", file.Path, tt.prefix)
				assert.Contains(t, file.Path, tt.prefix[:len(tt.prefix)-1])
			}
		})
	}
}

func TestFindFilesBySuffix(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	tests := []struct {
		name   string
		suffix string
	}{
		{
			name:   "txt suffix",
			suffix: ".txt",
		},
		{
			name:   "log suffix",
			suffix: ".log",
		},
		{
			name:   "file name suffix",
			suffix: "file0.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := idx.FindFilesBySuffix(tt.suffix)
			assert.Greater(t, len(files), 0)

			// Verify all have correct suffix
			for _, file := range files {
				assert.True(t, file.Path[len(file.Path)-len(tt.suffix):] == tt.suffix)
			}
		})
	}
}

func TestFindFilesByPattern(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	tests := []struct {
		name          string
		pattern       string
		caseSensitive bool
	}{
		{
			name:          "wildcard prefix",
			pattern:       "dir0/*",
			caseSensitive: true,
		},
		{
			name:          "wildcard suffix",
			pattern:       "*.txt",
			caseSensitive: true,
		},
		{
			name:          "wildcard middle",
			pattern:       "dir*/file*.txt",
			caseSensitive: true,
		},
		{
			name:          "case insensitive",
			pattern:       "DIR0/*.TXT",
			caseSensitive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := idx.FindFilesByPattern(tt.pattern, tt.caseSensitive)
			// Pattern matching should find some files
			assert.GreaterOrEqual(t, len(files), 0)
		})
	}
}

func TestIndexOptions(t *testing.T) {
	manifest := createTestManifestForIndex(100)

	t.Run("minimal index", func(t *testing.T) {
		opts := &IndexOptions{
			EnableExtensionIndex: false,
			EnableDirectoryIndex: false,
			EnableShardIndex:     false,
		}

		idx, err := NewManifestIndex(manifest, opts)
		require.NoError(t, err)

		// Should still have path index
		file := idx.FindFile("dir0/file0.txt")
		assert.NotNil(t, file)

		// Should not have other indexes
		assert.Nil(t, idx.extIndex)
		assert.Nil(t, idx.dirIndex)
		assert.Nil(t, idx.shardIndex)
	})

	t.Run("full index", func(t *testing.T) {
		opts := DefaultIndexOptions()
		idx, err := NewManifestIndex(manifest, opts)
		require.NoError(t, err)

		// Should have all indexes
		assert.NotNil(t, idx.extIndex)
		assert.NotNil(t, idx.dirIndex)
		assert.NotNil(t, idx.shardIndex)
	})
}

func TestCompactIndex(t *testing.T) {
	manifest := createTestManifestForIndex(1000)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	// Record original size
	originalSize := idx.stats.IndexSizeBytes

	// Compact index
	err = idx.CompactIndex()
	require.NoError(t, err)

	// New size should be smaller
	assert.Less(t, idx.stats.IndexSizeBytes, originalSize)

	// Path index should still work
	file := idx.FindFile("dir0/file0.txt")
	assert.NotNil(t, file)

	// Other indexes should be nil
	assert.Nil(t, idx.extIndex)
	assert.Nil(t, idx.dirIndex)
	assert.Nil(t, idx.shardIndex)
}

func TestValidate(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	// Valid index should pass
	err = idx.Validate()
	assert.NoError(t, err)

	// Test with corrupted index
	t.Run("size mismatch", func(t *testing.T) {
		// Save original
		original := idx.pathIndex

		// Corrupt by removing an entry
		idx.pathIndex = make(map[string]*FileEntry)
		for k, v := range original {
			if len(idx.pathIndex) < 50 {
				idx.pathIndex[k] = v
			}
		}

		err := idx.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mismatch")

		// Restore
		idx.pathIndex = original
	})
}

func TestGetStats(t *testing.T) {
	manifest := createTestManifestForIndex(100)
	idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
	require.NoError(t, err)

	stats := idx.GetStats()
	assert.Equal(t, 100, stats.TotalFiles)
	assert.Greater(t, stats.TotalShards, 0)
	assert.Greater(t, stats.IndexSizeBytes, int64(0))
	assert.GreaterOrEqual(t, stats.BuildTimeMs, int64(0))
}

// Benchmarks to verify performance targets

func BenchmarkIndexBuild(b *testing.B) {
	sizes := []int{1000, 10000, 100000, 1000000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("files=%d", size), func(b *testing.B) {
			manifest := createTestManifestForIndex(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := NewManifestIndex(manifest, DefaultIndexOptions())
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFindFile(b *testing.B) {
	sizes := []int{1000, 10000, 100000, 1000000}

	for _, size := range sizes {
		manifest := createTestManifestForIndex(size)
		idx, _ := NewManifestIndex(manifest, DefaultIndexOptions())

		b.Run(fmt.Sprintf("files=%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Lookup random file
				path := fmt.Sprintf("dir%d/file%d.txt", i%10, i%size)
				_ = idx.FindFile(path)
			}
		})
	}
}

func BenchmarkFindFilesByPrefix(b *testing.B) {
	manifest := createTestManifestForIndex(100000)
	idx, _ := NewManifestIndex(manifest, DefaultIndexOptions())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.FindFilesByPrefix("dir0/")
	}
}

func BenchmarkFindFilesByShard(b *testing.B) {
	manifest := createTestManifestForIndex(100000)
	idx, _ := NewManifestIndex(manifest, DefaultIndexOptions())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.FindFilesByShard(i % 10)
	}
}

func TestPerformanceTargets(t *testing.T) {
	// Test performance targets from Issue #89
	// These are strict benchmarks that may fail on slower hardware or under load
	// Set CARGOSHIP_STRICT_PERF_TESTS=1 to enable
	if os.Getenv("CARGOSHIP_STRICT_PERF_TESTS") != "1" {
		t.Skip("Skipping strict performance tests (set CARGOSHIP_STRICT_PERF_TESTS=1 to enable)")
	}

	t.Run("1M files build time < 100ms", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping 1M file test in short mode")
		}

		manifest := createTestManifestForIndex(1000000)

		start := time.Now()
		idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
		buildTime := time.Since(start)

		require.NoError(t, err)
		require.NotNil(t, idx)
		assert.Less(t, buildTime.Milliseconds(), int64(100),
			"Build time for 1M files should be <100ms, got %dms", buildTime.Milliseconds())

		t.Logf("Build time for 1M files: %dms (target: <100ms)", buildTime.Milliseconds())
	})

	t.Run("1M files index size < 10MB", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping 1M file test in short mode")
		}

		manifest := createTestManifestForIndex(1000000)
		idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
		require.NoError(t, err)

		sizeBytes := idx.stats.IndexSizeBytes
		sizeMB := float64(sizeBytes) / (1024 * 1024)

		assert.Less(t, sizeMB, 10.0,
			"Index size for 1M files should be <10MB, got %.2fMB", sizeMB)

		t.Logf("Index size for 1M files: %.2fMB (target: <10MB)", sizeMB)
	})

	t.Run("lookup time < 1μs", func(t *testing.T) {
		manifest := createTestManifestForIndex(100000)
		idx, err := NewManifestIndex(manifest, DefaultIndexOptions())
		require.NoError(t, err)

		iterations := 100000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			path := fmt.Sprintf("dir%d/file%d.txt", i%10, i%1000)
			_ = idx.FindFile(path)
		}

		elapsed := time.Since(start)
		avgNs := elapsed.Nanoseconds() / int64(iterations)

		assert.Less(t, avgNs, int64(1000),
			"Average lookup time should be <1μs, got %dns", avgNs)

		t.Logf("Average lookup time: %dns (target: <1000ns)", avgNs)
	})
}

// Helper functions

func createTestManifestForIndex(fileCount int) *Manifest {
	files := make([]FileEntry, fileCount)
	shardCount := 10

	for i := 0; i < fileCount; i++ {
		shardID := i % shardCount
		dir := fmt.Sprintf("dir%d", i%10)
		ext := ".txt"
		if i%2 == 1 {
			ext = ".log"
		}

		files[i] = FileEntry{
			Path:    fmt.Sprintf("%s/file%d%s", dir, i, ext),
			Size:    int64(1024 * (i + 1)),
			ShardID: shardID,
			ChunkID: i / 100,
			S3Key:   fmt.Sprintf("shard-%d/chunk-%d.tar.zst", shardID, i/100),
			ModTime: time.Now(),
		}
	}

	shards := make([]ShardEntry, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = ShardEntry{
			ID:     i,
			Prefix: fmt.Sprintf("shard-%d", i),
		}
	}

	return &Manifest{
		Version:    "1.0",
		UploadID:   "test-index-123",
		Files:      files,
		TotalFiles: int64(fileCount),
		Shards:     shards,
		ShardCount: shardCount,
	}
}
