// Package metrics provides Prometheus metrics for CargoShip observability.
//
// This package exposes time-series metrics for monitoring upload performance,
// throughput, errors, and pipeline stage latencies. Metrics are exposed via
// HTTP endpoint (/metrics) for scraping by Prometheus or compatible systems.
//
// # Usage Example
//
//	// Create metrics collector
//	collector := metrics.NewPrometheusCollector()
//
//	// Start HTTP server for metrics endpoint
//	go collector.ServeMetrics(":9090")
//
//	// Record upload metrics
//	collector.RecordUploadComplete(ctx, uploadID, size, duration, "STANDARD")
//	collector.RecordUploadError(ctx, uploadID, "s3_timeout")
//
//	// Record pipeline stage metrics
//	collector.RecordStageLatency("scanner", duration)
//	collector.RecordStageLatency("archiver", duration)
//
// # Available Metrics
//
//   - cargoship_upload_duration_seconds - Histogram of upload durations
//   - cargoship_upload_size_bytes - Histogram of upload sizes
//   - cargoship_upload_throughput_mbps - Gauge of current upload throughput
//   - cargoship_upload_errors_total - Counter of upload errors by type
//   - cargoship_retry_attempts_total - Counter of retry attempts
//   - cargoship_stage_latency_seconds - Histogram of pipeline stage latencies
//   - cargoship_active_uploads - Gauge of currently active uploads
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
)

// PrometheusCollector collects and exposes Prometheus metrics for CargoShip
type PrometheusCollector struct {
	// Upload metrics
	uploadDuration   *prometheus.HistogramVec
	uploadSize       *prometheus.HistogramVec
	uploadThroughput *prometheus.GaugeVec
	uploadErrors     *prometheus.CounterVec
	activeUploads    prometheus.Gauge

	// Retry metrics
	retryAttempts *prometheus.CounterVec

	// Pipeline stage metrics
	stageLatency *prometheus.HistogramVec

	// Registry for metrics (allows isolation in tests)
	registry *prometheus.Registry

	// HTTP server
	server *http.Server
}

// NewPrometheusCollector creates a new Prometheus metrics collector
func NewPrometheusCollector() *PrometheusCollector {
	// Create custom registry to avoid global registration conflicts
	registry := prometheus.NewRegistry()

	return &PrometheusCollector{
		registry: registry,
		uploadDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "cargoship_upload_duration_seconds",
				Help: "Duration of upload operations in seconds",
				Buckets: []float64{
					1, 5, 10, 30, 60, // 1s to 1min
					120, 300, 600, // 2min to 10min
					1800, 3600, 7200, 14400, // 30min to 4hr
				},
			},
			[]string{"storage_class", "transporter", "status"},
		),

		uploadSize: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "cargoship_upload_size_bytes",
				Help: "Size of uploaded objects in bytes",
				Buckets: []float64{
					1024 * 1024,              // 1 MB
					10 * 1024 * 1024,         // 10 MB
					100 * 1024 * 1024,        // 100 MB
					500 * 1024 * 1024,        // 500 MB
					1024 * 1024 * 1024,       // 1 GB
					5 * 1024 * 1024 * 1024,   // 5 GB
					10 * 1024 * 1024 * 1024,  // 10 GB
					50 * 1024 * 1024 * 1024,  // 50 GB
					100 * 1024 * 1024 * 1024, // 100 GB
				},
			},
			[]string{"storage_class", "transporter"},
		),

		uploadThroughput: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cargoship_upload_throughput_mbps",
				Help: "Current upload throughput in MB/s",
			},
			[]string{"upload_id", "transporter"},
		),

		uploadErrors: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "cargoship_upload_errors_total",
				Help: "Total number of upload errors by type",
			},
			[]string{"error_type", "stage"},
		),

		activeUploads: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "cargoship_active_uploads",
				Help: "Number of currently active uploads",
			},
		),

		retryAttempts: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "cargoship_retry_attempts_total",
				Help: "Total number of retry attempts",
			},
			[]string{"stage", "reason"},
		),

		stageLatency: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "cargoship_stage_latency_seconds",
				Help: "Latency of pipeline stages in seconds",
				Buckets: []float64{
					0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300,
				},
			},
			[]string{"stage"},
		),
	}
}

// RecordUploadStart increments the active uploads counter
func (c *PrometheusCollector) RecordUploadStart() {
	c.activeUploads.Inc()
}

// RecordUploadComplete records metrics for a completed upload
func (c *PrometheusCollector) RecordUploadComplete(ctx context.Context, uploadID string, size int64, duration time.Duration, storageClass, transporter string) {
	c.activeUploads.Dec()

	// Record duration
	c.uploadDuration.WithLabelValues(storageClass, transporter, "success").Observe(duration.Seconds())

	// Record size
	c.uploadSize.WithLabelValues(storageClass, transporter).Observe(float64(size))

	// Calculate and record throughput
	throughputMBps := float64(size) / duration.Seconds() / (1024 * 1024)
	c.uploadThroughput.WithLabelValues(uploadID, transporter).Set(throughputMBps)
}

// RecordUploadError records an upload error
func (c *PrometheusCollector) RecordUploadError(ctx context.Context, uploadID, errorType, stage string) {
	c.activeUploads.Dec()
	c.uploadErrors.WithLabelValues(errorType, stage).Inc()
}

// RecordRetryAttempt records a retry attempt
func (c *PrometheusCollector) RecordRetryAttempt(stage, reason string) {
	c.retryAttempts.WithLabelValues(stage, reason).Inc()
}

// RecordStageLatency records the latency of a pipeline stage
func (c *PrometheusCollector) RecordStageLatency(stage string, duration time.Duration) {
	c.stageLatency.WithLabelValues(stage).Observe(duration.Seconds())
}

// RecordJobMetrics records metrics for a job with trace context
func (c *PrometheusCollector) RecordJobMetrics(ctx context.Context, jobID string, size int64, duration time.Duration, retries int, transporter string) {
	// Extract trace ID if available for correlation
	traceID := tracing.TraceID(ctx)
	_ = traceID // Available for future use

	// Record size
	c.uploadSize.WithLabelValues("STANDARD", transporter).Observe(float64(size))

	// Record duration
	status := "success"
	if retries > 0 {
		status = "success_with_retries"
	}
	c.uploadDuration.WithLabelValues("STANDARD", transporter, status).Observe(duration.Seconds())
}

// ServeMetrics starts an HTTP server to expose Prometheus metrics
// The server listens on the specified address and exposes metrics at /metrics
func (c *PrometheusCollector) ServeMetrics(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{}))

	c.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return c.server.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics HTTP server
func (c *PrometheusCollector) Shutdown(ctx context.Context) error {
	if c.server == nil {
		return nil
	}
	return c.server.Shutdown(ctx)
}

// GetMetricsURL returns the URL for the metrics endpoint
func (c *PrometheusCollector) GetMetricsURL() string {
	if c.server == nil {
		return ""
	}
	return fmt.Sprintf("http://%s/metrics", c.server.Addr)
}
