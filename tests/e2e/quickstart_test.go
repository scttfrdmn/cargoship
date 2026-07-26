//go:build e2e

// Package e2e drives the real `cargoship` binary through the documented Quick
// Start (upload → info → verify → restore) against an in-process Substrate S3
// emulator. It is the executable form of docs/start/quickstart.md — if the
// documented happy path breaks (as it did for direct-upload mode in #228), this
// test fails.
//
// Run with:  go test -tags e2e ./tests/e2e/ -timeout 5m
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	substrate "github.com/scttfrdmn/substrate/emulator"
)

var (
	substrateURL string
	cargoshipBin string
)

func TestMain(m *testing.M) {
	url, cancel, err := launchSubstrate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: start emulator: %v\n", err)
		os.Exit(1)
	}
	substrateURL = url
	// Point the CLI (and its SDK) at the emulator. The endpoint contains
	// 127.0.0.1:, which the CLI auto-detects to enable path-style addressing.
	os.Setenv("AWS_ENDPOINT_URL", url)
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_REGION", "us-east-1")

	bin, buildErr := buildCargoship()
	if buildErr != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "e2e: build cargoship: %v\n", buildErr)
		os.Exit(1)
	}
	cargoshipBin = bin

	code := m.Run()
	cancel()
	os.Exit(code)
}

// TestQuickStart_RoundTrip mirrors docs/start/quickstart.md end to end: upload a
// directory, inspect it, verify integrity, and restore a single file — asserting
// the restored content matches the original.
func TestQuickStart_RoundTrip(t *testing.T) {
	bucket := "quickstart-e2e"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	// Source data.
	src := t.TempDir()
	want := "hello cargoship — quickstart e2e\n"
	writeFile(t, filepath.Join(src, "greeting.txt"), want)
	writeFile(t, filepath.Join(src, "notes.txt"), "second file\n")

	dest := "s3://" + bucket + "/archives"

	// 1. Upload.
	out := runCargoship(t, "upload", src, dest, "--region", "us-east-1")
	uploadID := extractUploadID(t, out)
	uploadURL := dest + "/uploads/" + uploadID

	// 2. Inspect (must not error and must report the files).
	runCargoship(t, "info", uploadURL, "--region", "us-east-1")

	// 3. Verify integrity (exits non-zero on failure).
	runCargoship(t, "verify", uploadURL, "--region", "us-east-1")

	// 4. Restore a single file and confirm the round trip.
	restoreDir := t.TempDir()
	runCargoship(t, "restore", uploadURL, restoreDir, "--file", "greeting.txt", "--region", "us-east-1")

	// Restore preserves the source directory structure under restoreDir
	// (escape-safe; source path minus leading slash — see #282), so locate the
	// restored file by basename wherever it landed rather than assuming a flat
	// layout.
	restored := findFileByBase(t, restoreDir, "greeting.txt")
	got, err := os.ReadFile(restored)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("restored content mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

// --- helpers ---------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// findFileByBase walks dir and returns the full path of the first regular file
// with the given basename, failing if none is found.
func findFileByBase(t *testing.T, dir, base string) string {
	t.Helper()
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == base {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if found == "" {
		t.Fatalf("restored file %q not found under %s", base, dir)
	}
	return found
}

// runCargoship runs the built binary with the given args, failing the test on a
// non-zero exit. Returns combined stdout+stderr.
func runCargoship(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cargoshipBin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cargoship %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// extractUploadID pulls the timestamped upload ID (e.g. 20260722-abcd1234) from
// upload output.
func extractUploadID(t *testing.T, out string) string {
	t.Helper()
	// Look for "uploads/<id>/" in the output.
	const marker = "uploads/"
	idx := -1
	if i := indexOf(out, marker); i >= 0 {
		idx = i + len(marker)
	}
	if idx < 0 {
		t.Fatalf("no upload ID in output:\n%s", out)
	}
	end := idx
	for end < len(out) && out[end] != '/' && out[end] != '\n' && out[end] != ' ' {
		end++
	}
	id := out[idx:end]
	if id == "" {
		t.Fatalf("empty upload ID parsed from output:\n%s", out)
	}
	return id
}

func indexOf(s, sub string) int { return bytes.Index([]byte(s), []byte(sub)) }

func buildCargoship() (string, error) {
	dir, err := os.MkdirTemp("", "cargoship-e2e-bin")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "cargoship")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "../../cmd/cargoship")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v\n%s", err, out)
	}
	return bin, nil
}

func createBucket(baseURL, bucket string) error {
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(baseURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	_, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// launchSubstrate starts an in-process Substrate S3 emulator on a random local
// port and returns its base URL plus a cancel func. Mirrors the helper used by
// the integration suites.
func launchSubstrate() (string, context.CancelFunc, error) {
	cfg := substrate.DefaultConfig()
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))

	ctx := context.Background()
	if err := substrate.RegisterDefaultPlugins(ctx, registry, state, tc, logger, store, nil); err != nil {
		return "", nil, fmt.Errorf("register plugins: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, pingErr := http.Get(baseURL + "/health"); pingErr == nil { //nolint:noctx
			_ = resp.Body.Close()
			return baseURL, cancel, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	return "", nil, fmt.Errorf("emulator did not become healthy")
}
