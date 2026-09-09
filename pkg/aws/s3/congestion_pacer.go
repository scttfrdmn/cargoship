package s3

import (
	"context"
	"io"
	"sync"
	"time"
)

// mbpsToBytesPerSec converts megabits/sec — the unit BBRBandwidthProber reports
// bandwidth and pacing in — to bytes/sec. It matches the Mbps→Bps conversion the
// prober itself uses (mebibits: Mbps * 1024 * 1024 / 8), so a pacing rate handed
// back here means the same thing it does inside the prober.
func mbpsToBytesPerSec(mbps float64) float64 {
	return mbps * 1024 * 1024 / 8
}

// pacerWarmup is how long a paced reader only measures before it will enforce a
// rate. BBR starts in STARTUP with an initial 10 Mbps estimate; enforcing that
// before real samples have raised the estimate would throttle a fast link down
// to 10 Mbps. During warmup we feed the prober real samples so its estimate
// climbs to the bottleneck first; only then can pacing engage, and by then the
// STARTUP gain (>1) keeps the rate at or above observed throughput.
const pacerWarmup = 750 * time.Millisecond

// CongestionPacer wires a real BBR bandwidth prober into an upload. It feeds the
// prober delivery-rate samples measured from the actual drain of the body reader
// and paces the body at the prober's estimated rate.
//
// Because BBR's pacing rate is the observed bottleneck bandwidth times a gain
// that is >= 1 outside of recovery, pacing converges to the link and does not
// throttle below measured throughput on an uncongested path. It only bites when
// a congestion signal — an S3 503 SlowDown fed through SignalThrottle, or BBR's
// own ProbeRTT/Drain phases — lowers the estimate. When optimization is disabled
// the pacer is a transparent passthrough with no prober and no overhead.
type CongestionPacer struct {
	prober  *BBRBandwidthProber
	enabled bool

	mu        sync.Mutex
	started   bool
	bytesSeen int64
}

// NewCongestionPacer builds a pacer. algorithm is "bbr", "cubic" or "auto"
// (retained for reporting and future CUBIC-only pacing; BBR drives pacing in all
// enabled modes today). When enabled is false the pacer does nothing.
func NewCongestionPacer(ctx context.Context, algorithm string, enabled bool) *CongestionPacer {
	// algorithm ("bbr"/"cubic"/"auto") is accepted for API stability and future
	// CUBIC-only pacing; BBR drives pacing in all enabled modes today.
	_ = algorithm
	p := &CongestionPacer{enabled: enabled}
	if !enabled {
		return p
	}
	p.prober = NewBBRBandwidthProber(ctx, nil)
	// StartProbing runs the BBR state machine loop; without it the pacing rate
	// and congestion window never advance past their initial values.
	if err := p.prober.StartProbing(); err == nil {
		p.started = true
	}
	return p
}

// WrapReader returns a reader that paces and measures r. When the pacer is
// disabled it returns r unchanged.
func (p *CongestionPacer) WrapReader(r io.Reader) io.Reader {
	if !p.enabled || p.prober == nil {
		return r
	}
	return newPacedReader(r,
		func() float64 { return mbpsToBytesPerSec(p.prober.GetPacingRate()) },
		p.sample,
		pacerWarmup,
	)
}

// sample feeds one measured drain interval to the prober and tallies bytes.
func (p *CongestionPacer) sample(n int, start, now time.Time) {
	rtt := now.Sub(start)
	if rtt <= 0 {
		rtt = time.Microsecond
	}
	p.prober.OnPacketSent(int64(n), start)
	p.prober.OnPacketAcknowledged(int64(n), start, now, rtt)
	p.mu.Lock()
	p.bytesSeen += int64(n)
	p.mu.Unlock()
}

// SignalThrottle reports a server-side congestion signal (e.g. an S3 503
// SlowDown / RequestLimitExceeded). It is BBR's loss input: the estimate and
// window contract, so subsequent pacing actually limits the send rate.
func (p *CongestionPacer) SignalThrottle(bytesInFlight int64) {
	if p.prober == nil {
		return
	}
	if bytesInFlight <= 0 {
		bytesInFlight = 1
	}
	p.prober.OnPacketLost(bytesInFlight, time.Now())
}

// PacingRateMbps is the prober's current pacing rate, or 0 when disabled.
func (p *CongestionPacer) PacingRateMbps() float64 {
	if p.prober == nil {
		return 0
	}
	return p.prober.GetPacingRate()
}

// Close stops the BBR loop. Safe to call more than once.
func (p *CongestionPacer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started && p.prober != nil {
		_ = p.prober.StopProbing()
		p.started = false
	}
}

// bbrMetrics exposes the prober's live metrics for real (measured) reporting.
// Returns nil when disabled.
func (p *CongestionPacer) bbrMetrics() *BBRMetrics {
	if p.prober == nil {
		return nil
	}
	return p.prober.GetMetrics()
}

// pacedReader wraps the upload body. Each Read (1) waits for token-bucket credit
// at the current pacing rate, then (2) reads from the underlying body and hands
// the elapsed-time/bytes pair to sampleFn as a delivery-rate sample. The read
// itself blocks on the SDK's part-buffer backpressure, so its duration reflects
// real upload throughput rather than local buffering.
//
// rateFn returns the current allowed rate in bytes/sec; <= 0 means unlimited.
// It is a function rather than a fixed value because BBR revises the rate
// continuously as samples arrive.
type pacedReader struct {
	r        io.Reader
	rateFn   func() float64
	sampleFn func(n int, start, now time.Time)
	warmup   time.Duration

	tokens     float64
	lastRefill time.Time
	firstRead  time.Time
}

func newPacedReader(r io.Reader, rateFn func() float64, sampleFn func(int, time.Time, time.Time), warmup time.Duration) *pacedReader {
	return &pacedReader{r: r, rateFn: rateFn, sampleFn: sampleFn, warmup: warmup, lastRefill: time.Now()}
}

func (pr *pacedReader) Read(b []byte) (int, error) {
	pr.throttle(len(b))

	start := time.Now()
	n, err := pr.r.Read(b)
	if n > 0 {
		now := time.Now()
		if pr.sampleFn != nil {
			pr.sampleFn(n, start, now)
		}
		if pr.firstRead.IsZero() {
			pr.firstRead = now
		}
	}
	return n, err
}

// throttle enforces the token bucket at the current rate. It is a no-op during
// warmup and whenever the rate is non-positive.
func (pr *pacedReader) throttle(want int) {
	if pr.rateFn == nil || pr.firstRead.IsZero() || time.Since(pr.firstRead) < pr.warmup {
		return
	}
	rate := pr.rateFn()
	if rate <= 0 {
		return
	}

	now := time.Now()
	// Credits accrue at the pacing rate over wall-clock elapsed, capped at one
	// read's worth of burst so a stall can't bank unbounded credit.
	pr.tokens += rate * now.Sub(pr.lastRefill).Seconds()
	pr.lastRefill = now
	if burst := float64(want); pr.tokens > burst {
		pr.tokens = burst
	}

	if pr.tokens >= float64(want) {
		pr.tokens -= float64(want)
		return
	}
	// Not enough credit: sleep just long enough to accrue the shortfall.
	need := float64(want) - pr.tokens
	wait := time.Duration(need / rate * float64(time.Second))
	if wait > 0 {
		time.Sleep(wait)
	}
	pr.tokens = 0
}
