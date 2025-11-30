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
)

// BenchmarkPipeline_MultiPrefixComparison compares Phase 2 (single-prefix) vs Phase 3.1 (multi-prefix)
// to validate the 5-8x throughput improvement claim.
//
// This benchmark REQUIRES real AWS S3 to accurately measure the partition-level parallelism effect.
//
// Run with:
// CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 CARGOSHIP_TEST_BUCKET=cargoship-pipeline-test \
//   AWS_PROFILE=aws AWS_REGION=us-west-2 go test -tags=benchmark \
//   -bench=BenchmarkPipeline_MultiPrefixComparison -benchtime=1x -timeout=30m ./pkg/pipeline
func BenchmarkPipeline_MultiPrefixComparison(b *testing.B) {
	// CRITICAL: Ensure no other benchmarks are running concurrently
	// Concurrent benchmarks will invalidate results
	ensureNoConcurrentBenchmarks(b)

	// Check if using real S3 (REQUIRED for this benchmark)
	useRealS3 := os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != ""
	if !useRealS3 {
		b.Skip("Skipping multi-prefix comparison: requires CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1")
	}

	// Create test directory with moderate workload (5,000 files @ 20KB = 100MB)
	// This is large enough to show throughput differences but not too large for quick iteration
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-multiprefix-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	b.Logf("Creating 5,000 test files (100MB total)...")
	startSetup := time.Now()

	// Create 5,000 files @ 20KB each = 100MB total
	fileSize := 20 * 1024 // 20KB
	for i := 0; i < 5000; i++ {
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

	// Get AWS credentials
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

	// Test cases: Phase 2 (single-prefix) vs Phase 3.1 (multi-prefix)
	testCases := []struct {
		name              string
		enableMultiPrefix bool
		shardCount        int
		workersPerPrefix  int
	}{
		{
			name:              "Phase2_SinglePrefix",
			enableMultiPrefix: false,
			shardCount:        0,
			workersPerPrefix:  0,
		},
		{
			name:              "Phase3.1_MultiPrefix_4shards",
			enableMultiPrefix: true,
			shardCount:        4,
			workersPerPrefix:  2,
		},
		{
			name:              "Phase3.1_MultiPrefix_8shards",
			enableMultiPrefix: true,
			shardCount:        8,
			workersPerPrefix:  2,
		},
		{
			name:              "Phase3.1_MultiPrefix_16shards",
			enableMultiPrefix: true,
			shardCount:        16,
			workersPerPrefix:  2,
		},
	}

	// Store results for comparison
	type BenchmarkResult struct {
		Name         string
		Duration     time.Duration
		ThroughputMB float64
		ChunksCreated int
		MemoryUsedMB float64
	}
	var results []BenchmarkResult

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			config := &PipelineConfig{
				ScannerWorkers:    4,
				ArchiverWorkers:   8,
				UploaderWorkers:   4,
				S3Bucket:          bucket,
				S3Prefix:          fmt.Sprintf("bench-multiprefix-%s-%d", tc.name, time.Now().Unix()),
				EnableProgress:    false, // Disable for cleaner benchmark output
				UseRealS3:         true,
				S3Client:          s3Client,
				S3PartSize:        64 * 1024 * 1024, // 64MB parts
				EnableMultiPrefix: tc.enableMultiPrefix,
				ShardCount:        tc.shardCount,
				WorkersPerPrefix:  tc.workersPerPrefix,
			}

			// Measure memory AFTER AWS SDK initialization
			runtime.GC()
			runtime.GC()
			time.Sleep(10 * time.Millisecond)

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

				// Calculate metrics
				duration := result.TotalTime
				throughputMBps := float64(result.TotalBytes) / duration.Seconds() / (1024 * 1024)
				memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)

				// Store result
				results = append(results, BenchmarkResult{
					Name:         tc.name,
					Duration:     duration,
					ThroughputMB: throughputMBps,
					ChunksCreated: result.ChunksCreated,
					MemoryUsedMB: memUsedMB,
				})

				// Report results
				b.Logf("\n=== %s Results ===", tc.name)
				b.Logf("Files:        %d", result.TotalFiles)
				b.Logf("Total Size:   %.2f MB", float64(result.TotalBytes)/(1024*1024))
				b.Logf("Chunks:       %d", result.ChunksCreated)
				b.Logf("Duration:     %v (%.2fs)", duration, duration.Seconds())
				b.Logf("Throughput:   %.2f MB/s", throughputMBps)
				b.Logf("Memory Used:  %.2f MB", memUsedMB)

				if tc.enableMultiPrefix {
					b.Logf("Multi-Prefix: %d shards × %d workers = %d total workers",
						tc.shardCount, tc.workersPerPrefix, tc.shardCount*tc.workersPerPrefix)
				} else {
					b.Logf("Single-Prefix: %d workers", config.UploaderWorkers)
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
		})
	}

	// After all benchmarks complete, print comparison summary
	b.Run("Summary", func(b *testing.B) {
		b.Logf("\n========================================")
		b.Logf("=== Phase 2 vs Phase 3.1 Comparison ===")
		b.Logf("========================================\n")

		if len(results) == 0 {
			b.Log("No results to compare")
			return
		}

		// Find Phase 2 baseline
		var baseline *BenchmarkResult
		for i := range results {
			if results[i].Name == "Phase2_SinglePrefix" {
				baseline = &results[i]
				break
			}
		}

		if baseline == nil {
			b.Log("Phase 2 baseline not found")
			return
		}

		b.Logf("Phase 2 (Single-Prefix Baseline):")
		b.Logf("  Duration:    %v (%.2fs)", baseline.Duration, baseline.Duration.Seconds())
		b.Logf("  Throughput:  %.2f MB/s", baseline.ThroughputMB)
		b.Logf("  Chunks:      %d", baseline.ChunksCreated)
		b.Logf("  Memory:      %.2f MB\n", baseline.MemoryUsedMB)

		// Compare Phase 3.1 configurations
		for i := range results {
			if results[i].Name == "Phase2_SinglePrefix" {
				continue
			}

			result := results[i]
			speedup := result.ThroughputMB / baseline.ThroughputMB
			timeReduction := (baseline.Duration.Seconds() - result.Duration.Seconds()) / baseline.Duration.Seconds() * 100

			b.Logf("%s:", result.Name)
			b.Logf("  Duration:    %v (%.2fs, %.1f%% faster)", result.Duration, result.Duration.Seconds(), timeReduction)
			b.Logf("  Throughput:  %.2f MB/s (%.2fx speedup)", result.ThroughputMB, speedup)
			b.Logf("  Chunks:      %d", result.ChunksCreated)
			b.Logf("  Memory:      %.2f MB (%.2fx vs baseline)\n", result.MemoryUsedMB, result.MemoryUsedMB/baseline.MemoryUsedMB)

			// Validate 5-8x improvement target
			if speedup >= 5.0 && speedup <= 8.0 {
				b.Logf("  ✅ PASS: %.2fx speedup is within 5-8x target range\n", speedup)
			} else if speedup > 8.0 {
				b.Logf("  ✅ EXCELLENT: %.2fx speedup exceeds 8x target!\n", speedup)
			} else if speedup >= 3.0 {
				b.Logf("  ⚠️  MARGINAL: %.2fx speedup is below 5x target but shows improvement\n", speedup)
			} else {
				b.Logf("  ❌ FAIL: %.2fx speedup is below 3x minimum expectation\n", speedup)
			}
		}

		// Find best Phase 3.1 configuration
		var bestPhase3 *BenchmarkResult
		var bestSpeedup float64
		for i := range results {
			if results[i].Name == "Phase2_SinglePrefix" {
				continue
			}
			speedup := results[i].ThroughputMB / baseline.ThroughputMB
			if speedup > bestSpeedup {
				bestSpeedup = speedup
				bestPhase3 = &results[i]
			}
		}

		if bestPhase3 != nil {
			b.Logf("\n=== Best Configuration ===")
			b.Logf("%s achieved %.2fx speedup", bestPhase3.Name, bestSpeedup)
			b.Logf("Duration: %v → %v (%.1f%% faster)",
				baseline.Duration, bestPhase3.Duration,
				(baseline.Duration.Seconds()-bestPhase3.Duration.Seconds())/baseline.Duration.Seconds()*100)
			b.Logf("Throughput: %.2f MB/s → %.2f MB/s",
				baseline.ThroughputMB, bestPhase3.ThroughputMB)
		}
	})
}

// BenchmarkPipeline_MultiPrefixScaling tests how throughput scales with shard count
func BenchmarkPipeline_MultiPrefixScaling(b *testing.B) {
	// CRITICAL: Ensure no other benchmarks are running concurrently
	// Concurrent benchmarks will invalidate results
	ensureNoConcurrentBenchmarks(b)

	// Check if using real S3 (REQUIRED for this benchmark)
	useRealS3 := os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != ""
	if !useRealS3 {
		b.Skip("Skipping multi-prefix scaling: requires CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1")
	}

	// Create test directory with moderate workload
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-scaling-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	b.Logf("Creating 2,000 test files (40MB total)...")
	fileSize := 20 * 1024 // 20KB
	for i := 0; i < 2000; i++ {
		content := make([]byte, fileSize)
		for j := range content {
			content[j] = byte((i + j) % 256)
		}
		filename := filepath.Join(tmpDir, fmt.Sprintf("file-%05d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}

	// Get AWS credentials
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		b.Fatalf("Failed to load AWS config: %v", err)
	}

	s3Client := s3.NewFromConfig(awsConfig)

	// Test different shard counts
	shardCounts := []int{1, 2, 4, 8, 16, 32}

	for _, shardCount := range shardCounts {
		b.Run(fmt.Sprintf("%d_shards", shardCount), func(b *testing.B) {
			config := &PipelineConfig{
				ScannerWorkers:    4,
				ArchiverWorkers:   8,
				UploaderWorkers:   4,
				S3Bucket:          bucket,
				S3Prefix:          fmt.Sprintf("bench-scaling-%d-%d", shardCount, time.Now().Unix()),
				EnableProgress:    false,
				UseRealS3:         true,
				S3Client:          s3Client,
				S3PartSize:        64 * 1024 * 1024,
				EnableMultiPrefix: shardCount > 1, // Disable multi-prefix for 1 shard
				ShardCount:        shardCount,
				WorkersPerPrefix:  2,
			}

			runtime.GC()
			runtime.GC()
			time.Sleep(10 * time.Millisecond)

			var memBefore runtime.MemStats
			runtime.ReadMemStats(&memBefore)

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
					b.Fatalf("Pipeline failed: %v", result.Errors)
				}

				b.StopTimer()

				var memAfter runtime.MemStats
				runtime.ReadMemStats(&memAfter)

				duration := result.TotalTime
				throughputMBps := float64(result.TotalBytes) / duration.Seconds() / (1024 * 1024)
				memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)

				b.Logf("\n=== %d Shards Results ===", shardCount)
				b.Logf("Total Workers:  %d (shards) × 2 (per-prefix) = %d", shardCount, shardCount*2)
				b.Logf("Duration:       %v (%.2fs)", duration, duration.Seconds())
				b.Logf("Throughput:     %.2f MB/s", throughputMBps)
				b.Logf("Memory:         %.2f MB", memUsedMB)
				b.Logf("Chunks Created: %d", result.ChunksCreated)

				// Cleanup S3 objects
				ctx = context.Background()
				listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
					Bucket: &config.S3Bucket,
					Prefix: &config.S3Prefix,
				})
				if err == nil && listResult.Contents != nil {
					for _, obj := range listResult.Contents {
						_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
							Bucket: &config.S3Bucket,
							Key:    obj.Key,
						})
					}
				}

				b.StartTimer()
			}
		})
	}
}
