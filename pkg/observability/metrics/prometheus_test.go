package metrics

import (
	"context"
	"testing"
	"time"
)

func TestNewPrometheusCollector(t *testing.T) {
	collector := NewPrometheusCollector()

	if collector == nil {
		t.Fatal("Expected non-nil collector")
	}

	if collector.uploadDuration == nil {
		t.Error("uploadDuration should be initialized")
	}

	if collector.uploadSize == nil {
		t.Error("uploadSize should be initialized")
	}

	if collector.uploadThroughput == nil {
		t.Error("uploadThroughput should be initialized")
	}

	if collector.uploadErrors == nil {
		t.Error("uploadErrors should be initialized")
	}

	if collector.activeUploads == nil {
		t.Error("activeUploads should be initialized")
	}

	if collector.retryAttempts == nil {
		t.Error("retryAttempts should be initialized")
	}

	if collector.stageLatency == nil {
		t.Error("stageLatency should be initialized")
	}
}

func TestRecordUploadStart(t *testing.T) {
	collector := NewPrometheusCollector()
	collector.RecordUploadStart()
	// Metric should be incremented (no panic)
}

func TestRecordUploadComplete(t *testing.T) {
	collector := NewPrometheusCollector()
	ctx := context.Background()

	collector.RecordUploadStart()
	collector.RecordUploadComplete(
		ctx,
		"test-upload-id",
		1024*1024*100, // 100 MB
		time.Minute,
		"STANDARD",
		"basic",
	)
	// Metrics should be recorded (no panic)
}

func TestRecordUploadError(t *testing.T) {
	collector := NewPrometheusCollector()
	ctx := context.Background()

	collector.RecordUploadStart()
	collector.RecordUploadError(ctx, "test-upload-id", "s3_timeout", "uploader")
	// Error metric should be incremented (no panic)
}

func TestRecordRetryAttempt(t *testing.T) {
	collector := NewPrometheusCollector()
	collector.RecordRetryAttempt("uploader", "network_error")
	// Retry metric should be incremented (no panic)
}

func TestRecordStageLatency(t *testing.T) {
	collector := NewPrometheusCollector()
	collector.RecordStageLatency("scanner", time.Second*5)
	collector.RecordStageLatency("archiver", time.Second*10)
	collector.RecordStageLatency("uploader", time.Second*30)
	// Stage latency metrics should be recorded (no panic)
}

func TestRecordJobMetrics(t *testing.T) {
	collector := NewPrometheusCollector()
	ctx := context.Background()

	collector.RecordJobMetrics(
		ctx,
		"job-123",
		1024*1024*50, // 50 MB
		time.Second*30,
		0, // no retries
		"basic",
	)

	// With retries
	collector.RecordJobMetrics(
		ctx,
		"job-456",
		1024*1024*75, // 75 MB
		time.Second*45,
		2, // 2 retries
		"optimized",
	)
	// Job metrics should be recorded (no panic)
}

func TestGetMetricsURL(t *testing.T) {
	collector := NewPrometheusCollector()

	// Before server starts, should return empty
	url := collector.GetMetricsURL()
	if url != "" {
		t.Errorf("Expected empty URL before server start, got %s", url)
	}
}

func TestShutdown(t *testing.T) {
	collector := NewPrometheusCollector()
	ctx := context.Background()

	// Shutdown without starting server should not error
	err := collector.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown without server should not error: %v", err)
	}
}
