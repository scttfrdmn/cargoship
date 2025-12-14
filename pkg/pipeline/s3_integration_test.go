//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3Integration_RealUpload tests the pipeline with real AWS S3
//
// Prerequisites:
// - AWS credentials configured (AWS_PROFILE or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)
// - S3 bucket available (CARGOSHIP_TEST_BUCKET or defaults to cargoship-pipeline-test)
// - Run with: go test -tags=integration -run TestS3Integration
func TestS3Integration_RealUpload(t *testing.T) {
	// Skip if not explicitly enabled
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") == "" {
		t.Skip("S3 integration tests disabled. Set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 to enable")
	}

	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Verify bucket exists
	ctx := context.Background()
	_, err = s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Skipf("S3 bucket %s not accessible: %v", bucket, err)
	}

	// Create test directory with files
	tmpDir, cleanup := createTestFiles(t, 20, 10*1024) // 20 files @ 10KB each
	defer cleanup()

	// Create pipeline config with real S3
	testPrefix := fmt.Sprintf("pipeline-test-%d", time.Now().Unix())
	pipelineConfig := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        bucket,
		S3Prefix:        testPrefix,
		S3Region:        region,
		UseRealS3:       true, // Enable real S3 uploader
		S3Client:        s3Client,
		S3PartSize:      5 * 1024 * 1024, // 5MB parts (minimum for S3)
	}

	// Create and run pipeline
	pipeline, err := NewPipeline(pipelineConfig)
	require.NoError(t, err)

	// Track progress
	progressUpdates := 0
	pipeline.SetProgressCallback(func(p Progress) {
		progressUpdates++
		t.Logf("Progress: %d/%d files, %d/%d chunks, %.2f MB/s",
			p.FilesProcessed, p.TotalFiles,
			p.ChunksCompleted, p.TotalChunks,
			p.BytesPerSecond/(1024*1024))
	})

	result, err := pipeline.Run(ctx, tmpDir)
	require.NoError(t, err)
	assert.True(t, result.Success)

	t.Logf("Upload completed:")
	t.Logf("  Files: %d", result.TotalFiles)
	t.Logf("  Chunks: %d", result.ChunksCreated)
	t.Logf("  Duration: %v", result.TotalTime)
	t.Logf("  Throughput: %.2f MB/s", float64(result.TotalBytes)/result.TotalTime.Seconds()/(1024*1024))

	// Verify objects in S3
	listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(testPrefix),
	})
	require.NoError(t, err)
	assert.Greater(t, len(listResult.Contents), 0, "Should have uploaded objects")

	t.Logf("Objects in S3:")
	for _, obj := range listResult.Contents {
		t.Logf("  - %s (%d bytes)", *obj.Key, obj.Size)
	}

	// Cleanup: Delete test objects
	for _, obj := range listResult.Contents {
		_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
		if err != nil {
			t.Logf("Warning: Failed to delete %s: %v", *obj.Key, err)
		}
	}
}

// TestS3Integration_LargeFile tests uploading a larger file that requires multipart
func TestS3Integration_LargeFile(t *testing.T) {
	// Skip if not explicitly enabled
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") == "" {
		t.Skip("S3 integration tests disabled. Set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 to enable")
	}

	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Verify bucket exists
	ctx := context.Background()
	_, err = s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Skipf("S3 bucket %s not accessible: %v", bucket, err)
	}

	// Create test directory with larger files
	tmpDir, err := os.MkdirTemp("", "pipeline-s3-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create 5 files @ 2MB each = 10MB total (will compress to ~1MB)
	for i := 0; i < 5; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("large-file-%d.dat", i))
		content := make([]byte, 2*1024*1024) // 2MB
		// Fill with some pattern (not random, so it compresses well)
		for j := range content {
			content[j] = byte(j % 256)
		}
		err := os.WriteFile(filename, content, 0644)
		require.NoError(t, err)
	}

	// Create pipeline config with real S3
	testPrefix := fmt.Sprintf("pipeline-large-test-%d", time.Now().Unix())
	pipelineConfig := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 4,
		UploaderWorkers: 2,
		S3Bucket:        bucket,
		S3Prefix:        testPrefix,
		S3Region:        region,
		UseRealS3:       true,
		S3Client:        s3Client,
		S3PartSize:      5 * 1024 * 1024, // 5MB parts
	}

	// Create and run pipeline
	pipeline, err := NewPipeline(pipelineConfig)
	require.NoError(t, err)

	startTime := time.Now()
	result, err := pipeline.Run(ctx, tmpDir)
	duration := time.Since(startTime)

	require.NoError(t, err)
	assert.True(t, result.Success)

	t.Logf("Large file upload completed:")
	t.Logf("  Files: %d (10MB uncompressed)", result.TotalFiles)
	t.Logf("  Chunks: %d", result.ChunksCreated)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f MB/s", float64(result.TotalBytes)/duration.Seconds()/(1024*1024))

	// Get stats
	stats := pipeline.GetStats()
	if s3Stats, ok := stats["s3_uploader"]; ok {
		t.Logf("S3 Uploader Stats:")
		t.Logf("  Jobs: %d", s3Stats.JobsProcessed)
		t.Logf("  Bytes: %d", s3Stats.BytesProcessed)
		t.Logf("  Avg Time: %v", s3Stats.AverageTime)
	}

	// Verify and cleanup
	listResult, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(testPrefix),
	})
	require.NoError(t, err)
	assert.Greater(t, len(listResult.Contents), 0)

	// Cleanup
	for _, obj := range listResult.Contents {
		_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
	}
}

// TestS3Integration_ErrorHandling tests error scenarios with real S3
func TestS3Integration_ErrorHandling(t *testing.T) {
	// Skip if not explicitly enabled
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") == "" {
		t.Skip("S3 integration tests disabled")
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background())
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg)

	// Test with non-existent bucket
	tmpDir, cleanup := createTestFiles(t, 5, 1024)
	defer cleanup()

	pipelineConfig := &PipelineConfig{
		ScannerWorkers:  2,
		ArchiverWorkers: 2,
		UploaderWorkers: 2,
		S3Bucket:        "cargoship-nonexistent-bucket-12345",
		UseRealS3:       true,
		S3Client:        s3Client,
	}

	pipeline, err := NewPipeline(pipelineConfig)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := pipeline.Run(ctx, tmpDir)

	// Should get an error (bucket doesn't exist)
	// Error might be in result.Errors or returned directly
	hasError := err != nil || (result != nil && len(result.Errors) > 0)
	assert.True(t, hasError, "Should have error for non-existent bucket")

	if err != nil {
		t.Logf("Expected error: %v", err)
	}
}
