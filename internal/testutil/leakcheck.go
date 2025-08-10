// Package testutil provides testing utilities for CargoShip
package testutil

import (
	"context"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// LeakCheckOptions configures goroutine leak detection
type LeakCheckOptions struct {
	// Timeout for waiting for goroutines to cleanup (default: 5s)
	Timeout time.Duration
	// IgnorePatterns are function patterns to ignore in leak detection
	IgnorePatterns []string
	// RequireCleanup fails test if goroutines don't cleanup within timeout
	RequireCleanup bool
}

// DefaultLeakCheckOptions returns sensible defaults for leak checking
func DefaultLeakCheckOptions() LeakCheckOptions {
	return LeakCheckOptions{
		Timeout:        5 * time.Second,
		RequireCleanup: true,
		IgnorePatterns: []string{
			// Common goroutines that are expected to leak in tests
			"go.opencensus.io/stats/view.(*worker).start",
			"github.com/aws/aws-sdk-go-v2/internal/shareddefaults.init",
		},
	}
}

// WithLeakCheck wraps a test function with goroutine leak detection
func WithLeakCheck(t *testing.T, opts LeakCheckOptions, testFunc func(t *testing.T)) {
	t.Helper()

	if opts.Timeout == 0 {
		opts = DefaultLeakCheckOptions()
	}

	// Build goleak options
	var goleakOpts []goleak.Option
	for _, pattern := range opts.IgnorePatterns {
		goleakOpts = append(goleakOpts, goleak.IgnoreTopFunction(pattern))
	}

	defer goleak.VerifyNone(t, goleakOpts...)

	// Record initial goroutine count for debugging
	beforeCount := runtime.NumGoroutine()
	t.Logf("Goroutines before test: %d", beforeCount)

	// Run the actual test
	testFunc(t)

	// Give some time for cleanup if required
	if opts.RequireCleanup {
		// Wait a bit for cleanup
		time.Sleep(50 * time.Millisecond)

		afterCount := runtime.NumGoroutine()
		t.Logf("Goroutines after test: %d", afterCount)

		if afterCount > beforeCount {
			t.Logf("Warning: %d more goroutines after test (%d vs %d)", 
				afterCount-beforeCount, afterCount, beforeCount)
		}
	}
}

// TestWithGoroutine helps test functions that spawn goroutines with proper cleanup validation
func TestWithGoroutine(t *testing.T, goroutineFunc func(ctx context.Context)) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		goroutineFunc(ctx)
	}()

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for goroutine to finish or timeout
	select {
	case <-done:
		// Good - goroutine stopped
		t.Logf("Goroutine completed successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Goroutine did not stop within timeout after context cancellation")
	}
}

// SkipIfShort skips a test in short mode with a descriptive message
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping %s in short mode: %s", t.Name(), reason)
	}
}

// RequireNoGoroutineLeak is a simpler version that just checks for leaks without configuration
func RequireNoGoroutineLeak(t *testing.T) {
	t.Helper()
	defer goleak.VerifyNone(t,
		// Ignore common AWS SDK goroutines that may not cleanup immediately
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("github.com/aws/aws-sdk-go-v2/internal/shareddefaults.init"),
	)
}

// BenchmarkNoGoroutineLeak checks for goroutine leaks in benchmarks
func BenchmarkNoGoroutineLeak(b *testing.B, benchFunc func(b *testing.B)) {
	b.Helper()
	
	defer goleak.VerifyNone(&testingTB{b},
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("github.com/aws/aws-sdk-go-v2/internal/shareddefaults.init"),
	)
	
	benchFunc(b)
}

// testingTB adapts testing.B to testing.TB interface for goleak
type testingTB struct {
	*testing.B
}

func (tb *testingTB) Fatalf(format string, args ...interface{}) {
	tb.B.Fatalf(format, args...)
}