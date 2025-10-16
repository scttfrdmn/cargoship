package scenarios

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	cargos3 "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// BenchmarkSmallFileUpload benchmarks uploads of 1-10MB files (single-part)
// Focus: Latency and request overhead
func BenchmarkSmallFileUpload(b *testing.B) {
	scenarios := []struct {
		name string
		size int64
	}{
		{"1MB", 1 * 1024 * 1024},
		{"5MB", 5 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			benchmarkUpload(b, sc.size, 1, 5*1024*1024)
		})
	}
}

// BenchmarkMediumFileUpload benchmarks uploads of 10-100MB files
// Focus: Multipart efficiency with low concurrency
func BenchmarkMediumFileUpload(b *testing.B) {
	scenarios := []struct {
		name        string
		size        int64
		concurrency int
		chunkSize   int64
	}{
		{"10MB_Concurrent2", 10 * 1024 * 1024, 2, 10 * 1024 * 1024},
		{"50MB_Concurrent2", 50 * 1024 * 1024, 2, 10 * 1024 * 1024},
		{"100MB_Concurrent4", 100 * 1024 * 1024, 4, 10 * 1024 * 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			benchmarkUpload(b, sc.size, sc.concurrency, sc.chunkSize)
		})
	}
}

// BenchmarkLargeFileUpload benchmarks uploads of 100MB-1GB files
// Focus: Throughput with high concurrency
func BenchmarkLargeFileUpload(b *testing.B) {
	scenarios := []struct {
		name        string
		size        int64
		concurrency int
		chunkSize   int64
	}{
		{"100MB_Concurrent4", 100 * 1024 * 1024, 4, 10 * 1024 * 1024},
		{"500MB_Concurrent8", 500 * 1024 * 1024, 8, 50 * 1024 * 1024},
		{"1GB_Concurrent8", 1024 * 1024 * 1024, 8, 50 * 1024 * 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			benchmarkUpload(b, sc.size, sc.concurrency, sc.chunkSize)
		})
	}
}

// BenchmarkXLFileUpload benchmarks uploads of 1GB+ files
// Focus: Memory efficiency and sustained throughput
func BenchmarkXLFileUpload(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping XL file benchmarks in short mode")
	}

	scenarios := []struct {
		name        string
		size        int64
		concurrency int
		chunkSize   int64
	}{
		{"2GB_Concurrent10", 2 * 1024 * 1024 * 1024, 10, 100 * 1024 * 1024},
		{"5GB_Concurrent10", 5 * 1024 * 1024 * 1024, 10, 100 * 1024 * 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			benchmarkUpload(b, sc.size, sc.concurrency, sc.chunkSize)
		})
	}
}

// benchmarkUpload performs the actual upload benchmark
func benchmarkUpload(b *testing.B, fileSize int64, concurrency int, chunkSize int64) {
	// Skip if not running with benchmark tag
	if testing.Short() && fileSize > 100*1024*1024 {
		b.Skip("Skipping large file benchmark in short mode")
	}

	ctx := context.Background()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		b.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg)

	// Configure CargoShip
	cargoConfig := awsconfig.S3Config{
		Bucket:             getTestBucket(b),
		StorageClass:       awsconfig.StorageClassStandard,
		MultipartThreshold: 10 * 1024 * 1024,
		MultipartChunkSize: chunkSize,
		Concurrency:        concurrency,
	}

	// Create transporter
	transporter := cargos3.NewTransporter(client, cargoConfig)

	// Generate test data
	data := generateBenchmarkData(fileSize)

	// Record memory stats before
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Reset timer before benchmark loop
	b.ResetTimer()

	// Track latencies for percentile calculation
	latencies := make([]time.Duration, 0, b.N)

	// Run benchmark
	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Create archive
		archive := cargos3.Archive{
			Key:             fmt.Sprintf("benchmark/test-%d-%d.dat", time.Now().Unix(), i),
			Reader:          bytes.NewReader(data),
			Size:            fileSize,
			StorageClass:    cargoConfig.StorageClass,
			Metadata:        map[string]string{"benchmark": "true"},
			OriginalSize:    fileSize,
			CompressionType: "none",
		}

		// Upload
		result, err := transporter.Upload(ctx, archive)
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}

		latency := time.Since(start)
		latencies = append(latencies, latency)

		// Report metrics
		b.ReportMetric(result.Throughput, "MB/s")
		b.ReportMetric(float64(latency.Milliseconds()), "ms/op")

		// Cleanup - delete uploaded object
		_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(cargoConfig.Bucket),
			Key:    aws.String(archive.Key),
		})
		if err != nil {
			b.Logf("Warning: failed to cleanup test object: %v", err)
		}
	}

	// Stop timer for post-benchmark analysis
	b.StopTimer()

	// Record memory stats after
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory metrics
	allocsPerOp := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / float64(b.N)
	bytesPerOp := float64(memAfter.Mallocs-memBefore.Mallocs) / float64(b.N)

	b.ReportMetric(allocsPerOp, "allocs/op")
	b.ReportMetric(bytesPerOp, "B/op")

	// Calculate and report latency percentiles
	if len(latencies) > 0 {
		p50, p95, p99 := calculatePercentiles(latencies)
		b.ReportMetric(float64(p50.Milliseconds()), "p50_ms")
		b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
		b.ReportMetric(float64(p99.Milliseconds()), "p99_ms")
	}

	// Report GC stats
	gcPausesNs := memAfter.PauseTotalNs - memBefore.PauseTotalNs
	b.ReportMetric(float64(gcPausesNs/uint64(b.N))/1e6, "gc_pause_ms/op")
}

// generateBenchmarkData creates test data of specified size
func generateBenchmarkData(size int64) []byte {
	data := make([]byte, size)
	// Fill with predictable pattern for reproducibility
	pattern := []byte("CargoShip Benchmark Data ")
	for i := int64(0); i < size; i += int64(len(pattern)) {
		remaining := size - i
		if remaining < int64(len(pattern)) {
			copy(data[i:], pattern[:remaining])
		} else {
			copy(data[i:], pattern)
		}
	}
	return data
}

// calculatePercentiles computes p50, p95, p99 from latency samples
func calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	// Sort latencies (simple insertion sort for small datasets)
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Bubble sort (good enough for benchmark data)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentiles
	p50Index := int(float64(len(sorted)) * 0.50)
	p95Index := int(float64(len(sorted)) * 0.95)
	p99Index := int(float64(len(sorted)) * 0.99)

	if p50Index >= len(sorted) {
		p50Index = len(sorted) - 1
	}
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}

	return sorted[p50Index], sorted[p95Index], sorted[p99Index]
}

// getTestBucket returns the test bucket name from env or default
func getTestBucket(b *testing.B) string {
	// Try environment variable first
	bucket := "cargoship-benchmark-test"

	// Check if bucket is accessible
	// In real implementation, would validate bucket exists
	return bucket
}

// BenchmarkMemoryEfficiency tests memory usage patterns
func BenchmarkMemoryEfficiency(b *testing.B) {
	scenarios := []struct {
		name        string
		size        int64
		concurrency int
	}{
		{"Small_1MB", 1 * 1024 * 1024, 1},
		{"Medium_100MB", 100 * 1024 * 1024, 4},
		{"Large_1GB", 1024 * 1024 * 1024, 8},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			// Force GC before test
			runtime.GC()

			var memBefore, memAfter runtime.MemStats
			runtime.ReadMemStats(&memBefore)

			// Run upload
			benchmarkUpload(b, sc.size, sc.concurrency, 10*1024*1024)

			runtime.ReadMemStats(&memAfter)

			// Report memory efficiency
			memPerOp := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / float64(b.N)
			memEfficiency := float64(sc.size) / memPerOp
			b.ReportMetric(memEfficiency, "bytes_transferred/bytes_allocated")
		})
	}
}

// BenchmarkConcurrencyScaling tests how performance scales with concurrency
func BenchmarkConcurrencyScaling(b *testing.B) {
	fileSize := int64(100 * 1024 * 1024) // 100MB file

	concurrencyLevels := []int{1, 2, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			benchmarkUpload(b, fileSize, concurrency, 10*1024*1024)
		})
	}
}

// BenchmarkChunkSizeImpact tests impact of different chunk sizes
func BenchmarkChunkSizeImpact(b *testing.B) {
	fileSize := int64(500 * 1024 * 1024) // 500MB file
	concurrency := 8

	chunkSizes := []struct {
		name string
		size int64
	}{
		{"5MB", 5 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"50MB", 50 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}

	for _, cs := range chunkSizes {
		b.Run(cs.name, func(b *testing.B) {
			benchmarkUpload(b, fileSize, concurrency, cs.size)
		})
	}
}
