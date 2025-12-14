//go:build integration
// +build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// AWS Integration Benchmarks - Tests against real S3 in us-west-2
// Run with: AWS_PROFILE=aws go test -bench=BenchmarkAWS -run=^$ -tags=integration
//
// IMPORTANT ARCHITECTURAL LIMITATION:
// The current implementation uses io.Pipe for zero-disk streaming, but AWS S3 PutObject
// requires Content-Length header (size must be known upfront). This causes 411 errors:
// "MissingContentLength: You must provide the Content-Length HTTP header"
//
// SOLUTION: Implement S3 Multipart Upload API (CreateMultipartUpload/UploadPart/CompleteMultipartUpload)
// which supports true streaming without knowing size upfront. This is tracked in Issue #TODO.
//
// For now, these benchmarks are DISABLED until multipart upload is implemented.

const (
	awsTestBucket = "cargoship-benchmark-test"
	awsTestRegion = "us-west-2"
	awsTestPrefix = "benchmark-test"
)

// createAWSClient creates a real AWS S3 client
func createAWSClient(b *testing.B) *s3.Client {
	b.Helper()

	// Verify AWS_PROFILE is set
	if os.Getenv("AWS_PROFILE") == "" {
		b.Skip("Skipping AWS benchmark: AWS_PROFILE not set (use AWS_PROFILE=aws)")
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsTestRegion),
	)
	if err != nil {
		b.Fatalf("Failed to load AWS config: %v", err)
	}

	return s3.NewFromConfig(cfg)
}

// ensureTestBucket creates the test bucket if it doesn't exist
func ensureTestBucket(b *testing.B, client *s3.Client) {
	b.Helper()

	ctx := context.Background()

	// Check if bucket exists
	bucket := awsTestBucket
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	})

	if err != nil {
		// Bucket doesn't exist, create it
		b.Logf("Creating test bucket: %s", bucket)
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
			CreateBucketConfiguration: &s3Types.CreateBucketConfiguration{
				LocationConstraint: s3Types.BucketLocationConstraint(awsTestRegion),
			},
		})
		if err != nil {
			b.Fatalf("Failed to create test bucket: %v", err)
		}

		// Wait for bucket to be available
		waiter := s3.NewBucketExistsWaiter(client)
		err = waiter.Wait(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucket),
		}, 30*time.Second)
		if err != nil {
			b.Fatalf("Failed waiting for bucket: %v", err)
		}
	}
}

// cleanupTestObjects removes test objects after benchmark
func cleanupTestObjects(b *testing.B, client *s3.Client, prefix string) {
	b.Helper()

	ctx := context.Background()
	bucket := awsTestBucket

	// List all objects with prefix
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	objectsDeleted := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			b.Logf("Warning: Failed to list objects for cleanup: %v", err)
			return
		}

		if len(page.Contents) == 0 {
			continue
		}

		// Delete objects in batches
		for _, obj := range page.Contents {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    obj.Key,
			})
			if err != nil {
				b.Logf("Warning: Failed to delete object %s: %v", *obj.Key, err)
			} else {
				objectsDeleted++
			}
		}
	}

	if objectsDeleted > 0 {
		b.Logf("Cleaned up %d test objects", objectsDeleted)
	}
}

// Benchmark real AWS S3 upload: 1k files
func BenchmarkAWS_ShardedUpload_1kFiles(b *testing.B) {
	const (
		fileCount  = 1000
		fileSize   = 1024 // 1KB per file
		shardCount = 10
	)

	ctx := context.Background()
	client := createAWSClient(b)
	ensureTestBucket(b, client)

	// Create test files once
	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	testPrefix := fmt.Sprintf("%s/1k-files-%d", awsTestPrefix, time.Now().Unix())

	defer cleanupTestObjects(b, client, testPrefix)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Create router
		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		// Create coordinator with real S3 client
		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     awsTestBucket,
			Prefix:     fmt.Sprintf("%s/run-%d", testPrefix, i),
			Router:     router,
			S3Client:   client,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		// Add all files
		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		// Close and wait for real S3 upload completion
		if err := coord.Close(); err != nil {
			b.Fatalf("Failed to close coordinator: %v", err)
		}

		b.StopTimer()

		// Report statistics
		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")

		b.Logf("Run %d: %v, %.2f MB/s, %.0f files/s, %d shards completed, %d failed",
			i+1, stats.Duration, throughputMBps,
			float64(stats.FilesAdded)/stats.Duration.Seconds(),
			stats.CompletedShards, stats.FailedShards)
	}
}

// Benchmark real AWS S3 upload: 10k files (target: <3s with real network)
func BenchmarkAWS_ShardedUpload_10kFiles(b *testing.B) {
	const (
		fileCount  = 10000
		fileSize   = 1024 // 1KB per file
		shardCount = 10
	)

	ctx := context.Background()
	client := createAWSClient(b)
	ensureTestBucket(b, client)

	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	testPrefix := fmt.Sprintf("%s/10k-files-%d", awsTestPrefix, time.Now().Unix())

	defer cleanupTestObjects(b, client, testPrefix)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     awsTestBucket,
			Prefix:     fmt.Sprintf("%s/run-%d", testPrefix, i),
			Router:     router,
			S3Client:   client,
		}

		coord, err := NewShardCoordinator(ctx, config)
		if err != nil {
			b.Fatalf("Failed to create coordinator: %v", err)
		}

		if err := coord.Start(); err != nil {
			b.Fatalf("Failed to start coordinator: %v", err)
		}

		b.StartTimer()

		if err := coord.AddFiles(files); err != nil {
			b.Fatalf("Failed to add files: %v", err)
		}

		if err := coord.Close(); err != nil {
			b.Fatalf("Failed to close coordinator: %v", err)
		}

		b.StopTimer()

		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")

		// Check against realistic target (accounting for network)
		targetDuration := 3 * time.Second
		if stats.Duration < targetDuration {
			b.Logf("✅ AWS TARGET MET: %v < %v (%.1fx faster)",
				stats.Duration, targetDuration,
				float64(targetDuration)/float64(stats.Duration))
		} else {
			b.Logf("⚠️  AWS TARGET MISSED: %v >= %v (%.1fx slower)",
				stats.Duration, targetDuration,
				float64(stats.Duration)/float64(targetDuration))
		}

		b.Logf("Run %d: %v, %.2f MB/s, %.0f files/s, %d shards completed, %d failed",
			i+1, stats.Duration, throughputMBps,
			float64(stats.FilesAdded)/stats.Duration.Seconds(),
			stats.CompletedShards, stats.FailedShards)
	}
}

// Benchmark traditional single-shard upload to AWS (baseline)
func BenchmarkAWS_TraditionalUpload_1kFiles(b *testing.B) {
	const (
		fileCount  = 1000
		fileSize   = 1024
		shardCount = 1 // Single shard = traditional approach
	)

	ctx := context.Background()
	client := createAWSClient(b)
	ensureTestBucket(b, client)

	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	testPrefix := fmt.Sprintf("%s/traditional-1k-%d", awsTestPrefix, time.Now().Unix())

	defer cleanupTestObjects(b, client, testPrefix)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		routerConfig := &chunking.ShardRouterConfig{
			Strategy:   chunking.ShardByHash,
			ShardCount: shardCount,
		}
		router, _ := chunking.NewShardRouter(routerConfig)

		config := &ShardCoordinatorConfig{
			ShardCount: shardCount,
			Bucket:     awsTestBucket,
			Prefix:     fmt.Sprintf("%s/run-%d", testPrefix, i),
			Router:     router,
			S3Client:   client,
		}

		coord, _ := NewShardCoordinator(ctx, config)
		_ = coord.Start()

		b.StartTimer()

		_ = coord.AddFiles(files)
		_ = coord.Close()

		b.StopTimer()

		stats := coord.GetStats()
		throughputMBps := stats.ThroughputMBps()
		b.ReportMetric(throughputMBps, "MB/s")
		b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")

		b.Logf("Traditional (1 shard): %v, %.2f MB/s, %.0f files/s",
			stats.Duration, throughputMBps,
			float64(stats.FilesAdded)/stats.Duration.Seconds())
	}
}

// Benchmark to compare different shard counts against real AWS
func BenchmarkAWS_ShardCount_Comparison(b *testing.B) {
	const (
		fileCount = 5000
		fileSize  = 1024
	)

	shardCounts := []int{1, 2, 4, 8, 10}

	ctx := context.Background()
	client := createAWSClient(b)
	ensureTestBucket(b, client)

	_, files := createBenchmarkFiles(b, fileCount, fileSize)

	for _, shardCount := range shardCounts {
		b.Run(fmt.Sprintf("shards=%d", shardCount), func(b *testing.B) {
			testPrefix := fmt.Sprintf("%s/shard-comparison-%d-%d",
				awsTestPrefix, shardCount, time.Now().Unix())

			defer cleanupTestObjects(b, client, testPrefix)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				routerConfig := &chunking.ShardRouterConfig{
					Strategy:   chunking.ShardByHash,
					ShardCount: shardCount,
				}
				router, _ := chunking.NewShardRouter(routerConfig)

				config := &ShardCoordinatorConfig{
					ShardCount: shardCount,
					Bucket:     awsTestBucket,
					Prefix:     fmt.Sprintf("%s/run-%d", testPrefix, i),
					Router:     router,
					S3Client:   client,
				}

				coord, _ := NewShardCoordinator(ctx, config)
				_ = coord.Start()

				b.StartTimer()
				_ = coord.AddFiles(files)
				_ = coord.Close()
				b.StopTimer()

				stats := coord.GetStats()
				b.ReportMetric(float64(stats.FilesAdded)/stats.Duration.Seconds(), "files/s")
				b.ReportMetric(stats.ThroughputMBps(), "MB/s")

				b.Logf("%d shards: %v, %.0f files/s, %.2f MB/s",
					shardCount, stats.Duration,
					float64(stats.FilesAdded)/stats.Duration.Seconds(),
					stats.ThroughputMBps())
			}
		})
	}
}
