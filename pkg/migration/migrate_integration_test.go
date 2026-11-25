//go:build integration

package migration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	substrate "github.com/scttfrdmn/substrate/emulator"
)

const migrateBucket = "cargoship-migrate-test"

func TestMain(m *testing.M) {
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"
	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))
	if err := substrate.RegisterDefaultPlugins(context.Background(), registry, state, tc, logger, store, nil); err != nil {
		fmt.Fprintf(os.Stderr, "substrate: %v\n", err)
		os.Exit(1)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx, ln) }()
	url := fmt.Sprintf("http://%s", ln.Addr().String())
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if resp, err := http.Get(url + "/health"); err == nil { //nolint:noctx
			_ = resp.Body.Close()
			break
		}
	}
	os.Setenv("AWS_ENDPOINT_URL", url)
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_REGION", "us-east-1")
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", BaseEndpoint: aws.String(url),
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}, func(o *s3.Options) { o.UsePathStyle = true })
	if _, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(migrateBucket)}); err != nil {
		fmt.Fprintf(os.Stderr, "create bucket: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	cancel()
	os.Exit(code)
}

type entry struct {
	hdr  tar.Header
	body string
}

func putTarGz(t *testing.T, client *s3.Client, key string, entries []entry) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := e.hdr
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		require.NoError(t, tw.WriteHeader(&h))
		if e.body != "" {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	_, err := client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(migrateBucket), Key: aws.String(key), Body: bytes.NewReader(buf.Bytes()),
	})
	require.NoError(t, err)
}

// TestMigrate_DeleteOriginalOnlyWhenNothingIsLost pins #911. The extractor
// skipped hard links, and migrate deleted the original whenever the re-upload
// "succeeded": an archive of one file and a hard link to it extracted 1 of 2
// names, re-uploaded, and deleted the only complete copy.
func TestMigrate_DeleteOriginalOnlyWhenNothingIsLost(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	require.NoError(t, err)
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })

	original := entry{tar.Header{Name: "data/original.txt", Typeflag: tar.TypeReg, Mode: 0o644}, "precious bytes"}
	hardlink := entry{tar.Header{Name: "data/second-name.txt", Typeflag: tar.TypeLink, Linkname: "data/original.txt", Mode: 0o644}, ""}
	symlink := entry{tar.Header{Name: "data/alias.txt", Typeflag: tar.TypeSymlink, Linkname: "original.txt", Mode: 0o777}, ""}

	tests := []struct {
		name        string
		entries     []entry
		wantDeleted bool
		wantFiles   int
		wantReason  string
	}{
		{"hard link is migrated, then the original deleted", []entry{original, hardlink}, true, 2, ""},
		{"a symlink the new format can't store keeps the original", []entry{original, symlink}, false, 1, "symlink"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := fmt.Sprintf("legacy/archive-%d-%d.tar.gz", i, time.Now().UnixNano())
			putTarGz(t, client, key, tc.entries)
			dest := fmt.Sprintf("s3://%s/migrated-%d-%d", migrateBucket, i, time.Now().UnixNano())

			res, err := NewMigrator(&MigratorConfig{S3Client: client}).Migrate(ctx, &MigrateRequest{
				SourceArchive: fmt.Sprintf("s3://%s/%s", migrateBucket, key), Destination: dest,
				Region: "us-east-1", TempDir: t.TempDir(), DeleteOriginal: true, ShardCount: 4,
				SkipValidation: true, Quiet: true,
			})
			require.NoError(t, err)
			assert.EqualValues(t, tc.wantFiles, res.FilesUploaded)

			_, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(migrateBucket), Key: aws.String(key)})
			if tc.wantDeleted {
				assert.True(t, res.Success, "errors: %v", res.Errors)
				assert.True(t, res.OriginalArchiveDeleted)
				assert.Error(t, headErr, "the original is deleted once everything is migrated")
				return
			}
			assert.False(t, res.Success, "a refused --delete-original must not report success")
			assert.False(t, res.OriginalArchiveDeleted)
			assert.True(t, strings.Contains(res.OriginalDeleteRefused, tc.wantReason), res.OriginalDeleteRefused)
			assert.NoError(t, headErr, "the original must be kept")
		})
	}
}
