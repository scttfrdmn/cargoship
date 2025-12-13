package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock S3 client for testing
type mockS3Client struct {
	chunks map[string][]byte // S3 key -> compressed tar data
}

func (m *mockS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := m.chunks[*input.Key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", *input.Key)
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func TestNewExtractor(t *testing.T) {
	manifest := &Manifest{
		UploadID: "test-123",
		Files:    []FileEntry{{Path: "test.txt"}},
	}

	client := &mockS3Client{}
	extractor := NewExtractor(manifest, client)

	assert.NotNil(t, extractor)
	assert.Equal(t, manifest, extractor.manifest)
	assert.NotNil(t, extractor.query)
}

func TestExtractRequest_Validation(t *testing.T) {
	manifest := createTestManifest()
	client := &mockS3Client{}
	extractor := NewExtractor(manifest, client)

	tests := []struct {
		name    string
		req     *ExtractRequest
		wantErr bool
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
		{
			name: "empty file paths",
			req: &ExtractRequest{
				OutputDir: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "empty output dir",
			req: &ExtractRequest{
				FilePaths: []string{"test.txt"},
			},
			wantErr: true,
		},
		{
			name: "valid request",
			req: &ExtractRequest{
				FilePaths: []string{"test.txt"},
				OutputDir: "/tmp/test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := extractor.validateRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResolveFiles(t *testing.T) {
	manifest := createTestManifest()
	extractor := NewExtractor(manifest, &mockS3Client{})

	tests := []struct {
		name     string
		patterns []string
		wantLen  int
	}{
		{
			name:     "exact match",
			patterns: []string{"dir1/file1.txt"},
			wantLen:  1,
		},
		{
			name:     "glob pattern - all txt files",
			patterns: []string{"*.txt"},
			wantLen:  3,
		},
		{
			name:     "glob pattern - all log files",
			patterns: []string{"*.log"},
			wantLen:  1,
		},
		{
			name:     "multiple patterns",
			patterns: []string{"*.txt", "*.log"},
			wantLen:  4,
		},
		{
			name:     "no matches",
			patterns: []string{"*.xyz"},
			wantLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := extractor.resolveFiles(tt.patterns)
			require.NoError(t, err)
			assert.Len(t, files, tt.wantLen)
		})
	}
}

func TestGroupFilesByShard(t *testing.T) {
	manifest := createTestManifest()
	extractor := NewExtractor(manifest, &mockS3Client{})

	files := manifest.Files
	shardMap := extractor.groupFilesByShard(files)

	// Check that files are correctly grouped
	assert.Len(t, shardMap, 2) // 2 shards in test manifest
	assert.Len(t, shardMap[0], 3)
	assert.Len(t, shardMap[1], 1)
}

func TestEstimateExtractCost(t *testing.T) {
	manifest := createTestManifest()
	extractor := NewExtractor(manifest, &mockS3Client{})

	tests := []struct {
		name       string
		patterns   []string
		wantShards int
		wantChunks int
	}{
		{
			name:       "single file",
			patterns:   []string{"dir1/file1.txt"},
			wantShards: 1,
			wantChunks: 1,
		},
		{
			name:       "all txt files",
			patterns:   []string{"*.txt"},
			wantShards: 2,
			wantChunks: 2,
		},
		{
			name:       "no matches",
			patterns:   []string{"*.xyz"},
			wantShards: 0,
			wantChunks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shards, chunks, totalBytes, err := extractor.EstimateExtractCost(tt.patterns)
			require.NoError(t, err)
			assert.Equal(t, tt.wantShards, shards)
			assert.Equal(t, tt.wantChunks, chunks)
			assert.GreaterOrEqual(t, totalBytes, int64(0))
		})
	}
}

func TestListExtractableFiles(t *testing.T) {
	manifest := createTestManifest()
	extractor := NewExtractor(manifest, &mockS3Client{})

	files, err := extractor.ListExtractableFiles([]string{"*.txt"})
	require.NoError(t, err)
	assert.Len(t, files, 3)

	// Verify file details
	for _, file := range files {
		assert.True(t, filepath.Ext(file.Path) == ".txt")
		assert.Greater(t, file.Size, int64(0))
	}
}

func TestExtract_Integration(t *testing.T) {
	// Create test manifest with mock data
	manifest := createTestManifest()

	// Create mock chunks
	mockChunks := createMockChunks(t, manifest)
	client := &mockS3Client{chunks: mockChunks}

	extractor := NewExtractor(manifest, client)

	// Create temporary output directory
	tmpDir := t.TempDir()

	req := &ExtractRequest{
		FilePaths:         []string{"*.txt"},
		OutputDir:         tmpDir,
		PreserveStructure: true,
		Overwrite:         true,
		Concurrency:       2,
	}

	result, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 3, result.FilesExtracted)
	assert.Greater(t, result.BytesExtracted, int64(0))
	assert.Greater(t, result.ShardsDownloaded, 0)

	// Verify files were extracted
	for _, file := range manifest.Files {
		if filepath.Ext(file.Path) == ".txt" {
			extractedPath := filepath.Join(tmpDir, file.Path)
			info, err := os.Stat(extractedPath)
			require.NoError(t, err, "file %s should exist", file.Path)
			assert.Greater(t, info.Size(), int64(0))
		}
	}
}

func TestExtract_ProgressCallbacks(t *testing.T) {
	manifest := createTestManifest()
	mockChunks := createMockChunks(t, manifest)
	client := &mockS3Client{chunks: mockChunks}
	extractor := NewExtractor(manifest, client)

	tmpDir := t.TempDir()

	var progressCalls int
	var filesCalled int

	req := &ExtractRequest{
		FilePaths:         []string{"*.txt"},
		OutputDir:         tmpDir,
		PreserveStructure: true,
		Concurrency:       1,
		OnProgress: func(extracted, total int64) {
			progressCalls++
			assert.LessOrEqual(t, extracted, total)
		},
		OnFileExtracted: func(path string, size int64) {
			filesCalled++
			assert.Greater(t, size, int64(0))
		},
	}

	result, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, result.FilesExtracted, filesCalled)
	assert.Greater(t, progressCalls, 0)
}

func TestExtract_FlattenStructure(t *testing.T) {
	manifest := createTestManifest()
	mockChunks := createMockChunks(t, manifest)
	client := &mockS3Client{chunks: mockChunks}
	extractor := NewExtractor(manifest, client)

	tmpDir := t.TempDir()

	req := &ExtractRequest{
		FilePaths:         []string{"dir1/file1.txt", "dir2/file2.txt"},
		OutputDir:         tmpDir,
		PreserveStructure: false, // Flatten
		Overwrite:         true,
		Concurrency:       1,
	}

	result, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, result.FilesExtracted)

	// Verify files are in root of output dir (flattened)
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	// All files should be in tmpDir root
	for _, f := range files {
		assert.False(t, f.IsDir(), "flattened output should not contain directories")
	}
}

func TestExtract_OverwriteHandling(t *testing.T) {
	manifest := createTestManifest()
	mockChunks := createMockChunks(t, manifest)
	client := &mockS3Client{chunks: mockChunks}
	extractor := NewExtractor(manifest, client)

	tmpDir := t.TempDir()

	// First extraction
	req := &ExtractRequest{
		FilePaths:         []string{"dir1/file1.txt"},
		OutputDir:         tmpDir,
		PreserveStructure: true,
		Overwrite:         false,
		Concurrency:       1,
	}

	result1, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, result1.FilesExtracted)

	// Second extraction with overwrite=false should skip
	result2, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.FilesExtracted) // File exists, should skip

	// Third extraction with overwrite=true should extract
	req.Overwrite = true
	result3, err := extractor.Extract(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, result3.FilesExtracted)
}

// Helper functions

func createTestManifest() *Manifest {
	return &Manifest{
		Version:          "1.0",
		UploadID:         "test-upload-123",
		CreatedAt:        time.Now(),
		CompletedAt:      time.Now(),
		Bucket:           "test-bucket",
		Prefix:           "test-prefix",
		Region:           "us-west-2",
		CompressionType:  "zstd",
		CompressionLevel: 3,
		Files: []FileEntry{
			{Path: "dir1/file1.txt", Size: 100, ChunkID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst"},
			{Path: "dir2/file2.txt", Size: 200, ChunkID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst"},
			{Path: "dir1/file3.txt", Size: 150, ChunkID: 1, ShardID: 1, S3Key: "shard-1/chunk-1.tar.zst"},
			{Path: "dir3/app.log", Size: 300, ChunkID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst"},
		},
		Chunks: []ChunkEntry{
			{
				ID:               0,
				ShardID:          0,
				S3Key:            "shard-0/chunk-0.tar.zst",
				FileCount:        3,
				FilePaths:        []string{"dir1/file1.txt", "dir2/file2.txt", "dir3/app.log"},
				UncompressedSize: 600,
			},
			{
				ID:               1,
				ShardID:          1,
				S3Key:            "shard-1/chunk-1.tar.zst",
				FileCount:        1,
				FilePaths:        []string{"dir1/file3.txt"},
				UncompressedSize: 150,
			},
		},
		Shards: []ShardEntry{
			{ID: 0, ChunkCount: 1},
			{ID: 1, ChunkCount: 1},
		},
	}
}

func createMockChunks(t *testing.T, manifest *Manifest) map[string][]byte {
	chunks := make(map[string][]byte)

	// Create mock tar.zst archives for each chunk
	for _, chunk := range manifest.Chunks {
		// Get files in this chunk
		var files []FileEntry
		for _, file := range manifest.Files {
			if file.ChunkID == chunk.ID {
				files = append(files, file)
			}
		}

		// Create tar archive
		tarBuf := new(bytes.Buffer)
		tarWriter := tar.NewWriter(tarBuf)

		for _, file := range files {
			header := &tar.Header{
				Name: file.Path,
				Size: file.Size,
				Mode: 0644,
			}

			err := tarWriter.WriteHeader(header)
			require.NoError(t, err)

			// Write dummy file content
			content := bytes.Repeat([]byte("test data\n"), int(file.Size/10)+1)
			_, err = tarWriter.Write(content[:file.Size])
			require.NoError(t, err)
		}

		err := tarWriter.Close()
		require.NoError(t, err)

		// Compress with zstd
		compressedBuf := new(bytes.Buffer)
		encoder, err := zstd.NewWriter(compressedBuf)
		require.NoError(t, err)

		_, err = encoder.Write(tarBuf.Bytes())
		require.NoError(t, err)

		err = encoder.Close()
		require.NoError(t, err)

		chunks[chunk.S3Key] = compressedBuf.Bytes()
	}

	return chunks
}
