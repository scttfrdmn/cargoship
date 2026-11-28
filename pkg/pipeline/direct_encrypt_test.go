package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestRequireChunkedUpload pins #1037: once a config requires chunked upload,
// direct mode is never chosen, whatever the workload or the direct flags.
func TestRequireChunkedUpload(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  PipelineConfig
	}{
		{"automatic", PipelineConfig{}},
		{"--direct-upload", PipelineConfig{EnableDirectUpload: true}},
		{"--force-direct-upload", PipelineConfig{ForceDirectUpload: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.S3Client, cfg.S3Bucket = &s3.Client{}, "b"
			p, err := NewPipeline(&cfg)
			require.NoError(t, err)
			defer p.cancel()
			require.True(t, p.shouldUseDirectUpload(3, 3*1024), "a tiny workload takes the fast path by default")

			cfg = tc.cfg
			cfg.S3Client, cfg.S3Bucket = &s3.Client{}, "b"
			RequireChunkedUpload(&cfg)
			p, err = NewPipeline(&cfg)
			require.NoError(t, err)
			defer p.cancel()
			assert.False(t, p.shouldUseDirectUpload(3, 3*1024), "direct mode must never be chosen")
			assert.False(t, p.shouldUseDirectUpload(1, 1), "direct mode must never be chosen")
		})
	}
}

// TestDirectUploader_NoPathMetadata pins the metadata half of #1037: direct
// objects no longer carry cargoship-file-path, which held the file's absolute
// local path and exposed every name to anyone who can HeadObject.
func TestDirectUploader_NoPathMetadata(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	p := filepath.Join(src, "secret-filename.txt")
	require.NoError(t, os.WriteFile(p, []byte("cargoship-test"), 0o600))
	client := &mockS3Client{}
	in, out := make(chan *Job, 1), make(chan *Job, 1)
	up, err := NewDirectUploaderStage(&DirectUploaderConfig{
		S3Client: client, Bucket: "b", SourcePath: src, UploadID: "u",
		Workers: 1, MaxRetries: 1, RetryDelay: time.Millisecond,
		WorkerPool: NewAdaptiveWorkerPool(ctx, &AdaptiveWorkerPoolConfig{InitialWorkers: 1, MaxWorkers: 1}),
	}, in, out)
	require.NoError(t, err)
	require.NoError(t, up.Start(ctx))
	in <- &Job{ID: 1, Chunk: chunking.Chunk{ID: 1, Files: []chunking.File{{Path: p, Size: int64(len("cargoship-test"))}}}}
	close(in)
	<-out
	_ = up.Stop()

	require.NotNil(t, client.last)
	assert.NotContains(t, client.last.Metadata, "cargoship-file-path")
	for k, v := range client.last.Metadata {
		assert.NotContains(t, v, "secret-filename", "metadata %s carries the file name", k)
	}
	assert.Equal(t, "direct", client.last.Metadata["cargoship-mode"])
}
