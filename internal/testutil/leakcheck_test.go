package testutil

import (
	"context"
	"testing"
	"time"
)

func TestDefaultLeakCheckOptions(t *testing.T) {
	opts := DefaultLeakCheckOptions()
	if opts.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", opts.Timeout)
	}
	if !opts.RequireCleanup {
		t.Error("RequireCleanup = false, want true")
	}
	if len(opts.IgnorePatterns) == 0 {
		t.Error("IgnorePatterns is empty, want at least one pattern")
	}
}

func TestSkipIfShort(t *testing.T) {
	// Not in short mode here, so SkipIfShort should not skip.
	SkipIfShort(t, "test description")
}

func TestWithLeakCheck(t *testing.T) {
	ran := false
	WithLeakCheck(t, DefaultLeakCheckOptions(), func(t *testing.T) {
		ran = true
	})
	if !ran {
		t.Error("testFunc was not called")
	}
}

func TestWithLeakCheck_ZeroTimeout(t *testing.T) {
	// Zero timeout should use defaults without panicking.
	WithLeakCheck(t, LeakCheckOptions{}, func(t *testing.T) {})
}

func TestRequireNoGoroutineLeak(t *testing.T) {
	// Should not report a leak when no goroutines are spawned.
	RequireNoGoroutineLeak(t)
}

func TestTestWithGoroutine(t *testing.T) {
	started := make(chan struct{})
	TestWithGoroutine(t, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
	})
	select {
	case <-started:
	default:
		t.Error("goroutine did not start")
	}
}
