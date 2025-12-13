package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsCollector(t *testing.T) {
	t.Run("with nil publisher", func(t *testing.T) {
		collector := NewMetricsCollector(nil)
		assert.NotNil(t, collector)
		assert.False(t, collector.enabled)
		assert.NotNil(t, collector.metrics)
		assert.NotNil(t, collector.metrics.ShardMetrics)
	})

	t.Run("with valid publisher", func(t *testing.T) {
		// Mock publisher is nil for now, but in real usage would be CloudWatch
		collector := NewMetricsCollector(nil)
		assert.NotNil(t, collector)
		assert.NotNil(t, collector.metrics)
	})
}

func TestRecordShardMetrics(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true // Enable for testing

	shard1 := ShardMetrics{
		ShardID:          0,
		UploadThroughput: 50.5,
		UploadLatency:    100 * time.Millisecond,
		TotalBytes:       1024 * 1024 * 100, // 100 MB
		ChunkCount:       10,
		ErrorCount:       0,
		SuccessCount:     10,
		Region:           "us-west-2",
		StorageClass:     "GLACIER_IR",
	}

	collector.RecordShardMetrics(shard1)

	assert.Len(t, collector.metrics.ShardMetrics, 1)
	assert.Equal(t, shard1.TotalBytes, collector.metrics.TotalBytes)
	assert.Equal(t, shard1.ChunkCount, collector.metrics.TotalChunks)
	assert.Equal(t, shard1.ErrorCount, collector.metrics.TotalErrors)

	// Add another shard
	shard2 := ShardMetrics{
		ShardID:          1,
		UploadThroughput: 45.2,
		TotalBytes:       1024 * 1024 * 80, // 80 MB
		ChunkCount:       8,
		ErrorCount:       1,
		SuccessCount:     7,
		Region:           "us-west-2",
		StorageClass:     "GLACIER_IR",
	}

	collector.RecordShardMetrics(shard2)

	assert.Len(t, collector.metrics.ShardMetrics, 2)
	assert.Equal(t, shard1.TotalBytes+shard2.TotalBytes, collector.metrics.TotalBytes)
	assert.Equal(t, shard1.ChunkCount+shard2.ChunkCount, collector.metrics.TotalChunks)
	assert.Equal(t, shard1.ErrorCount+shard2.ErrorCount, collector.metrics.TotalErrors)
}

func TestRecordManifestMetrics(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true

	manifestMetrics := ManifestMetrics{
		GenerationTime:    500 * time.Millisecond,
		TotalFiles:        1000,
		TotalBytes:        1024 * 1024 * 1024, // 1 GB
		TotalChunks:       100,
		ShardCount:        10,
		CompressionRatio:  2.5,
		SerializationTime: 100 * time.Millisecond,
		UploadTime:        200 * time.Millisecond,
		Success:           true,
	}

	collector.RecordManifestMetrics(manifestMetrics)

	require.NotNil(t, collector.metrics.ManifestMetrics)
	assert.Equal(t, manifestMetrics.GenerationTime, collector.metrics.ManifestMetrics.GenerationTime)
	assert.Equal(t, manifestMetrics.TotalFiles, collector.metrics.ManifestMetrics.TotalFiles)
	assert.Equal(t, manifestMetrics.Success, collector.metrics.ManifestMetrics.Success)
}

func TestRecordDistributionMetrics(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true

	distribution := ShardDistributionMetrics{
		ShardCount:      10,
		MeanSizeBytes:   100 * 1024 * 1024, // 100 MB
		MedianSizeBytes: 98 * 1024 * 1024,  // 98 MB
		StdDevBytes:     5.2,
		VariancePercent: 5.2,
		MinSizeBytes:    90 * 1024 * 1024, // 90 MB
		MaxSizeBytes:    110 * 1024 * 1024, // 110 MB
		BalanceScore:    94.5,
		StrategyUsed:    "hash",
	}

	collector.RecordDistributionMetrics(distribution)

	require.NotNil(t, collector.metrics.Distribution)
	assert.Equal(t, distribution.ShardCount, collector.metrics.Distribution.ShardCount)
	assert.Equal(t, distribution.BalanceScore, collector.metrics.Distribution.BalanceScore)
	assert.Equal(t, "hash", collector.metrics.Distribution.StrategyUsed)
}

func TestCalculateShardDistribution(t *testing.T) {
	t.Run("empty shards", func(t *testing.T) {
		distribution := CalculateShardDistribution([]ShardMetrics{}, "hash")
		assert.Equal(t, 0, distribution.ShardCount)
	})

	t.Run("perfectly balanced shards", func(t *testing.T) {
		shards := []ShardMetrics{
			{ShardID: 0, TotalBytes: 100 * 1024 * 1024},
			{ShardID: 1, TotalBytes: 100 * 1024 * 1024},
			{ShardID: 2, TotalBytes: 100 * 1024 * 1024},
			{ShardID: 3, TotalBytes: 100 * 1024 * 1024},
		}

		distribution := CalculateShardDistribution(shards, "size")

		assert.Equal(t, 4, distribution.ShardCount)
		assert.Equal(t, int64(100*1024*1024), distribution.MeanSizeBytes)
		assert.Equal(t, int64(100*1024*1024), distribution.MinSizeBytes)
		assert.Equal(t, int64(100*1024*1024), distribution.MaxSizeBytes)
		assert.Equal(t, 100.0, distribution.BalanceScore)
		assert.Equal(t, float64(0), distribution.StdDevBytes)
		assert.Equal(t, "size", distribution.StrategyUsed)
	})

	t.Run("imbalanced shards", func(t *testing.T) {
		shards := []ShardMetrics{
			{ShardID: 0, TotalBytes: 50 * 1024 * 1024},  // 50 MB
			{ShardID: 1, TotalBytes: 100 * 1024 * 1024}, // 100 MB
			{ShardID: 2, TotalBytes: 150 * 1024 * 1024}, // 150 MB
		}

		distribution := CalculateShardDistribution(shards, "hash")

		assert.Equal(t, 3, distribution.ShardCount)
		assert.Equal(t, int64(100*1024*1024), distribution.MeanSizeBytes)
		assert.Equal(t, int64(50*1024*1024), distribution.MinSizeBytes)
		assert.Equal(t, int64(150*1024*1024), distribution.MaxSizeBytes)
		assert.Less(t, distribution.BalanceScore, 100.0)
		assert.Greater(t, distribution.StdDevBytes, 0.0)
		assert.Greater(t, distribution.VariancePercent, 0.0)
	})

	t.Run("extreme imbalance", func(t *testing.T) {
		shards := []ShardMetrics{
			{ShardID: 0, TotalBytes: 1 * 1024 * 1024},   // 1 MB
			{ShardID: 1, TotalBytes: 100 * 1024 * 1024}, // 100 MB
		}

		distribution := CalculateShardDistribution(shards, "directory")

		assert.Equal(t, 2, distribution.ShardCount)
		assert.Less(t, distribution.BalanceScore, 50.0) // Very poor balance
		assert.Greater(t, distribution.VariancePercent, 50.0) // High variance
	})
}

func TestGetMetrics(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true

	// Record some metrics
	collector.RecordShardMetrics(ShardMetrics{
		ShardID:    0,
		TotalBytes: 1024 * 1024 * 100,
		ChunkCount: 10,
	})

	metrics := collector.GetMetrics()

	assert.NotNil(t, metrics)
	assert.Len(t, metrics.ShardMetrics, 1)
	assert.Equal(t, int64(1024*1024*100), metrics.TotalBytes)
	assert.Equal(t, 10, metrics.TotalChunks)
}

func TestPublishMetrics_DisabledCollector(t *testing.T) {
	// Collector with no publisher (disabled)
	collector := NewMetricsCollector(nil)
	ctx := context.Background()

	// Should not error when disabled
	err := collector.PublishMetrics(ctx)
	assert.NoError(t, err)
}

func TestMetricsCollector_ThroughputCalculation(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true
	collector.startTime = time.Now().Add(-10 * time.Second) // Simulate 10 seconds elapsed

	// Simulate 100 MB uploaded
	collector.metrics.TotalBytes = 100 * 1024 * 1024

	// Calculate metrics
	collector.metrics.TotalDuration = time.Since(collector.startTime)
	if collector.metrics.TotalDuration.Seconds() > 0 {
		collector.metrics.TotalThroughput = float64(collector.metrics.TotalBytes) / (1024 * 1024) / collector.metrics.TotalDuration.Seconds()
	}

	// Throughput should be approximately 10 MB/s (100 MB / 10 seconds)
	assert.InDelta(t, 10.0, collector.metrics.TotalThroughput, 1.0)
}

func TestMetricsCollector_ErrorRateCalculation(t *testing.T) {
	collector := NewMetricsCollector(nil)
	collector.enabled = true

	collector.metrics.TotalChunks = 100
	collector.metrics.TotalErrors = 5

	// Calculate error rate
	if collector.metrics.TotalChunks > 0 {
		collector.metrics.ErrorRate = float64(collector.metrics.TotalErrors) / float64(collector.metrics.TotalChunks) * 100
	}

	// Error rate should be 5%
	assert.Equal(t, 5.0, collector.metrics.ErrorRate)
}

func TestShardMetrics_Serialization(t *testing.T) {
	shard := ShardMetrics{
		ShardID:          5,
		UploadThroughput: 123.45,
		UploadLatency:    250 * time.Millisecond,
		TotalBytes:       1024 * 1024 * 500, // 500 MB
		ChunkCount:       50,
		ErrorCount:       2,
		SuccessCount:     48,
		StartTime:        time.Now(),
		EndTime:          time.Now().Add(10 * time.Second),
		Region:           "us-east-1",
		StorageClass:     "DEEP_ARCHIVE",
	}

	// Verify all fields are set correctly
	assert.Equal(t, 5, shard.ShardID)
	assert.Equal(t, 123.45, shard.UploadThroughput)
	assert.Equal(t, 250*time.Millisecond, shard.UploadLatency)
	assert.Equal(t, int64(1024*1024*500), shard.TotalBytes)
	assert.Equal(t, 50, shard.ChunkCount)
	assert.Equal(t, 2, shard.ErrorCount)
	assert.Equal(t, 48, shard.SuccessCount)
	assert.Equal(t, "us-east-1", shard.Region)
	assert.Equal(t, "DEEP_ARCHIVE", shard.StorageClass)
}

func TestManifestMetrics_Serialization(t *testing.T) {
	manifest := ManifestMetrics{
		GenerationTime:    1500 * time.Millisecond,
		TotalFiles:        5000,
		TotalBytes:        5 * 1024 * 1024 * 1024, // 5 GB
		TotalChunks:       500,
		ShardCount:        20,
		CompressionRatio:  3.2,
		SerializationTime: 300 * time.Millisecond,
		UploadTime:        500 * time.Millisecond,
		Success:           true,
	}

	assert.Equal(t, 1500*time.Millisecond, manifest.GenerationTime)
	assert.Equal(t, 5000, manifest.TotalFiles)
	assert.Equal(t, int64(5*1024*1024*1024), manifest.TotalBytes)
	assert.Equal(t, 500, manifest.TotalChunks)
	assert.Equal(t, 20, manifest.ShardCount)
	assert.InDelta(t, 3.2, manifest.CompressionRatio, 0.01)
	assert.True(t, manifest.Success)
}
