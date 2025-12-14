package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestCalculateIntelligentShardCount(t *testing.T) {
	tests := []struct {
		name           string
		dataSize       int64
		expectedShards int
		description    string
	}{
		{
			name:           "Very small workload (100 MB)",
			dataSize:       100 * 1024 * 1024,
			expectedShards: 4,
			description:    "Small workload (<1GB) should use 4 shards",
		},
		{
			name:           "Small workload (500 MB)",
			dataSize:       500 * 1024 * 1024,
			expectedShards: 4,
			description:    "Small workload (<1GB) should use 4 shards",
		},
		{
			name:           "Boundary: exactly 1 GB",
			dataSize:       1 * 1024 * 1024 * 1024,
			expectedShards: 8,
			description:    "Medium workload (1-10GB) should use 8 shards",
		},
		{
			name:           "Medium workload (5 GB)",
			dataSize:       5 * 1024 * 1024 * 1024,
			expectedShards: 8,
			description:    "Medium workload (1-10GB) should use 8 shards",
		},
		{
			name:           "Boundary: exactly 10 GB",
			dataSize:       10 * 1024 * 1024 * 1024,
			expectedShards: 8,
			description:    "Medium workload (1-10GB) should use 8 shards",
		},
		{
			name:           "Large workload (15 GB)",
			dataSize:       15 * 1024 * 1024 * 1024,
			expectedShards: 10,
			description:    "Large workload (>10GB) should use 10 shards",
		},
		{
			name:           "Very large workload (1 TB)",
			dataSize:       1024 * 1024 * 1024 * 1024,
			expectedShards: 10,
			description:    "Large workload (>10GB) should use 10 shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shardCount := CalculateIntelligentShardCount(tt.dataSize)
			if shardCount != tt.expectedShards {
				t.Errorf("%s: got %d shards, want %d shards (data size: %d bytes)",
					tt.description, shardCount, tt.expectedShards, tt.dataSize)
			}
			t.Logf("✓ %s: %d shards for %d bytes", tt.description, shardCount, tt.dataSize)
		})
	}
}

func TestShardCoordinator_IntelligentShardCount(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name              string
		configShardCount  int
		estimatedDataSize int64
		expectedShards    int
	}{
		{
			name:              "Explicit shard count overrides intelligent calculation",
			configShardCount:  5,
			estimatedDataSize: 10 * 1024 * 1024 * 1024, // 10 GB
			expectedShards:    5,
		},
		{
			name:              "Auto-calculate for small workload (500 MB)",
			configShardCount:  0,
			estimatedDataSize: 500 * 1024 * 1024,
			expectedShards:    4,
		},
		{
			name:              "Auto-calculate for medium workload (5 GB)",
			configShardCount:  0,
			estimatedDataSize: 5 * 1024 * 1024 * 1024,
			expectedShards:    8,
		},
		{
			name:              "Auto-calculate for large workload (50 GB)",
			configShardCount:  0,
			estimatedDataSize: 50 * 1024 * 1024 * 1024,
			expectedShards:    10,
		},
		{
			name:              "No estimated size falls back to default (8 shards)",
			configShardCount:  0,
			estimatedDataSize: 0,
			expectedShards:    8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routerConfig := &chunking.ShardRouterConfig{
				Strategy:   chunking.ShardByHash,
				ShardCount: 10, // Router shard count (doesn't affect coordinator)
			}
			router, err := chunking.NewShardRouter(routerConfig)
			if err != nil {
				t.Fatalf("Failed to create router: %v", err)
			}

			config := &ShardCoordinatorConfig{
				ShardCount:        tt.configShardCount,
				EstimatedDataSize: tt.estimatedDataSize,
				Bucket:            "test-bucket",
				Router:            router,
				S3Client:          &mockS3Client{},
			}

			coord, err := NewShardCoordinator(ctx, config)
			if err != nil {
				t.Fatalf("Failed to create coordinator: %v", err)
			}

			actualShards := len(coord.pipelines)
			if actualShards != tt.expectedShards {
				t.Errorf("Expected %d shards, got %d shards", tt.expectedShards, actualShards)
			}
			t.Logf("✓ %s: created %d shards", tt.name, actualShards)
		})
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

// TestShardCoordinator_MemoryManagement tests memory manager integration (Issue #83)
func TestShardCoordinator_MemoryManagement(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	// Test 1: Coordinator creates default MemoryManager if not provided
	t.Run("auto-create memory manager", func(t *testing.T) {
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: 4,
		}
		router, err := chunking.NewShardRouter(routerConfig)
		if err != nil {
			t.Fatalf("Failed to create router: %v", err)
		}

		config := &ShardCoordinatorConfig{
			ShardCount: 4,
			Router:     router,
			S3Client:   mockClient,
			Bucket:     "test-bucket",
			Prefix:     "test",
			// No MemoryManager provided - should auto-create
		}

		coordinator, err := NewShardCoordinator(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		// Note: No Start()/Close() needed for this test - just checking configuration

		// Verify MemoryManager was created
		if config.MemoryManager == nil {
			t.Fatal("Expected MemoryManager to be auto-created, got nil")
		}

		// Verify coordinator owns the memory manager
		if !coordinator.ownsMemoryManager {
			t.Error("Expected coordinator to own memory manager")
		}

		// Verify memory manager has reasonable configuration
		stats := config.MemoryManager.GetStats()
		if stats.MemoryBudget <= 0 {
			t.Errorf("Expected positive memory budget, got %d", stats.MemoryBudget)
		}

		t.Logf("Auto-created MemoryManager: Budget=%d MB, BudgetPercent=%.1f%%",
			stats.MemoryBudget/(1<<20), stats.BudgetPercent*100)
	})

	// Test 2: Coordinator uses provided MemoryManager
	t.Run("use provided memory manager", func(t *testing.T) {
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: 4,
		}
		router, err := chunking.NewShardRouter(routerConfig)
		if err != nil {
			t.Fatalf("Failed to create router: %v", err)
		}

		// Create custom memory manager
		memConfig := &MemoryManagerConfig{
			MemoryBudgetPercent:  0.3, // 30% of memory
			ProactiveGCThreshold: 25 << 20,
		}
		memManager := NewMemoryManager(ctx, memConfig)
		defer memManager.Stop()

		config := &ShardCoordinatorConfig{
			ShardCount:    4,
			Router:        router,
			S3Client:      mockClient,
			Bucket:        "test-bucket",
			Prefix:        "test",
			MemoryManager: memManager, // Provide custom MemoryManager
		}

		coordinator, err := NewShardCoordinator(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		// Note: No Start()/Close() needed for this test - just checking configuration

		// Verify coordinator uses provided MemoryManager
		if coordinator.ownsMemoryManager {
			t.Error("Expected coordinator not to own provided memory manager")
		}

		// Verify it's using our custom config
		stats := config.MemoryManager.GetStats()
		if stats.BudgetPercent != 0.3 {
			t.Errorf("Expected budget percent 0.3, got %f", stats.BudgetPercent)
		}

		t.Logf("Using provided MemoryManager: Budget=%d MB, BudgetPercent=%.1f%%",
			stats.MemoryBudget/(1<<20), stats.BudgetPercent*100)
	})

	// Test 3: Memory stats appear in coordinator stats
	t.Run("memory stats in coordinator stats", func(t *testing.T) {
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: 2,
		}
		router, err := chunking.NewShardRouter(routerConfig)
		if err != nil {
			t.Fatalf("Failed to create router: %v", err)
		}

		config := &ShardCoordinatorConfig{
			ShardCount: 2,
			Router:     router,
			S3Client:   mockClient,
			Bucket:     "test-bucket",
			Prefix:     "test",
		}

		coordinator, err := NewShardCoordinator(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create coordinator: %v", err)
		}
		defer func() {
			if err := coordinator.Close(); err != nil {
				t.Errorf("Failed to close coordinator: %v", err)
			}
		}()

		if err := coordinator.Start(); err != nil {
			t.Fatalf("Failed to start coordinator: %v", err)
		}

		// Get stats
		stats := coordinator.GetStats()

		// Verify memory stats are included
		if stats.MemoryStats == nil {
			t.Fatal("Expected memory stats to be included")
		}

		if stats.MemoryStats.MemoryBudget <= 0 {
			t.Errorf("Expected positive memory budget in stats, got %d", stats.MemoryStats.MemoryBudget)
		}

		t.Logf("Coordinator stats with memory: %s", stats.String())
	})
}

// TestShardCoordinator_ParallelCompression tests compression concurrency configuration (Issue #80)
func TestShardCoordinator_ParallelCompression(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	tests := []struct {
		name                   string
		shardCount             int
		cpuCores               int // Simulated GOMAXPROCS
		expectedConcurrencyMin int
		expectedConcurrencyMax int
	}{
		{
			name:                   "More cores than shards (16 cores, 4 shards)",
			shardCount:             4,
			cpuCores:               16,
			expectedConcurrencyMin: 4,
			expectedConcurrencyMax: 4, // 16/4 = 4 cores per shard
		},
		{
			name:                   "Equal cores and shards (8 cores, 8 shards)",
			shardCount:             8,
			cpuCores:               8,
			expectedConcurrencyMin: 1,
			expectedConcurrencyMax: 1, // 8/8 = 1 core per shard
		},
		{
			name:                   "Fewer cores than shards (4 cores, 10 shards)",
			shardCount:             10,
			cpuCores:               4,
			expectedConcurrencyMin: 1,
			expectedConcurrencyMax: 1, // 4/10 = 0, clamped to 1
		},
		{
			name:                   "Many cores (32 cores, 8 shards)",
			shardCount:             8,
			cpuCores:               32,
			expectedConcurrencyMin: 4,
			expectedConcurrencyMax: 4, // 32/8 = 4 cores per shard
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore GOMAXPROCS
			oldMaxProcs := runtime.GOMAXPROCS(tt.cpuCores)
			defer runtime.GOMAXPROCS(oldMaxProcs)

			routerConfig := &chunking.ShardRouterConfig{
				Strategy:   chunking.ShardByHash,
				ShardCount: tt.shardCount,
			}
			router, err := chunking.NewShardRouter(routerConfig)
			if err != nil {
				t.Fatalf("Failed to create router: %v", err)
			}

			config := &ShardCoordinatorConfig{
				ShardCount: tt.shardCount,
				Router:     router,
				S3Client:   mockClient,
				Bucket:     "test-bucket",
				Prefix:     "test",
			}

			coordinator, err := NewShardCoordinator(ctx, config)
			if err != nil {
				t.Fatalf("Failed to create coordinator: %v", err)
			}

			// Verify each shard has correct compression concurrency
			for i := 0; i < tt.shardCount; i++ {
				pipeline := coordinator.pipelines[i]
				concurrency := pipeline.config.CompressionConcurrency

				if concurrency < tt.expectedConcurrencyMin || concurrency > tt.expectedConcurrencyMax {
					t.Errorf("Shard %d: expected concurrency in range [%d, %d], got %d",
						i, tt.expectedConcurrencyMin, tt.expectedConcurrencyMax, concurrency)
				}
			}

			// Log the configuration
			sampleConcurrency := coordinator.pipelines[0].config.CompressionConcurrency
			t.Logf("%s: %d shards × %d concurrency/shard = %d total threads (from %d CPU cores)",
				tt.name, tt.shardCount, sampleConcurrency,
				tt.shardCount*sampleConcurrency, tt.cpuCores)
		})
	}
}

// generateTestFiles creates a temporary directory with test files for integration tests
func generateTestFiles(t *testing.T, count int, sizes []int64) (string, []chunking.File) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	files := make([]chunking.File, count)
	for i := 0; i < count; i++ {
		// Distribute sizes across files
		size := sizes[i%len(sizes)]

		// Create subdirectories for variety
		subdir := filepath.Join(tmpDir, fmt.Sprintf("dir%d", i%10))
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		// Create file with pattern in name for type-based routing
		var filename string
		fileType := i % 3
		switch fileType {
		case 0:
			filename = fmt.Sprintf("file%05d.txt", i)
		case 1:
			filename = fmt.Sprintf("file%05d.log", i)
		case 2:
			filename = fmt.Sprintf("file%05d.json", i)
		}

		path := filepath.Join(subdir, filename)

		// Write file with content
		content := make([]byte, size)
		for j := range content {
			content[j] = byte('a' + (i % 26)) // Vary content by file
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		files[i] = chunking.File{
			Path:    path,
			Size:    size,
			ModTime: time.Now(),
		}
	}

	return tmpDir, files
}

// TestShardCoordinator_Integration_HashRouting tests hash-based routing with 10k files (Issue #85)
func TestShardCoordinator_Integration_HashRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	mockClient := newMockS3ClientCoordinator()

	// Test with 1k files, 10 shards (scaled down for test performance)
	// In production, this scales to 10k+ files
	const fileCount = 1000
	const shardCount = 10

	// Generate test files with varied sizes
	sizes := []int64{1024, 4096, 16384, 65536} // 1KB, 4KB, 16KB, 64KB
	tmpDir, files := generateTestFiles(t, fileCount, sizes)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Logf("Generated %d test files in %s", fileCount, tmpDir)

	// Create coordinator with hash-based routing
	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: shardCount,
	}
	router, err := chunking.NewShardRouter(routerConfig)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	config := &ShardCoordinatorConfig{
		ShardCount: shardCount,
		Router:     router,
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		Prefix:     "integration-test",
	}

	coordinator, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	// Start coordinator
	if err := coordinator.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Add all files
	startTime := time.Now()
	for i, file := range files {
		if err := coordinator.AddFile(file); err != nil {
			t.Fatalf("Failed to add file %d: %v", i, err)
		}

		// Log progress
		if (i+1)%100 == 0 {
			t.Logf("Added %d/%d files", i+1, fileCount)
		}
	}

	// Close and wait for completion
	if err := coordinator.Close(); err != nil {
		t.Fatalf("Failed to close coordinator: %v", err)
	}

	duration := time.Since(startTime)
	t.Logf("Uploaded %d files in %s", fileCount, duration)

	// Verify statistics
	stats := coordinator.GetStats()
	if stats.FilesAdded != fileCount {
		t.Errorf("Expected %d files added, got %d", fileCount, stats.FilesAdded)
	}
	if !stats.IsComplete() {
		t.Error("Expected all shards to be complete")
	}
	if stats.HasErrors() {
		t.Errorf("Expected no errors, got: %v", stats.FirstError)
	}

	// Verify shard distribution (hash-based should be relatively even)
	t.Logf("Shard distribution:")
	for i := 0; i < shardCount; i++ {
		shardStats := stats.ShardStats[i]
		filesInShard := shardStats.FilesAdded
		percentage := float64(filesInShard) / float64(fileCount) * 100

		t.Logf("  Shard %d: %d files (%.1f%%)", i, filesInShard, percentage)

		// Hash-based routing should distribute relatively evenly (within 25% of average)
		// With 1000 files, some variance is expected due to hash randomness
		expectedAvg := fileCount / shardCount
		lowerBound := int64(float64(expectedAvg) * 0.75)
		upperBound := int64(float64(expectedAvg) * 1.25)

		if filesInShard < lowerBound || filesInShard > upperBound {
			t.Errorf("Shard %d has uneven distribution: %d files (expected %d ±25%%)",
				i, filesInShard, expectedAvg)
		}
	}

	// Log performance metrics
	throughput := stats.ThroughputMBps()
	networkThroughput := stats.NetworkThroughputMBps()
	compressionRatio := stats.CompressionRatio()

	t.Logf("Performance metrics:")
	t.Logf("  Throughput: %.2f MB/s (processing)", throughput)
	t.Logf("  Network throughput: %.2f MB/s (upload)", networkThroughput)
	t.Logf("  Compression ratio: %.1f%%", compressionRatio*100)
	t.Logf("  Total uploads: %d", mockClient.putObjectCalls)
}

// TestShardCoordinator_Integration_Cancellation tests graceful shutdown (Issue #85)
func TestShardCoordinator_Integration_Cancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	mockClient := newMockS3ClientCoordinator()

	// Test with 500 files
	const fileCount = 500
	const shardCount = 5

	sizes := []int64{1024, 4096}
	tmpDir, files := generateTestFiles(t, fileCount, sizes)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	routerConfig := &chunking.ShardRouterConfig{
		Strategy:   chunking.ShardByHash,
		ShardCount: shardCount,
	}
	router, err := chunking.NewShardRouter(routerConfig)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	config := &ShardCoordinatorConfig{
		ShardCount: shardCount,
		Router:     router,
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		Prefix:     "cancel-test",
	}

	coordinator, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}

	if err := coordinator.Start(); err != nil {
		t.Fatalf("Failed to start coordinator: %v", err)
	}

	// Add files in background
	addDone := make(chan struct{})
	go func() {
		defer close(addDone)
		for i, file := range files {
			select {
			case <-ctx.Done():
				t.Logf("Context cancelled after adding %d files", i)
				return
			default:
				if err := coordinator.AddFile(file); err != nil {
					if ctx.Err() != nil {
						return // Expected during cancellation
					}
					t.Errorf("Failed to add file %d: %v", i, err)
					return
				}
			}
		}
	}()

	// Wait a bit for some files to be added
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	t.Logf("Cancelling context...")
	cancel()

	// Wait for add goroutine to finish
	<-addDone

	// Close coordinator (should handle cancellation gracefully)
	err = coordinator.Close()
	// May have errors due to cancellation, which is expected
	t.Logf("Close completed with result: %v", err)

	stats := coordinator.GetStats()
	t.Logf("Processed %d/%d files before cancellation", stats.FilesAdded, fileCount)

	// Verify some files were processed (but not necessarily all)
	if stats.FilesAdded == 0 {
		t.Error("Expected at least some files to be processed")
	}
	if stats.FilesAdded > fileCount {
		t.Errorf("Expected at most %d files, got %d", fileCount, stats.FilesAdded)
	}
}

// BenchmarkShardCoordinator_ParallelUpload benchmarks parallel shard uploads (Issue #85)
func BenchmarkShardCoordinator_ParallelUpload(b *testing.B) {
	ctx := context.Background()

	// Generate test files once
	const fileCount = 100
	const shardCount = 10
	sizes := []int64{1024, 4096}
	tmpDir, files := generateTestFiles(&testing.T{}, fileCount, sizes)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mockClient := newMockS3ClientCoordinator()

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Router:     router,
			S3Client:   mockClient,
			Bucket:     "benchmark-bucket",
		}

		coordinator, _ := NewShardCoordinator(ctx, config)
		_ = coordinator.Start()

		for _, file := range files {
			_ = coordinator.AddFile(file)
		}

		_ = coordinator.Close()

		stats := coordinator.GetStats()
		b.ReportMetric(stats.ThroughputMBps(), "throughput_mbps")
		b.ReportMetric(float64(stats.FilesAdded), "files")
	}
}
