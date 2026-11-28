//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestUpload_PartialManifestSavedEveryMillisecond is the audit's reproduction for
// #1008: a real upload with the partial manifest saved every millisecond. Under
// -race it reported the save's JSON encoding racing AddChunk; the default 30 s
// interval hid it from the suite. It must be race-free and upload every file.
func TestUpload_PartialManifestSavedEveryMillisecond(t *testing.T) {
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
	writePrefixedFiles(t, src, "f", 3000, 4096)
	prefix := fmt.Sprintf("partial-race-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 4, UploaderWorkers: 4,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, EnablePartialManifest: true,
		PartialManifestSaveInterval: time.Millisecond,
		SourcePath:                  src, DirectUploadMaxFiles: 1,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers: 2, AvailableMemory: 1 << 30,
			TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
		},
		EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
	})
	require.NoError(t, err)
	result, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, result.Success, "errors: %v", result.Errors)
	assert.Greater(t, result.ChunksUploaded, 1, "the saves need chunks to overlap")

	m, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	assert.Len(t, m.Files, 3000)
}
