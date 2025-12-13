package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToJSONAuto_SmallManifest tests automatic compression with small manifest (Issue #92)
func TestToJSONAuto_SmallManifest(t *testing.T) {
	// Create a small manifest (< 10MB)
	m := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-upload-123",
		CreatedAt:  time.Now(),
		Bucket:     "test-bucket",
		Region:     "us-west-2",
		Prefix:     "test",
		ShardCount: 1,
		TotalFiles: 10,
		TotalBytes: 1000,
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ChunkID: 0, ShardID: 0},
			{Path: "file2.txt", Size: 100, ChunkID: 0, ShardID: 0},
		},
	}

	data, compressed, err := m.ToJSONAuto()
	require.NoError(t, err)
	assert.False(t, compressed, "Small manifest should not be compressed")
	assert.NotEmpty(t, data)

	// Verify data is valid JSON (not gzip)
	assert.NotEqual(t, GzipMagicNumber1, data[0], "Should not have gzip magic number")
}

// TestToJSONAuto_LargeManifest tests automatic compression with large manifest (Issue #92)
func TestToJSONAuto_LargeManifest(t *testing.T) {
	// Create a large manifest (> 10MB)
	m := &Manifest{
		Version:     ManifestVersion,
		UploadID:    "test-upload-large",
		CreatedAt:   time.Now(),
		Bucket:      "test-bucket",
		Region:      "us-west-2",
		Prefix:      "test",
		ShardCount:  10,
		TotalFiles:  100000, // 100K files
		TotalBytes:  10000000000,
		TotalChunks: 1000,
		Files:       make([]FileEntry, 100000),
		Chunks:      make([]ChunkEntry, 1000),
		Shards:      make([]ShardEntry, 10),
	}

	// Populate with test data to exceed 10MB
	for i := 0; i < 100000; i++ {
		m.Files[i] = FileEntry{
			Path:     "test/very/long/path/to/simulate/realistic/manifest/size/file" + string(rune(i%26+'a')) + ".txt",
			Size:     100000,
			ChunkID:  i / 100,
			ShardID:  i / 10000,
			Checksum: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}
	}

	for i := 0; i < 1000; i++ {
		m.Chunks[i] = ChunkEntry{
			ID:               i,
			ShardID:          i / 100,
			S3Key:            "shard-" + string(rune(i/100+'0')) + "/chunk-" + string(rune(i%100+'0')) + ".tar.zst",
			FileCount:        100,
			UncompressedSize: 10000000,
			CompressedSize:   5000000,
			Checksum:         "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		}
	}

	for i := 0; i < 10; i++ {
		m.Shards[i] = ShardEntry{
			ID:               i,
			Prefix:           "shard-" + string(rune(i+'0')),
			ChunkCount:       100,
			FileCount:        10000,
			UncompressedSize: 1000000000,
			CompressedSize:   500000000,
		}
	}

	data, compressed, err := m.ToJSONAuto()
	require.NoError(t, err)
	assert.True(t, compressed, "Large manifest should be compressed")
	assert.NotEmpty(t, data)

	// Verify data has gzip magic number
	require.GreaterOrEqual(t, len(data), 2, "Data should have at least 2 bytes")
	assert.Equal(t, GzipMagicNumber1, data[0], "Should have gzip magic number byte 1")
	assert.Equal(t, GzipMagicNumber2, data[1], "Should have gzip magic number byte 2")

	// Verify compression ratio is good (< 30% of original)
	uncompressed, compressedSize, ratio, err := m.EstimateCompressedSize()
	require.NoError(t, err)
	t.Logf("Uncompressed: %d bytes, Compressed: %d bytes, Ratio: %.2f%%", uncompressed, compressedSize, ratio*100)
	assert.Less(t, ratio, 0.3, "Compression ratio should be less than 30%% for large manifests")
}

// TestFromJSONAuto_Uncompressed tests automatic decompression with uncompressed data (Issue #92)
func TestFromJSONAuto_Uncompressed(t *testing.T) {
	// Create and serialize a manifest without compression
	original := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-upload-456",
		CreatedAt:  time.Now(),
		Bucket:     "test-bucket",
		Region:     "us-west-2",
		Prefix:     "test",
		ShardCount: 1,
		TotalFiles: 5,
		TotalBytes: 500,
	}

	data, err := original.ToJSON()
	require.NoError(t, err)

	// Deserialize using automatic detection
	deserialized, err := FromJSONAuto(data)
	require.NoError(t, err)
	assert.Equal(t, original.UploadID, deserialized.UploadID)
	assert.Equal(t, original.Bucket, deserialized.Bucket)
	assert.Equal(t, original.TotalFiles, deserialized.TotalFiles)
}

// TestFromJSONAuto_Compressed tests automatic decompression with compressed data (Issue #92)
func TestFromJSONAuto_Compressed(t *testing.T) {
	// Create and serialize a manifest with compression
	original := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-upload-789",
		CreatedAt:  time.Now(),
		Bucket:     "test-bucket",
		Region:     "us-west-2",
		Prefix:     "test",
		ShardCount: 1,
		TotalFiles: 5,
		TotalBytes: 500,
	}

	data, err := original.ToJSONCompressed()
	require.NoError(t, err)

	// Verify data is compressed
	require.GreaterOrEqual(t, len(data), 2, "Data should have at least 2 bytes")
	assert.Equal(t, GzipMagicNumber1, data[0], "Should have gzip magic number")

	// Deserialize using automatic detection
	deserialized, err := FromJSONAuto(data)
	require.NoError(t, err)
	assert.Equal(t, original.UploadID, deserialized.UploadID)
	assert.Equal(t, original.Bucket, deserialized.Bucket)
	assert.Equal(t, original.TotalFiles, deserialized.TotalFiles)
}

// TestFromJSONAuto_Empty tests automatic decompression with empty data (Issue #92)
func TestFromJSONAuto_Empty(t *testing.T) {
	_, err := FromJSONAuto([]byte{})
	assert.Error(t, err, "Should error on empty data")
	assert.Contains(t, err.Error(), "empty data")
}

// TestEstimateCompressedSize tests compression size estimation (Issue #92)
func TestEstimateCompressedSize(t *testing.T) {
	// Create a manifest
	m := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "test-estimate",
		CreatedAt:  time.Now(),
		Bucket:     "test-bucket",
		Region:     "us-west-2",
		Prefix:     "test",
		ShardCount: 1,
		TotalFiles: 100,
		TotalBytes: 10000,
		Files:      make([]FileEntry, 100),
	}

	// Populate with test data
	for i := 0; i < 100; i++ {
		m.Files[i] = FileEntry{
			Path:    "file" + string(rune(i+'0')) + ".txt",
			Size:    100,
			ChunkID: 0,
			ShardID: 0,
		}
	}

	uncompressed, compressed, ratio, err := m.EstimateCompressedSize()
	require.NoError(t, err)

	assert.Greater(t, uncompressed, int64(0), "Uncompressed size should be positive")
	assert.Greater(t, compressed, int64(0), "Compressed size should be positive")
	assert.Less(t, compressed, uncompressed, "Compressed size should be less than uncompressed")
	assert.Greater(t, ratio, 0.0, "Ratio should be positive")
	assert.Less(t, ratio, 1.0, "Ratio should be less than 1.0")

	t.Logf("Compression: %d → %d bytes (%.1f%% of original)", uncompressed, compressed, ratio*100)
}

// TestShouldCompress tests compression decision logic (Issue #92)
func TestShouldCompress(t *testing.T) {
	tests := []struct {
		name           string
		fileCount      int
		expectCompress bool
	}{
		{
			name:           "small manifest (10 files)",
			fileCount:      10,
			expectCompress: false,
		},
		{
			name:           "medium manifest (1000 files)",
			fileCount:      1000,
			expectCompress: false,
		},
		{
			name:           "large manifest (100K files)",
			fileCount:      100000,
			expectCompress: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				Version:    ManifestVersion,
				UploadID:   "test-should-compress",
				CreatedAt:  time.Now(),
				Bucket:     "test-bucket",
				Region:     "us-west-2",
				Prefix:     "test",
				ShardCount: 10,
				TotalFiles: int64(tt.fileCount),
				TotalBytes: int64(tt.fileCount * 100),
				Files:      make([]FileEntry, tt.fileCount),
			}

			// Populate with test data
			for i := 0; i < tt.fileCount; i++ {
				m.Files[i] = FileEntry{
					Path:     "test/path/to/file" + string(rune(i%26+'a')) + ".txt",
					Size:     100,
					ChunkID:  i / 100,
					ShardID:  i / 10000,
					Checksum: "0123456789abcdef0123456789abcdef",
				}
			}

			shouldCompress, err := m.ShouldCompress()
			require.NoError(t, err)
			assert.Equal(t, tt.expectCompress, shouldCompress,
				"Compression decision mismatch for %d files", tt.fileCount)

			// Verify ToJSONAuto matches ShouldCompress
			_, compressed, err := m.ToJSONAuto()
			require.NoError(t, err)
			assert.Equal(t, tt.expectCompress, compressed,
				"ToJSONAuto should match ShouldCompress")
		})
	}
}

// TestCompressionRoundTrip tests full compression/decompression cycle (Issue #92)
func TestCompressionRoundTrip(t *testing.T) {
	// Create a manifest that will be compressed
	original := &Manifest{
		Version:     ManifestVersion,
		UploadID:    "test-roundtrip",
		CreatedAt:   time.Now(),
		CompletedAt: time.Now().Add(1 * time.Hour),
		Bucket:      "test-bucket",
		Region:      "us-west-2",
		Prefix:      "test",
		SourcePath:  "/test/path",
		Hostname:    "test-host",
		ShardCount:  10,
		TotalFiles:  50000,
		TotalBytes:  5000000000,
		TotalChunks: 500,
		Files:       make([]FileEntry, 50000),
		Chunks:      make([]ChunkEntry, 500),
		Shards:      make([]ShardEntry, 10),
	}

	// Populate with test data to ensure compression
	for i := 0; i < 50000; i++ {
		original.Files[i] = FileEntry{
			Path:     "test/very/long/path/structure/for/compression/file" + string(rune(i%26+'a')) + ".txt",
			Size:     100000,
			ChunkID:  i / 100,
			ShardID:  i / 5000,
			Checksum: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		}
	}

	// Serialize with automatic compression
	data, compressed, err := original.ToJSONAuto()
	require.NoError(t, err)
	assert.True(t, compressed, "Should be compressed")

	// Deserialize with automatic decompression
	deserialized, err := FromJSONAuto(data)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.Version, deserialized.Version)
	assert.Equal(t, original.UploadID, deserialized.UploadID)
	assert.Equal(t, original.Bucket, deserialized.Bucket)
	assert.Equal(t, original.Region, deserialized.Region)
	assert.Equal(t, original.Prefix, deserialized.Prefix)
	assert.Equal(t, original.ShardCount, deserialized.ShardCount)
	assert.Equal(t, original.TotalFiles, deserialized.TotalFiles)
	assert.Equal(t, original.TotalBytes, deserialized.TotalBytes)
	assert.Equal(t, original.TotalChunks, deserialized.TotalChunks)
	assert.Equal(t, len(original.Files), len(deserialized.Files))
	assert.Equal(t, len(original.Chunks), len(deserialized.Chunks))
	assert.Equal(t, len(original.Shards), len(deserialized.Shards))

	// Verify first and last file entries
	assert.Equal(t, original.Files[0].Path, deserialized.Files[0].Path)
	assert.Equal(t, original.Files[len(original.Files)-1].Path, deserialized.Files[len(deserialized.Files)-1].Path)
}

// TestCompressionThreshold tests the 10MB threshold constant (Issue #92)
func TestCompressionThreshold(t *testing.T) {
	assert.Equal(t, 10*1024*1024, CompressionThreshold,
		"Compression threshold should be 10MB")
}
