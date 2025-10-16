// Package main demonstrates basic usage of CargoShip as a library for optimized S3 uploads.
//
// This example shows how to integrate CargoShip into your own applications,
// similar to how ObjectFS uses it for FUSE filesystem operations.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	cargos3 "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

func main() {
	// Command-line flags
	bucket := flag.String("bucket", "", "S3 bucket name (required)")
	key := flag.String("key", "test-upload.dat", "S3 object key")
	sizeKB := flag.Int("size", 1024, "File size in KB to upload")
	profile := flag.String("profile", "", "AWS profile to use")
	region := flag.String("region", "us-west-2", "AWS region")
	flag.Parse()

	if *bucket == "" {
		log.Fatal("Error: -bucket flag is required\n\nUsage: basic-upload -bucket my-bucket")
	}

	// Run the example
	if err := run(*bucket, *key, *sizeKB, *profile, *region); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(bucket, key string, sizeKB int, profile, region string) error {
	ctx := context.Background()

	fmt.Printf("CargoShip Basic Upload Example\n")
	fmt.Printf("==============================\n\n")

	// Step 1: Load AWS configuration
	fmt.Println("Step 1: Loading AWS configuration...")
	cfg, err := loadAWSConfig(ctx, profile, region)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	fmt.Printf("✓ AWS config loaded (region: %s)\n\n", region)

	// Step 2: Create S3 client
	fmt.Println("Step 2: Creating S3 client...")
	client := s3.NewFromConfig(cfg)
	fmt.Println("✓ S3 client created")

	// Step 3: Configure CargoShip
	fmt.Println("Step 3: Configuring CargoShip for optimal performance...")
	cargoConfig := cargoconfig.S3Config{
		Bucket:             bucket,
		StorageClass:       cargoconfig.StorageClassIntelligentTiering,
		MultipartThreshold: 5 * 1024 * 1024,  // 5MB - use multipart for files larger than this
		MultipartChunkSize: 10 * 1024 * 1024, // 10MB chunks
		Concurrency:        4,                 // 4 parallel uploads for multipart
	}
	fmt.Printf("✓ CargoShip configured:\n")
	fmt.Printf("  - Storage Class: %s\n", cargoConfig.StorageClass)
	fmt.Printf("  - Multipart Threshold: %.1f MB\n", float64(cargoConfig.MultipartThreshold)/(1024*1024))
	fmt.Printf("  - Chunk Size: %.1f MB\n", float64(cargoConfig.MultipartChunkSize)/(1024*1024))
	fmt.Printf("  - Concurrency: %d\n\n", cargoConfig.Concurrency)

	// Step 4: Create CargoShip transporter
	fmt.Println("Step 4: Creating CargoShip transporter...")
	transporter := cargos3.NewTransporter(client, cargoConfig)
	fmt.Println("✓ Transporter created with optimization enabled")

	// Step 5: Prepare test data
	fmt.Println("Step 5: Preparing test data...")
	data := generateTestData(sizeKB * 1024)
	fmt.Printf("✓ Generated %d KB of test data\n\n", sizeKB)

	// Step 6: Upload with CargoShip optimization
	fmt.Println("Step 6: Uploading with CargoShip optimization...")
	fmt.Printf("Uploading to s3://%s/%s...\n", bucket, key)

	archive := cargos3.Archive{
		Key:             key,
		Reader:          bytes.NewReader(data),
		Size:            int64(len(data)),
		StorageClass:    cargoConfig.StorageClass,
		Metadata:        map[string]string{"source": "cargoship-example"},
		OriginalSize:    int64(len(data)),
		CompressionType: "none",
		AccessPattern:   "unknown", // Let Intelligent Tiering decide
	}

	startTime := time.Now()
	result, err := transporter.Upload(ctx, archive)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	duration := time.Since(startTime)

	// Step 7: Display results
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✓ Upload Complete!")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Location:      %s\n", result.Location)
	fmt.Printf("ETag:          %s\n", result.ETag)
	fmt.Printf("Storage Class: %s\n", result.StorageClass)
	fmt.Printf("Duration:      %v\n", duration)
	fmt.Printf("Throughput:    %.2f MB/s\n", result.Throughput)
	fmt.Printf("Size:          %.2f MB\n", float64(len(data))/(1024*1024))

	if result.UploadID != "" {
		fmt.Printf("Upload ID:     %s (multipart used)\n", result.UploadID)
	}

	// Step 8: Verify upload
	fmt.Println("\nStep 8: Verifying upload...")
	exists, err := transporter.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	if exists {
		fmt.Println("✓ Upload verified successfully!")

		// Get object info
		info, err := transporter.GetObjectInfo(ctx, key)
		if err == nil {
			fmt.Printf("\nObject Info:\n")
			fmt.Printf("  - Size: %d bytes\n", *info.ContentLength)
			fmt.Printf("  - Last Modified: %v\n", *info.LastModified)
			if info.StorageClass != "" {
				fmt.Printf("  - Storage Class: %s\n", info.StorageClass)
			}
		}
	} else {
		return fmt.Errorf("upload verification failed: object not found")
	}

	fmt.Println("\n✓ Example completed successfully!")
	fmt.Printf("\nTo view your uploaded file:\n")
	fmt.Printf("  aws s3 ls s3://%s/%s\n", bucket, key)
	fmt.Printf("\nTo download it:\n")
	fmt.Printf("  aws s3 cp s3://%s/%s ./downloaded-file\n", bucket, key)

	return nil
}

// loadAWSConfig loads AWS configuration with optional profile and region
func loadAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	return config.LoadDefaultConfig(ctx, opts...)
}

// generateTestData creates sample data for upload
func generateTestData(size int) []byte {
	data := make([]byte, size)
	pattern := []byte("CargoShip test data - ")

	// Fill with repeating pattern
	pos := 0
	for pos < size {
		n := copy(data[pos:], pattern)
		pos += n
	}

	return data
}

// Example output:
//
// CargoShip Basic Upload Example
// ==============================
//
// Step 1: Loading AWS configuration...
// ✓ AWS config loaded (region: us-west-2)
//
// Step 2: Creating S3 client...
// ✓ S3 client created
//
// Step 3: Configuring CargoShip for optimal performance...
// ✓ CargoShip configured:
//   - Storage Class: INTELLIGENT_TIERING
//   - Multipart Threshold: 5.0 MB
//   - Chunk Size: 10.0 MB
//   - Concurrency: 4
//
// Step 4: Creating CargoShip transporter...
// ✓ Transporter created with optimization enabled
//
// Step 5: Preparing test data...
// ✓ Generated 1024 KB of test data
//
// Step 6: Uploading with CargoShip optimization...
// Uploading to s3://my-bucket/test-upload.dat...
//
// ==================================================
// ✓ Upload Complete!
// ==================================================
// Location:      https://my-bucket.s3.amazonaws.com/test-upload.dat
// ETag:          "d41d8cd98f00b204e9800998ecf8427e"
// Storage Class: INTELLIGENT_TIERING
// Duration:      1.234s
// Throughput:    0.83 MB/s
// Size:          1.00 MB
//
// Step 8: Verifying upload...
// ✓ Upload verified successfully!
//
// Object Info:
//   - Size: 1048576 bytes
//   - Last Modified: 2025-10-15 20:30:45 +0000 UTC
//   - Storage Class: INTELLIGENT_TIERING
//
// ✓ Example completed successfully!
//
// To view your uploaded file:
//   aws s3 ls s3://my-bucket/test-upload.dat
//
// To download it:
//   aws s3 cp s3://my-bucket/test-upload.dat ./downloaded-file
