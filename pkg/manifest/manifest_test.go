package manifest

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewBuilder tests creating a new manifest builder
func TestNewBuilder(t *testing.T) {
	uploadID := "test-upload-123"
	sourcePath := "/data/test"
	bucket := "test-bucket"
	prefix := "cargoship"
	region := "us-west-2"

	builder, err := NewBuilder(uploadID, sourcePath, bucket, prefix, region)
	require.NoError(t, err)
	require.NotNil(t, builder)

	m := builder.Build()
	assert.Equal(t, ManifestVersion, m.Version)
	assert.Equal(t, uploadID, m.UploadID)
	assert.Equal(t, sourcePath, m.SourcePath)
	assert.Equal(t, bucket, m.Bucket)
	assert.Equal(t, prefix, m.Prefix)
	assert.Equal(t, region, m.Region)
	assert.NotEmpty(t, m.Hostname)
	assert.NotZero(t, m.CreatedAt)
	assert.Zero(t, m.CompletedAt)
	assert.Empty(t, m.Files)
	assert.Empty(t, m.Chunks)
	assert.Empty(t, m.Shards)
}

// TestNewBuilderFromExisting tests creating a builder from existing manifest
func TestNewBuilderFromExisting(t *testing.T) {
	existing := &Manifest{
		Version:          ManifestVersion,
		UploadID:         "existing-123",
		CreatedAt:        time.Now().Add(-1 * time.Hour),
		CompletedAt:      time.Now().Add(-30 * time.Minute),
		SourcePath:       "/data/original",
		Hostname:         "original-host",
		Bucket:           "existing-bucket",
		Prefix:           "existing-prefix",
		Region:           "us-east-1",
		TotalFiles:       10,
		TotalBytes:       1000,
		TotalChunks:      2,
		ShardCount:       4,
		CompressionType:  "zstd",
		CompressionLevel: 3,
		CompressionRatio: 0.5,
		Files: []FileEntry{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
		},
		Chunks: []ChunkEntry{
			{ID: 0, CompressedSize: 250},
			{ID: 1, CompressedSize: 250},
		},
		Shards: []ShardEntry{
			{ID: 0, ChunkCount: 1},
			{ID: 1, ChunkCount: 1},
		},
	}

	builder, err := NewBuilderFromExisting(existing)
	require.NoError(t, err)
	require.NotNil(t, builder)

	m := builder.Build()

	// Verify all fields copied correctly
	assert.Equal(t, existing.Version, m.Version)
	assert.Equal(t, existing.UploadID, m.UploadID)
	assert.Equal(t, existing.CreatedAt, m.CreatedAt)
	assert.Equal(t, existing.CompletedAt, m.CompletedAt)
	assert.Equal(t, existing.SourcePath, m.SourcePath)
	assert.Equal(t, existing.Bucket, m.Bucket)
	assert.Equal(t, existing.Prefix, m.Prefix)
	assert.Equal(t, existing.Region, m.Region)
	assert.Equal(t, existing.TotalFiles, m.TotalFiles)
	assert.Equal(t, existing.TotalBytes, m.TotalBytes)
	assert.Equal(t, existing.TotalChunks, m.TotalChunks)
	assert.Equal(t, existing.ShardCount, m.ShardCount)
	assert.Equal(t, existing.CompressionType, m.CompressionType)
	assert.Equal(t, existing.CompressionLevel, m.CompressionLevel)
	assert.Equal(t, existing.CompressionRatio, m.CompressionRatio)

	// Verify hostname updated to current host (not original)
	assert.NotEqual(t, existing.Hostname, m.Hostname)
	assert.NotEmpty(t, m.Hostname)

	// Verify slices are copied (not shared)
	assert.Len(t, m.Files, len(existing.Files))
	assert.Len(t, m.Chunks, len(existing.Chunks))
	assert.Len(t, m.Shards, len(existing.Shards))

	// Modify cloned manifest shouldn't affect original
	m.Files[0].Path = "modified.txt"
	assert.NotEqual(t, existing.Files[0].Path, m.Files[0].Path)
}

// TestBuilder_AddFile tests adding files to manifest
func TestBuilder_AddFile(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add files
	file1 := FileEntry{Path: "file1.txt", Size: 100, ChunkID: 0}
	file2 := FileEntry{Path: "file2.txt", Size: 200, ChunkID: 0}
	file3 := FileEntry{Path: "file3.txt", Size: 300, ChunkID: 1}

	builder.AddFile(file1)
	builder.AddFile(file2)
	builder.AddFile(file3)

	m := builder.Build()
	assert.Len(t, m.Files, 3)
	assert.Equal(t, int64(3), m.TotalFiles)
	assert.Equal(t, int64(600), m.TotalBytes)

	// Verify files in order
	assert.Equal(t, "file1.txt", m.Files[0].Path)
	assert.Equal(t, "file2.txt", m.Files[1].Path)
	assert.Equal(t, "file3.txt", m.Files[2].Path)
}

// TestBuilder_AddFileBatch tests adding multiple files in a single batch (Issue #34 Phase 1.4)
func TestBuilder_AddFileBatch(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add files in batch
	entries := []FileEntry{
		{Path: "file1.txt", Size: 100, ChunkID: 0},
		{Path: "file2.txt", Size: 200, ChunkID: 0},
		{Path: "file3.txt", Size: 300, ChunkID: 1},
	}

	builder.AddFileBatch(entries)

	m := builder.Build()
	assert.Len(t, m.Files, 3)
	assert.Equal(t, int64(3), m.TotalFiles)
	assert.Equal(t, int64(600), m.TotalBytes)

	// Verify files in order
	assert.Equal(t, "file1.txt", m.Files[0].Path)
	assert.Equal(t, "file2.txt", m.Files[1].Path)
	assert.Equal(t, "file3.txt", m.Files[2].Path)
}

// TestBuilder_AddFileBatch_Empty tests that adding empty batch is safe (Issue #34 Phase 1.4)
func TestBuilder_AddFileBatch_Empty(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add empty batch - should not error
	builder.AddFileBatch([]FileEntry{})

	m := builder.Build()
	assert.Len(t, m.Files, 0)
	assert.Equal(t, int64(0), m.TotalFiles)
	assert.Equal(t, int64(0), m.TotalBytes)
}

// TestBuilder_AddFileBatch_Concurrent tests concurrent batch additions (Issue #34 Phase 1.4)
func TestBuilder_AddFileBatch_Concurrent(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add files concurrently from multiple goroutines
	numGoroutines := 10
	filesPerBatch := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(batchID int) {
			defer wg.Done()

			entries := make([]FileEntry, filesPerBatch)
			for j := 0; j < filesPerBatch; j++ {
				entries[j] = FileEntry{
					Path:    fmt.Sprintf("batch%d-file%d.txt", batchID, j),
					Size:    100,
					ChunkID: batchID,
				}
			}

			builder.AddFileBatch(entries)
		}(i)
	}

	wg.Wait()

	m := builder.Build()
	assert.Len(t, m.Files, numGoroutines*filesPerBatch)
	assert.Equal(t, int64(numGoroutines*filesPerBatch), m.TotalFiles)
	assert.Equal(t, int64(numGoroutines*filesPerBatch*100), m.TotalBytes)
}

// TestBuilder_AddChunk tests adding chunks to manifest
func TestBuilder_AddChunk(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add chunks
	chunk1 := ChunkEntry{ID: 0, CompressedSize: 1024, UncompressedSize: 2048}
	chunk2 := ChunkEntry{ID: 1, CompressedSize: 2048, UncompressedSize: 4096}

	builder.AddChunk(chunk1)
	builder.AddChunk(chunk2)

	m := builder.Build()
	assert.Len(t, m.Chunks, 2)
	assert.Equal(t, 2, m.TotalChunks)

	// Verify chunks
	assert.Equal(t, 0, m.Chunks[0].ID)
	assert.Equal(t, int64(1024), m.Chunks[0].CompressedSize)
	assert.Equal(t, 1, m.Chunks[1].ID)
	assert.Equal(t, int64(2048), m.Chunks[1].CompressedSize)
}

// TestBuilder_SetCompression tests setting compression info
func TestBuilder_SetCompression(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	builder.SetCompression("zstd", 3, 0.42)

	m := builder.Build()
	assert.Equal(t, "zstd", m.CompressionType)
	assert.Equal(t, 3, m.CompressionLevel)
	assert.Equal(t, 0.42, m.CompressionRatio)
}

// TestBuilder_SetShardCount tests setting shard count
func TestBuilder_SetShardCount(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	builder.SetShardCount(8)

	m := builder.Build()
	assert.Equal(t, 8, m.ShardCount)
	assert.Len(t, m.Shards, 8)

	// Verify each shard initialized correctly
	for i := 0; i < 8; i++ {
		assert.Equal(t, i, m.Shards[i].ID)
		assert.Equal(t, "prefix/uploads/test-123/shard-"+string(rune('0'+i)), m.Shards[i].Prefix)
		assert.NotNil(t, m.Shards[i].ChunkKeys)
		assert.Empty(t, m.Shards[i].ChunkKeys)
	}
}

// TestBuilder_UpdateShardStats tests updating shard statistics
func TestBuilder_UpdateShardStats(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	builder.SetShardCount(4)

	// Update shard 0 stats
	builder.UpdateShardStats(0, "chunk-0.tar.zst", 10, 1000, 500)
	builder.UpdateShardStats(0, "chunk-1.tar.zst", 5, 500, 250)

	// Update shard 1 stats
	builder.UpdateShardStats(1, "chunk-2.tar.zst", 15, 2000, 1000)

	m := builder.Build()

	// Verify shard 0
	assert.Equal(t, 2, m.Shards[0].ChunkCount)
	assert.Equal(t, int64(15), m.Shards[0].FileCount)
	assert.Equal(t, int64(1500), m.Shards[0].UncompressedSize)
	assert.Equal(t, int64(750), m.Shards[0].CompressedSize)
	assert.Equal(t, []string{"chunk-0.tar.zst", "chunk-1.tar.zst"}, m.Shards[0].ChunkKeys)

	// Verify shard 1
	assert.Equal(t, 1, m.Shards[1].ChunkCount)
	assert.Equal(t, int64(15), m.Shards[1].FileCount)
	assert.Equal(t, int64(2000), m.Shards[1].UncompressedSize)
	assert.Equal(t, int64(1000), m.Shards[1].CompressedSize)
	assert.Equal(t, []string{"chunk-2.tar.zst"}, m.Shards[1].ChunkKeys)

	// Verify shard 2 unchanged
	assert.Equal(t, 0, m.Shards[2].ChunkCount)
	assert.Equal(t, int64(0), m.Shards[2].FileCount)
}

// TestBuilder_UpdateShardStats_OutOfBounds tests boundary checking
func TestBuilder_UpdateShardStats_OutOfBounds(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	builder.SetShardCount(2)

	// Update with invalid shard IDs should not panic
	builder.UpdateShardStats(-1, "chunk.tar.zst", 10, 1000, 500)
	builder.UpdateShardStats(10, "chunk.tar.zst", 10, 1000, 500)

	m := builder.Build()
	// Shards should remain unchanged
	assert.Equal(t, 0, m.Shards[0].ChunkCount)
	assert.Equal(t, 0, m.Shards[1].ChunkCount)
}

// TestBuilder_UpdateFileS3Keys tests updating file S3 keys
func TestBuilder_UpdateFileS3Keys(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add files to different chunks
	builder.AddFile(FileEntry{Path: "file1.txt", Size: 100, ChunkID: 0})
	builder.AddFile(FileEntry{Path: "file2.txt", Size: 200, ChunkID: 0})
	builder.AddFile(FileEntry{Path: "file3.txt", Size: 300, ChunkID: 1})
	builder.AddFile(FileEntry{Path: "file4.txt", Size: 400, ChunkID: 1})

	// Update chunk 0 files
	builder.UpdateFileS3Keys(0, 2, "shard-2/chunk-0.tar.zst")

	// Update chunk 1 files
	builder.UpdateFileS3Keys(1, 3, "shard-3/chunk-1.tar.zst")

	m := builder.Build()

	// Verify chunk 0 files updated
	assert.Equal(t, 2, m.Files[0].ShardID)
	assert.Equal(t, "shard-2/chunk-0.tar.zst", m.Files[0].S3Key)
	assert.Equal(t, 2, m.Files[1].ShardID)
	assert.Equal(t, "shard-2/chunk-0.tar.zst", m.Files[1].S3Key)

	// Verify chunk 1 files updated
	assert.Equal(t, 3, m.Files[2].ShardID)
	assert.Equal(t, "shard-3/chunk-1.tar.zst", m.Files[2].S3Key)
	assert.Equal(t, 3, m.Files[3].ShardID)
	assert.Equal(t, "shard-3/chunk-1.tar.zst", m.Files[3].S3Key)
}

// TestBuilder_Finalize tests finalizing the manifest
func TestBuilder_Finalize(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	createdAt := builder.Build().CreatedAt

	// CompletedAt should be zero before finalize
	assert.Zero(t, builder.Build().CompletedAt)

	// Finalize should set CompletedAt
	manifest := builder.Finalize()
	assert.NotZero(t, manifest.CompletedAt)
	assert.True(t, manifest.CompletedAt.After(createdAt))
}

// TestBuilder_Build tests building without finalizing
func TestBuilder_Build(t *testing.T) {
	builder, err := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Build should return manifest without setting CompletedAt
	m1 := builder.Build()
	assert.Zero(t, m1.CompletedAt)

	// Build can be called multiple times
	m2 := builder.Build()
	assert.Zero(t, m2.CompletedAt)

	// Both should be the same manifest
	assert.Equal(t, m1.UploadID, m2.UploadID)
}

// TestManifest_ToJSON tests JSON serialization
func TestManifest_ToJSON(t *testing.T) {
	m := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-123",
		CreatedAt:  time.Date(2025, 12, 6, 12, 0, 0, 0, time.UTC),
		SourcePath: "/data/test",
		Bucket:     "test-bucket",
		Prefix:     "cargoship",
		Region:     "us-west-2",
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100},
		},
	}

	data, err := m.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify it's valid JSON by unmarshaling
	parsed, err := FromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, m.UploadID, parsed.UploadID)
	assert.Equal(t, m.Version, parsed.Version)
	assert.Len(t, parsed.Files, 1)
	assert.Equal(t, "file1.txt", parsed.Files[0].Path)
}

// TestManifest_ToJSONCompressed tests compressed JSON serialization
func TestManifest_ToJSONCompressed(t *testing.T) {
	m := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-123",
		CreatedAt:  time.Date(2025, 12, 6, 12, 0, 0, 0, time.UTC),
		SourcePath: "/data/test",
		Bucket:     "test-bucket",
		Prefix:     "cargoship",
		Region:     "us-west-2",
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100},
			{Path: "file2.txt", Size: 200},
		},
	}

	compressed, err := m.ToJSONCompressed()
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)

	// Note: For small data, gzip might actually be larger due to overhead
	// We just verify compression and decompression work correctly

	// Verify we can decompress and parse
	parsed, err := FromJSONCompressed(compressed)
	require.NoError(t, err)
	assert.Equal(t, m.UploadID, parsed.UploadID)
	assert.Len(t, parsed.Files, 2)
}

// TestFromJSON tests JSON deserialization
func TestFromJSON(t *testing.T) {
	jsonData := []byte(`{
		"version": "1.0",
		"upload_id": "test-123",
		"created_at": "2025-12-06T12:00:00Z",
		"source_path": "/data/test",
		"bucket": "test-bucket",
		"prefix": "cargoship",
		"region": "us-west-2",
		"files": [
			{"path": "file1.txt", "size": 100}
		]
	}`)

	m, err := FromJSON(jsonData)
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, "test-123", m.UploadID)
	assert.Equal(t, "/data/test", m.SourcePath)
	assert.Len(t, m.Files, 1)
	assert.Equal(t, "file1.txt", m.Files[0].Path)
	assert.Equal(t, int64(100), m.Files[0].Size)
}

// TestFromJSON_Invalid tests error handling for invalid JSON
func TestFromJSON_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "invalid JSON",
			data: []byte(`{"invalid json`),
		},
		{
			name: "empty data",
			data: []byte{},
		},
		{
			name: "not JSON",
			data: []byte(`this is not json`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := FromJSON(tt.data)
			assert.Error(t, err)
			assert.Nil(t, m)
		})
	}
}

// TestFromJSONCompressed tests compressed JSON deserialization
func TestFromJSONCompressed(t *testing.T) {
	// Create a manifest and compress it
	original := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-123",
		CreatedAt:  time.Date(2025, 12, 6, 12, 0, 0, 0, time.UTC),
		SourcePath: "/data/test",
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100},
			{Path: "file2.txt", Size: 200},
		},
	}

	compressed, err := original.ToJSONCompressed()
	require.NoError(t, err)

	// Deserialize
	parsed, err := FromJSONCompressed(compressed)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, original.UploadID, parsed.UploadID)
	assert.Equal(t, original.Version, parsed.Version)
	assert.Len(t, parsed.Files, 2)
	assert.Equal(t, "file1.txt", parsed.Files[0].Path)
	assert.Equal(t, "file2.txt", parsed.Files[1].Path)
}

// TestFromJSONCompressed_Invalid tests error handling for invalid compressed data
func TestFromJSONCompressed_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "not gzip data",
			data: []byte(`{"not": "gzip"}`),
		},
		{
			name: "empty data",
			data: []byte{},
		},
		{
			name: "invalid gzip data",
			data: []byte{0x1f, 0x8b, 0x08, 0x00}, // Valid gzip header but truncated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := FromJSONCompressed(tt.data)
			assert.Error(t, err)
			assert.Nil(t, m)
		})
	}
}

// TestParseS3URL tests S3 URL parsing
func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		expectBucket string
		expectPrefix string
		expectError  bool
	}{
		{
			name:         "bucket only",
			url:          "s3://my-bucket",
			expectBucket: "my-bucket",
			expectPrefix: "",
		},
		{
			name:         "bucket with prefix",
			url:          "s3://my-bucket/my-prefix",
			expectBucket: "my-bucket",
			expectPrefix: "my-prefix",
		},
		{
			name:         "bucket with nested prefix",
			url:          "s3://my-bucket/path/to/data",
			expectBucket: "my-bucket",
			expectPrefix: "path/to/data",
		},
		{
			name:         "bucket with trailing slash",
			url:          "s3://my-bucket/",
			expectBucket: "my-bucket",
			expectPrefix: "",
		},
		{
			name:        "invalid - no s3:// prefix",
			url:         "my-bucket/prefix",
			expectError: true,
		},
		{
			name:        "invalid - http URL",
			url:         "http://my-bucket",
			expectError: true,
		},
		{
			name:        "invalid - empty",
			url:         "",
			expectError: true,
		},
		{
			name:         "invalid - just s3://",
			url:          "s3://",
			expectBucket: "",
			expectPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix, err := ParseS3URL(tt.url)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectBucket, bucket, "bucket mismatch")
			assert.Equal(t, tt.expectPrefix, prefix, "prefix mismatch")
		})
	}
}

// TestManifest_RoundTrip tests full serialization/deserialization cycle
func TestManifest_RoundTrip(t *testing.T) {
	original := &Manifest{
		Version:          ManifestVersion,
		UploadID:         "test-123",
		CreatedAt:        time.Date(2025, 12, 6, 12, 0, 0, 0, time.UTC),
		CompletedAt:      time.Date(2025, 12, 6, 12, 30, 0, 0, time.UTC),
		SourcePath:       "/data/test",
		Hostname:         "test-host",
		Bucket:           "test-bucket",
		Prefix:           "cargoship",
		Region:           "us-west-2",
		TotalFiles:       3,
		TotalBytes:       600,
		TotalChunks:      2,
		ShardCount:       4,
		CompressionType:  "zstd",
		CompressionLevel: 3,
		CompressionRatio: 0.42,
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ChunkID: 0, ShardID: 0},
			{Path: "file2.txt", Size: 200, ChunkID: 0, ShardID: 0},
			{Path: "file3.txt", Size: 300, ChunkID: 1, ShardID: 1},
		},
		Chunks: []ChunkEntry{
			{ID: 0, CompressedSize: 150, UncompressedSize: 300},
			{ID: 1, CompressedSize: 150, UncompressedSize: 300},
		},
		Shards: []ShardEntry{
			{ID: 0, ChunkCount: 1, FileCount: 2, CompressedSize: 150, UncompressedSize: 300},
			{ID: 1, ChunkCount: 1, FileCount: 1, CompressedSize: 150, UncompressedSize: 300},
		},
	}

	// Test uncompressed round-trip
	t.Run("uncompressed", func(t *testing.T) {
		data, err := original.ToJSON()
		require.NoError(t, err)

		parsed, err := FromJSON(data)
		require.NoError(t, err)

		assert.Equal(t, original.UploadID, parsed.UploadID)
		assert.Equal(t, original.TotalFiles, parsed.TotalFiles)
		assert.Equal(t, original.TotalBytes, parsed.TotalBytes)
		assert.Len(t, parsed.Files, 3)
		assert.Len(t, parsed.Chunks, 2)
		assert.Len(t, parsed.Shards, 2)
	})

	// Test compressed round-trip
	t.Run("compressed", func(t *testing.T) {
		data, err := original.ToJSONCompressed()
		require.NoError(t, err)

		parsed, err := FromJSONCompressed(data)
		require.NoError(t, err)

		assert.Equal(t, original.UploadID, parsed.UploadID)
		assert.Equal(t, original.TotalFiles, parsed.TotalFiles)
		assert.Equal(t, original.TotalBytes, parsed.TotalBytes)
		assert.Len(t, parsed.Files, 3)
		assert.Len(t, parsed.Chunks, 2)
		assert.Len(t, parsed.Shards, 2)
	})
}

// TestBuilder_FullWorkflow tests a complete manifest building workflow
func TestBuilder_FullWorkflow(t *testing.T) {
	// Create builder
	builder, err := NewBuilder("workflow-123", "/data/upload", "my-bucket", "cargoship", "us-west-2")
	require.NoError(t, err)

	// Set shard count
	builder.SetShardCount(4)

	// Set compression
	builder.SetCompression("zstd", 3, 0.45)

	// Add files
	builder.AddFile(FileEntry{Path: "doc1.txt", Size: 1024, ChunkID: 0})
	builder.AddFile(FileEntry{Path: "doc2.txt", Size: 2048, ChunkID: 0})
	builder.AddFile(FileEntry{Path: "doc3.txt", Size: 4096, ChunkID: 1})

	// Add chunks
	builder.AddChunk(ChunkEntry{ID: 0, CompressedSize: 1382, UncompressedSize: 3072, CreatedAt: time.Now()})
	builder.AddChunk(ChunkEntry{ID: 1, CompressedSize: 1843, UncompressedSize: 4096, CreatedAt: time.Now()})

	// Update file S3 keys
	builder.UpdateFileS3Keys(0, 0, "shard-0/chunk-0.tar.zst")
	builder.UpdateFileS3Keys(1, 1, "shard-1/chunk-1.tar.zst")

	// Update shard stats
	builder.UpdateShardStats(0, "chunk-0.tar.zst", 2, 3072, 1382)
	builder.UpdateShardStats(1, "chunk-1.tar.zst", 1, 4096, 1843)

	// Finalize
	manifest := builder.Finalize()

	// Verify final manifest
	assert.Equal(t, "workflow-123", manifest.UploadID)
	assert.Equal(t, int64(3), manifest.TotalFiles)
	assert.Equal(t, int64(7168), manifest.TotalBytes)
	assert.Equal(t, 2, manifest.TotalChunks)
	assert.Equal(t, 4, manifest.ShardCount)
	assert.Equal(t, "zstd", manifest.CompressionType)
	assert.Equal(t, 3, manifest.CompressionLevel)
	assert.Equal(t, 0.45, manifest.CompressionRatio)
	assert.NotZero(t, manifest.CompletedAt)

	// Verify files have S3 keys
	assert.Equal(t, 0, manifest.Files[0].ShardID)
	assert.Equal(t, "shard-0/chunk-0.tar.zst", manifest.Files[0].S3Key)
	assert.Equal(t, 0, manifest.Files[1].ShardID)
	assert.Equal(t, "shard-0/chunk-0.tar.zst", manifest.Files[1].S3Key)
	assert.Equal(t, 1, manifest.Files[2].ShardID)
	assert.Equal(t, "shard-1/chunk-1.tar.zst", manifest.Files[2].S3Key)

	// Verify shard stats
	assert.Equal(t, 1, manifest.Shards[0].ChunkCount)
	assert.Equal(t, int64(2), manifest.Shards[0].FileCount)
	assert.Equal(t, int64(3072), manifest.Shards[0].UncompressedSize)
	assert.Equal(t, int64(1382), manifest.Shards[0].CompressedSize)
}

// ============================================================================
// Performance Tests (Issue #94 Acceptance Criteria)
// ============================================================================

// TestManifestQuery_FindFile_Performance tests O(1) lookup with large dataset
func TestManifestQuery_FindFile_Performance(t *testing.T) {
	// Create manifest with 10K files (scaled down from 1M for test speed)
	numFiles := 10000
	files := make([]FileEntry, numFiles)
	for i := 0; i < numFiles; i++ {
		files[i] = FileEntry{
			Path: fmt.Sprintf("dir%d/file%d.txt", i/100, i),
			Size: int64(i * 1000),
		}
	}

	m := &Manifest{Files: files}
	query := NewManifestQuery(m)

	// Measure lookup time for files at different positions
	start := time.Now()
	iterations := 1000

	for i := 0; i < iterations; i++ {
		// Lookup files at beginning, middle, and end
		query.FindFile(files[0].Path)
		query.FindFile(files[numFiles/2].Path)
		query.FindFile(files[numFiles-1].Path)
	}

	elapsed := time.Since(start)
	avgPerLookup := elapsed / time.Duration(iterations*3)

	// With O(1) hash map index, lookups should be nearly instantaneous
	t.Logf("Average lookup time for %d files: %v (hash map O(1))", numFiles, avgPerLookup)

	// Verify O(1) performance - should be microseconds, not milliseconds
	assert.Less(t, avgPerLookup, 100*time.Microsecond, "O(1) hash map lookup should be <100μs even for 10K files")
}

// TestManifestQuery_Performance_1MFiles tests performance with 1M files (Issue #94)
func TestManifestQuery_Performance_1MFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping 1M file performance test in short mode")
	}

	// Create manifest with 1M files
	numFiles := 1000000
	files := make([]FileEntry, numFiles)
	for i := 0; i < numFiles; i++ {
		files[i] = FileEntry{
			Path:    fmt.Sprintf("dir%d/subdir%d/file%d.txt", i/10000, (i%10000)/100, i),
			Size:    int64(i * 1000),
			ChunkID: i / 1000,
			ShardID: i / 100000,
		}
	}

	m := &Manifest{Files: files}

	// Measure index build time
	start := time.Now()
	query := NewManifestQuery(m)
	buildTime := time.Since(start)

	t.Logf("Index build time for %d files: %v", numFiles, buildTime)

	// Measure lookup performance
	lookupStart := time.Now()
	iterations := 10000

	for i := 0; i < iterations; i++ {
		idx := i * (numFiles / iterations) // Distributed across dataset
		query.FindFile(files[idx].Path)
	}

	lookupTime := time.Since(lookupStart)
	avgLookup := lookupTime / time.Duration(iterations)

	t.Logf("Total lookup time: %v for %d lookups, average per lookup: %v", lookupTime, iterations, avgLookup)

	// With O(1) hash map index, lookups should be constant time regardless of dataset size
	// Target: <10μs per lookup even with 1M files (Issue #159)
	t.Logf("Hash map O(1) lookup - performance independent of file count")
	assert.Less(t, avgLookup, 10*time.Microsecond, "O(1) hash map lookup should be <10μs even for 1M files")

	// Test selective extraction (100 files from 1M) - Issue #94 acceptance criteria
	pattern := "dir0/*/file*.txt"
	extractStart := time.Now()
	results := query.ListFiles(pattern)
	extractTime := time.Since(extractStart)

	t.Logf("Selective extraction time: %v, found %d files", extractTime, len(results))
	assert.Greater(t, len(results), 0, "Should find matching files")
}

// TestManifest_MemoryUsage tests memory usage for manifest index (Issue #94)
func TestManifest_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	// Create manifest with 100K files (scaled down for test speed)
	numFiles := 100000
	files := make([]FileEntry, numFiles)
	for i := 0; i < numFiles; i++ {
		files[i] = FileEntry{
			Path:    fmt.Sprintf("path/to/file%d.txt", i),
			Size:    int64(i * 1000),
			ChunkID: i / 100,
			ShardID: i / 10000,
			S3Key:   fmt.Sprintf("shard-%d/chunk-%d.tar.zst", i/10000, i/100),
		}
	}

	m := &Manifest{
		Files:      files,
		TotalFiles: int64(numFiles),
	}

	// Create query (builds index)
	query := NewManifestQuery(m)

	// Verify query is usable
	result := query.FindFile(files[50000].Path)
	require.NotNil(t, result)
	assert.Equal(t, files[50000].Path, result.Path)

	// Memory usage is implicit (managed by Go runtime)
	// The map structure should be efficient - O(n) space for n files
	t.Logf("Created index for %d files", numFiles)
}

// TestManifestQuery_IndexVerification verifies O(1) hash map index is built and used (Issue #159)
func TestManifestQuery_IndexVerification(t *testing.T) {
	// Create manifest with known files
	files := []FileEntry{
		{Path: "file1.txt", Size: 100},
		{Path: "file2.txt", Size: 200},
		{Path: "file3.txt", Size: 300},
	}

	m := &Manifest{Files: files}
	query := NewManifestQuery(m)

	// Verify index is built
	require.NotNil(t, query.fileIndex, "File index should be initialized")
	assert.Equal(t, 3, len(query.fileIndex), "Index should contain all files")

	// Verify index contains correct mappings
	for _, file := range files {
		entry := query.fileIndex[file.Path]
		require.NotNil(t, entry, "Index should contain entry for %s", file.Path)
		assert.Equal(t, file.Path, entry.Path)
		assert.Equal(t, file.Size, entry.Size)
	}

	// Verify FindFile uses the index
	result := query.FindFile("file2.txt")
	require.NotNil(t, result)
	assert.Equal(t, "file2.txt", result.Path)
	assert.Equal(t, int64(200), result.Size)

	// Verify non-existent file returns nil
	result = query.FindFile("nonexistent.txt")
	assert.Nil(t, result, "FindFile should return nil for non-existent files")
}

// BenchmarkManifestQuery_FindFile benchmarks file lookup
func BenchmarkManifestQuery_FindFile(b *testing.B) {
	// Create manifest with 10K files
	numFiles := 10000
	files := make([]FileEntry, numFiles)
	for i := 0; i < numFiles; i++ {
		files[i] = FileEntry{
			Path: fmt.Sprintf("dir%d/file%d.txt", i/100, i),
			Size: int64(i * 1000),
		}
	}

	m := &Manifest{Files: files}
	query := NewManifestQuery(m)
	targetPath := files[numFiles/2].Path

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query.FindFile(targetPath)
	}
}

// BenchmarkManifest_Build benchmarks manifest building
func BenchmarkManifest_Build(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder, _ := NewBuilder("test-123", "/data", "bucket", "prefix", "us-west-2")
		builder.SetShardCount(8)

		for j := 0; j < 1000; j++ {
			builder.AddFile(FileEntry{
				Path: fmt.Sprintf("file%d.txt", j),
				Size: int64(j * 1000),
			})
		}

		builder.Finalize()
	}
}

// BenchmarkManifest_Serialization benchmarks JSON serialization
func BenchmarkManifest_Serialization(b *testing.B) {
	// Create manifest with 1000 files
	files := make([]FileEntry, 1000)
	for i := 0; i < 1000; i++ {
		files[i] = FileEntry{
			Path: fmt.Sprintf("file%d.txt", i),
			Size: int64(i * 1000),
		}
	}

	m := &Manifest{
		Version:  ManifestVersion,
		UploadID: "bench-test",
		Files:    files,
	}

	b.Run("ToJSON", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = m.ToJSON()
		}
	})

	b.Run("ToJSONCompressed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = m.ToJSONCompressed()
		}
	})
}

// ============================================================================
// Thread-Safety Tests (Issue #88)
// ============================================================================

// TestBuilder_ConcurrentAccess tests thread-safe concurrent access to builder
func TestBuilder_ConcurrentAccess(t *testing.T) {
	builder, err := NewBuilder("concurrent-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Initialize shards first (non-concurrent)
	builder.SetShardCount(4)

	// Number of concurrent operations per type
	numFiles := 100
	numChunks := 10
	numUpdates := 50

	// WaitGroup to synchronize all goroutines
	var wg sync.WaitGroup

	// Concurrent file additions
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numFiles; i++ {
			builder.AddFile(FileEntry{
				Path:    fmt.Sprintf("file-%d.txt", i),
				Size:    int64(i * 1024),
				ChunkID: i % numChunks,
			})
		}
	}()

	// Concurrent chunk additions
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numChunks; i++ {
			builder.AddChunk(ChunkEntry{
				ID:               i,
				CompressedSize:   1024,
				UncompressedSize: 2048,
			})
		}
	}()

	// Concurrent shard stats updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numUpdates; i++ {
			shardID := i % 4
			builder.UpdateShardStats(shardID, fmt.Sprintf("chunk-%d.tar.zst", i), 10, 2048, 1024)
		}
	}()

	// Concurrent compression updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numUpdates; i++ {
			builder.SetCompression("zstd", 3, 0.5+float64(i)*0.001)
		}
	}()

	// Concurrent reads (should not interfere with writes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numUpdates; i++ {
			m := builder.Build()
			_ = m.UploadID // Use the manifest
		}
	}()

	// Wait for all operations to complete
	wg.Wait()

	// Verify final state
	manifest := builder.Build()
	assert.Equal(t, int64(numFiles), manifest.TotalFiles)
	assert.Equal(t, numChunks, manifest.TotalChunks)
	assert.Equal(t, 4, manifest.ShardCount)
	assert.Len(t, manifest.Files, numFiles)
	assert.Len(t, manifest.Chunks, numChunks)
	assert.Len(t, manifest.Shards, 4)

	// Verify shard stats were accumulated correctly
	totalChunkCount := 0
	for i := 0; i < 4; i++ {
		totalChunkCount += manifest.Shards[i].ChunkCount
	}
	assert.Equal(t, numUpdates, totalChunkCount)
}

// TestBuilder_ConcurrentFileS3KeyUpdates tests concurrent UpdateFileS3Keys calls
func TestBuilder_ConcurrentFileS3KeyUpdates(t *testing.T) {
	builder, err := NewBuilder("update-keys-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add files with different chunk IDs
	numChunks := 10
	filesPerChunk := 10
	for chunk := 0; chunk < numChunks; chunk++ {
		for i := 0; i < filesPerChunk; i++ {
			builder.AddFile(FileEntry{
				Path:    fmt.Sprintf("chunk-%d-file-%d.txt", chunk, i),
				Size:    1024,
				ChunkID: chunk,
			})
		}
	}

	// Concurrently update S3 keys for different chunks
	var wg sync.WaitGroup
	for chunk := 0; chunk < numChunks; chunk++ {
		wg.Add(1)
		chunkID := chunk
		go func() {
			defer wg.Done()
			s3Key := fmt.Sprintf("shard-%d/chunk-%d.tar.zst", chunkID%4, chunkID)
			builder.UpdateFileS3Keys(chunkID, chunkID%4, s3Key)
		}()
	}

	wg.Wait()

	// Verify all files have been updated correctly
	manifest := builder.Build()
	assert.Len(t, manifest.Files, numChunks*filesPerChunk)

	for _, file := range manifest.Files {
		// S3Key should be set based on ChunkID
		expectedKey := fmt.Sprintf("shard-%d/chunk-%d.tar.zst", file.ChunkID%4, file.ChunkID)
		assert.Equal(t, expectedKey, file.S3Key, "File %s should have correct S3 key", file.Path)
		assert.Equal(t, file.ChunkID%4, file.ShardID, "File %s should have correct shard ID", file.Path)
	}
}

// TestBuilder_ConcurrentFinalize tests that Finalize is thread-safe
func TestBuilder_ConcurrentFinalize(t *testing.T) {
	builder, err := NewBuilder("finalize-123", "/data", "bucket", "prefix", "us-west-2")
	require.NoError(t, err)

	// Add some data
	for i := 0; i < 10; i++ {
		builder.AddFile(FileEntry{Path: fmt.Sprintf("file-%d.txt", i), Size: 1024})
	}

	// Multiple goroutines try to finalize concurrently
	// Only one should succeed in setting CompletedAt
	var wg sync.WaitGroup
	numGoroutines := 10
	manifests := make([]*Manifest, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			manifests[idx] = builder.Finalize()
		}()
	}

	wg.Wait()

	// All manifests should have CompletedAt set
	for i, m := range manifests {
		assert.NotZero(t, m.CompletedAt, "Manifest %d should have CompletedAt set", i)
	}

	// All manifests should be the same instance
	for i := 1; i < numGoroutines; i++ {
		assert.Equal(t, manifests[0].UploadID, manifests[i].UploadID)
	}
}

// TestBuilder_SetSyncInfo tests setting sync information
func TestBuilder_SetSyncInfo(t *testing.T) {
	builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)

	// Test setting sync info
	builder.SetSyncInfo("incremental", "previous-upload-456")

	manifest := builder.Build()
	assert.Equal(t, "incremental", manifest.SyncType)
	assert.Equal(t, "previous-upload-456", manifest.PreviousManifestID)
}

// TestBuilder_SetSyncInfo_Empty tests setting empty sync info
func TestBuilder_SetSyncInfo_Empty(t *testing.T) {
	builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)

	// Test setting empty sync info
	builder.SetSyncInfo("", "")

	manifest := builder.Build()
	assert.Empty(t, manifest.SyncType)
	assert.Empty(t, manifest.PreviousManifestID)
}

// TestBuilder_SetSyncInfo_Concurrent tests thread-safety
func TestBuilder_SetSyncInfo_Concurrent(t *testing.T) {
	builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			syncType := fmt.Sprintf("sync-type-%d", idx)
			prevID := fmt.Sprintf("prev-id-%d", idx)
			builder.SetSyncInfo(syncType, prevID)
		}(i)
	}

	wg.Wait()

	// Verify manifest was updated (exact values unpredictable due to concurrency)
	manifest := builder.Build()
	assert.NotEmpty(t, manifest.SyncType)
	assert.NotEmpty(t, manifest.PreviousManifestID)
}

// TestBuilder_SetEncryption tests setting encryption metadata
func TestBuilder_SetEncryption(t *testing.T) {
	tests := []struct {
		name              string
		kmsKeyID          string
		manifestEncrypted bool
		expectNil         bool
		expectEnabled     bool
	}{
		{
			name:              "with KMS key and manifest encryption",
			kmsKeyID:          "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789abc",
			manifestEncrypted: true,
			expectNil:         false,
			expectEnabled:     true,
		},
		{
			name:              "with KMS key only",
			kmsKeyID:          "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789abc",
			manifestEncrypted: false,
			expectNil:         false,
			expectEnabled:     true,
		},
		{
			name:              "manifest encrypted without KMS key",
			kmsKeyID:          "",
			manifestEncrypted: true,
			expectNil:         false,
			expectEnabled:     false,
		},
		{
			name:              "no encryption",
			kmsKeyID:          "",
			manifestEncrypted: false,
			expectNil:         true,
			expectEnabled:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)
			builder.SetEncryption(tt.kmsKeyID, tt.manifestEncrypted)

			manifest := builder.Build()

			if tt.expectNil {
				assert.Nil(t, manifest.Encryption)
			} else {
				require.NotNil(t, manifest.Encryption)
				assert.Equal(t, tt.expectEnabled, manifest.Encryption.Enabled)
				assert.Equal(t, tt.kmsKeyID, manifest.Encryption.DataKMSKeyID)
				assert.Equal(t, tt.manifestEncrypted, manifest.Encryption.ManifestEncrypted)
			}
		})
	}
}

// TestBuilder_SetEncryption_Concurrent tests thread-safety
func TestBuilder_SetEncryption_Concurrent(t *testing.T) {
	builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			kmsKey := fmt.Sprintf("kms-key-%d", idx)
			builder.SetEncryption(kmsKey, idx%2 == 0)
		}(i)
	}

	wg.Wait()

	// Verify encryption metadata was set
	manifest := builder.Build()
	assert.NotNil(t, manifest.Encryption)
}

// TestBuilder_SetEncryption_ClearEncryption tests clearing encryption
func TestBuilder_SetEncryption_ClearEncryption(t *testing.T) {
	builder, err := NewBuilder("test-upload-123", "/test/source", "test-bucket", "test-prefix", "us-west-2")
	require.NoError(t, err)

	// Set encryption first
	builder.SetEncryption("test-kms-key", true)
	manifest := builder.Build()
	require.NotNil(t, manifest.Encryption)

	// Clear encryption
	builder.SetEncryption("", false)
	manifest = builder.Build()
	assert.Nil(t, manifest.Encryption)
}
