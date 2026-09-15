package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	cargoshipconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/launch"
)

func TestRunLoop_OnceRunsFnOnceAndReturnsError(t *testing.T) {
	calls := 0
	err := runLoop(context.Background(), time.Hour, true, func(context.Context) error {
		calls++
		return errors.New("boom")
	})
	if calls != 1 {
		t.Fatalf("once should call fn exactly once, got %d", calls)
	}
	if err == nil {
		t.Error("once should return fn's error")
	}
}

func TestRunLoop_LoopsUntilContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	fn := func(context.Context) error {
		if atomic.AddInt32(&calls, 1) >= 3 {
			cancel() // stop after the 3rd cycle
		}
		return errors.New("transient") // per-cycle errors must NOT stop the loop
	}
	if err := runLoop(ctx, time.Millisecond, false, fn); err != nil {
		t.Errorf("loop should return nil when the context is canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Errorf("expected at least 3 cycles before cancel, got %d", got)
	}
}

func TestGhostshipRunCmd_Validation(t *testing.T) {
	run := func(args ...string) error {
		cmd := NewGhostshipCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	dir := t.TempDir()

	// These all fail before any AWS call.
	if err := run("run", filepath.Join(dir, "nope"), "s3://b/p", "--once"); err == nil {
		t.Error("nonexistent source should error")
	}
	if err := run("run", dir, "nots3://x", "--once"); err == nil {
		t.Error("bad S3 URL should error")
	}
	if err := run("run", dir, "s3://b/p", "--writer-id", "bad/id", "--once"); err == nil {
		t.Error("invalid --writer-id should error")
	}
}

func TestRunOneSync_NoChangesOnEmptyDir(t *testing.T) {
	// force skips the S3 previous-manifest fetch; an empty dir yields no changes, so
	// the pipeline never runs and no S3 client is needed.
	res, err := runOneSync(context.Background(), syncRunParams{
		bucket: "b", prefix: "p", sourcePath: t.TempDir(), force: true,
	})
	if err != nil {
		t.Fatalf("runOneSync: %v", err)
	}
	if !res.NoChanges {
		t.Errorf("empty dir with --force should report NoChanges, got %+v", res)
	}
}

func TestRunOneSync_DryRunDoesNotUpload(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := runOneSync(context.Background(), syncRunParams{
		bucket: "b", prefix: "p", sourcePath: dir, force: true, dryRun: true,
	})
	if err != nil {
		t.Fatalf("runOneSync: %v", err)
	}
	if res.NoChanges {
		t.Error("a file present should be detected as a change")
	}
	if res.Result != nil {
		t.Error("dry run must not run the pipeline")
	}
}

func TestBuildRunPlan(t *testing.T) {
	cfg := &launch.GhostShipConfig{
		ID:       "box-id",
		WriterID: "lab-nas-1",
		S3Config: cargoshipconfig.S3Config{Bucket: "b"},
		WatchPaths: []launch.WatchPath{
			{Path: "/data/a"},
			{Path: "/data/b", StorageClass: "GLACIER_IR"},
		},
		ArchivalRules: []launch.ArchivalRule{{Name: "legacy"}},
	}
	d := runDefaults{region: "us-west-2", storageClass: "STANDARD", shardCount: 10, shardStrategy: "round-robin"}

	wid, sources, warns, err := buildRunPlan(cfg, "", d)
	if err != nil {
		t.Fatalf("buildRunPlan: %v", err)
	}
	if wid != "lab-nas-1" {
		t.Errorf("writer-id = %q, want config writer_id lab-nas-1", wid)
	}
	if len(sources) != 2 {
		t.Fatalf("want 2 sources, got %d", len(sources))
	}
	if sources[0].prefix != "writers/lab-nas-1" {
		t.Errorf("prefix = %q, want writers/lab-nas-1", sources[0].prefix)
	}
	if sources[0].sourcePath != "/data/a" || sources[1].sourcePath != "/data/b" {
		t.Errorf("source paths not mapped: %q, %q", sources[0].sourcePath, sources[1].sourcePath)
	}
	if sources[0].storageClass != "STANDARD" {
		t.Errorf("source0 storage class = %q, want run default STANDARD", sources[0].storageClass)
	}
	if sources[1].storageClass != "GLACIER_IR" {
		t.Errorf("source1 storage class = %q, want per-source GLACIER_IR", sources[1].storageClass)
	}
	if len(warns) == 0 {
		t.Error("archival_rules present should produce an ignore warning")
	}

	// --writer-id flag overrides config writer_id.
	if got, _, _, _ := buildRunPlan(cfg, "override-id", d); got != "override-id" {
		t.Errorf("flag should override config writer_id, got %q", got)
	}
	// Falls back to id when no writer_id.
	cfg.WriterID = ""
	if got, _, _, _ := buildRunPlan(cfg, "", d); got != "box-id" {
		t.Errorf("should fall back to id, got %q", got)
	}
	// A bad config writer_id is an error.
	cfg.WriterID = "bad/id"
	if _, _, _, err := buildRunPlan(cfg, "", d); err == nil {
		t.Error("bad config writer_id should error")
	}
}

func TestGhostshipRunConfigMode(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	run := func(args ...string) error {
		cmd := NewGhostshipCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	// --config with positional args is a usage error.
	if err := run("run", "--config", "x.yaml", "src", "s3://b/p"); err == nil {
		t.Error("--config plus positional args should error")
	}
	// A config missing the bucket fails validation before any AWS call.
	bad := write("bad.yaml", "id: box\nwatch_paths:\n  - path: /data\n")
	if err := run("run", "--config", bad); err == nil {
		t.Error("config missing s3_config.bucket should error")
	}
}
