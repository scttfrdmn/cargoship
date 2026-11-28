//go:build integration

package pipeline

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// writePrefixedFiles writes n files of size bytes under dir. Each starts with a
// fixed text prefix, so random bytes can never look like a magic number.
func writePrefixedFiles(t *testing.T, dir, name string, n, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	for i := 0; i < n; i++ {
		buf := make([]byte, size)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		copy(buf, fmt.Sprintf("cargoship-test %s %05d\n", name, i))
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s%05d.dat", name, i)), buf, 0o600))
	}
}

// lockedDir creates dir with one file in it and then removes every permission
// from it, so the walk can't read it. The test is skipped where that can't work.
func lockedDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mode 000 does not make a directory unreadable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read a mode-000 directory")
	}
	writePrefixedFiles(t, dir, "hidden", 1, 64)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) }) //nolint:gosec // restore so TempDir cleanup can remove it
}

// TestUpload_ScannerErrorWritesNoFinalManifest pins #999. A walk error (here an
// unreadable directory) used to leave the final manifest decision to the chunk
// jobs alone: none had failed, so manifest.json.gz was written, listing only the
// files scanned before the error, and the partial manifest and resume state were
// deleted. With CleanupOnFailure the chunks were then deleted too, leaving a
// manifest that points at nothing. Either way the next sync diffed the scanned
// files as unchanged and never uploaded them, and resume was refused because the
// upload looked complete.
//
// The unreadable directory sorts first, so the walk fails before any batch is
// handed to the chunker. That keeps this test about the manifest decision alone,
// clear of the scanner's own exit-path races (#1005, #1006).
func TestUpload_ScannerErrorWritesNoFinalManifest(t *testing.T) {
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

	tests := []struct {
		name             string
		cleanupOnFailure bool
	}{
		// `upload` deletes what it stored when the run fails.
		{name: "cleanup on failure", cleanupOnFailure: true},
		// `sync` keeps it, and the partial manifest, for the next run.
		{name: "keep for resume", cleanupOnFailure: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			lockedDir(t, filepath.Join(src, "000-locked"))
			writePrefixedFiles(t, filepath.Join(src, "data"), "f", 50, 1024)

			prefix := fmt.Sprintf("scanner-error-%d", time.Now().UnixNano())
			uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
			p, err := NewPipeline(&PipelineConfig{
				ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
				S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
				UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
				UploadID:                    uploadID,
				EnableManifest:              true,
				EnablePartialManifest:       true,
				PartialManifestSaveInterval: time.Hour,
				CleanupOnFailure:            tt.cleanupOnFailure,
				SourcePath:                  src,
				DirectUploadMaxFiles:        1,
				ChunkingConfig: &chunking.ChunkingConfig{
					Workers: 2, AvailableMemory: 1 << 30,
					TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
				},
				EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
			})
			require.NoError(t, err)

			result, runErr := p.Run(ctx, src)
			require.Error(t, runErr, "a run whose scan failed must report the scan error")
			require.NotNil(t, result)
			assert.False(t, result.Success, "a run whose scan failed must not succeed")

			final := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
			_, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(final)})
			assert.True(t, isNoSuchKeyErr(headErr),
				"a run whose scan failed must not write its final manifest (HeadObject err=%v)", headErr)
			assert.True(t, p.GetManifest() == nil, "a run whose scan failed must not finalize a manifest")
		})
	}
}
