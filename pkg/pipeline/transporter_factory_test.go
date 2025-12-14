package pipeline

import (
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// mockS3ClientForTransporter creates a mock S3 client for transporter testing
func mockS3ClientForTransporter() *s3.Client {
	// Return a nil client - the factory doesn't actually use it in tests
	// In real usage, this would be a properly configured S3 client
	return &s3.Client{}
}

func TestNewPipelineTransporter_Basic(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterBasic,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 64 * 1024 * 1024,
			Concurrency:        4,
		},
		EnableOptimization: false,
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create basic transporter: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}

	// Verify transporter has correct config
	transporterConfig := transporter.GetConfig()
	if transporterConfig.Bucket != "test-bucket" {
		t.Errorf("expected bucket test-bucket, got %s", transporterConfig.Bucket)
	}
}

func TestNewPipelineTransporter_Staging(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterStaging,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 64 * 1024 * 1024,
			Concurrency:        4,
		},
		EnableOptimization: false,
		DisableStaging:     false,
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create staging transporter: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}

func TestNewPipelineTransporter_StagingWithOptimization(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterStaging,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 64 * 1024 * 1024,
			Concurrency:        4,
		},
		EnableOptimization: true,
		CongestionControl:  "auto",
		DisableStaging:     false,
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create staging transporter with optimization: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}

func TestNewPipelineTransporter_Adaptive(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterAdaptive,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 64 * 1024 * 1024,
			Concurrency:        4,
		},
		EnableOptimization: true,
		CongestionControl:  "bbr",
		DisableStaging:     false,
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create adaptive transporter: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}

func TestNewPipelineTransporter_Optimized(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterOptimized,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 64 * 1024 * 1024,
			Concurrency:        4,
		},
		EnableOptimization: true,
		CongestionControl:  "cubic",
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create optimized transporter: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}

func TestNewPipelineTransporter_InvalidType(t *testing.T) {
	config := TransporterConfig{
		Type:     "invalid",
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket: "test-bucket",
		},
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	_, err := NewPipelineTransporter(config)
	if err == nil {
		t.Fatal("expected error for invalid transporter type")
	}
}

func TestNewPipelineTransporter_MissingS3Client(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterBasic,
		S3Client: nil, // Missing S3 client
		S3Config: awsconfig.S3Config{
			Bucket: "test-bucket",
		},
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	_, err := NewPipelineTransporter(config)
	if err == nil {
		t.Fatal("expected error for missing S3 client")
	}
}

func TestNewPipelineTransporter_MissingBucket(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterBasic,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket: "", // Missing bucket
		},
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	_, err := NewPipelineTransporter(config)
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestNewPipelineTransporter_DefaultChunkSize(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterBasic,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket:             "test-bucket",
			MultipartChunkSize: 0, // Should use default
			Concurrency:        0, // Should use default
		},
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create transporter with defaults: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}

	// Verify defaults were applied
	transporterConfig := transporter.GetConfig()
	expectedChunkSize := int64(64 * 1024 * 1024) // 64MB
	if transporterConfig.MultipartChunkSize != expectedChunkSize {
		t.Errorf("expected default chunk size %d, got %d", expectedChunkSize, transporterConfig.MultipartChunkSize)
	}

	expectedConcurrency := 4
	if transporterConfig.Concurrency != expectedConcurrency {
		t.Errorf("expected default concurrency %d, got %d", expectedConcurrency, transporterConfig.Concurrency)
	}
}

func TestNewPipelineTransporter_CongestionControlOptions(t *testing.T) {
	tests := []struct {
		name              string
		congestionControl string
		expectError       bool
	}{
		{"BBR", "bbr", false},
		{"CUBIC", "cubic", false},
		{"Auto", "auto", false},
		{"Empty (defaults to auto)", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TransporterConfig{
				Type:     TransporterOptimized,
				S3Client: mockS3ClientForTransporter(),
				S3Config: awsconfig.S3Config{
					Bucket: "test-bucket",
				},
				EnableOptimization: true,
				CongestionControl:  tt.congestionControl,
				Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			}

			transporter, err := NewPipelineTransporter(config)
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if transporter == nil {
				t.Fatal("expected non-nil transporter")
			}
		})
	}
}

func TestNewPipelineTransporter_DisableStaging(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterStaging,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket: "test-bucket",
		},
		EnableOptimization: false,
		DisableStaging:     true, // Disable staging
		Logger:             slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create transporter with staging disabled: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}

func TestNewPipelineTransporter_DefaultLogger(t *testing.T) {
	config := TransporterConfig{
		Type:     TransporterBasic,
		S3Client: mockS3ClientForTransporter(),
		S3Config: awsconfig.S3Config{
			Bucket: "test-bucket",
		},
		Logger: nil, // Should use default logger
	}

	transporter, err := NewPipelineTransporter(config)
	if err != nil {
		t.Fatalf("failed to create transporter with default logger: %v", err)
	}

	if transporter == nil {
		t.Fatal("expected non-nil transporter")
	}
}
