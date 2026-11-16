package config

import (
	"net"
	"strings"
	"testing"
)

func TestNormalizeCongestionControl(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"", CongestionAuto, false},
		{"auto", CongestionAuto, false},
		{" BBR ", CongestionBBR, false},
		{"cubic", CongestionCubic, false},
		{"reno", "", true},
		{"bbr2", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeCongestionControl(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("NormalizeCongestionControl(%q) = %q, %v; want %q, err=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestResolveCongestion(t *testing.T) {
	linux := func(def string, avail, allowed []string, priv bool) hostCongestion {
		return hostCongestion{supported: true, defaultCC: def, available: avail, allowed: allowed, privileged: priv}
	}
	bbrAllowed := linux("cubic", []string{"reno", "cubic", "bbr"}, []string{"reno", "cubic", "bbr"}, false)
	bbrNotAllowed := linux("cubic", []string{"reno", "cubic", "bbr"}, []string{"reno", "cubic"}, false)
	noBBR := linux("cubic", []string{"reno", "cubic"}, []string{"reno", "cubic"}, false)
	unsupported := hostCongestion{supported: false, reason: "not linux"}

	tests := []struct {
		name          string
		req           string
		host          hostCongestion
		wantEffective string
		wantWarn      string // substring; "" = no warning
	}{
		{"auto picks bbr when allowed", CongestionAuto, bbrAllowed, "bbr", ""},
		{"auto falls back silently when bbr not allowed", CongestionAuto, bbrNotAllowed, "", ""},
		{"auto falls back silently without bbr", CongestionAuto, noBBR, "", ""},
		{"root may set an available but unallowed bbr", CongestionAuto,
			linux("cubic", []string{"cubic", "bbr"}, []string{"cubic"}, true), "bbr", ""},
		{"explicit bbr honoured", CongestionBBR, bbrAllowed, "bbr", ""},
		{"explicit bbr, module missing", CongestionBBR, noBBR, "", "modprobe tcp_bbr"},
		{"explicit bbr, not allowed", CongestionBBR, bbrNotAllowed, "", "tcp_allowed_congestion_control"},
		{"explicit cubic honoured", CongestionCubic, bbrAllowed, "cubic", ""},
		{"explicit cubic unavailable", CongestionCubic,
			linux("bbr", []string{"bbr"}, []string{"bbr"}, false), "", "does not offer it"},
		{"auto on unsupported OS is quiet", CongestionAuto, unsupported, "", ""},
		{"explicit on unsupported OS warns", CongestionBBR, unsupported, "", "has no effect here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := resolveCongestion(tt.req, tt.host)
			if r.Effective != tt.wantEffective {
				t.Errorf("Effective = %q, want %q", r.Effective, tt.wantEffective)
			}
			if tt.wantWarn == "" && r.Warning != "" {
				t.Errorf("unexpected warning: %s", r.Warning)
			}
			if tt.wantWarn != "" && !strings.Contains(r.Warning, tt.wantWarn) {
				t.Errorf("warning %q does not mention %q", r.Warning, tt.wantWarn)
			}
		})
	}
}

func TestCongestionReportString(t *testing.T) {
	cases := map[string]CongestionReport{
		"bbr (requested auto)":                  {Requested: "auto", Effective: "bbr", KernelDefault: "cubic"},
		"kernel default, cubic (requested bbr)": {Requested: "bbr", KernelDefault: "cubic"},
		"OS default (requested auto)":           {Requested: "auto"},
	}
	for want, r := range cases {
		if got := r.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestClientCacheKey_CongestionControlSeparatesClients(t *testing.T) {
	a := ClientCacheKey{Bucket: "b", Region: "r"}
	b := ClientCacheKey{Bucket: "b", Region: "r", CongestionControl: "bbr"}
	if a.Hash() == b.Hash() {
		t.Fatal("clients with different TCP algorithms must not share a cache entry")
	}
	if a.String() != "b:r:" {
		t.Errorf("a key without an algorithm must keep its pre-#881 form, got %q", a.String())
	}
}

func TestDialerReportsUnsettableAlgorithmOnceAndStillConnects(t *testing.T) {
	var calls int
	cfg := DefaultHTTPTransportConfig()
	cfg.CongestionControl = "no-such-algorithm"
	cfg.OnCongestionError = func(error) { calls++ }

	stats := &congestionDialStats{warn: cfg.OnCongestionError}
	d := cfg.dialer()
	d.Control = stats.controlFunc(cfg.CongestionControl)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	for i := 0; i < 3; i++ {
		conn, err := d.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("a slower algorithm must never fail the connection: %v", err)
		}
		_ = conn.Close()
	}
	if calls != 1 {
		t.Errorf("warning reported %d times, want exactly once", calls)
	}
	if got := stats.failed.Load(); got != 3 {
		t.Errorf("failed count = %d, want 3", got)
	}
}

// Guard against the hook silently doing nothing for non-TCP networks being
// mistaken for success: it must leave them alone without counting them.
func TestControlFuncIgnoresNonTCP(t *testing.T) {
	stats := &congestionDialStats{}
	if err := stats.controlFunc("cubic")("udp", "x", nil); err != nil {
		t.Fatal(err)
	}
	if stats.set.Load() != 0 || stats.failed.Load() != 0 {
		t.Error("non-TCP sockets must not be counted")
	}
}

// TestResolveCongestionControlOnThisHost exercises the real probe. Whatever the
// host, auto must never warn and an explicit request must either take effect or
// say why not.
func TestResolveCongestionControlOnThisHost(t *testing.T) {
	if r := ResolveCongestionControl(CongestionAuto); r.Warning != "" {
		t.Errorf("auto must not warn, got %q", r.Warning)
	}
	r := ResolveCongestionControl(CongestionBBR)
	if r.Effective != CongestionBBR && r.Warning == "" {
		t.Error("an explicit bbr that isn't honoured must carry a warning")
	}
}
