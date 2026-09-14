package bindguard

import (
	"strings"
	"testing"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"localhost:6060", true},
		{"LocalHost:6060", true},
		{"127.0.0.1:6060", true},
		{"127.0.0.5:9090", true},
		{"[::1]:6060", true},
		{":9090", false},        // wildcard: all interfaces
		{"0.0.0.0:9090", false}, // explicit all-interfaces
		{"[::]:9090", false},
		{"192.168.1.10:9090", false},
		{"10.0.0.1:6060", false},
		{"metrics.internal:9090", false}, // unresolved hostname → fail safe
		{"localhost", true},              // no port
		{"127.0.0.1", true},
		{"example.com", false},
		{"", false}, // empty host
	}
	for _, tt := range tests {
		if got := IsLoopback(tt.addr); got != tt.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestGuard(t *testing.T) {
	// No listener configured always passes.
	if err := Guard("pprof", "", false); err != nil {
		t.Errorf("empty addr should pass: %v", err)
	}
	// Loopback passes without the opt-in.
	if err := Guard("pprof", "localhost:6060", false); err != nil {
		t.Errorf("loopback should pass: %v", err)
	}
	// Public bind is refused without the opt-in...
	err := Guard("Prometheus metrics", ":9090", false)
	if err == nil {
		t.Fatal("public bind without --allow-public-observability must be refused")
	}
	if !strings.Contains(err.Error(), "--allow-public-observability") ||
		!strings.Contains(err.Error(), "Prometheus metrics") {
		t.Errorf("error should name the listener and the opt-out flag, got: %v", err)
	}
	// ...and allowed with it.
	if err := Guard("Prometheus metrics", ":9090", true); err != nil {
		t.Errorf("public bind with opt-in should pass: %v", err)
	}
}
