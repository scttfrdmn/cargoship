//go:build linux

package config

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// procNetIPv4 is where Linux publishes TCP congestion-control state. A variable
// so tests can point it at fixtures.
var procNetIPv4 = "/proc/sys/net/ipv4"

func probeHost() hostCongestion {
	return probeProc(procNetIPv4, os.Geteuid() == 0)
}

func probeProc(dir string, privileged bool) hostCongestion {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	h := hostCongestion{
		supported:  true,
		defaultCC:  read("tcp_congestion_control"),
		available:  strings.Fields(read("tcp_available_congestion_control")),
		allowed:    strings.Fields(read("tcp_allowed_congestion_control")),
		privileged: privileged,
	}
	if len(h.available) == 0 {
		h.supported = false
		h.reason = "the kernel's congestion-control list in " + dir + " is unreadable"
	}
	return h
}

func setSocketCongestion(fd uintptr, alg string) error {
	return unix.SetsockoptString(int(fd), unix.IPPROTO_TCP, unix.TCP_CONGESTION, alg)
}

// socketCongestion reads the algorithm a socket is using. Tests use it to
// prove the hook took effect on a real connection.
func socketCongestion(fd uintptr) (string, error) {
	return unix.GetsockoptString(int(fd), unix.IPPROTO_TCP, unix.TCP_CONGESTION)
}
