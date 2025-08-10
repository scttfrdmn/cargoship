//go:build performance
// +build performance

package s3

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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
	stressTestProfile = "aws"
	stressTestRegion  = "us-west-2"
	stressTestBucket  = "cargoship-stress-test"
	stressTestPrefix  = "stress-test"
)

// TestCargoShipStressTest exercises CargoShip S3 optimization under extreme load
func TestCargoShipStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(stressTestProfile),
		config.WithRegion(stressTestRegion),
	)
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg)

	// Ensure test bucket exists
	err = ensureStressBucket(ctx, s3Client, stressTestBucket)
	require.NoError(t, err)

	// High-performance S3 config for stress testing
	s3Config := awsconfig.S3Config{
		Bucket:             stressTestBucket,
		Concurrency:        50,                // Max concurrency for stress testing
		MultipartChunkSize: 256 * 1024 * 1024, // 256MB chunks
		MultipartThreshold: 500 * 1024 * 1024, // 500MB threshold
	}

	t.Run("MaxConcurrencyStressTest", func(t *testing.T) {
		testMaxConcurrencyStress(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("LargeFileStressTest", func(t *testing.T) {
		testLargeFileStress(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("SustainedThroughputTest", func(t *testing.T) {
		testSustainedThroughput(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("BandwidthSaturationTest", func(t *testing.T) {
		testBandwidthSaturation(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("MultiGBTransferTest", func(t *testing.T) {
		testMultiGBTransfer(t, ctx, s3Client, s3Config, logger)
	})

	// Cleanup
	t.Cleanup(func() {
		cleanupStressObjects(ctx, s3Client, stressTestBucket, stressTestPrefix)
	})
}

// testMaxConcurrencyStress tests maximum concurrent upload capacity
func testMaxConcurrencyStress(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Test with increasing concurrency levels
	concurrencyLevels := []int{10, 25, 50, 75, 100}
	fileSize := 10 * 1024 * 1024 // 10MB per file

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(t *testing.T) {
			t.Logf("🚀 Testing %d concurrent uploads of 10MB each...", concurrency)

			var wg sync.WaitGroup
			var successCount int64
			var totalBytes int64
			var totalDuration int64

			startTime := time.Now()

			// Launch concurrent uploads
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(uploadID int) {
					defer wg.Done()

					testData := strings.Repeat(fmt.Sprintf("Stress test data %d for maximum concurrency validation. ", uploadID), fileSize/100)
					testKey := fmt.Sprintf("%s/max-concurrency-%d-%d-%d.txt", stressTestPrefix, concurrency, time.Now().Unix(), uploadID)

					archive := &Archive{
						Key:          testKey,
						Reader:       strings.NewReader(testData),
						Size:         int64(len(testData)),
						StorageClass: awsconfig.StorageClassStandard,
						Metadata: map[string]string{
							"test":        "max-concurrency",
							"concurrency": fmt.Sprintf("%d", concurrency),
							"upload_id":   fmt.Sprintf("%d", uploadID),
						},
					}

					uploadStart := time.Now()
					_, err := transporter.Upload(ctx, archive)
					uploadDuration := time.Since(uploadStart)

					if err == nil {
						atomic.AddInt64(&successCount, 1)
						atomic.AddInt64(&totalBytes, archive.Size)
						atomic.AddInt64(&totalDuration, uploadDuration.Nanoseconds())
					} else {
						t.Logf("Upload %d failed: %v", uploadID, err)
					}
				}(i)
			}

			wg.Wait()
			overallDuration := time.Since(startTime)

			success := atomic.LoadInt64(&successCount)
			bytes := atomic.LoadInt64(&totalBytes)
			avgDurationNs := atomic.LoadInt64(&totalDuration) / success

			aggregateThroughputMBps := float64(bytes) / (1024 * 1024) / overallDuration.Seconds()
			avgThroughputMBps := float64(fileSize) / (1024 * 1024) / (float64(avgDurationNs) / 1e9)

			t.Logf("📊 Concurrency %d Results:", concurrency)
			t.Logf("  Successful uploads: %d/%d (%.1f%%)", success, concurrency, float64(success)/float64(concurrency)*100)
			t.Logf("  Total data: %.2f MB", float64(bytes)/(1024*1024))
			t.Logf("  Overall duration: %s", overallDuration)
			t.Logf("  Aggregate throughput: %.2f MB/s (%.3f Gbps)", aggregateThroughputMBps, aggregateThroughputMBps*8/1000)
			t.Logf("  Average per-upload throughput: %.2f MB/s", avgThroughputMBps)
			t.Logf("  Network utilization: %.1f%% of 5 Gbps", (aggregateThroughputMBps*8/1000)/5.0*100)

			// Performance assertions
			assert.Greater(t, float64(success)/float64(concurrency), 0.8, "At least 80% of uploads should succeed")
			assert.Greater(t, aggregateThroughputMBps, 20.0, "Aggregate throughput should exceed 20 MB/s")

			if aggregateThroughputMBps > 100 {
				t.Logf("🏆 Outstanding performance: >100 MB/s aggregate throughput!")
			} else if aggregateThroughputMBps > 50 {
				t.Logf("🎯 Excellent performance: >50 MB/s aggregate throughput!")
			}
		})
	}

	// Get final optimization stats
	stats := transporter.GetOptimizationStats()
	t.Logf("🔧 Final Optimization Statistics:")
	t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
	t.Logf("  Total optimizations: %d", stats.TotalOptimizations)
	t.Logf("  Optimization effective: %t", stats.IsOptimizationEffective())
}

// testLargeFileStress tests with very large files from astrapi.local
func testLargeFileStress(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Look for the largest files available in astrapi.local
	publicPaths := []string{
		"/Volumes/Public",
		"/mnt/astrapi-public",
		"//astrapi.local/Public",
		"/media/astrapi/Public",
	}

	var largeFiles []FileInfo

	for _, basePath := range publicPaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		t.Logf("🔍 Scanning %s for large files...", basePath)
		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Look for files larger than 100MB
			if !info.IsDir() && info.Size() > 100*1024*1024 {
				largeFiles = append(largeFiles, FileInfo{
					Path: path,
					Size: info.Size(),
					Name: info.Name(),
				})
			}
			return nil
		})

		if len(largeFiles) > 0 {
			break
		}
	}

	if len(largeFiles) == 0 {
		t.Skip("No large files (>100MB) found in astrapi.local for stress testing")
	}

	// Sort by size descending to test largest files first
	sort.Slice(largeFiles, func(i, j int) bool {
		return largeFiles[i].Size > largeFiles[j].Size
	})

	// Test up to 3 largest files
	maxFiles := 3
	if len(largeFiles) < maxFiles {
		maxFiles = len(largeFiles)
	}

	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	for i := 0; i < maxFiles; i++ {
		fileInfo := largeFiles[i]
		t.Run(fmt.Sprintf("LargeFile_%d_%.1fMB", i+1, float64(fileInfo.Size)/(1024*1024)), func(t *testing.T) {
			t.Logf("🎯 Testing large file: %s", fileInfo.Path)
			t.Logf("📏 File size: %.2f MB (%.3f GB)", float64(fileInfo.Size)/(1024*1024), float64(fileInfo.Size)/(1024*1024*1024))

			file, err := os.Open(fileInfo.Path)
			require.NoError(t, err)
			defer file.Close()

			testKey := fmt.Sprintf("%s/large-stress-%d-%s", stressTestPrefix, time.Now().Unix(), filepath.Base(fileInfo.Path))

			archive := &Archive{
				Key:          testKey,
				Reader:       file,
				Size:         fileInfo.Size,
				StorageClass: awsconfig.StorageClassStandard,
				Metadata: map[string]string{
					"test":          "large-file-stress",
					"source":        "astrapi.local",
					"original_path": fileInfo.Path,
					"size_mb":       fmt.Sprintf("%.2f", float64(fileInfo.Size)/(1024*1024)),
				},
			}

			t.Logf("🚀 Starting large file upload...")
			startTime := time.Now()
			result, err := transporter.Upload(ctx, archive)
			duration := time.Since(startTime)

			require.NoError(t, err)

			// Calculate comprehensive metrics
			sizeMB := float64(fileInfo.Size) / (1024 * 1024)
			sizeGB := sizeMB / 1024
			throughputMBps := sizeMB / duration.Seconds()
			throughputGbps := throughputMBps * 8 / 1000
			efficiency := (throughputGbps / 5.0) * 100 // Against 5Gbps internet

			t.Logf("📊 Large File Upload Results:")
			t.Logf("  File size: %.2f MB (%.3f GB)", sizeMB, sizeGB)
			t.Logf("  Upload duration: %s", duration)
			t.Logf("  Average throughput: %.2f MB/s", throughputMBps)
			t.Logf("  Network utilization: %.1f Mbps (%.3f Gbps)", throughputMBps*8, throughputGbps)
			t.Logf("  Bandwidth efficiency: %.1f%% of 5 Gbps", efficiency)
			t.Logf("  Result throughput: %.2f MB/s", result.Throughput)

			// Performance assertions for large files
			assert.Greater(t, throughputMBps, 30.0, "Large file should achieve >30 MB/s")
			assert.Greater(t, efficiency, 5.0, "Should utilize >5% of available bandwidth")

			if throughputMBps > 100 {
				t.Logf("🏆 Exceptional large file performance: >100 MB/s!")
			} else if throughputMBps > 60 {
				t.Logf("🎯 Excellent large file performance: >60 MB/s!")
			}

			// Test download performance
			t.Logf("🔄 Testing large file download performance...")
			downloadStart := time.Now()
			reader, err := transporter.Download(ctx, testKey)
			require.NoError(t, err)

			// Stream download with progress tracking for large files
			buffer := make([]byte, 4*1024*1024) // 4MB buffer for large files
			totalDownloaded := int64(0)
			lastProgress := time.Now()

			for {
				n, err := reader.Read(buffer)
				totalDownloaded += int64(n)

				// Progress every 10 seconds for large files
				if time.Since(lastProgress) > 10*time.Second {
					progress := float64(totalDownloaded) / float64(fileInfo.Size) * 100
					currentThroughput := float64(totalDownloaded) / (1024 * 1024) / time.Since(downloadStart).Seconds()
					t.Logf("  📥 Download progress: %.1f%% (%.2f MB/s)", progress, currentThroughput)
					lastProgress = time.Now()
				}

				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			reader.Close()

			downloadDuration := time.Since(downloadStart)
			downloadThroughputMBps := float64(totalDownloaded) / (1024 * 1024) / downloadDuration.Seconds()

			t.Logf("📥 Large File Download Results:")
			t.Logf("  Downloaded: %.2f MB", float64(totalDownloaded)/(1024*1024))
			t.Logf("  Duration: %s", downloadDuration)
			t.Logf("  Throughput: %.2f MB/s", downloadThroughputMBps)

			// Cleanup large file
			err = transporter.DeleteObject(ctx, testKey)
			require.NoError(t, err)
		})
	}
}

// testSustainedThroughput tests sustained throughput over time
func testSustainedThroughput(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Test sustained throughput for 5 minutes
	testDuration := 5 * time.Minute
	fileSize := 20 * 1024 * 1024       // 20MB files
	uploadInterval := 10 * time.Second // Upload every 10 seconds

	t.Logf("🔄 Testing sustained throughput for %s...", testDuration)
	t.Logf("📊 Upload pattern: 20MB files every 10 seconds")

	var uploadCount int64
	var totalBytes int64
	var totalThroughput float64
	var mu sync.Mutex
	var throughputSamples []float64

	startTime := time.Now()
	ticker := time.NewTicker(uploadInterval)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(ctx, testDuration)
	defer cancel()

	// Sustained upload loop
	go func() {
		uploadID := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				uploadID++
				go func(id int) {
					testData := strings.Repeat(fmt.Sprintf("Sustained throughput test %d. ", id), fileSize/50)
					testKey := fmt.Sprintf("%s/sustained-%d-%d.txt", stressTestPrefix, time.Now().Unix(), id)

					archive := &Archive{
						Key:          testKey,
						Reader:       strings.NewReader(testData),
						Size:         int64(len(testData)),
						StorageClass: awsconfig.StorageClassStandard,
						Metadata: map[string]string{
							"test":      "sustained-throughput",
							"upload_id": fmt.Sprintf("%d", id),
						},
					}

					uploadStart := time.Now()
					_, err := transporter.Upload(ctx, archive)
					uploadDuration := time.Since(uploadStart)

					if err == nil {
						throughputMBps := float64(archive.Size) / (1024 * 1024) / uploadDuration.Seconds()

						mu.Lock()
						atomic.AddInt64(&uploadCount, 1)
						atomic.AddInt64(&totalBytes, archive.Size)
						totalThroughput += throughputMBps
						throughputSamples = append(throughputSamples, throughputMBps)
						mu.Unlock()

						t.Logf("  Upload %d: %.2f MB/s", id, throughputMBps)
					}
				}(uploadID)
			}
		}
	}()

	// Wait for test duration
	<-ctx.Done()
	time.Sleep(5 * time.Second) // Allow final uploads to complete

	uploads := atomic.LoadInt64(&uploadCount)
	bytes := atomic.LoadInt64(&totalBytes)
	actualDuration := time.Since(startTime)

	mu.Lock()
	avgThroughput := totalThroughput / float64(len(throughputSamples))

	// Calculate throughput variance
	var variance float64
	for _, sample := range throughputSamples {
		variance += (sample - avgThroughput) * (sample - avgThroughput)
	}
	variance /= float64(len(throughputSamples))
	stdDev := variance // Simplified for this test
	mu.Unlock()

	sustainedThroughput := float64(bytes) / (1024 * 1024) / actualDuration.Seconds()

	t.Logf("📊 Sustained Throughput Results:")
	t.Logf("  Test duration: %s", actualDuration)
	t.Logf("  Total uploads: %d", uploads)
	t.Logf("  Total data: %.2f MB", float64(bytes)/(1024*1024))
	t.Logf("  Sustained throughput: %.2f MB/s", sustainedThroughput)
	t.Logf("  Average per-upload throughput: %.2f MB/s", avgThroughput)
	t.Logf("  Throughput standard deviation: %.2f MB/s", stdDev)
	t.Logf("  Throughput consistency: %.1f%%", (1.0-stdDev/avgThroughput)*100)

	// Performance assertions
	assert.Greater(t, uploads, int64(15), "Should complete at least 15 uploads in 5 minutes")
	assert.Greater(t, avgThroughput, 10.0, "Average throughput should exceed 10 MB/s")
	assert.Less(t, stdDev/avgThroughput, 0.5, "Throughput should be reasonably consistent")

	if avgThroughput > 50 {
		t.Logf("🏆 Excellent sustained performance: >50 MB/s average!")
	} else if avgThroughput > 25 {
		t.Logf("🎯 Good sustained performance: >25 MB/s average!")
	}
}

// testBandwidthSaturation attempts to saturate available bandwidth
func testBandwidthSaturation(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Create multiple transporters to maximize bandwidth utilization
	numTransporters := 4
	transporters := make([]*OptimizedTransporter, numTransporters)

	for i := 0; i < numTransporters; i++ {
		transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
		require.NoError(t, err)
		transporters[i] = transporter
		defer transporter.Shutdown(ctx)
	}

	t.Logf("🚀 Attempting bandwidth saturation with %d parallel transporters...", numTransporters)

	fileSize := 50 * 1024 * 1024 // 50MB files
	uploadsPerTransporter := 3

	var wg sync.WaitGroup
	var totalBytes int64
	var totalDuration int64

	overallStart := time.Now()

	// Launch parallel uploads across multiple transporters
	for transporterID := 0; transporterID < numTransporters; transporterID++ {
		wg.Add(1)
		go func(tID int, transporter *OptimizedTransporter) {
			defer wg.Done()

			for uploadID := 0; uploadID < uploadsPerTransporter; uploadID++ {
				testData := strings.Repeat(fmt.Sprintf("Bandwidth saturation test T%d-U%d. ", tID, uploadID), fileSize/100)
				testKey := fmt.Sprintf("%s/bandwidth-sat-%d-%d-%d.txt", stressTestPrefix, time.Now().Unix(), tID, uploadID)

				archive := &Archive{
					Key:          testKey,
					Reader:       strings.NewReader(testData),
					Size:         int64(len(testData)),
					StorageClass: awsconfig.StorageClassStandard,
					Metadata: map[string]string{
						"test":           "bandwidth-saturation",
						"transporter_id": fmt.Sprintf("%d", tID),
						"upload_id":      fmt.Sprintf("%d", uploadID),
					},
				}

				uploadStart := time.Now()
				_, err := transporter.Upload(ctx, archive)
				uploadDuration := time.Since(uploadStart)

				if err == nil {
					atomic.AddInt64(&totalBytes, archive.Size)
					atomic.AddInt64(&totalDuration, uploadDuration.Nanoseconds())

					throughput := float64(archive.Size) / (1024 * 1024) / uploadDuration.Seconds()
					t.Logf("  T%d-U%d: %.2f MB/s", tID, uploadID, throughput)
				} else {
					t.Logf("  T%d-U%d failed: %v", tID, uploadID, err)
				}
			}
		}(transporterID, transporters[transporterID])
	}

	wg.Wait()
	overallDuration := time.Since(overallStart)

	bytes := atomic.LoadInt64(&totalBytes)
	aggregateThroughput := float64(bytes) / (1024 * 1024) / overallDuration.Seconds()
	bandwidthGbps := aggregateThroughput * 8 / 1000
	saturationPercentage := (bandwidthGbps / 5.0) * 100 // Against 5Gbps connection

	t.Logf("📊 Bandwidth Saturation Results:")
	t.Logf("  Parallel transporters: %d", numTransporters)
	t.Logf("  Total uploads: %d", numTransporters*uploadsPerTransporter)
	t.Logf("  Total data: %.2f MB", float64(bytes)/(1024*1024))
	t.Logf("  Overall duration: %s", overallDuration)
	t.Logf("  Aggregate throughput: %.2f MB/s", aggregateThroughput)
	t.Logf("  Bandwidth utilization: %.3f Gbps", bandwidthGbps)
	t.Logf("  Saturation: %.1f%% of 5 Gbps internet", saturationPercentage)

	// Performance assertions
	assert.Greater(t, aggregateThroughput, 75.0, "Should achieve >75 MB/s aggregate throughput")
	assert.Greater(t, saturationPercentage, 10.0, "Should utilize >10% of available bandwidth")

	if saturationPercentage > 25 {
		t.Logf("🏆 Exceptional bandwidth utilization: >25%% of 5 Gbps!")
	} else if saturationPercentage > 15 {
		t.Logf("🎯 Excellent bandwidth utilization: >15%% of 5 Gbps!")
	}
}

// testMultiGBTransfer tests transfer of multi-GB files if available
func testMultiGBTransfer(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Look for files larger than 1GB
	publicPaths := []string{
		"/Volumes/Public",
		"/mnt/astrapi-public",
		"//astrapi.local/Public",
	}

	var hugeFiles []FileInfo

	for _, basePath := range publicPaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		t.Logf("🔍 Scanning %s for multi-GB files...", basePath)
		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Look for files larger than 1GB but smaller than 10GB
			if !info.IsDir() && info.Size() > 1024*1024*1024 && info.Size() < 10*1024*1024*1024 {
				hugeFiles = append(hugeFiles, FileInfo{
					Path: path,
					Size: info.Size(),
					Name: info.Name(),
				})
			}
			return nil
		})

		if len(hugeFiles) > 0 {
			break
		}
	}

	if len(hugeFiles) == 0 {
		t.Skip("No multi-GB files found in astrapi.local for extreme stress testing")
	}

	// Test the largest file found
	fileInfo := hugeFiles[0]
	t.Logf("🎯 Testing multi-GB file: %s", fileInfo.Path)
	t.Logf("📏 File size: %.3f GB", float64(fileInfo.Size)/(1024*1024*1024))

	// Configure for maximum performance
	maxPerfConfig := s3Config
	maxPerfConfig.Concurrency = 100                      // Maximum concurrency
	maxPerfConfig.MultipartChunkSize = 512 * 1024 * 1024 // 512MB chunks

	transporter, err := NewOptimizedTransporter(ctx, s3Client, maxPerfConfig, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	file, err := os.Open(fileInfo.Path)
	require.NoError(t, err)
	defer file.Close()

	testKey := fmt.Sprintf("%s/multi-gb-%d-%s", stressTestPrefix, time.Now().Unix(), filepath.Base(fileInfo.Path))

	archive := &Archive{
		Key:          testKey,
		Reader:       file,
		Size:         fileInfo.Size,
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"test":    "multi-gb-transfer",
			"source":  "astrapi.local",
			"size_gb": fmt.Sprintf("%.3f", float64(fileInfo.Size)/(1024*1024*1024)),
		},
	}

	t.Logf("🚀 Starting multi-GB upload with maximum performance settings...")
	t.Logf("⚙️  Concurrency: %d, Chunk size: %d MB", maxPerfConfig.Concurrency, maxPerfConfig.MultipartChunkSize/(1024*1024))

	startTime := time.Now()
	result, err := transporter.Upload(ctx, archive)
	duration := time.Since(startTime)

	require.NoError(t, err)

	// Calculate metrics
	sizeGB := float64(fileInfo.Size) / (1024 * 1024 * 1024)
	throughputMBps := float64(fileInfo.Size) / (1024 * 1024) / duration.Seconds()
	throughputGbps := throughputMBps * 8 / 1000
	efficiency := (throughputGbps / 5.0) * 100

	t.Logf("🏆 Multi-GB Transfer Results:")
	t.Logf("  File size: %.3f GB", sizeGB)
	t.Logf("  Upload duration: %s", duration)
	t.Logf("  Average throughput: %.2f MB/s", throughputMBps)
	t.Logf("  Network utilization: %.3f Gbps", throughputGbps)
	t.Logf("  Bandwidth efficiency: %.1f%% of 5 Gbps", efficiency)
	t.Logf("  Result throughput: %.2f MB/s", result.Throughput)
	t.Logf("  Data transfer rate: %.1f MB/minute", throughputMBps*60)

	// Get detailed optimization stats for multi-GB transfer
	stats := transporter.GetOptimizationStats()
	t.Logf("🔧 Multi-GB Optimization Statistics:")
	t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
	t.Logf("  Bandwidth savings: %.1f%%", stats.BandwidthSavings)
	t.Logf("  BBR activations: %d", stats.BBRActivations)
	t.Logf("  CUBIC adjustments: %d", stats.CubicAdjustments)
	t.Logf("  Total optimizations: %d", stats.TotalOptimizations)

	// Performance assertions for multi-GB files
	assert.Greater(t, throughputMBps, 40.0, "Multi-GB transfer should achieve >40 MB/s")
	assert.Greater(t, efficiency, 6.0, "Should utilize >6% of available bandwidth")

	if throughputMBps > 150 {
		t.Logf("🏆 Outstanding multi-GB performance: >150 MB/s!")
	} else if throughputMBps > 100 {
		t.Logf("🎯 Excellent multi-GB performance: >100 MB/s!")
	} else if throughputMBps > 60 {
		t.Logf("✅ Good multi-GB performance: >60 MB/s!")
	}

	// Cleanup (multi-GB cleanup may take time)
	t.Logf("🧹 Cleaning up multi-GB test file...")
	err = transporter.DeleteObject(ctx, testKey)
	require.NoError(t, err)
}

// FileInfo represents file information for testing
type FileInfo struct {
	Path string
	Size int64
	Name string
}

// Helper functions
func ensureStressBucket(ctx context.Context, s3Client *s3.Client, bucket string) error {
	return ensureTestBucket(ctx, s3Client, bucket) // Reuse existing function
}

func cleanupStressObjects(ctx context.Context, s3Client *s3.Client, bucket, prefix string) {
	cleanupTestObjects(ctx, s3Client, bucket, prefix) // Reuse existing function
}
