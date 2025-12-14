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

// BenchmarkPipeline_LargeScale_MultiPrefix is a comprehensive large-scale benchmark
// designed to properly exercise S3 partition-level parallelism.
//
// This benchmark uses 50,000 files @ 1GB total to create enough load to demonstrate
// the true benefit of multi-prefix parallel uploads. The small test (5,000 @ 100MB)
// was too small to show the expected 5-8x improvement.
//
// Run with (SINGLE TEST ONLY - DO NOT RUN MULTIPLE BENCHMARKS CONCURRENTLY):
//
//	CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 CARGOSHIP_TEST_BUCKET=cargoship-pipeline-test \
//	  AWS_PROFILE=aws AWS_REGION=us-west-2 go test -v -tags=benchmark \
//	  -run='^$' -bench=BenchmarkPipeline_LargeScale_MultiPrefix -benchtime=1x -timeout=60m ./pkg/pipeline
func BenchmarkPipeline_LargeScale_MultiPrefix(b *testing.B) {
	// CRITICAL: Ensure no other benchmarks are running concurrently
	// Concurrent benchmarks will invalidate results
	ensureNoConcurrentBenchmarks(b)

	// Check if using real S3 (REQUIRED for this benchmark)
	useRealS3 := os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != ""
	if !useRealS3 {
		b.Skip("Skipping large-scale multi-prefix benchmark: requires CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1")
	}

	// Create test directory with large-scale workload (50,000 files @ 20KB = 1GB)
	// This is large enough to properly exercise S3 partition-level parallelism
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-large-multiprefix-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		b.Logf("Cleaning up test directory: %s", tmpDir)
		_ = os.RemoveAll(tmpDir)
	}()

	b.Logf("Creating 50,000 test files (1GB total)...")
	b.Logf("This will take 2-3 minutes...")
	startSetup := time.Now()

	// Create 50,000 files @ 20KB each = 1GB total
	fileSize := 20 * 1024 // 20KB
	for i := 0; i < 50000; i++ {
		content := make([]byte, fileSize)
		// Fill with some data (not random, compresses well)
		for j := range content {
			content[j] = byte((i + j) % 256)
		}

		filename := filepath.Join(tmpDir, fmt.Sprintf("file-%06d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}

		if i > 0 && i%5000 == 0 {
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

	// Test cases: Phase 2 (single-prefix baseline) vs Phase 3.1 (multi-prefix with router) vs Phase 3.2 (archiver sharding)
	testCases := []struct {
		name                   string
		enableMultiPrefix      bool
		enableArchiverSharding bool
		shardCount             int
		workersPerPrefix       int
	}{
		{
			name:                   "Phase2_SinglePrefix_Baseline",
			enableMultiPrefix:      false,
			enableArchiverSharding: false,
			shardCount:             0,
			workersPerPrefix:       0,
		},
		{
			name:                   "Phase3.1_MultiPrefix_8shards",
			enableMultiPrefix:      true,
			enableArchiverSharding: false,
			shardCount:             8,
			workersPerPrefix:       2,
		},
		{
			name:                   "Phase3.2_ArchiverSharding_8shards",
			enableMultiPrefix:      true,
			enableArchiverSharding: true,
			shardCount:             8,
			workersPerPrefix:       2,
		},
	}

	// Store results for comparison
	type BenchmarkResult struct {
		Name           string
		Duration       time.Duration
		ThroughputMBps float64
		ChunksCreated  int
		MemoryUsedMB   float64
		FilesProcessed int64
		BytesProcessed int64
	}
	var results []BenchmarkResult

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.Logf("\n========================================")
			b.Logf("=== %s ===", tc.name)
			b.Logf("========================================\n")

			config := &PipelineConfig{
				ScannerWorkers:         4,
				ArchiverWorkers:        8,
				UploaderWorkers:        4,
				S3Bucket:               bucket,
				S3Prefix:               fmt.Sprintf("bench-large-scale-%s-%d", tc.name, time.Now().Unix()),
				EnableProgress:         false, // Disable for cleaner benchmark output
				UseRealS3:              true,
				S3Client:               s3Client,
				S3PartSize:             64 * 1024 * 1024, // 64MB parts
				EnableMultiPrefix:      tc.enableMultiPrefix,
				EnableArchiverSharding: tc.enableArchiverSharding,
				ShardCount:             tc.shardCount,
				WorkersPerPrefix:       tc.workersPerPrefix,
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

				b.Logf("Starting pipeline run...")
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
					Name:           tc.name,
					Duration:       duration,
					ThroughputMBps: throughputMBps,
					ChunksCreated:  result.ChunksCreated,
					MemoryUsedMB:   memUsedMB,
					FilesProcessed: result.TotalFiles,
					BytesProcessed: result.TotalBytes,
				})

				// Report results
				b.Logf("\n=== %s Results ===", tc.name)
				b.Logf("Files:        %d", result.TotalFiles)
				b.Logf("Total Size:   %.2f GB (%.2f MB)", float64(result.TotalBytes)/(1024*1024*1024), float64(result.TotalBytes)/(1024*1024))
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
				b.Logf("\nCleaning up S3 objects...")
				ctx = context.Background()
				listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
					Bucket: &config.S3Bucket,
					Prefix: &config.S3Prefix,
				})
				if err == nil && listResult.Contents != nil {
					b.Logf("Deleting %d S3 objects...", len(listResult.Contents))
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
		b.Logf("=== Large-Scale Phase 2 vs Phase 3.1 Comparison ===")
		b.Logf("========================================\n")

		if len(results) == 0 {
			b.Log("No results to compare")
			return
		}

		// Find Phase 2 baseline
		var baseline *BenchmarkResult
		for i := range results {
			if results[i].Name == "Phase2_SinglePrefix_Baseline" {
				baseline = &results[i]
				break
			}
		}

		if baseline == nil {
			b.Log("Phase 2 baseline not found")
			return
		}

		b.Logf("Phase 2 (Single-Prefix Baseline):")
		b.Logf("  Files:       %d", baseline.FilesProcessed)
		b.Logf("  Total Size:  %.2f GB", float64(baseline.BytesProcessed)/(1024*1024*1024))
		b.Logf("  Duration:    %v (%.2fs)", baseline.Duration, baseline.Duration.Seconds())
		b.Logf("  Throughput:  %.2f MB/s", baseline.ThroughputMBps)
		b.Logf("  Chunks:      %d", baseline.ChunksCreated)
		b.Logf("  Memory:      %.2f MB\n", baseline.MemoryUsedMB)

		// Compare Phase 3.1 configuration
		for i := range results {
			if results[i].Name == "Phase2_SinglePrefix_Baseline" {
				continue
			}

			result := results[i]
			speedup := result.ThroughputMBps / baseline.ThroughputMBps
			timeReduction := (baseline.Duration.Seconds() - result.Duration.Seconds()) / baseline.Duration.Seconds() * 100

			b.Logf("%s:", result.Name)
			b.Logf("  Files:       %d", result.FilesProcessed)
			b.Logf("  Total Size:  %.2f GB", float64(result.BytesProcessed)/(1024*1024*1024))
			b.Logf("  Duration:    %v (%.2fs, %.1f%% faster)", result.Duration, result.Duration.Seconds(), timeReduction)
			b.Logf("  Throughput:  %.2f MB/s (%.2fx speedup)", result.ThroughputMBps, speedup)
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

		b.Logf("\n=== Conclusion ===")
		if len(results) >= 2 {
			phase3Result := results[len(results)-1]
			speedup := phase3Result.ThroughputMBps / baseline.ThroughputMBps

			if speedup >= 5.0 {
				b.Logf("Phase 3.1 Multi-Prefix Parallel Upload is VALIDATED (%.2fx speedup)", speedup)
			} else if speedup >= 3.0 {
				b.Logf("Phase 3.1 shows improvement (%.2fx) but below 5x target", speedup)
				b.Logf("Consider increasing test size or shard count for better results")
			} else {
				b.Logf("Phase 3.1 improvement (%.2fx) is below expectations", speedup)
				b.Logf("Further optimization or investigation required")
			}
		}
	})
}
