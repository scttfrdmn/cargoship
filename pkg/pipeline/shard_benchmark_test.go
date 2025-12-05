package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// Benchmark-specific mock S3 client with minimal overhead
type benchmarkS3Client struct {
	uploadCount int64
}

func (m *benchmarkS3Client) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	atomic.AddInt64(&m.uploadCount, 1)
	// Don't actually read the body to minimize overhead
	return &s3.PutObjectOutput{}, nil
}

// Multipart upload methods (required by S3Uploader interface)
func (m *benchmarkS3Client) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	uploadID := "test-upload-id"
	return &s3.CreateMultipartUploadOutput{
		UploadId: &uploadID,
	}, nil
}

func (m *benchmarkS3Client) UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	atomic.AddInt64(&m.uploadCount, 1)
	etag := "test-etag"
	return &s3.UploadPartOutput{
		ETag: &etag,
	}, nil
}

func (m *benchmarkS3Client) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *benchmarkS3Client) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

// createBenchmarkFiles creates a temporary directory with N test files
func createBenchmarkFiles(b *testing.B, count int, sizeBytes int64) (string, []chunking.File) {
	b.Helper()

	tmpDir := b.TempDir()
	files := make([]chunking.File, count)

	// Create test data once and reuse
	testData := make([]byte, sizeBytes)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	for i := 0; i < count; i++ {
		// Use subdirectories to avoid too many files in one dir
		subdir := filepath.Join(tmpDir, fmt.Sprintf("dir%d", i/1000))
		if i%1000 == 0 {
			if err := os.MkdirAll(subdir, 0755); err != nil {
				b.Fatalf("Failed to create subdir: %v", err)
			}
		}

		filename := filepath.Join(subdir, fmt.Sprintf("file%d.dat", i))
		if err := os.WriteFile(filename, testData, 0644); err != nil {
			b.Fatalf("Failed to create file %d: %v", i, err)
		}

		files[i] = chunking.File{
			Path:    filename,
			Size:    sizeBytes,
			ModTime: time.Now(),
		}
	}

	return tmpDir, files
}

// Benchmark 10k files (target: <3s, 20x improvement over 60s)
func BenchmarkShardedUpload_10kFiles(b *testing.B) {
	const (
		fileCount = 10000
		fileSize  = 1024 // 1KB per file
		shardCount = 10
	)

	ctx := context.Background()
	mockClient := &benchmarkS3Client{}

	// Create test files once
	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Reset mock client
		atomic.StoreInt64(&mockClient.uploadCount, 0)

		// Create router
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		// Create coordinator
		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     "benchmark-bucket",
			Router:     router,
			S3Client:   mockClient,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		// Add all files
		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		// Close and wait for completion
		_ = coord.Close()

		b.StopTimer()

		// Report statistics
		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
	}

	// Check if we met the target (<3s for 10k files)
	if b.N > 0 {
		avgDuration := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
		targetDuration := 3 * time.Second
		if avgDuration < targetDuration {
			b.Logf("✅ TARGET MET: %v < %v (%.1fx faster than target)",
				avgDuration, targetDuration, float64(targetDuration)/float64(avgDuration))
		} else {
			b.Logf("⚠️  TARGET MISSED: %v >= %v", avgDuration, targetDuration)
		}
	}
}

// Benchmark 100k files (target: <30s, 20x improvement over 10min)
func BenchmarkShardedUpload_100kFiles(b *testing.B) {
	const (
		fileCount = 100000
		fileSize  = 512 // 512 bytes per file
		shardCount = 10
	)

	ctx := context.Background()
	mockClient := &benchmarkS3Client{}

	// Create test files once
	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		atomic.StoreInt64(&mockClient.uploadCount, 0)

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     "benchmark-bucket",
			Router:     router,
			S3Client:   mockClient,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		_ = coord.Close()

		b.StopTimer()

		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
	}

	if b.N > 0 {
		avgDuration := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
		targetDuration := 30 * time.Second
		if avgDuration < targetDuration {
			b.Logf("✅ TARGET MET: %v < %v (%.1fx faster than target)",
				avgDuration, targetDuration, float64(targetDuration)/float64(avgDuration))
		} else {
			b.Logf("⚠️  TARGET MISSED: %v >= %v", avgDuration, targetDuration)
		}
	}
}

// Benchmark 1M files (target: <2min, 10x improvement over 20min)
func BenchmarkShardedUpload_1MFiles(b *testing.B) {
	const (
		fileCount = 1000000
		fileSize  = 256 // 256 bytes per file
		shardCount = 10
	)

	ctx := context.Background()
	mockClient := &benchmarkS3Client{}

	// Create test files once
	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		atomic.StoreInt64(&mockClient.uploadCount, 0)

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     "benchmark-bucket",
			Router:     router,
			S3Client:   mockClient,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		_ = coord.Close()

		b.StopTimer()

		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
	}

	if b.N > 0 {
		avgDuration := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
		targetDuration := 2 * time.Minute
		if avgDuration < targetDuration {
			b.Logf("✅ TARGET MET: %v < %v (%.1fx faster than target)",
				avgDuration, targetDuration, float64(targetDuration)/float64(avgDuration))
		} else {
			b.Logf("⚠️  TARGET MISSED: %v >= %v", avgDuration, targetDuration)
		}
	}
}

// Benchmark traditional single-shard upload (baseline comparison)
func BenchmarkTraditionalUpload_10kFiles(b *testing.B) {
	const (
		fileCount = 10000
		fileSize  = 1024
		shardCount = 1 // Single shard = traditional approach
	)

	ctx := context.Background()
	mockClient := &benchmarkS3Client{}

	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		atomic.StoreInt64(&mockClient.uploadCount, 0)

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     "benchmark-bucket",
			Router:     router,
			S3Client:   mockClient,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		_ = coord.Close()

		b.StopTimer()

		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
	}
}

// Benchmark memory usage for large workloads
func BenchmarkMemoryUsage_100kFiles(b *testing.B) {
	const (
		fileCount = 100000
		fileSize  = 1024
		shardCount = 10
	)

	ctx := context.Background()
	mockClient := &benchmarkS3Client{}

	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     "benchmark-bucket",
			Router:     router,
			S3Client:   mockClient,
		}

		coord, _ := NewShardCoordinator(ctx, config)
		_ = coord.Start()
		_ = coord.AddFiles(files)
		_ = coord.Close()
	}
}

// Benchmark different shard counts to find optimal parallelism
func BenchmarkShardCount_Comparison(b *testing.B) {
	const (
		fileCount = 10000
		fileSize  = 1024
	)

	shardCounts := []int{1, 2, 4, 8, 10, 16, 20}

	for _, shardCount := range shardCounts {
		b.Run(fmt.Sprintf("shards=%d", shardCount), func(b *testing.B) {
			ctx := context.Background()
			mockClient := &benchmarkS3Client{}

			_, files := createBenchmarkFiles(b, fileCount, fileSize)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				routerConfig := &chunking.ShardRouterConfig{
					Strategy:   chunking.ShardByHash,
					ShardCount: shardCount,
				}
				router, _ := chunking.NewShardRouter(routerConfig)

				config := &ShardCoordinatorConfig{
					ShardCount: shardCount,
					Bucket:     "benchmark-bucket",
					Router:     router,
					S3Client:   mockClient,
				}

				coord, _ := NewShardCoordinator(ctx, config)
				_ = coord.Start()

				b.StartTimer()
				_ = coord.AddFiles(files)
				_ = coord.Close()
				b.StopTimer()

				stats := coord.GetStats()
				b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
			}
		})
	}
}

// Benchmark scaling: measure throughput vs file count
func BenchmarkScaling_FileCounts(b *testing.B) {
	fileCounts := []int{100, 1000, 10000, 50000}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("files=%d", fileCount), func(b *testing.B) {
			const (
				fileSize  = 1024
				shardCount = 10
			)

			ctx := context.Background()
			mockClient := &benchmarkS3Client{}

			_, files := createBenchmarkFiles(b, fileCount, fileSize)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				routerConfig := &chunking.ShardRouterConfig{
					Strategy:   chunking.ShardByHash,
					ShardCount: shardCount,
				}
				router, _ := chunking.NewShardRouter(routerConfig)

				config := &ShardCoordinatorConfig{
					ShardCount: shardCount,
					Bucket:     "benchmark-bucket",
					Router:     router,
					S3Client:   mockClient,
				}

				coord, _ := NewShardCoordinator(ctx, config)
				_ = coord.Start()

				b.StartTimer()
				_ = coord.AddFiles(files)
				_ = coord.Close()
				b.StopTimer()

				stats := coord.GetStats()
				b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
			}
		})
	}
}
