//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestCleanupOnFailure_DeletesOwnChunksUnderPrefixOnly pins #906 for the pipeline.
// Cleanup deleted the tracked, prefix-relative chunk keys as if they were full
// keys: under a prefix it missed the run's own chunks while reporting them
// deleted, and it deleted whatever sat at the bucket root under the same key.
// An explicit upload ID makes that collision certain.
func TestCleanupOnFailure_DeletesOwnChunksUnderPrefixOnly(t *testing.T) {
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
	pathStyle := func(o *s3.Options) { o.UsePathStyle = substrateURL != "" }
	plain := s3.NewFromConfig(cfg, pathStyle)
	failing := &failOneChunk{next: http.DefaultTransport}
	flaky := s3.NewFromConfig(cfg, pathStyle, func(o *s3.Options) { o.HTTPClient = &http.Client{Transport: failing} })

	src := t.TempDir()
	writeRandomFiles(t, src, "f", 300, 40*1024)
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("cleanup-prefix-%d/nested", time.Now().UnixNano())

	// Decoys at the bucket root under every key a chunk of this run could use.
	var decoys []string
	for shard := 0; shard < 4; shard++ {
		for id := 0; id < 16; id++ {
			key := fmt.Sprintf("uploads/%s/shard-%d/chunk-%d.tar.zst", uploadID, shard, id)
			_, err := plain.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader([]byte("not yours")),
			})
			require.NoError(t, err)
			decoys = append(decoys, key)
		}
	}

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: flaky, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, SourcePath: src, DirectUploadMaxFiles: 1,
		CleanupOnFailure: true,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers: 2, AvailableMemory: 1 << 30,
			TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
		},
		EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
	})
	require.NoError(t, err)
	r, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.False(t, r.Success)
	require.Greater(t, r.ChunksCreated, 1, "some chunks must have uploaded for cleanup to matter")

	// The run's own chunks under the prefix are gone...
	out, err := plain.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix + "/")})
	require.NoError(t, err)
	for _, obj := range out.Contents {
		assert.False(t, strings.Contains(aws.ToString(obj.Key), "/chunk-"),
			"cleanup must delete the run's own chunk %s", aws.ToString(obj.Key))
	}
	// ...and nothing at the bucket root was touched.
	for _, key := range decoys {
		_, err := plain.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		assert.NoError(t, err, "cleanup must not delete the unrelated object %s", key)
	}
}
