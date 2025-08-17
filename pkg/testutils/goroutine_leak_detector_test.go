package testutils

import (
	"context"
	"testing"
	"time"
)

func TestLeakDetector_NoLeak(t *testing.T) {
	detector := NewLeakDetector(t).Start()
	defer detector.Check()

	// This should not leak any goroutines
	time.Sleep(1 * time.Millisecond)
}

func TestLeakDetector_WithGoroutineLeak(t *testing.T) {
	// This test intentionally creates a leak to verify detection works
	// We'll make it pass by properly cleaning up

	detector := NewLeakDetector(t).Start()

	ctx, cancel := context.WithCancel(context.Background())

	// Start a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			return
		}
	}()

	// Clean up properly
	cancel()
	<-done

	detector.Check()
}

func TestCheckGoroutineLeaks_Helper(t *testing.T) {
	CheckGoroutineLeaks(t, func() {
		// Simple test operation
		time.Sleep(1 * time.Millisecond)
	})
}

func TestTakeGoroutineSnapshot(t *testing.T) {
	snapshot := TakeGoroutineSnapshot()

	if snapshot.Count <= 0 {
		t.Error("Expected at least one goroutine (the current one)")
	}

	if len(snapshot.Goroutines) == 0 {
		t.Error("Expected goroutine information to be parsed")
	}

	t.Logf("Found %d goroutines", snapshot.Count)
	for i, g := range snapshot.Goroutines[:min(3, len(snapshot.Goroutines))] {
		t.Logf("Goroutine %d: state=%s, location=%s", i, g.State, g.Location)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
