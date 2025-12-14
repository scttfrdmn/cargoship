package monitoring

import (
	"context"
	"sync"
	"time"
)

// MetricsCollector collects and aggregates performance metrics from various sources.
type MetricsCollector struct {
	config         *MonitoringConfig
	currentMetrics *PerformanceMetrics
	customMetrics  map[string]*CustomMetric
	mu             sync.RWMutex
	isRunning      bool
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(config *MonitoringConfig) *MetricsCollector {
	return &MetricsCollector{
		config:         config,
		currentMetrics: NewDefaultPerformanceMetrics(),
		customMetrics:  make(map[string]*CustomMetric),
	}
}

// Start begins metrics collection.
func (mc *MetricsCollector) Start(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.isRunning {
		return nil
	}

	mc.isRunning = true
	return nil
}

// Stop stops metrics collection.
func (mc *MetricsCollector) Stop() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.isRunning = false
}

// GetCurrentMetrics returns the current performance metrics.
func (mc *MetricsCollector) GetCurrentMetrics() *PerformanceMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy
	metrics := *mc.currentMetrics
	return &metrics
}

// CollectFrom collects metrics from a monitor interface.
func (mc *MetricsCollector) CollectFrom(monitor interface{}) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	switch m := monitor.(type) {
	case *TransferPerformanceMonitor:
		transferMetrics := m.GetMetrics()
		mc.currentMetrics.TransferMetrics = transferMetrics
	case *SystemResourceMonitor:
		systemMetrics := m.GetMetrics()
		mc.currentMetrics.SystemMetrics = systemMetrics
	case *NetworkPerformanceMonitor:
		networkMetrics := m.GetMetrics()
		mc.currentMetrics.NetworkMetrics = networkMetrics
	case *S3PerformanceMonitor:
		s3Metrics := m.GetMetrics()
		mc.currentMetrics.S3Metrics = s3Metrics
	case *StagingPerformanceMonitor:
		stagingMetrics := m.GetMetrics()
		mc.currentMetrics.StagingMetrics = stagingMetrics
	}

	mc.currentMetrics.LastUpdated = time.Now()
}

// RegisterMetric registers a custom metric.
func (mc *MetricsCollector) RegisterMetric(metric *CustomMetric) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.customMetrics[metric.Name] = metric
	return nil
}

// RecordMetric records a metric value with labels.
func (mc *MetricsCollector) RecordMetric(name string, value float64, labels map[string]string, timestamp time.Time) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if metric, exists := mc.customMetrics[name]; exists {
		dataPoint := &MetricDataPoint{
			Value:     value,
			Labels:    labels,
			Timestamp: timestamp,
		}
		metric.DataPoints = append(metric.DataPoints, dataPoint)

		// Keep only recent data points
		maxPoints := 1000
		if len(metric.DataPoints) > maxPoints {
			metric.DataPoints = metric.DataPoints[len(metric.DataPoints)-maxPoints:]
		}
	}
}

// PerformanceMetrics contains all performance metrics.
type PerformanceMetrics struct {
	TransferMetrics *TransferMetrics         `json:"transfer_metrics"`
	SystemMetrics   *SystemMetrics           `json:"system_metrics"`
	NetworkMetrics  *NetworkMetrics          `json:"network_metrics"`
	S3Metrics       *S3Metrics               `json:"s3_metrics"`
	StagingMetrics  *StagingMetrics          `json:"staging_metrics"`
	CustomMetrics   map[string]*CustomMetric `json:"custom_metrics"`
	LastUpdated     time.Time                `json:"last_updated"`
}

// TransferMetrics contains transfer performance metrics.
type TransferMetrics struct {
	ActiveTransfers     int       `json:"active_transfers"`
	TotalThroughputMBps float64   `json:"total_throughput_mbps"`
	AverageLatencyMs    float64   `json:"average_latency_ms"`
	SuccessRate         float64   `json:"success_rate"`
	ErrorCount          int64     `json:"error_count"`
	TotalBytesProcessed int64     `json:"total_bytes_processed"`
	AverageChunkSizeMB  float64   `json:"average_chunk_size_mb"`
	CompressionRatio    float64   `json:"compression_ratio"`
	LastUpdated         time.Time `json:"last_updated"`
}

// SystemMetrics contains system resource metrics.
type SystemMetrics struct {
	CPUUsagePercent      float64   `json:"cpu_usage_percent"`
	MemoryUsageMB        float64   `json:"memory_usage_mb"`
	MemoryUsagePercent   float64   `json:"memory_usage_percent"`
	DiskUsagePercent     float64   `json:"disk_usage_percent"`
	NetworkIOBytesPerSec float64   `json:"network_io_bytes_per_sec"`
	DiskIOBytesPerSec    float64   `json:"disk_io_bytes_per_sec"`
	ActiveGoroutines     int       `json:"active_goroutines"`
	HeapSizeMB           float64   `json:"heap_size_mb"`
	GCPauseMs            float64   `json:"gc_pause_ms"`
	LastUpdated          time.Time `json:"last_updated"`
}

// NetworkMetrics contains network performance metrics.
type NetworkMetrics struct {
	BandwidthMBps      float64   `json:"bandwidth_mbps"`
	LatencyMs          float64   `json:"latency_ms"`
	PacketLossPercent  float64   `json:"packet_loss_percent"`
	JitterMs           float64   `json:"jitter_ms"`
	ConnectionCount    int       `json:"connection_count"`
	ActiveConnections  int       `json:"active_connections"`
	ReliabilityScore   float64   `json:"reliability_score"`
	OptimalChunkSizeMB int       `json:"optimal_chunk_size_mb"`
	OptimalConcurrency int       `json:"optimal_concurrency"`
	LastUpdated        time.Time `json:"last_updated"`
}

// S3Metrics contains S3-specific performance metrics.
type S3Metrics struct {
	RequestLatencyMs   float64            `json:"request_latency_ms"`
	SuccessfulRequests int64              `json:"successful_requests"`
	FailedRequests     int64              `json:"failed_requests"`
	ErrorRate          float64            `json:"error_rate"`
	ThroughputMBps     float64            `json:"throughput_mbps"`
	ActiveConnections  int                `json:"active_connections"`
	RegionLatencyMs    map[string]float64 `json:"region_latency_ms"`
	RetryCount         int64              `json:"retry_count"`
	ThrottleCount      int64              `json:"throttle_count"`
	LastUpdated        time.Time          `json:"last_updated"`
}

// StagingMetrics contains staging performance metrics.
type StagingMetrics struct {
	ActiveChunks           int       `json:"active_chunks"`
	StagingBufferUsageMB   float64   `json:"staging_buffer_usage_mb"`
	ChunkDeduplicationRate float64   `json:"chunk_deduplication_rate"`
	CompressionEfficiency  float64   `json:"compression_efficiency"`
	PredictionAccuracy     float64   `json:"prediction_accuracy"`
	AdaptationRate         float64   `json:"adaptation_rate"`
	QueueDepth             int       `json:"queue_depth"`
	ProcessingLatencyMs    float64   `json:"processing_latency_ms"`
	LastUpdated            time.Time `json:"last_updated"`
}

// CustomMetric represents a user-defined metric.
type CustomMetric struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Type        MetricType         `json:"type"`
	Unit        string             `json:"unit"`
	DataPoints  []*MetricDataPoint `json:"data_points"`
	Labels      map[string]string  `json:"labels"`
	CreatedAt   time.Time          `json:"created_at"`
}

// MetricType defines the type of metric.
type MetricType int

const (
	MetricTypeCounter MetricType = iota
	MetricTypeGauge
	MetricTypeHistogram
	MetricTypeSummary
)

// MetricDataPoint represents a single metric measurement.
type MetricDataPoint struct {
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
}

// NewDefaultPerformanceMetrics creates default performance metrics.
func NewDefaultPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		TransferMetrics: &TransferMetrics{LastUpdated: time.Now()},
		SystemMetrics:   &SystemMetrics{LastUpdated: time.Now()},
		NetworkMetrics:  &NetworkMetrics{LastUpdated: time.Now()},
		S3Metrics: &S3Metrics{
			RegionLatencyMs: make(map[string]float64),
			LastUpdated:     time.Now(),
		},
		StagingMetrics: &StagingMetrics{LastUpdated: time.Now()},
		CustomMetrics:  make(map[string]*CustomMetric),
		LastUpdated:    time.Now(),
	}
}
