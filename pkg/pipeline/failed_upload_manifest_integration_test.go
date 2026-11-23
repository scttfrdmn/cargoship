//go:build integration

package pipeline

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// failOneChunk fails every PUT to the first chunk object it sees, so exactly one
// chunk fails however the SDK retries, and the others succeed.
type failOneChunk struct {
	next   http.RoundTripper
	mu     sync.Mutex
	victim string
	failed atomic.Int64
}

func (f *failOneChunk) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/chunk-") {
		f.mu.Lock()
		if f.victim == "" {
			f.victim = r.URL.Path
		}
		hit := f.victim == r.URL.Path
		f.mu.Unlock()
		if hit {
			f.failed.Add(1)
			return (&failingChunkPuts{fail: 1}).RoundTrip(r)
		}
	}
	return f.next.RoundTrip(r)
}

// TestUpload_FailedChunkWritesNoFinalManifest pins #902. Before the fix a run with a
// failed chunk still wrote manifest.json.gz, listing that chunk's files with an empty
// S3Key, and deleted the partial manifest. The next sync's delta then reported every
// file unchanged, so the failed files were never uploaded, and resume had nothing to
// resume from.
func TestUpload_FailedChunkWritesNoFinalManifest(t *testing.T) {
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

	transport := &failOneChunk{next: http.DefaultTransport}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if substrateURL != "" {
			o.UsePathStyle = true
		}
		o.HTTPClient = &http.Client{Transport: transport}
	})
	plain := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" })

	// ~6 MB of incompressible data in 1 MB chunks: several chunks, so some
	// succeed and some fail.
	src := t.TempDir()
	for i := 0; i < 20; i++ {
		buf := make([]byte, 300*1024)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(src, fmt.Sprintf("file%02d.dat", i)), buf, 0o600))
	}
	prefix := fmt.Sprintf("failed-manifest-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID:                    uploadID,
		EnableManifest:              true,
		EnablePartialManifest:       true,
		PartialManifestSaveInterval: time.Hour, // only the on-failure snapshot
		SourcePath:                  src,
		DirectUploadMaxFiles:        1,
		ChunkingConfig: &chunking.ChunkingConfig{
			Workers:         2,
			AvailableMemory: 1 << 30,
			TargetChunkSize: 1 << 20,
			MinChunkSize:    512 << 10,
			MaxChunkSize:    1 << 20,
		},
		EnableMultiPrefix: true,
		ShardCount:        4,
		WorkersPerPrefix:  2,
	})
	require.NoError(t, err)
	result, runErr := p.Run(ctx, src)
	require.NoError(t, runErr)
	require.Positive(t, transport.failed.Load(), "the injected failure must have fired")
	require.False(t, result.Success, "a run with a failed chunk must not succeed")
	require.Greater(t, result.ChunksCreated, 1, "the test needs more than one chunk")

	keys := map[string]int64{}
	out, err := plain.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	require.NoError(t, err)
	for _, obj := range out.Contents {
		keys[aws.ToString(obj.Key)] = aws.ToInt64(obj.Size)
	}

	final := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
	partial := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", prefix, uploadID)
	_, hasFinal := keys[final]
	assert.False(t, hasFinal, "a failed run must not write its final manifest; objects: %v", keys)
	_, hasPartial := keys[partial]
	assert.True(t, hasPartial, "a failed run must keep a partial manifest for resume; objects: %v", keys)

	// If a manifest did land, the next sync must still see the failed files as
	// needing upload, which is what #902 broke.
	if hasFinal {
		m, err := manifest.DownloadFromS3(ctx, plain, bucket, prefix, uploadID)
		require.NoError(t, err)
		local, err := manifest.ScanLocalFiles(src)
		require.NoError(t, err)
		delta, err := manifest.ComputeDelta(local, m, &manifest.SyncOptions{})
		require.NoError(t, err)
		assert.True(t, delta.HasChanges(), "the files of the failed chunk must be uploaded again")
	}
}
