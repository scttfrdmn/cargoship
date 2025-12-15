package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	s3TracerName = "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// S3Tracer provides helpers for creating S3 operation-related spans
type S3Tracer struct {
	tracer trace.Tracer
}

// NewS3Tracer creates a new S3 operation tracer
func NewS3Tracer() *S3Tracer {
	return &S3Tracer{
		tracer: otel.Tracer(s3TracerName),
	}
}

// StartS3OperationSpan creates a span for an S3 API operation
// Returns the span and a context with the span attached
func (t *S3Tracer) StartS3OperationSpan(ctx context.Context, operation string, bucket string, key string) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, fmt.Sprintf("s3.%s", operation),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", operation),
			AttrS3Bucket.String(bucket),
			AttrS3Key.String(key),
		),
	)
	return ctx, span
}

// StartUploadSpan creates a span for a complete upload operation (may involve multiple API calls)
func (t *S3Tracer) StartUploadSpan(ctx context.Context, bucket string, key string, size int64) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "s3.upload",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "Upload"),
			AttrS3Bucket.String(bucket),
			AttrS3Key.String(key),
			AttrFileSize.Int64(size),
		),
	)
	return ctx, span
}

// StartMultipartUploadSpan creates a span for a multipart upload operation
func (t *S3Tracer) StartMultipartUploadSpan(ctx context.Context, bucket string, key string, parts int) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "s3.multipart_upload",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("aws.service", "s3"),
			attribute.String("aws.operation", "MultipartUpload"),
			AttrS3Bucket.String(bucket),
			AttrS3Key.String(key),
			attribute.Int("s3.multipart.parts", parts),
		),
	)
	return ctx, span
}

// RecordError records an error on a span with proper status code
func (t *S3Tracer) RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks a span as successful
func (t *S3Tracer) RecordSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "success")
}

// AddUploadMetrics adds upload performance metrics to a span
func (t *S3Tracer) AddUploadMetrics(span trace.Span, bytesTransferred int64, durationMs int64, throughputMbps float64) {
	span.SetAttributes(
		attribute.Int64("s3.bytes_transferred", bytesTransferred),
		attribute.Int64("s3.duration_ms", durationMs),
		attribute.Float64("s3.throughput_mbps", throughputMbps),
	)
}

// AddStorageClass adds S3 storage class to a span
func (t *S3Tracer) AddStorageClass(span trace.Span, storageClass string) {
	span.SetAttributes(AttrS3StorageClass.String(storageClass))
}

// AddTransporterInfo adds transporter-specific information to a span
func (t *S3Tracer) AddTransporterInfo(span trace.Span, transporterType string, concurrency int) {
	span.SetAttributes(
		AttrTransporter.String(transporterType),
		attribute.Int("s3.concurrency", concurrency),
	)
}

// NoopS3Tracer returns a tracer that does nothing (for when tracing is disabled)
func NoopS3Tracer() *S3Tracer {
	return &S3Tracer{
		tracer: otel.Tracer("noop"),
	}
}
