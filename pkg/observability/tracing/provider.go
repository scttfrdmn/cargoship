// Package tracing provides OpenTelemetry distributed tracing for CargoShip
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/scttfrdmn/cargoship/internal/version"
)

// Config holds configuration for OpenTelemetry tracing
type Config struct {
	ServiceName string // Name of the service (e.g., "cargoship")

	// ServiceVersion labels every emitted span. Empty means "use the binary's
	// own version" (#318) — a hardcoded literal here silently mislabels traces
	// for every release after the one it was written in.
	ServiceVersion string
	ExporterType   string  // "stdout", "otlp", or "none"
	Endpoint       string  // Collector endpoint for OTLP (e.g., "localhost:4317")
	SampleRate     float64 // Sampling rate (0.0 to 1.0, default 1.0 = 100%)
	Enabled        bool    // Whether tracing is enabled
}

// NewTracerProvider creates and configures an OpenTelemetry tracer provider
func NewTracerProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	if !cfg.Enabled {
		return nil, nil // No-op when disabled
	}

	// Default sample rate to 100% if not specified
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 1.0
	}

	// #318: fall back to the single version source rather than emitting an
	// unlabeled service.version attribute.
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "v" + version.Version
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on type
	var exporter sdktrace.SpanExporter
	switch cfg.ExporterType {
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}

	case "otlp":
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("OTLP endpoint is required")
		}
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
			otlptracegrpc.WithInsecure(), // Use TLS in production
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}

	case "none":
		// No exporter - tracing is effectively disabled
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown exporter type: %s (supported: stdout, otlp)", cfg.ExporterType)
	}

	// Create tracer provider with batch span processor and sampling
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	return tp, nil
}
