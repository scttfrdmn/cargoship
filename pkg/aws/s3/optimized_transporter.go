// Package s3 provides CargoShip integration with the new S3 optimization modules
package s3

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// OptimizedTransporter provides backward-compatible CargoShip transport using new optimization modules
type OptimizedTransporter struct {
	optimizer *s3optimization.S3Optimizer
	config    awsconfig.S3Config
	logger    *slog.Logger
}

// NewOptimizedTransporter creates a new optimized transporter for CargoShip
func NewOptimizedTransporter(ctx context.Context, s3Client *s3.Client, config awsconfig.S3Config, logger *slog.Logger) (*OptimizedTransporter, error) {
	if s3Client == nil {
		return nil, fmt.Errorf("S3 client cannot be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Convert CargoShip config to optimization config
	optimizerConfig := &s3optimization.Config{
		EnableBBR:           true,  // Always enable BBR for performance
		EnableCUBIC:         true,  // Always enable CUBIC for performance
		NetworkAdaptation:   true,  // Enable adaptive optimization
		RTTSmoothingFactor:  0.125, // Standard TCP smoothing factor
		MaxConnections:      int(config.Concurrency),
		ConnectionPoolSize:  int(config.Concurrency),
		LoadBalancingMode:   "round_robin",
		HealthCheckInterval: 30 * time.Second,
		BufferSize:          config.MultipartChunkSize,
		CompressionLevel:    6, // Balanced compression
		PipelineDepth:       4,
		MetricsEnabled:      true,
		PredictiveMode:      true,
		AdaptationInterval:  5 * time.Second,
		LearningRate:        0.1,
	}

	// Create the S3 optimizer
	optimizer, err := s3optimization.NewS3Optimizer(ctx, s3Client, optimizerConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 optimizer: %w", err)
	}

	transporter := &OptimizedTransporter{
		optimizer: optimizer,
		config:    config,
		logger:    logger,
	}

	logger.Info("optimized transporter initialized",
		"bbr_enabled", optimizerConfig.EnableBBR,
		"cubic_enabled", optimizerConfig.EnableCUBIC,
		"adaptive_enabled", optimizerConfig.NetworkAdaptation,
		"predictive_mode", optimizerConfig.PredictiveMode,
		"concurrency", config.Concurrency)

	return transporter, nil
}

// Upload performs an optimized CargoShip archive upload
func (t *OptimizedTransporter) Upload(ctx context.Context, archive *Archive) (*UploadResult, error) {
	if archive == nil {
		return nil, fmt.Errorf("archive cannot be nil")
	}

	startTime := time.Now()

	// Convert CargoShip archive to S3 input
	input := &s3.PutObjectInput{
		Bucket:       &t.config.Bucket,
		Key:          &archive.Key,
		Body:         archive.Reader,
		StorageClass: types.StorageClass(archive.StorageClass),
		Metadata:     archive.Metadata,
	}

	// Add content length if available
	if archive.Size > 0 {
		input.ContentLength = &archive.Size
	}

	// Add KMS encryption if configured
	if t.config.KMSKeyID != "" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = &t.config.KMSKeyID
	}

	// Use optimized S3 client for upload
	result, err := t.optimizer.PutObjectOptimized(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("optimized upload failed: %w", err)
	}

	duration := time.Since(startTime)

	// Calculate throughput
	var throughput float64
	if archive.Size > 0 && duration.Seconds() > 0 {
		bytesPerSecond := float64(archive.Size) / duration.Seconds()
		throughput = bytesPerSecond / (1024 * 1024) // Convert to MB/s
	}

	// Convert result back to CargoShip format
	uploadResult := &UploadResult{
		Location:     fmt.Sprintf("s3://%s/%s", t.config.Bucket, archive.Key),
		Key:          archive.Key,
		ETag:         *result.ETag,
		Duration:     duration,
		Throughput:   throughput,
		StorageClass: types.StorageClass(archive.StorageClass),
	}

	// Note: Upload ID is not available in PutObjectOutput for single uploads
	// For multipart uploads, this would need to be tracked separately

	t.logger.Info("optimized upload completed",
		"key", archive.Key,
		"size", archive.Size,
		"duration", duration,
		"throughput_mbps", throughput,
		"storage_class", archive.StorageClass)

	return uploadResult, nil
}

// Download performs an optimized CargoShip archive download
func (t *OptimizedTransporter) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	input := &s3.GetObjectInput{
		Bucket: &t.config.Bucket,
		Key:    &key,
	}

	// Use optimized S3 client for download
	result, err := t.optimizer.GetObjectOptimized(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("optimized download failed: %w", err)
	}

	t.logger.Info("optimized download initiated", "key", key)
	return result.Body, nil
}

// DownloadRange performs an optimized range download
func (t *OptimizedTransporter) DownloadRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	input := &s3.GetObjectInput{
		Bucket: &t.config.Bucket,
		Key:    &key,
		Range:  &rangeHeader,
	}

	// Use optimized S3 client for range download
	result, err := t.optimizer.GetObjectOptimized(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("optimized range download failed: %w", err)
	}

	t.logger.Info("optimized range download initiated",
		"key", key,
		"offset", offset,
		"length", length)
	return result.Body, nil
}

// HeadObject gets object metadata using optimization
func (t *OptimizedTransporter) HeadObject(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	input := &s3.HeadObjectInput{
		Bucket: &t.config.Bucket,
		Key:    &key,
	}

	// Use optimized S3 client for head operation
	result, err := t.optimizer.HeadObjectOptimized(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("optimized head object failed: %w", err)
	}

	return result, nil
}

// DeleteObject deletes an object using optimization
func (t *OptimizedTransporter) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	input := &s3.DeleteObjectInput{
		Bucket: &t.config.Bucket,
		Key:    &key,
	}

	// Use optimized S3 client for delete operation
	_, err := t.optimizer.DeleteObjectOptimized(ctx, input)
	if err != nil {
		return fmt.Errorf("optimized delete failed: %w", err)
	}

	t.logger.Info("optimized delete completed", "key", key)
	return nil
}

// GetPerformanceMetrics returns performance metrics from the optimizer
func (t *OptimizedTransporter) GetPerformanceMetrics() *s3optimization.PerformanceMetrics {
	return t.optimizer.GetPerformanceMetrics()
}

// UpdateNetworkConditions updates network conditions for optimization
func (t *OptimizedTransporter) UpdateNetworkConditions(conditions *s3optimization.NetworkConditions) error {
	return t.optimizer.UpdateNetworkConditions(conditions)
}

// HealthCheck performs health check of the optimizer
func (t *OptimizedTransporter) HealthCheck(ctx context.Context) error {
	return t.optimizer.HealthCheck(ctx)
}

// Shutdown gracefully shuts down the optimized transporter
func (t *OptimizedTransporter) Shutdown(ctx context.Context) error {
	t.logger.Info("shutting down optimized transporter")
	return t.optimizer.Shutdown(ctx)
}

// GetOptimizationStats returns optimization effectiveness statistics
func (t *OptimizedTransporter) GetOptimizationStats() OptimizationStats {
	metrics := t.optimizer.GetPerformanceMetrics()

	return OptimizationStats{
		PerformanceImprovement: metrics.OptimizationRatio,
		BandwidthSavings:       metrics.BandwidthSavings,
		LatencyReduction:       metrics.LatencyReduction,
		BBRActivations:         metrics.BBRActivations,
		CubicAdjustments:       metrics.CubicAdjustments,
		TotalOptimizations:     metrics.BBRActivations + metrics.CubicAdjustments,
		AverageThroughput:      metrics.ThroughputMbps,
		OptimizationEnabled:    true,
	}
}

// OptimizationStats contains optimization effectiveness statistics
type OptimizationStats struct {
	PerformanceImprovement float64 `json:"performance_improvement"` // Ratio vs baseline
	BandwidthSavings       float64 `json:"bandwidth_savings"`       // Percentage saved
	LatencyReduction       float64 `json:"latency_reduction"`       // Percentage reduced
	BBRActivations         int64   `json:"bbr_activations"`         // Number of BBR activations
	CubicAdjustments       int64   `json:"cubic_adjustments"`       // Number of CUBIC adjustments
	TotalOptimizations     int64   `json:"total_optimizations"`     // Total optimization events
	AverageThroughput      float64 `json:"average_throughput"`      // Average throughput (Mbps)
	OptimizationEnabled    bool    `json:"optimization_enabled"`    // Whether optimization is active
}

// IsOptimizationEffective returns true if optimization is providing measurable benefits
func (s OptimizationStats) IsOptimizationEffective() bool {
	return s.OptimizationEnabled &&
		s.PerformanceImprovement > 1.1 && // At least 10% improvement
		s.TotalOptimizations > 0
}
