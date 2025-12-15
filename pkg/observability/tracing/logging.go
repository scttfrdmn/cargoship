// Package tracing provides utilities for enriching logs with distributed tracing context.
//
// This enables correlation between log messages and distributed traces by automatically
// adding trace_id and span_id to structured logs.
//
// # Usage Examples
//
// Basic trace attribute extraction:
//
//	attrs := tracing.TraceAttrs(ctx)
//	logger.Info("processing upload", attrs...)
//
// Enrich logger with trace context:
//
//	enrichedLogger := tracing.EnrichLogger(ctx, logger)
//	enrichedLogger.Info("processing upload", "job_id", jobID)
//	// Output: {"level":"INFO","msg":"processing upload","trace_id":"abc123...","span_id":"def456...","job_id":42}
//
// Add trace context to specific log calls:
//
//	logger.Info("upload complete",
//	    tracing.WithTraceContext(ctx,
//	        slog.Int("job_id", jobID),
//	        slog.String("s3_key", key),
//	    )...)
//
// Convenience functions for common log levels:
//
//	tracing.InfoWithTrace(ctx, logger, "upload started", slog.Int("job_id", jobID))
//	tracing.ErrorWithTrace(ctx, logger, "upload failed", slog.String("error", err.Error()))
//
// # Integration with OpenTelemetry
//
// This package works seamlessly with OpenTelemetry distributed tracing. When a span
// is active in the context, trace IDs are automatically extracted and added to logs.
// When no span exists, the functions degrade gracefully and return the original logger
// or attributes unchanged.
package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceAttrs extracts trace ID and span ID from context as slog attributes
// Returns empty attributes if no active span exists
func TraceAttrs(ctx context.Context) []slog.Attr {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return nil
	}

	spanCtx := span.SpanContext()
	return []slog.Attr{
		slog.String("trace_id", spanCtx.TraceID().String()),
		slog.String("span_id", spanCtx.SpanID().String()),
	}
}

// TraceID extracts just the trace ID from context
// Returns empty string if no active span exists
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// SpanID extracts just the span ID from context
// Returns empty string if no active span exists
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// EnrichLogger returns a new logger with trace context attributes
// If no active span exists, returns the original logger unchanged
func EnrichLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	attrs := TraceAttrs(ctx)
	if len(attrs) == 0 {
		return logger
	}

	// Create new logger with trace attributes
	return logger.With(
		slog.String("trace_id", attrs[0].Value.String()),
		slog.String("span_id", attrs[1].Value.String()),
	)
}

// WithTraceContext is a convenience function that adds trace attributes to a slog.Attr slice
// This is useful for adding trace context to specific log calls without creating a new logger
func WithTraceContext(ctx context.Context, attrs ...slog.Attr) []slog.Attr {
	traceAttrs := TraceAttrs(ctx)
	if len(traceAttrs) == 0 {
		return attrs
	}

	// Prepend trace attributes
	result := make([]slog.Attr, 0, len(attrs)+len(traceAttrs))
	result = append(result, traceAttrs...)
	result = append(result, attrs...)
	return result
}

// LogWithTrace is a helper that logs a message with trace context automatically added
func LogWithTrace(ctx context.Context, logger *slog.Logger, level slog.Level, msg string, attrs ...slog.Attr) {
	enrichedAttrs := WithTraceContext(ctx, attrs...)
	logger.LogAttrs(ctx, level, msg, enrichedAttrs...)
}

// InfoWithTrace logs an info message with trace context
func InfoWithTrace(ctx context.Context, logger *slog.Logger, msg string, attrs ...slog.Attr) {
	LogWithTrace(ctx, logger, slog.LevelInfo, msg, attrs...)
}

// ErrorWithTrace logs an error message with trace context
func ErrorWithTrace(ctx context.Context, logger *slog.Logger, msg string, attrs ...slog.Attr) {
	LogWithTrace(ctx, logger, slog.LevelError, msg, attrs...)
}

// WarnWithTrace logs a warning message with trace context
func WarnWithTrace(ctx context.Context, logger *slog.Logger, msg string, attrs ...slog.Attr) {
	LogWithTrace(ctx, logger, slog.LevelWarn, msg, attrs...)
}

// DebugWithTrace logs a debug message with trace context
func DebugWithTrace(ctx context.Context, logger *slog.Logger, msg string, attrs ...slog.Attr) {
	LogWithTrace(ctx, logger, slog.LevelDebug, msg, attrs...)
}
