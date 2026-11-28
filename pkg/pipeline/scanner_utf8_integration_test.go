//go:build integration

package pipeline

import (
	"context"
	"errors"
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

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestUpload_NonUTF8NameFailsWithoutManifest pins #1003 end to end. An upload
// with a Latin-1 name used to succeed and write a manifest whose entry for it
// held U+FFFD, so restore failed for that file and the next sync saw it as New.
// Now the run fails, names the file, and writes no final manifest.
func TestUpload_NonUTF8NameFailsWithoutManifest(t *testing.T) {
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" })

	src := t.TempDir()
	writeRawName(t, src, "caf\xe9.txt")
	writePrefixedFiles(t, filepath.Join(src, "good"), "f", 20, 1024)

	prefix := fmt.Sprintf("non-utf8-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, EnablePartialManifest: true,
		PartialManifestSaveInterval: time.Hour, CleanupOnFailure: true,
		SourcePath: src, DirectUploadMaxFiles: 1,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers: 2, AvailableMemory: 1 << 30,
			TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
		},
		EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
	})
	require.NoError(t, err)

	result, runErr := p.Run(ctx, src)
	var bad *manifest.NonUTF8PathsError
	require.True(t, errors.As(runErr, &bad), "want a NonUTF8PathsError, got %v", runErr)
	assert.Equal(t, []string{"caf\xe9.txt"}, bad.Paths)
	assert.False(t, result.Success)

	final := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
	_, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(final)})
	assert.True(t, isNoSuchKeyErr(headErr), "no final manifest after a refused name (HeadObject err=%v)", headErr)
}
