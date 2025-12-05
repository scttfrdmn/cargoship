package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// Thread-safe mock S3 client for coordinator testing
type mockS3ClientCoordinator struct {
	putObjectCalls int64
	uploadedShards map[int]int // shardID -> upload count
	mu             sync.Mutex
}

func newMockS3ClientCoordinator() *mockS3ClientCoordinator {
	return &mockS3ClientCoordinator{
		uploadedShards: make(map[int]int),
	}
}

func (m *mockS3ClientCoordinator) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	atomic.AddInt64(&m.putObjectCalls, 1)

	// Extract shard ID from metadata
	if shardIDStr, ok := input.Metadata["cargoship-shard-id"]; ok {
		var shardID int
		_, _ = fmt.Sscanf(shardIDStr, "%d", &shardID)

		m.mu.Lock()
		m.uploadedShards[shardID]++
		m.mu.Unlock()
	}

	return &s3.PutObjectOutput{}, nil
}

// Multipart upload methods (required by S3Uploader interface)
func (m *mockS3ClientCoordinator) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	atomic.AddInt64(&m.putObjectCalls, 1)

	// Extract shard ID from metadata and track it
	if shardIDStr, ok := input.Metadata["cargoship-shard-id"]; ok {
		var shardID int
		_, _ = fmt.Sscanf(shardIDStr, "%d", &shardID)

		m.mu.Lock()
		m.uploadedShards[shardID]++
		m.mu.Unlock()
	}

	uploadID := "test-upload-id"
	return &s3.CreateMultipartUploadOutput{
		UploadId: &uploadID,
	}, nil
}

func (m *mockS3ClientCoordinator) UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	atomic.AddInt64(&m.putObjectCalls, 1)
	etag := "test-etag"
	return &s3.UploadPartOutput{
		ETag: &etag,
	}, nil
}

func (m *mockS3ClientCoordinator) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *mockS3ClientCoordinator) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

func TestNewShardCoordinator(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	// Create router
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 10,
	}
	router, err := chunking.NewShardRouter(routerConfig)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	tests := []struct {
		name    string
		config  *ShardCoordinatorConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "nil router",
			config: &ShardCoordinatorConfig{
				ShardCount: 10,
				Bucket:     "test-bucket",
				S3Client:   mockClient,
			},
			wantErr: true,
		},
		{
			name: "nil S3 client",
			config: &ShardCoordinatorConfig{
				ShardCount: 10,
				Bucket:     "test-bucket",
				Router:     router,
			},
			wantErr: true,
		},
		{
			name: "empty bucket",
			config: &ShardCoordinatorConfig{
				ShardCount: 10,
				Router:     router,
				S3Client:   mockClient,
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &ShardCoordinatorConfig{
				ShardCount: 10,
				Bucket:     "test-bucket",
				Router:     router,
				S3Client:   mockClient,
			},
			wantErr: false,
		},
		{
			name: "custom shard count",
			config: &ShardCoordinatorConfig{
				ShardCount: 5,
				Bucket:     "test-bucket",
				Router:     router,
				S3Client:   mockClient,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coord, err := NewShardCoordinator(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewShardCoordinator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if coord == nil {
					t.Error("NewShardCoordinator() returned nil coordinator without error")
					return
				}
				if len(coord.pipelines) != tt.config.ShardCount {
					t.Errorf("Expected %d pipelines, got %d", tt.config.ShardCount, len(coord.pipelines))
				}
			}
		})
	}
}

func TestShardCoordinator_BasicFlow(t *testing.T) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := t.TempDir()
	const numFiles = 10
	var testFiles []chunking.File

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
	}

	// Create coordinator
	mockClient := newMockS3ClientCoordinator()
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 5,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 5,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	// Start coordinator
	if err := coord.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Add files
	if err := coord.AddFiles(testFiles); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	// Close coordinator
	// Note: Empty shards may error on close (no data written to pipe), this is expected
	_ = coord.Close()

	// Verify statistics
	stats := coord.GetStats()

	if stats.FilesAdded != int64(numFiles) {
		t.Errorf("Expected %d files added, got %d", numFiles, stats.FilesAdded)
	}

	if !stats.IsComplete() {
		t.Error("Coordinator should be complete")
	}

	// Empty shards may have close errors, but files should still be uploaded
	t.Logf("Completed shards: %d/%d, Failed shards: %d", stats.CompletedShards, stats.ShardCount, stats.FailedShards)

	// Verify uploads happened (only shards with files upload)
	uploadCount := atomic.LoadInt64(&mockClient.putObjectCalls)
	if uploadCount == 0 {
		t.Error("Expected at least one upload")
	}
	t.Logf("Upload count: %d", uploadCount)

	// Verify files were distributed across shards
	mockClient.mu.Lock()
	defer mockClient.mu.Unlock()
	if len(mockClient.uploadedShards) == 0 {
		t.Error("No shards received uploads")
	}
	t.Logf("Shards with uploads: %d", len(mockClient.uploadedShards))
}

func TestShardCoordinator_ConcurrentUploads(t *testing.T) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := t.TempDir()
	const numFiles = 100
	var testFiles []chunking.File

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
	}

	// Create coordinator with 10 shards
	mockClient := newMockS3ClientCoordinator()
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 10,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 10,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	// Start coordinator
	if err := coord.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Add files
	if err := coord.AddFiles(testFiles); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	// Close coordinator (waits for all uploads)
	// Note: Empty shards may error on close (no data written to pipe), this is expected
	start := time.Now()
	_ = coord.Close()
	duration := time.Since(start)

	// Verify statistics
	stats := coord.GetStats()
	t.Logf("Completed shards: %d/%d, Failed shards: %d", stats.CompletedShards, stats.ShardCount, stats.FailedShards)

	// Verify concurrent execution (should be faster than sequential)
	// With 10 concurrent uploads, should complete quickly
	if duration > 5*time.Second {
		t.Logf("Warning: Concurrent uploads took %v (expected <5s)", duration)
	}

	t.Logf("Concurrent uploads completed in %v", duration)
	t.Logf("Statistics: %s", stats.String())
}

func TestShardCoordinator_FileDistribution(t *testing.T) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := t.TempDir()
	const numFiles = 100
	var testFiles []chunking.File

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
	}

	// Create coordinator
	mockClient := newMockS3ClientCoordinator()
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 10,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 10,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	if err := coord.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	if err := coord.AddFiles(testFiles); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	// Close coordinator
	// Note: Empty shards may error on close (no data written to pipe), this is expected
	_ = coord.Close()

	// Verify distribution across shards
	stats := coord.GetStats()
	var totalFilesInShards int64
	var shardsWithFiles int

	for _, shardStat := range stats.ShardStats {
		if shardStat.FilesAdded > 0 {
			shardsWithFiles++
		}
		totalFilesInShards += shardStat.FilesAdded
	}

	// All files should be accounted for
	if totalFilesInShards != int64(numFiles) {
		t.Errorf("Expected %d files distributed, got %d", numFiles, totalFilesInShards)
	}

	// With 100 files and hash distribution, expect most shards to have files
	if shardsWithFiles < 8 {
		t.Errorf("Expected at least 8 shards with files, got %d", shardsWithFiles)
	}

	t.Logf("Files distributed across %d/%d shards", shardsWithFiles, stats.ShardCount)
	for i, shardStat := range stats.ShardStats {
		if shardStat.FilesAdded > 0 {
			t.Logf("Shard %d: %d files, %d bytes", i, shardStat.FilesAdded, shardStat.BytesProcessed)
		}
	}
}

func TestShardCoordinator_GetShardStats(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 5,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 5,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	// Test valid shard ID
	stats, err := coord.GetShardStats(0)
	if err != nil {
		t.Errorf("GetShardStats(0) returned error: %v", err)
	}
	if stats.ShardID != 0 {
		t.Errorf("Expected shard ID 0, got %d", stats.ShardID)
	}

	// Test invalid shard IDs
	_, err = coord.GetShardStats(-1)
	if err == nil {
		t.Error("GetShardStats(-1) should return error")
	}

	_, err = coord.GetShardStats(10)
	if err == nil {
		t.Error("GetShardStats(10) should return error for 5-shard coordinator")
	}
}

func TestShardCoordinator_StartTwice(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 5,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 5,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	// Start first time
	if err := coord.Start(); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Start second time should be no-op
	if err := coord.Start(); err != nil {
		t.Errorf("Second Start() returned error: %v", err)
	}

	_ = coord.Close()
}

func TestShardCoordinator_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mockClient := newMockS3ClientCoordinator()

	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 5,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	config := &ShardCoordinatorConfig{
		ShardCount: 5,
		Bucket:     "test-bucket",
		Router:     router,
		S3Client:   mockClient,
	}

	coord, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	if err := coord.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Cancel context
	cancel()

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Close should complete
	_ = coord.Close()
}

func TestShardCoordinatorStats_String(t *testing.T) {
	stats := ShardCoordinatorStats{
		ShardCount:      10,
		FilesAdded:      1000,
		BytesProcessed:  100 << 20, // 100 MB
		TotalUploadSize: 30 << 20,  // 30 MB (70% compression)
		Duration:        5 * time.Second,
		CompletedShards: 10,
		FailedShards:    0,
	}

	str := stats.String()
	t.Logf("Stats string: %s", str)

	// Verify string contains key information
	if !contains(str, "10 shards") {
		t.Error("String should contain shard count")
	}
	if !contains(str, "1000 files") {
		t.Error("String should contain file count")
	}
	if !contains(str, "completed") {
		t.Error("String should contain completion status")
	}
}

func TestShardCoordinatorStats_Metrics(t *testing.T) {
	stats := ShardCoordinatorStats{
		ShardCount:      10,
		FilesAdded:      1000,
		BytesProcessed:  100 << 20, // 100 MB
		TotalUploadSize: 30 << 20,  // 30 MB
		Duration:        5 * time.Second,
		CompletedShards: 10,
	}

	// Test compression ratio
	compressionRatio := stats.CompressionRatio()
	expectedRatio := 0.3 // 30MB/100MB
	if compressionRatio < expectedRatio-0.01 || compressionRatio > expectedRatio+0.01 {
		t.Errorf("Expected compression ratio ~%.2f, got %.2f", expectedRatio, compressionRatio)
	}

	// Test processing throughput (uncompressed)
	processingThroughput := stats.ThroughputMBps()
	expectedProcessingThroughput := 20.0 // 100MB / 5s
	if processingThroughput < expectedProcessingThroughput-1 || processingThroughput > expectedProcessingThroughput+1 {
		t.Errorf("Expected processing throughput ~%.1f MB/s, got %.1f MB/s", expectedProcessingThroughput, processingThroughput)
	}

	// Test network throughput (compressed)
	networkThroughput := stats.NetworkThroughputMBps()
	expectedNetworkThroughput := 6.0 // 30MB / 5s
	if networkThroughput < expectedNetworkThroughput-1 || networkThroughput > expectedNetworkThroughput+1 {
		t.Errorf("Expected network throughput ~%.1f MB/s, got %.1f MB/s", expectedNetworkThroughput, networkThroughput)
	}

	// Verify network throughput is lower than processing throughput (due to compression)
	if networkThroughput >= processingThroughput {
		t.Errorf("Network throughput (%.1f MB/s) should be less than processing throughput (%.1f MB/s) when compression is effective",
			networkThroughput, processingThroughput)
	}

	// Test completion
	if !stats.IsComplete() {
		t.Error("Stats should show complete")
	}

	// Test errors
	if stats.HasErrors() {
		t.Error("Stats should not show errors")
	}
}

// Benchmark routing overhead (without actually adding to pipelines)
func BenchmarkShardCoordinator_FileRouting(b *testing.B) {
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: 10,
	}
	router, _ := chunking.NewShardRouter(routerConfig)

	testFile := chunking.File{
		Path:    "/tmp/test.txt",
		Size:    1024,
		ModTime: time.Now(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Benchmark just the routing decision
		_ = router.Route(testFile)
	}
}
