//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	substrate "github.com/scttfrdmn/substrate/emulator"
)

var substrateURL string

func TestMain(m *testing.M) {
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") == "1" {
		// Real AWS — env untouched, tests run as before
	} else {
		url, cancel, err := launchSubstrate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "substrate: %v\n", err)
			os.Exit(1)
		}
		defer cancel()
		substrateURL = url
		os.Setenv("AWS_ENDPOINT_URL", url)
		os.Setenv("AWS_ACCESS_KEY_ID", "test")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		os.Setenv("AWS_REGION", "us-east-1")
		if err := createSubstrateBucket(url, "cargoship-pipeline-test"); err != nil {
			fmt.Fprintf(os.Stderr, "create bucket: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// launchSubstrate starts an in-process Substrate server for use in TestMain.
func launchSubstrate() (string, context.CancelFunc, error) {
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))

	ctx := context.Background()
	if err := substrate.RegisterDefaultPlugins(ctx, registry, state, tc, logger, store, nil); err != nil {
		return "", nil, fmt.Errorf("register plugins: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, pingErr := http.Get(baseURL + "/health") //nolint:noctx
		if pingErr == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return baseURL, cancel, nil
}

// createSubstrateBucket creates an S3 bucket on the Substrate server.
func createSubstrateBucket(baseURL, bucket string) error {
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(baseURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	_, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// TestS3Integration_RealUpload tests the pipeline with real AWS S3
//
// Prerequisites:
// - AWS credentials configured (AWS_PROFILE or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)
// - S3 bucket available (CARGOSHIP_TEST_BUCKET or defaults to cargoship-pipeline-test)
// - Run with: go test -tags=integration -run TestS3Integration
func TestS3Integration_RealUpload(t *testing.T) {
	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

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
	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

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
	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background())
	require.NoError(t, err)

	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)

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
