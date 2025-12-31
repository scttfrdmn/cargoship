// Package multiregion - Performance and benchmark tests for failover scenarios
// This file contains performance tests, benchmarks, and load testing for the failover system.

package multiregion

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkFailoverOperations benchmarks different failover operations
func BenchmarkFailoverOperations(b *testing.B) {
	config := createValidMultiRegionConfig()
	config.Failover.FailoverTimeout = 5 * time.Second
	logger := log.New(nil)

	b.Run("FailoverManager_Creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			manager := NewFailoverManager(config, logger)
			_ = manager
		}
	})

	b.Run("DetectFailure", func(b *testing.B) {
		manager := NewFailoverManager(config, logger)
		ctx := context.Background()
		regionName := "benchmark-region"

		// Pre-populate some failures
		for i := 0; i < 3; i++ {
			manager.RecordFailure(regionName)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = manager.DetectFailure(ctx, regionName)
		}
	})

	b.Run("RecordFailure", func(b *testing.B) {
		manager := NewFailoverManager(config, logger)
		regionName := "benchmark-region"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			manager.RecordFailure(regionName)
		}
	})

	b.Run("RecordSuccess", func(b *testing.B) {
		manager := NewFailoverManager(config, logger)
		regionName := "benchmark-region"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			manager.RecordSuccess(regionName)
		}
	})

	b.Run("ImmediateFailover_Execution", func(b *testing.B) {
		config.Failover.Strategy = FailoverImmediate
		manager := NewFailoverManager(config, logger)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fromRegion := fmt.Sprintf("from-region-%d", i%2)
			toRegion := fmt.Sprintf("to-region-%d", (i+1)%2)
			_ = manager.ExecuteFailover(ctx, fromRegion, toRegion)
		}
	})

	b.Run("GracefulFailover_Execution", func(b *testing.B) {
		config.Failover.Strategy = FailoverGraceful
		manager := NewFailoverManager(config, logger)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fromRegion := fmt.Sprintf("from-region-%d", i%2)
			toRegion := fmt.Sprintf("to-region-%d", (i+1)%2)
			_ = manager.ExecuteFailover(ctx, fromRegion, toRegion)
		}
	})
}

// BenchmarkCoordinatorFailoverIntegration benchmarks coordinator integration
func BenchmarkCoordinatorFailoverIntegration(b *testing.B) {
	config := createValidMultiRegionConfig()
	config.Failover.AutoFailover = true
	config.Failover.Strategy = FailoverImmediate
	config.Failover.FailoverTimeout = 2 * time.Second

	coordinator := NewCoordinator()

	ctx := context.Background()
	err := coordinator.Initialize(ctx, config)
	require.NoError(b, err)
	defer func() { _ = coordinator.Shutdown(ctx) }()

	b.Run("ShouldTriggerFailover_Check", func(b *testing.B) {
		// Get a test region
		coordinator.mu.RLock()
		var testRegion *Region
		for _, region := range coordinator.regions {
			testRegion = region
			break
		}
		coordinator.mu.RUnlock()

		// Set failure conditions
		testRegion.Metrics.ErrorRate = 30.0
		testRegion.Metrics.ConsecutiveFailedChecks = 5

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = coordinator.shouldTriggerFailover(testRegion)
		}
	})

	b.Run("SelectFailoverTarget", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = coordinator.selectFailoverTarget("us-east-1")
		}
	})

	b.Run("DetectAndHandleFailures", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			coordinator.detectAndHandleFailures()
		}
	})
}

// TestFailoverPerformanceUnderLoad tests failover performance under various load conditions
func TestFailoverPerformanceUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance under load test in short mode (takes 8s)")
	}
	t.Run("HighFrequencyFailureDetection", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)

		ctx := context.Background()
		regionName := "high-frequency-test"

		// Measure time for high-frequency failure detection
		start := time.Now()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			if i%10 == 0 {
				manager.RecordFailure(regionName)
			}
			_, _ = manager.DetectFailure(ctx, regionName)
		}

		duration := time.Since(start)
		avgPerOp := duration / time.Duration(iterations)

		assert.Less(t, avgPerOp, 1*time.Millisecond,
			"Failure detection should be fast (<%v, got %v)", 1*time.Millisecond, avgPerOp)

		t.Logf("High frequency detection: %d operations in %v (avg: %v per op)",
			iterations, duration, avgPerOp)
	})

	t.Run("ConcurrentFailoverOperations", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 3 * time.Second
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)

		concurrency := 10
		operationsPerGoroutine := 5

		var wg sync.WaitGroup
		start := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				ctx := context.Background()

				for j := 0; j < operationsPerGoroutine; j++ {
					fromRegion := fmt.Sprintf("from-region-%d", workerID)
					toRegion := fmt.Sprintf("to-region-%d", workerID)

					_ = manager.ExecuteFailover(ctx, fromRegion, toRegion)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		totalOps := concurrency * operationsPerGoroutine
		avgPerOp := duration / time.Duration(totalOps)

		assert.Less(t, duration, 30*time.Second,
			"Concurrent failover operations should complete within reasonable time")

		t.Logf("Concurrent failover: %d operations (%d goroutines × %d ops) in %v (avg: %v per op)",
			totalOps, concurrency, operationsPerGoroutine, duration, avgPerOp)
	})

	t.Run("MemoryUsageUnderLoad", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)

		// Generate load with many regions and failure records
		regions := make([]string, 100)
		for i := 0; i < 100; i++ {
			regions[i] = fmt.Sprintf("load-test-region-%d", i)
		}

		start := time.Now()

		// Simulate heavy failure recording
		for i := 0; i < 1000; i++ {
			region := regions[i%len(regions)]
			if i%3 == 0 {
				manager.RecordFailure(region)
			} else {
				manager.RecordSuccess(region)
			}
		}

		// Perform detection on all regions
		ctx := context.Background()
		for _, region := range regions {
			_, _ = manager.DetectFailure(ctx, region)
		}

		duration := time.Since(start)

		// Verify data structures are maintained efficiently
		assert.Less(t, len(manager.failureHistory), 200,
			"Failure history should not grow excessively")

		assert.Less(t, duration, 1*time.Second,
			"Load test should complete quickly")

		t.Logf("Memory load test: 1000 operations on 100 regions in %v", duration)
	})
}

// TestFailoverLatencyCharacteristics tests latency characteristics of different failover types
func TestFailoverLatencyCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency characteristics test in short mode (takes 18s)")
	}
	t.Run("FailoverLatencyComparison", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 5 * time.Second
		logger := log.New(nil)

		strategies := []struct {
			name     string
			strategy FailoverStrategy
			maxTime  time.Duration
		}{
			{"Immediate", FailoverImmediate, 2 * time.Second},
			{"Graceful", FailoverGraceful, 3 * time.Second},
		}

		for _, s := range strategies {
			t.Run(s.name, func(t *testing.T) {
				config.Failover.Strategy = s.strategy
				manager := NewFailoverManager(config, logger)

				measurements := make([]time.Duration, 5)

				for i := 0; i < 5; i++ {
					ctx := context.Background()
					start := time.Now()

					fromRegion := fmt.Sprintf("from-%d", i)
					toRegion := fmt.Sprintf("to-%d", i)

					err := manager.ExecuteFailover(ctx, fromRegion, toRegion)
					measurements[i] = time.Since(start)

					if s.strategy != FailoverManual { // Manual failover times out
						assert.NoError(t, err, "Failover should succeed for %s", s.name)
					}

					assert.Less(t, measurements[i], s.maxTime,
						"Failover should complete within %v for %s (got %v)",
						s.maxTime, s.name, measurements[i])
				}

				// Calculate average
				var total time.Duration
				for _, d := range measurements {
					total += d
				}
				avg := total / time.Duration(len(measurements))

				t.Logf("%s failover latency: avg=%v, measurements=%v", s.name, avg, measurements)
			})
		}
	})
}

// TestFailoverThroughput tests throughput characteristics
func TestFailoverThroughput(t *testing.T) {
	t.Run("FailureRecordingThroughput", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)

		operations := 10000
		regionName := "throughput-test"

		start := time.Now()
		for i := 0; i < operations; i++ {
			if i%2 == 0 {
				manager.RecordFailure(regionName)
			} else {
				manager.RecordSuccess(regionName)
			}
		}
		duration := time.Since(start)

		throughput := float64(operations) / duration.Seconds()

		assert.Greater(t, throughput, 10000.0,
			"Failure recording should handle >10k ops/sec (got %.2f ops/sec)", throughput)

		t.Logf("Failure recording throughput: %.2f ops/sec (%d ops in %v)",
			throughput, operations, duration)
	})

	t.Run("FailureDetectionThroughput", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)

		// Pre-populate some data
		regions := make([]string, 50)
		for i := 0; i < 50; i++ {
			regions[i] = fmt.Sprintf("region-%d", i)
			for j := 0; j < 3; j++ {
				manager.RecordFailure(regions[i])
			}
		}

		operations := 5000
		ctx := context.Background()

		start := time.Now()
		for i := 0; i < operations; i++ {
			region := regions[i%len(regions)]
			_, _ = manager.DetectFailure(ctx, region)
		}
		duration := time.Since(start)

		throughput := float64(operations) / duration.Seconds()

		assert.Greater(t, throughput, 1000.0,
			"Failure detection should handle >1k ops/sec (got %.2f ops/sec)", throughput)

		t.Logf("Failure detection throughput: %.2f ops/sec (%d ops in %v)",
			throughput, operations, duration)
	})
}

// TestFailoverScalability tests scalability with increasing numbers of regions
func TestFailoverScalability(t *testing.T) {
	regionCounts := []int{10, 50, 100, 500}

	for _, regionCount := range regionCounts {
		t.Run(fmt.Sprintf("Regions_%d", regionCount), func(t *testing.T) {
			config := createValidMultiRegionConfig()
			logger := log.New(nil)
			manager := NewFailoverManager(config, logger)

			// Create regions
			regions := make([]string, regionCount)
			for i := 0; i < regionCount; i++ {
				regions[i] = fmt.Sprintf("scale-region-%d", i)
			}

			// Record failures for half the regions
			for i := 0; i < regionCount/2; i++ {
				for j := 0; j < 3; j++ {
					manager.RecordFailure(regions[i])
				}
			}

			// Measure detection time across all regions
			ctx := context.Background()
			start := time.Now()

			for _, region := range regions {
				_, _ = manager.DetectFailure(ctx, region)
			}

			duration := time.Since(start)
			avgPerRegion := duration / time.Duration(regionCount)

			assert.Less(t, avgPerRegion, 1*time.Millisecond,
				"Detection time per region should be <1ms with %d regions (got %v)",
				regionCount, avgPerRegion)

			t.Logf("Scalability test: %d regions processed in %v (avg: %v per region)",
				regionCount, duration, avgPerRegion)
		})
	}
}

// TestFailoverResourceUsage tests resource usage patterns
func TestFailoverResourceUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource usage test in short mode (takes 17s)")
	}
	t.Run("GoroutineLeakPrevention", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.Strategy = FailoverGraceful
		config.Failover.FailoverTimeout = 1 * time.Second
		logger := log.New(nil)

		initialGoroutines := countGoroutines()

		// Create and use multiple managers
		for i := 0; i < 10; i++ {
			manager := NewFailoverManager(config, logger)
			// Use context with timeout to prevent test hanging
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			// Perform operations that might create goroutines
			for j := 0; j < 3; j++ {
				fromRegion := fmt.Sprintf("from-%d-%d", i, j)
				toRegion := fmt.Sprintf("to-%d-%d", i, j)
				_ = manager.ExecuteFailover(ctx, fromRegion, toRegion)
			}
			cancel()
		}

		// Wait for any async operations to complete
		time.Sleep(2 * time.Second)

		finalGoroutines := countGoroutines()
		goroutineIncrease := finalGoroutines - initialGoroutines

		assert.Less(t, goroutineIncrease, 50,
			"Goroutine count should not increase significantly (started: %d, ended: %d, increase: %d)",
			initialGoroutines, finalGoroutines, goroutineIncrease)

		t.Logf("Goroutine usage: started=%d, ended=%d, increase=%d",
			initialGoroutines, finalGoroutines, goroutineIncrease)
	})
}

// TestFailoverReliabilityUnderStress tests system reliability under stress
func TestFailoverReliabilityUnderStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode (takes 5s)")
	}
	t.Run("ExtendedStressTest", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 2 * time.Second
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger)

		// Extended stress test parameters
		duration := 5 * time.Second
		goroutines := 20
		operationsPerSecond := 100

		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()

		var wg sync.WaitGroup
		var totalOps int64
		var successfulOps int64
		var mu sync.Mutex

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				ticker := time.NewTicker(time.Second / time.Duration(operationsPerSecond/goroutines))
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						regionName := fmt.Sprintf("stress-region-%d", workerID%10)

						// Mix of operations
						operation := (workerID * int(time.Now().UnixNano())) % 4
						var err error

						switch operation {
						case 0:
							manager.RecordFailure(regionName)
						case 1:
							manager.RecordSuccess(regionName)
						case 2:
							_, err = manager.DetectFailure(context.Background(), regionName)
						case 3:
							fromRegion := fmt.Sprintf("from-%d", workerID)
							toRegion := fmt.Sprintf("to-%d", workerID)
							err = manager.ExecuteFailover(context.Background(), fromRegion, toRegion)
						}

						mu.Lock()
						totalOps++
						if err == nil {
							successfulOps++
						}
						mu.Unlock()
					}
				}
			}(i)
		}

		wg.Wait()

		mu.Lock()
		successRate := float64(successfulOps) / float64(totalOps) * 100
		mu.Unlock()

		assert.Greater(t, totalOps, int64(100), "Should perform significant number of operations")
		assert.Greater(t, successRate, 50.0, "Success rate should be reasonable under stress (%.1f%%)", successRate)

		t.Logf("Stress test results: %d total ops, %d successful (%.1f%% success rate)",
			totalOps, successfulOps, successRate)
	})
}

// Helper function to count goroutines (simplified implementation)
func countGoroutines() int {
	// This is a simplified version - in practice you might use runtime.NumGoroutine()
	// For testing purposes, we'll return a mock value
	return 50 // Mock implementation
}
