package config

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestDefaultHTTPTransportConfig(t *testing.T) {
	cfg := DefaultHTTPTransportConfig()

	if !cfg.EnableHTTP2 {
		t.Error("Expected HTTP/2 enabled by default")
	}
	if cfg.MaxConcurrentStreams != 250 {
		t.Errorf("Expected 250 streams, got %d", cfg.MaxConcurrentStreams)
	}
	if cfg.InitialWindowSize != 4*1024*1024 {
		t.Errorf("Expected 4MB window size, got %d", cfg.InitialWindowSize)
	}
	if cfg.MaxFrameSize != 256*1024 {
		t.Errorf("Expected 256KB frame size, got %d", cfg.MaxFrameSize)
	}
	if cfg.MaxIdleConnsPerHost != 100 {
		t.Errorf("Expected 100 max idle conns, got %d", cfg.MaxIdleConnsPerHost)
	}
	if cfg.MaxConnsPerHost != 100 {
		t.Errorf("Expected 100 max conns, got %d", cfg.MaxConnsPerHost)
	}
	if cfg.IdleConnTimeout != 300*time.Second {
		t.Errorf("Expected 300s timeout, got %v", cfg.IdleConnTimeout)
	}
	if cfg.TCPKeepAlive != 30*time.Second {
		t.Errorf("Expected 30s keep-alive, got %v", cfg.TCPKeepAlive)
	}
	if cfg.ReadBufferSize != 4*1024*1024 {
		t.Errorf("Expected 4MB read buffer, got %d", cfg.ReadBufferSize)
	}
	if cfg.WriteBufferSize != 4*1024*1024 {
		t.Errorf("Expected 4MB write buffer, got %d", cfg.WriteBufferSize)
	}
	if cfg.TLSMinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2, got %d", cfg.TLSMinVersion)
	}
}

func TestAggressiveHTTPTransportConfig(t *testing.T) {
	cfg := AggressiveHTTPTransportConfig()

	if !cfg.EnableHTTP2 {
		t.Error("Expected HTTP/2 enabled for aggressive profile")
	}
	if cfg.MaxConcurrentStreams != 500 {
		t.Errorf("Expected 500 streams for aggressive, got %d", cfg.MaxConcurrentStreams)
	}
	if cfg.InitialWindowSize != 8*1024*1024 {
		t.Errorf("Expected 8MB window size for aggressive, got %d", cfg.InitialWindowSize)
	}
	if cfg.MaxFrameSize != 1024*1024 {
		t.Errorf("Expected 1MB frame size for aggressive, got %d", cfg.MaxFrameSize)
	}
	if cfg.MaxIdleConnsPerHost != 200 {
		t.Errorf("Expected 200 max idle conns for aggressive, got %d", cfg.MaxIdleConnsPerHost)
	}
	if cfg.MaxConnsPerHost != 200 {
		t.Errorf("Expected 200 max conns for aggressive, got %d", cfg.MaxConnsPerHost)
	}
	if cfg.ReadBufferSize != 8*1024*1024 {
		t.Errorf("Expected 8MB read buffer for aggressive, got %d", cfg.ReadBufferSize)
	}
	if cfg.WriteBufferSize != 8*1024*1024 {
		t.Errorf("Expected 8MB write buffer for aggressive, got %d", cfg.WriteBufferSize)
	}
}

func TestConservativeHTTPTransportConfig(t *testing.T) {
	cfg := ConservativeHTTPTransportConfig()

	if !cfg.EnableHTTP2 {
		t.Error("Expected HTTP/2 enabled for conservative profile")
	}
	if cfg.MaxConcurrentStreams != 100 {
		t.Errorf("Expected 100 streams for conservative, got %d", cfg.MaxConcurrentStreams)
	}
	if cfg.InitialWindowSize != 1*1024*1024 {
		t.Errorf("Expected 1MB window size for conservative, got %d", cfg.InitialWindowSize)
	}
	if cfg.MaxFrameSize != 64*1024 {
		t.Errorf("Expected 64KB frame size for conservative, got %d", cfg.MaxFrameSize)
	}
	if cfg.MaxIdleConnsPerHost != 50 {
		t.Errorf("Expected 50 max idle conns for conservative, got %d", cfg.MaxIdleConnsPerHost)
	}
	if cfg.MaxConnsPerHost != 50 {
		t.Errorf("Expected 50 max conns for conservative, got %d", cfg.MaxConnsPerHost)
	}
	if cfg.ReadBufferSize != 1*1024*1024 {
		t.Errorf("Expected 1MB read buffer for conservative, got %d", cfg.ReadBufferSize)
	}
	if cfg.WriteBufferSize != 1*1024*1024 {
		t.Errorf("Expected 1MB write buffer for conservative, got %d", cfg.WriteBufferSize)
	}
}

func TestBuildTransport(t *testing.T) {
	cfg := DefaultHTTPTransportConfig()
	transport := cfg.BuildTransport()

	if transport.MaxIdleConnsPerHost != 100 {
		t.Errorf("Expected 100 max idle conns, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 100 {
		t.Errorf("Expected 100 max conns per host, got %d", transport.MaxConnsPerHost)
	}
	// Issue #34 Phase 2.3: Increased from 200 to 1024 for better connection reuse
	if transport.MaxIdleConns != 1024 {
		t.Errorf("Expected 1024 max idle conns, got %d", transport.MaxIdleConns)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("Expected ForceAttemptHTTP2 enabled")
	}
	if !transport.DisableCompression {
		t.Error("Expected compression disabled")
	}
	if transport.IdleConnTimeout != 300*time.Second {
		t.Errorf("Expected 300s idle timeout, got %v", transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("Expected 10s TLS timeout, got %v", transport.TLSHandshakeTimeout)
	}
	if transport.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("Expected 1s expect continue timeout, got %v", transport.ExpectContinueTimeout)
	}
	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("Expected 30s response header timeout, got %v", transport.ResponseHeaderTimeout)
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2, got %d", transport.TLSClientConfig.MinVersion)
	}
	if len(transport.TLSClientConfig.NextProtos) != 2 {
		t.Errorf("Expected 2 NextProtos, got %d", len(transport.TLSClientConfig.NextProtos))
	}
	if transport.TLSClientConfig.NextProtos[0] != "h2" {
		t.Errorf("Expected first NextProto to be h2, got %s", transport.TLSClientConfig.NextProtos[0])
	}
}

func TestBuildTransportWithHTTP2Disabled(t *testing.T) {
	cfg := DefaultHTTPTransportConfig()
	cfg.EnableHTTP2 = false
	transport := cfg.BuildTransport()

	if transport.ForceAttemptHTTP2 {
		t.Error("Expected ForceAttemptHTTP2 disabled when HTTP/2 is disabled")
	}
}

func TestNetworkProfiles(t *testing.T) {
	tests := []struct {
		name           string
		config         *HTTPTransportConfig
		streams        int
		windowSize     int
		frameSize      int
		maxIdleConns   int
		readBufferSize int
	}{
		{
			name:           "default",
			config:         DefaultHTTPTransportConfig(),
			streams:        250,
			windowSize:     4 * 1024 * 1024,
			frameSize:      256 * 1024,
			maxIdleConns:   100,
			readBufferSize: 4 * 1024 * 1024,
		},
		{
			name:           "aggressive",
			config:         AggressiveHTTPTransportConfig(),
			streams:        500,
			windowSize:     8 * 1024 * 1024,
			frameSize:      1024 * 1024,
			maxIdleConns:   200,
			readBufferSize: 8 * 1024 * 1024,
		},
		{
			name:           "conservative",
			config:         ConservativeHTTPTransportConfig(),
			streams:        100,
			windowSize:     1 * 1024 * 1024,
			frameSize:      64 * 1024,
			maxIdleConns:   50,
			readBufferSize: 1 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.MaxConcurrentStreams != tt.streams {
				t.Errorf("Expected %d streams, got %d", tt.streams, tt.config.MaxConcurrentStreams)
			}
			if tt.config.InitialWindowSize != tt.windowSize {
				t.Errorf("Expected %d window size, got %d", tt.windowSize, tt.config.InitialWindowSize)
			}
			if tt.config.MaxFrameSize != tt.frameSize {
				t.Errorf("Expected %d frame size, got %d", tt.frameSize, tt.config.MaxFrameSize)
			}
			if tt.config.MaxIdleConnsPerHost != tt.maxIdleConns {
				t.Errorf("Expected %d max idle conns, got %d", tt.maxIdleConns, tt.config.MaxIdleConnsPerHost)
			}
			if tt.config.ReadBufferSize != tt.readBufferSize {
				t.Errorf("Expected %d read buffer size, got %d", tt.readBufferSize, tt.config.ReadBufferSize)
			}
		})
	}
}

func TestHTTPTransportConfigCustomization(t *testing.T) {
	cfg := DefaultHTTPTransportConfig()

	// Modify configuration
	cfg.MaxConcurrentStreams = 300
	cfg.InitialWindowSize = 6 * 1024 * 1024
	cfg.MaxIdleConnsPerHost = 150
	cfg.IdleConnTimeout = 600 * time.Second

	// Verify modifications
	if cfg.MaxConcurrentStreams != 300 {
		t.Errorf("Expected 300 streams after modification, got %d", cfg.MaxConcurrentStreams)
	}
	if cfg.InitialWindowSize != 6*1024*1024 {
		t.Errorf("Expected 6MB window size after modification, got %d", cfg.InitialWindowSize)
	}
	if cfg.MaxIdleConnsPerHost != 150 {
		t.Errorf("Expected 150 max idle conns after modification, got %d", cfg.MaxIdleConnsPerHost)
	}
	if cfg.IdleConnTimeout != 600*time.Second {
		t.Errorf("Expected 600s idle timeout after modification, got %v", cfg.IdleConnTimeout)
	}

	// Build transport and verify changes propagate
	transport := cfg.BuildTransport()
	if transport.MaxIdleConnsPerHost != 150 {
		t.Errorf("Expected transport to have 150 max idle conns, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 600*time.Second {
		t.Errorf("Expected transport to have 600s idle timeout, got %v", transport.IdleConnTimeout)
	}
}

func BenchmarkBuildTransport(b *testing.B) {
	cfg := DefaultHTTPTransportConfig()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cfg.BuildTransport()
	}
}

func BenchmarkDefaultHTTPTransportConfig(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = DefaultHTTPTransportConfig()
	}
}
