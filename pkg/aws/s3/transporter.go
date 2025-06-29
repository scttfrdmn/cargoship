// Package s3 provides AWS S3 transport implementation for CargoShip
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship-cli/pkg/aws/config"
)

// Transporter implements S3-based transport for CargoShip
type Transporter struct {
	client   *s3.Client
	uploader *manager.Uploader
	config   awsconfig.S3Config
}

// Archive represents a CargoShip archive for upload
type Archive struct {
	Key              string            // S3 object key
	Reader           io.Reader         // Archive content
	Size             int64             // Archive size in bytes
	StorageClass     awsconfig.StorageClass // Target storage class
	Metadata         map[string]string // Custom metadata
	OriginalSize     int64             // Original uncompressed size
	CompressionType  string            // Compression algorithm used
	AccessPattern    string            // Expected access pattern
	RetentionDays    int               // Expected retention period
}

// UploadResult contains the result of an S3 upload
type UploadResult struct {
	Location     string        // S3 URL
	Key          string        // S3 object key
	ETag         string        // S3 ETag
	UploadID     string        // Multipart upload ID (if used)
	Duration     time.Duration // Upload duration
	Throughput   float64       // Upload throughput in MB/s
	StorageClass types.StorageClass // Actual storage class used
}

// NewTransporter creates a new S3 transporter
func NewTransporter(client *s3.Client, config awsconfig.S3Config) *Transporter {
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = config.MultipartChunkSize
		u.Concurrency = config.Concurrency
		u.LeavePartsOnError = false // Clean up failed uploads
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(25 * 1024 * 1024) // 25MB buffer pool
	})

	return &Transporter{
		client:   client,
		uploader: uploader,
		config:   config,
	}
}

// Upload uploads an archive to S3 with intelligent storage class selection
func (t *Transporter) Upload(ctx context.Context, archive Archive) (*UploadResult, error) {
	startTime := time.Now()
	
	// Optimize storage class based on archive characteristics
	storageClass := t.optimizeStorageClass(archive)
	
	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:       aws.String(t.config.Bucket),
		Key:          aws.String(archive.Key),
		Body:         archive.Reader,
		StorageClass: storageClass,
		Metadata:     t.buildMetadata(archive),
	}
	
	// Add KMS encryption if configured
	if t.config.KMSKeyID != "" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(t.config.KMSKeyID)
	}
	
	// Perform upload
	result, err := t.uploader.Upload(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to upload archive: %w", err)
	}
	
	duration := time.Since(startTime)
	throughput := float64(archive.Size) / duration.Seconds() / (1024 * 1024) // MB/s
	
	return &UploadResult{
		Location:     result.Location,
		Key:          archive.Key,
		ETag:         aws.ToString(result.ETag),
		UploadID:     result.UploadID,
		Duration:     duration,
		Throughput:   throughput,
		StorageClass: storageClass,
	}, nil
}

// optimizeStorageClass selects the optimal storage class based on archive characteristics
func (t *Transporter) optimizeStorageClass(archive Archive) types.StorageClass {
	// Use configured default if no optimization criteria
	if archive.AccessPattern == "" && archive.RetentionDays == 0 {
		return types.StorageClass(t.config.StorageClass)
	}
	
	// Deep Archive for long-term archival with no expected access
	if archive.AccessPattern == "archive" && archive.RetentionDays > 365 {
		return types.StorageClassDeepArchive
	}
	
	// Glacier for long-term storage with rare access
	if archive.RetentionDays > 90 || archive.AccessPattern == "rare" {
		return types.StorageClassGlacier
	}
	
	// Standard-IA for infrequent access
	if archive.AccessPattern == "infrequent" {
		return types.StorageClassStandardIa
	}
	
	// Intelligent Tiering for unknown access patterns
	if archive.AccessPattern == "unknown" || archive.AccessPattern == "" {
		return types.StorageClassIntelligentTiering
	}
	
	// Default to Standard for frequent access
	return types.StorageClassStandard
}

// buildMetadata creates S3 metadata from archive information
func (t *Transporter) buildMetadata(archive Archive) map[string]string {
	metadata := make(map[string]string)
	
	// Copy custom metadata
	for k, v := range archive.Metadata {
		metadata[k] = v
	}
	
	// Add CargoShip-specific metadata
	metadata["cargoship-original-size"] = strconv.FormatInt(archive.OriginalSize, 10)
	metadata["cargoship-compression-type"] = archive.CompressionType
	metadata["cargoship-created-by"] = "cargoship"
	metadata["cargoship-upload-time"] = time.Now().UTC().Format(time.RFC3339)
	
	if archive.AccessPattern != "" {
		metadata["cargoship-access-pattern"] = archive.AccessPattern
	}
	
	if archive.RetentionDays > 0 {
		metadata["cargoship-retention-days"] = strconv.Itoa(archive.RetentionDays)
	}
	
	return metadata
}

// Exists checks if an object exists in S3
func (t *Transporter) Exists(ctx context.Context, key string) (bool, error) {
	_, err := t.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(t.config.Bucket),
		Key:    aws.String(key),
	})
	
	if err != nil {
		// Check if it's a "not found" error
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, err
	}
	
	return true, nil
}

// GetObjectInfo retrieves metadata about an object
func (t *Transporter) GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return t.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(t.config.Bucket),
		Key:    aws.String(key),
	})
}