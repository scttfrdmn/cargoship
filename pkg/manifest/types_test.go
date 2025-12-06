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
