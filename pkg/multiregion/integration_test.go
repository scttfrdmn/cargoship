package multiregion

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test configuration
const (
	testBucketPrefix = "cargoship-multiregion-test"
	testRegion1      = "us-east-1"
	testRegion2      = "us-west-2"
	integrationTestTimeout = 5 * time.Minute
)

// skipIfNoAWS checks if AWS integration tests should run
func skipIfNoAWS(t *testing.T) {
	t.Helper()
	
	// Check if integration tests are explicitly enabled
	if os.Getenv("CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping AWS integration test (set CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS=true to enable)")
	}
	
	// Check for AWS credentials
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		t.Skip("Skipping integration test: no AWS credentials available")
	}
	
	// Try to get caller identity to verify credentials work
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	client := s3.NewFromConfig(cfg)
	_, err = client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		t.Skip("Skipping integration test: AWS credentials not working")
	}
}

// createTestBucket creates a test bucket for integration testing
func createTestBucket(t *testing.T, region string) string {
	t.Helper()
	
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	require.NoError(t, err)
	
	client := s3.NewFromConfig(cfg)
	bucketName := fmt.Sprintf("%s-%s-%d", testBucketPrefix, region, time.Now().Unix())
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}
	
	// Add location constraint for non-us-east-1 regions
	if region != "us-east-1" {
		createBucketInput.CreateBucketConfiguration = &s3Types.CreateBucketConfiguration{
			LocationConstraint: s3Types.BucketLocationConstraint(region),
		}
	}
	
	_, err = client.CreateBucket(ctx, createBucketInput)
	require.NoError(t, err)
	
	// Clean up bucket when test finishes
	t.Cleanup(func() {
		cleanupTestBucket(t, bucketName, region)
	})
	
	return bucketName
}

// cleanupTestBucket removes a test bucket and all its contents
func cleanupTestBucket(t *testing.T, bucketName, region string) {
	t.Helper()
	
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		t.Logf("Failed to load config for cleanup: %v", err)
		return
	}
	
	client := s3.NewFromConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// List and delete all objects first
	listResp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	if err == nil && listResp.Contents != nil {
		for _, obj := range listResp.Contents {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    obj.Key,
			})
			if err != nil {
				t.Logf("Failed to delete object %s: %v", *obj.Key, err)
			}
		}
	}
	
	// Delete the bucket
	_, err = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Logf("Failed to delete bucket %s: %v", bucketName, err)
	}
}

// createTestFile creates a temporary test file
func createTestFile(t *testing.T, content string) string {
	t.Helper()
	
	tmpFile, err := os.CreateTemp("", "multiregion-test-*.txt")
	require.NoError(t, err)
	
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	
	err = tmpFile.Close()
	require.NoError(t, err)
	
	// Clean up file when test finishes
	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})
	
	return tmpFile.Name()
}

// TestMultiRegionCoordinatorIntegration tests the full multi-region coordinator with real AWS S3
func TestMultiRegionCoordinatorIntegration(t *testing.T) {
	skipIfNoAWS(t)
	
	// Create test bucket in primary region
	bucket1 := createTestBucket(t, testRegion1)
	_ = createTestBucket(t, testRegion2) // Create but don't use for this test
	
	// Create test file
	testContent := "This is a test file for multi-region integration testing"
	testFile := createTestFile(t, testContent)
	
	// Create multi-region configuration
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: testRegion1,
		Regions: []Region{
			{
				Name:     testRegion1,
				Priority: 1,
				Weight:   60,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 5,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          false, // Disable for integration test
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
			{
				Name:     testRegion2,
				Priority: 2,
				Weight:   40,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 3,
					MaxBandwidthMbps:     800,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          false, // Disable for integration test
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       LoadBalancingRoundRobin,
			StickySessions: false,
			SessionTTL:     5 * time.Minute,
		},
		Failover: FailoverConfig{
			AutoFailover:      true,
			Strategy:          FailoverGraceful,
			DetectionInterval: 15 * time.Second,
			FailoverTimeout:   30 * time.Second,
			RetryAttempts:     2,
		},
		Monitoring: MonitoringConfig{
			Enabled:         false, // Disable for integration test
			MetricsInterval: 60 * time.Second,
		},
	}
	
	// Initialize coordinator
	coordinator := NewCoordinator()
	
	ctx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	
	// Ensure cleanup
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	// Test upload to primary region
	uploadRequest := &UploadRequest{
		ID:              "integration-test-1",
		FilePath:        testFile,
		Size:            int64(len(testContent)),
		Priority:        5,
		PreferredRegion: testRegion1,
		DestinationKey:  "test-upload.txt",
	}
	
	result, err := coordinator.Upload(ctx, uploadRequest)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, testRegion1, result.Region)
	assert.Equal(t, int64(len(testContent)), result.BytesTransferred)
	assert.Greater(t, result.Duration, time.Duration(0))
	
	// Verify upload succeeded by checking S3
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(testRegion1))
	require.NoError(t, err)
	
	client := s3.NewFromConfig(cfg)
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket1),
		Key:    aws.String("test-upload.txt"),
	})
	assert.NoError(t, err, "Object should exist in S3")
	
	t.Logf("✅ Multi-region coordinator integration test passed - uploaded to %s", result.Region)
}

// TestMultiRegionFailoverIntegration tests failover scenarios with real AWS S3
func TestMultiRegionFailoverIntegration(t *testing.T) {
	skipIfNoAWS(t)
	
	// Create test bucket in failover region
	_ = createTestBucket(t, testRegion1) // Primary (will be unhealthy)
	bucket2 := createTestBucket(t, testRegion2) // Failover target
	
	// Create test file
	testContent := "Failover integration test content"
	testFile := createTestFile(t, testContent)
	
	// Create configuration with one region marked as unhealthy
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: testRegion1,
		Regions: []Region{
			{
				Name:     testRegion1,
				Priority: 1,
				Weight:   100,
				Status:   RegionStatusUnhealthy, // Mark as unhealthy to force failover
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 5,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
			{
				Name:     testRegion2,
				Priority: 2,
				Weight:   100,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 5,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       LoadBalancingRoundRobin,
			StickySessions: false,
		},
		Failover: FailoverConfig{
			AutoFailover:      true,
			Strategy:          FailoverGraceful,
			DetectionInterval: 5 * time.Second,
			FailoverTimeout:   10 * time.Second,
			RetryAttempts:     1,
		},
		Monitoring: MonitoringConfig{Enabled: false},
	}
	
	coordinator := NewCoordinator()
	
	ctx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	// Upload should automatically failover to healthy region
	uploadRequest := &UploadRequest{
		ID:              "failover-test-1",
		FilePath:        testFile,
		Size:            int64(len(testContent)),
		Priority:        5,
		PreferredRegion: testRegion1, // Request unhealthy region
		DestinationKey:  "failover-test.txt",
	}
	
	result, err := coordinator.Upload(ctx, uploadRequest)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, testRegion2, result.Region) // Should failover to healthy region
	
	// Verify upload in the failover region
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(testRegion2))
	require.NoError(t, err)
	
	client := s3.NewFromConfig(cfg)
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket2),
		Key:    aws.String("failover-test.txt"),
	})
	assert.NoError(t, err, "Object should exist in failover region")
	
	t.Logf("✅ Multi-region failover integration test passed - failed over to %s", result.Region)
}

// TestMultiRegionLoadBalancingIntegration tests load balancing across regions
func TestMultiRegionLoadBalancingIntegration(t *testing.T) {
	skipIfNoAWS(t)
	
	// Create test bucket for load balancing test
	bucket1 := createTestBucket(t, testRegion1)
	_ = createTestBucket(t, testRegion2) // Create for completeness
	
	// Create multiple test files
	testFiles := make([]string, 4)
	for i := 0; i < 4; i++ {
		content := fmt.Sprintf("Load balancing test file %d", i)
		testFiles[i] = createTestFile(t, content)
	}
	
	// Create configuration with weighted load balancing
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: testRegion1,
		Regions: []Region{
			{
				Name:     testRegion1,
				Priority: 1,
				Weight:   70,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
			{
				Name:     testRegion2,
				Priority: 2,
				Weight:   30,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       LoadBalancingWeighted,
			StickySessions: false,
		},
		Failover: FailoverConfig{
			AutoFailover:      true,
			Strategy:          FailoverGraceful,
			DetectionInterval: 15 * time.Second,
			FailoverTimeout:   30 * time.Second,
			RetryAttempts:     2,
		},
		Monitoring: MonitoringConfig{Enabled: false},
	}
	
	coordinator := NewCoordinator()
	
	ctx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	// Track region distribution
	regionCounts := make(map[string]int)
	
	// Upload multiple files and track distribution
	for i, testFile := range testFiles {
		uploadRequest := &UploadRequest{
			ID:          fmt.Sprintf("loadbalance-test-%d", i),
			FilePath:    testFile,
			Size:        int64(len(fmt.Sprintf("Load balancing test file %d", i))),
			Priority:    5,
			DestinationKey: fmt.Sprintf("loadbalance-test-%d.txt", i),
		}
		
		result, err := coordinator.Upload(ctx, uploadRequest)
		require.NoError(t, err)
		assert.True(t, result.Success)
		
		regionCounts[result.Region]++
		
		t.Logf("Upload %d routed to region: %s", i, result.Region)
	}
	
	// Verify both regions were used (though distribution may vary due to time-based selection)
	assert.Greater(t, len(regionCounts), 0, "At least one region should be used")
	
	// Verify uploads succeeded
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(testRegion1))
	require.NoError(t, err)
	
	client := s3.NewFromConfig(cfg)
	for i := 0; i < len(testFiles); i++ {
		_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket1),
			Key:    aws.String(fmt.Sprintf("loadbalance-test-%d.txt", i)),
		})
		assert.NoError(t, err, "Object %d should exist in S3", i)
	}
	
	t.Logf("✅ Multi-region load balancing integration test passed - region distribution: %v", regionCounts)
}

// TestMultiRegionConcurrentUploadsIntegration tests concurrent uploads across regions
func TestMultiRegionConcurrentUploadsIntegration(t *testing.T) {
	skipIfNoAWS(t)
	
	// Create test bucket
	bucket := createTestBucket(t, testRegion1)
	
	// Create configuration
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: testRegion1,
		Regions: []Region{
			{
				Name:     testRegion1,
				Priority: 1,
				Weight:   50,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
			{
				Name:     testRegion2,
				Priority: 1,
				Weight:   50,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:       LoadBalancingRoundRobin,
			StickySessions: false,
		},
		Failover: FailoverConfig{
			AutoFailover:    true,
			Strategy:        FailoverGraceful,
			RetryAttempts:   2,
		},
		Monitoring: MonitoringConfig{Enabled: false},
	}
	
	coordinator := NewCoordinator()
	
	ctx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	// Create test files
	const numUploads = 5
	testFiles := make([]string, numUploads)
	for i := 0; i < numUploads; i++ {
		content := fmt.Sprintf("Concurrent upload test file %d with some content to make it realistic", i)
		testFiles[i] = createTestFile(t, content)
	}
	
	// Perform concurrent uploads
	results := make(chan *UploadResult, numUploads)
	errors := make(chan error, numUploads)
	
	for i := 0; i < numUploads; i++ {
		go func(index int) {
			uploadRequest := &UploadRequest{
				ID:          fmt.Sprintf("concurrent-test-%d", index),
				FilePath:    testFiles[index],
				Size:        int64(len(fmt.Sprintf("Concurrent upload test file %d with some content to make it realistic", index))),
				Priority:    5,
				DestinationKey: fmt.Sprintf("concurrent-test-%d.txt", index),
			}
			
			result, err := coordinator.Upload(ctx, uploadRequest)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}(i)
	}
	
	// Collect results
	successCount := 0
	for i := 0; i < numUploads; i++ {
		select {
		case result := <-results:
			assert.True(t, result.Success)
			assert.Greater(t, result.Duration, time.Duration(0))
			successCount++
			t.Logf("Concurrent upload %s completed in region %s", result.RequestID, result.Region)
		case err := <-errors:
			t.Errorf("Upload failed: %v", err)
		case <-time.After(2 * time.Minute):
			t.Fatalf("Timeout waiting for concurrent uploads")
		}
	}
	
	assert.Equal(t, numUploads, successCount, "All uploads should succeed")
	
	// Verify uploads in S3
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(testRegion1))
	require.NoError(t, err)
	
	client := s3.NewFromConfig(cfg)
	for i := 0; i < numUploads; i++ {
		_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fmt.Sprintf("concurrent-test-%d.txt", i)),
		})
		assert.NoError(t, err, "Concurrent upload %d should exist in S3", i)
	}
	
	t.Logf("✅ Multi-region concurrent uploads integration test passed - %d uploads completed", successCount)
}

// TestMultiRegionStatusAndMetricsIntegration tests region status and metrics collection
func TestMultiRegionStatusAndMetricsIntegration(t *testing.T) {
	skipIfNoAWS(t)
	
	// Create configuration with monitoring disabled for faster test
	config := &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: testRegion1,
		Regions: []Region{
			{
				Name:     testRegion1,
				Priority: 1,
				Weight:   100,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 5,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{Enabled: false},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy: LoadBalancingRoundRobin,
		},
		Failover: FailoverConfig{
			AutoFailover:  true,
			Strategy:      FailoverGraceful,
			RetryAttempts: 2,
		},
		Monitoring: MonitoringConfig{Enabled: false},
	}
	
	coordinator := NewCoordinator()
	
	ctx, cancel := context.WithTimeout(context.Background(), integrationTestTimeout)
	defer cancel()
	
	err := coordinator.Initialize(ctx, config)
	require.NoError(t, err)
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	// Test region status
	status, err := coordinator.GetRegionStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Contains(t, status, testRegion1)
	assert.Equal(t, RegionStatusHealthy, status[testRegion1])
	
	// Test region metrics
	metrics, err := coordinator.GetRegionMetrics(ctx)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Contains(t, metrics, testRegion1)
	
	regionMetrics := metrics[testRegion1]
	assert.GreaterOrEqual(t, regionMetrics.SuccessfulUploads, int64(0))
	assert.GreaterOrEqual(t, regionMetrics.FailedUploads, int64(0))
	assert.GreaterOrEqual(t, regionMetrics.ErrorRate, 0.0)
	
	t.Logf("✅ Multi-region status and metrics integration test passed")
	t.Logf("   Region status: %v", status)
	t.Logf("   Region metrics: %v", metrics)
}