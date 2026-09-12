package main

import (
	"fmt"
	"os/exec"
	"testing"
)

// TestRunInstrumentedSamplesChild proves the fix for the wrong-PID sampler: a
// CPU-burning CHILD process must show non-zero CPU, which only happens if we
// sample cmd.Process.Pid rather than os.Getpid() (the near-idle harness).
func TestRunInstrumentedSamplesChild(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	// Busy-loop for ~1s so the 100ms sampler captures several live samples.
	burn := `end=$(($(date +%s)+1)); while [ $(date +%s) -lt $end ]; do :; done`
	cmd := exec.Command("sh", "-c", burn)

	var result BenchmarkResult
	if _, dur, err := runInstrumented(cmd, &result); err != nil {
		t.Fatalf("runInstrumented: %v (dur=%s)", err, dur)
	}
	if result.AvgCPUPercent <= 0 {
		t.Fatalf("expected non-zero CPU for the busy child, got %.2f%% — sampler is watching the wrong PID", result.AvgCPUPercent)
	}
	t.Logf("child sampled: avg CPU %.1f%%, peak mem %.1f MB", result.AvgCPUPercent, result.PeakMemoryMB)
}

func TestRunBest_HonorsIterations(t *testing.T) {
	calls := 0
	best, err := runBest(3, func() (BenchmarkResult, error) {
		calls++
		return BenchmarkResult{UploadThroughputMBps: float64(calls)}, nil // 1, 2, 3
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 iterations, ran %d", calls)
	}
	if best.UploadThroughputMBps != 3 {
		t.Fatalf("expected best-of-N to keep the fastest (3), got %.0f", best.UploadThroughputMBps)
	}
}

func TestRunBest_DefaultsToOneAndPropagatesFailure(t *testing.T) {
	calls := 0
	if _, err := runBest(0, func() (BenchmarkResult, error) {
		calls++
		return BenchmarkResult{}, fmt.Errorf("boom")
	}); err == nil {
		t.Fatal("expected an error when every iteration fails")
	}
	if calls != 1 {
		t.Fatalf("iterations<1 must run exactly once, ran %d", calls)
	}
}
