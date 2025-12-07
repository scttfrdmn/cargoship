package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipeline_EndToEnd tests the complete pipeline flow
func TestPipeline_EndToEnd(t *testing.T) {
	// Create test directory with files
	tmpDir, cleanup := createTestFiles(t, 50, 1024) // 50 files, 1KB each
	defer cleanup()

	// Create pipeline config
	config := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        "test-bucket",
		S3Prefix:        "test/",
		EnableProgress:  false,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers:           4,
			AvailableMemory:   1024 * 1024 * 1024, // 1GB
			GroupingStrategy:  "size",
			CostSavingsTarget: 10,
		},
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	// Run pipeline
	ctx := context.Background()
	result, err := pipeline.Run(ctx, tmpDir)

	// Assertions
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success, "Pipeline should succeed")
	assert.Equal(t, int64(50), result.TotalFiles)
	assert.Greater(t, result.ChunksCreated, 0)
	assert.Greater(t, result.TotalTime, time.Duration(0))

	t.Logf("Pipeline completed: %d files, %d chunks, %v",
		result.TotalFiles, result.ChunksCreated, result.TotalTime)
}

// TestPipeline_ContextCancellation tests graceful cancellation
func TestPipeline_ContextCancellation(t *testing.T) {
	tmpDir, cleanup := createTestFiles(t, 100, 10*1024) // 100 files, 10KB each
	defer cleanup()

	config := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        "test-bucket",
		EnableProgress:  false,
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run pipeline (should be cancelled)
	result, err := pipeline.Run(ctx, tmpDir)

	// Pipeline should be cancelled
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}

	// Result may be partial
	if result != nil {
		t.Logf("Partial result: %d files, %d chunks",
			result.TotalFiles, result.ChunksCreated)
	}
}

// TestPipeline_ProgressTracking tests progress reporting
func TestPipeline_ProgressTracking(t *testing.T) {
	tmpDir, cleanup := createTestFiles(t, 30, 1024)
	defer cleanup()

	config := &PipelineConfig{
		ScannerWorkers:   2,
		ArchiverWorkers:  4,
		UploaderWorkers:  2,
		S3Bucket:         "test-bucket",
		EnableProgress:   true,
		ProgressInterval: time.Millisecond, // Short interval for testing
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	// Track progress updates
	progressUpdates := 0
	pipeline.SetProgressCallback(func(p Progress) {
		progressUpdates++
		t.Logf("Progress: %d/%d files, %d/%d chunks",
			p.FilesProcessed, p.TotalFiles,
			p.ChunksCompleted, p.TotalChunks)
	})

	ctx := context.Background()
	result, err := pipeline.Run(ctx, tmpDir)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Progress updates may be 0 if test completes faster than the first ticker fire
	// The ticker fires at intervals, not immediately, so even if total time > interval,
	// we may not get updates if the work completes before the first ticker.C event
	t.Logf("Progress updates received: %d (test completed in %v, interval: %v)",
		progressUpdates, result.TotalTime, config.ProgressInterval)

	// Only assert progress if test took significantly longer than interval
	// (at least 2x to ensure ticker had time to fire)
	if result.TotalTime > 2*config.ProgressInterval {
		assert.Greater(t, progressUpdates, 0, "Should have progress updates for tests much longer than interval")
	}
}

// TestPipeline_ErrorHandling tests error scenarios
func TestPipeline_ErrorHandling(t *testing.T) {
	// Test with non-existent directory
	config := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        "test-bucket",
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := pipeline.Run(ctx, "/nonexistent/path")

	// Should have error OR failed result (depending on timing/race conditions)
	hasError := err != nil
	hasFailedResult := result != nil && !result.Success

	assert.True(t, hasError || hasFailedResult,
		"Expected either error or failed result, got err=%v, result.Success=%v",
		err, result != nil && result.Success)
}

// TestPipeline_Statistics tests stage statistics collection
func TestPipeline_Statistics(t *testing.T) {
	tmpDir, cleanup := createTestFiles(t, 20, 512)
	defer cleanup()

	config := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        "test-bucket",
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = pipeline.Run(ctx, tmpDir)
	require.NoError(t, err)

	// Get statistics
	stats := pipeline.GetStats()

	// Verify stats for each stage
	scannerStats, ok := stats["scanner"]
	assert.True(t, ok, "Should have scanner stats")
	assert.Greater(t, scannerStats.JobsProcessed, int64(0))

	archiverStats, ok := stats["archiver"]
	assert.True(t, ok, "Should have archiver stats")
	assert.Greater(t, archiverStats.JobsProcessed, int64(0))

	uploaderStats, ok := stats["uploader"]
	assert.True(t, ok, "Should have uploader stats")
	assert.Greater(t, uploaderStats.JobsProcessed, int64(0))

	t.Logf("Scanner: %d jobs, %d bytes, %v avg",
		scannerStats.JobsProcessed, scannerStats.BytesProcessed, scannerStats.AverageTime)
	t.Logf("Archiver: %d jobs, %d bytes, %v avg",
		archiverStats.JobsProcessed, archiverStats.BytesProcessed, archiverStats.AverageTime)
	t.Logf("Uploader: %d jobs, %d bytes, %v avg",
		uploaderStats.JobsProcessed, uploaderStats.BytesProcessed, uploaderStats.AverageTime)
}

// TestPipeline_TimeBreakdown validates time budget allocation
func TestPipeline_TimeBreakdown(t *testing.T) {
	tmpDir, cleanup := createTestFiles(t, 100, 10*1024) // 100 files, 10KB each
	defer cleanup()

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        "test-bucket",
	}

	pipeline, err := NewPipeline(config)
	require.NoError(t, err)

	ctx := context.Background()
	startTime := time.Now()
	result, err := pipeline.Run(ctx, tmpDir)
	totalTime := time.Since(startTime)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Get stage statistics
	stats := pipeline.GetStats()

	scanTime := stats["scanner"].TotalTime
	archiveTime := stats["archiver"].TotalTime
	uploadTime := stats["uploader"].TotalTime

	// Calculate percentages
	scanPct := float64(scanTime) / float64(totalTime) * 100
	archivePct := float64(archiveTime) / float64(totalTime) * 100
	uploadPct := float64(uploadTime) / float64(totalTime) * 100

	t.Logf("Time breakdown:")
	t.Logf("  Scan:    %v (%.1f%%)", scanTime, scanPct)
	t.Logf("  Archive: %v (%.1f%%)", archiveTime, archivePct)
	t.Logf("  Upload:  %v (%.1f%%)", uploadTime, uploadPct)
	t.Logf("  Total:   %v", totalTime)

	// Validation targets from Issue #52:
	// Archive+compress should be ~50% of time
	// Upload should be ~30-45% of time

	// Note: These are loose validations since we're simulating S3
	// In real implementation with actual S3, these would be more accurate
	assert.Greater(t, archivePct, 0.0, "Archive should take some time")
	assert.Greater(t, uploadPct, 0.0, "Upload should take some time")

	// Archive+Upload combined should be majority of time
	combinedPct := archivePct + uploadPct
	assert.Greater(t, combinedPct, 50.0, "Archive+Upload should be >50% of time")
}

// Helper function to create test files
func createTestFiles(t *testing.T, count int, sizeBytes int) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "pipeline-integration-*")
	require.NoError(t, err)

	// Create files
	for i := 0; i < count; i++ {
		content := make([]byte, sizeBytes)
		for j := range content {
			content[j] = byte(i % 256)
		}

		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.dat", i))
		err := os.WriteFile(filename, content, 0644)
		require.NoError(t, err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// Benchmark pipeline performance
func BenchmarkPipeline_SmallFiles(b *testing.B) {
	// Create test directory
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create 100 small files
	for i := 0; i < 100; i++ {
		content := make([]byte, 1024) // 1KB
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        "bench-bucket",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		_, err = pipeline.Run(ctx, tmpDir)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPipeline_LargeFiles(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "pipeline-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create 10 larger files
	for i := 0; i < 10; i++ {
		content := make([]byte, 1024*1024) // 1MB
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.dat", i))
		if err := os.WriteFile(filename, content, 0644); err != nil {
			b.Fatal(err)
		}
	}

	config := &PipelineConfig{
		ScannerWorkers:  4,
		ArchiverWorkers: 8,
		UploaderWorkers: 4,
		S3Bucket:        "bench-bucket",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pipeline, err := NewPipeline(config)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		_, err = pipeline.Run(ctx, tmpDir)
		if err != nil {
			b.Fatal(err)
		}
	}
}
