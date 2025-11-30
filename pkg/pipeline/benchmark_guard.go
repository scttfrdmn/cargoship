//go:build benchmark

package pipeline

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// checkForConcurrentBenchmarks checks if any other benchmark tests are currently running.
// This is critical for accurate benchmarking - concurrent benchmarks will invalidate results.
//
// Returns true if no other benchmarks are running, false otherwise.
func checkForConcurrentBenchmarks(b *testing.B) bool {
	// Use ps to check for other go test processes running benchmarks
	cmd := exec.Command("ps", "aux")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		b.Logf("WARNING: Failed to check for concurrent benchmarks: %v", err)
		return true // Assume OK if we can't check
	}

	lines := strings.Split(out.String(), "\n")
	benchmarkCount := 0

	for _, line := range lines {
		// Look for "go test" processes with "-bench" flag
		// Exclude shell wrappers (/bin/zsh, /bin/bash, etc.) that contain the command as a string
		if strings.Contains(line, "go test") && strings.Contains(line, "-bench") {
			// Skip shell wrapper processes (they contain the full command in quotes)
			if strings.Contains(line, "/bin/zsh") || strings.Contains(line, "/bin/bash") || strings.Contains(line, "/bin/sh") {
				continue
			}
			benchmarkCount++
			b.Logf("Found benchmark process: %s", strings.TrimSpace(line))
		}
	}

	// We expect to find exactly 1 benchmark process (the current one)
	if benchmarkCount > 1 {
		b.Logf("\n========================================")
		b.Logf("❌ CONCURRENT BENCHMARK DETECTED")
		b.Logf("========================================")
		b.Logf("Found %d benchmark processes running concurrently.", benchmarkCount)
		b.Logf("This WILL INVALIDATE benchmark results!")
		b.Logf("")
		b.Logf("Please wait for other benchmarks to complete before running.")
		b.Logf("To check running benchmarks: ps aux | grep 'go test.*bench'")
		b.Logf("To kill running benchmarks: pkill -9 -f 'go test.*bench'")
		b.Logf("========================================")
		return false
	}

	return true
}

// ensureNoConcurrentBenchmarks checks for concurrent benchmarks and skips the test if found.
// This should be called at the start of every benchmark to ensure accurate results.
func ensureNoConcurrentBenchmarks(b *testing.B) {
	if !checkForConcurrentBenchmarks(b) {
		b.Skip("Skipping benchmark: concurrent benchmark detected (would invalidate results)")
	}
}

// formatBenchmarkHeader prints a clear header for benchmark execution
func formatBenchmarkHeader(b *testing.B, benchmarkName string, description string) {
	b.Logf("\n========================================")
	b.Logf("=== %s ===", benchmarkName)
	b.Logf("========================================")
	if description != "" {
		b.Logf("%s", description)
		b.Logf("========================================")
	}
	b.Logf("")
}
