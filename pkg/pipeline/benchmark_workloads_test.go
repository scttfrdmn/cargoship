//go:build benchmark

package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// createS3Client creates an AWS S3 client for real S3 integration tests.
// Returns nil if CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS is not set to "1".
func createS3Client(tb testing.TB) *s3.Client {
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != "1" {
		return nil
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(os.Getenv("AWS_REGION")),
	)
	if err != nil {
		tb.Fatalf("Failed to load AWS config: %v", err)
	}

	return s3.NewFromConfig(cfg)
}

// BenchmarkPipeline_MixedWorkload benchmarks the pipeline with a realistic mix of file sizes
func BenchmarkPipeline_MixedWorkload(b *testing.B) {
	// Create test data directory
	testDir := b.TempDir()

	b.Logf("Creating mixed workload...")

	// Create a realistic mix of file sizes
	// 1000 tiny files (1KB - 50KB)
	for i := 0; i < 1000; i++ {
		size := 1024 + rand.Intn(49*1024)
		data := make([]byte, size)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("tiny_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Created 1000 tiny files (1KB-50KB)")

	// 500 small files (50KB - 500KB)
	for i := 0; i < 500; i++ {
		size := 50*1024 + rand.Intn(450*1024)
		data := make([]byte, size)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("small_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Created 500 small files (50KB-500KB)")

	// 200 medium files (500KB - 5MB)
	for i := 0; i < 200; i++ {
		size := 500*1024 + rand.Intn(4500*1024)
		data := make([]byte, size)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("medium_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Created 200 medium files (500KB-5MB)")

	// 50 large files (5MB - 50MB)
	for i := 0; i < 50; i++ {
		size := 5*1024*1024 + rand.Intn(45*1024*1024)
		data := make([]byte, size)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("large_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Created 50 large files (5MB-50MB)")

	// 10 huge files (50MB - 200MB)
	for i := 0; i < 10; i++ {
		size := 50*1024*1024 + rand.Intn(150*1024*1024)
		data := make([]byte, size)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("huge_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Created 10 huge files (50MB-200MB)")

	// Calculate total size
	var totalSize int64
	filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	b.Logf("Setup completed: 1760 files, %.2f GB total", float64(totalSize)/(1024*1024*1024))

	// Create S3 client if integration testing is enabled
	s3Client := createS3Client(b)
	testBucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if testBucket == "" {
		testBucket = "test-bucket"
	}

	if s3Client != nil {
		b.Logf("Using REAL AWS S3 for mixed workload benchmark (bucket: %s)", testBucket)
	} else {
		b.Logf("Using SIMULATED uploader (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 for real S3)")
	}

	config := &PipelineConfig{
		ScannerWorkers:    4,
		ArchiverWorkers:   8,
		UploaderWorkers:   4,
		ChunkBufferSize:   100,
		ArchiveBufferSize: 50,
		ResultBufferSize:  50,
		EnableProgress:    false,
		UseRealS3:         s3Client != nil,
		S3Client:          s3Client,
		S3Bucket:          testBucket,
		S3Region:          os.Getenv("AWS_REGION"),
		ChunkingConfig: &chunking.ChunkingConfig{
			TargetChunkSize: 100 * 1024 * 1024, // 100MB chunks
			MinChunkSize:    10 * 1024 * 1024,  // 10MB min
			MaxChunkSize:    500 * 1024 * 1024, // 500MB max
		},
	}

	pipeline, err := NewPipeline(config)
	if err != nil {
		b.Fatalf("Failed to create pipeline: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		result, err := pipeline.Run(ctx, testDir)
		if err != nil {
			b.Fatalf("Pipeline failed: %v", err)
		}

		b.StopTimer()

		// Log detailed results
		b.Logf("\n=== Mixed Workload Benchmark Results ===")
		b.Logf("Files:        %d", result.TotalFiles)
		b.Logf("Total Size:   %.2f GB", float64(result.TotalBytes)/(1024*1024*1024))
		b.Logf("Chunks:       %d", result.ChunksCreated)
		b.Logf("Duration:     %v", result.TotalTime)
		b.Logf("Throughput:   %.2f MB/s", float64(result.TotalBytes)/(1024*1024)/result.TotalTime.Seconds())

		// Cleanup S3 objects if using real S3
		if s3Client != nil && result.ChunksCreated > 0 {
			b.Logf("Cleaning up %d S3 objects...", result.ChunksCreated)
		}

		b.StartTimer()
	}

	// Cleanup AWS SDK HTTP connections (Issue #72)
	// Close idle connections to prevent goroutine leaks in goleak tests
	if s3Client != nil {
		if httpClient, ok := s3Client.Options().HTTPClient.(*http.Client); ok {
			httpClient.CloseIdleConnections()
			b.Logf("Closed AWS SDK HTTP idle connections")
		}
	}
}

// BenchmarkPipeline_BurstyPattern benchmarks the pipeline with bursty file arrival patterns
func BenchmarkPipeline_BurstyPattern(b *testing.B) {
	testDir := b.TempDir()

	b.Logf("Creating bursty pattern workload...")

	// Burst 1: 2000 files of 10KB
	for i := 0; i < 2000; i++ {
		data := make([]byte, 10*1024)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("burst1_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Burst 1: 2000 files @ 10KB")

	// Quiet period: 100 files of 5KB
	for i := 0; i < 100; i++ {
		data := make([]byte, 5*1024)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("quiet1_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Quiet 1: 100 files @ 5KB")

	// Burst 2: 3000 files of 15KB
	for i := 0; i < 3000; i++ {
		data := make([]byte, 15*1024)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("burst2_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Burst 2: 3000 files @ 15KB")

	// Quiet period: 50 files of 1MB
	for i := 0; i < 50; i++ {
		data := make([]byte, 1024*1024)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("quiet2_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Quiet 2: 50 files @ 1MB")

	// Burst 3: 4000 files of 8KB
	for i := 0; i < 4000; i++ {
		data := make([]byte, 8*1024)
		rand.Read(data)
		path := filepath.Join(testDir, fmt.Sprintf("burst3_%d.dat", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}
	b.Logf("  Burst 3: 4000 files @ 8KB")

	// Calculate total size
	var totalSize int64
	filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	b.Logf("Setup completed: 9150 files, %.2f MB total", float64(totalSize)/(1024*1024))

	// Create S3 client if integration testing is enabled
	s3Client := createS3Client(b)
	testBucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if testBucket == "" {
		testBucket = "test-bucket"
	}

	if s3Client != nil {
		b.Logf("Using REAL AWS S3 for bursty pattern benchmark (bucket: %s)", testBucket)
	} else {
		b.Logf("Using SIMULATED uploader (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 for real S3)")
	}

	config := &PipelineConfig{
		ScannerWorkers:    4,
		ArchiverWorkers:   8,
		UploaderWorkers:   4,
		ChunkBufferSize:   100,
		ArchiveBufferSize: 50,
		ResultBufferSize:  50,
		EnableProgress:    false,
		UseRealS3:         s3Client != nil,
		S3Client:          s3Client,
		S3Bucket:          testBucket,
		S3Region:          os.Getenv("AWS_REGION"),
		ChunkingConfig: &chunking.ChunkingConfig{
			TargetChunkSize: 50 * 1024 * 1024, // 50MB chunks
			MinChunkSize:    10 * 1024 * 1024, // 10MB min
			MaxChunkSize:    100 * 1024 * 1024, // 100MB max
		},
	}

	pipeline, err := NewPipeline(config)
	if err != nil {
		b.Fatalf("Failed to create pipeline: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		result, err := pipeline.Run(ctx, testDir)
		if err != nil {
			b.Fatalf("Pipeline failed: %v", err)
		}

		b.StopTimer()

		// Log detailed results
		b.Logf("\n=== Bursty Pattern Benchmark Results ===")
		b.Logf("Files:        %d", result.TotalFiles)
		b.Logf("Total Size:   %.2f MB", float64(result.TotalBytes)/(1024*1024))
		b.Logf("Chunks:       %d", result.ChunksCreated)
		b.Logf("Duration:     %v", result.TotalTime)
		b.Logf("Throughput:   %.2f MB/s", float64(result.TotalBytes)/(1024*1024)/result.TotalTime.Seconds())

		// Cleanup S3 objects if using real S3
		if s3Client != nil && result.ChunksCreated > 0 {
			b.Logf("Cleaning up %d S3 objects...", result.ChunksCreated)
		}

		b.StartTimer()
	}

	// Cleanup AWS SDK HTTP connections (Issue #72)
	// Close idle connections to prevent goroutine leaks in goleak tests
	if s3Client != nil {
		if httpClient, ok := s3Client.Options().HTTPClient.(*http.Client); ok {
			httpClient.CloseIdleConnections()
			b.Logf("Closed AWS SDK HTTP idle connections")
		}
	}
}

