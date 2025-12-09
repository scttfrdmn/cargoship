// +build benchmark

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// S3 integration test helper functions

// enableS3IntegrationTests checks if S3 integration tests are enabled
func enableS3IntegrationTests() bool {
	return os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") == "1"
}

// getTestBucket returns the test S3 bucket name from environment
func getTestBucket() string {
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		return "cargoship-pipeline-test"
	}
	return bucket
}

// getTestRegion returns the test AWS region from environment
func getTestRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		return "us-west-2"
	}
	return region
}

// getTestS3Client creates an S3 client for testing
func getTestS3Client(tb testing.TB) *s3.Client {
	tb.Helper()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(getTestRegion()))
	if err != nil {
		tb.Fatalf("Failed to load AWS config: %v", err)
	}

	return s3.NewFromConfig(cfg)
}

// Phase3_3Metrics tracks Phase 3.3 performance metrics
type Phase3_3Metrics struct {
	TotalTime              time.Duration
	AvgChunkSizeCompressed int64
	ChunkSizeStdDev        float64 // Lower is better (more uniform)
	TotalChunks            int
	ChunksWithPadding      int
	TotalPaddingBytes      int64
	PaddingRatio           float64
	S3RequestCount         int
	EstimatedAPICost       float64
}

// BenchmarkPipeline_Phase3_3_SmallFiles tests Phase 3.3 with 10K small files
// Expected: ~25% faster than Phase 3.1 baseline due to uniform chunk sizes
func BenchmarkPipeline_Phase3_3_SmallFiles(b *testing.B) {
	if !enableS3IntegrationTests() {
		b.Skip("S3 integration tests disabled (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1)")
	}

	// Setup test data directory
	testDataDir := filepath.Join(b.TempDir(), "test-data")
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		b.Fatalf("Failed to create test data dir: %v", err)
	}

	// Create 10,000 small files (same as Phase 3.1 baseline)
	b.Logf("Creating 10,000 test files...")
	fileCount := 10000
	avgFileSize := 20 * 1024 // 20KB average

	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join(testDataDir, fmt.Sprintf("file-%05d.txt", i))
		data := make([]byte, avgFileSize)
		for j := range data {
			data[j] = byte(i % 256)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		// Create pipeline with Phase 3.3 optimizations ENABLED
		config := &PipelineConfig{
			ScannerWorkers:  2,
			ArchiverWorkers: 8,
			UploaderWorkers: 16,

			ChunkBufferSize:   100,
			ArchiveBufferSize: 100,
			ResultBufferSize:  100,

			S3Bucket: getTestBucket(),
			S3Prefix: fmt.Sprintf("phase3.3-benchmark-%d", time.Now().Unix()),
			S3Region: getTestRegion(),

			UseRealS3:         true,
			S3Client:          getTestS3Client(b),
			EnableMultiPrefix: true,
			ShardCount:        8,
			WorkersPerPrefix:  2,

			// Phase 3.3: Enable compressed-aware chunking and padding
			EnableCompressedAwareChunking: true,
			EnableArchivePadding:          true,
			MaxPaddingRatio:               0.25,
			ForceChunkSizeMB:              0, // Use adaptive sizing

			ChunkingConfig: &chunking.ChunkingConfig{
				TargetChunkSize: 100 * 1024 * 1024, // 100MB target
				MinChunkSize:    50 * 1024 * 1024,
				MaxChunkSize:    200 * 1024 * 1024,
			},

			EnableProgress:   false,
			ProgressInterval: 1 * time.Second,
		}

		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatalf("Failed to create pipeline: %v", err)
		}

		ctx := context.Background()
		startTime := time.Now()

		result, err := pipeline.Run(ctx, testDataDir)
		if err != nil {
			b.Fatalf("Pipeline failed: %v", err)
		}

		duration := time.Since(startTime)

		// Collect Phase 3.3 metrics
		metrics := collectPhase3_3Metrics(b, pipeline, result, duration)

		b.Logf("Phase 3.3 Results:")
		b.Logf("  Total time: %v", metrics.TotalTime)
		b.Logf("  Chunks created: %d", metrics.TotalChunks)
		b.Logf("  Chunks with padding: %d", metrics.ChunksWithPadding)
		b.Logf("  Total padding: %.2f MB (%.2f%% overhead)",
			float64(metrics.TotalPaddingBytes)/(1024*1024),
			metrics.PaddingRatio*100)
		b.Logf("  Avg chunk size (compressed): %.2f MB",
			float64(metrics.AvgChunkSizeCompressed)/(1024*1024))
		b.Logf("  Chunk size std dev: %.2f MB", metrics.ChunkSizeStdDev/(1024*1024))
		b.Logf("  S3 requests: %d", metrics.S3RequestCount)

		// Cleanup
		if err := pipeline.Stop(); err != nil {
			b.Logf("Warning: pipeline stop error: %v", err)
		}
	}
}

// BenchmarkPipeline_Phase3_3_MixedFiles tests Phase 3.3 with mixed workload
// Tests adaptive sizing with varied compression ratios
func BenchmarkPipeline_Phase3_3_MixedFiles(b *testing.B) {
	if !enableS3IntegrationTests() {
		b.Skip("S3 integration tests disabled (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1)")
	}

	// Setup test data directory
	testDataDir := filepath.Join(b.TempDir(), "mixed-data")
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		b.Fatalf("Failed to create test data dir: %v", err)
	}

	// Create mixed workload: text files (highly compressible),
	// pseudo-random data (less compressible), and pre-compressed files
	b.Logf("Creating mixed workload...")

	// 1000 text files (high compression ratio ~80%)
	textDir := filepath.Join(testDataDir, "text")
	if err := os.MkdirAll(textDir, 0755); err != nil {
		b.Fatalf("Failed to create text dir: %v", err)
	}
	for i := 0; i < 1000; i++ {
		filePath := filepath.Join(textDir, fmt.Sprintf("text-%04d.txt", i))
		// Repetitive text compresses well
		data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")
		repeated := make([]byte, 0, 50*1024)
		for len(repeated) < 50*1024 {
			repeated = append(repeated, data...)
		}
		if err := os.WriteFile(filePath, repeated[:50*1024], 0644); err != nil {
			b.Fatalf("Failed to create text file: %v", err)
		}
	}

	// 500 binary files (medium compression ratio ~50%)
	binaryDir := filepath.Join(testDataDir, "binary")
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		b.Fatalf("Failed to create binary dir: %v", err)
	}
	for i := 0; i < 500; i++ {
		filePath := filepath.Join(binaryDir, fmt.Sprintf("binary-%04d.dat", i))
		data := make([]byte, 100*1024)
		for j := range data {
			data[j] = byte(j % 256)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatalf("Failed to create binary file: %v", err)
		}
	}

	// 100 large files (varied compression ratios)
	largeDir := filepath.Join(testDataDir, "large")
	if err := os.MkdirAll(largeDir, 0755); err != nil {
		b.Fatalf("Failed to create large dir: %v", err)
	}
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(largeDir, fmt.Sprintf("large-%04d.bin", i))
		data := make([]byte, 1024*1024) // 1MB each
		for j := range data {
			data[j] = byte((i + j) % 256)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatalf("Failed to create large file: %v", err)
		}
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		config := &PipelineConfig{
			ScannerWorkers:  2,
			ArchiverWorkers: 8,
			UploaderWorkers: 16,

			ChunkBufferSize:   100,
			ArchiveBufferSize: 100,
			ResultBufferSize:  100,

			S3Bucket: getTestBucket(),
			S3Prefix: fmt.Sprintf("phase3.3-mixed-%d", time.Now().Unix()),
			S3Region: getTestRegion(),

			UseRealS3:         true,
			S3Client:          getTestS3Client(b),
			EnableMultiPrefix: true,
			ShardCount:        8,
			WorkersPerPrefix:  2,

			// Phase 3.3: Adaptive sizing should handle diversity
			EnableCompressedAwareChunking: true,
			EnableArchivePadding:          true,
			MaxPaddingRatio:               0.25,
			ForceChunkSizeMB:              0,

			ChunkingConfig: &chunking.ChunkingConfig{
				TargetChunkSize: 100 * 1024 * 1024,
				MinChunkSize:    50 * 1024 * 1024,
				MaxChunkSize:    200 * 1024 * 1024,
			},

			EnableProgress:   false,
			ProgressInterval: 1 * time.Second,
		}

		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatalf("Failed to create pipeline: %v", err)
		}

		ctx := context.Background()
		startTime := time.Now()

		result, err := pipeline.Run(ctx, testDataDir)
		if err != nil {
			b.Fatalf("Pipeline failed: %v", err)
		}

		duration := time.Since(startTime)
		metrics := collectPhase3_3Metrics(b, pipeline, result, duration)

		b.Logf("Phase 3.3 Mixed Workload Results:")
		b.Logf("  Total time: %v", metrics.TotalTime)
		b.Logf("  Chunks: %d (padding: %d)", metrics.TotalChunks, metrics.ChunksWithPadding)
		b.Logf("  Padding overhead: %.2f%%", metrics.PaddingRatio*100)
		b.Logf("  Chunk size variance: %.2f MB", metrics.ChunkSizeStdDev/(1024*1024))

		if err := pipeline.Stop(); err != nil {
			b.Logf("Warning: pipeline stop error: %v", err)
		}
	}
}

// BenchmarkPipeline_Phase3_3_vs_Baseline compares Phase 3.3 to baseline
func BenchmarkPipeline_Phase3_3_vs_Baseline(b *testing.B) {
	if !enableS3IntegrationTests() {
		b.Skip("S3 integration tests disabled (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1)")
	}

	// Setup test data
	testDataDir := filepath.Join(b.TempDir(), "baseline-test")
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		b.Fatalf("Failed to create test data dir: %v", err)
	}

	// Create 1000 files for quick comparison
	for i := 0; i < 1000; i++ {
		filePath := filepath.Join(testDataDir, fmt.Sprintf("file-%04d.txt", i))
		data := make([]byte, 50*1024) // 50KB each
		for j := range data {
			data[j] = byte(i % 256)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	b.Run("Baseline_NoOptimizations", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			config := &PipelineConfig{
				ScannerWorkers:  2,
				ArchiverWorkers: 8,
				UploaderWorkers: 16,

				ChunkBufferSize:   100,
				ArchiveBufferSize: 100,
				ResultBufferSize:  100,

				S3Bucket: getTestBucket(),
				S3Prefix: fmt.Sprintf("baseline-%d", time.Now().Unix()),
				S3Region: getTestRegion(),

				UseRealS3:         true,
				S3Client:          getTestS3Client(b),
				EnableMultiPrefix: true,
				ShardCount:        8,
				WorkersPerPrefix:  2,

				// Phase 3.3 optimizations DISABLED
				EnableCompressedAwareChunking: false,
				EnableArchivePadding:          false,

				ChunkingConfig: &chunking.ChunkingConfig{
					TargetChunkSize: 100 * 1024 * 1024,
					MinChunkSize:    50 * 1024 * 1024,
					MaxChunkSize:    200 * 1024 * 1024,
				},

				EnableProgress: false,
			}

			pipeline, err := NewPipeline(config)
			if err != nil {
				b.Fatalf("Failed to create pipeline: %v", err)
			}

			ctx := context.Background()
			startTime := time.Now()

			result, err := pipeline.Run(ctx, testDataDir)
			if err != nil {
				b.Fatalf("Pipeline failed: %v", err)
			}

			duration := time.Since(startTime)
			metrics := collectPhase3_3Metrics(b, pipeline, result, duration)

			b.Logf("Baseline: %v (chunk variance: %.2f MB)",
				duration, metrics.ChunkSizeStdDev/(1024*1024))

			if err := pipeline.Stop(); err != nil {
				b.Logf("Warning: pipeline stop error: %v", err)
			}
		}
	})

	b.Run("Phase3_3_FullOptimizations", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			config := &PipelineConfig{
				ScannerWorkers:  2,
				ArchiverWorkers: 8,
				UploaderWorkers: 16,

				ChunkBufferSize:   100,
				ArchiveBufferSize: 100,
				ResultBufferSize:  100,

				S3Bucket: getTestBucket(),
				S3Prefix: fmt.Sprintf("optimized-%d", time.Now().Unix()),
				S3Region: getTestRegion(),

				UseRealS3:         true,
				S3Client:          getTestS3Client(b),
				EnableMultiPrefix: true,
				ShardCount:        8,
				WorkersPerPrefix:  2,

				// Phase 3.3 optimizations ENABLED
				EnableCompressedAwareChunking: true,
				EnableArchivePadding:          true,
				MaxPaddingRatio:               0.25,
				ForceChunkSizeMB:              0,

				ChunkingConfig: &chunking.ChunkingConfig{
					TargetChunkSize: 100 * 1024 * 1024,
					MinChunkSize:    50 * 1024 * 1024,
					MaxChunkSize:    200 * 1024 * 1024,
				},

				EnableProgress: false,
			}

			pipeline, err := NewPipeline(config)
			if err != nil {
				b.Fatalf("Failed to create pipeline: %v", err)
			}

			ctx := context.Background()
			startTime := time.Now()

			result, err := pipeline.Run(ctx, testDataDir)
			if err != nil {
				b.Fatalf("Pipeline failed: %v", err)
			}

			duration := time.Since(startTime)
			metrics := collectPhase3_3Metrics(b, pipeline, result, duration)

			b.Logf("Phase 3.3: %v (chunk variance: %.2f MB, padding: %.2f%%)",
				duration, metrics.ChunkSizeStdDev/(1024*1024), metrics.PaddingRatio*100)

			if err := pipeline.Stop(); err != nil {
				b.Logf("Warning: pipeline stop error: %v", err)
			}
		}
	})
}

// collectPhase3_3Metrics collects Phase 3.3 performance metrics
func collectPhase3_3Metrics(b *testing.B, pipeline *Pipeline, result *Result, duration time.Duration) Phase3_3Metrics {
	metrics := Phase3_3Metrics{
		TotalTime:   duration,
		TotalChunks: result.ChunksCreated,
	}

	// Get archiver stats for padding information
	if pipeline.archiver != nil {
		paddingBytes := pipeline.archiver.GetPaddingStats()
		metrics.TotalPaddingBytes = paddingBytes

		if result.TotalBytes > 0 {
			metrics.PaddingRatio = float64(paddingBytes) / float64(result.TotalBytes)
		}
	}

	// Calculate chunk size statistics
	// Note: This is a placeholder - actual implementation would need to track
	// individual chunk sizes during pipeline execution
	if result.ChunksCreated > 0 {
		metrics.AvgChunkSizeCompressed = result.TotalBytes / int64(result.ChunksCreated)
		// StdDev calculation would require tracking individual chunk sizes
		metrics.ChunkSizeStdDev = 0 // TODO: Implement actual calculation
	}

	// Estimate S3 request count (uploads + metadata operations)
	metrics.S3RequestCount = result.ChunksCreated * 2 // Upload + metadata

	// Estimate API cost ($0.005 per 1000 requests)
	metrics.EstimatedAPICost = float64(metrics.S3RequestCount) * 0.005 / 1000.0

	return metrics
}

// validatePhase3_3Improvement validates that Phase 3.3 achieves performance goals
func validatePhase3_3Improvement(baseline, optimized Phase3_3Metrics) error {
	// 1. Check 25% upload time improvement
	improvement := 1.0 - (float64(optimized.TotalTime) / float64(baseline.TotalTime))
	if improvement < 0.25 {
		return fmt.Errorf("upload time improvement %.1f%% below 25%% target", improvement*100)
	}

	// 2. Check chunk size uniformity (lower std dev)
	if optimized.ChunkSizeStdDev >= baseline.ChunkSizeStdDev {
		return fmt.Errorf("chunk size variance not improved (baseline: %.2f, optimized: %.2f)",
			baseline.ChunkSizeStdDev/(1024*1024), optimized.ChunkSizeStdDev/(1024*1024))
	}

	// 3. Check padding ratio is acceptable (<25%)
	if optimized.PaddingRatio > 0.25 {
		return fmt.Errorf("padding ratio %.1f%% exceeds 25%% threshold", optimized.PaddingRatio*100)
	}

	return nil
}

// BenchmarkPipeline_Phase5_FileSplitting validates Phase 5 adaptive file splitting (Issue #69)
// Expected: 23% improvement over baseline (311s → <240s) with 100 files @ 56GB
func BenchmarkPipeline_Phase5_FileSplitting(b *testing.B) {
	if !enableS3IntegrationTests() {
		b.Skip("S3 integration tests disabled (set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1)")
	}

	// Check if test data exists
	testDataDir := "/Volumes/External HD/benchmark-data/large-files"
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		b.Skipf("Skipping Phase 5 benchmark: test data not found at %s", testDataDir)
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
		b.Skipf("Skipping Phase 5 benchmark: expected 100 files, found %d", fileCount)
	}

	b.Logf("Using REAL AWS S3 for Phase 5 benchmark (100 files @ 56GB with file splitting)")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		config := &PipelineConfig{
			ScannerWorkers:  4,
			ArchiverWorkers: 8,
			UploaderWorkers: 8, // Same as Phase 4 baseline

			ChunkBufferSize:   100,
			ArchiveBufferSize: 100,
			ResultBufferSize:  100,

			S3Bucket: getTestBucket(),
			S3Prefix: fmt.Sprintf("phase5-filesplit-%d", time.Now().Unix()),
			S3Region: getTestRegion(),

			UseRealS3:         true,
			S3Client:          getTestS3Client(b),
			S3PartSize:        64 * 1024 * 1024, // 64MB parts
			EnableMultiPrefix: true,
			ShardCount:        8,
			WorkersPerPrefix:  1, // 8 total workers across 8 shards

			// Phase 5: Enable file splitting with adaptive chunking
			ChunkingConfig: &chunking.ChunkingConfig{
				Workers:             8,
				AvailableMemory:     4 * 1024 * 1024 * 1024, // 4GB
				GroupingStrategy:    "mixed",
				EnableFileSplitting: true,                          // KEY: Enable Phase 5 file splitting
				MaxFileChunkSize:    200 * 1024 * 1024,             // 200MB chunks (for 560MB files = 3 chunks each)
				TargetChunkSize:     200 * 1024 * 1024,             // 200MB target
				MinChunkSize:        10 * 1024 * 1024,              // 10MB minimum
				MaxChunkSize:        5 * 1024 * 1024 * 1024,        // 5GB maximum (S3 limit)
				CostSavingsTarget:   100,                           // 100x cost savings target
				LargeFileThreshold:  100 * 1024 * 1024,             // 100MB threshold
				MultipartPartSize:   100 * 1024 * 1024,             // 100MB multipart parts
				Bandwidth:           100 * 1024 * 1024,             // 100MB/s assumed bandwidth
			},

			EnableProgress: false,
		}

		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatalf("Failed to create pipeline: %v", err)
		}

		ctx := context.Background()
		startTime := time.Now()

		result, err := pipeline.Run(ctx, testDataDir)
		if err != nil {
			b.Fatalf("Pipeline failed: %v", err)
		}

		if !result.Success {
			b.Fatalf("Pipeline failed: %v", result.Errors)
		}

		duration := time.Since(startTime)

		// Report results
		totalGB := float64(result.TotalBytes) / (1024 * 1024 * 1024)
		throughputMBps := float64(result.TotalBytes) / duration.Seconds() / (1024 * 1024)

		b.Logf("\n=== Phase 5 File Splitting Results (Issue #69) ===")
		b.Logf("Files:        %d", result.TotalFiles)
		b.Logf("Total Size:   %.2f GB", totalGB)
		b.Logf("Chunks:       %d", result.ChunksCreated)
		b.Logf("Duration:     %v", duration)
		b.Logf("Throughput:   %.2f MB/s", throughputMBps)
	// Phase 5: Per-stage timing breakdown (instrumentation per user request)
	// Write to separate file to avoid test logger truncation
	stats := pipeline.GetStats()
	stageFile, err := os.Create("/tmp/phase5-stage-breakdown.txt")
	if err != nil {
		b.Logf("WARNING: Failed to create stage breakdown file: %v", err)
	} else {
		defer stageFile.Close()
		fmt.Fprintf(stageFile, "\n=== Phase 5 Stage Breakdown (Issue #69) ===\n")
		fmt.Fprintf(stageFile, "Total Duration: %v\n\n", duration)

		// Sort stage names for consistent output
		stageNames := make([]string, 0, len(stats))
		for name := range stats {
			stageNames = append(stageNames, name)
		}
		sort.Strings(stageNames)

		for _, name := range stageNames {
			stat := stats[name]
			pct := float64(stat.TotalTime) / float64(duration) * 100
			avgTime := time.Duration(0)
			if stat.JobsProcessed > 0 {
				avgTime = stat.TotalTime / time.Duration(stat.JobsProcessed)
			}
			fmt.Fprintf(stageFile, "%-12s %12v (%5.1f%%) | %d jobs | avg %v/job\n",
				name+":", stat.TotalTime, pct, stat.JobsProcessed, avgTime)
		}
		b.Logf("\n=== Phase 5 Stage Breakdown (Issue #69) ===")
		b.Logf("Stage timing details written to: /tmp/phase5-stage-breakdown.txt")
	}


		b.Logf("\n=== Phase 5 Target Validation (Issue #69) ===")

		// Phase 5 Target 1: Duration ≤240s (23% improvement over 311s baseline)
		b.Logf("Target Time:  ≤240s (23%% improvement over 311s baseline)")
		if duration.Seconds() <= 240 {
			improvement := (311 - duration.Seconds()) / 311 * 100
			b.Logf("✅ PASS: Completed in %.2fs (%.1f%% improvement)", duration.Seconds(), improvement)
		} else {
			b.Logf("❌ FAIL: Took %.2fs (%.2fx over 240s target)", duration.Seconds(), duration.Seconds()/240.0)
			b.Logf("   Baseline was 311s, expected 23%% improvement to ≤240s")
		}

		// Phase 5 Target 2: ~300 chunks (3× parallelism from splitting 100 files)
		b.Logf("Target Chunks: ~300 chunks (3× parallelism, 100 files @ 560MB → 200MB chunks)")
		if result.ChunksCreated >= 250 && result.ChunksCreated <= 350 {
			parallelism := float64(result.ChunksCreated) / 100.0
			b.Logf("✅ PASS: Created %d chunks (%.1f× parallelism)", result.ChunksCreated, parallelism)
		} else {
			parallelism := float64(result.ChunksCreated) / 100.0
			b.Logf("⚠️  WARN: Created %d chunks (%.1f× parallelism, expected ~300)", result.ChunksCreated, parallelism)
		}

		// Comparison with Phase 4 baseline
		b.Logf("\n=== Comparison with Phase 4 Baseline ===")
		baselineDuration := 311.0 // seconds (from Phase 4 benchmark)
		improvement := (baselineDuration - duration.Seconds()) / baselineDuration * 100
		b.Logf("Baseline:     311s (100 files @ 56GB, 8 workers, NO file splitting)")
		b.Logf("Phase 5:      %.2fs (100 files @ 56GB, 8 workers, WITH file splitting)", duration.Seconds())
		if improvement > 0 {
			b.Logf("Improvement:  %.1f%% faster", improvement)
		} else {
			b.Logf("Regression:   %.1f%% slower", -improvement)
		}

		// Cleanup
		if err := pipeline.Stop(); err != nil {
			b.Logf("Warning: pipeline stop error: %v", err)
		}
	}
}
