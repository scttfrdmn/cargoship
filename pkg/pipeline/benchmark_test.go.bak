//go:build benchmark

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/goleak"
)

// BenchmarkPipeline_SmallFiles_10K benchmarks the Issue #52 small files scenario
// Target: 10,000 files @ 185MB total in ≤12s (target 10s ±20%)
//
// Run with simulated S3: go test -tags=benchmark -bench=BenchmarkPipeline_SmallFiles_10K -benchtime=1x -timeout=30m
// Run with REAL S3: CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 AWS_PROFILE=aws go test -tags=benchmark -bench=BenchmarkPipeline_SmallFiles_10K -benchtime=1x -timeout=30m
func BenchmarkPipeline_SmallFiles_10K(b *testing.B) {
	// Check if using real S3
	useRealS3 := os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != ""

	// Create test directory with 10,000 small files
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-10k-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	b.Logf("Creating 10,000 test files...")
	startSetup := time.Now()

	// Create 10,000 files @ ~18.5KB each = 185MB total
	fileSize := 18500 // 18.5KB
	for i := 0; i < 10000; i++ {
		content := make([]byte, fileSize)
		// Fill with some data (not random, compresses well)
		for j := range content {
			content[j] = byte((i + j) % 256)
		}

		filename := filepath.Join(tmpDir, fmt.Sprintf("file-%05d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}

		if i > 0 && i%1000 == 0 {
			b.Logf("  Created %d files...", i)
		}
	}

	setupDuration := time.Since(startSetup)
	b.Logf("Setup completed in %v", setupDuration)

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        "test-bucket",
		EnableProgress:  false, // Disable for cleaner benchmark output
	}

	// Configure real S3 if enabled
	if useRealS3 {
		b.Logf("Using REAL AWS S3 for benchmark")
		bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
		if bucket == "" {
			bucket = "cargoship-pipeline-test"
		}
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-west-2"
		}

		// Create AWS config
		awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(region),
		)
		if err != nil {
			b.Fatalf("Failed to load AWS config: %v", err)
		}

		// Create S3 client
		s3Client := s3.NewFromConfig(awsConfig)

		config.S3Bucket = bucket
		config.S3Prefix = fmt.Sprintf("benchmark-10k-%d", time.Now().Unix())
		config.UseRealS3 = true
		config.S3Client = s3Client
		config.S3PartSize = 64 * 1024 * 1024 // 64MB parts
	} else {
		b.Logf("Using SIMULATED uploader (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 for real S3)")
	}

	// Measure memory AFTER AWS SDK initialization to get accurate pipeline memory
	// Force GC to stabilize memory measurements and reduce variance
	runtime.GC()
	runtime.GC() // Run twice to ensure thorough collection
	time.Sleep(10 * time.Millisecond) // Allow finalizers to complete

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		result, err := pipeline.Run(ctx, tmpDir)
		if err != nil {
			b.Fatal(err)
		}

		if !result.Success {
			b.Fatalf("Pipeline failed: %v", result.Errors)
		}

		b.StopTimer()

		// Measure memory after
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		// Report results
		duration := result.TotalTime
		throughputMBps := float64(result.TotalBytes) / duration.Seconds() / (1024 * 1024)
		memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)
		peakMemMB := float64(memAfter.TotalAlloc) / (1024 * 1024)

		b.Logf("\n=== Small Files Benchmark Results ===")
		b.Logf("Files:        %d", result.TotalFiles)
		b.Logf("Total Size:   %.2f MB", float64(result.TotalBytes)/(1024*1024))
		b.Logf("Chunks:       %d", result.ChunksCreated)
		b.Logf("Duration:     %v", duration)
		b.Logf("Throughput:   %.2f MB/s", throughputMBps)
		b.Logf("Memory Used:  %.2f MB", memUsedMB)
		b.Logf("Peak Memory:  %.2f MB", peakMemMB)
		b.Logf("\n=== Target Validation ===")
		b.Logf("Target Time:  ≤12s (goal: 10s)")
		if duration.Seconds() <= 12 {
			b.Logf("✅ PASS: Completed in %.2fs", duration.Seconds())
		} else {
			b.Logf("❌ FAIL: Took %.2fs (%.2fx slower than target)", duration.Seconds(), duration.Seconds()/12.0)
		}
		b.Logf("Target Ops:   10-50 chunks (100x-1000x cost savings)")
		if result.ChunksCreated >= 10 && result.ChunksCreated <= 50 {
			b.Logf("✅ PASS: Created %d chunks", result.ChunksCreated)
			b.Logf("   Cost savings: %.0fx (10,000 files → %d chunks)", 10000.0/float64(result.ChunksCreated), result.ChunksCreated)
		} else {
			b.Logf("⚠️  WARN: Created %d chunks (outside 10-50 range)", result.ChunksCreated)
		}
		b.Logf("Target Memory: ≤500MB")
		if memUsedMB <= 500 {
			b.Logf("✅ PASS: Used %.2f MB", memUsedMB)
		} else {
			b.Logf("❌ FAIL: Used %.2f MB (%.2fx over target)", memUsedMB, memUsedMB/500.0)
		}

		// Get stage stats
		stats := pipeline.GetStats()
		b.Logf("\n=== Stage Breakdown ===")
		for name, stat := range stats {
			pct := float64(stat.TotalTime) / float64(duration) * 100
			b.Logf("%s: %v (%.1f%%)", name, stat.TotalTime, pct)
		}

		// Cleanup S3 objects if using real S3
		if useRealS3 {
			s3Client := config.S3Client.(*s3.Client)
			ctx := context.Background()
			listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
				Bucket: &config.S3Bucket,
				Prefix: &config.S3Prefix,
			})
			if err == nil && listResult.Contents != nil {
				b.Logf("Cleaning up %d S3 objects...", len(listResult.Contents))
				for _, obj := range listResult.Contents {
					_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
						Bucket: &config.S3Bucket,
						Key:    obj.Key,
					})
				}
			}
		}

		b.StartTimer()
	}
}

// BenchmarkPipeline_SmallFiles_Varied tests different file counts
func BenchmarkPipeline_SmallFiles_Varied(b *testing.B) {
	testCases := []struct {
		name      string
		fileCount int
		fileSize  int
	}{
		{"100_files", 100, 10 * 1024},       // 100 files @ 10KB = 1MB
		{"1000_files", 1000, 10 * 1024},     // 1,000 files @ 10KB = 10MB
		{"5000_files", 5000, 18 * 1024},     // 5,000 files @ 18KB = 90MB
		{"10000_files", 10000, 18500},       // 10,000 files @ 18.5KB = 185MB
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Create test directory
			tmpDir, err := os.MkdirTemp("", "pipeline-bench-*")
			if err != nil {
				b.Fatal(err)
			}
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			// Create files
			for i := 0; i < tc.fileCount; i++ {
				content := make([]byte, tc.fileSize)
				for j := range content {
					content[j] = byte((i + j) % 256)
				}
				filename := filepath.Join(tmpDir, fmt.Sprintf("file-%05d.dat", i))
				if err := os.WriteFile(filename, content, 0644); err != nil {
					b.Fatal(err)
				}
			}

			config := &PipelineConfig{
				ScannerWorkers:  4,
				ArchiverWorkers: 8,
				UploaderWorkers: 4,
				S3Bucket:        "test-bucket",
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				pipeline, err := NewPipeline(config)
				if err != nil {
					b.Fatal(err)
				}

				ctx := context.Background()
				result, err := pipeline.Run(ctx, tmpDir)
				if err != nil {
					b.Fatal(err)
				}

				if !result.Success {
					b.Fatalf("Pipeline failed")
				}

				throughputMBps := float64(result.TotalBytes) / result.TotalTime.Seconds() / (1024 * 1024)
				b.Logf("%s: %d files, %d chunks, %.2fs, %.2f MB/s",
					tc.name, result.TotalFiles, result.ChunksCreated,
					result.TotalTime.Seconds(), throughputMBps)
			}
		})
	}
}

// BenchmarkPipeline_WorkerScaling tests different worker configurations
func BenchmarkPipeline_WorkerScaling(b *testing.B) {
	// Create test directory with 1000 files
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-workers-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	for i := 0; i < 1000; i++ {
		content := make([]byte, 10*1024) // 10KB
		filename := filepath.Join(tmpDir, fmt.Sprintf("file-%04d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}

	testCases := []struct {
		name            string
		scannerWorkers  int
		archiverWorkers int
		uploaderWorkers int
	}{
		{"2_4_2", 2, 4, 2},
		{"4_8_4", 4, 8, 4},
		{"8_16_8", 8, 16, 8},
		{"16_32_16", 16, 32, 16},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			config := &PipelineConfig{
				ScannerWorkers:  tc.scannerWorkers,
				ArchiverWorkers: tc.archiverWorkers,
				UploaderWorkers: tc.uploaderWorkers,
				S3Bucket:        "test-bucket",
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				pipeline, err := NewPipeline(config)
				if err != nil {
					b.Fatal(err)
				}

				ctx := context.Background()
				result, err := pipeline.Run(ctx, tmpDir)
				if err != nil {
					b.Fatal(err)
				}

				stats := pipeline.GetStats()
				b.Logf("%s: %.2fs (scan: %.2fs, archive: %.2fs, upload: %.2fs)",
					tc.name,
					result.TotalTime.Seconds(),
					stats["scanner"].TotalTime.Seconds(),
					stats["archiver"].TotalTime.Seconds(),
					stats["uploader"].TotalTime.Seconds())
			}
		})
	}
}

// BenchmarkPipeline_MemoryProfile profiles memory usage
func BenchmarkPipeline_MemoryProfile(b *testing.B) {
	// Create test with various file sizes
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-memory-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create mix of file sizes
	// 100 small (10KB), 50 medium (100KB), 10 large (1MB)
	for i := 0; i < 100; i++ {
		content := make([]byte, 10*1024)
		filename := filepath.Join(tmpDir, fmt.Sprintf("small-%03d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		content := make([]byte, 100*1024)
		filename := filepath.Join(tmpDir, fmt.Sprintf("medium-%02d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		content := make([]byte, 1024*1024)
		filename := filepath.Join(tmpDir, fmt.Sprintf("large-%02d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        "test-bucket",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var memStart, memEnd runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&memStart)

		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		result, err := pipeline.Run(ctx, tmpDir)
		if err != nil {
			b.Fatal(err)
		}

		runtime.ReadMemStats(&memEnd)

		allocMB := float64(memEnd.TotalAlloc-memStart.TotalAlloc) / (1024 * 1024)
		peakMB := float64(memEnd.Sys) / (1024 * 1024)

		b.Logf("Files: %d, Duration: %v, Alloc: %.2f MB, Peak: %.2f MB",
			result.TotalFiles, result.TotalTime, allocMB, peakMB)
	}
}

// BenchmarkPipeline_LargeFiles_100_56GB benchmarks the Issue #55 large files scenario
// Target: 100 files @ 56GB total in ≤240s with ≤4GB memory and no OOM
//
// Requires: Pre-generated test data at /Volumes/External HD/benchmark-data/large-files
// (100 files totaling 56GB, generated by scripts/generate-test-data.sh)
//
// Run with REAL S3 only (simulated uploader will not validate memory behavior accurately):
// CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 AWS_PROFILE=aws AWS_REGION=us-west-2 \
//   go test -tags=benchmark -bench=BenchmarkPipeline_LargeFiles_100_56GB -benchtime=1x -timeout=60m ./pkg/pipeline
func BenchmarkPipeline_LargeFiles_100_56GB(b *testing.B) {
	// Ignore AWS SDK HTTP connection pool goroutines (known leak from persistent connections)
	defer goleak.VerifyNone(b,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)

	// Check if using real S3 (REQUIRED for this benchmark)
	useRealS3 := os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != ""
	if !useRealS3 {
		b.Skip("Skipping large files benchmark: requires CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1")
	}

	// Check if test data exists
	testDataDir := "/Volumes/External HD/benchmark-data/large-files"
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		b.Skipf("Skipping large files benchmark: test data not found at %s", testDataDir)
	}

	// Verify file count
	entries, err := os.ReadDir(testDataDir)
	if err != nil {
		b.Fatalf("Failed to read test data directory: %v", err)
	}
	fileCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			fileCount++
		}
	}
	if fileCount < 100 {
		b.Skipf("Skipping large files benchmark: expected 100 files, found %d", fileCount)
	}

	b.Logf("Using REAL AWS S3 for large files benchmark (100 files @ 56GB)")
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	// Create AWS config
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		b.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        bucket,
		S3Prefix:        fmt.Sprintf("benchmark-large-%d", time.Now().Unix()),
		EnableProgress:  false, // Disable for cleaner benchmark output
		UseRealS3:       true,
		S3Client:        s3Client,
		S3PartSize:      64 * 1024 * 1024, // 64MB parts
	}

	// Measure memory AFTER AWS SDK initialization to get accurate pipeline memory
	// Force GC to stabilize memory measurements and reduce variance
	runtime.GC()
	runtime.GC() // Run twice to ensure thorough collection
	time.Sleep(10 * time.Millisecond) // Allow finalizers to complete

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		result, err := pipeline.Run(ctx, testDataDir)
		if err != nil {
			b.Fatal(err)
		}

		if !result.Success {
			b.Fatalf("Pipeline failed: %v", result.Errors)
		}

		b.StopTimer()

		// Measure memory after
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		// Report results
		duration := result.TotalTime
		totalGB := float64(result.TotalBytes) / (1024 * 1024 * 1024)
		throughputMBps := float64(result.TotalBytes) / duration.Seconds() / (1024 * 1024)
		memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)
		peakMemMB := float64(memAfter.TotalAlloc) / (1024 * 1024)

		b.Logf("\n=== Large Files Benchmark Results (Issue #55) ===")
		b.Logf("Files:        %d", result.TotalFiles)
		b.Logf("Total Size:   %.2f GB", totalGB)
		b.Logf("Chunks:       %d", result.ChunksCreated)
		b.Logf("Duration:     %v", duration)
		b.Logf("Throughput:   %.2f MB/s", throughputMBps)
		b.Logf("Memory Used:  %.2f MB", memUsedMB)
		b.Logf("Peak Memory:  %.2f MB", peakMemMB)
		b.Logf("\n=== Issue #55 Target Validation ===")

		// Target 1: Duration ≤240s
		b.Logf("Target Time:  ≤240s (4 minutes)")
		if duration.Seconds() <= 240 {
			b.Logf("✅ PASS: Completed in %.2fs", duration.Seconds())
		} else {
			b.Logf("❌ FAIL: Took %.2fs (%.2fx over target)", duration.Seconds(), duration.Seconds()/240.0)
		}

		// Target 2: 100-200 chunks (50x-100x cost savings)
		b.Logf("Target Ops:   100-200 chunks (50x-100x cost savings)")
		if result.ChunksCreated >= 100 && result.ChunksCreated <= 200 {
			b.Logf("✅ PASS: Created %d chunks", result.ChunksCreated)
			b.Logf("   Cost savings: %.0fx (100 files → %d chunks)", 100.0/float64(result.ChunksCreated), result.ChunksCreated)
		} else {
			b.Logf("⚠️  WARN: Created %d chunks (outside 100-200 range)", result.ChunksCreated)
			b.Logf("   Cost savings: %.0fx (100 files → %d chunks)", 100.0/float64(result.ChunksCreated), result.ChunksCreated)
		}

		// Target 3: Memory ≤4GB
		b.Logf("Target Memory: ≤4GB (no OOM)")
		if memUsedMB <= 4096 {
			b.Logf("✅ PASS: Used %.2f MB (%.2f GB)", memUsedMB, memUsedMB/1024)
		} else {
			b.Logf("❌ FAIL: Used %.2f MB (%.2f GB, %.2fx over 4GB target)", memUsedMB, memUsedMB/1024, memUsedMB/4096.0)
		}

		// OOM validation
		b.Logf("OOM Check:    Pipeline completed without crashing")
		b.Logf("✅ PASS: No out-of-memory errors")

		// Memory scaling validation
		b.Logf("\n=== Memory Scaling Validation ===")
		b.Logf("Data Size:    %.2f GB", totalGB)
		b.Logf("Memory Used:  %.2f MB", memUsedMB)
		memRatio := memUsedMB / (totalGB * 1024)
		b.Logf("Memory Ratio: %.2f%% (memory / data size)", memRatio*100)
		if memRatio < 0.1 { // Less than 10% of data size
			b.Logf("✅ EXCELLENT: Memory bounded at %.2f%% of data size", memRatio*100)
		} else {
			b.Logf("⚠️  WARN: Memory is %.2f%% of data size (target <10%%)", memRatio*100)
		}

		// Get stage stats
		stats := pipeline.GetStats()
		b.Logf("\n=== Stage Breakdown ===")
		for name, stat := range stats {
			pct := float64(stat.TotalTime) / float64(duration) * 100
			b.Logf("%s: %v (%.1f%%)", name, stat.TotalTime, pct)
		}

		// Cleanup S3 objects
		ctx = context.Background()
		listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: &config.S3Bucket,
			Prefix: &config.S3Prefix,
		})
		if err == nil && listResult.Contents != nil {
			b.Logf("Cleaning up %d S3 objects...", len(listResult.Contents))
			for _, obj := range listResult.Contents {
				_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: &config.S3Bucket,
					Key:    obj.Key,
				})
			}
		}

		b.StartTimer()
	}
}
