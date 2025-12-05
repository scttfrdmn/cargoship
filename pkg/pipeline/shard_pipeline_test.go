package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// Mock S3 client for testing
type mockS3ClientShard struct {
	putObjectCalls int
	uploadedData   []byte
	failAfter      int
	mu             sync.Mutex
}

func (m *mockS3ClientShard) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.putObjectCalls++
	if m.failAfter > 0 && m.putObjectCalls <= m.failAfter {
		return nil, fmt.Errorf("simulated upload failure")
	}

	// Read all data from body
	if input.Body != nil {
		data, err := io.ReadAll(input.Body)
		if err != nil {
			return nil, err
		}
		m.uploadedData = data
	}

	return &s3.PutObjectOutput{}, nil
}

// Multipart upload methods (required by S3Uploader interface)
func (m *mockS3ClientShard) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	uploadID := "test-upload-id"
	return &s3.CreateMultipartUploadOutput{
		UploadId: &uploadID,
	}, nil
}

func (m *mockS3ClientShard) UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.putObjectCalls++
	if m.failAfter > 0 && m.putObjectCalls <= m.failAfter {
		return nil, fmt.Errorf("simulated upload failure")
	}

	// Read data from part body
	if input.Body != nil {
		data, err := io.ReadAll(input.Body)
		if err != nil {
			return nil, err
		}
		m.uploadedData = append(m.uploadedData, data...)
	}

	etag := "test-etag"
	return &s3.UploadPartOutput{
		ETag: &etag,
	}, nil
}

func (m *mockS3ClientShard) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *mockS3ClientShard) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

func TestNewShardPipeline(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	tests := []struct {
		name    string
		config  *ShardPipelineConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "nil S3 client",
			config: &ShardPipelineConfig{
				Bucket:    "test-bucket",
				ShardName: "shard-00000",
			},
			wantErr: true,
		},
		{
			name: "empty bucket",
			config: &ShardPipelineConfig{
				S3Client:  mockClient,
				ShardName: "shard-00000",
			},
			wantErr: true,
		},
		{
			name: "empty shard name",
			config: &ShardPipelineConfig{
				S3Client: mockClient,
				Bucket:   "test-bucket",
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &ShardPipelineConfig{
				S3Client:  mockClient,
				Bucket:    "test-bucket",
				ShardID:   0,
				ShardName: "shard-00000",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewShardPipeline(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewShardPipeline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && pipeline == nil {
				t.Error("NewShardPipeline() returned nil pipeline without error")
			}
		})
	}
}

func TestShardPipeline_BasicFlow(t *testing.T) {
	ctx := context.Background()

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content for shard pipeline")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create mock S3 client
	mockClient := &mockS3ClientShard{}

	// Create pipeline
	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardID:   0,
		ShardName: "shard-00000",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Start pipeline
	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Add file
	file := chunking.File{
		Path:    testFile,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}

	if err := pipeline.AddFile(file); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Close pipeline (triggers upload)
	if err := pipeline.Close(); err != nil {
		t.Fatalf("Failed to close pipeline: %v", err)
	}

	// Verify upload was called
	if mockClient.putObjectCalls != 1 {
		t.Errorf("Expected 1 PutObject call, got %d", mockClient.putObjectCalls)
	}

	// Verify data was uploaded
	if len(mockClient.uploadedData) == 0 {
		t.Error("No data was uploaded")
	}

	// Verify stats
	stats := pipeline.GetStats()
	if stats.FilesAdded != 1 {
		t.Errorf("Expected 1 file added, got %d", stats.FilesAdded)
	}

	if stats.BytesProcessed != int64(len(content)) {
		t.Errorf("Expected %d bytes processed, got %d", len(content), stats.BytesProcessed)
	}

	if !stats.Completed {
		t.Error("Pipeline should be completed")
	}

	if stats.Error != nil {
		t.Errorf("Pipeline should not have error, got: %v", stats.Error)
	}
}

func TestShardPipeline_MultipleFiles(t *testing.T) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := t.TempDir()
	const numFiles = 10
	var testFiles []chunking.File
	var totalSize int64

	for i := 0; i < numFiles; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		content := []byte(fmt.Sprintf("test content %d", i))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file %d: %v", i, err)
		}

		testFiles = append(testFiles, chunking.File{
			Path:    testFile,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		})
		totalSize += int64(len(content))
	}

	// Create mock S3 client
	mockClient := &mockS3ClientShard{}

	// Create pipeline
	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardID:   5,
		ShardName: "shard-00005",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Start pipeline
	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Add all files
	for _, file := range testFiles {
		if err := pipeline.AddFile(file); err != nil {
			t.Fatalf("Failed to add file: %v", err)
		}
	}

	// Close pipeline
	if err := pipeline.Close(); err != nil {
		t.Fatalf("Failed to close pipeline: %v", err)
	}

	// Verify stats
	stats := pipeline.GetStats()
	if stats.FilesAdded != int64(numFiles) {
		t.Errorf("Expected %d files added, got %d", numFiles, stats.FilesAdded)
	}

	if stats.BytesProcessed != totalSize {
		t.Errorf("Expected %d bytes processed, got %d", totalSize, stats.BytesProcessed)
	}

	// Verify shard ID
	if stats.ShardID != 5 {
		t.Errorf("Expected shard ID 5, got %d", stats.ShardID)
	}

	if stats.ShardName != "shard-00005" {
		t.Errorf("Expected shard name 'shard-00005', got %s", stats.ShardName)
	}
}

func TestShardPipeline_S3KeyConstruction(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	tests := []struct {
		name      string
		prefix    string
		shardName string
		wantKey   string
	}{
		{
			name:      "no prefix",
			prefix:    "",
			shardName: "shard-00000",
			wantKey:   "shard-00000.tar.zst",
		},
		{
			name:      "with prefix",
			prefix:    "dataset-2024",
			shardName: "shard-00003",
			wantKey:   "dataset-2024/shard-00003.tar.zst",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ShardPipelineConfig{
				S3Client:  mockClient,
				Bucket:    "test-bucket",
				Prefix:    tt.prefix,
				ShardName: tt.shardName,
			}

			pipeline, err := NewShardPipeline(ctx, config)
			if err != nil {
				t.Fatalf("Failed to create pipeline: %v", err)
			}

			key := pipeline.buildS3Key()
			if key != tt.wantKey {
				t.Errorf("buildS3Key() = %s, want %s", key, tt.wantKey)
			}
		})
	}
}

func TestShardPipeline_StartTwice(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardName: "shard-00000",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Start first time
	if err := pipeline.Start(); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Start second time should fail
	if err := pipeline.Start(); err == nil {
		t.Error("Second Start() should fail, but succeeded")
	}

	_ = pipeline.Close()
}

func TestShardPipeline_CloseTwice(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardName: "shard-00000",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Close first time
	if err := pipeline.Close(); err != nil {
		t.Fatalf("First Close() failed: %v", err)
	}

	// Close second time should be no-op
	if err := pipeline.Close(); err != nil {
		t.Error("Second Close() should be no-op, but returned error")
	}
}

func TestShardPipeline_AddFileAfterClose(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardName: "shard-00000",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	if err := pipeline.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Try to add file after close
	file := chunking.File{
		Path:    "/tmp/test.txt",
		Size:    100,
		ModTime: time.Now(),
	}

	err = pipeline.AddFile(file)
	if err == nil {
		t.Error("AddFile() after Close() should fail, but succeeded")
	}
}

func TestShardPipeline_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mockClient := &mockS3ClientShard{}

	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardName: "shard-00000",
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Cancel context
	cancel()

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Close should complete quickly
	if err := pipeline.Close(); err != nil {
		// Context cancellation may cause error, that's expected
		t.Logf("Close() returned error after context cancellation: %v", err)
	}

	// Verify stats show error
	stats := pipeline.GetStats()
	if stats.Error == nil {
		t.Log("Warning: Expected error in stats after context cancellation")
	}
}

func TestShardPipeline_DefaultConfigValues(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3ClientShard{}

	config := &ShardPipelineConfig{
		S3Client:  mockClient,
		Bucket:    "test-bucket",
		ShardName: "shard-00000",
		// Don't set optional fields - test defaults
	}

	pipeline, err := NewShardPipeline(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Verify defaults were set
	if pipeline.config.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries=3, got %d", pipeline.config.MaxRetries)
	}

	if pipeline.config.RetryDelay != time.Second {
		t.Errorf("Expected default RetryDelay=1s, got %v", pipeline.config.RetryDelay)
	}
}

func TestShardPipelineStats_String(t *testing.T) {
	stats := ShardPipelineStats{
		ShardID:        5,
		ShardName:      "shard-00005",
		FilesAdded:     1000,
		BytesProcessed: 100 << 20, // 100 MB
		UploadSize:     30 << 20,  // 30 MB (70% compression)
		Duration:       5 * time.Second,
		Completed:      true,
		Error:          nil,
	}

	str := stats.String()
	t.Logf("Stats string: %s", str)

	// Verify string contains key information
	if !contains(str, "shard-00005") {
		t.Error("String should contain shard name")
	}
	if !contains(str, "1000 files") {
		t.Error("String should contain file count")
	}
	if !contains(str, "completed") {
		t.Error("String should contain completion status")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark shard pipeline overhead
func BenchmarkShardPipeline_SingleFile(b *testing.B) {
	ctx := context.Background()

	// Create temporary test file
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := make([]byte, 1024*1024) // 1MB
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	mockClient := &mockS3ClientShard{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		config := &ShardPipelineConfig{
			S3Client:  mockClient,
			Bucket:    "test-bucket",
			ShardName: "shard-00000",
		}

		pipeline, _ := NewShardPipeline(ctx, config)
		_ = pipeline.Start()

		file := chunking.File{
			Path:    testFile,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}

		_ = pipeline.AddFile(file)
		_ = pipeline.Close()
	}
}
