package profiling

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
	"time"
)

// RuntimeProfiler provides HTTP endpoint for runtime profiling
type RuntimeProfiler struct {
	server *http.Server
	addr   string
}

// RuntimeConfig configures runtime profiling
type RuntimeConfig struct {
	// Addr is the address to listen on (e.g., "localhost:6060")
	Addr string

	// ReadTimeout for HTTP server
	ReadTimeout time.Duration

	// WriteTimeout for HTTP server
	WriteTimeout time.Duration
}

// DefaultRuntimeConfig returns sensible defaults
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Addr:         "localhost:6060",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

// NewRuntimeProfiler creates a new runtime profiler
func NewRuntimeProfiler(config RuntimeConfig) *RuntimeProfiler {
	mux := http.NewServeMux()

	// pprof handlers are automatically registered at DefaultServeMux
	// We need to explicitly add them to our custom mux
	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		http.DefaultServeMux.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         config.Addr,
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return &RuntimeProfiler{
		server: server,
		addr:   config.Addr,
	}
}

// Start begins serving pprof endpoints
func (rp *RuntimeProfiler) Start(ctx context.Context) error {
	go func() {
		slog.Info("runtime profiler started",
			"addr", rp.addr,
			"endpoints", []string{
				"/debug/pprof/",
				"/debug/pprof/cmdline",
				"/debug/pprof/profile",
				"/debug/pprof/symbol",
				"/debug/pprof/trace",
			})

		if err := rp.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("runtime profiler failed", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return rp.Stop()
}

// Stop gracefully shuts down the profiler
func (rp *RuntimeProfiler) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rp.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown runtime profiler: %w", err)
	}

	slog.Info("runtime profiler stopped")
	return nil
}

// Addr returns the address the profiler is listening on
func (rp *RuntimeProfiler) Addr() string {
	return rp.addr
}

// StartRuntimeProfiler is a convenience function to start profiling in the background
func StartRuntimeProfiler(addr string) (*RuntimeProfiler, error) {
	if addr == "" {
		addr = "localhost:6060"
	}

	config := RuntimeConfig{
		Addr:         addr,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	profiler := NewRuntimeProfiler(config)

	// Start in background
	go func() {
		if err := profiler.Start(context.Background()); err != nil {
			slog.Error("runtime profiler error", "error", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	slog.Info("runtime profiler enabled",
		"addr", addr,
		"usage", fmt.Sprintf("go tool pprof http://%s/debug/pprof/profile?seconds=30", addr))

	return profiler, nil
}
