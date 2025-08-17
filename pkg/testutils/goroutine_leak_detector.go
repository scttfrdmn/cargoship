/*
Package testutils provides utilities for testing, including goroutine leak detection
*/
package testutils

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// GoroutineSnapshot represents the state of goroutines at a point in time
type GoroutineSnapshot struct {
	Count      int
	Goroutines []GoroutineInfo
}

// GoroutineInfo contains information about a single goroutine
type GoroutineInfo struct {
	ID       int
	State    string
	Location string
	Stack    string
}

// TakeGoroutineSnapshot captures the current state of all goroutines
func TakeGoroutineSnapshot() *GoroutineSnapshot {
	// Get the number of goroutines
	count := runtime.NumGoroutine()

	// Get stack traces for all goroutines
	buf := make([]byte, 1<<20) // 1MB buffer
	stackSize := runtime.Stack(buf, true)
	stackTrace := string(buf[:stackSize])

	// Parse goroutine information from stack trace
	goroutines := parseGoroutines(stackTrace)

	return &GoroutineSnapshot{
		Count:      count,
		Goroutines: goroutines,
	}
}

// parseGoroutines parses goroutine information from a stack trace string
func parseGoroutines(stackTrace string) []GoroutineInfo {
	lines := strings.Split(stackTrace, "\n")
	var goroutines []GoroutineInfo

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "goroutine ") {
			// Parse goroutine header: "goroutine 123 [running]:"
			goroutine := GoroutineInfo{}

			// Extract goroutine information
			if strings.Contains(line, "[") && strings.Contains(line, "]") {
				start := strings.Index(line, "[")
				end := strings.Index(line, "]")
				if start < end {
					goroutine.State = line[start+1 : end]
				}
			}

			// Get the stack trace for this goroutine
			stackLines := []string{line}
			for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "goroutine "); j++ {
				stackLines = append(stackLines, lines[j])
				if strings.Contains(lines[j], ".go:") {
					goroutine.Location = strings.TrimSpace(lines[j])
				}
			}

			goroutine.Stack = strings.Join(stackLines, "\n")
			goroutines = append(goroutines, goroutine)
		}
	}

	return goroutines
}

// LeakDetector helps detect goroutine leaks in tests
type LeakDetector struct {
	t            *testing.T
	initialCount int
	initialState *GoroutineSnapshot
	ignored      []string
	timeout      time.Duration
	maxRetries   int
}

// NewLeakDetector creates a new goroutine leak detector
func NewLeakDetector(t *testing.T) *LeakDetector {
	return &LeakDetector{
		t:          t,
		ignored:    getDefaultIgnoredGoroutines(),
		timeout:    time.Second * 5,
		maxRetries: 10,
	}
}

// WithIgnored adds goroutines to ignore (by function name or state)
func (ld *LeakDetector) WithIgnored(patterns ...string) *LeakDetector {
	ld.ignored = append(ld.ignored, patterns...)
	return ld
}

// WithTimeout sets the timeout for leak detection
func (ld *LeakDetector) WithTimeout(timeout time.Duration) *LeakDetector {
	ld.timeout = timeout
	return ld
}

// Start captures the initial goroutine state
func (ld *LeakDetector) Start() *LeakDetector {
	// Allow some time for transient goroutines to settle
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	ld.initialState = TakeGoroutineSnapshot()
	ld.initialCount = ld.countRelevantGoroutines(ld.initialState)

	ld.t.Logf("LeakDetector started with %d relevant goroutines", ld.initialCount)
	return ld
}

// Check verifies no goroutines have leaked
func (ld *LeakDetector) Check() {
	if ld.initialState == nil {
		ld.t.Fatal("LeakDetector.Start() must be called before Check()")
	}

	// Retry logic to handle transient goroutines
	for attempt := 0; attempt < ld.maxRetries; attempt++ {
		// Force garbage collection to clean up any finalizers
		runtime.GC()
		time.Sleep(time.Millisecond * 50)

		currentState := TakeGoroutineSnapshot()
		currentCount := ld.countRelevantGoroutines(currentState)

		if currentCount <= ld.initialCount {
			ld.t.Logf("LeakDetector passed: %d goroutines (initial: %d)", currentCount, ld.initialCount)
			return
		}

		if attempt == ld.maxRetries-1 {
			// Final attempt - report the leak
			leaked := ld.findLeakedGoroutines(currentState)
			ld.t.Errorf("Goroutine leak detected! Initial: %d, Current: %d, Leaked: %d",
				ld.initialCount, currentCount, len(leaked))

			for i, leak := range leaked {
				ld.t.Errorf("Leaked goroutine %d:\n%s", i+1, leak.Stack)
			}
		}

		// Wait before retry
		time.Sleep(time.Millisecond * time.Duration(100*(attempt+1)))
	}
}

// countRelevantGoroutines counts goroutines that are not in the ignore list
func (ld *LeakDetector) countRelevantGoroutines(snapshot *GoroutineSnapshot) int {
	count := 0
	for _, g := range snapshot.Goroutines {
		if !ld.shouldIgnore(g) {
			count++
		}
	}
	return count
}

// findLeakedGoroutines identifies goroutines that weren't present initially
func (ld *LeakDetector) findLeakedGoroutines(currentState *GoroutineSnapshot) []GoroutineInfo {
	var leaked []GoroutineInfo

	// Simple approach: any relevant goroutine beyond the initial count is considered leaked
	relevantCount := 0
	for _, g := range currentState.Goroutines {
		if !ld.shouldIgnore(g) {
			relevantCount++
			if relevantCount > ld.initialCount {
				leaked = append(leaked, g)
			}
		}
	}

	return leaked
}

// shouldIgnore checks if a goroutine should be ignored based on its stack trace
func (ld *LeakDetector) shouldIgnore(g GoroutineInfo) bool {
	for _, pattern := range ld.ignored {
		if strings.Contains(g.Stack, pattern) || strings.Contains(g.State, pattern) {
			return true
		}
	}
	return false
}

// getDefaultIgnoredGoroutines returns patterns for goroutines that should typically be ignored
func getDefaultIgnoredGoroutines() []string {
	return []string{
		// Testing framework goroutines
		"testing.(*T).Run",
		"testing.tRunner",
		"testing.Main",

		// Runtime goroutines
		"runtime.goexit",
		"runtime.main",
		"runtime/pprof",
		"runtime.gcBgMarkWorker",
		"runtime.bgsweep",
		"runtime.bgscavenge",
		"runtime.forcegchelper",

		// HTTP transport goroutines (common in cloud SDKs)
		"net/http.(*Transport).dialConn",
		"net/http.(*persistConn).readLoop",
		"net/http.(*persistConn).writeLoop",

		// Common third-party library goroutines
		"github.com/aws/aws-sdk-go",

		// Finalizer goroutines
		"runfinq",

		// Deadlock detector or similar tools
		"github.com/sasha-s/go-deadlock",
	}
}

// CheckGoroutineLeaks is a helper function for simple leak detection in tests
func CheckGoroutineLeaks(t *testing.T, testFunc func()) {
	detector := NewLeakDetector(t).Start()
	defer detector.Check()

	testFunc()
}
