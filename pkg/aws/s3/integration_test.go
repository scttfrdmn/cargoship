// +build integration

package s3

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

const (
	localStackEndpoint = "http://localhost:4566"
	testBucket         = "test-cargoship-bucket"
	testRegion         = "us-east-1"
)

func TestMain(m *testing.M) {
	// Check if LocalStack is available
	if !isLocalStackAvailable() {
		fmt.Println("Skipping integration tests - LocalStack not available")
		fmt.Println("To run integration tests:")
		fmt.Println("  docker run --rm -d -p 4566:4566 localstack/localstack")
		os.Exit(0)
	}

	// Setup test environment
	if err := setupTestEnvironment(); err != nil {
		fmt.Printf("Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupTestEnvironment()

	os.Exit(code)
}

func isLocalStackAvailable() bool {
	client := getTestS3Client()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	return err == nil
}

func setupTestEnvironment() error {
	client := getTestS3Client()
	ctx := context.Background()

	// Create test bucket
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})
	if err != nil {
		return fmt.Errorf("failed to create test bucket: %w", err)
	}

	// Wait for bucket to be ready
	time.Sleep(1 * time.Second)

	return nil
}

func cleanupTestEnvironment() {
	client := getTestS3Client()
	ctx := context.Background()

	// List and delete all objects in bucket
	listOutput, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(testBucket),
	})
	if err == nil && len(listOutput.Contents) > 0 {
		var objects []types.ObjectIdentifier
		for _, obj := range listOutput.Contents {
			objects = append(objects, types.ObjectIdentifier{
				Key: obj.Key,
			})
		}

		client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(testBucket),
			Delete: &types.Delete{
				Objects: objects,
			},
		})
	}

	// Delete bucket
	client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(testBucket),
	})
}

func getTestS3Client() *s3.Client {
	cfg := aws.Config{
		Region: testRegion,
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           localStackEndpoint,
					SigningRegion: testRegion,
				}, nil
			},
		),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Required for LocalStack
	})

	return client
}

func getTestTransporter() *Transporter {
	client := getTestS3Client()
	config := awsconfig.S3Config{
		Bucket:             testBucket,
		StorageClass:       awsconfig.StorageClassStandard,
		MultipartChunkSize: 5 * 1024 * 1024, // 5MB
		Concurrency:        2,
	}

	return NewTransporter(client, config)
}

func TestTransporterUploadIntegration(t *testing.T) {
	transporter := getTestTransporter()
	ctx := context.Background()

	testCases := []struct {
		name    string
		archive Archive
		wantErr bool
	}{
		{
			name: "simple upload",
			archive: Archive{
				Key:              "test/simple.txt",
				Reader:           bytes.NewReader([]byte("Hello, LocalStack!")),
				Size:             18,
				StorageClass:     awsconfig.StorageClassStandard,
				OriginalSize:     18,
				CompressionType:  "none",
				AccessPattern:    "frequent",
				RetentionDays:    30,
				Metadata: map[string]string{
					"test-key": "test-value",
				},
			},
			wantErr: false,
		},
		{
			name: "large file upload",
			archive: Archive{
				Key:              "test/large-file.dat",
				Reader:           bytes.NewReader(make([]byte, 10*1024*1024)), // 10MB
				Size:             10 * 1024 * 1024,
				StorageClass:     awsconfig.StorageClassStandard,
				OriginalSize:     15 * 1024 * 1024,
				CompressionType:  "gzip",
				AccessPattern:    "infrequent",
				RetentionDays:    365,
			},
			wantErr: false,
		},
		{
			name: "upload with intelligent tiering",
			archive: Archive{
				Key:              "test/intelligent.tar.gz",
				Reader:           bytes.NewReader([]byte("Archive content for intelligent tiering")),
				Size:             39,
				StorageClass:     awsconfig.StorageClassIntelligentTiering,
				OriginalSize:     100,
				CompressionType:  "gzip",
				AccessPattern:    "unknown",
				RetentionDays:    90,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := transporter.Upload(ctx, tc.archive)

			if tc.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Fatal("Expected upload result, got nil")
			}

			// Verify result fields
			if result.Key != tc.archive.Key {
				t.Errorf("Expected key %s, got %s", tc.archive.Key, result.Key)
			}

			if result.Location == "" {
				t.Error("Location should not be empty")
			}

			if result.ETag == "" {
				t.Error("ETag should not be empty")
			}

			if result.Duration <= 0 {
				t.Error("Duration should be positive")
			}

			if result.Throughput <= 0 {
				t.Error("Throughput should be positive")
			}

			// Verify object exists in S3
			exists, err := transporter.Exists(ctx, tc.archive.Key)
			if err != nil {
				t.Errorf("Failed to check if object exists: %v", err)
			}
			if !exists {
				t.Error("Object should exist after upload")
			}

			// Get object info and verify metadata
			objInfo, err := transporter.GetObjectInfo(ctx, tc.archive.Key)
			if err != nil {
				t.Errorf("Failed to get object info: %v", err)
			}

			if objInfo.ContentLength == nil || *objInfo.ContentLength != tc.archive.Size {
				t.Errorf("Expected content length %d, got %v", tc.archive.Size, objInfo.ContentLength)
			}

			// Verify CargoShip metadata
			if objInfo.Metadata["cargoship-created-by"] != "cargoship" {
				t.Error("CargoShip metadata not found")
			}

			if len(tc.archive.Metadata) > 0 {
				for key, expectedValue := range tc.archive.Metadata {
					if objInfo.Metadata[key] != expectedValue {
						t.Errorf("Expected metadata %s=%s, got %s", key, expectedValue, objInfo.Metadata[key])
					}
				}
			}
		})
	}
}

func TestTransporterExistsIntegration(t *testing.T) {
	transporter := getTestTransporter()
	ctx := context.Background()

	// Test non-existent object
	exists, err := transporter.Exists(ctx, "non-existent-key")
	if err != nil {
		t.Errorf("Unexpected error checking non-existent object: %v", err)
	}
	if exists {
		t.Error("Non-existent object should not exist")
	}

	// Upload an object
	archive := Archive{
		Key:    "test-exists.txt",
		Reader: bytes.NewReader([]byte("test content")),
		Size:   12,
	}

	_, err = transporter.Upload(ctx, archive)
	if err != nil {
		t.Fatalf("Failed to upload test object: %v", err)
	}

	// Test existing object
	exists, err = transporter.Exists(ctx, archive.Key)
	if err != nil {
		t.Errorf("Unexpected error checking existing object: %v", err)
	}
	if !exists {
		t.Error("Uploaded object should exist")
	}
}

func TestTransporterGetObjectInfoIntegration(t *testing.T) {
	transporter := getTestTransporter()
	ctx := context.Background()

	// Test non-existent object
	_, err := transporter.GetObjectInfo(ctx, "non-existent-key")
	if err == nil {
		t.Error("Expected error for non-existent object")
	}

	// Upload an object with metadata
	archive := Archive{
		Key:              "test-info.txt",
		Reader:           bytes.NewReader([]byte("test content for info")),
		Size:             21,
		OriginalSize:     50,
		CompressionType:  "gzip",
		AccessPattern:    "rare",
		RetentionDays:    180,
		Metadata: map[string]string{
			"project":     "cargoship-test",
			"environment": "integration",
		},
	}

	_, err = transporter.Upload(ctx, archive)
	if err != nil {
		t.Fatalf("Failed to upload test object: %v", err)
	}

	// Get object info
	objInfo, err := transporter.GetObjectInfo(ctx, archive.Key)
	if err != nil {
		t.Errorf("Unexpected error getting object info: %v", err)
	}

	if objInfo == nil {
		t.Fatal("Expected object info, got nil")
	}

	// Verify basic properties
	if objInfo.ContentLength == nil || *objInfo.ContentLength != archive.Size {
		t.Errorf("Expected content length %d, got %v", archive.Size, objInfo.ContentLength)
	}

	if objInfo.ETag == nil || *objInfo.ETag == "" {
		t.Error("ETag should not be empty")
	}

	if objInfo.LastModified == nil {
		t.Error("LastModified should not be nil")
	}

	// Verify metadata
	expectedMetadata := map[string]string{
		"cargoship-created-by":      "cargoship",
		"cargoship-compression-type": "gzip",
		"cargoship-access-pattern":  "rare",
		"cargoship-retention-days":  "180",
		"project":                   "cargoship-test",
		"environment":               "integration",
	}

	for key, expectedValue := range expectedMetadata {
		if objInfo.Metadata[key] != expectedValue {
			t.Errorf("Expected metadata %s=%s, got %s", key, expectedValue, objInfo.Metadata[key])
		}
	}
}

func TestParallelUploaderIntegration(t *testing.T) {
	transporter := getTestTransporter()
	
	config := ParallelConfig{
		MaxPrefixes:          2,
		MaxConcurrentUploads: 2,
		PrefixPattern:        "sequential",
		LoadBalancing:        "round_robin",
	}

	uploader := NewParallelUploader(transporter, config)
	ctx := context.Background()

	// Create test archives
	archives := []Archive{
		{
			Key:    "archive-1.txt",
			Reader: bytes.NewReader([]byte("Content of archive 1")),
			Size:   20,
		},
		{
			Key:    "archive-2.txt", 
			Reader: bytes.NewReader([]byte("Content of archive 2")),
			Size:   20,
		},
		{
			Key:    "archive-3.txt",
			Reader: bytes.NewReader([]byte("Content of archive 3")),
			Size:   20,
		},
		{
			Key:    "archive-4.txt",
			Reader: bytes.NewReader([]byte("Content of archive 4")),
			Size:   20,
		},
	}

	// Test parallel upload
	result, err := uploader.UploadParallel(ctx, archives)

	if err != nil {
		t.Errorf("Unexpected error in parallel upload: %v", err)
	}

	if result == nil {
		t.Fatal("Expected parallel upload result, got nil")
	}

	// Verify results
	if result.TotalUploaded != int64(len(archives)) {
		t.Errorf("Expected %d uploads, got %d", len(archives), result.TotalUploaded)
	}

	if result.TotalErrors != 0 {
		t.Errorf("Expected 0 errors, got %d", result.TotalErrors)
	}

	if len(result.Results) != len(archives) {
		t.Errorf("Expected %d results, got %d", len(archives), len(result.Results))
	}

	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}

	if result.AverageThroughputMBps <= 0 {
		t.Error("Average throughput should be positive")
	}

	// Verify all objects exist with correct prefixes
	for _, archive := range archives {
		// Check both possible prefixes (since round-robin distribution)
		prefixes := []string{"archives/batch-0000/", "archives/batch-0001/"}
		
		found := false
		for _, prefix := range prefixes {
			key := prefix + archive.Key
			exists, err := transporter.Exists(ctx, key)
			if err == nil && exists {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Archive %s not found with any expected prefix", archive.Key)
		}
	}

	// Verify prefix stats
	if len(result.PrefixStats) == 0 {
		t.Error("Expected prefix stats to be populated")
	}

	for prefix, stats := range result.PrefixStats {
		if stats.UploadCount <= 0 {
			t.Errorf("Prefix %s should have positive upload count", prefix)
		}
		if stats.TotalBytes <= 0 {
			t.Errorf("Prefix %s should have positive total bytes", prefix)
		}
	}
}

func TestParallelUploaderEmptyInput(t *testing.T) {
	transporter := getTestTransporter()
	uploader := NewParallelUploader(transporter, ParallelConfig{})
	ctx := context.Background()

	result, err := uploader.UploadParallel(ctx, []Archive{})

	if err != nil {
		t.Errorf("Unexpected error with empty input: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result even with empty input")
	}

	if result.TotalUploaded != 0 {
		t.Errorf("Expected 0 uploads for empty input, got %d", result.TotalUploaded)
	}

	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results for empty input, got %d", len(result.Results))
	}
}

func TestUploadStorageClassOptimization(t *testing.T) {
	transporter := getTestTransporter()
	ctx := context.Background()

	testCases := []struct {
		name            string
		accessPattern   string
		retentionDays   int
		expectedClass   types.StorageClass
	}{
		{
			name:            "deep archive for long-term",
			accessPattern:   "archive",
			retentionDays:   400,
			expectedClass:   types.StorageClassDeepArchive,
		},
		{
			name:            "glacier for rare access",
			accessPattern:   "rare",
			retentionDays:   100,
			expectedClass:   types.StorageClassGlacier,
		},
		{
			name:            "standard-ia for infrequent",
			accessPattern:   "infrequent",
			retentionDays:   30,
			expectedClass:   types.StorageClassStandardIa,
		},
		{
			name:            "intelligent tiering for unknown",
			accessPattern:   "unknown",
			retentionDays:   60,
			expectedClass:   types.StorageClassIntelligentTiering,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			archive := Archive{
				Key:           fmt.Sprintf("storage-class-test/%s.txt", tc.name),
				Reader:        bytes.NewReader([]byte("test content")),
				Size:          12,
				AccessPattern: tc.accessPattern,
				RetentionDays: tc.retentionDays,
			}

			result, err := transporter.Upload(ctx, archive)
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}

			if result.StorageClass != tc.expectedClass {
				t.Errorf("Expected storage class %s, got %s", tc.expectedClass, result.StorageClass)
			}
		})
	}
}

func BenchmarkUploadIntegration(b *testing.B) {
	transporter := getTestTransporter()
	ctx := context.Background()

	// Create test data
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		archive := Archive{
			Key:    fmt.Sprintf("benchmark/upload-%d.dat", i),
			Reader: bytes.NewReader(data),
			Size:   int64(len(data)),
		}

		_, err := transporter.Upload(ctx, archive)
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}
	}
}

func BenchmarkParallelUploadIntegration(b *testing.B) {
	transporter := getTestTransporter()
	config := ParallelConfig{
		MaxPrefixes:          2,
		MaxConcurrentUploads: 2,
	}
	uploader := NewParallelUploader(transporter, config)
	ctx := context.Background()

	// Create test archives
	data := make([]byte, 512*1024) // 512KB each
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		archives := []Archive{
			{
				Key:    fmt.Sprintf("parallel-bench/file-1-%d.dat", i),
				Reader: bytes.NewReader(data),
				Size:   int64(len(data)),
			},
			{
				Key:    fmt.Sprintf("parallel-bench/file-2-%d.dat", i),
				Reader: bytes.NewReader(data),
				Size:   int64(len(data)),
			},
		}

		_, err := uploader.UploadParallel(ctx, archives)
		if err != nil {
			b.Fatalf("Parallel upload failed: %v", err)
		}
	}
}