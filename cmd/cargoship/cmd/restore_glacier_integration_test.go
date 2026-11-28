//go:build integration

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// glacierProxy sits between a command and the emulator. It forwards every
// request except RestoreObject, which it answers itself (202 Accepted, as S3
// does) and counts. The emulator doesn't model archive retrieval, and what
// matters here is whether the paid request is sent at all (#973).
type glacierProxy struct {
	restores atomic.Int64
	url      string
}

func newGlacierProxy(t *testing.T) *glacierProxy {
	t.Helper()
	target, err := url.Parse(substrateURL)
	require.NoError(t, err)
	rp := httputil.NewSingleHostReverseProxy(target)
	p := &glacierProxy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Has("restore") {
			p.restores.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	p.url = srv.URL
	return p
}

// seedFrozenArchive writes a one-file, one-chunk archive whose chunk object is
// in the GLACIER storage class, and returns the S3 URL of its upload and the
// file's manifest path.
func seedFrozenArchive(t *testing.T) (s3URL, filePath string) {
	t.Helper()
	ctx := context.Background()
	cfg := aws.Config{Region: testRegion, BaseEndpoint: aws.String(substrateURL),
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	bucket := fmt.Sprintf("glacier-gate-%d", time.Now().UnixNano())
	_, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)

	const prefix = "archive"
	const uploadID = "u-frozen"
	chunkKey := "uploads/" + uploadID + "/shard-0/chunk-0.tar.zst"
	body := bytes.Repeat([]byte("cargoship-glacier-gate-"), 100_000) // ~2.3 MB
	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(prefix + "/" + chunkKey),
		Body:         bytes.NewReader(body),
		StorageClass: s3types.StorageClassGlacier,
	})
	require.NoError(t, err)

	filePath = "/src/frozen.txt"
	m := &manifest.Manifest{
		Version: manifest.ManifestVersion, UploadID: uploadID, Bucket: bucket, Prefix: prefix,
		SourcePath: "/src", TotalFiles: 1, TotalChunks: 1,
		Files:  []manifest.FileEntry{{Path: filePath, Size: int64(len(body)), S3Key: chunkKey}},
		Chunks: []manifest.ChunkEntry{{ID: 0, S3Key: chunkKey, FileCount: 1, CompressedSize: int64(len(body))}},
	}
	require.NoError(t, m.UploadToS3(ctx, s3c, true))
	return "s3://" + bucket + "/" + prefix + "/uploads/" + uploadID, filePath
}

// TestRestoreCmd_GlacierGatesRunBeforeRetrieval pins #973. The pre-flight used
// to request every frozen chunk's retrieval while it was still estimating, so
// --dry-run and --max-restore-cost were checked after the money was spent. Now
// the check is read-only and RestoreObject is sent only once both gates pass.
func TestRestoreCmd_GlacierGatesRunBeforeRetrieval(t *testing.T) {
	tests := []struct {
		name         string
		flags        []string
		wantErr      string
		wantRequests int64
	}{
		{name: "dry run requests nothing", flags: []string{"--dry-run"}, wantRequests: 0},
		{name: "over the cost cap requests nothing", flags: []string{"--max-restore-cost", "0.000001"},
			wantErr: "exceeds --max-restore-cost", wantRequests: 0},
		{name: "dry run over the cap requests nothing", flags: []string{"--dry-run", "--max-restore-cost", "0.000001"},
			wantErr: "exceeds --max-restore-cost", wantRequests: 0},
		{name: "a real restore requests the frozen chunk", flags: nil, wantRequests: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3URL, filePath := seedFrozenArchive(t)
			proxy := newGlacierProxy(t)
			t.Setenv("AWS_ENDPOINT_URL", proxy.url)
			t.Setenv("AWS_ACCESS_KEY_ID", "test")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

			cmd := NewRestoreCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(append([]string{s3URL, t.TempDir(), "--region", testRegion, "--file", filePath}, tt.flags...))
			err := cmd.Execute()

			if tt.wantErr != "" {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), tt.wantErr), "error %q should mention %q", err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantRequests, proxy.restores.Load(),
				"RestoreObject requests sent (each one is a billed Glacier retrieval)")
		})
	}
}
