package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"
)

// TestPacedReader_EnforcesRate proves the token bucket actually limits
// throughput: 512 KiB at 1 MiB/s must take on the order of half a second, versus
// microseconds unthrottled.
func TestPacedReader_EnforcesRate(t *testing.T) {
	const size = 512 * 1024
	const rate = 1024 * 1024 // bytes/sec

	src := bytes.NewReader(bytes.Repeat([]byte("x"), size))
	pr := newPacedReader(src, func() float64 { return rate }, nil, 0)

	start := time.Now()
	n, err := io.Copy(io.Discard, pr)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != size {
		t.Fatalf("copied %d bytes, want %d", n, size)
	}
	// Expected ~0.5s. Assert clearly-throttled without being flaky about the
	// exact figure.
	if elapsed < 250*time.Millisecond {
		t.Errorf("expected pacing to slow the copy to ~0.5s, took only %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("pacing overshot badly: %v", elapsed)
	}
}

// TestPacedReader_UnlimitedIsTransparent confirms a non-positive rate does not
// throttle and preserves the bytes exactly.
func TestPacedReader_UnlimitedIsTransparent(t *testing.T) {
	want := make([]byte, 256*1024)
	if _, err := rand.Read(want); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pr := newPacedReader(bytes.NewReader(want), func() float64 { return 0 }, nil, 0)

	start := time.Now()
	got, err := io.ReadAll(pr)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("data corrupted through paced reader (got %d bytes, want %d)", len(got), len(want))
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("unlimited rate should not throttle, took %v", elapsed)
	}
}

// TestPacedReader_SampleReceivesEveryByte checks the sample callback observes the
// full byte count — the delivery-rate signal fed to BBR must not miss data.
func TestPacedReader_SampleReceivesEveryByte(t *testing.T) {
	const size = 200 * 1024
	var sampled int64
	pr := newPacedReader(
		bytes.NewReader(bytes.Repeat([]byte("y"), size)),
		func() float64 { return 0 },
		func(n int, _, _ time.Time) { sampled += int64(n) },
		0,
	)
	n, err := io.Copy(io.Discard, pr)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != size || sampled != size {
		t.Fatalf("copied=%d sampled=%d, want both %d", n, sampled, size)
	}
}

func TestCongestionPacer_DisabledIsPassthrough(t *testing.T) {
	p := NewCongestionPacer(context.Background(), "auto", false)
	defer p.Close()

	src := bytes.NewReader([]byte("hello world"))
	wrapped := p.WrapReader(src)
	if wrapped != io.Reader(src) {
		t.Errorf("disabled pacer must return the reader unchanged")
	}
	if got := p.PacingRateMbps(); got != 0 {
		t.Errorf("disabled pacer PacingRateMbps() = %v, want 0", got)
	}
	if p.bbrMetrics() != nil {
		t.Errorf("disabled pacer should have no BBR metrics")
	}
	p.SignalThrottle(1024) // must not panic
	p.Close()              // idempotent
}

// TestCongestionPacer_EnabledFeedsProber runs data through an enabled pacer and
// verifies the bytes are intact, the BBR prober is live (non-zero pacing rate),
// and real metrics are available.
func TestCongestionPacer_EnabledFeedsProber(t *testing.T) {
	p := NewCongestionPacer(context.Background(), "bbr", true)
	defer p.Close()

	want := make([]byte, 300*1024)
	if _, err := rand.Read(want); err != nil {
		t.Fatalf("rand: %v", err)
	}
	got, err := io.ReadAll(p.WrapReader(bytes.NewReader(want)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("data corrupted through enabled pacer")
	}
	if p.PacingRateMbps() <= 0 {
		t.Errorf("enabled pacer should report a positive pacing rate, got %v", p.PacingRateMbps())
	}
	if p.bbrMetrics() == nil {
		t.Errorf("enabled pacer should expose BBR metrics")
	}
	if p.bytesSeen != int64(len(want)) {
		t.Errorf("pacer saw %d bytes, want %d", p.bytesSeen, len(want))
	}
}

func TestCongestionPacer_SignalThrottleIsSafe(t *testing.T) {
	p := NewCongestionPacer(context.Background(), "auto", true)
	defer p.Close()
	// Feed a little data so the prober has state, then signal repeated throttles.
	_, _ = io.Copy(io.Discard, p.WrapReader(bytes.NewReader(bytes.Repeat([]byte("z"), 64*1024))))
	for i := 0; i < 10; i++ {
		p.SignalThrottle(1 << 20)
	}
	if p.PacingRateMbps() <= 0 {
		t.Errorf("prober should still report a rate after throttles, got %v", p.PacingRateMbps())
	}
}
