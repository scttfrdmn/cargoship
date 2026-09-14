// Package bindguard decides whether an observability listener address
// (pprof, Prometheus metrics) may be bound, refusing a non-loopback bind unless
// the operator explicitly opts in.
//
// The pprof and Prometheus endpoints are unauthenticated: pprof exposes runtime
// internals (and its /debug/pprof/cmdline can leak arguments), and /metrics can
// leak operational detail. Binding them to a public interface exposes an
// unauthenticated endpoint to the network. Defaults are already safe (pprof
// binds localhost, Prometheus is unset), so this only guards an explicit
// non-loopback address (CSH-SEC-007).
package bindguard

import (
	"fmt"
	"net"
	"strings"
)

// IsLoopback reports whether addr binds only the loopback interface.
//
// It is deliberately conservative and does NOT resolve DNS: a missing or
// wildcard host ("", ":9090", "0.0.0.0:9090", "[::]:9090") binds all interfaces
// and is not loopback; "localhost" and any loopback IP literal (127.0.0.0/8,
// ::1) are loopback; any other hostname is treated as non-loopback (fail safe)
// so an unresolved or externally-resolvable name is guarded rather than trusted.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No host:port split (e.g. a bare host or a bare ":port" edge case
		// already handled above returns ""); fall back to the whole string.
		host = addr
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false // wildcard bind → all interfaces
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false // unknown hostname → treat as public
}

// Guard returns a non-nil error when addr binds a non-loopback interface and
// allowPublic is false. kind names the listener in the message (e.g. "pprof" or
// "Prometheus metrics"). An empty addr means "no listener" and always passes.
func Guard(kind, addr string, allowPublic bool) error {
	if addr == "" || allowPublic || IsLoopback(addr) {
		return nil
	}
	return fmt.Errorf(
		"%s address %q binds a non-loopback interface, exposing an unauthenticated endpoint to the network; "+
			"bind a loopback address (e.g. localhost:PORT) or pass --allow-public-observability to accept the exposure",
		kind, addr)
}
