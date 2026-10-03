package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestWaitForCompletion_ManifestUploadFailureFailsTheResult guards #704, which is a
// data-protection failure rather than a cost one.
//
// A failed manifest upload used to be a printed warning that left Result.Success true.
// Downstream, runOneSync caches the effective manifest whenever the result is a success,
// so the cache recorded a dataset whose manifest never reached S3. Every later cycle then
// reported "no changes", while the chunks already in S3 had no manifest and were therefore
// unreachable to restore and verify. Uploaded, billed, unrecoverable, and never retried.
//
// Observed on a real Synology: 23 chunk objects (2.49 GiB) present, manifest.json.gz a
// 404, and the next cycle saying "no changes" with 26 GB still unprotected.
func TestWaitForCompletion_ManifestUploadFailureFailsTheResult(t *testing.T) {
	builder, err := manifest.NewBuilder("20261003-test", t.TempDir(), "no-such-bucket", "prefix", "us-east-1")
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// An endpoint that cannot resolve, with retries off, so the manifest PUT fails fast.
	// The failure mode under test is "the upload did not land", whatever the cause.
	cfg := aws.Config{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String("http://127.0.0.1:1"),
		Credentials:      credentials.NewStaticCredentialsProvider("x", "y", ""),
		RetryMaxAttempts: 1,
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })

	p := &Pipeline{
		config: &PipelineConfig{
			UseRealS3: true, // the manifest-upload branch only runs for real S3
			S3Client:  client,
			S3Bucket:  "no-such-bucket",
			S3Prefix:  "prefix",
			UploadID:  "20261003-test",
		},
		manifestBuilder: builder,
		resultChan:      make(chan *Job, 1),
		progress:        &ProgressTracker{progress: Progress{StartTime: time.Now()}},
	}
	// One successfully "uploaded" chunk: the chunks are fine, only the manifest fails.
	p.resultChan <- &Job{Chunk: chunking.Chunk{FileCount: 1}}
	close(p.resultChan)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res := p.waitForCompletion(ctx)

	if res.Success {
		t.Error("Result.Success is true despite the manifest never reaching S3; the cycle " +
			"must fail so the manifest cache is not poisoned and the next cycle retries (#704)")
	}
	if len(res.Errors) == 0 {
		t.Error("the manifest-upload failure must be recorded in Result.Errors, not only printed")
	}
}
