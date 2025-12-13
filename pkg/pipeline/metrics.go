// Package pipeline provides CloudWatch metrics for CargoHold operations (Issue #111)
package pipeline

import (
	"context"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/metrics"
)

// ShardMetrics tracks performance metrics for individual shards
type ShardMetrics struct {
	ShardID            int           `json:"shard_id"`
	UploadThroughput   float64       `json:"upload_throughput_mbps"`   // MB/s for this shard
	UploadLatency      time.Duration `json:"upload_latency_ms"`        // Latency per upload
	TotalBytes         int64         `json:"total_bytes"`              // Total bytes uploaded
	ChunkCount         int           `json:"chunk_count"`              // Number of chunks
	ErrorCount         int           `json:"error_count"`              // Errors encountered
	SuccessCount       int           `json:"success_count"`            // Successful uploads
	StartTime          time.Time     `json:"start_time"`               // When shard started
	EndTime            time.Time     `json:"end_time"`                 // When shard completed
	Region             string        `json:"region"`                   // AWS region
	StorageClass       string        `json:"storage_class"`            // S3 storage class
}

// ManifestMetrics tracks manifest generation performance
type ManifestMetrics struct {
	GenerationTime    time.Duration `json:"generation_time_ms"` // Time to generate manifest
	TotalFiles        int           `json:"total_files"`        // Number of files
	TotalBytes        int64         `json:"total_bytes"`        // Total data size
	TotalChunks       int           `json:"total_chunks"`       // Number of chunks
	ShardCount        int           `json:"shard_count"`        // Number of shards
	CompressionRatio  float64       `json:"compression_ratio"`  // Compression efficiency
	SerializationTime time.Duration `json:"serialization_time"` // JSON serialization time
	UploadTime        time.Duration `json:"upload_time"`        // S3 upload time
	Success           bool          `json:"success"`            // Whether generation succeeded
}

// ShardDistributionMetrics tracks shard distribution balance
type ShardDistributionMetrics struct {
	ShardCount        int       `json:"shard_count"`         // Total number of shards
	MeanSizeBytes     int64     `json:"mean_size_bytes"`     // Average shard size
	MedianSizeBytes   int64     `json:"median_size_bytes"`   // Median shard size
	StdDevBytes       float64   `json:"std_dev_bytes"`       // Standard deviation
	VariancePercent   float64   `json:"variance_percent"`    // Size variance as percentage
	MinSizeBytes      int64     `json:"min_size_bytes"`      // Smallest shard
	MaxSizeBytes      int64     `json:"max_size_bytes"`      // Largest shard
	BalanceScore      float64   `json:"balance_score"`       // 0-100 score (100 = perfect balance)
	StrategyUsed      string    `json:"strategy_used"`       // Sharding strategy (hash, size, etc.)
}

// PipelineMetrics aggregates all CargoHold metrics
type PipelineMetrics struct {
	// Overall pipeline metrics
	TotalDuration     time.Duration `json:"total_duration"`
	TotalThroughput   float64       `json:"total_throughput_mbps"`
	TotalBytes        int64         `json:"total_bytes"`
	TotalFiles        int           `json:"total_files"`
	TotalChunks       int           `json:"total_chunks"`

	// Shard-level metrics
	ShardMetrics      []ShardMetrics            `json:"shard_metrics"`
	Distribution      *ShardDistributionMetrics `json:"distribution"`

	// Manifest metrics
	ManifestMetrics   *ManifestMetrics          `json:"manifest_metrics"`

	// Error tracking
	TotalErrors       int                       `json:"total_errors"`
	ErrorRate         float64                   `json:"error_rate"`

	// Configuration
	UploadID          string                    `json:"upload_id"`
	Region            string                    `json:"region"`
	StorageClass      string                    `json:"storage_class"`
}

// MetricsCollector collects and publishes CargoHold metrics
type MetricsCollector struct {
	publisher  *metrics.CloudWatchPublisher
	metrics    *PipelineMetrics
	startTime  time.Time
	enabled    bool
}

// NewMetricsCollector creates a new metrics collector for the pipeline
func NewMetricsCollector(publisher *metrics.CloudWatchPublisher) *MetricsCollector {
	return &MetricsCollector{
		publisher: publisher,
		metrics: &PipelineMetrics{
			ShardMetrics: make([]ShardMetrics, 0),
		},
		startTime: time.Now(),
		enabled:   publisher != nil,
	}
}

// RecordShardMetrics records metrics for a shard upload
func (mc *MetricsCollector) RecordShardMetrics(shard ShardMetrics) {
	if !mc.enabled {
		return
	}

	mc.metrics.ShardMetrics = append(mc.metrics.ShardMetrics, shard)
	mc.metrics.TotalBytes += shard.TotalBytes
	mc.metrics.TotalChunks += shard.ChunkCount
	mc.metrics.TotalErrors += shard.ErrorCount
}

// RecordManifestMetrics records manifest generation metrics
func (mc *MetricsCollector) RecordManifestMetrics(manifest ManifestMetrics) {
	if !mc.enabled {
		return
	}

	mc.metrics.ManifestMetrics = &manifest
}

// RecordDistributionMetrics records shard distribution metrics
func (mc *MetricsCollector) RecordDistributionMetrics(distribution ShardDistributionMetrics) {
	if !mc.enabled {
		return
	}

	mc.metrics.Distribution = &distribution
}

// PublishMetrics publishes all collected metrics to CloudWatch
func (mc *MetricsCollector) PublishMetrics(ctx context.Context) error {
	if !mc.enabled {
		return nil
	}

	// Calculate overall metrics
	mc.metrics.TotalDuration = time.Since(mc.startTime)
	if mc.metrics.TotalDuration.Seconds() > 0 {
		mc.metrics.TotalThroughput = float64(mc.metrics.TotalBytes) / (1024 * 1024) / mc.metrics.TotalDuration.Seconds()
	}
	if mc.metrics.TotalChunks > 0 {
		mc.metrics.ErrorRate = float64(mc.metrics.TotalErrors) / float64(mc.metrics.TotalChunks) * 100
	}

	// Publish per-shard metrics
	for _, shard := range mc.metrics.ShardMetrics {
		if err := mc.publishShardMetrics(ctx, shard); err != nil {
			return err
		}
	}

	// Publish manifest metrics
	if mc.metrics.ManifestMetrics != nil {
		if err := mc.publishManifestMetrics(ctx, *mc.metrics.ManifestMetrics); err != nil {
			return err
		}
	}

	// Publish distribution metrics
	if mc.metrics.Distribution != nil {
		if err := mc.publishDistributionMetrics(ctx, *mc.metrics.Distribution); err != nil {
			return err
		}
	}

	// Publish aggregate pipeline metrics
	return mc.publishPipelineMetrics(ctx)
}

// publishShardMetrics publishes metrics for a single shard
func (mc *MetricsCollector) publishShardMetrics(ctx context.Context, shard ShardMetrics) error {
	// Convert to CloudWatch upload metrics format
	uploadMetrics := &metrics.UploadMetrics{
		Duration:        shard.EndTime.Sub(shard.StartTime),
		ThroughputMBps:  shard.UploadThroughput,
		TotalBytes:      shard.TotalBytes,
		ChunkCount:      shard.ChunkCount,
		ErrorCount:      shard.ErrorCount,
		Success:         shard.ErrorCount == 0,
		StorageClass:    shard.StorageClass,
		ContentType:     "application/x-tar+zstd",
		CompressionType: "zstd",
	}

	return mc.publisher.PublishUploadMetrics(ctx, uploadMetrics)
}

// publishManifestMetrics publishes manifest generation metrics
func (mc *MetricsCollector) publishManifestMetrics(ctx context.Context, manifest ManifestMetrics) error {
	// Manifest metrics are published as operational metrics
	operationalMetrics := &metrics.OperationalMetrics{
		CompletedUploads: 1,
		FailedUploads:    0,
	}

	if !manifest.Success {
		operationalMetrics.CompletedUploads = 0
		operationalMetrics.FailedUploads = 1
	}

	return mc.publisher.PublishOperationalMetrics(ctx, operationalMetrics)
}

// publishDistributionMetrics publishes shard distribution metrics
func (mc *MetricsCollector) publishDistributionMetrics(ctx context.Context, distribution ShardDistributionMetrics) error {
	// Distribution metrics are published as operational metrics
	operationalMetrics := &metrics.OperationalMetrics{
		ActiveUploads: distribution.ShardCount,
	}

	return mc.publisher.PublishOperationalMetrics(ctx, operationalMetrics)
}

// publishPipelineMetrics publishes aggregate pipeline metrics
func (mc *MetricsCollector) publishPipelineMetrics(ctx context.Context) error {
	uploadMetrics := &metrics.UploadMetrics{
		Duration:        mc.metrics.TotalDuration,
		ThroughputMBps:  mc.metrics.TotalThroughput,
		TotalBytes:      mc.metrics.TotalBytes,
		ChunkCount:      mc.metrics.TotalChunks,
		Concurrency:     len(mc.metrics.ShardMetrics),
		ErrorCount:      mc.metrics.TotalErrors,
		Success:         mc.metrics.TotalErrors == 0,
		StorageClass:    mc.metrics.StorageClass,
		ContentType:     "application/x-tar+zstd",
		CompressionType: "zstd",
	}

	return mc.publisher.PublishUploadMetrics(ctx, uploadMetrics)
}

// GetMetrics returns the collected metrics
func (mc *MetricsCollector) GetMetrics() *PipelineMetrics {
	return mc.metrics
}

// CalculateShardDistribution calculates distribution statistics from shard metrics
func CalculateShardDistribution(shards []ShardMetrics, strategy string) ShardDistributionMetrics {
	if len(shards) == 0 {
		return ShardDistributionMetrics{}
	}

	// Calculate basic statistics
	var totalSize int64
	var minSize int64 = shards[0].TotalBytes
	var maxSize int64 = shards[0].TotalBytes

	for _, shard := range shards {
		totalSize += shard.TotalBytes
		if shard.TotalBytes < minSize {
			minSize = shard.TotalBytes
		}
		if shard.TotalBytes > maxSize {
			maxSize = shard.TotalBytes
		}
	}

	meanSize := totalSize / int64(len(shards))

	// Calculate standard deviation
	var sumSquaredDiff float64
	for _, shard := range shards {
		diff := float64(shard.TotalBytes - meanSize)
		sumSquaredDiff += diff * diff
	}
	stdDev := 0.0
	if len(shards) > 1 {
		stdDev = sumSquaredDiff / float64(len(shards))
	}

	// Calculate variance percentage
	variancePercent := 0.0
	if meanSize > 0 {
		variancePercent = (stdDev / float64(meanSize)) * 100
	}

	// Calculate balance score (100 = perfect balance, 0 = worst)
	balanceScore := 100.0
	if maxSize > 0 {
		balanceScore = (1.0 - (float64(maxSize-minSize) / float64(maxSize))) * 100
	}

	return ShardDistributionMetrics{
		ShardCount:      len(shards),
		MeanSizeBytes:   meanSize,
		MedianSizeBytes: meanSize, // Simplified: use mean as median
		StdDevBytes:     stdDev,
		VariancePercent: variancePercent,
		MinSizeBytes:    minSize,
		MaxSizeBytes:    maxSize,
		BalanceScore:    balanceScore,
		StrategyUsed:    strategy,
	}
}
