//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func writeRandomFiles(t *testing.T, dir, prefix string, n, size int) {
	t.Helper()
	for i := 0; i < n; i++ {
		buf := make([]byte, size)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s%03d.dat", prefix, i)), buf, 0o600))
	}
}

// TestResume_AfterSourceChangeRestoresEveryFile pins #904. Resume used to skip any
// chunk whose ID matched a stored chunk, and to bind files to chunks by ID. IDs
// are not stable between runs, so once the source changed, files were bound to
// objects that don't hold them, or not bound at all, while the resume reported
// success. The audit saw 1506 of 2506 files missing after a "successful" resume.
func TestResume_AfterSourceChangeRestoresEveryFile(t *testing.T) {
	tests := []struct {
		name      string
		change    func(t *testing.T, src string)
		wantFiles int
		wantSkips bool
	}{
		{
			name:      "unchanged source skips stored chunks",
			change:    func(*testing.T, string) {},
			wantFiles: 300,
			wantSkips: true,
		},
		{
			// Sorts first, so every later chunk's ID and file set shift.
			name:      "files added before resuming",
			change:    func(t *testing.T, src string) { writeRandomFiles(t, src, "0new", 6, 40*1024) },
			wantFiles: 306,
		},
		{
			// Same size, new content and mtime: its stored chunk must not be reused.
			name: "file modified before resuming",
			change: func(t *testing.T, src string) {
				for i := 0; i < 300; i += 50 {
					p := filepath.Join(src, fmt.Sprintf("a%03d.dat", i))
					buf := make([]byte, 40*1024)
					_, err := rand.Read(buf)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(p, buf, 0o600))
					later := time.Now().Add(time.Duration(i+1) * time.Minute)
					require.NoError(t, os.Chtimes(p, later, later))
				}
			},
			wantFiles: 300,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runResumeScenario(t, tc.change, tc.wantFiles, tc.wantSkips)
		})
	}
}

func runResumeScenario(t *testing.T, change func(*testing.T, string), wantFiles int, wantSkips bool) {
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
	pathStyle := func(o *s3.Options) { o.UsePathStyle = substrateURL != "" }
	plain := s3.NewFromConfig(cfg, pathStyle)
	failing := &failOneChunk{next: http.DefaultTransport}
	flaky := s3.NewFromConfig(cfg, pathStyle, func(o *s3.Options) { o.HTTPClient = &http.Client{Transport: failing} })

	src := t.TempDir()
	writeRandomFiles(t, src, "a", 300, 40*1024)
	prefix := fmt.Sprintf("resume-rebind-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())

	newPipe := func(client *s3.Client, resume bool) *Pipeline {
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
			SourcePath: src, DirectUploadMaxFiles: 1,
			ChunkingConfig: &chunking.ChunkingConfig{
				Workers: 2, AvailableMemory: 1 << 30,
				TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
			},
			EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
		})
		require.NoError(t, err)
		return p
	}

	// Run 1: one chunk fails, so the run stops with a partial manifest.
	r1, err := newPipe(flaky, false).Run(ctx, src)
	require.NoError(t, err)
	require.False(t, r1.Success)
	require.Greater(t, r1.ChunksCreated, 2, "the test needs several chunks")
	partial, err := manifest.DownloadPartialManifestFromS3(ctx, plain, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.NotEmpty(t, partial.Chunks, "run 1 must have stored some chunks")

	change(t, src)

	r2, err := newPipe(plain, true).Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r2.Success, "resume errors: %v", r2.Errors)
	if wantSkips {
		assert.Positive(t, r2.ChunksSkipped, "unchanged stored chunks should be skipped")
	}

	m, err := manifest.DownloadFromS3(ctx, plain, bucket, prefix, uploadID)
	require.NoError(t, err)
	assert.Len(t, m.Files, wantFiles, "every source file exactly once")

	var targets []string
	for _, f := range m.Files {
		targets = append(targets, f.Path)
	}
	out := t.TempDir()
	stats, err := manifest.NewSelectiveExtractor(m, plain, 0).SetBucket(bucket).BatchRestore(ctx, targets, out)
	require.NoError(t, err)
	assert.Equal(t, int64(wantFiles), stats.Restored, "reasons: %v", stats.Errors)
	assert.Zero(t, stats.Failed, "reasons: %v", stats.Errors)

	// Byte-for-byte against the CURRENT source, wherever restore placed each file.
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

// TestResume_RefusesCompletedUpload pins #904's second half: resuming an upload
// that already finished used to run a fresh upload under the same ID, overwriting
// the completed archive.
func TestResume_RefusesCompletedUpload(t *testing.T) {
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
	writeRandomFiles(t, src, "f", 5, 1024)
	prefix := fmt.Sprintf("resume-completed-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	base := PipelineConfig{
		ScannerWorkers: 1, ArchiverWorkers: 1, UploaderWorkers: 1,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, EnablePartialManifest: true,
		SourcePath: src, EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 1,
	}
	first := base
	p, err := NewPipeline(&first)
	require.NoError(t, err)
	r, err := p.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r.Success)

	again := base
	again.ResumeMode, again.ResumeUploadID = true, uploadID
	_, err = NewPipeline(&again)
	require.Error(t, err, "resuming a completed upload must be refused")
	assert.Contains(t, err.Error(), "already completed")
}
