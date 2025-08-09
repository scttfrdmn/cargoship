//go:build integration
// +build integration

package s3optimization

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testProfile = "aws"
	testRegion  = "us-west-2"
	testBucket  = "cargoship-integration-test"
	testPrefix  = "s3optimization-test"
)

// TestRealAWSS3Integration performs comprehensive integration testing with real AWS S3
func TestRealAWSS3Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load AWS config with specific profile and region
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(testProfile),
		config.WithRegion(testRegion),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Verify bucket exists or create it
	err = ensureTestBucket(ctx, s3Client, testBucket)
	require.NoError(t, err, "Failed to ensure test bucket exists")

	t.Run("BasicOptimizedOperations", func(t *testing.T) {
		testBasicOptimizedOperations(t, ctx, s3Client, logger)
	})

	t.Run("PerformanceComparison", func(t *testing.T) {
		testPerformanceComparison(t, ctx, s3Client, logger)
	})

	t.Run("LargeFileUpload", func(t *testing.T) {
		testLargeFileUpload(t, ctx, s3Client, logger)
	})

	t.Run("BatchOperations", func(t *testing.T) {
		testBatchOperations(t, ctx, s3Client, logger)
	})

	t.Run("NetworkConditionsAdaptation", func(t *testing.T) {
		testNetworkConditionsAdaptation(t, ctx, s3Client, logger)
	})

	// Cleanup test objects
	t.Cleanup(func() {
		cleanupTestObjects(ctx, s3Client, testBucket, testPrefix)
	})
}

func testBasicOptimizedOperations(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// Create optimized S3 client
	optimizer, err := NewS3Optimizer(ctx, s3Client, nil, logger)
	require.NoError(t, err)
	defer optimizer.Shutdown(ctx)

	testKey := fmt.Sprintf("%s/basic-test-%d.txt", testPrefix, time.Now().Unix())
	testData := "Hello, CargoShip S3 Optimization! This is a test file for integration testing."

	t.Run("PutObjectOptimized", func(t *testing.T) {
		startTime := time.Now()

		result, err := optimizer.PutObjectOptimized(ctx, &s3.PutObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey),
			Body:   strings.NewReader(testData),
		})

		duration := time.Since(startTime)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotEmpty(t, *result.ETag)

		t.Logf("Upload completed in %s", duration)
	})

	t.Run("GetObjectOptimized", func(t *testing.T) {
		startTime := time.Now()

		result, err := optimizer.GetObjectOptimized(ctx, &s3.GetObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey),
		})

		duration := time.Since(startTime)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Body)

		// Read and verify content
		content, err := io.ReadAll(result.Body)
		require.NoError(t, err)
		assert.Equal(t, testData, string(content))

		t.Logf("Download completed in %s", duration)
	})

	t.Run("HeadObjectOptimized", func(t *testing.T) {
		result, err := optimizer.HeadObjectOptimized(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey),
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, int64(len(testData)), *result.ContentLength)
	})

	t.Run("DeleteObjectOptimized", func(t *testing.T) {
		_, err := optimizer.DeleteObjectOptimized(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey),
		})

		require.NoError(t, err)
	})
}

func testPerformanceComparison(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// Create test data (1MB)
	testData := strings.Repeat("Performance test data for CargoShip S3 optimization. ", 14000) // ~1MB
	testKey := fmt.Sprintf("%s/perf-test-%d.txt", testPrefix, time.Now().Unix())

	// Test regular S3 client
	t.Run("RegularS3Client", func(t *testing.T) {
		startTime := time.Now()

		_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey + "-regular"),
			Body:   strings.NewReader(testData),
		})

		regularDuration := time.Since(startTime)
		require.NoError(t, err)

		t.Logf("Regular S3 client upload: %s", regularDuration)

		// Cleanup
		s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey + "-regular"),
		})
	})

	// Test optimized S3 client
	t.Run("OptimizedS3Client", func(t *testing.T) {
		optimizer, err := NewS3Optimizer(ctx, s3Client, DefaultConfig(), logger)
		require.NoError(t, err)
		defer optimizer.Shutdown(ctx)

		startTime := time.Now()

		_, err = optimizer.PutObjectOptimized(ctx, &s3.PutObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey + "-optimized"),
			Body:   strings.NewReader(testData),
		})

		optimizedDuration := time.Since(startTime)
		require.NoError(t, err)

		t.Logf("Optimized S3 client upload: %s", optimizedDuration)

		// Get performance metrics
		metrics := optimizer.GetPerformanceMetrics()
		t.Logf("Optimization ratio: %.2fx", metrics.OptimizationRatio)
		t.Logf("Bandwidth savings: %.1f%%", metrics.BandwidthSavings)

		// Cleanup
		optimizer.DeleteObjectOptimized(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKey + "-optimized"),
		})
	})
}

func testLargeFileUpload(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// Check if we can access test data from astrapi.local
	testDataPath := "/Volumes/Public" // Common mount point for network shares
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		// Try alternative paths
		alternatives := []string{
			"/mnt/astrapi-public",
			"/media/astrapi/Public",
			"//astrapi.local/Public",
		}

		found := false
		for _, alt := range alternatives {
			if _, err := os.Stat(alt); err == nil {
				testDataPath = alt
				found = true
				break
			}
		}

		if !found {
			t.Skip("Cannot access astrapi.local Public directory - skipping large file test")
		}
	}

	// Find a suitable test file
	var testFile string
	err := filepath.Walk(testDataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Look for files between 10MB and 100MB
		if !info.IsDir() && info.Size() > 10*1024*1024 && info.Size() < 100*1024*1024 {
			testFile = path
			return filepath.SkipDir // Stop walking
		}
		return nil
	})

	if testFile == "" {
		t.Skip("No suitable test file found in astrapi.local Public directory")
	}

	t.Logf("Using test file: %s", testFile)

	// Open test file
	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	fileInfo, err := file.Stat()
	require.NoError(t, err)

	testKey := fmt.Sprintf("%s/large-file-test-%d%s", testPrefix, time.Now().Unix(), filepath.Ext(testFile))

	// Test with optimized client
	optimizer, err := NewS3Optimizer(ctx, s3Client, DefaultConfig(), logger)
	require.NoError(t, err)
	defer optimizer.Shutdown(ctx)

	startTime := time.Now()

	_, err = optimizer.PutObjectOptimized(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(testBucket),
		Key:           aws.String(testKey),
		Body:          file,
		ContentLength: aws.Int64(fileInfo.Size()),
	})

	uploadDuration := time.Since(startTime)
	require.NoError(t, err)

	// Calculate throughput
	throughputMBps := float64(fileInfo.Size()) / (1024 * 1024) / uploadDuration.Seconds()

	t.Logf("Large file upload completed:")
	t.Logf("  File size: %.2f MB", float64(fileInfo.Size())/(1024*1024))
	t.Logf("  Duration: %s", uploadDuration)
	t.Logf("  Throughput: %.2f MB/s", throughputMBps)

	// Get optimization metrics
	metrics := optimizer.GetPerformanceMetrics()
	t.Logf("  Optimization ratio: %.2fx", metrics.OptimizationRatio)
	t.Logf("  Total requests: %d", metrics.TotalRequests)
	t.Logf("  Success rate: %.1f%%", float64(metrics.SuccessfulRequests)/float64(metrics.TotalRequests)*100)

	// Cleanup
	_, err = optimizer.DeleteObjectOptimized(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(testKey),
	})
	require.NoError(t, err)
}

func testBatchOperations(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	optimizer, err := NewS3Optimizer(ctx, s3Client, DefaultConfig(), logger)
	require.NoError(t, err)
	defer optimizer.Shutdown(ctx)

	// Create batch PUT requests
	batchSize := 5
	putRequests := make([]*s3.PutObjectInput, batchSize)
	testKeys := make([]string, batchSize)

	for i := 0; i < batchSize; i++ {
		testKeys[i] = fmt.Sprintf("%s/batch-test-%d-%d.txt", testPrefix, time.Now().Unix(), i)
		putRequests[i] = &s3.PutObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(testKeys[i]),
			Body:   strings.NewReader(fmt.Sprintf("Batch test data %d", i)),
		}
	}

	t.Run("BatchPutOperations", func(t *testing.T) {
		startTime := time.Now()

		results, err := optimizer.PutObjectsBatch(ctx, putRequests)

		batchDuration := time.Since(startTime)
		require.NoError(t, err)
		assert.Len(t, results, batchSize)

		t.Logf("Batch PUT completed: %d objects in %s", batchSize, batchDuration)
	})

	// Create batch GET requests
	getRequests := make([]*s3.GetObjectInput, batchSize)
	for i, key := range testKeys {
		getRequests[i] = &s3.GetObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(key),
		}
	}

	t.Run("BatchGetOperations", func(t *testing.T) {
		startTime := time.Now()

		results, err := optimizer.GetObjectsBatch(ctx, getRequests)

		batchDuration := time.Since(startTime)
		require.NoError(t, err)
		assert.Len(t, results, batchSize)

		t.Logf("Batch GET completed: %d objects in %s", batchSize, batchDuration)

		// Verify content
		for i, result := range results {
			content, err := io.ReadAll(result.Body)
			require.NoError(t, err)
			expectedContent := fmt.Sprintf("Batch test data %d", i)
			assert.Equal(t, expectedContent, string(content))
		}
	})

	// Cleanup batch objects
	for _, key := range testKeys {
		optimizer.DeleteObjectOptimized(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(key),
		})
	}
}

func testNetworkConditionsAdaptation(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	optimizer, err := NewS3Optimizer(ctx, s3Client, DefaultConfig(), logger)
	require.NoError(t, err)
	defer optimizer.Shutdown(ctx)

	// Test different network conditions
	conditions := []*NetworkConditions{
		{
			Bandwidth:   100.0, // 100 Mbps
			RTT:         20 * time.Millisecond,
			PacketLoss:  0.1,
			Congestion:  5.0,
			LastUpdated: time.Now(),
		},
		{
			Bandwidth:   50.0, // 50 Mbps - slower connection
			RTT:         100 * time.Millisecond,
			PacketLoss:  1.0,
			Congestion:  15.0,
			LastUpdated: time.Now(),
		},
		{
			Bandwidth:   200.0, // 200 Mbps - faster connection
			RTT:         10 * time.Millisecond,
			PacketLoss:  0.01,
			Congestion:  2.0,
			LastUpdated: time.Now(),
		},
	}

	testData := strings.Repeat("Network adaptation test data. ", 1000) // ~30KB

	for i, condition := range conditions {
		t.Run(fmt.Sprintf("NetworkCondition_%d", i+1), func(t *testing.T) {
			// Update network conditions
			err := optimizer.UpdateNetworkConditions(condition)
			require.NoError(t, err)

			testKey := fmt.Sprintf("%s/network-test-%d-%d.txt", testPrefix, time.Now().Unix(), i)

			startTime := time.Now()

			_, err = optimizer.PutObjectOptimized(ctx, &s3.PutObjectInput{
				Bucket: aws.String(testBucket),
				Key:    aws.String(testKey),
				Body:   strings.NewReader(testData),
			})

			duration := time.Since(startTime)
			require.NoError(t, err)

			t.Logf("Network condition %d: Upload completed in %s", i+1, duration)
			t.Logf("  Bandwidth: %.1f Mbps, RTT: %s, Loss: %.1f%%",
				condition.Bandwidth, condition.RTT, condition.PacketLoss)

			// Cleanup
			optimizer.DeleteObjectOptimized(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(testBucket),
				Key:    aws.String(testKey),
			})
		})
	}

	// Get final performance metrics
	metrics := optimizer.GetPerformanceMetrics()
	t.Logf("Final optimization metrics:")
	t.Logf("  Total requests: %d", metrics.TotalRequests)
	t.Logf("  Success rate: %.1f%%", float64(metrics.SuccessfulRequests)/float64(metrics.TotalRequests)*100)
	t.Logf("  Optimization ratio: %.2fx", metrics.OptimizationRatio)
}

// Helper functions

func ensureTestBucket(ctx context.Context, s3Client *s3.Client, bucket string) error {
	// Check if bucket exists
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err == nil {
		return nil // Bucket exists
	}

	// Try to create bucket
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(testRegion),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create test bucket %s: %w", bucket, err)
	}

	return nil
}

func cleanupTestObjects(ctx context.Context, s3Client *s3.Client, bucket, prefix string) {
	// List objects with prefix
	result, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		return // Skip cleanup on error
	}

	// Delete each object
	for _, obj := range result.Contents {
		s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
	}
}
