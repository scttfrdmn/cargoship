//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingChunkPuts reads the whole body of the first `fail` chunk PUTs and
// answers 500, as a backend that errors after receiving the upload would. That
// exhausts the SDK's own retries on the first outer attempt AFTER the one-shot
// archive stream has been consumed, which is the #901 precondition.
type failingChunkPuts struct {
	next   http.RoundTripper
	fail   int64
	failed atomic.Int64
}

func (f *failingChunkPuts) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/chunk-") && f.failed.Add(1) <= f.fail {
		if r.Body != nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
		body := `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code><Message>injected</Message></Error>`
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     http.Header{"Content-Type": []string{"application/xml"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}
	return f.next.RoundTrip(r)
}

// TestUpload_RetryAfterConsumedStreamFailsInsteadOfStoringEmptyChunk pins #901.
// Before the fix the outer retry re-read the drained pipe, S3 stored a 0-byte
// chunk, and the upload reported success with the chunk's full size recorded.
func TestUpload_RetryAfterConsumedStreamFailsInsteadOfStoringEmptyChunk(t *testing.T) {
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

	// Fail every SDK attempt of the first outer attempt (default retryer: 3).
	transport := &failingChunkPuts{next: http.DefaultTransport, fail: 3}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if substrateURL != "" {
			o.UsePathStyle = true
		}
		o.HTTPClient = &http.Client{Transport: transport}
	})
	plain := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" })

	src, cleanup := createTestFiles(t, 20, 10*1024) // one chunk
	defer cleanup()
	prefix := fmt.Sprintf("retry-consumed-%d", time.Now().UnixNano())

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest:       true,
		SourcePath:           src,
		DirectUploadMaxFiles: 1, // force the chunked path
		EnableMultiPrefix:    true,
		ShardCount:           4,
		WorkersPerPrefix:     2,
	})
	require.NoError(t, err)
	result, runErr := p.Run(ctx, src)
	require.GreaterOrEqual(t, transport.failed.Load(), int64(3), "the injected failures must have fired")

	// The run must not report success...
	succeeded := runErr == nil && result != nil && result.Success
	assert.False(t, succeeded, "an upload whose chunk could not be sent must not report success")

	// ...and S3 must hold no empty chunk object.
	out, err := plain.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	require.NoError(t, err)
	for _, obj := range out.Contents {
		if strings.Contains(aws.ToString(obj.Key), "/chunk-") {
			assert.NotZero(t, aws.ToInt64(obj.Size), "chunk object %s is empty", aws.ToString(obj.Key))
		}
	}
}
