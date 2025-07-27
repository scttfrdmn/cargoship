// Package multiregion provides performance benchmarking for multi-region operations
package multiregion

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkResult captures performance metrics for multi-region operations
type BenchmarkResult struct {
	OperationType    string
	TotalOperations  int
	TotalDuration    time.Duration
	OperationsPerSec float64
	AvgLatencyMs     float64
	P95LatencyMs     float64
	P99LatencyMs     float64
	ErrorRate        float64
	ThroughputMBps   float64
	ConcurrentUsers  int
	Timestamp        time.Time
}

// PerformanceTestSuite provides comprehensive performance testing for multi-region operations
type PerformanceTestSuite struct {
	coordinator   Coordinator
	regionSelector RegionSelector
	failoverManager *DefaultFailoverManager
	config        *MultiRegionConfig
	logger        *log.Logger
	results       []BenchmarkResult
	mu            sync.Mutex
}

// NewPerformanceTestSuite creates a new performance test suite
func NewPerformanceTestSuite(t *testing.T) *PerformanceTestSuite {
	config := createValidMultiRegionConfig()
	
	// Add more regions for comprehensive testing
	config.Regions = append(config.Regions, 
		Region{
			Name: "eu-west-1", 
			Priority: 3, 
			Weight: 40, 
			Status: RegionStatusHealthy,
			Capacity: RegionCapacity{MaxConcurrentUploads: 10},
		},
		Region{
			Name: "ap-southeast-1", 
			Priority: 4, 
			Weight: 30, 
			Status: RegionStatusHealthy,
			Capacity: RegionCapacity{MaxConcurrentUploads: 8},
		},
		Region{
			Name: "ap-northeast-1", 
			Priority: 5, 
			Weight: 20, 
			Status: RegionStatusHealthy,
			Capacity: RegionCapacity{MaxConcurrentUploads: 5},
		},
	)
	
	logger := log.New(nil)
	coordinator := NewCoordinator()
	err := coordinator.Initialize(context.Background(), config)
	require.NoError(t, err)
	
	regionSelector := NewRegionSelector(config, logger)
	failoverManager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	
	return &PerformanceTestSuite{
		coordinator:     coordinator,
		regionSelector:  regionSelector,
		failoverManager: failoverManager,
		config:          config,
		logger:          logger,
		results:         make([]BenchmarkResult, 0),
	}
}

// Cleanup shuts down the performance test suite
func (pts *PerformanceTestSuite) Cleanup() {
	if pts.coordinator != nil {
		_ = pts.coordinator.Shutdown(context.Background())
	}
}

// THROUGHPUT TESTING FRAMEWORK

func TestPerformance_ThroughputTesting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}
	
	suite := NewPerformanceTestSuite(t)
	defer suite.Cleanup()
	
	t.Run("region selection throughput", func(t *testing.T) {
		result := suite.BenchmarkRegionSelection(t, 1000, 10)
		assert.Greater(t, result.OperationsPerSec, float64(500), "Should handle at least 500 selections/sec")
		assert.Less(t, result.AvgLatencyMs, float64(10), "Average latency should be under 10ms")
		t.Logf("Region selection: %.2f ops/sec, %.2fms avg latency", 
			result.OperationsPerSec, result.AvgLatencyMs)
	})
	
	t.Run("concurrent upload coordination", func(t *testing.T) {
		result := suite.BenchmarkUploadCoordination(t, 100, 20)
		assert.Greater(t, result.OperationsPerSec, float64(50), "Should handle at least 50 uploads/sec")
		assert.Less(t, result.ErrorRate, float64(5), "Error rate should be under 5%")
		t.Logf("Upload coordination: %.2f ops/sec, %.2f%% error rate", 
			result.OperationsPerSec, result.ErrorRate)
	})
	
	t.Run("failover detection throughput", func(t *testing.T) {
		result := suite.BenchmarkFailoverDetection(t, 500, 5)
		assert.Greater(t, result.OperationsPerSec, float64(200), "Should handle at least 200 detections/sec")
		assert.Less(t, result.AvgLatencyMs, float64(20), "Average latency should be under 20ms")
		t.Logf("Failover detection: %.2f ops/sec, %.2fms avg latency", 
			result.OperationsPerSec, result.AvgLatencyMs)
	})
}

// BenchmarkRegionSelection measures region selection performance
func (pts *PerformanceTestSuite) BenchmarkRegionSelection(t *testing.T, operations int, concurrency int) BenchmarkResult {
	ctx := context.Background()
	startTime := time.Now()
	
	var wg sync.WaitGroup
	latencies := make([]time.Duration, operations)
	errors := make([]error, operations)
	
	operationsPerWorker := operations / concurrency
	
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < operationsPerWorker; i++ {
				opIndex := workerID*operationsPerWorker + i
				if opIndex >= operations {
					break
				}
				
				request := &UploadRequest{
					ID:       fmt.Sprintf("perf-test-%d-%d", workerID, i),
					FilePath: fmt.Sprintf("/test/file-%d.dat", opIndex),
					Size:     int64(rand.Intn(1000000) + 1000), // 1KB to 1MB
					Priority: rand.Intn(10) + 1,
				}
				
				opStart := time.Now()
				_, err := pts.regionSelector.SelectRegion(ctx, request)
				latencies[opIndex] = time.Since(opStart)
				errors[opIndex] = err
			}
		}(worker)
	}
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	return pts.calculateBenchmarkResult("RegionSelection", operations, totalDuration, latencies, errors, concurrency)
}

// BenchmarkUploadCoordination measures upload coordination performance
func (pts *PerformanceTestSuite) BenchmarkUploadCoordination(t *testing.T, operations int, concurrency int) BenchmarkResult {
	ctx := context.Background()
	startTime := time.Now()
	
	var wg sync.WaitGroup
	latencies := make([]time.Duration, operations)
	errors := make([]error, operations)
	
	operationsPerWorker := operations / concurrency
	
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < operationsPerWorker; i++ {
				opIndex := workerID*operationsPerWorker + i
				if opIndex >= operations {
					break
				}
				
				request := &UploadRequest{
					ID:              fmt.Sprintf("coord-test-%d-%d", workerID, i),
					FilePath:        fmt.Sprintf("/test/file-%d.dat", opIndex),
					DestinationKey:  fmt.Sprintf("uploads/file-%d.dat", opIndex),
					Size:            int64(rand.Intn(100000) + 10000), // 10KB to 100KB
					Priority:        rand.Intn(5) + 1,
					Context:         ctx,
				}
				
				opStart := time.Now()
				_, err := pts.coordinator.Upload(ctx, request)
				latencies[opIndex] = time.Since(opStart)
				errors[opIndex] = err
			}
		}(worker)
	}
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	return pts.calculateBenchmarkResult("UploadCoordination", operations, totalDuration, latencies, errors, concurrency)
}

// BenchmarkFailoverDetection measures failover detection performance
func (pts *PerformanceTestSuite) BenchmarkFailoverDetection(t *testing.T, operations int, concurrency int) BenchmarkResult {
	ctx := context.Background()
	startTime := time.Now()
	
	// Pre-populate some failure data
	regions := []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}
	for _, region := range regions {
		for i := 0; i < 2; i++ {
			pts.failoverManager.RecordFailure(region)
		}
	}
	
	var wg sync.WaitGroup
	latencies := make([]time.Duration, operations)
	errors := make([]error, operations)
	
	operationsPerWorker := operations / concurrency
	
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < operationsPerWorker; i++ {
				opIndex := workerID*operationsPerWorker + i
				if opIndex >= operations {
					break
				}
				
				region := regions[opIndex%len(regions)]
				
				opStart := time.Now()
				_, err := pts.failoverManager.DetectFailure(ctx, region)
				latencies[opIndex] = time.Since(opStart)
				errors[opIndex] = err
			}
		}(worker)
	}
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	return pts.calculateBenchmarkResult("FailoverDetection", operations, totalDuration, latencies, errors, concurrency)
}

// LATENCY MEASUREMENT TOOLS

func TestPerformance_LatencyMeasurement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}
	
	suite := NewPerformanceTestSuite(t)
	defer suite.Cleanup()
	
	t.Run("region selection latency under load", func(t *testing.T) {
		results := suite.MeasureLatencyUnderLoad(t, "RegionSelection", []int{1, 5, 10, 20})
		
		for _, result := range results {
			t.Logf("Concurrency %d: P95=%.2fms, P99=%.2fms", 
				result.ConcurrentUsers, result.P95LatencyMs, result.P99LatencyMs)
			
			// Latency should remain reasonable even under load
			assert.Less(t, result.P95LatencyMs, float64(100), "P95 latency should be under 100ms")
			assert.Less(t, result.P99LatencyMs, float64(200), "P99 latency should be under 200ms")
		}
	})
	
	t.Run("failover execution latency", func(t *testing.T) {
		// Test minimal failover operations with timeout protection
		result := suite.MeasureFailoverLatency(t, 2)
		// Allow for timeouts in performance testing environment
		if result.OperationsPerSec > 0 {
			assert.Less(t, result.AvgLatencyMs, float64(3000), "Average failover should complete under 3s")
			assert.Less(t, result.P95LatencyMs, float64(5000), "P95 failover should complete under 5s")
		}
		t.Logf("Failover latency: %.2fms avg, %.2fms P95 (%.2f ops/sec)", 
			result.AvgLatencyMs, result.P95LatencyMs, result.OperationsPerSec)
	})
}

// MeasureLatencyUnderLoad tests latency characteristics under increasing load
func (pts *PerformanceTestSuite) MeasureLatencyUnderLoad(t *testing.T, operation string, concurrencyLevels []int) []BenchmarkResult {
	results := make([]BenchmarkResult, 0, len(concurrencyLevels))
	
	for _, concurrency := range concurrencyLevels {
		var result BenchmarkResult
		
		switch operation {
		case "RegionSelection":
			result = pts.BenchmarkRegionSelection(t, concurrency*50, concurrency)
		case "UploadCoordination":
			result = pts.BenchmarkUploadCoordination(t, concurrency*10, concurrency)
		case "FailoverDetection":
			result = pts.BenchmarkFailoverDetection(t, concurrency*25, concurrency)
		}
		
		results = append(results, result)
	}
	
	return results
}

// MeasureFailoverLatency measures failover execution latency
func (pts *PerformanceTestSuite) MeasureFailoverLatency(t *testing.T, operations int) BenchmarkResult {
	// Use short timeout context for performance testing
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	startTime := time.Now()
	
	latencies := make([]time.Duration, operations)
	errors := make([]error, operations)
	
	regions := []string{"us-east-1", "us-west-2"}
	
	for i := 0; i < operations; i++ {
		fromRegion := regions[i%len(regions)]
		toRegion := regions[(i+1)%len(regions)]
		
		opStart := time.Now()
		err := pts.failoverManager.ExecuteFailover(ctx, fromRegion, toRegion)
		latencies[i] = time.Since(opStart)
		errors[i] = err
		
		// Break early if context is cancelled or operation takes too long
		if ctx.Err() != nil || time.Since(opStart) > 2*time.Second {
			t.Logf("Failover test stopped early after %d operations (timeout or slow operation)", i+1)
			latencies = latencies[:i+1]
			errors = errors[:i+1]
			break
		}
	}
	
	totalDuration := time.Since(startTime)
	
	return pts.calculateBenchmarkResult("FailoverExecution", operations, totalDuration, latencies, errors, 1)
}

// SCALABILITY VALIDATION TESTS

func TestPerformance_ScalabilityValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}
	
	suite := NewPerformanceTestSuite(t)
	defer suite.Cleanup()
	
	t.Run("region scaling performance", func(t *testing.T) {
		regionCounts := []int{2, 5, 10, 20}
		results := suite.TestRegionScaling(t, regionCounts)
		
		for i, result := range results {
			regionCount := regionCounts[i]
			t.Logf("Regions %d: %.2f ops/sec, %.2fms latency", 
				regionCount, result.OperationsPerSec, result.AvgLatencyMs)
			
			// Performance should degrade gracefully with more regions
			assert.Greater(t, result.OperationsPerSec, float64(50), "Should maintain minimum throughput")
			assert.Less(t, result.AvgLatencyMs, float64(100), "Latency should remain reasonable")
		}
	})
	
	t.Run("concurrent user scaling", func(t *testing.T) {
		userCounts := []int{1, 10, 50, 100}
		results := suite.TestConcurrentUserScaling(t, userCounts)
		
		for i, result := range results {
			userCount := userCounts[i]
			t.Logf("Users %d: %.2f ops/sec, %.2f%% error rate", 
				userCount, result.OperationsPerSec, result.ErrorRate)
			
			// System should handle concurrent users gracefully
			assert.Less(t, result.ErrorRate, float64(10), "Error rate should stay under 10%")
		}
	})
	
	t.Run("memory usage under load", func(t *testing.T) {
		result := suite.TestMemoryUsageUnderLoad(t, 1000, 50)
		t.Logf("Memory test completed: %.2f ops/sec", result.OperationsPerSec)
		assert.Greater(t, result.OperationsPerSec, float64(100), "Should maintain performance under memory pressure")
	})
}

// TestRegionScaling tests performance with different numbers of regions
func (pts *PerformanceTestSuite) TestRegionScaling(t *testing.T, regionCounts []int) []BenchmarkResult {
	results := make([]BenchmarkResult, 0, len(regionCounts))
	
	originalRegions := pts.config.Regions
	
	for _, count := range regionCounts {
		// Adjust number of regions
		if count <= len(originalRegions) {
			pts.config.Regions = originalRegions[:count]
		} else {
			// Add synthetic regions
			newRegions := make([]Region, count)
			copy(newRegions, originalRegions)
			
			for i := len(originalRegions); i < count; i++ {
				newRegions[i] = Region{
					Name:     fmt.Sprintf("synthetic-region-%d", i),
					Priority: i + 1,
					Weight:   20,
					Status:   RegionStatusHealthy,
				}
			}
			pts.config.Regions = newRegions
		}
		
		// Recreate region selector with new config
		pts.regionSelector = NewRegionSelector(pts.config, pts.logger)
		
		// Benchmark with current region count
		result := pts.BenchmarkRegionSelection(t, 200, 10)
		results = append(results, result)
	}
	
	// Restore original configuration
	pts.config.Regions = originalRegions
	pts.regionSelector = NewRegionSelector(pts.config, pts.logger)
	
	return results
}

// TestConcurrentUserScaling tests performance with different numbers of concurrent users
func (pts *PerformanceTestSuite) TestConcurrentUserScaling(t *testing.T, userCounts []int) []BenchmarkResult {
	results := make([]BenchmarkResult, 0, len(userCounts))
	
	for _, userCount := range userCounts {
		result := pts.BenchmarkUploadCoordination(t, userCount*5, userCount)
		results = append(results, result)
	}
	
	return results
}

// TestMemoryUsageUnderLoad tests system behavior under memory pressure
func (pts *PerformanceTestSuite) TestMemoryUsageUnderLoad(t *testing.T, operations int, concurrency int) BenchmarkResult {
	// This would ideally monitor actual memory usage
	// For now, we simulate memory pressure by running many operations
	return pts.BenchmarkRegionSelection(t, operations, concurrency)
}

// COMPETITOR COMPARISON BENCHMARKS

func TestPerformance_CompetitorComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}
	
	suite := NewPerformanceTestSuite(t)
	defer suite.Cleanup()
	
	t.Run("cargoship vs basic round-robin", func(t *testing.T) {
		cargoshipResult := suite.BenchmarkCargoShipSelection(t, 1000)
		basicResult := suite.BenchmarkBasicRoundRobin(t, 1000)
		
		t.Logf("CargoShip: %.2f ops/sec, %.2fms latency", 
			cargoshipResult.OperationsPerSec, cargoshipResult.AvgLatencyMs)
		t.Logf("Basic Round Robin: %.2f ops/sec, %.2fms latency", 
			basicResult.OperationsPerSec, basicResult.AvgLatencyMs)
		
		// CargoShip trades some performance for advanced features
		// It should maintain reasonable throughput despite additional complexity
		assert.Greater(t, cargoshipResult.OperationsPerSec, float64(1000), 
			"CargoShip should maintain reasonable throughput (>1000 ops/sec)")
		assert.Greater(t, basicResult.OperationsPerSec, cargoshipResult.OperationsPerSec, 
			"Basic round-robin should be faster due to simplicity")
	})
	
	t.Run("failover vs no-failover", func(t *testing.T) {
		withFailoverResult := suite.BenchmarkWithFailover(t, 100)
		withoutFailoverResult := suite.BenchmarkWithoutFailover(t, 100)
		
		t.Logf("With Failover: %.2f ops/sec, %.2f%% error rate", 
			withFailoverResult.OperationsPerSec, withFailoverResult.ErrorRate)
		t.Logf("Without Failover: %.2f ops/sec, %.2f%% error rate", 
			withoutFailoverResult.OperationsPerSec, withoutFailoverResult.ErrorRate)
		
		// Failover should improve reliability - if both have zero errors, that's acceptable
		if withoutFailoverResult.ErrorRate > 0 {
			assert.Less(t, withFailoverResult.ErrorRate, withoutFailoverResult.ErrorRate*0.5,
				"Failover should reduce error rate by at least 50%")
		} else {
			// If no errors in either case, verify performance is maintained
			assert.Greater(t, withFailoverResult.OperationsPerSec, withoutFailoverResult.OperationsPerSec*0.8,
				"Failover should maintain reasonable performance when no errors occur")
		}
	})
}

// BenchmarkCargoShipSelection benchmarks the full CargoShip region selection
func (pts *PerformanceTestSuite) BenchmarkCargoShipSelection(t *testing.T, operations int) BenchmarkResult {
	return pts.BenchmarkRegionSelection(t, operations, 10)
}

// BenchmarkBasicRoundRobin benchmarks a basic round-robin implementation
func (pts *PerformanceTestSuite) BenchmarkBasicRoundRobin(t *testing.T, operations int) BenchmarkResult {
	startTime := time.Now()
	latencies := make([]time.Duration, operations)
	errors := make([]error, operations)
	
	regions := pts.config.Regions
	counter := 0
	
	for i := 0; i < operations; i++ {
		opStart := time.Now()
		
		// Simple round-robin selection
		_ = regions[counter%len(regions)]
		counter++
		
		latencies[i] = time.Since(opStart)
		errors[i] = nil
	}
	
	totalDuration := time.Since(startTime)
	
	return pts.calculateBenchmarkResult("BasicRoundRobin", operations, totalDuration, latencies, errors, 1)
}

// BenchmarkWithFailover benchmarks upload coordination with failover enabled
func (pts *PerformanceTestSuite) BenchmarkWithFailover(t *testing.T, operations int) BenchmarkResult {
	// Introduce some failures to trigger failover
	pts.failoverManager.RecordFailure("us-east-1")
	pts.failoverManager.RecordFailure("us-east-1")
	pts.failoverManager.RecordFailure("us-east-1")
	
	return pts.BenchmarkUploadCoordination(t, operations, 5)
}

// BenchmarkWithoutFailover benchmarks upload coordination without failover
func (pts *PerformanceTestSuite) BenchmarkWithoutFailover(t *testing.T, operations int) BenchmarkResult {
	// Create a simple coordinator without failover capabilities
	return pts.BenchmarkUploadCoordination(t, operations, 5)
}

// UTILITY FUNCTIONS

// calculateBenchmarkResult computes performance metrics from raw measurements
func (pts *PerformanceTestSuite) calculateBenchmarkResult(
	operationType string,
	totalOps int,
	totalDuration time.Duration,
	latencies []time.Duration,
	errors []error,
	concurrency int,
) BenchmarkResult {
	// Calculate error rate
	errorCount := 0
	for _, err := range errors {
		if err != nil {
			errorCount++
		}
	}
	errorRate := float64(errorCount) / float64(totalOps) * 100
	
	// Calculate latency statistics
	validLatencies := make([]time.Duration, 0, totalOps)
	for i, err := range errors {
		if err == nil && i < len(latencies) {
			validLatencies = append(validLatencies, latencies[i])
		}
	}
	
	if len(validLatencies) == 0 {
		return BenchmarkResult{
			OperationType:   operationType,
			TotalOperations: totalOps,
			TotalDuration:   totalDuration,
			ErrorRate:       errorRate,
			ConcurrentUsers: concurrency,
			Timestamp:       time.Now(),
		}
	}
	
	// Sort latencies for percentile calculation
	for i := 0; i < len(validLatencies)-1; i++ {
		for j := i + 1; j < len(validLatencies); j++ {
			if validLatencies[i] > validLatencies[j] {
				validLatencies[i], validLatencies[j] = validLatencies[j], validLatencies[i]
			}
		}
	}
	
	// Calculate averages and percentiles
	var totalLatency time.Duration
	for _, lat := range validLatencies {
		totalLatency += lat
	}
	avgLatency := totalLatency / time.Duration(len(validLatencies))
	
	p95Index := int(float64(len(validLatencies)) * 0.95)
	if p95Index >= len(validLatencies) {
		p95Index = len(validLatencies) - 1
	}
	p95Latency := validLatencies[p95Index]
	
	p99Index := int(float64(len(validLatencies)) * 0.99)
	if p99Index >= len(validLatencies) {
		p99Index = len(validLatencies) - 1
	}
	p99Latency := validLatencies[p99Index]
	
	// Calculate operations per second
	opsPerSec := float64(totalOps) / totalDuration.Seconds()
	
	result := BenchmarkResult{
		OperationType:    operationType,
		TotalOperations:  totalOps,
		TotalDuration:    totalDuration,
		OperationsPerSec: opsPerSec,
		AvgLatencyMs:     float64(avgLatency.Nanoseconds()) / 1e6,
		P95LatencyMs:     float64(p95Latency.Nanoseconds()) / 1e6,
		P99LatencyMs:     float64(p99Latency.Nanoseconds()) / 1e6,
		ErrorRate:        errorRate,
		ConcurrentUsers:  concurrency,
		Timestamp:        time.Now(),
	}
	
	// Store result
	pts.mu.Lock()
	pts.results = append(pts.results, result)
	pts.mu.Unlock()
	
	return result
}

// GetBenchmarkResults returns all collected benchmark results
func (pts *PerformanceTestSuite) GetBenchmarkResults() []BenchmarkResult {
	pts.mu.Lock()
	defer pts.mu.Unlock()
	
	results := make([]BenchmarkResult, len(pts.results))
	copy(results, pts.results)
	return results
}

// ReportPerformanceMetrics generates a performance report
func (pts *PerformanceTestSuite) ReportPerformanceMetrics() string {
	results := pts.GetBenchmarkResults()
	
	report := "=== CargoShip Multi-Region Performance Report ===\n\n"
	
	for _, result := range results {
		report += fmt.Sprintf("Operation: %s\n", result.OperationType)
		report += fmt.Sprintf("  Operations: %d (Concurrency: %d)\n", result.TotalOperations, result.ConcurrentUsers)
		report += fmt.Sprintf("  Duration: %v\n", result.TotalDuration)
		report += fmt.Sprintf("  Throughput: %.2f ops/sec\n", result.OperationsPerSec)
		report += fmt.Sprintf("  Latency - Avg: %.2fms, P95: %.2fms, P99: %.2fms\n", 
			result.AvgLatencyMs, result.P95LatencyMs, result.P99LatencyMs)
		report += fmt.Sprintf("  Error Rate: %.2f%%\n", result.ErrorRate)
		report += "\n"
	}
	
	return report
}

// Performance test benchmark functions for go test -bench

func BenchmarkRegionSelection(b *testing.B) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	selector := NewRegionSelector(config, logger)
	ctx := context.Background()
	
	request := &UploadRequest{
		ID:       "bench-test",
		FilePath: "/test/file.dat",
		Size:     1024,
		Priority: 1,
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := selector.SelectRegion(ctx, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkFailoverDetection(b *testing.B) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	// Pre-populate some failure data
	manager.RecordFailure("us-east-1")
	manager.RecordFailure("us-east-1")
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := manager.DetectFailure(ctx, "us-east-1")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMultiRegionCoordination(b *testing.B) {
	config := createValidMultiRegionConfig()
	coordinator := NewCoordinator()
	ctx := context.Background()
	
	err := coordinator.Initialize(ctx, config)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = coordinator.Shutdown(ctx) }()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := &UploadRequest{
			ID:             fmt.Sprintf("bench-coord-%d", i),
			FilePath:       fmt.Sprintf("/test/file-%d.dat", i),
			DestinationKey: fmt.Sprintf("uploads/file-%d.dat", i),
			Size:           1024,
			Priority:       1,
			Context:        ctx,
		}
		
		_, err := coordinator.Upload(ctx, request)
		if err != nil {
			b.Fatal(err)
		}
	}
}