package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransferSchedulerStart(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	// Start scheduler (should not block)
	scheduler.Start(ctx)
	
	// Give it a moment to start background loops
	time.Sleep(time.Millisecond * 100)
	
	// Should not panic or error
}

func TestTransferSchedulerRegisterPrefix(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("test-prefix", 100.0)
	
	scheduler.mu.RLock()
	metrics, exists := scheduler.prefixMetrics["test-prefix"]
	scheduler.mu.RUnlock()
	
	assert.True(t, exists)
	assert.NotNil(t, metrics)
	assert.Equal(t, "test-prefix", metrics.PrefixID)
	assert.Equal(t, 0, metrics.ActiveUploads)
	assert.Equal(t, 0.0, metrics.ThroughputMBps)
	assert.Equal(t, 50.0, metrics.LatencyMs) // Default baseline
	assert.Equal(t, 0.0, metrics.ErrorRate)
	assert.Equal(t, 100.0, metrics.ProcessingCapacity)
}

func TestTransferSchedulerSelectOptimalPrefixNoPrefix(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	_, err := scheduler.SelectOptimalPrefix(upload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no prefixes registered")
}

func TestTransferSchedulerSelectOptimalPrefixTCPLike(t *testing.T) {
	config := DefaultCoordinationConfig()
	config.Strategy = "tcp_like"
	scheduler := NewTransferScheduler(config)
	
	// Register multiple prefixes with different performance characteristics
	scheduler.RegisterPrefix("prefix-1", 100.0)
	scheduler.RegisterPrefix("prefix-2", 100.0)
	scheduler.RegisterPrefix("prefix-3", 100.0)
	
	// Update metrics for different performance levels
	scheduler.UpdatePrefixMetrics("prefix-1", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-1",
		ActiveUploads:        2,
		ThroughputMBps:       50.0,
		LatencyMs:            100.0,
		ErrorRate:            0.01,
		CongestionWindow:     16,
		BandwidthUtilization: 0.5,
		QueueLength:          5,
		ProcessingCapacity:   100.0,
	})
	
	scheduler.UpdatePrefixMetrics("prefix-2", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-2",
		ActiveUploads:        1,
		ThroughputMBps:       80.0,
		LatencyMs:            50.0,
		ErrorRate:            0.001,
		CongestionWindow:     24,
		BandwidthUtilization: 0.3,
		QueueLength:          2,
		ProcessingCapacity:   100.0,
	})
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	selectedPrefix, err := scheduler.SelectOptimalPrefix(upload)
	assert.NoError(t, err)
	// prefix-2 should be selected as it has better performance metrics
	assert.Equal(t, "prefix-2", selectedPrefix)
}

func TestTransferSchedulerSelectOptimalPrefixFairShare(t *testing.T) {
	config := DefaultCoordinationConfig()
	config.Strategy = "fair_share"
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("prefix-1", 100.0)
	scheduler.RegisterPrefix("prefix-2", 100.0)
	
	// Set different utilization levels
	scheduler.UpdatePrefixMetrics("prefix-1", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-1",
		BandwidthUtilization: 0.8,
		QueueLength:          10,
		ProcessingCapacity:   100.0,
	})
	
	scheduler.UpdatePrefixMetrics("prefix-2", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-2",
		BandwidthUtilization: 0.3,
		QueueLength:          3,
		ProcessingCapacity:   100.0,
	})
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	selectedPrefix, err := scheduler.SelectOptimalPrefix(upload)
	assert.NoError(t, err)
	// prefix-2 should be selected as it has lower utilization
	assert.Equal(t, "prefix-2", selectedPrefix)
}

func TestTransferSchedulerSelectOptimalPrefixAdaptive(t *testing.T) {
	config := DefaultCoordinationConfig()
	config.Strategy = "adaptive"
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("prefix-1", 100.0)
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	selectedPrefix, err := scheduler.SelectOptimalPrefix(upload)
	assert.NoError(t, err)
	assert.Equal(t, "prefix-1", selectedPrefix)
}

func TestTransferSchedulerUpdatePrefixMetrics(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("test-prefix", 100.0)
	
	newMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ActiveUploads:        5,
		ThroughputMBps:       75.0,
		LatencyMs:            80.0,
		ErrorRate:            0.02,
		BandwidthUtilization: 0.6,
		QueueLength:          8,
	}
	
	scheduler.UpdatePrefixMetrics("test-prefix", newMetrics)
	
	scheduler.mu.RLock()
	updated := scheduler.prefixMetrics["test-prefix"]
	scheduler.mu.RUnlock()
	
	assert.Equal(t, 5, updated.ActiveUploads)
	assert.Equal(t, 75.0, updated.ThroughputMBps)
	assert.Equal(t, 80.0, updated.LatencyMs)
	assert.Equal(t, 0.02, updated.ErrorRate)
	assert.Equal(t, 0.6, updated.BandwidthUtilization)
	assert.Equal(t, 8, updated.QueueLength)
	
	// Check that historical data was updated
	assert.Len(t, updated.ThroughputHistory, 1)
	assert.Equal(t, 75.0, updated.ThroughputHistory[0])
	assert.Len(t, updated.LatencyHistory, 1)
	assert.Equal(t, 80.0, updated.LatencyHistory[0])
	assert.Len(t, updated.ErrorHistory, 1)
	assert.Equal(t, 0.02, updated.ErrorHistory[0])
}

func TestTransferSchedulerUpdatePrefixMetricsNonExistent(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	newMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "non-existent",
		ActiveUploads:        5,
		ThroughputMBps:       75.0,
	}
	
	// Should not panic or error when updating non-existent prefix
	scheduler.UpdatePrefixMetrics("non-existent", newMetrics)
}

func TestTransferSchedulerGetMetrics(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("prefix-1", 100.0)
	scheduler.RegisterPrefix("prefix-2", 100.0)
	
	// Update some metrics
	scheduler.UpdatePrefixMetrics("prefix-1", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-1",
		ActiveUploads:        3,
		ThroughputMBps:       40.0,
		BandwidthUtilization: 0.4,
		QueueLength:          6,
	})
	
	scheduler.UpdatePrefixMetrics("prefix-2", &PrefixPerformanceMetrics{
		PrefixID:             "prefix-2",
		ActiveUploads:        2,
		ThroughputMBps:       60.0,
		BandwidthUtilization: 0.6,
		QueueLength:          4,
	})
	
	metrics := scheduler.GetMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, 100.0, metrics.GlobalThroughputMBps) // 40 + 60
	assert.Equal(t, 2, metrics.ActivePrefixes)
	assert.Equal(t, 5.0, metrics.AverageQueueLength) // (6 + 4) / 2
}

func TestTransferSchedulerHistoricalMetricsLimit(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("test-prefix", 100.0)
	
	// Add more than the maximum history size (20)
	for i := 0; i < 25; i++ {
		metrics := &PrefixPerformanceMetrics{
			PrefixID:       "test-prefix",
			ThroughputMBps: float64(i),
			LatencyMs:      float64(i * 10),
			ErrorRate:      float64(i) * 0.01,
		}
		scheduler.UpdatePrefixMetrics("test-prefix", metrics)
	}
	
	scheduler.mu.RLock()
	updated := scheduler.prefixMetrics["test-prefix"]
	scheduler.mu.RUnlock()
	
	// Should be limited to 20 entries
	assert.Len(t, updated.ThroughputHistory, 20)
	assert.Len(t, updated.LatencyHistory, 20)
	assert.Len(t, updated.ErrorHistory, 20)
	
	// Should contain the most recent values
	assert.Equal(t, 24.0, updated.ThroughputHistory[19]) // Last value
	assert.Equal(t, 5.0, updated.ThroughputHistory[0])   // First retained value
}

func TestTransferSchedulerNetworkProfileUpdate(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("test-prefix", 100.0)
	
	initialBandwidth := scheduler.networkProfile.EstimatedBandwidthMBps
	initialRTT := scheduler.networkProfile.BaselineRTT
	
	// Update with higher bandwidth
	metrics := &PrefixPerformanceMetrics{
		PrefixID:       "test-prefix",
		ThroughputMBps: 150.0, // Higher than initial estimate
		LatencyMs:      30.0,  // Lower than initial RTT
	}
	scheduler.UpdatePrefixMetrics("test-prefix", metrics)
	
	// Bandwidth should be updated (with learning rate)
	assert.Greater(t, scheduler.networkProfile.EstimatedBandwidthMBps, initialBandwidth)
	
	// RTT should be updated to lower value
	assert.Less(t, scheduler.networkProfile.BaselineRTT, initialRTT)
	
	// Learning confidence should increase
	assert.Greater(t, scheduler.networkProfile.LearningConfidence, 0.5)
}

func TestTransferSchedulerAdaptiveAdjustments(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("prefix-1", 100.0)
	scheduler.RegisterPrefix("prefix-2", 100.0)
	
	// Build up history for prefix-1 showing declining performance
	for i := 0; i < 10; i++ {
		throughput := 100.0 - float64(i)*5 // Declining from 100 to 55
		metrics := &PrefixPerformanceMetrics{
			PrefixID:       "prefix-1",
			ThroughputMBps: throughput,
		}
		scheduler.UpdatePrefixMetrics("prefix-1", metrics)
	}
	
	// Build up history for prefix-2 showing stable performance
	for i := 0; i < 5; i++ {
		metrics := &PrefixPerformanceMetrics{
			PrefixID:           "prefix-2",
			ThroughputMBps:     80.0,
			QueueLength:        3,
			ProcessingCapacity: 100.0,
		}
		scheduler.UpdatePrefixMetrics("prefix-2", metrics)
	}
	
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	// The adaptive selection should detect the poor performance of prefix-1
	// and potentially select prefix-2 instead
	selectedPrefix, err := scheduler.SelectOptimalPrefix(upload)
	assert.NoError(t, err)
	assert.NotEmpty(t, selectedPrefix)
}

func TestTransferSchedulerNetworkProfileLearning(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	scheduler.RegisterPrefix("prefix-1", 100.0)
	scheduler.RegisterPrefix("prefix-2", 100.0)
	
	// Set high learning confidence and increasing bandwidth trend
	scheduler.networkProfile.LearningConfidence = 0.8
	scheduler.networkProfile.BandwidthTrend = TrendIncreasing
	
	// Create upload with deadline
	upload := &ScheduledUpload{
		ArchivePath:   "/test/archive.tar",
		Priority:      3,
		EstimatedSize: 2 * 1024 * 1024 * 1024, // 2GB - large upload
		Deadline:      time.Now().Add(time.Minute * 30), // Not urgent
	}
	
	// Build bandwidth trends for prefixes
	for i := 0; i < 10; i++ {
		// prefix-1 with increasing trend
		metrics1 := &PrefixPerformanceMetrics{
			PrefixID:       "prefix-1",
			ThroughputMBps: 50.0 + float64(i)*2, // Increasing
		}
		scheduler.UpdatePrefixMetrics("prefix-1", metrics1)
		
		// prefix-2 with stable performance
		metrics2 := &PrefixPerformanceMetrics{
			PrefixID:       "prefix-2",
			ThroughputMBps: 60.0, // Stable
		}
		scheduler.UpdatePrefixMetrics("prefix-2", metrics2)
	}
	
	selectedPrefix, err := scheduler.SelectOptimalPrefix(upload)
	assert.NoError(t, err)
	assert.NotEmpty(t, selectedPrefix)
}

func TestTransferSchedulerConcurrency(t *testing.T) {
	config := DefaultCoordinationConfig()
	scheduler := NewTransferScheduler(config)
	
	var wg sync.WaitGroup
	
	// Register prefixes concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id)
			scheduler.RegisterPrefix(prefixID, 100.0)
		}(i)
	}
	
	// Update metrics concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id%10)
			metrics := &PrefixPerformanceMetrics{
				PrefixID:       prefixID,
				ThroughputMBps: float64(id % 100),
				LatencyMs:      float64((id % 200) + 50),
			}
			scheduler.UpdatePrefixMetrics(prefixID, metrics)
		}(i)
	}
	
	// Select prefixes concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			upload := &ScheduledUpload{
				ArchivePath:   fmt.Sprintf("/test/archive-%d.tar", id),
				Priority:      3,
				EstimatedSize: 1024 * 1024,
			}
			_, err := scheduler.SelectOptimalPrefix(upload)
			assert.NoError(t, err)
		}(i)
	}
	
	wg.Wait()
	
	// Verify final state
	metrics := scheduler.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, 10, metrics.ActivePrefixes)
}

func TestTransferSchedulerBackgroundLoops(t *testing.T) {
	config := DefaultCoordinationConfig()
	config.UpdateInterval = time.Millisecond * 50 // Fast for testing
	scheduler := NewTransferScheduler(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()
	
	scheduler.RegisterPrefix("test-prefix", 100.0)
	
	// Start background loops
	scheduler.Start(ctx)
	
	// Wait for loops to run
	time.Sleep(time.Millisecond * 150)
	
	// Should complete without errors or panics
}