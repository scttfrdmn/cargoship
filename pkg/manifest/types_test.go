package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewManifestQuery tests creation of manifest query interface
func TestNewManifestQuery(t *testing.T) {
	m := &Manifest{
		Version:   "1.0",
		UploadID:  "test-123",
		CreatedAt: time.Now(),
	}

	query := NewManifestQuery(m)
	require.NotNil(t, query)
	assert.Equal(t, m, query.manifest)
}

// TestManifestQuery_FindFile tests exact file path lookup
func TestManifestQuery_FindFile(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100},
			{Path: "dir/file2.log", Size: 200},
			{Path: "data/file3.csv", Size: 300},
		},
	}
	query := NewManifestQuery(m)

	tests := []struct {
		name     string
		path     string
		expected *FileEntry
	}{
		{
			name: "find first file",
			path: "file1.txt",
			expected: &FileEntry{
				Path: "file1.txt",
				Size: 100,
			},
		},
		{
			name: "find nested file",
			path: "dir/file2.log",
			expected: &FileEntry{
				Path: "dir/file2.log",
				Size: 200,
			},
		},
		{
			name:     "file not found",
			path:     "nonexistent.txt",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := query.FindFile(tt.path)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Path, result.Path)
				assert.Equal(t, tt.expected.Size, result.Size)
			}
		})
	}
}

// TestManifestQuery_ListFiles_NoPattern tests listing all files
func TestManifestQuery_ListFiles_NoPattern(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt"},
			{Path: "file2.log"},
			{Path: "file3.csv"},
		},
	}
	query := NewManifestQuery(m)

	// Empty pattern should return all files
	result := query.ListFiles("")
	assert.Len(t, result, 3)
}

// TestManifestQuery_ListFiles_GlobPatterns tests glob pattern matching
func TestManifestQuery_ListFiles_GlobPatterns(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt"},
			{Path: "file2.log"},
			{Path: "file3.txt"},
			{Path: "dir/file4.txt"},
			{Path: "dir/file5.log"},
			{Path: "data/report.csv"},
			{Path: "data/summary.csv"},
		},
	}
	query := NewManifestQuery(m)

	tests := []struct {
		name          string
		pattern       string
		expectedPaths []string
	}{
		{
			name:    "match *.txt files",
			pattern: "*.txt",
			expectedPaths: []string{
				"file1.txt",
				"file3.txt",
				"dir/file4.txt", // matches basename
			},
		},
		{
			name:    "match *.log files",
			pattern: "*.log",
			expectedPaths: []string{
				"file2.log",
				"dir/file5.log",
			},
		},
		{
			name:    "match files in dir/",
			pattern: "dir/*.txt",
			expectedPaths: []string{
				"dir/file4.txt",
			},
		},
		{
			name:    "match csv files",
			pattern: "*.csv",
			expectedPaths: []string{
				"data/report.csv",
				"data/summary.csv",
			},
		},
		{
			name:    "match with ? wildcard",
			pattern: "file?.txt",
			expectedPaths: []string{
				"file1.txt",
				"file3.txt",
				"dir/file4.txt", // matches basename
			},
		},
		{
			name:          "no matches",
			pattern:       "*.json",
			expectedPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := query.ListFiles(tt.pattern)

			var resultPaths []string
			for _, f := range result {
				resultPaths = append(resultPaths, f.Path)
			}

			if len(tt.expectedPaths) == 0 {
				assert.Empty(t, resultPaths)
			} else {
				assert.ElementsMatch(t, tt.expectedPaths, resultPaths)
			}
		})
	}
}

// TestManifestQuery_ListFiles_InvalidPattern tests graceful handling of invalid patterns
func TestManifestQuery_ListFiles_InvalidPattern(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt"},
			{Path: "file2.log"},
		},
	}
	query := NewManifestQuery(m)

	// Invalid glob pattern should return empty results (not panic)
	result := query.ListFiles("[invalid")
	assert.Empty(t, result, "Invalid pattern should return empty results")
}

// TestManifestQuery_FilesInShard tests shard-based file filtering
func TestManifestQuery_FilesInShard(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", ShardID: 0},
			{Path: "file2.txt", ShardID: 1},
			{Path: "file3.txt", ShardID: 0},
			{Path: "file4.txt", ShardID: 2},
			{Path: "file5.txt", ShardID: 1},
		},
	}
	query := NewManifestQuery(m)

	tests := []struct {
		name          string
		shardID       int
		expectedPaths []string
	}{
		{
			name:    "shard 0",
			shardID: 0,
			expectedPaths: []string{
				"file1.txt",
				"file3.txt",
			},
		},
		{
			name:    "shard 1",
			shardID: 1,
			expectedPaths: []string{
				"file2.txt",
				"file5.txt",
			},
		},
		{
			name:    "shard 2",
			shardID: 2,
			expectedPaths: []string{
				"file4.txt",
			},
		},
		{
			name:          "shard 999 (nonexistent)",
			shardID:       999,
			expectedPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := query.FilesInShard(tt.shardID)

			var resultPaths []string
			for _, f := range result {
				resultPaths = append(resultPaths, f.Path)
			}

			if len(tt.expectedPaths) == 0 {
				assert.Empty(t, resultPaths)
			} else {
				assert.ElementsMatch(t, tt.expectedPaths, resultPaths)
			}
		})
	}
}

// TestManifestQuery_FilesInChunk tests chunk-based file filtering
func TestManifestQuery_FilesInChunk(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", ChunkID: 0},
			{Path: "file2.txt", ChunkID: 1},
			{Path: "file3.txt", ChunkID: 0},
			{Path: "file4.txt", ChunkID: 2},
		},
	}
	query := NewManifestQuery(m)

	tests := []struct {
		name          string
		chunkID       int
		expectedPaths []string
	}{
		{
			name:    "chunk 0",
			chunkID: 0,
			expectedPaths: []string{
				"file1.txt",
				"file3.txt",
			},
		},
		{
			name:    "chunk 1",
			chunkID: 1,
			expectedPaths: []string{
				"file2.txt",
			},
		},
		{
			name:          "chunk 999 (nonexistent)",
			chunkID:       999,
			expectedPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := query.FilesInChunk(tt.chunkID)

			var resultPaths []string
			for _, f := range result {
				resultPaths = append(resultPaths, f.Path)
			}

			if len(tt.expectedPaths) == 0 {
				assert.Empty(t, resultPaths)
			} else {
				assert.ElementsMatch(t, tt.expectedPaths, resultPaths)
			}
		})
	}
}

// TestManifestQuery_GetSummary tests manifest summary generation
func TestManifestQuery_GetSummary(t *testing.T) {
	now := time.Now()
	m := &Manifest{
		UploadID:         "test-upload-123",
		TotalFiles:       1000,
		TotalBytes:       500000000,
		TotalChunks:      50,
		ShardCount:       8,
		CompressionRatio: 0.42,
		CreatedAt:        now,
		CompletedAt:      now.Add(5 * time.Minute),
	}
	query := NewManifestQuery(m)

	summary := query.GetSummary()

	assert.Equal(t, int64(1000), summary.TotalFiles)
	assert.Equal(t, int64(500000000), summary.TotalBytes)
	assert.Equal(t, 50, summary.TotalChunks)
	assert.Equal(t, 8, summary.ShardCount)
	assert.Equal(t, 0.42, summary.CompressionRatio)
	assert.Equal(t, "test-upload-123", summary.UploadID)
	assert.Equal(t, now, summary.CreatedAt)
	assert.Equal(t, now.Add(5*time.Minute), summary.CompletedAt)
}

// TestManifestQuery_FilesInShard_VerifyShardID tests that shard filtering is correct
func TestManifestQuery_FilesInShard_VerifyShardID(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", ShardID: 0},
			{Path: "file2.txt", ShardID: 1},
			{Path: "file3.txt", ShardID: 0},
		},
	}
	query := NewManifestQuery(m)

	result := query.FilesInShard(0)

	// All returned files should have ShardID == 0
	for _, file := range result {
		assert.Equal(t, 0, file.ShardID, "File %s should be in shard 0", file.Path)
	}
}

// TestManifestQuery_FilesInChunk_VerifyChunkID tests that chunk filtering is correct
func TestManifestQuery_FilesInChunk_VerifyChunkID(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", ChunkID: 5},
			{Path: "file2.txt", ChunkID: 10},
			{Path: "file3.txt", ChunkID: 5},
		},
	}
	query := NewManifestQuery(m)

	result := query.FilesInChunk(5)

	// All returned files should have ChunkID == 5
	for _, file := range result {
		assert.Equal(t, 5, file.ChunkID, "File %s should be in chunk 5", file.Path)
	}
}

// TestManifestQuery_GetShard tests shard metadata retrieval (Issue #90)
func TestManifestQuery_GetShard(t *testing.T) {
	m := &Manifest{
		Shards: []ShardEntry{
			{
				ID:               0,
				Prefix:           "shard-0",
				ChunkCount:       10,
				FileCount:        100,
				UncompressedSize: 1000000,
				CompressedSize:   500000,
			},
			{
				ID:               1,
				Prefix:           "shard-1",
				ChunkCount:       8,
				FileCount:        80,
				UncompressedSize: 800000,
				CompressedSize:   400000,
			},
			{
				ID:               2,
				Prefix:           "shard-2",
				ChunkCount:       12,
				FileCount:        120,
				UncompressedSize: 1200000,
				CompressedSize:   600000,
			},
		},
	}
	query := NewManifestQuery(m)

	tests := []struct {
		name        string
		shardID     int
		expectFound bool
		expectShard *ShardEntry
	}{
		{
			name:        "valid shard 0",
			shardID:     0,
			expectFound: true,
			expectShard: &m.Shards[0],
		},
		{
			name:        "valid shard 1",
			shardID:     1,
			expectFound: true,
			expectShard: &m.Shards[1],
		},
		{
			name:        "valid shard 2",
			shardID:     2,
			expectFound: true,
			expectShard: &m.Shards[2],
		},
		{
			name:        "invalid shard (negative)",
			shardID:     -1,
			expectFound: false,
			expectShard: nil,
		},
		{
			name:        "invalid shard (out of range)",
			shardID:     999,
			expectFound: false,
			expectShard: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard := query.GetShard(tt.shardID)

			if tt.expectFound {
				assert.NotNil(t, shard, "Expected to find shard %d", tt.shardID)
				assert.Equal(t, tt.expectShard.ID, shard.ID)
				assert.Equal(t, tt.expectShard.Prefix, shard.Prefix)
				assert.Equal(t, tt.expectShard.ChunkCount, shard.ChunkCount)
				assert.Equal(t, tt.expectShard.FileCount, shard.FileCount)
				assert.Equal(t, tt.expectShard.UncompressedSize, shard.UncompressedSize)
				assert.Equal(t, tt.expectShard.CompressedSize, shard.CompressedSize)
			} else {
				assert.Nil(t, shard, "Expected nil for invalid shard %d", tt.shardID)
			}
		})
	}
}

// TestManifestQuery_CountFiles tests file count retrieval (Issue #90)
func TestManifestQuery_CountFiles(t *testing.T) {
	tests := []struct {
		name        string
		totalFiles  int64
		actualFiles int
		expectCount int64
	}{
		{
			name:        "zero files",
			totalFiles:  0,
			actualFiles: 0,
			expectCount: 0,
		},
		{
			name:        "10 files",
			totalFiles:  10,
			actualFiles: 10,
			expectCount: 10,
		},
		{
			name:        "1000 files",
			totalFiles:  1000,
			actualFiles: 1000,
			expectCount: 1000,
		},
		{
			name:        "1M files",
			totalFiles:  1000000,
			actualFiles: 0, // Don't actually create 1M files in test
			expectCount: 1000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				TotalFiles: tt.totalFiles,
				Files:      make([]FileEntry, tt.actualFiles),
			}

			query := NewManifestQuery(m)
			count := query.CountFiles()

			assert.Equal(t, tt.expectCount, count, "File count mismatch")
		})
	}
}

// TestManifestQuery_TotalSize tests total size retrieval (Issue #90)
func TestManifestQuery_TotalSize(t *testing.T) {
	tests := []struct {
		name       string
		totalBytes int64
		expectSize int64
	}{
		{
			name:       "zero bytes",
			totalBytes: 0,
			expectSize: 0,
		},
		{
			name:       "1 KB",
			totalBytes: 1024,
			expectSize: 1024,
		},
		{
			name:       "1 MB",
			totalBytes: 1024 * 1024,
			expectSize: 1024 * 1024,
		},
		{
			name:       "1 GB",
			totalBytes: 1024 * 1024 * 1024,
			expectSize: 1024 * 1024 * 1024,
		},
		{
			name:       "1 TB",
			totalBytes: 1024 * 1024 * 1024 * 1024,
			expectSize: 1024 * 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				TotalBytes: tt.totalBytes,
			}

			query := NewManifestQuery(m)
			size := query.TotalSize()

			assert.Equal(t, tt.expectSize, size, "Total size mismatch")
		})
	}
}

// TestManifestQuery_API tests the complete query API (Issue #90)
func TestManifestQuery_API(t *testing.T) {
	// Create a comprehensive manifest
	m := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-query-api",
		TotalFiles: 5,
		TotalBytes: 5000,
		Shards: []ShardEntry{
			{ID: 0, Prefix: "shard-0", ChunkCount: 2, FileCount: 3, UncompressedSize: 3000, CompressedSize: 1500},
			{ID: 1, Prefix: "shard-1", ChunkCount: 1, FileCount: 2, UncompressedSize: 2000, CompressedSize: 1000},
		},
		Files: []FileEntry{
			{Path: "file1.txt", Size: 1000, ShardID: 0, ChunkID: 0},
			{Path: "file2.log", Size: 1000, ShardID: 0, ChunkID: 0},
			{Path: "file3.txt", Size: 1000, ShardID: 0, ChunkID: 1},
			{Path: "file4.log", Size: 1000, ShardID: 1, ChunkID: 2},
			{Path: "file5.txt", Size: 1000, ShardID: 1, ChunkID: 2},
		},
	}

	query := NewManifestQuery(m)

	// Test FindFile
	file := query.FindFile("file1.txt")
	assert.NotNil(t, file, "Should find file1.txt")
	assert.Equal(t, "file1.txt", file.Path)
	assert.Equal(t, int64(1000), file.Size)

	// Test ListFiles with pattern
	txtFiles := query.ListFiles("*.txt")
	assert.Equal(t, 3, len(txtFiles), "Should find 3 txt files")

	// Test GetShard
	shard := query.GetShard(0)
	assert.NotNil(t, shard, "Should find shard 0")
	assert.Equal(t, "shard-0", shard.Prefix)
	assert.Equal(t, int64(3), shard.FileCount)

	// Test CountFiles
	count := query.CountFiles()
	assert.Equal(t, int64(5), count, "Should have 5 files")

	// Test TotalSize
	size := query.TotalSize()
	assert.Equal(t, int64(5000), size, "Should have 5000 bytes total")
}
