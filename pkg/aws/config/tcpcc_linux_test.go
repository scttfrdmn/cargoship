//go:build linux

package config

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeProc(t *testing.T) {
	dir := t.TempDir()
	write := func(name, v string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tcp_congestion_control", "cubic")
	write("tcp_available_congestion_control", "reno cubic bbr")
	write("tcp_allowed_congestion_control", "reno cubic")

	h := probeProc(dir, false)
	if !h.supported || h.defaultCC != "cubic" || len(h.available) != 3 || len(h.allowed) != 2 {
		t.Fatalf("probeProc parsed %+v", h)
	}
	if h.canSet("bbr") {
		t.Error("an unprivileged process must not be able to set bbr when it isn't allowed")
	}
	if !probeProc(dir, true).canSet("bbr") {
		t.Error("a privileged process may set any available algorithm")
	}
	if probeProc(filepath.Join(dir, "missing"), false).supported {
		t.Error("an unreadable /proc must be reported as unsupported, not as an empty kernel")
	}
}

// dialAndRead dials a loopback listener through the transport's dialer and
// returns the congestion-control algorithm the kernel reports for the socket.
func dialAndRead(t *testing.T, cfg *HTTPTransportConfig) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer func() { _ = c.Close() }()
			time.Sleep(200 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cfg.dialer().DialContext(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	raw, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var alg string
	var gerr error
	if err := raw.Control(func(fd uintptr) { alg, gerr = socketCongestion(fd) }); err != nil {
		t.Fatal(err)
	}
	if gerr != nil {
		t.Fatalf("getsockopt TCP_CONGESTION: %v", gerr)
	}
	return alg
}

// TestDialerSetsCongestionOnRealSocket proves the hook changes what the KERNEL
// uses for the connection — the property #881 found missing.
func TestDialerSetsCongestionOnRealSocket(t *testing.T) {
	h := probeHost()
	for _, alg := range []string{CongestionCubic, CongestionBBR} {
		t.Run(alg, func(t *testing.T) {
			if !h.canSet(alg) {
				t.Skipf("this host can't set %s (available %v, allowed %v)", alg, h.available, h.allowed)
			}
			cfg := DefaultHTTPTransportConfig()
			cfg.CongestionControl = alg
			if got := dialAndRead(t, cfg); got != alg {
				t.Fatalf("socket uses %q, want %q", got, alg)
			}
		})
	}
}
