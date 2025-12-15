package tracing

import (
	"context"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceAttrs_NoSpan(t *testing.T) {
	ctx := context.Background()
	attrs := TraceAttrs(ctx)

	if attrs != nil {
		t.Errorf("Expected nil attributes for context without span, got %v", attrs)
	}
}

func TestTraceAttrs_WithSpan(t *testing.T) {
	// Create a test tracer
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	attrs := TraceAttrs(ctx)

	if len(attrs) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != "trace_id" {
		t.Errorf("Expected first attribute key to be 'trace_id', got '%s'", attrs[0].Key)
	}

	if attrs[1].Key != "span_id" {
		t.Errorf("Expected second attribute key to be 'span_id', got '%s'", attrs[1].Key)
	}

	// Verify values are non-empty
	traceID := attrs[0].Value.String()
	spanID := attrs[1].Value.String()

	if traceID == "" {
		t.Error("trace_id should not be empty")
	}

	if spanID == "" {
		t.Error("span_id should not be empty")
	}
}

func TestTraceID(t *testing.T) {
	// No span
	ctx := context.Background()
	if id := TraceID(ctx); id != "" {
		t.Errorf("Expected empty trace ID for context without span, got '%s'", id)
	}

	// With span
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if id := TraceID(ctx); id == "" {
		t.Error("Expected non-empty trace ID")
	}
}

func TestSpanID(t *testing.T) {
	// No span
	ctx := context.Background()
	if id := SpanID(ctx); id != "" {
		t.Errorf("Expected empty span ID for context without span, got '%s'", id)
	}

	// With span
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if id := SpanID(ctx); id == "" {
		t.Error("Expected non-empty span ID")
	}
}

func TestEnrichLogger_NoSpan(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	enriched := EnrichLogger(ctx, logger)

	// Should return the same logger when no span
	if enriched != logger {
		t.Error("Expected same logger instance when no span exists")
	}
}

func TestEnrichLogger_WithSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger := slog.Default()
	enriched := EnrichLogger(ctx, logger)

	// Should return a different logger instance with trace context
	if enriched == logger {
		t.Error("Expected different logger instance when span exists")
	}
}

func TestWithTraceContext_NoSpan(t *testing.T) {
	ctx := context.Background()
	attrs := []slog.Attr{
		slog.String("key", "value"),
	}

	result := WithTraceContext(ctx, attrs...)

	if len(result) != len(attrs) {
		t.Errorf("Expected %d attributes, got %d", len(attrs), len(result))
	}
}

func TestWithTraceContext_WithSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	attrs := []slog.Attr{
		slog.String("key", "value"),
	}

	result := WithTraceContext(ctx, attrs...)

	// Should have trace_id, span_id, plus original attribute
	expectedLen := 3
	if len(result) != expectedLen {
		t.Errorf("Expected %d attributes (2 trace + 1 original), got %d", expectedLen, len(result))
	}

	// First two should be trace attributes
	if result[0].Key != "trace_id" {
		t.Errorf("Expected first attribute to be trace_id, got %s", result[0].Key)
	}

	if result[1].Key != "span_id" {
		t.Errorf("Expected second attribute to be span_id, got %s", result[1].Key)
	}

	// Last should be original attribute
	if result[2].Key != "key" {
		t.Errorf("Expected third attribute to be 'key', got %s", result[2].Key)
	}
}
