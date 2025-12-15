// Package tracing provides integration tests for distributed tracing
package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestEndToEndTracing tests the complete tracing flow from provider setup to span creation
func TestEndToEndTracing(t *testing.T) {
	ctx := context.Background()

	// Create tracer provider with stdout exporter
	config := Config{
		Enabled:        true,
		ExporterType:   "stdout",
		ServiceName:    "cargoship-test",
		ServiceVersion: "v0.6.2",
		SampleRate:     1.0,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create tracer provider: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected non-nil tracer provider")
	}

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	// Create a test span hierarchy similar to CargoShip pipeline
	tracer := otel.Tracer("test")

	// Root upload span
	ctx, uploadSpan := tracer.Start(ctx, "upload-request")
	defer uploadSpan.End()

	// Pipeline execution span
	ctx, pipelineSpan := tracer.Start(ctx, "pipeline-execution")

	// Scanner stage span
	ctx, scannerSpan := tracer.Start(ctx, "scanner-stage")
	scannerSpan.End()

	// Archiver stage span
	ctx, archiverSpan := tracer.Start(ctx, "archiver-stage")
	archiverSpan.End()

	// Uploader stage span
	ctx, uploaderSpan := tracer.Start(ctx, "uploader-stage")

	// Job span within uploader
	_, jobSpan := tracer.Start(ctx, "job-1")
	jobSpan.End()

	uploaderSpan.End()
	pipelineSpan.End()

	// Shutdown tracer provider
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tp.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown tracer provider: %v", err)
	}
}

// TestTraceContextPropagation verifies that trace context propagates through nested spans
func TestTraceContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Create in-memory exporter for verification
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("test")

	// Create parent span
	ctx, parentSpan := tracer.Start(ctx, "parent")
	parentTraceID := parentSpan.SpanContext().TraceID().String()
	parentSpanID := parentSpan.SpanContext().SpanID().String()

	// Create child span
	ctx, childSpan := tracer.Start(ctx, "child")
	childTraceID := childSpan.SpanContext().TraceID().String()
	childSpanID := childSpan.SpanContext().SpanID().String()

	childSpan.End()
	parentSpan.End()

	// Force flush
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Verify trace IDs match (same trace)
	if parentTraceID != childTraceID {
		t.Errorf("Trace IDs don't match: parent=%s, child=%s", parentTraceID, childTraceID)
	}

	// Verify span IDs are different (different spans)
	if parentSpanID == childSpanID {
		t.Errorf("Span IDs should be different: parent=%s, child=%s", parentSpanID, childSpanID)
	}

	// Verify both spans were exported
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(spans))
	}
}

// TestLogEnrichmentWithTracing verifies that trace context is available for log enrichment
func TestLogEnrichmentWithTracing(t *testing.T) {
	ctx := context.Background()

	// Create tracer provider
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("test")

	// Create span
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	// Extract trace attributes for logging
	attrs := TraceAttrs(ctx)

	// Verify attributes exist
	if len(attrs) != 2 {
		t.Fatalf("Expected 2 trace attributes, got %d", len(attrs))
	}

	// Verify trace_id attribute
	if attrs[0].Key != "trace_id" {
		t.Errorf("Expected first attribute to be trace_id, got %s", attrs[0].Key)
	}
	traceID := attrs[0].Value.String()
	if traceID == "" {
		t.Error("Trace ID should not be empty")
	}

	// Verify span_id attribute
	if attrs[1].Key != "span_id" {
		t.Errorf("Expected second attribute to be span_id, got %s", attrs[1].Key)
	}
	spanID := attrs[1].Value.String()
	if spanID == "" {
		t.Error("Span ID should not be empty")
	}

	// Verify extracted IDs match span context
	expectedTraceID := span.SpanContext().TraceID().String()
	expectedSpanID := span.SpanContext().SpanID().String()

	if traceID != expectedTraceID {
		t.Errorf("Trace ID mismatch: extracted=%s, expected=%s", traceID, expectedTraceID)
	}

	if spanID != expectedSpanID {
		t.Errorf("Span ID mismatch: extracted=%s, expected=%s", spanID, expectedSpanID)
	}
}

// TestPipelineTracerIntegration tests the PipelineTracer with realistic scenarios
func TestPipelineTracerIntegration(t *testing.T) {
	ctx := context.Background()

	// Create tracer provider
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	// Create pipeline tracer
	pipelineTracer := NewPipelineTracer()

	// Simulate pipeline execution
	uploadCtx, uploadSpan := pipelineTracer.StartUploadSpan(ctx, "test-upload-123")
	defer uploadSpan.End()

	// Simulate stage execution
	stageCtx, stageSpan := pipelineTracer.StartStageSpan(uploadCtx, "scanner")
	pipelineTracer.AddFileAttributes(stageSpan, "/test/path", 1024*1024*100, 42)
	pipelineTracer.RecordSuccess(stageSpan)
	stageSpan.End()

	// Simulate job execution
	_, jobSpan := pipelineTracer.StartJobSpan(stageCtx, 1, 0)
	pipelineTracer.AddS3Attributes(jobSpan, "test-bucket", "test-key", "us-west-2")
	pipelineTracer.RecordSuccess(jobSpan)
	jobSpan.End()

	// Force flush
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Verify spans were created
	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Errorf("Expected at least 2 spans, got %d", len(spans))
	}
	// Note: Upload span may not be exported yet since it's still active

	// Verify all spans share the same trace ID
	if len(spans) >= 2 {
		traceID1 := spans[0].SpanContext.TraceID().String()
		traceID2 := spans[1].SpanContext.TraceID().String()
		if traceID1 != traceID2 {
			t.Errorf("Trace IDs should match across spans: %s != %s", traceID1, traceID2)
		}
	}
}

// TestS3TracerIntegration tests the S3Tracer with realistic upload scenarios
func TestS3TracerIntegration(t *testing.T) {
	ctx := context.Background()

	// Create tracer provider
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	// Create S3 tracer
	s3Tracer := NewS3Tracer()

	// Simulate S3 upload operation
	_, uploadSpan := s3Tracer.StartUploadSpan(ctx, "test-bucket", "test-key.tar.zst", 1024*1024*50)
	s3Tracer.AddStorageClass(uploadSpan, "STANDARD")
	s3Tracer.AddTransporterInfo(uploadSpan, "optimized", 4)
	s3Tracer.AddUploadMetrics(uploadSpan, 1024*1024*50, 5000, 10.5)
	s3Tracer.RecordSuccess(uploadSpan)
	uploadSpan.End()

	// Force flush
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Verify span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Errorf("Expected 1 span, got %d", len(spans))
	}

	// Verify span name
	if len(spans) > 0 && spans[0].Name != "s3.upload" {
		t.Errorf("Expected span name 's3.upload', got '%s'", spans[0].Name)
	}
}

// TestDisabledTracing verifies that tracing can be disabled
func TestDisabledTracing(t *testing.T) {
	ctx := context.Background()

	// Create disabled tracer provider
	config := Config{
		Enabled:      false,
		ExporterType: "stdout",
		ServiceName:  "cargoship-test",
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create tracer provider: %v", err)
	}

	// Should return nil when disabled
	if tp != nil {
		t.Error("Expected nil tracer provider when tracing is disabled")
	}
}

// TestNoneExporter verifies that "none" exporter returns nil
func TestNoneExporter(t *testing.T) {
	ctx := context.Background()

	// Create tracer provider with "none" exporter
	config := Config{
		Enabled:      true,
		ExporterType: "none",
		ServiceName:  "cargoship-test",
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create tracer provider: %v", err)
	}

	// Should return nil with "none" exporter
	if tp != nil {
		t.Error("Expected nil tracer provider with 'none' exporter")
	}
}
