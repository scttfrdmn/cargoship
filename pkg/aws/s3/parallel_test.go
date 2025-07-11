package s3

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

func TestParallelUploader_GetCoordinationMetrics(t *testing.T) {
	client := &s3.Client{}
	s3Config := awsconfig.S3Config{
		Bucket:             "test-bucket",
		MultipartChunkSize: 16 * 1024 * 1024,
		Concurrency:        4,
	}

	transporter := NewTransporter(client, s3Config)

	config := ParallelConfig{
		MaxPrefixes:        8,
		PrefixPattern:      "hash",
		EnableCoordination: true,
		CoordinationConfig: DefaultCoordinationConfig(),
	}

	uploader := NewParallelUploader(transporter, config)
	assert.NotNil(t, uploader)

	metrics := uploader.GetCoordinationMetrics()
	assert.NotNil(t, metrics)
}

func TestParallelUploader_Close(t *testing.T) {
	client := &s3.Client{}
	s3Config := awsconfig.S3Config{
		Bucket:             "test-bucket",
		MultipartChunkSize: 16 * 1024 * 1024,
		Concurrency:        4,
	}

	transporter := NewTransporter(client, s3Config)

	config := ParallelConfig{
		MaxPrefixes:              8,
		PrefixPattern:            "hash",
		EnableCoordination:       true,
		CoordinationConfig: DefaultCoordinationConfig(),
	}

	uploader := NewParallelUploader(transporter, config)
	assert.NotNil(t, uploader)

	err := uploader.Close()
	assert.NoError(t, err)
}

func TestParallelUploader_UpdateCoordinationMetrics(t *testing.T) {
	client := &s3.Client{}
	s3Config := awsconfig.S3Config{
		Bucket:             "test-bucket",
		MultipartChunkSize: 16 * 1024 * 1024,
		Concurrency:        4,
	}

	transporter := NewTransporter(client, s3Config)

	config := ParallelConfig{
		MaxPrefixes:              8,
		PrefixPattern:            "hash",
		EnableCoordination:       true,
		CoordinationConfig: DefaultCoordinationConfig(),
	}

	uploader := NewParallelUploader(transporter, config)
	assert.NotNil(t, uploader)

	// Test updating coordination metrics with correct parameters
	uploadResult := &UploadResult{
		Location:   "s3://test-bucket/test-key",
		Key:        "test-key",
		ETag:       "test-etag",
		Duration:   time.Second * 5,
		Throughput: 50.0,
	}
	uploader.updateCoordinationMetrics("test-prefix", uploadResult, time.Second*5, nil)
	// Should not panic and should update internal metrics
}

func TestParallelUploader_UploadPrefixBatchCoordinated(t *testing.T) {
	client := &s3.Client{}
	s3Config := awsconfig.S3Config{
		Bucket:             "test-bucket",
		MultipartChunkSize: 16 * 1024 * 1024,
		Concurrency:        4,
	}

	transporter := NewTransporter(client, s3Config)

	config := ParallelConfig{
		MaxPrefixes:              8,
		PrefixPattern:            "hash",
		EnableCoordination:       true,
		CoordinationConfig: DefaultCoordinationConfig(),
	}

	uploader := NewParallelUploader(transporter, config)
	assert.NotNil(t, uploader)

	// Create test archive
	archive := Archive{
		Key:             "test-archive.tar.gz",
		Size:            1024,
		CompressionType: "gzip",
		Reader:          strings.NewReader("test archive content"),
	}

	prefixBatch := PrefixBatch{
		Prefix:   "test-prefix",
		Archives: []Archive{archive},
	}

	ctx := context.Background()
	
	// Test upload prefix batch coordinated (will fail without proper AWS setup but tests function exists)
	_, err := uploader.uploadPrefixBatchCoordinated(ctx, prefixBatch)
	
	// Expect error due to AWS config but function should be callable
	assert.Error(t, err)
}

func TestParallelUploader_MinFunction(t *testing.T) {
	// Test the utility min function
	result := min(5, 3)
	assert.Equal(t, 3, result)

	result = min(10, 15)
	assert.Equal(t, 10, result)

	result = min(7, 7)
	assert.Equal(t, 7, result)
}