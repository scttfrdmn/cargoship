//go:build integration

package benchharness

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	substrate "github.com/scttfrdmn/substrate/emulator"

	"github.com/scttfrdmn/cargoship/pkg/corpus"
	"github.com/scttfrdmn/cargoship/pkg/s3count"
)

// TestHarnessRunOnEmulator drives the full plant→upload→restore→cost loop against
// the in-process Substrate S3 emulator, validating that the harness measures real
// request counts, restores byte-identically, and produces a cost breakdown.
func TestHarnessRunOnEmulator(t *testing.T) {
	url, cancel := launchSubstrate(t)
	defer cancel()
	const bucket = "cargoship-bench-test"
	require.NoError(t, createSubstrateBucket(url, bucket))

	counter := s3count.New()
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(url),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	counter.Instrument(&cfg)
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })

	prof, ok := corpus.ProfileByName("mixed")
	require.True(t, ok)

	rr, err := Run(context.Background(), Options{
		Profile: prof, Client: client, Counter: counter,
		Bucket: bucket, Prefix: fmt.Sprintf("bench-%d", time.Now().UnixNano()),
		Region: "us-east-1", SrcDir: t.TempDir(), RestoreDir: t.TempDir(),
	})
	require.NoError(t, err)

	assert.True(t, rr.ByteIdentical, "corpus must restore byte-identically")
	assert.Equal(t, 4, rr.Files, "mixed profile has 4 files")
	assert.Positive(t, rr.StoredBytes)
	// Exact, middleware-counted upload requests: at least one object was PUT.
	up := rr.Upload.OpCounts
	assert.Positive(t, up["PutObject"]+up["CreateMultipartUpload"], "upload should issue PUT-tier requests: %v", up)
	// Restore issued GET-tier requests.
	assert.Positive(t, rr.Restore.OpCounts["GetObject"], "restore should issue GET requests: %v", rr.Restore.OpCounts)
	// Cost is modeled (storage is non-zero for real stored bytes).
	assert.Positive(t, rr.Cost.MonthlyStorageUSD)
	t.Logf("mixed: %d files, stored=%d B, up=%.1f MB/s (%v), restore=%.1f MB/s, storage=$%.6f/mo",
		rr.Files, rr.StoredBytes, rr.Upload.MBPerSec, up, rr.Restore.MBPerSec, rr.Cost.MonthlyStorageUSD)
}

// TestHarnessModesOnEmulator confirms the forced upload modes engage the path
// they name: packed produces chunks, direct produces one object per file (no
// chunks) — the head-to-head that answers #466. Both round-trip byte-identical.
func TestHarnessModesOnEmulator(t *testing.T) {
	url, cancel := launchSubstrate(t)
	defer cancel()
	const bucket = "cargoship-bench-modes"
	require.NoError(t, createSubstrateBucket(url, bucket))

	prof, ok := corpus.ProfileByName("many-tiny")
	require.True(t, ok)

	for _, tc := range []struct {
		mode        Mode
		wantChunked bool
	}{
		{ModePacked, true},
		{ModeDirect, false},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			counter := s3count.New()
			cfg := aws.Config{
				Region:       "us-east-1",
				BaseEndpoint: aws.String(url),
				Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
			}
			counter.Instrument(&cfg)
			client := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })

			rr, err := Run(context.Background(), Options{
				Profile: prof, Client: client, Counter: counter, Mode: tc.mode,
				Bucket: bucket, Prefix: fmt.Sprintf("modes-%s-%d", tc.mode, time.Now().UnixNano()),
				Region: "us-east-1", SrcDir: t.TempDir(), RestoreDir: t.TempDir(),
			})
			require.NoError(t, err)
			assert.True(t, rr.ByteIdentical)
			assert.Equal(t, tc.mode, rr.Mode)
			if tc.wantChunked {
				assert.Positive(t, rr.Chunks, "packed mode must produce chunks")
				assert.Positive(t, rr.Upload.OpCounts["PutObject"]+rr.Upload.OpCounts["CreateMultipartUpload"])
			} else {
				assert.Zero(t, rr.Chunks, "direct mode must produce no chunks")
				// One PutObject per file (5000) + the manifest.
				assert.Greater(t, rr.Upload.OpCounts["PutObject"], 5000, "direct mode PUTs each file")
			}
			t.Logf("many-tiny [%s]: chunks=%d up=%v", tc.mode, rr.Chunks, rr.Upload.OpCounts)
		})
	}
}

// launchSubstrate starts an in-process Substrate S3 emulator (mirrors the
// pipeline integration tests' bootstrap).
func launchSubstrate(t *testing.T) (string, context.CancelFunc) {
	t.Helper()
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))
	require.NoError(t, substrate.RegisterDefaultPlugins(context.Background(), registry, state, tc, logger, store, nil))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, pingErr := http.Get(baseURL + "/health"); pingErr == nil { //nolint:noctx
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return baseURL, cancel
}

func createSubstrateBucket(baseURL, bucket string) error {
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(baseURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })
	_, err := client.CreateBucket(context.Background(), &awss3.CreateBucketInput{Bucket: aws.String(bucket)})
	return err
}
