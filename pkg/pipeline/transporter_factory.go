package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// transporterAdapter adapts transporters to the BasicTransporter interface
// Handles both value and pointer Archive types
type transporterAdapter struct {
	uploadFunc func(ctx context.Context, archive s3transport.Archive) (*s3transport.UploadResult, error)
	config     awsconfig.S3Config
}

func (a *transporterAdapter) Upload(ctx context.Context, archive s3transport.Archive) (*s3transport.UploadResult, error) {
	return a.uploadFunc(ctx, archive)
}

func (a *transporterAdapter) GetConfig() awsconfig.S3Config {
	return a.config
}

// wrapTransporter creates an adapter for transporters that take *Archive (pointer)
func wrapPointerTransporter(t interface {
	Upload(ctx context.Context, archive *s3transport.Archive) (*s3transport.UploadResult, error)
}, config awsconfig.S3Config) *transporterAdapter {
	return &transporterAdapter{
		uploadFunc: func(ctx context.Context, archive s3transport.Archive) (*s3transport.UploadResult, error) {
			return t.Upload(ctx, &archive)
		},
		config: config,
	}
}

// wrapTransporter creates an adapter for transporters that take Archive (value)
func wrapValueTransporter(t interface {
	Upload(ctx context.Context, archive s3transport.Archive) (*s3transport.UploadResult, error)
}, config awsconfig.S3Config) *transporterAdapter {
	return &transporterAdapter{
		uploadFunc: t.Upload,
		config:     config,
	}
}

// TransporterType defines the type of S3 transporter to use
type TransporterType string

const (
	// TransporterBasic uses simple AWS SDK multipart upload
	TransporterBasic TransporterType = "basic"

	// TransporterStaging uses staging transporter with predictive staging
	// and content-aware chunking (DEFAULT, recommended for most use cases)
	TransporterStaging TransporterType = "staging"

	// TransporterAdaptive uses adaptive transporter with real-time
	// network adaptation and parameter tuning
	TransporterAdaptive TransporterType = "adaptive"

	// TransporterOptimized uses optimized transporter with BBR/CUBIC
	// congestion control and S3-specific optimizations
	TransporterOptimized TransporterType = "optimized"
)

// TransporterConfig configures the creation of a pipeline transporter
type TransporterConfig struct {
	// Type specifies which transporter implementation to use
	Type TransporterType

	// S3Client is the AWS S3 client
	S3Client *s3.Client

	// S3Config contains S3-specific configuration
	S3Config awsconfig.S3Config

	// EnableOptimization enables BBR/CUBIC congestion control,
	// adaptive staging, BDP optimization, and other advanced features
	EnableOptimization bool

	// CongestionControl specifies the congestion control algorithm
	// Valid values: "bbr", "cubic", "auto" (auto selects BBR with CUBIC fallback)
	CongestionControl string

	// DisableStaging disables adaptive staging (reduces memory usage)
	// Only applies to staging and adaptive transporters
	DisableStaging bool

	// Logger for transporter operations (optional, uses default if nil)
	Logger *slog.Logger
}

// NewPipelineTransporter creates a new S3 transporter based on the provided configuration
//
// The factory handles:
// - Creating the appropriate transporter type (basic, staging, adaptive, optimized)
// - Configuring optimization features (BBR/CUBIC, staging, BDP)
// - Graceful degradation if optional components are unavailable
// - Returning a BasicTransporter interface that works with all implementations
//
// Example usage:
//
//	config := TransporterConfig{
//	    Type:               TransporterStaging,
//	    S3Client:           s3Client,
//	    S3Config:           s3cfg,
//	    EnableOptimization: true,
//	    CongestionControl:  "auto",
//	}
//	transporter, err := NewPipelineTransporter(config)
func NewPipelineTransporter(config TransporterConfig) (s3transport.BasicTransporter, error) {
	// Validate configuration
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client is required")
	}

	// Set default logger if not provided
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create base S3 config with pipeline-appropriate defaults
	s3Config := config.S3Config
	if s3Config.Bucket == "" {
		return nil, fmt.Errorf("S3Config.Bucket is required")
	}

	// Set sensible defaults for multipart upload if not configured
	if s3Config.MultipartChunkSize == 0 {
		s3Config.MultipartChunkSize = 64 * 1024 * 1024 // 64MB (matches pipeline default)
	}
	if s3Config.Concurrency == 0 {
		s3Config.Concurrency = 4 // Conservative default
	}

	// Create transporter based on type
	switch config.Type {
	case TransporterBasic:
		return createBasicTransporter(config.S3Client, s3Config, logger)

	case TransporterStaging:
		return createStagingTransporter(config, s3Config, logger)

	case TransporterAdaptive:
		return createAdaptiveTransporter(config, s3Config, logger)

	case TransporterOptimized:
		return createOptimizedTransporter(config, s3Config, logger)

	default:
		return nil, fmt.Errorf("unknown transporter type: %s (valid: basic, staging, adaptive, optimized)", config.Type)
	}
}

// createBasicTransporter creates a basic S3 transporter using AWS SDK uploader
func createBasicTransporter(s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) (s3transport.BasicTransporter, error) {
	transporter := s3transport.NewTransporter(s3Client, s3Config)
	logger.Info("created basic S3 transporter",
		"bucket", s3Config.Bucket,
		"chunk_size", s3Config.MultipartChunkSize,
		"concurrency", s3Config.Concurrency)
	return transporter, nil
}

// createStagingTransporter creates a staging transporter with predictive staging
func createStagingTransporter(config TransporterConfig, s3Config awsconfig.S3Config, logger *slog.Logger) (s3transport.BasicTransporter, error) {
	// Create staging configuration
	stagingConfig := &s3transport.StagingConfig{
		EnableStaging:       !config.DisableStaging,
		EnableNetworkAdapt:  config.EnableOptimization,
		EnableOptimization:  config.EnableOptimization,
		StageAheadChunks:    3,   // Default: stage 3 chunks ahead
		MaxStagingMemoryMB:  256, // Default: 256MB staging buffer
		NetworkMonitoringHz: 1.0, // Monitor network once per second
		OptimizationConfig:  nil, // Will be set if optimization enabled
	}

	// Configure S3 optimization if enabled
	if config.EnableOptimization {
		stagingConfig.OptimizationConfig = createOptimizationConfig(config.CongestionControl)
	}

	// Create staging transporter with context
	ctx := context.Background()
	transporter, err := s3transport.NewStagingTransporter(
		ctx,
		config.S3Client,
		s3Config,
		stagingConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging transporter: %w", err)
	}

	logger.Info("created staging S3 transporter",
		"bucket", s3Config.Bucket,
		"staging_enabled", stagingConfig.EnableStaging,
		"optimization_enabled", config.EnableOptimization,
		"stage_ahead_chunks", stagingConfig.StageAheadChunks)

	// Wrap in interface adapter since StagingTransporter doesn't implement GetConfig yet
	return wrapValueTransporter(transporter, s3Config), nil
}

// createAdaptiveTransporter creates an adaptive transporter with real-time network adaptation
func createAdaptiveTransporter(config TransporterConfig, s3Config awsconfig.S3Config, logger *slog.Logger) (s3transport.BasicTransporter, error) {
	// Create base staging configuration
	stagingConfig := &s3transport.StagingConfig{
		EnableStaging:       !config.DisableStaging,
		EnableNetworkAdapt:  true, // Always enable for adaptive
		EnableOptimization:  config.EnableOptimization,
		StageAheadChunks:    3,
		MaxStagingMemoryMB:  256,
		NetworkMonitoringHz: 2.0, // Higher frequency for adaptive
		OptimizationConfig:  nil,
	}

	if config.EnableOptimization {
		stagingConfig.OptimizationConfig = createOptimizationConfig(config.CongestionControl)
	}

	// Create adaptive configuration with real-time adaptation
	adaptiveConfig := &s3transport.AdaptiveTransporterConfig{
		StagingConfig:            stagingConfig,
		EnableRealTimeAdaptation: true,
		AdaptationSensitivity:    0.8, // High sensitivity to network changes
		MinAdaptationInterval:    10,  // 10 seconds minimum between adaptations
		MaxAdaptationsPerSession: 10,  // Max 10 adaptations per upload session
	}

	// Create adaptive transporter with context
	ctx := context.Background()
	transporter, err := s3transport.NewAdaptiveTransporter(
		ctx,
		config.S3Client,
		s3Config,
		adaptiveConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create adaptive transporter: %w", err)
	}

	logger.Info("created adaptive S3 transporter",
		"bucket", s3Config.Bucket,
		"staging_enabled", stagingConfig.EnableStaging,
		"optimization_enabled", config.EnableOptimization,
		"real_time_adaptation", adaptiveConfig.EnableRealTimeAdaptation)

	// Wrap in interface adapter since AdaptiveTransporter doesn't implement GetConfig yet
	return wrapValueTransporter(transporter, s3Config), nil
}

// createOptimizedTransporter creates an optimized transporter with BBR/CUBIC congestion control
func createOptimizedTransporter(config TransporterConfig, s3Config awsconfig.S3Config, logger *slog.Logger) (s3transport.BasicTransporter, error) {
	// Create optimized transporter with context (it creates the optimizer internally)
	ctx := context.Background()
	transporter, err := s3transport.NewOptimizedTransporter(
		ctx,
		config.S3Client,
		s3Config,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create optimized transporter: %w", err)
	}

	logger.Info("created optimized S3 transporter",
		"bucket", s3Config.Bucket,
		"congestion_control", config.CongestionControl,
		"bbr_enabled", true,
		"cubic_enabled", true)

	// Wrap in interface adapter since OptimizedTransporter takes *Archive (pointer)
	return wrapPointerTransporter(transporter, s3Config), nil
}

// createOptimizationConfig creates an S3 optimization configuration
func createOptimizationConfig(congestionControl string) *s3optimization.Config {
	config := &s3optimization.Config{
		EnableBBR:          false,
		EnableCUBIC:        false,
		PredictiveMode:     true,             // Always enable predictive mode
		AdaptationInterval: 5 * time.Second,  // 5 seconds
		NetworkAdaptation:  true,             // Enable network adaptation
		BufferSize:         64 * 1024 * 1024, // 64MB
		MetricsEnabled:     true,
		LearningRate:       0.1,
	}

	// Configure congestion control based on selection
	switch congestionControl {
	case "bbr":
		config.EnableBBR = true
	case "cubic":
		config.EnableCUBIC = true
	case "auto", "":
		// Auto: prefer BBR, fallback to CUBIC
		config.EnableBBR = true
		config.EnableCUBIC = true // Both enabled = BBR preferred with CUBIC fallback
	}

	return config
}
