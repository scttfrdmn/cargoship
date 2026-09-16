//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestWriterIsolationRoundTrip proves writer isolation (#520) on the real byte path:
// an upload scoped to a writer id lands under writers/<id>/uploads/ and restores
// byte-identical. It runs against the in-process Substrate emulator by default (via
// TestMain) and against REAL S3 in the real-aws-integration lane
// (CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1) — the same pipeline that `cargoship
// ghostship run` and `cargoship sync` drive.
//
// Manual real-S3 run:
//
//	CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 CARGOSHIP_TEST_BUCKET=<bucket> AWS_REGION=<r> \
//	  go test -tags integration -run TestWriterIsolationRoundTrip ./pkg/pipeline/ -count=1 -v
func TestWriterIsolationRoundTrip(t *testing.T) {
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	src := t.TempDir()
	rng := rand.New(rand.NewSource(520))
	corpus := plantHostileCorpus(t, src, rng, 4096)
	require.NotEmpty(t, corpus)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)
	var s3Opts []func(*s3.Options)
	if substrateURL != "" { // emulator path-style; empty ⇒ real AWS
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)

	base := fmt.Sprintf("writer-iso-%d", time.Now().UnixNano())
	const writerID = "lab-nas-1"
	effPrefix := WriterPrefix(base, writerID) // base/writers/lab-nas-1
	uploadID := fmt.Sprintf("%d-wiso", time.Now().UnixNano())

	// Clean up every object under the unique base prefix, even on failure. Never
	// touches the bucket itself.
	t.Cleanup(func() {
		pg := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket), Prefix: aws.String(base + "/"),
		})
		for pg.HasMorePages() {
			page, perr := pg.NextPage(ctx)
			if perr != nil {
				return
			}
			for _, o := range page.Contents {
				_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: o.Key})
			}
		}
	})

	pc := &PipelineConfig{
		ScannerWorkers: 4, ArchiverWorkers: 4, UploaderWorkers: 4,
		S3Bucket: bucket, S3Prefix: effPrefix, S3Region: region,
		WriterID:  writerID,
		UseRealS3: true, S3Client: s3Client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, SourcePath: src, UploadID: uploadID,
		EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
	}
	p, err := NewPipeline(pc)
	require.NoError(t, err)
	result, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, result.Success, "upload should succeed")

	// Objects must physically live under writers/<id>/.
	layoutPrefix := fmt.Sprintf("%s/writers/%s/uploads/", base, writerID)
	lst, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(layoutPrefix), MaxKeys: aws.Int32(1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, lst.Contents, "expected objects under %s", layoutPrefix)

	// Fetch the manifest from the FOLDED (writer-scoped) prefix — not from base.
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", effPrefix, uploadID)
	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(manifestKey)})
	require.NoError(t, err, "manifest should be under the writer prefix")
	mBytes, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	_ = obj.Body.Close()
	m, err := manifest.FromJSONCompressed(mBytes)
	require.NoError(t, err)
	require.Equal(t, int64(len(corpus)), m.TotalFiles, "manifest file count should match corpus")
	require.Equal(t, writerID, m.WriterID, "manifest should record the writer id")

	// Restore all files and verify byte-identity by RELATIVE PATH.
	outDir := t.TempDir()
	se := manifest.NewSelectiveExtractor(m, s3Client, 0)
	targets := make([]string, len(corpus))
	for i, f := range corpus {
		targets[i] = f.relPath
	}
	stats, err := se.BatchRestore(ctx, targets, outDir)
	require.NoError(t, err)
	require.Equal(t, int64(len(corpus)), stats.Restored, "every file should restore (failed=%d)", stats.Failed)
	require.Zero(t, stats.Failed)
	for _, want := range corpus {
		got, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(want.relPath)))
		require.NoError(t, err, "restored file missing: %s", want.relPath)
		require.Equal(t, want.size, len(got), "size mismatch: %s", want.relPath)
		require.Equal(t, want.sum, sha256hex(got),
			"BYTE MISMATCH after writer-scoped round-trip: %s (integrity invariant failed)", want.relPath)
	}

	// Report marker: surfaced by scripts/ci/verification-report.sh as a distinct line,
	// deliberately NOT summed into the fixed-corpus files/bytes totals.
	t.Logf("VERIFICATION_WRITER_ISO writer_id=%s files=%d prefix=%s", writerID, len(corpus), effPrefix)
	t.Logf("writer-isolation round-trip OK: %d files byte-identical under %s", len(corpus), layoutPrefix)
}
