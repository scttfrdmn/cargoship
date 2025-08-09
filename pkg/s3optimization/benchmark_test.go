package s3optimization

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// BenchmarkS3OptimizerInitialization benchmarks the creation of S3 optimizers
func BenchmarkS3OptimizerInitialization(b *testing.B) {
	ctx := context.Background()
	s3Client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		optimizer, err := NewS3Optimizer(ctx, s3Client, nil, nil)
		if err != nil {
			b.Fatalf("Failed to create optimizer: %v", err)
		}
		_ = optimizer.Shutdown(ctx)
	}
}

// BenchmarkPerformanceMetricsCollection benchmarks metrics collection
func BenchmarkPerformanceMetricsCollection(b *testing.B) {
	ctx := context.Background()
	s3Client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	optimizer, err := NewS3Optimizer(ctx, s3Client, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create optimizer: %v", err)
	}
	defer func() {
		if err := optimizer.Shutdown(ctx); err != nil {
			b.Logf("Warning: failed to shutdown optimizer: %v", err)
		}
	}()

	// Simulate some requests to generate metrics
	for i := 0; i < 10; i++ {
		optimizer.recordRequest(time.Millisecond*50, nil)
		optimizer.recordBytes(1024)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := optimizer.GetPerformanceMetrics()
		_ = metrics.OptimizationRatio
	}
}

// BenchmarkNetworkConditionsUpdate benchmarks network condition updates
func BenchmarkNetworkConditionsUpdate(b *testing.B) {
	ctx := context.Background()
	s3Client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	optimizer, err := NewS3Optimizer(ctx, s3Client, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create optimizer: %v", err)
	}
	defer func() {
		if err := optimizer.Shutdown(ctx); err != nil {
			b.Logf("Warning: failed to shutdown optimizer: %v", err)
		}
	}()

	conditions := &NetworkConditions{
		Bandwidth:   100.0,
		RTT:         50 * time.Millisecond,
		PacketLoss:  0.5,
		Congestion:  10.0,
		LastUpdated: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := optimizer.UpdateNetworkConditions(conditions)
		if err != nil {
			b.Fatalf("Failed to update network conditions: %v", err)
		}
	}
}

// BenchmarkConfigurationValidation benchmarks configuration processing
func BenchmarkConfigurationValidation(b *testing.B) {
	config := &Config{
		EnableBBR:          true,
		EnableCUBIC:        true,
		NetworkAdaptation:  true,
		PredictiveMode:     true,
		MaxConnections:     20,
		ConnectionPoolSize: 16,
		BufferSize:         128 * 1024 * 1024,
		MetricsEnabled:     true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate configuration validation
		_ = config.EnableBBR && config.EnableCUBIC
		_ = config.MaxConnections > 0
		_ = config.BufferSize > 0
	}
}

// BenchmarkSafeStringOperations benchmarks string safety operations
func BenchmarkSafeStringOperations(b *testing.B) {
	testStrings := []*string{
		nil,
		aws.String("test-key-1"),
		aws.String("test-key-2"),
		aws.String(""),
		aws.String("very-long-test-key-with-many-characters-to-simulate-real-world-usage"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, str := range testStrings {
			result := safeStringValue(str)
			_ = len(result) // Use the result to prevent optimization
		}
	}
}

// BenchmarkOptimizationRatioCalculation benchmarks optimization effectiveness calculation
func BenchmarkOptimizationRatioCalculation(b *testing.B) {
	metrics := &Metrics{
		totalRequests:      1000,
		successfulRequests: 950,
		failedRequests:     50,
		totalLatency:       time.Second * 10,
		totalBytes:         1024 * 1024 * 100, // 100MB
		startTime:          time.Now().Add(-time.Minute),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		perfMetrics := metrics.getPerformanceMetrics()
		_ = perfMetrics.OptimizationRatio
		_ = perfMetrics.BandwidthSavings
		_ = perfMetrics.ThroughputMbps
	}
}

// BenchmarkBatchOperationsPreparation benchmarks batch operation setup
func BenchmarkBatchOperationsPreparation(b *testing.B) {
	ctx := context.Background()

	// Create test requests
	batchSize := 10
	getRequests := make([]*s3.GetObjectInput, batchSize)
	putRequests := make([]*s3.PutObjectInput, batchSize)

	for i := 0; i < batchSize; i++ {
		getRequests[i] = &s3.GetObjectInput{
			Bucket: aws.String("test-bucket"),
			Key:    aws.String("test-key-" + string(rune(i+'0'))),
		}

		putRequests[i] = &s3.PutObjectInput{
			Bucket: aws.String("test-bucket"),
			Key:    aws.String("test-key-" + string(rune(i+'0'))),
			Body:   strings.NewReader("test data"),
		}
	}

	s3Client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	optimizer, err := NewS3Optimizer(ctx, s3Client, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create optimizer: %v", err)
	}
	defer func() {
		if err := optimizer.Shutdown(ctx); err != nil {
			b.Logf("Warning: failed to shutdown optimizer: %v", err)
		}
	}()

	b.ResetTimer()

	b.Run("BatchGETPreparation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate batch preparation overhead
			batchResults := make([]*s3.GetObjectOutput, len(getRequests))
			for j := range getRequests {
				batchResults[j] = &s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader("mock response")),
				}
			}
			_ = batchResults
		}
	})

	b.Run("BatchPUTPreparation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate batch preparation overhead
			batchResults := make([]*s3.PutObjectOutput, len(putRequests))
			for j := range putRequests {
				batchResults[j] = &s3.PutObjectOutput{
					ETag: aws.String("mock-etag"),
				}
			}
			_ = batchResults
		}
	})
}

// BenchmarkHealthCheckOperations benchmarks health check performance
func BenchmarkHealthCheckOperations(b *testing.B) {
	ctx := context.Background()
	s3Client := s3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	optimizer, err := NewS3Optimizer(ctx, s3Client, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create optimizer: %v", err)
	}
	defer func() {
		if err := optimizer.Shutdown(ctx); err != nil {
			b.Logf("Warning: failed to shutdown optimizer: %v", err)
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := optimizer.HealthCheck(ctx)
		if err != nil {
			b.Fatalf("Health check failed: %v", err)
		}
	}
}

// BenchmarkDefaultConfigGeneration benchmarks default configuration creation
func BenchmarkDefaultConfigGeneration(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config := DefaultConfig()
		_ = config.EnableBBR
		_ = config.MaxConnections
		_ = config.BufferSize
	}
}
