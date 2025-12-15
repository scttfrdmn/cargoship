package tracing

import (
	"context"
	"testing"
	"time"
)

func TestNewTracerProvider_Disabled(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "test",
		Enabled:        false,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Expected no error when disabled, got: %v", err)
	}
	if tp != nil {
		t.Errorf("Expected nil TracerProvider when disabled, got: %v", tp)
	}
}

func TestNewTracerProvider_StdoutExporter(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "stdout",
		SampleRate:     1.0,
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create TracerProvider with stdout exporter: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected TracerProvider, got nil")
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown TracerProvider: %v", err)
	}
}

func TestNewTracerProvider_NoneExporter(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "none",
		SampleRate:     1.0,
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Expected no error with none exporter, got: %v", err)
	}
	if tp != nil {
		t.Errorf("Expected nil TracerProvider with 'none' exporter, got: %v", tp)
	}
}

func TestNewTracerProvider_OTLPExporterNoEndpoint(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "otlp",
		Endpoint:       "", // No endpoint specified
		SampleRate:     1.0,
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err == nil {
		t.Error("Expected error when OTLP endpoint is empty, got nil")
		if tp != nil {
			_ = tp.Shutdown(context.Background())
		}
	}
}

func TestNewTracerProvider_InvalidExporter(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "invalid",
		SampleRate:     1.0,
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err == nil {
		t.Error("Expected error for invalid exporter type, got nil")
		if tp != nil {
			_ = tp.Shutdown(context.Background())
		}
	}
}

func TestNewTracerProvider_DefaultSampleRate(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "stdout",
		SampleRate:     0, // Should default to 1.0
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create TracerProvider: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected TracerProvider, got nil")
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown TracerProvider: %v", err)
	}
}

func TestNewTracerProvider_PartialSampling(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:    "cargoship",
		ServiceVersion: "v0.6.2",
		ExporterType:   "stdout",
		SampleRate:     0.1, // 10% sampling
		Enabled:        true,
	}

	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create TracerProvider with 10%% sampling: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected TracerProvider, got nil")
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown TracerProvider: %v", err)
	}
}

func TestConfig_Defaults(t *testing.T) {
	ctx := context.Background()
	config := Config{
		ServiceName:  "cargoship",
		ExporterType: "stdout", // Required field
		Enabled:      true,
		// ServiceVersion and SampleRate will use defaults
	}

	// Should succeed with defaults
	tp, err := NewTracerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create TracerProvider with defaults: %v", err)
	}
	if tp == nil {
		t.Fatal("Expected TracerProvider, got nil")
	}

	// Cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown TracerProvider: %v", err)
	}
}
