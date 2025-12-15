// Package tracing provides OpenTelemetry distributed tracing for CargoShip
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// PipelineTracer provides helpers for creating pipeline-related spans
type PipelineTracer struct {
	tracer trace.Tracer
}

// NewPipelineTracer creates a new pipeline tracer
func NewPipelineTracer() *PipelineTracer {
	return &PipelineTracer{
		tracer: otel.Tracer(tracerName),
	}
}

// StartUploadSpan creates a root span for an entire upload operation
// Returns the span and a context with the span attached
func (t *PipelineTracer) StartUploadSpan(ctx context.Context, uploadID string) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "upload",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			AttrUploadID.String(uploadID),
		),
	)
	return ctx, span
}

// StartStageSpan creates a span for a pipeline stage (scanner, archiver, uploader)
func (t *PipelineTracer) StartStageSpan(ctx context.Context, stageName string) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, fmt.Sprintf("pipeline.%s", stageName),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			AttrPipelineStage.String(stageName),
		),
	)
	return ctx, span
}

// StartJobSpan creates a span for processing a single job (chunk upload)
func (t *PipelineTracer) StartJobSpan(ctx context.Context, jobID int, shardID int) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "pipeline.job",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			AttrJobID.Int(jobID),
			AttrShardID.Int(shardID),
		),
	)
	return ctx, span
}

// StartRetrySpan creates a span for a retry attempt
func (t *PipelineTracer) StartRetrySpan(ctx context.Context, attempt int) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "pipeline.retry",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			AttrAttempt.Int(attempt),
		),
	)
	return ctx, span
}

// RecordError records an error on a span with proper status code
func (t *PipelineTracer) RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks a span as successful
func (t *PipelineTracer) RecordSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "success")
}

// AddFileAttributes adds file-related attributes to a span
func (t *PipelineTracer) AddFileAttributes(span trace.Span, fileName string, fileSize int64, fileCount int) {
	span.SetAttributes(
		AttrFileName.String(fileName),
		AttrFileSize.Int64(fileSize),
		AttrFileCount.Int(fileCount),
	)
}

// AddS3Attributes adds S3-related attributes to a span
func (t *PipelineTracer) AddS3Attributes(span trace.Span, bucket, key, region string) {
	span.SetAttributes(
		AttrS3Bucket.String(bucket),
		AttrS3Key.String(key),
		AttrS3Region.String(region),
	)
}

// AddNetworkAttributes adds network-related attributes to a span
func (t *PipelineTracer) AddNetworkAttributes(span trace.Span, bandwidth int64, rtt int64, transporter string) {
	span.SetAttributes(
		AttrBandwidth.Int64(bandwidth),
		AttrRTT.Int64(rtt),
		AttrTransporter.String(transporter),
	)
}

// AddChunkAttributes adds chunk-related attributes to a span
func (t *PipelineTracer) AddChunkAttributes(span trace.Span, chunkID int, uncompressedSize, compressedSize int64) {
	span.SetAttributes(
		AttrChunkID.Int(chunkID),
		AttrFileSize.Int64(uncompressedSize),
	)
	// Add compression ratio if both sizes available
	if uncompressedSize > 0 && compressedSize > 0 {
		ratio := float64(compressedSize) / float64(uncompressedSize)
		span.SetAttributes(
			AttrFileName.String(fmt.Sprintf("compression_ratio=%.2f", ratio)),
		)
	}
}

// GetTracer returns the underlying OpenTelemetry tracer
func (t *PipelineTracer) GetTracer() trace.Tracer {
	return t.tracer
}

// NoopTracer returns a tracer that does nothing (for when tracing is disabled)
func NoopTracer() *PipelineTracer {
	return &PipelineTracer{
		tracer: otel.Tracer("noop"),
	}
}
