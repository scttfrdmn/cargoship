package profiling

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// Timer tracks execution time for operations
type Timer struct {
	name      string
	start     time.Time
	labels    map[string]interface{}
	memBefore runtime.MemStats
}

// NewTimer creates a new timer for an operation
func NewTimer(name string) *Timer {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &Timer{
		name:      name,
		start:     time.Now(),
		labels:    make(map[string]interface{}),
		memBefore: memStats,
	}
}

// WithLabel adds a label to the timer
func (t *Timer) WithLabel(key string, value interface{}) *Timer {
	t.labels[key] = value
	return t
}

// Stop stops the timer and logs the duration
func (t *Timer) Stop() time.Duration {
	duration := time.Since(t.start)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory delta
	allocDelta := memAfter.Alloc - t.memBefore.Alloc
	gcDelta := memAfter.NumGC - t.memBefore.NumGC

	// Build log attributes
	attrs := []slog.Attr{
		slog.String("operation", t.name),
		slog.Duration("duration", duration),
		slog.Uint64("alloc_bytes", allocDelta),
		slog.Uint64("num_gc", uint64(gcDelta)),
	}

	// Add custom labels
	for k, v := range t.labels {
		attrs = append(attrs, slog.Any(k, v))
	}

	slog.LogAttrs(context.Background(), slog.LevelDebug, "operation completed", attrs...)

	return duration
}

// StopWithError stops the timer and logs with error
func (t *Timer) StopWithError(err error) time.Duration {
	duration := time.Since(t.start)

	attrs := []slog.Attr{
		slog.String("operation", t.name),
		slog.Duration("duration", duration),
		slog.String("error", err.Error()),
	}

	for k, v := range t.labels {
		attrs = append(attrs, slog.Any(k, v))
	}

	slog.LogAttrs(context.Background(), slog.LevelError, "operation failed", attrs...)

	return duration
}

// Track wraps a function with timing and logging
func Track(name string, fn func() error) error {
	timer := NewTimer(name)
	defer func() {
		timer.Stop()
	}()

	return fn()
}

// TrackWithContext wraps a function with timing, logging, and context
func TrackWithContext(ctx context.Context, name string, fn func(context.Context) error) error {
	timer := NewTimer(name)
	defer func() {
		timer.Stop()
	}()

	return fn(ctx)
}

// ResourceSnapshot captures current resource usage
type ResourceSnapshot struct {
	Timestamp    time.Time
	Goroutines   int
	Alloc        uint64
	TotalAlloc   uint64
	Sys          uint64
	NumGC        uint32
	PauseTotalNs uint64
}

// CaptureResourceSnapshot takes a snapshot of current resource usage
func CaptureResourceSnapshot() ResourceSnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return ResourceSnapshot{
		Timestamp:    time.Now(),
		Goroutines:   runtime.NumGoroutine(),
		Alloc:        m.Alloc,
		TotalAlloc:   m.TotalAlloc,
		Sys:          m.Sys,
		NumGC:        m.NumGC,
		PauseTotalNs: m.PauseTotalNs,
	}
}

// ResourceDelta calculates the difference between two snapshots
type ResourceDelta struct {
	Duration     time.Duration
	GoroutineDiff int
	AllocDiff    int64
	TotalAllocDiff uint64
	SysDiff      int64
	NumGCDiff    uint32
	PauseDiff    uint64
}

// Delta computes the delta between two resource snapshots
func (before ResourceSnapshot) Delta(after ResourceSnapshot) ResourceDelta {
	return ResourceDelta{
		Duration:       after.Timestamp.Sub(before.Timestamp),
		GoroutineDiff:  after.Goroutines - before.Goroutines,
		AllocDiff:      int64(after.Alloc) - int64(before.Alloc),
		TotalAllocDiff: after.TotalAlloc - before.TotalAlloc,
		SysDiff:        int64(after.Sys) - int64(before.Sys),
		NumGCDiff:      after.NumGC - before.NumGC,
		PauseDiff:      after.PauseTotalNs - before.PauseTotalNs,
	}
}

// LogDelta logs the resource delta
func (d ResourceDelta) LogDelta(operation string) {
	slog.Info("resource usage",
		"operation", operation,
		"duration", d.Duration,
		"goroutine_diff", d.GoroutineDiff,
		"alloc_diff_bytes", d.AllocDiff,
		"total_alloc_bytes", d.TotalAllocDiff,
		"sys_diff_bytes", d.SysDiff,
		"gc_cycles", d.NumGCDiff,
		"gc_pause_ns", d.PauseDiff,
	)
}

// TrackResources wraps a function with resource usage tracking
func TrackResources(name string, fn func() error) error {
	before := CaptureResourceSnapshot()

	err := fn()

	after := CaptureResourceSnapshot()
	delta := before.Delta(after)
	delta.LogDelta(name)

	return err
}
