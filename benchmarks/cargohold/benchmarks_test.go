package main

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/corpus"
)

func TestMirrorPrefix(t *testing.T) {
	cfg := &BenchmarkConfig{Prefix: "bench", Scenario: "small"}
	for _, tool := range []string{"s5cmd", "rclone", "mc"} {
		got, ok := mirrorPrefix(tool, cfg)
		if !ok {
			t.Fatalf("%s should be a mirror tool", tool)
		}
		if want := "bench/" + tool + "-small"; got != want {
			t.Fatalf("%s prefix = %q, want %q", tool, got, want)
		}
	}
	for _, tool := range []string{"cargohold", "tar"} {
		if _, ok := mirrorPrefix(tool, cfg); ok {
			t.Fatalf("%s must NOT be treated as a mirror tool (chunked/archive)", tool)
		}
	}
}

func TestCompareDownloaded(t *testing.T) {
	dir := t.TempDir()
	files, err := corpus.UniformProfile(20, 512).Plant(dir)
	if err != nil {
		t.Fatalf("plant: %v", err)
	}

	t.Run("all present and identical", func(t *testing.T) {
		n, err := compareDownloaded(dir, files)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if n != len(files) {
			t.Fatalf("verified %d, want %d", n, len(files))
		}
	})

	t.Run("content mismatch is caught", func(t *testing.T) {
		bad := append([]corpus.File(nil), files...)
		bad[0].Sum = "deadbeef"
		if _, err := compareDownloaded(dir, bad); err == nil {
			t.Fatal("expected a mismatch error")
		}
	})

	t.Run("missing file is caught", func(t *testing.T) {
		bad := append([]corpus.File(nil), files...)
		bad = append(bad, corpus.File{Base: "not-uploaded.dat", Sum: "x"})
		if _, err := compareDownloaded(dir, bad); err == nil {
			t.Fatal("expected a missing-file error")
		}
	})
}

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
