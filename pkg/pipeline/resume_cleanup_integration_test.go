//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
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

// TestResume_FailedResumeKeepsEarlierProgress pins #1009. `upload --resume` runs
// with CleanupOnFailure, and on a failure cleanup deleted this run's chunks plus
// the partial manifest. That manifest is the only record of the chunks earlier
// runs stored, so they were orphaned (billed, unreferenced) and the progress was
// lost. A failed resume must keep a partial manifest that records every chunk
// still in S3, so the next resume can finish the upload.
func TestResume_FailedResumeKeepsEarlierProgress(t *testing.T) {
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
	flaky := func() *s3.Client {
		failing := &failOneChunk{next: http.DefaultTransport}
		return s3.NewFromConfig(cfg, pathStyle, func(o *s3.Options) { o.HTTPClient = &http.Client{Transport: failing} })
	}

	src := t.TempDir()
	writePrefixedFiles(t, src, "a", 200, 40*1024)
	prefix := fmt.Sprintf("resume-cleanup-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())

	run := func(client *s3.Client, resume, cleanup bool) *Result {
		t.Helper()
		resumeID := ""
		if resume {
			resumeID = uploadID
		}
		p, err := NewPipeline(&PipelineConfig{
			ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
			S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
			UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
			UploadID: uploadID, ResumeMode: resume, ResumeUploadID: resumeID,
			EnableManifest: true, EnablePartialManifest: true, PartialManifestSaveInterval: time.Hour,
			CleanupOnFailure: cleanup,
			SourcePath:       src, DirectUploadMaxFiles: 1,
			ChunkingConfig: &chunking.ChunkingConfig{
				Workers: 2, AvailableMemory: 1 << 30,
				TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
			},
			EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
		})
		require.NoError(t, err)
		r, err := p.Run(ctx, src)
		require.NoError(t, err)
		return r
	}
	objects := func() map[string]bool {
		t.Helper()
		keys := map[string]bool{}
		out, err := plain.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix + "/")})
		require.NoError(t, err)
		for _, obj := range out.Contents {
			keys[aws.ToString(obj.Key)] = true
		}
		return keys
	}

	// Run 1, interrupted: one chunk fails and the partial manifest records the rest.
	r1 := run(flaky(), false, false)
	require.False(t, r1.Success)
	p1, err := manifest.DownloadPartialManifestFromS3(ctx, plain, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.NotEmpty(t, p1.Chunks, "run 1 must have stored some chunks")

	// New files give the resume new chunks to upload, so one can fail while others
	// succeed. The resume runs as `upload --resume` does, with cleanup on failure.
	writePrefixedFiles(t, src, "b", 100, 40*1024)
	r2 := run(flaky(), true, true)
	require.False(t, r2.Success)
	require.Positive(t, r2.ChunksUploaded, "the resume must have stored some new chunks before failing")

	p2, err := manifest.DownloadPartialManifestFromS3(ctx, plain, bucket, prefix, uploadID)
	require.NoError(t, err, "a failed resume must keep the partial manifest")
	stored := objects()
	recorded := map[string]bool{}
	for _, c := range p2.Chunks {
		recorded[c.S3Key] = true
		assert.True(t, stored[objectKeyFor(prefix, c.S3Key)], "recorded chunk %s must still be in S3", c.S3Key)
	}
	for _, c := range p1.Chunks {
		assert.True(t, recorded[c.S3Key], "the earlier run's chunk %s must still be recorded", c.S3Key)
	}

	// The next resume finishes the upload, and every file restores intact.
	r3 := run(plain, true, true)
	require.True(t, r3.Success, "final resume errors: %v", r3.Errors)
	m, err := manifest.DownloadFromS3(ctx, plain, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.Len(t, m.Files, 300)
	var targets []string
	for _, f := range m.Files {
		targets = append(targets, f.Path)
	}
	out := t.TempDir()
	stats, err := manifest.NewSelectiveExtractor(m, plain, 0).SetBucket(bucket).BatchRestore(ctx, targets, out)
	require.NoError(t, err)
	assert.Equal(t, int64(300), stats.Restored, "reasons: %v", stats.Errors)
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
