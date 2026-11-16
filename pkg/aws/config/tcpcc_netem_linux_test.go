//go:build linux

package config

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// TestCongestionControlNetemAB is the #881 acceptance check: on a long, lossy
// path, BBR set through cargoship's dialer must clearly beat CUBIC. Run only
// where the workflow has shaped loopback, e.g.
//
//	sudo tc qdisc add dev lo root netem delay 50ms loss 0.1%
//	CARGOSHIP_NETEM_AB=1 go test -run NetemAB ./pkg/aws/config
//
// aws-elastic-gcs measured 0.97 Gbps (bbr) vs 0.16 Gbps (cubic) at +100 ms and
// 0.1% loss; this asserts a conservative 2x so it isn't flaky.
func TestCongestionControlNetemAB(t *testing.T) {
	if os.Getenv("CARGOSHIP_NETEM_AB") != "1" {
		t.Skip("set CARGOSHIP_NETEM_AB=1 on a host with netem on lo")
	}
	h := probeHost()
	for _, alg := range []string{CongestionCubic, CongestionBBR} {
		if !h.canSet(alg) {
			t.Fatalf("A/B needs %s settable (available %v, allowed %v)", alg, h.available, h.allowed)
		}
	}
	window := 20 * time.Second
	rate := map[string]float64{}
	for _, alg := range []string{CongestionCubic, CongestionBBR} {
		rate[alg] = sendFor(t, alg, window)
		t.Logf("%-5s %.1f Mbps over %s", alg, rate[alg], window)
	}
	if rate[CongestionBBR] < 2*rate[CongestionCubic] {
		t.Fatalf("bbr %.1f Mbps is not >= 2x cubic %.1f Mbps on the lossy path", rate[CongestionBBR], rate[CongestionCubic])
	}
}

// sendFor streams zeros over one connection dialed with alg for d, and
// returns the receiver-side goodput in Mbps.
func sendFor(t *testing.T, alg string, d time.Duration) float64 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan int64, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- 0
			return
		}
		defer func() { _ = c.Close() }()
		n, _ := io.Copy(io.Discard, c)
		got <- n
	}()

	cfg := DefaultHTTPTransportConfig()
	cfg.CongestionControl = alg
	conn, err := cfg.dialer().DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256<<10)
	deadline := time.Now().Add(d)
	start := time.Now()
	for time.Now().Before(deadline) {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write(buf); err != nil {
			break // deadline reached mid-write
		}
	}
	_ = conn.Close()
	n := <-got
	return float64(n*8) / time.Since(start).Seconds() / 1e6
}
