package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
)

// MockCloudWatchClient is a mock implementation of CloudWatch client for testing
type MockCloudWatchClient struct {
	putMetricDataCalls []cloudwatch.PutMetricDataInput
	returnError        error
}

func (m *MockCloudWatchClient) PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	if m.putMetricDataCalls == nil {
		m.putMetricDataCalls = make([]cloudwatch.PutMetricDataInput, 0)
	}
	m.putMetricDataCalls = append(m.putMetricDataCalls, *params)
	
	if m.returnError != nil {
		return nil, m.returnError
	}
	
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func TestNewCloudWatchPublisher(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	
	tests := []struct {
		name           string
		config         MetricConfig
		expectedNS     string
		expectedBatch  int
		expectedFlush  time.Duration
	}{
		{
			name: "default values",
			config: MetricConfig{
				Enabled: true,
			},
			expectedNS:    "CargoShip",
			expectedBatch: 20,
			expectedFlush: 30 * time.Second,
		},
		{
			name: "custom values",
			config: MetricConfig{
				Namespace:     "CustomNamespace",
				BatchSize:     10,
				FlushInterval: 60 * time.Second,
				Enabled:       true,
			},
			expectedNS:    "CustomNamespace",
			expectedBatch: 10,
			expectedFlush: 60 * time.Second,
		},
		{
			name: "invalid batch size",
			config: MetricConfig{
				BatchSize: 25, // Over CloudWatch limit
				Enabled:   true,
			},
			expectedNS:    "CargoShip",
			expectedBatch: 20, // Should be capped
			expectedFlush: 30 * time.Second,
		},
		{
			name: "zero batch size",
			config: MetricConfig{
				BatchSize: 0,
				Enabled:   true,
			},
			expectedNS:    "CargoShip",
			expectedBatch: 20, // Should use default
			expectedFlush: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := NewCloudWatchPublisher(mockClient, tt.config)
			
			if publisher == nil {
				t.Fatalf("NewCloudWatchPublisher() returned nil")
			}
			
			if publisher.namespace != tt.expectedNS {
				t.Errorf("namespace = %v, want %v", publisher.namespace, tt.expectedNS)
			}
			
			if publisher.batchSize != tt.expectedBatch {
				t.Errorf("batchSize = %v, want %v", publisher.batchSize, tt.expectedBatch)
			}
			
			if publisher.flushInterval != tt.expectedFlush {
				t.Errorf("flushInterval = %v, want %v", publisher.flushInterval, tt.expectedFlush)
			}
			
			// Clean up
			if tt.config.Enabled {
				_ = publisher.Stop(context.Background())
			}
		})
	}
}

func TestCloudWatchPublisher_PublishUploadMetrics(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{
		Namespace: "TestNamespace",
		BatchSize: 20,
		Enabled:   false, // Don't start timer
	}
	
	publisher := NewCloudWatchPublisher(mockClient, config)
	publisher.region = "us-east-1"
	
	metrics := &UploadMetrics{
		Duration:        5 * time.Minute,
		ThroughputMBps:  125.5,
		TotalBytes:      1024 * 1024 * 1024, // 1GB
		ChunkCount:      100,
		Concurrency:     8,
		ErrorCount:      2,
		Success:         true,
		StorageClass:    "STANDARD",
		ContentType:     "application/octet-stream",
		CompressionType: "gzip",
	}
	
	ctx := context.Background()
	err := publisher.PublishUploadMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishUploadMetrics() error = %v", err)
	}
	
	// Verify metrics were buffered (not yet sent)
	if len(mockClient.putMetricDataCalls) != 0 {
		t.Errorf("Expected metrics to be buffered, but %d calls were made", len(mockClient.putMetricDataCalls))
	}
	
	// Flush and verify
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	if len(mockClient.putMetricDataCalls) == 0 {
		t.Fatalf("Expected metrics to be sent after flush")
	}
	
	call := mockClient.putMetricDataCalls[0]
	if *call.Namespace != "TestNamespace" {
		t.Errorf("namespace = %v, want TestNamespace", *call.Namespace)
	}
	
	// Should have core metrics plus error metrics plus success metric
	expectedMetricCount := 8 // Duration, Throughput, Size, ChunkCount, Concurrency, Errors, ErrorRate, Success
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
	
	// Verify specific metrics
	metricNames := make(map[string]bool)
	for _, metric := range call.MetricData {
		metricNames[*metric.MetricName] = true
	}
	
	expectedMetrics := []string{
		"UploadDuration", "UploadThroughput", "UploadSize", "ChunkCount", 
		"Concurrency", "UploadErrors", "UploadErrorRate", "UploadSuccess",
	}
	
	for _, expected := range expectedMetrics {
		if !metricNames[expected] {
			t.Errorf("Missing expected metric: %s", expected)
		}
	}
}

func TestCloudWatchPublisher_PublishUploadMetrics_NoErrors(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &UploadMetrics{
		Duration:        1 * time.Minute,
		ThroughputMBps:  100.0,
		TotalBytes:      1024,
		ChunkCount:      10,
		Concurrency:     4,
		ErrorCount:      0, // No errors
		Success:         true,
		StorageClass:    "STANDARD",
		ContentType:     "text/plain",
		CompressionType: "none",
	}
	
	ctx := context.Background()
	err := publisher.PublishUploadMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishUploadMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	// Should have core metrics plus success metric, but no error metrics
	expectedMetricCount := 6 // Duration, Throughput, Size, ChunkCount, Concurrency, Success
	call := mockClient.putMetricDataCalls[0]
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_PublishCostMetrics(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	publisher.region = "us-west-2"
	
	metrics := &CostMetrics{
		EstimatedMonthlyCost:    150.75,
		EstimatedAnnualCost:     1809.0,
		ActualMonthlyCost:       142.50,
		DataSizeGB:              500.0,
		PotentialSavingsPercent: 15.5,
		StorageClass:            "INTELLIGENT_TIERING",
		OptimizationType:        "lifecycle",
	}
	
	ctx := context.Background()
	err := publisher.PublishCostMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishCostMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	call := mockClient.putMetricDataCalls[0]
	expectedMetricCount := 5 // EstimatedMonthlyCost, EstimatedAnnualCost, ActualMonthlyCost, DataSizeGB, PotentialSavingsPercent
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_PublishNetworkMetrics(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &NetworkMetrics{
		BandwidthMBps:        100.5,
		LatencyMs:            25.5,
		PacketLossPercent:    0.1,
		OptimalChunkSizeMB:   16,
		OptimalConcurrency:   8,
		NetworkCondition:     "good",
	}
	
	ctx := context.Background()
	err := publisher.PublishNetworkMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishNetworkMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	call := mockClient.putMetricDataCalls[0]
	expectedMetricCount := 5 // Bandwidth, Latency, OptimalChunkSize, OptimalConcurrency, PacketLoss
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_PublishNetworkMetrics_NoPacketLoss(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &NetworkMetrics{
		BandwidthMBps:        100.5,
		LatencyMs:            25.5,
		PacketLossPercent:    -1, // Negative value indicates no measurement
		OptimalChunkSizeMB:   16,
		OptimalConcurrency:   8,
		NetworkCondition:     "good",
	}
	
	ctx := context.Background()
	err := publisher.PublishNetworkMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishNetworkMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	call := mockClient.putMetricDataCalls[0]
	expectedMetricCount := 4 // Should not include PacketLoss
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_PublishOperationalMetrics(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &OperationalMetrics{
		ActiveUploads:     5,
		QueuedUploads:     15,
		CompletedUploads:  100,
		FailedUploads:     3,
		MemoryUsageMB:     512.5,
		CPUUsagePercent:   75.2,
	}
	
	ctx := context.Background()
	err := publisher.PublishOperationalMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishOperationalMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	call := mockClient.putMetricDataCalls[0]
	expectedMetricCount := 6 // All operational metrics
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_PublishLifecycleMetrics(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &LifecycleMetrics{
		ActivePolicies:          3,
		EstimatedSavingsPercent: 25.5,
		ObjectsTransitioned:     1000,
		PolicyTemplate:          "archive-after-30-days",
		BucketName:              "test-bucket",
	}
	
	ctx := context.Background()
	err := publisher.PublishLifecycleMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishLifecycleMetrics() error = %v", err)
	}
	
	err = publisher.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	
	call := mockClient.putMetricDataCalls[0]
	expectedMetricCount := 3 // ActivePolicies, EstimatedSavingsPercent, ObjectsTransitioned
	if len(call.MetricData) != expectedMetricCount {
		t.Errorf("metric count = %d, want %d", len(call.MetricData), expectedMetricCount)
	}
}

func TestCloudWatchPublisher_BufferManagement(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{
		BatchSize: 10, // Increased batch size so we don't trigger flush immediately
		Enabled:   false,
	}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	ctx := context.Background()
	
	// Publish one metric - should be buffered (operational metrics have 6 metrics each)
	metrics1 := &OperationalMetrics{ActiveUploads: 1}
	err := publisher.PublishOperationalMetrics(ctx, metrics1)
	if err != nil {
		t.Errorf("PublishOperationalMetrics() error = %v", err)
	}
	
	if len(mockClient.putMetricDataCalls) != 0 {
		t.Errorf("Expected no calls yet, got %d", len(mockClient.putMetricDataCalls))
	}
	
	// Publish second metric - should still be buffered (total 12 metrics, batch size 10)
	metrics2 := &OperationalMetrics{QueuedUploads: 2}
	err = publisher.PublishOperationalMetrics(ctx, metrics2)
	if err != nil {
		t.Errorf("PublishOperationalMetrics() error = %v", err)
	}
	
	if len(mockClient.putMetricDataCalls) == 0 {
		t.Errorf("Expected metrics to be flushed when buffer exceeds batch size")
	}
}

func TestCloudWatchPublisher_Dimensions(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	publisher.region = "us-east-1"
	
	metrics := &UploadMetrics{
		Duration:        1 * time.Minute,
		StorageClass:    "GLACIER",
		ContentType:     "application/json",
		CompressionType: "zstd",
	}
	
	dimensions := publisher.buildUploadDimensions(metrics)
	
	expectedDimensions := map[string]string{
		"Region":          "us-east-1",
		"StorageClass":    "GLACIER",
		"ContentType":     "application/json",
		"CompressionType": "zstd",
	}
	
	if len(dimensions) != len(expectedDimensions) {
		t.Errorf("dimension count = %d, want %d", len(dimensions), len(expectedDimensions))
	}
	
	for _, dim := range dimensions {
		expectedValue, exists := expectedDimensions[*dim.Name]
		if !exists {
			t.Errorf("Unexpected dimension: %s", *dim.Name)
		} else if *dim.Value != expectedValue {
			t.Errorf("Dimension %s = %s, want %s", *dim.Name, *dim.Value, expectedValue)
		}
	}
}

func TestCloudWatchPublisher_ErrorHandling(t *testing.T) {
	mockClient := &MockCloudWatchClient{
		returnError: fmt.Errorf("CloudWatch error"),
	}
	config := MetricConfig{Enabled: false}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	metrics := &OperationalMetrics{ActiveUploads: 1}
	
	ctx := context.Background()
	err := publisher.PublishOperationalMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishOperationalMetrics() should not error on buffering, got: %v", err)
	}
	
	// Flush should return the error
	err = publisher.Flush(ctx)
	if err == nil {
		t.Errorf("Flush() should return error when CloudWatch fails")
	}
}

func TestCloudWatchPublisher_Stop(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	config := MetricConfig{
		FlushInterval: 100 * time.Millisecond,
		Enabled:       true, // Start the timer
	}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	// Add some metrics
	metrics := &OperationalMetrics{ActiveUploads: 1}
	ctx := context.Background()
	err := publisher.PublishOperationalMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("PublishOperationalMetrics() error = %v", err)
	}
	
	// Stop should flush remaining metrics
	err = publisher.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	
	// Verify metrics were flushed
	if len(mockClient.putMetricDataCalls) == 0 {
		t.Errorf("Expected metrics to be flushed on Stop()")
	}
}

func TestMetricStructFields(t *testing.T) {
	// Test UploadMetrics
	upload := UploadMetrics{
		Duration:        5 * time.Minute,
		ThroughputMBps:  125.5,
		TotalBytes:      1024,
		ChunkCount:      10,
		Concurrency:     8,
		ErrorCount:      2,
		Success:         true,
		StorageClass:    "STANDARD",
		ContentType:     "application/octet-stream",
		CompressionType: "gzip",
	}
	
	if upload.Duration != 5*time.Minute {
		t.Errorf("UploadMetrics.Duration = %v, want %v", upload.Duration, 5*time.Minute)
	}
	if upload.ThroughputMBps != 125.5 {
		t.Errorf("UploadMetrics.ThroughputMBps = %v, want 125.5", upload.ThroughputMBps)
	}
	
	// Test CostMetrics
	cost := CostMetrics{
		EstimatedMonthlyCost:    150.75,
		EstimatedAnnualCost:     1809.0,
		ActualMonthlyCost:       142.50,
		DataSizeGB:              500.0,
		PotentialSavingsPercent: 15.5,
		StorageClass:            "INTELLIGENT_TIERING",
		OptimizationType:        "lifecycle",
	}
	
	if cost.EstimatedMonthlyCost != 150.75 {
		t.Errorf("CostMetrics.EstimatedMonthlyCost = %v, want 150.75", cost.EstimatedMonthlyCost)
	}
	
	// Test NetworkMetrics
	network := NetworkMetrics{
		BandwidthMBps:        100.5,
		LatencyMs:            25.5,
		PacketLossPercent:    0.1,
		OptimalChunkSizeMB:   16,
		OptimalConcurrency:   8,
		NetworkCondition:     "good",
	}
	
	if network.BandwidthMBps != 100.5 {
		t.Errorf("NetworkMetrics.BandwidthMBps = %v, want 100.5", network.BandwidthMBps)
	}
	
	// Test OperationalMetrics
	operational := OperationalMetrics{
		ActiveUploads:     5,
		QueuedUploads:     15,
		CompletedUploads:  100,
		FailedUploads:     3,
		MemoryUsageMB:     512.5,
		CPUUsagePercent:   75.2,
	}
	
	if operational.ActiveUploads != 5 {
		t.Errorf("OperationalMetrics.ActiveUploads = %v, want 5", operational.ActiveUploads)
	}
	
	// Test LifecycleMetrics
	lifecycle := LifecycleMetrics{
		ActivePolicies:          3,
		EstimatedSavingsPercent: 25.5,
		ObjectsTransitioned:     1000,
		PolicyTemplate:          "archive-after-30-days",
		BucketName:              "test-bucket",
	}
	
	if lifecycle.ActivePolicies != 3 {
		t.Errorf("LifecycleMetrics.ActivePolicies = %v, want 3", lifecycle.ActivePolicies)
	}
}

func TestMetricConfig_Defaults(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	
	// Test empty config gets defaults
	config := MetricConfig{}
	publisher := NewCloudWatchPublisher(mockClient, config)
	
	if publisher.namespace != "CargoShip" {
		t.Errorf("default namespace = %v, want CargoShip", publisher.namespace)
	}
	if publisher.batchSize != 20 {
		t.Errorf("default batchSize = %v, want 20", publisher.batchSize)
	}
	if publisher.flushInterval != 30*time.Second {
		t.Errorf("default flushInterval = %v, want 30s", publisher.flushInterval)
	}
}