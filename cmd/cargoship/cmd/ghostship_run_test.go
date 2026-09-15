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
