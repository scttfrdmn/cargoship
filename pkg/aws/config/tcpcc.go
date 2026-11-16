package config

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// Congestion-control requests accepted by --congestion-control (#881).
const (
	// CongestionAuto prefers BBR where this process may set it, and otherwise
	// leaves the kernel default alone.
	CongestionAuto = "auto"
	// CongestionBBR requests BBR on every upload socket.
	CongestionBBR = "bbr"
	// CongestionCubic requests CUBIC on every upload socket.
	CongestionCubic = "cubic"
)

// CongestionReport is the outcome of resolving a --congestion-control request
// against what the host actually supports.
//
// Effective is the algorithm the S3 sockets will be set to. Empty means
// "whatever the OS default is" — nothing is set per socket. Warning is non-empty
// when an explicit request could not be honoured; it names the cause and the
// fix, and callers must surface it rather than let the request be silently
// ignored (the bug #881 fixed).
type CongestionReport struct {
	Requested     string
	Effective     string
	KernelDefault string
	Warning       string
}

// String renders the report for a one-line status display.
func (r CongestionReport) String() string {
	switch {
	case r.Effective != "":
		return fmt.Sprintf("%s (requested %s)", r.Effective, r.Requested)
	case r.KernelDefault != "":
		return fmt.Sprintf("kernel default, %s (requested %s)", r.KernelDefault, r.Requested)
	default:
		return fmt.Sprintf("OS default (requested %s)", r.Requested)
	}
}

// NormalizeCongestionControl validates a --congestion-control value. The empty
// string is treated as auto.
func NormalizeCongestionControl(v string) (string, error) {
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "", CongestionAuto:
		return CongestionAuto, nil
	case CongestionBBR, CongestionCubic:
		return s, nil
	default:
		return "", fmt.Errorf("unknown congestion control %q (want bbr, cubic or auto)", v)
	}
}

// ResolveCongestionControl decides which algorithm upload sockets will use for
// a validated request, from the running host's capabilities.
func ResolveCongestionControl(requested string) CongestionReport {
	return resolveCongestion(requested, probeHost())
}

// hostCongestion is what the platform reports about per-socket congestion
// control. supported is false where it can't be set at all.
type hostCongestion struct {
	supported  bool
	defaultCC  string
	available  []string
	allowed    []string
	privileged bool
	reason     string // why unsupported, when !supported
}

func (h hostCongestion) canSet(alg string) bool {
	if !contains(h.available, alg) {
		return false
	}
	// Unprivileged processes may only pick from the allowed list; CAP_NET_ADMIN
	// (approximated by root) may set anything available.
	return h.privileged || contains(h.allowed, alg)
}

func resolveCongestion(requested string, h hostCongestion) CongestionReport {
	r := CongestionReport{Requested: requested, KernelDefault: h.defaultCC}
	if !h.supported {
		if requested != CongestionAuto {
			r.Warning = fmt.Sprintf("--congestion-control %s has no effect here: %s; using the OS default", requested, h.reason)
		}
		return r
	}

	want := requested
	if requested == CongestionAuto {
		if h.canSet(CongestionBBR) {
			r.Effective = CongestionBBR
		}
		// auto never warns: falling back to the kernel default is its contract.
		return r
	}

	if h.canSet(want) {
		r.Effective = want
		return r
	}
	switch {
	case !contains(h.available, want) && want == CongestionBBR:
		r.Warning = "--congestion-control bbr requested but the kernel has no BBR module loaded " +
			"(load it with `sudo modprobe tcp_bbr`); using the kernel default " + h.defaultCC
	case !contains(h.available, want):
		r.Warning = fmt.Sprintf("--congestion-control %s requested but the kernel does not offer it "+
			"(available: %s); using the kernel default %s", want, strings.Join(h.available, " "), h.defaultCC)
	default:
		r.Warning = fmt.Sprintf("--congestion-control %s requested but unprivileged processes may only use: %s. "+
			"Allow it with `sudo sysctl -w net.ipv4.tcp_allowed_congestion_control=\"%s %s\"`; using the kernel default %s",
			want, strings.Join(h.allowed, " "), strings.Join(h.allowed, " "), want, h.defaultCC)
	}
	return r
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// congestionDialStats counts per-socket outcomes so a failure at dial time is
// visible rather than silently leaving sockets on the kernel default.
type congestionDialStats struct {
	set    atomic.Int64
	failed atomic.Int64
	once   sync.Once
	warn   func(error)
}

// controlFunc returns a net.Dialer.Control hook that sets alg on each new
// socket. alg must be non-empty; failures are counted and reported once.
func (s *congestionDialStats) controlFunc(alg string) func(network, address string, c syscall.RawConn) error {
	return func(network, _ string, c syscall.RawConn) error {
		if !strings.HasPrefix(network, "tcp") {
			return nil
		}
		var sockErr error
		if err := c.Control(func(fd uintptr) { sockErr = setSocketCongestion(fd, alg) }); err != nil {
			sockErr = err
		}
		if sockErr != nil {
			s.failed.Add(1)
			s.once.Do(func() {
				if s.warn != nil {
					s.warn(fmt.Errorf("could not set TCP congestion control %q on an S3 socket (connections use the kernel default): %w", alg, sockErr))
				}
			})
			return nil // a slower algorithm is not a reason to fail the upload
		}
		s.set.Add(1)
		return nil
	}
}
