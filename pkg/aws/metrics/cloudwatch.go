// Package metrics provides CloudWatch integration for CargoShip observability
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchClient defines the interface for CloudWatch operations
type CloudWatchClient interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
}

// CloudWatchPublisher publishes CargoShip metrics to AWS CloudWatch
type CloudWatchPublisher struct {
	client     CloudWatchClient
	namespace  string
	region     string
	batchSize  int
	flushInterval time.Duration
	buffer     []types.MetricDatum
	mutex      sync.Mutex
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// MetricConfig configures CloudWatch metrics publishing
type MetricConfig struct {
	Namespace     string        // CloudWatch namespace (default: "CargoShip")
	Region        string        // AWS region
	BatchSize     int           // Metrics per batch (default: 20, max: 20 for CloudWatch)
	FlushInterval time.Duration // How often to flush metrics (default: 30s)
	Enabled       bool          // Enable/disable metrics publishing
}

// NewCloudWatchPublisher creates a new CloudWatch metrics publisher
func NewCloudWatchPublisher(client CloudWatchClient, config MetricConfig) *CloudWatchPublisher {
	if config.Namespace == "" {
		config.Namespace = "CargoShip"
	}
	if config.BatchSize <= 0 || config.BatchSize > 20 {
		config.BatchSize = 20 // CloudWatch limit
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 30 * time.Second
	}

	publisher := &CloudWatchPublisher{
		client:        client,
		namespace:     config.Namespace,
		region:        config.Region,
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		buffer:        make([]types.MetricDatum, 0, config.BatchSize),
		stopChan:      make(chan struct{}),
	}

	if config.Enabled {
		publisher.startFlushTimer()
	}

	return publisher
}

// PublishUploadMetrics publishes upload performance metrics
func (c *CloudWatchPublisher) PublishUploadMetrics(ctx context.Context, metrics *UploadMetrics) error {
	now := time.Now()

	// Core upload metrics
	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("UploadDuration"),
			Value:      aws.Float64(metrics.Duration.Seconds()),
			Unit:       types.StandardUnitSeconds,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		},
		{
			MetricName: aws.String("UploadThroughput"),
			Value:      aws.Float64(metrics.ThroughputMBps),
			Unit:       types.StandardUnitBytesSecond,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		},
		{
			MetricName: aws.String("UploadSize"),
			Value:      aws.Float64(float64(metrics.TotalBytes)),
			Unit:       types.StandardUnitBytes,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		},
		{
			MetricName: aws.String("ChunkCount"),
			Value:      aws.Float64(float64(metrics.ChunkCount)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		},
		{
			MetricName: aws.String("Concurrency"),
			Value:      aws.Float64(float64(metrics.Concurrency)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		},
	}

	// Error metrics
	if metrics.ErrorCount > 0 {
		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("UploadErrors"),
			Value:      aws.Float64(float64(metrics.ErrorCount)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		})

		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("UploadErrorRate"),
			Value:      aws.Float64(float64(metrics.ErrorCount) / float64(metrics.ChunkCount) * 100),
			Unit:       types.StandardUnitPercent,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildUploadDimensions(metrics),
		})
	}

	// Success/failure metrics
	successValue := 0.0
	if metrics.Success {
		successValue = 1.0
	}

	metricData = append(metricData, types.MetricDatum{
		MetricName: aws.String("UploadSuccess"),
		Value:      aws.Float64(successValue),
		Unit:       types.StandardUnitCount,
		Timestamp:  aws.Time(now),
		Dimensions: c.buildUploadDimensions(metrics),
	})

	return c.publishMetrics(ctx, metricData)
}

// PublishCostMetrics publishes cost estimation and optimization metrics
func (c *CloudWatchPublisher) PublishCostMetrics(ctx context.Context, metrics *CostMetrics) error {
	now := time.Now()

	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("EstimatedMonthlyCost"),
			Value:      aws.Float64(metrics.EstimatedMonthlyCost),
			Unit:       types.StandardUnitNone,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildCostDimensions(metrics),
		},
		{
			MetricName: aws.String("EstimatedAnnualCost"),
			Value:      aws.Float64(metrics.EstimatedAnnualCost),
			Unit:       types.StandardUnitNone,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildCostDimensions(metrics),
		},
		{
			MetricName: aws.String("DataSizeGB"),
			Value:      aws.Float64(metrics.DataSizeGB),
			Unit:       types.StandardUnitGigabytes,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildCostDimensions(metrics),
		},
		{
			MetricName: aws.String("PotentialSavingsPercent"),
			Value:      aws.Float64(metrics.PotentialSavingsPercent),
			Unit:       types.StandardUnitPercent,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildCostDimensions(metrics),
		},
	}

	if metrics.ActualMonthlyCost > 0 {
		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("ActualMonthlyCost"),
			Value:      aws.Float64(metrics.ActualMonthlyCost),
			Unit:       types.StandardUnitNone,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildCostDimensions(metrics),
		})
	}

	return c.publishMetrics(ctx, metricData)
}

// PublishNetworkMetrics publishes network performance metrics
func (c *CloudWatchPublisher) PublishNetworkMetrics(ctx context.Context, metrics *NetworkMetrics) error {
	now := time.Now()

	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("NetworkBandwidth"),
			Value:      aws.Float64(metrics.BandwidthMBps),
			Unit:       types.StandardUnitBytesSecond,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildNetworkDimensions(metrics),
		},
		{
			MetricName: aws.String("NetworkLatency"),
			Value:      aws.Float64(metrics.LatencyMs),
			Unit:       types.StandardUnitMilliseconds,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildNetworkDimensions(metrics),
		},
		{
			MetricName: aws.String("OptimalChunkSize"),
			Value:      aws.Float64(float64(metrics.OptimalChunkSizeMB)),
			Unit:       types.StandardUnitMegabytes,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildNetworkDimensions(metrics),
		},
		{
			MetricName: aws.String("OptimalConcurrency"),
			Value:      aws.Float64(float64(metrics.OptimalConcurrency)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildNetworkDimensions(metrics),
		},
	}

	if metrics.PacketLossPercent >= 0 {
		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("PacketLoss"),
			Value:      aws.Float64(metrics.PacketLossPercent),
			Unit:       types.StandardUnitPercent,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildNetworkDimensions(metrics),
		})
	}

	return c.publishMetrics(ctx, metricData)
}

// PublishOperationalMetrics publishes general operational metrics
func (c *CloudWatchPublisher) PublishOperationalMetrics(ctx context.Context, metrics *OperationalMetrics) error {
	now := time.Now()

	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("ActiveUploads"),
			Value:      aws.Float64(float64(metrics.ActiveUploads)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
		},
		{
			MetricName: aws.String("QueuedUploads"),
			Value:      aws.Float64(float64(metrics.QueuedUploads)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
		},
		{
			MetricName: aws.String("CompletedUploads"),
			Value:      aws.Float64(float64(metrics.CompletedUploads)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
		},
		{
			MetricName: aws.String("FailedUploads"),
			Value:      aws.Float64(float64(metrics.FailedUploads)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
		},
		{
			MetricName: aws.String("MemoryUsageMB"),
			Value:      aws.Float64(metrics.MemoryUsageMB),
			Unit:       types.StandardUnitMegabytes,
			Timestamp:  aws.Time(now),
		},
		{
			MetricName: aws.String("CPUUsagePercent"),
			Value:      aws.Float64(metrics.CPUUsagePercent),
			Unit:       types.StandardUnitPercent,
			Timestamp:  aws.Time(now),
		},
	}

	return c.publishMetrics(ctx, metricData)
}

// PublishLifecycleMetrics publishes lifecycle policy metrics
func (c *CloudWatchPublisher) PublishLifecycleMetrics(ctx context.Context, metrics *LifecycleMetrics) error {
	now := time.Now()

	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("LifecyclePoliciesActive"),
			Value:      aws.Float64(float64(metrics.ActivePolicies)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildLifecycleDimensions(metrics),
		},
		{
			MetricName: aws.String("LifecycleSavingsPercent"),
			Value:      aws.Float64(metrics.EstimatedSavingsPercent),
			Unit:       types.StandardUnitPercent,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildLifecycleDimensions(metrics),
		},
		{
			MetricName: aws.String("ObjectsTransitioned"),
			Value:      aws.Float64(float64(metrics.ObjectsTransitioned)),
			Unit:       types.StandardUnitCount,
			Timestamp:  aws.Time(now),
			Dimensions: c.buildLifecycleDimensions(metrics),
		},
	}

	return c.publishMetrics(ctx, metricData)
}

// buildUploadDimensions creates dimensions for upload metrics
func (c *CloudWatchPublisher) buildUploadDimensions(metrics *UploadMetrics) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("Region"),
			Value: aws.String(c.region),
		},
		{
			Name:  aws.String("StorageClass"),
			Value: aws.String(metrics.StorageClass),
		},
		{
			Name:  aws.String("ContentType"),
			Value: aws.String(metrics.ContentType),
		},
		{
			Name:  aws.String("CompressionType"),
			Value: aws.String(metrics.CompressionType),
		},
	}
}

// buildCostDimensions creates dimensions for cost metrics
func (c *CloudWatchPublisher) buildCostDimensions(metrics *CostMetrics) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("Region"),
			Value: aws.String(c.region),
		},
		{
			Name:  aws.String("StorageClass"),
			Value: aws.String(metrics.StorageClass),
		},
		{
			Name:  aws.String("OptimizationType"),
			Value: aws.String(metrics.OptimizationType),
		},
	}
}

// buildNetworkDimensions creates dimensions for network metrics
func (c *CloudWatchPublisher) buildNetworkDimensions(metrics *NetworkMetrics) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("Region"),
			Value: aws.String(c.region),
		},
		{
			Name:  aws.String("NetworkCondition"),
			Value: aws.String(metrics.NetworkCondition),
		},
	}
}

// buildLifecycleDimensions creates dimensions for lifecycle metrics
func (c *CloudWatchPublisher) buildLifecycleDimensions(metrics *LifecycleMetrics) []types.Dimension {
	return []types.Dimension{
		{
			Name:  aws.String("Region"),
			Value: aws.String(c.region),
		},
		{
			Name:  aws.String("PolicyTemplate"),
			Value: aws.String(metrics.PolicyTemplate),
		},
		{
			Name:  aws.String("Bucket"),
			Value: aws.String(metrics.BucketName),
		},
	}
}

// publishMetrics sends metrics to CloudWatch with buffering
func (c *CloudWatchPublisher) publishMetrics(ctx context.Context, metricData []types.MetricDatum) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Add to buffer
	c.buffer = append(c.buffer, metricData...)

	// Flush if buffer is full
	if len(c.buffer) >= c.batchSize {
		return c.flushBuffer(ctx)
	}

	return nil
}

// flushBuffer sends buffered metrics to CloudWatch
func (c *CloudWatchPublisher) flushBuffer(ctx context.Context) error {
	if len(c.buffer) == 0 {
		return nil
	}

	// Create a copy of buffer for sending
	metricsToSend := make([]types.MetricDatum, len(c.buffer))
	copy(metricsToSend, c.buffer)
	
	// Clear buffer
	c.buffer = c.buffer[:0]

	// Send metrics in batches (CloudWatch limit: 20 metrics per request)
	for i := 0; i < len(metricsToSend); i += c.batchSize {
		end := i + c.batchSize
		if end > len(metricsToSend) {
			end = len(metricsToSend)
		}

		batch := metricsToSend[i:end]
		
		_, err := c.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(c.namespace),
			MetricData: batch,
		})

		if err != nil {
			slog.Error("failed to publish metrics to CloudWatch", 
				"error", err, 
				"batch_size", len(batch),
				"namespace", c.namespace)
			return fmt.Errorf("failed to publish metrics: %w", err)
		}

		slog.Debug("published metrics to CloudWatch", 
			"count", len(batch),
			"namespace", c.namespace)
	}

	return nil
}

// startFlushTimer starts the periodic flush timer
func (c *CloudWatchPublisher) startFlushTimer() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				c.mutex.Lock()
				err := c.flushBuffer(ctx)
				c.mutex.Unlock()
				cancel()
				
				if err != nil {
					slog.Error("periodic flush failed", "error", err)
				}
			case <-c.stopChan:
				return
			}
		}
	}()
}

// Flush manually flushes all buffered metrics
func (c *CloudWatchPublisher) Flush(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.flushBuffer(ctx)
}

// Stop stops the publisher and flushes remaining metrics
func (c *CloudWatchPublisher) Stop(ctx context.Context) error {
	close(c.stopChan)
	c.wg.Wait()
	
	// Final flush
	return c.Flush(ctx)
}

// Metric data structures
type UploadMetrics struct {
	Duration        time.Duration `json:"duration"`
	ThroughputMBps  float64       `json:"throughput_mbps"`
	TotalBytes      int64         `json:"total_bytes"`
	ChunkCount      int           `json:"chunk_count"`
	Concurrency     int           `json:"concurrency"`
	ErrorCount      int           `json:"error_count"`
	Success         bool          `json:"success"`
	StorageClass    string        `json:"storage_class"`
	ContentType     string        `json:"content_type"`
	CompressionType string        `json:"compression_type"`
}

type CostMetrics struct {
	EstimatedMonthlyCost     float64 `json:"estimated_monthly_cost"`
	EstimatedAnnualCost      float64 `json:"estimated_annual_cost"`
	ActualMonthlyCost        float64 `json:"actual_monthly_cost"`
	DataSizeGB               float64 `json:"data_size_gb"`
	PotentialSavingsPercent  float64 `json:"potential_savings_percent"`
	StorageClass             string  `json:"storage_class"`
	OptimizationType         string  `json:"optimization_type"`
}

type NetworkMetrics struct {
	BandwidthMBps        float64 `json:"bandwidth_mbps"`
	LatencyMs            float64 `json:"latency_ms"`
	PacketLossPercent    float64 `json:"packet_loss_percent"`
	OptimalChunkSizeMB   int     `json:"optimal_chunk_size_mb"`
	OptimalConcurrency   int     `json:"optimal_concurrency"`
	NetworkCondition     string  `json:"network_condition"`
}

type OperationalMetrics struct {
	ActiveUploads      int     `json:"active_uploads"`
	QueuedUploads      int     `json:"queued_uploads"`
	CompletedUploads   int     `json:"completed_uploads"`
	FailedUploads      int     `json:"failed_uploads"`
	MemoryUsageMB      float64 `json:"memory_usage_mb"`
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`
}

type LifecycleMetrics struct {
	ActivePolicies           int     `json:"active_policies"`
	EstimatedSavingsPercent  float64 `json:"estimated_savings_percent"`
	ObjectsTransitioned      int     `json:"objects_transitioned"`
	PolicyTemplate           string  `json:"policy_template"`
	BucketName               string  `json:"bucket_name"`
}