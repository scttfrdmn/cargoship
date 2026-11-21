//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestRestore_EmptyChunkObjectFailsLoudly is the #905 end-to-end check against a
// manifest written by the CURRENT pipeline: replace an uploaded chunk with the
// 0-byte object an upload retry can leave behind (#901), and restore must count
// that chunk's files as failed with a size reason, not report a clean restore.
//
// It also proves the current writer records a CompressedSize that restore can
// trust (format >= 2.1 plus a whole-object checksum or frames); otherwise the
// size reason below would never appear.
func TestRestore_EmptyChunkObjectFailsLoudly(t *testing.T) {
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
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	client := s3.NewFromConfig(cfg, s3Opts...)

	src, cleanup := createTestFiles(t, 20, 10*1024)
	defer cleanup()
	prefix := fmt.Sprintf("restore-empty-chunk-%d", time.Now().UnixNano())

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, // as `cargoship upload` sets it
		SourcePath:     src,
		// Force the chunked path (#905 is about chunk objects); 20 small files
		// would otherwise be uploaded directly, one object per file.
		DirectUploadMaxFiles: 1,
		// The multi-prefix uploader, as `cargoship upload` configures it.
		EnableMultiPrefix: true,
		ShardCount:        4,
		WorkersPerPrefix:  2,
	})
	require.NoError(t, err)
	result, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, result.Success)

	m, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, result.UploadID)
	require.NoError(t, err)
	require.NotEmpty(t, m.Chunks)
	victim := m.Chunks[0]

	// Overwrite the chunk with an empty object, as a retry that re-sent a
	// drained stream would.
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(manifest.ResolveObjectKey(m.Prefix, m.Bucket, victim.S3Key)),
		Body:   bytes.NewReader(nil),
	})
	require.NoError(t, err)

	var targets []string
	inVictim := 0
	for _, f := range m.Files {
		targets = append(targets, f.Path)
		if f.S3Key == victim.S3Key {
			inVictim++
		}
	}
	require.Greater(t, inVictim, 0)

	se := manifest.NewSelectiveExtractor(m, client, 0).SetBucket(bucket)
	stats, err := se.BatchRestore(ctx, targets, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, int64(len(targets)-inVictim), stats.Restored, "files in intact chunks still restore")
	assert.Equal(t, int64(inVictim), stats.Failed, "every file in the empty chunk must count as failed")
	found := false
	for _, e := range stats.Errors {
		if strings.Contains(e, "truncated or incomplete upload") {
			found = true
		}
	}
	assert.True(t, found, "a current manifest's chunk size must be trusted and reported; reasons: %v", stats.Errors)
}
