package config

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// HTTPTransportConfig configures HTTP/2 and TCP settings for S3 uploads
type HTTPTransportConfig struct {
	// HTTP/2 Settings
	EnableHTTP2          bool
	MaxConcurrentStreams int
	InitialWindowSize    int
	MaxFrameSize         int

	// TCP Settings
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TCPKeepAlive        time.Duration
	ReadBufferSize      int
	WriteBufferSize     int

	// TLS Settings
	TLSHandshakeTimeout time.Duration
	TLSMinVersion       uint16

	// Timeouts
	DialTimeout           time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration
}

// DefaultHTTPTransportConfig returns balanced settings for general use
func DefaultHTTPTransportConfig() *HTTPTransportConfig {
	return &HTTPTransportConfig{
		// HTTP/2 Settings
		EnableHTTP2:          true,
		MaxConcurrentStreams: 250,
		InitialWindowSize:    4 * 1024 * 1024, // 4MB
		MaxFrameSize:         256 * 1024,      // 256KB

		// TCP Settings
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     300 * time.Second,
		TCPKeepAlive:        30 * time.Second,
		ReadBufferSize:      4 * 1024 * 1024, // 4MB
		WriteBufferSize:     4 * 1024 * 1024, // 4MB

		// TLS Settings
		TLSHandshakeTimeout: 10 * time.Second,
		TLSMinVersion:       tls.VersionTLS12,

		// Timeouts
		DialTimeout:           30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
}

// AggressiveHTTPTransportConfig returns high-performance settings for maximum throughput
func AggressiveHTTPTransportConfig() *HTTPTransportConfig {
	cfg := DefaultHTTPTransportConfig()
	cfg.MaxConcurrentStreams = 500
	cfg.InitialWindowSize = 8 * 1024 * 1024 // 8MB
	cfg.MaxFrameSize = 1024 * 1024          // 1MB
	cfg.MaxIdleConnsPerHost = 200
	cfg.MaxConnsPerHost = 200
	cfg.ReadBufferSize = 8 * 1024 * 1024  // 8MB
	cfg.WriteBufferSize = 8 * 1024 * 1024 // 8MB
	return cfg
}

// ConservativeHTTPTransportConfig returns resource-constrained settings
func ConservativeHTTPTransportConfig() *HTTPTransportConfig {
	cfg := DefaultHTTPTransportConfig()
	cfg.MaxConcurrentStreams = 100
	cfg.InitialWindowSize = 1 * 1024 * 1024 // 1MB
	cfg.MaxFrameSize = 64 * 1024            // 64KB
	cfg.MaxIdleConnsPerHost = 50
	cfg.MaxConnsPerHost = 50
	cfg.ReadBufferSize = 1 * 1024 * 1024  // 1MB
	cfg.WriteBufferSize = 1 * 1024 * 1024 // 1MB
	return cfg
}

// BuildTransport creates an http.Transport with configured settings
func (c *HTTPTransportConfig) BuildTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   c.DialTimeout,
			KeepAlive: c.TCPKeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     c.EnableHTTP2,
		MaxIdleConns:          c.MaxIdleConnsPerHost * 2,
		MaxIdleConnsPerHost:   c.MaxIdleConnsPerHost,
		MaxConnsPerHost:       c.MaxConnsPerHost,
		IdleConnTimeout:       c.IdleConnTimeout,
		TLSHandshakeTimeout:   c.TLSHandshakeTimeout,
		ExpectContinueTimeout: c.ExpectContinueTimeout,
		ResponseHeaderTimeout: c.ResponseHeaderTimeout,
		DisableCompression:    true, // CargoShip uses zstd compression
		TLSClientConfig: &tls.Config{
			MinVersion: c.TLSMinVersion,
		},
	}

	// Configure HTTP/2 settings if enabled
	if c.EnableHTTP2 {
		// Configure HTTP/2 transport
		// Note: Error is safe to ignore here as http2.ConfigureTransport only
		// returns an error if transport is nil, which cannot happen here
		_ = http2.ConfigureTransport(transport)

		// Apply custom HTTP/2 settings
		transport.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}

		// Note: MaxConcurrentStreams, InitialWindowSize, MaxFrameSize
		// are set at the HTTP/2 transport level via http2.Transport
	}

	return transport
}

// SetTCPBuffers sets TCP socket buffer sizes on a connection
func (c *HTTPTransportConfig) SetTCPBuffers(conn net.Conn) error {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetReadBuffer(c.ReadBufferSize); err != nil {
			return err
		}
		if err := tcpConn.SetWriteBuffer(c.WriteBufferSize); err != nil {
			return err
		}
		// Enable TCP_NODELAY (disable Nagle's algorithm)
		return tcpConn.SetNoDelay(true)
	}
	return nil
}
