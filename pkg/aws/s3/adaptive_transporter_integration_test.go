//go:build integration

package s3

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// createSubstrateS3Client returns an S3 client pointed at the in-process Substrate
// emulator started by TestMain in integration_test.go.
func createSubstrateS3Client(t *testing.T) *s3.Client {
	t.Helper()
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(substrateURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
}

// createSubstrateTestBucket creates a bucket in Substrate and registers cleanup.
func createSubstrateTestBucket(t *testing.T, client *s3.Client, bucketName string) {
	t.Helper()
	ctx := context.Background()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Logf("Warning: Could not create test bucket (may already exist): %v", err)
	}

	t.Cleanup(func() {
		listResp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
		})
		if err == nil {
			for _, obj := range listResp.Contents {
				_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucketName),
					Key:    obj.Key,
				})
			}
		}
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucketName),
		})
	})
}

func TestAdaptiveTransporter_UploadWithAdaptation(t *testing.T) {
	ctx := context.Background()
	client := createSubstrateS3Client(t)
	bucketName := "test-adaptive-bucket"

	createSubstrateTestBucket(t, client, bucketName)

	s3Config := awsconfig.S3Config{
		Bucket:             bucketName,
		MultipartChunkSize: 16 * 1024 * 1024,
		Concurrency:        4,
	}

	config := DefaultAdaptiveTransporterConfig()
	config.EnableRealTimeAdaptation = false

	at, err := NewAdaptiveTransporter(ctx, client, s3Config, config, nil)
	assert.NoError(t, err)

	archive := Archive{
		Key:             "test-archive.tar.gz",
		Size:            1024,
		CompressionType: "gzip",
		Reader:          strings.NewReader("test archive content for adaptive upload"),
	}

	result, err := at.UploadWithAdaptation(ctx, archive)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Location)
	assert.Greater(t, result.Duration, time.Duration(0))
}
