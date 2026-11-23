//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestRebalance_ExecuteKeepsEveryFileRestorable pins #903. Execution extracted
// only the moved files but deleted every affected source chunk, so the files that
// stayed pointed at deleted objects: at the bucket root, 3000 of 3000 files became
// unrestorable. Under a prefix it 404'd instead, because keys were used raw.
func TestRebalance_ExecuteKeepsEveryFileRestorable(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		files        int
		wantDeleted  bool
	}{
		// One chunk over 8 shards: part of it moves, so the source chunk must stay.
		{"bucket root, source chunk keeps files", "", 300, false},
		{"under a prefix, source chunk keeps files", "rebalance/nested", 300, false},
		// Two files over 4 shards: both move, so the vacated chunk is deleted.
		{"every file moved, vacated chunk deleted", "rebalance/vacated", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) { runRebalanceScenario(t, tc.prefix, tc.files, tc.wantDeleted) })
	}
}

func runRebalanceScenario(t *testing.T, basePrefix string, nFiles int, wantDeleted bool) {
	t.Helper()
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
	writeRandomFiles(t, src, "f", nFiles, 16*1024)
	shards := 8
	if nFiles < 8 {
		shards = 4
	}
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	prefix := basePrefix
	if prefix != "" {
		prefix = fmt.Sprintf("%s-%d", basePrefix, time.Now().UnixNano())
	}

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, SourcePath: src, DirectUploadMaxFiles: 1,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers: 2, AvailableMemory: 1 << 30,
			TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
		},
		EnableMultiPrefix: true, ShardCount: shards, WorkersPerPrefix: 2,
	})
	require.NoError(t, err)
	r, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r.Success, "errors: %v", r.Errors)

	m, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	before := len(m.Chunks)

	res, err := RebalanceShards(ctx, m, &RebalanceConfig{
		ImbalanceThreshold: 1.2,
		S3Client:           client, Bucket: bucket, Prefix: prefix, UploadID: uploadID,
		CompressionType: m.CompressionType, StorageClass: types.StorageClassStandard,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Positive(t, res.FilesReassigned, "the scenario must actually move files")

	// The CLI's order: store the manifest, then delete what it no longer references.
	require.NoError(t, m.UploadToS3(ctx, client, true))
	_, errs := DeleteObsoleteChunks(ctx, client, bucket, res.ObsoleteChunkKeys)
	require.Empty(t, errs)
	t.Logf("chunks: %d before, %d after, %d deleted", before, len(m.Chunks), len(res.ObsoleteChunkKeys))
	if wantDeleted {
		require.NotEmpty(t, res.ObsoleteChunkKeys, "a fully vacated chunk should be deleted")
		for _, key := range res.ObsoleteChunkKeys {
			_, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
			assert.Error(t, err, "vacated chunk %s should be gone", key)
		}
	} else {
		assert.Empty(t, res.ObsoleteChunkKeys, "a source chunk that still holds files must not be deleted")
	}

	got, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	v := manifest.NewValidator(got).Validate()
	assert.True(t, v.Valid, "validation errors: %v", v.Errors)
	for _, c := range got.Chunks {
		_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(manifest.ResolveObjectKey(got.Prefix, got.Bucket, c.S3Key)),
		})
		assert.NoError(t, err, "chunk %s listed in the manifest must exist", c.S3Key)
	}

	var targets []string
	for _, f := range got.Files {
		targets = append(targets, f.Path)
	}
	out := t.TempDir()
	stats, err := manifest.NewSelectiveExtractor(got, client, 0).SetBucket(bucket).BatchRestore(ctx, targets, out)
	require.NoError(t, err)
	assert.Equal(t, int64(nFiles), stats.Restored, "reasons: %v", stats.Errors)
	assert.Zero(t, stats.Failed, "reasons: %v", stats.Errors)

	restored := map[string][]byte{}
	require.NoError(t, filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, rerr := os.ReadFile(p)
			restored[filepath.Base(p)] = b
			return rerr
		}
		return err
	}))
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		want, err := os.ReadFile(filepath.Join(src, e.Name()))
		require.NoError(t, err)
		assert.True(t, bytes.Equal(want, restored[e.Name()]), "%s restored wrong or missing", e.Name())
	}
}
