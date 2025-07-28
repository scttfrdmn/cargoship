//go:build integration
// +build integration

package s3

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

const (
	extremeTestProfile = "aws"
	extremeTestRegion  = "us-west-2"
	extremeTestBucket  = "cargoship-extreme-test"
	extremeTestPrefix  = "extreme-test"
)

// TestExtremePerformance pushes CargoShip to its absolute limits
func TestExtremePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping extreme performance test in short mode")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(extremeTestProfile),
		config.WithRegion(extremeTestRegion),
	)
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg)
	
	// Ensure test bucket exists
	err = ensureStressBucket(ctx, s3Client, extremeTestBucket)
	require.NoError(t, err)

	t.Run("MaximumConcurrencyPush", func(t *testing.T) {
		testMaximumConcurrencyPush(t, ctx, s3Client, logger)
	})

	t.Run("SustainedHighThroughput", func(t *testing.T) {
		testSustainedHighThroughput(t, ctx, s3Client, logger)
	})

	t.Run("BurstCapacityTest", func(t *testing.T) {
		testBurstCapacity(t, ctx, s3Client, logger)
	})

	// Cleanup
	t.Cleanup(func() {
		cleanupStressObjects(ctx, s3Client, extremeTestBucket, extremeTestPrefix)
	})
}

// testMaximumConcurrencyPush tests with extreme concurrency
func testMaximumConcurrencyPush(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// Maximum performance configuration
	s3Config := awsconfig.S3Config{
		Bucket:              extremeTestBucket,
		Concurrency:         200, // Maximum concurrency
		MultipartChunkSize:  512 * 1024 * 1024, // 512MB chunks
		MultipartThreshold:  1024 * 1024 * 1024, // 1GB threshold
	}

	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Test extreme concurrency
	concurrency := 200
	fileSize := 25 * 1024 * 1024 // 25MB per file

	t.Logf("🚀 EXTREME TEST: %d concurrent uploads of 25MB each (5GB total)...", concurrency)
	t.Logf("⚙️  Configuration: Concurrency=%d, ChunkSize=%dMB", s3Config.Concurrency, s3Config.MultipartChunkSize/(1024*1024))
	
	var wg sync.WaitGroup
	var successCount int64
	var totalBytes int64
	var totalLatencyNs int64
	
	// Track throughput samples for analysis
	var mu sync.Mutex
	var throughputSamples []float64
	
	startTime := time.Now()
	
	// Launch massive concurrent upload burst
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(uploadID int) {
			defer wg.Done()
			
			testData := strings.Repeat(fmt.Sprintf("EXTREME performance test %d pushing CargoShip to limits. ", uploadID), fileSize/100)
			testKey := fmt.Sprintf("%s/extreme-concurrent-%d-%d.txt", extremeTestPrefix, time.Now().Unix(), uploadID)
			
			archive := &Archive{
				Key:             testKey,
				Reader:          strings.NewReader(testData),
				Size:            int64(len(testData)),
				StorageClass:    awsconfig.StorageClassStandard,
				Metadata:        map[string]string{
					"test": "extreme-concurrency",
					"upload_id": fmt.Sprintf("%d", uploadID),
					"target": "maximum-performance",
				},
			}

			uploadStart := time.Now()
			_, err := transporter.Upload(ctx, archive)
			uploadDuration := time.Since(uploadStart)
			
			if err == nil {
				throughputMBps := float64(archive.Size) / (1024 * 1024) / uploadDuration.Seconds()
				
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalBytes, archive.Size)
				atomic.AddInt64(&totalLatencyNs, uploadDuration.Nanoseconds())
				
				mu.Lock()
				throughputSamples = append(throughputSamples, throughputMBps)
				mu.Unlock()
				
				if uploadID%25 == 0 { // Log every 25th upload
					t.Logf("  Upload %d: %.2f MB/s", uploadID, throughputMBps)
				}
			} else {
				t.Logf("  Upload %d FAILED: %v", uploadID, err)
			}
		}(i)
	}
	
	wg.Wait()
	overallDuration := time.Since(startTime)
	
	// Calculate comprehensive metrics
	success := atomic.LoadInt64(&successCount)
	bytes := atomic.LoadInt64(&totalBytes)
	avgLatencyNs := atomic.LoadInt64(&totalLatencyNs) / success
	
	aggregateThroughputMBps := float64(bytes) / (1024 * 1024) / overallDuration.Seconds()
	avgThroughputMBps := float64(fileSize) / (1024 * 1024) / (float64(avgLatencyNs) / 1e9)
	bandwidthGbps := aggregateThroughputMBps * 8 / 1000
	efficiency := (bandwidthGbps / 5.0) * 100
	
	// Calculate throughput statistics
	mu.Lock()
	var minThroughput, maxThroughput, totalThroughput float64 = 999999, 0, 0
	for _, sample := range throughputSamples {
		if sample < minThroughput { minThroughput = sample }
		if sample > maxThroughput { maxThroughput = sample }
		totalThroughput += sample
	}
	mu.Unlock()
	
	t.Logf("🏆 EXTREME CONCURRENCY RESULTS:")
	t.Logf("  Target concurrency: %d uploads", concurrency)
	t.Logf("  Successful uploads: %d/%d (%.1f%%)", success, concurrency, float64(success)/float64(concurrency)*100)
	t.Logf("  Total data transferred: %.2f GB", float64(bytes)/(1024*1024*1024))
	t.Logf("  Overall duration: %s", overallDuration)
	t.Logf("  Aggregate throughput: %.2f MB/s", aggregateThroughputMBps)
	t.Logf("  Network utilization: %.3f Gbps (%.1f%% of 5 Gbps)", bandwidthGbps, efficiency)
	t.Logf("  Average upload throughput: %.2f MB/s", avgThroughputMBps)
	t.Logf("  Throughput range: %.2f - %.2f MB/s", minThroughput, maxThroughput)
	t.Logf("  Average latency: %.2f seconds", float64(avgLatencyNs)/1e9)
	
	// Get optimization statistics
	stats := transporter.GetOptimizationStats()
	t.Logf("🔧 Optimization Statistics:")
	t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
	t.Logf("  Total optimizations: %d", stats.TotalOptimizations)
	t.Logf("  BBR activations: %d", stats.BBRActivations)
	t.Logf("  CUBIC adjustments: %d", stats.CubicAdjustments)
	
	// Performance assertions for extreme test
	assert.Greater(t, float64(success)/float64(concurrency), 0.85, "At least 85% success rate under extreme load")
	assert.Greater(t, aggregateThroughputMBps, 100.0, "Should achieve >100 MB/s aggregate under extreme load")
	
	if aggregateThroughputMBps > 300 {
		t.Logf("🏆🏆🏆 PHENOMENAL: >300 MB/s aggregate throughput!")
	} else if aggregateThroughputMBps > 200 {
		t.Logf("🏆🏆 OUTSTANDING: >200 MB/s aggregate throughput!")
	} else if aggregateThroughputMBps > 150 {
		t.Logf("🏆 EXCELLENT: >150 MB/s aggregate throughput!")
	}
}

// testSustainedHighThroughput tests sustained high performance
func testSustainedHighThroughput(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// High-performance sustained configuration
	s3Config := awsconfig.S3Config{
		Bucket:              extremeTestBucket,
		Concurrency:         100,
		MultipartChunkSize:  256 * 1024 * 1024, // 256MB chunks
		MultipartThreshold:  500 * 1024 * 1024, // 500MB threshold
	}

	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Sustained test parameters
	testDuration := 3 * time.Minute // 3 minutes of sustained performance
	batchSize := 15 // 15 concurrent uploads per batch
	fileSize := 30 * 1024 * 1024 // 30MB files
	batchInterval := 15 * time.Second // New batch every 15 seconds

	t.Logf("🔥 SUSTAINED HIGH THROUGHPUT TEST:")
	t.Logf("  Duration: %s", testDuration)
	t.Logf("  Pattern: %d concurrent 30MB uploads every %s", batchSize, batchInterval)
	t.Logf("  Target: Sustained high performance over time")

	var batchCount int64
	var totalUploads int64
	var totalBytes int64
	var totalThroughput float64
	var mu sync.Mutex
	var performanceSamples []PerformanceSample

	ctx, cancel := context.WithTimeout(ctx, testDuration)
	defer cancel()

	startTime := time.Now()
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	batchID := 0

	// Sustained performance loop
	sustainedLoop := func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				batchID++
				atomic.AddInt64(&batchCount, 1)
				
				go func(bid int) {
					batchStart := time.Now()
					var wg sync.WaitGroup
					var batchBytes int64
					var batchThroughput float64
					var batchMu sync.Mutex
					
					// Launch batch of concurrent uploads
					for i := 0; i < batchSize; i++ {
						wg.Add(1)
						go func(uploadID int) {
							defer wg.Done()
							
							testData := strings.Repeat(fmt.Sprintf("Sustained test B%d-U%d. ", bid, uploadID), fileSize/50)
							testKey := fmt.Sprintf("%s/sustained-%d-%d-%d.txt", extremeTestPrefix, time.Now().Unix(), bid, uploadID)
							
							archive := &Archive{
								Key:             testKey,
								Reader:          strings.NewReader(testData),
								Size:            int64(len(testData)),
								StorageClass:    awsconfig.StorageClassStandard,
							}

							uploadStart := time.Now()
							_, err := transporter.Upload(ctx, archive)
							uploadDuration := time.Since(uploadStart)
							
							if err == nil {
								throughputMBps := float64(archive.Size) / (1024 * 1024) / uploadDuration.Seconds()
								
								atomic.AddInt64(&totalUploads, 1)
								atomic.AddInt64(&totalBytes, archive.Size)
								
								batchMu.Lock()
								batchBytes += archive.Size
								batchThroughput += throughputMBps
								batchMu.Unlock()
							}
						}(i)
					}
					
					wg.Wait()
					batchDuration := time.Since(batchStart)
					
					batchAggregateThroughput := float64(batchBytes) / (1024 * 1024) / batchDuration.Seconds()
					
					mu.Lock()
					totalThroughput += batchThroughput
					performanceSamples = append(performanceSamples, PerformanceSample{
						Timestamp: time.Now(),
						BatchID: bid,
						AggregateThroughput: batchAggregateThroughput,
						AverageThroughput: batchThroughput / float64(batchSize),
						Duration: batchDuration,
					})
					mu.Unlock()
					
					t.Logf("  Batch %d: %.2f MB/s aggregate (%.2f MB/s avg per upload)", bid, batchAggregateThroughput, batchThroughput/float64(batchSize))
				}(batchID)
			}
		}
	}

	go sustainedLoop()
	<-ctx.Done()
	time.Sleep(10 * time.Second) // Allow final batches to complete

	actualDuration := time.Since(startTime)
	batches := atomic.LoadInt64(&batchCount)
	uploads := atomic.LoadInt64(&totalUploads)
	bytes := atomic.LoadInt64(&totalBytes)

	overallThroughput := float64(bytes) / (1024 * 1024) / actualDuration.Seconds()
	
	// Analyze performance consistency
	mu.Lock()
	var minBatchThroughput, maxBatchThroughput, totalBatchThroughput float64 = 999999, 0, 0
	for _, sample := range performanceSamples {
		if sample.AggregateThroughput < minBatchThroughput { minBatchThroughput = sample.AggregateThroughput }
		if sample.AggregateThroughput > maxBatchThroughput { maxBatchThroughput = sample.AggregateThroughput }
		totalBatchThroughput += sample.AggregateThroughput
	}
	avgBatchThroughput := totalBatchThroughput / float64(len(performanceSamples))
	throughputVariance := maxBatchThroughput - minBatchThroughput
	consistency := (1.0 - throughputVariance/avgBatchThroughput) * 100
	mu.Unlock()

	t.Logf("🔥 SUSTAINED THROUGHPUT RESULTS:")
	t.Logf("  Test duration: %s", actualDuration)
	t.Logf("  Batches completed: %d", batches)
	t.Logf("  Total uploads: %d", uploads)
	t.Logf("  Total data: %.2f GB", float64(bytes)/(1024*1024*1024))
	t.Logf("  Overall sustained throughput: %.2f MB/s", overallThroughput)
	t.Logf("  Average batch throughput: %.2f MB/s", avgBatchThroughput)
	t.Logf("  Throughput range: %.2f - %.2f MB/s", minBatchThroughput, maxBatchThroughput)
	t.Logf("  Performance consistency: %.1f%%", consistency)
	t.Logf("  Upload rate: %.1f uploads/minute", float64(uploads)/actualDuration.Minutes())

	// Performance assertions
	assert.Greater(t, uploads, int64(30), "Should complete at least 30 uploads")
	assert.Greater(t, overallThroughput, 50.0, "Sustained throughput should exceed 50 MB/s")
	assert.Greater(t, consistency, 70.0, "Performance should be reasonably consistent")

	if overallThroughput > 150 {
		t.Logf("🏆🏆 EXCEPTIONAL sustained performance: >150 MB/s!")
	} else if overallThroughput > 100 {
		t.Logf("🏆 OUTSTANDING sustained performance: >100 MB/s!")
	} else if overallThroughput > 75 {
		t.Logf("🎯 EXCELLENT sustained performance: >75 MB/s!")
	}
}

// testBurstCapacity tests burst upload capacity
func testBurstCapacity(t *testing.T, ctx context.Context, s3Client *s3.Client, logger *slog.Logger) {
	// Burst configuration
	s3Config := awsconfig.S3Config{
		Bucket:              extremeTestBucket,
		Concurrency:         150,
		MultipartChunkSize:  128 * 1024 * 1024, // 128MB chunks for burst
		MultipartThreshold:  256 * 1024 * 1024, // 256MB threshold
	}

	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Burst test: rapid succession of uploads
	burstCount := 50
	fileSize := 20 * 1024 * 1024 // 20MB files
	burstInterval := 100 * time.Millisecond // Very rapid

	t.Logf("⚡ BURST CAPACITY TEST:")
	t.Logf("  Burst count: %d uploads", burstCount)
	t.Logf("  File size: %d MB each", fileSize/(1024*1024))
	t.Logf("  Burst interval: %s", burstInterval)
	t.Logf("  Testing rapid-fire upload capability")

	var successCount int64
	var totalBytes int64
	var minLatency, maxLatency time.Duration = time.Hour, 0
	var totalLatency time.Duration

	startTime := time.Now()

	// Launch burst uploads with minimal delay
	for i := 0; i < burstCount; i++ {
		go func(uploadID int) {
			testData := strings.Repeat(fmt.Sprintf("Burst test %d rapid upload. ", uploadID), fileSize/50)
			testKey := fmt.Sprintf("%s/burst-%d-%d.txt", extremeTestPrefix, time.Now().UnixNano(), uploadID)
			
			archive := &Archive{
				Key:             testKey,
				Reader:          strings.NewReader(testData),
				Size:            int64(len(testData)),
				StorageClass:    awsconfig.StorageClassStandard,
			}

			uploadStart := time.Now()
			_, err := transporter.Upload(ctx, archive)
			uploadLatency := time.Since(uploadStart)
			
			if err == nil {
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalBytes, archive.Size)
				
				// Track latency statistics
				if uploadLatency < minLatency { minLatency = uploadLatency }
				if uploadLatency > maxLatency { maxLatency = uploadLatency }
				totalLatency += uploadLatency
			}
		}(i)
		
		time.Sleep(burstInterval) // Minimal delay between launches
	}

	// Wait for all uploads to complete (with timeout)
	waitStart := time.Now()
	for atomic.LoadInt64(&successCount) < int64(burstCount) && time.Since(waitStart) < 2*time.Minute {
		time.Sleep(100 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)
	success := atomic.LoadInt64(&successCount)
	bytes := atomic.LoadInt64(&totalBytes)
	
	avgLatency := totalLatency / time.Duration(success)
	burstThroughput := float64(bytes) / (1024 * 1024) / totalDuration.Seconds()
	
	t.Logf("⚡ BURST CAPACITY RESULTS:")
	t.Logf("  Launched: %d uploads", burstCount)
	t.Logf("  Completed: %d uploads (%.1f%%)", success, float64(success)/float64(burstCount)*100)
	t.Logf("  Total data: %.2f GB", float64(bytes)/(1024*1024*1024))
	t.Logf("  Total duration: %s", totalDuration)
	t.Logf("  Burst throughput: %.2f MB/s", burstThroughput)
	t.Logf("  Average latency: %s", avgLatency)
	t.Logf("  Latency range: %s - %s", minLatency, maxLatency)
	t.Logf("  Upload completion rate: %.1f uploads/second", float64(success)/totalDuration.Seconds())

	// Performance assertions
	assert.Greater(t, float64(success)/float64(burstCount), 0.9, "Burst should achieve >90% success rate")
	assert.Greater(t, burstThroughput, 40.0, "Burst throughput should exceed 40 MB/s")

	if burstThroughput > 120 {
		t.Logf("🏆🏆 PHENOMENAL burst capacity: >120 MB/s!")
	} else if burstThroughput > 80 {
		t.Logf("🏆 OUTSTANDING burst capacity: >80 MB/s!")
	} else if burstThroughput > 60 {
		t.Logf("🎯 EXCELLENT burst capacity: >60 MB/s!")
	}
}

// PerformanceSample represents a performance measurement sample
type PerformanceSample struct {
	Timestamp           time.Time
	BatchID             int
	AggregateThroughput float64
	AverageThroughput   float64
	Duration            time.Duration
}