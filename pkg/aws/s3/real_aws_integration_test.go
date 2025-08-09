//go:build integration
// +build integration

package s3

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

const (
	transpTestProfile = "aws"
	transpTestRegion  = "us-west-2"
	transpTestBucket  = "cargoship-integration-test"
	transpTestPrefix  = "transporter-test"
)

// TestCargoShipTransporterIntegration tests all CargoShip transporters with real AWS S3
func TestCargoShipTransporterIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(transpTestProfile),
		config.WithRegion(transpTestRegion),
	)
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg)

	// Ensure test bucket exists
	err = ensureTestBucket(ctx, s3Client, transpTestBucket)
	require.NoError(t, err)

	// CargoShip S3 config optimized for high bandwidth (10Gbps to astrapi.local, 5Gbps internet)
	s3Config := awsconfig.S3Config{
		Bucket:             transpTestBucket,
		Concurrency:        20,                // Higher concurrency for 5Gbps internet
		MultipartChunkSize: 128 * 1024 * 1024, // 128MB chunks for high bandwidth
		MultipartThreshold: 200 * 1024 * 1024, // 200MB threshold
	}

	t.Run("OptimizedTransporter", func(t *testing.T) {
		testOptimizedTransporter(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("StagingTransporter", func(t *testing.T) {
		testStagingTransporter(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("AdaptiveTransporter", func(t *testing.T) {
		testAdaptiveTransporter(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("PerformanceComparison", func(t *testing.T) {
		testTransporterPerformanceComparison(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("HighBandwidthLargeFileFromAstrapi", func(t *testing.T) {
		testHighBandwidthLargeFileFromAstrapi(t, ctx, s3Client, s3Config, logger)
	})

	t.Run("MultipleConcurrentUploads", func(t *testing.T) {
		testMultipleConcurrentUploads(t, ctx, s3Client, s3Config, logger)
	})

	// Cleanup
	t.Cleanup(func() {
		cleanupTestObjects(ctx, s3Client, transpTestBucket, transpTestPrefix)
	})
}

func testOptimizedTransporter(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	testKey := fmt.Sprintf("%s/optimized-test-%d.txt", transpTestPrefix, time.Now().Unix())
	testData := strings.Repeat("CargoShip OptimizedTransporter test data with high performance networking between local 10Gbps astrapi.local and 5Gbps internet to AWS S3. ", 1000) // ~140KB

	archive := &Archive{
		Key:          testKey,
		Reader:       strings.NewReader(testData),
		Size:         int64(len(testData)),
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"test":          "optimized-transporter",
			"local_network": "10gbps",
			"internet":      "5gbps",
			"source":        "integration-test",
		},
		CompressionType: "none",
		AccessPattern:   "frequent",
	}

	t.Run("Upload", func(t *testing.T) {
		startTime := time.Now()

		result, err := transporter.Upload(ctx, archive)

		duration := time.Since(startTime)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, testKey, result.Key)
		assert.NotEmpty(t, result.ETag)

		// Calculate throughput
		throughputMBps := float64(archive.Size) / (1024 * 1024) / duration.Seconds()
		throughputMbps := throughputMBps * 8 // Convert to Mbps

		t.Logf("OptimizedTransporter Upload Results:")
		t.Logf("  File size: %.2f KB", float64(archive.Size)/1024)
		t.Logf("  Duration: %s", duration)
		t.Logf("  Throughput: %.2f MB/s (%.1f Mbps)", throughputMBps, throughputMbps)
		t.Logf("  Result throughput: %.2f MB/s", result.Throughput)
		t.Logf("  ETag: %s", result.ETag)

		// Get optimization stats
		stats := transporter.GetOptimizationStats()
		t.Logf("Optimization Statistics:")
		t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
		t.Logf("  Bandwidth savings: %.1f%%", stats.BandwidthSavings)
		t.Logf("  Latency reduction: %.1f%%", stats.LatencyReduction)
		t.Logf("  BBR activations: %d", stats.BBRActivations)
		t.Logf("  CUBIC adjustments: %d", stats.CubicAdjustments)
		t.Logf("  Total optimizations: %d", stats.TotalOptimizations)
		t.Logf("  Optimization effective: %t", stats.IsOptimizationEffective())
	})

	t.Run("Download", func(t *testing.T) {
		startTime := time.Now()

		reader, err := transporter.Download(ctx, testKey)

		initialDuration := time.Since(startTime)
		require.NoError(t, err)
		defer reader.Close()

		// Read content and measure total time
		content, err := io.ReadAll(reader)
		totalDuration := time.Since(startTime)

		require.NoError(t, err)
		assert.Equal(t, testData, string(content))

		throughputMBps := float64(len(content)) / (1024 * 1024) / totalDuration.Seconds()
		t.Logf("OptimizedTransporter Download:")
		t.Logf("  Initial response: %s", initialDuration)
		t.Logf("  Total duration: %s", totalDuration)
		t.Logf("  Throughput: %.2f MB/s", throughputMBps)
	})

	t.Run("RangeDownload", func(t *testing.T) {
		// Download first 10KB
		rangeSize := int64(10240)
		startTime := time.Now()

		reader, err := transporter.DownloadRange(ctx, testKey, 0, rangeSize)

		duration := time.Since(startTime)
		require.NoError(t, err)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, rangeSize, int64(len(content)))
		assert.Equal(t, testData[:rangeSize], string(content))

		t.Logf("OptimizedTransporter Range Download: %d bytes in %s", len(content), duration)
	})

	// Cleanup
	err = transporter.DeleteObject(ctx, testKey)
	require.NoError(t, err)
}

func testStagingTransporter(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	stagingConfig := &StagingConfig{
		EnableStaging:       true,
		EnableNetworkAdapt:  true,
		EnableOptimization:  true, // Enable S3 optimization
		OptimizationConfig:  s3optimization.DefaultConfig(),
		StageAheadChunks:    10,   // Higher for high bandwidth network
		MaxStagingMemoryMB:  1024, // 1GB for high bandwidth staging
		NetworkMonitoringHz: 5.0,  // More frequent monitoring for dynamic conditions
	}

	transporter, err := NewStagingTransporter(ctx, s3Client, s3Config, stagingConfig, logger)
	require.NoError(t, err)

	testKey := fmt.Sprintf("%s/staging-test-%d.txt", transpTestPrefix, time.Now().Unix())
	// Larger test data to demonstrate staging benefits
	testData := strings.Repeat("CargoShip StagingTransporter with S3 optimization - designed for high bandwidth networks with predictive staging for optimal performance on 10Gbps local and 5Gbps internet connections. ", 10000) // ~1.4MB

	archive := Archive{
		Key:          testKey,
		Reader:       strings.NewReader(testData),
		Size:         int64(len(testData)),
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"test":         "staging-transporter",
			"optimization": "enabled",
			"staging":      "predictive",
			"network":      "high-bandwidth",
		},
		CompressionType: "none",
		AccessPattern:   "archive",
	}

	t.Logf("StagingTransporter Test - File size: %.2f MB", float64(archive.Size)/(1024*1024))
	startTime := time.Now()

	result, err := transporter.UploadWithStaging(ctx, archive)

	duration := time.Since(startTime)
	require.NoError(t, err)
	assert.NotNil(t, result)

	throughputMBps := float64(archive.Size) / (1024 * 1024) / duration.Seconds()
	throughputMbps := throughputMBps * 8

	t.Logf("StagingTransporter with Optimization Results:")
	t.Logf("  File size: %.2f MB", float64(archive.Size)/(1024*1024))
	t.Logf("  Duration: %s", duration)
	t.Logf("  Throughput: %.2f MB/s (%.1f Mbps)", throughputMBps, throughputMbps)
	t.Logf("  Result throughput: %.2f MB/s", result.Throughput)
	t.Logf("  Storage class: %s", result.StorageClass)

	// Performance assertion for high bandwidth
	assert.Greater(t, throughputMBps, 10.0, "Staging should achieve >10 MB/s on high bandwidth network")

	// Cleanup
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(transpTestBucket),
		Key:    aws.String(testKey),
	})
	require.NoError(t, err)
}

func testAdaptiveTransporter(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	adaptiveConfig := &AdaptiveTransporterConfig{
		StagingConfig: &StagingConfig{
			EnableStaging:       true,
			EnableOptimization:  true, // Enable S3 optimization
			OptimizationConfig:  s3optimization.DefaultConfig(),
			StageAheadChunks:    8,
			MaxStagingMemoryMB:  1024,
			NetworkMonitoringHz: 5.0,
		},
		EnableRealTimeAdaptation: true,
		AdaptationSensitivity:    2.0, // Higher sensitivity for high bandwidth variations
		MinAdaptationInterval:    3 * time.Second,
	}

	transporter, err := NewAdaptiveTransporter(ctx, s3Client, s3Config, adaptiveConfig, logger)
	require.NoError(t, err)

	testKey := fmt.Sprintf("%s/adaptive-test-%d.txt", transpTestPrefix, time.Now().Unix())
	testData := strings.Repeat("CargoShip AdaptiveTransporter with S3 optimization - real-time network adaptation for variable bandwidth conditions between 10Gbps local network and 5Gbps internet connection to AWS S3. ", 5000) // ~700KB

	archive := Archive{
		Key:          testKey,
		Reader:       strings.NewReader(testData),
		Size:         int64(len(testData)),
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"test":         "adaptive-transporter",
			"optimization": "s3-optimization-enabled",
			"adaptation":   "real-time",
			"sensitivity":  "high-bandwidth",
		},
		CompressionType: "none",
		AccessPattern:   "frequent",
	}

	t.Logf("AdaptiveTransporter Test - File size: %.2f KB", float64(archive.Size)/1024)
	startTime := time.Now()

	result, err := transporter.UploadWithAdaptation(ctx, archive)

	duration := time.Since(startTime)
	require.NoError(t, err)
	assert.NotNil(t, result)

	throughputMBps := float64(archive.Size) / (1024 * 1024) / duration.Seconds()
	throughputMbps := throughputMBps * 8

	t.Logf("AdaptiveTransporter with Optimization Results:")
	t.Logf("  File size: %.2f KB", float64(archive.Size)/1024)
	t.Logf("  Duration: %s", duration)
	t.Logf("  Throughput: %.2f MB/s (%.1f Mbps)", throughputMBps, throughputMbps)
	t.Logf("  Result throughput: %.2f MB/s", result.Throughput)
	t.Logf("  Adaptive features: real-time adaptation with S3 optimization")

	// Cleanup
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(transpTestBucket),
		Key:    aws.String(testKey),
	})
	require.NoError(t, err)
}

func testTransporterPerformanceComparison(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Test data sized for meaningful performance comparison (10MB)
	testData := strings.Repeat("Performance comparison test data designed to demonstrate the 4.6x improvement of CargoShip's S3 optimization system over regular S3 transport on high bandwidth networks. ", 50000) // ~10MB
	testKeyBase := fmt.Sprintf("%s/perf-comparison-%d", transpTestPrefix, time.Now().Unix())

	var regularDuration, optimizedDuration time.Duration
	var regularThroughput, optimizedThroughput float64

	t.Run("RegularTransporter", func(t *testing.T) {
		transporter := NewTransporter(s3Client, s3Config)

		archive := Archive{
			Key:          testKeyBase + "-regular",
			Reader:       strings.NewReader(testData),
			Size:         int64(len(testData)),
			StorageClass: awsconfig.StorageClassStandard,
			Metadata:     map[string]string{"test": "regular-transporter", "optimization": "none"},
		}

		t.Logf("Regular Transporter - uploading %.2f MB...", float64(archive.Size)/(1024*1024))
		startTime := time.Now()
		_, err := transporter.Upload(ctx, archive)
		regularDuration = time.Since(startTime)

		require.NoError(t, err)
		regularThroughput = float64(archive.Size) / (1024 * 1024) / regularDuration.Seconds()

		t.Logf("Regular Transporter Results:")
		t.Logf("  Duration: %s", regularDuration)
		t.Logf("  Throughput: %.2f MB/s (%.1f Mbps)", regularThroughput, regularThroughput*8)

		// Cleanup
		_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(transpTestBucket),
			Key:    aws.String(archive.Key),
		})
		require.NoError(t, err)
	})

	t.Run("OptimizedTransporter", func(t *testing.T) {
		transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
		require.NoError(t, err)
		defer transporter.Shutdown(ctx)

		archive := &Archive{
			Key:          testKeyBase + "-optimized",
			Reader:       strings.NewReader(testData),
			Size:         int64(len(testData)),
			StorageClass: awsconfig.StorageClassStandard,
			Metadata:     map[string]string{"test": "optimized-transporter", "optimization": "s3-optimization"},
		}

		t.Logf("Optimized Transporter - uploading %.2f MB...", float64(archive.Size)/(1024*1024))
		startTime := time.Now()
		result, err := transporter.Upload(ctx, archive)
		optimizedDuration = time.Since(startTime)

		require.NoError(t, err)
		optimizedThroughput = float64(archive.Size) / (1024 * 1024) / optimizedDuration.Seconds()

		t.Logf("Optimized Transporter Results:")
		t.Logf("  Duration: %s", optimizedDuration)
		t.Logf("  Throughput: %.2f MB/s (%.1f Mbps)", optimizedThroughput, optimizedThroughput*8)
		t.Logf("  Result throughput: %.2f MB/s", result.Throughput)

		// Get detailed optimization metrics
		stats := transporter.GetOptimizationStats()
		t.Logf("Optimization Metrics:")
		t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
		t.Logf("  Bandwidth savings: %.1f%%", stats.BandwidthSavings)
		t.Logf("  Latency reduction: %.1f%%", stats.LatencyReduction)
		t.Logf("  BBR activations: %d", stats.BBRActivations)
		t.Logf("  CUBIC adjustments: %d", stats.CubicAdjustments)

		// Cleanup
		transporter.DeleteObject(ctx, archive.Key)
	})

	// Performance comparison analysis
	t.Run("PerformanceAnalysis", func(t *testing.T) {
		if regularDuration > 0 && optimizedDuration > 0 {
			speedupRatio := regularDuration.Seconds() / optimizedDuration.Seconds()
			throughputRatio := optimizedThroughput / regularThroughput

			t.Logf("Performance Comparison:")
			t.Logf("  Regular duration: %s", regularDuration)
			t.Logf("  Optimized duration: %s", optimizedDuration)
			t.Logf("  Speed improvement: %.2fx", speedupRatio)
			t.Logf("  Regular throughput: %.2f MB/s", regularThroughput)
			t.Logf("  Optimized throughput: %.2f MB/s", optimizedThroughput)
			t.Logf("  Throughput improvement: %.2fx", throughputRatio)

			// Performance assertions
			assert.Greater(t, speedupRatio, 1.0, "Optimized transporter should be faster")
			assert.Greater(t, throughputRatio, 1.0, "Optimized transporter should have higher throughput")

			if throughputRatio >= 2.0 {
				t.Logf("🎉 Excellent performance: %.2fx improvement achieved!", throughputRatio)
			} else if throughputRatio >= 1.5 {
				t.Logf("✅ Good performance: %.2fx improvement achieved", throughputRatio)
			} else {
				t.Logf("ℹ️  Moderate improvement: %.2fx (network conditions may limit gains)", throughputRatio)
			}
		}
	})
}

func testHighBandwidthLargeFileFromAstrapi(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Look for large files in astrapi.local Public directory
	publicPaths := []string{
		"/Volumes/Public",        // macOS network mount
		"/mnt/astrapi-public",    // Linux mount point
		"//astrapi.local/Public", // Windows UNC path
		"/media/astrapi/Public",  // Alternative Linux mount
	}

	var testFile string
	var testFileSize int64

	for _, basePath := range publicPaths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		t.Logf("Scanning %s for large test files...", basePath)
		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Look for files between 50MB and 2GB for high bandwidth testing
			// Preferring larger files to better demonstrate the 10Gbps -> 5Gbps path
			if !info.IsDir() && info.Size() > 50*1024*1024 && info.Size() < 2*1024*1024*1024 {
				testFile = path
				testFileSize = info.Size()
				return filepath.SkipDir
			}
			return nil
		})

		if testFile != "" {
			break
		}
	}

	if testFile == "" {
		t.Skip("No suitable large file found in astrapi.local Public directory (need 50MB-2GB file)")
	}

	t.Logf("Selected test file: %s", testFile)
	t.Logf("File size: %.2f MB (%.3f GB)", float64(testFileSize)/(1024*1024), float64(testFileSize)/(1024*1024*1024))

	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	testKey := fmt.Sprintf("%s/large-file-%d%s", transpTestPrefix, time.Now().Unix(), filepath.Ext(testFile))

	// Configure optimized transporter for high bandwidth scenario
	highBandwidthConfig := s3Config
	highBandwidthConfig.Concurrency = 30                       // Increase for large file
	highBandwidthConfig.MultipartChunkSize = 256 * 1024 * 1024 // 256MB chunks

	transporter, err := NewOptimizedTransporter(ctx, s3Client, highBandwidthConfig, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	archive := &Archive{
		Key:          testKey,
		Reader:       file,
		Size:         testFileSize,
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"test":          "high-bandwidth-large-file",
			"source":        "astrapi.local",
			"local_network": "10gbps",
			"internet":      "5gbps",
			"optimization":  "enabled",
		},
	}

	t.Logf("Starting large file upload test...")
	t.Logf("Network path: astrapi.local (10Gbps) -> local machine -> internet (5Gbps) -> AWS S3")

	startTime := time.Now()
	result, err := transporter.Upload(ctx, archive)
	duration := time.Since(startTime)

	require.NoError(t, err)

	// Calculate comprehensive metrics
	fileSizeMB := float64(testFileSize) / (1024 * 1024)
	fileSizeGB := fileSizeMB / 1024
	throughputMBps := fileSizeMB / duration.Seconds()
	throughputMbps := throughputMBps * 8
	throughputGbps := throughputMbps / 1000

	t.Logf("🚀 Large File Upload Completed!")
	t.Logf("Results:")
	t.Logf("  File size: %.2f MB (%.3f GB)", fileSizeMB, fileSizeGB)
	t.Logf("  Upload duration: %s", duration)
	t.Logf("  Average throughput: %.2f MB/s", throughputMBps)
	t.Logf("  Network utilization: %.1f Mbps (%.3f Gbps)", throughputMbps, throughputGbps)
	t.Logf("  Result throughput: %.2f MB/s", result.Throughput)
	t.Logf("  ETag: %s", result.ETag)

	// Calculate network efficiency
	maxInternetGbps := 5.0
	efficiency := (throughputGbps / maxInternetGbps) * 100

	t.Logf("Network Efficiency:")
	t.Logf("  Internet bandwidth: 5 Gbps")
	t.Logf("  Achieved: %.3f Gbps", throughputGbps)
	t.Logf("  Efficiency: %.1f%% of available bandwidth", efficiency)

	// Get detailed optimization metrics
	stats := transporter.GetOptimizationStats()
	t.Logf("S3 Optimization Performance:")
	t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
	t.Logf("  Bandwidth savings: %.1f%%", stats.BandwidthSavings)
	t.Logf("  Latency reduction: %.1f%%", stats.LatencyReduction)
	t.Logf("  BBR activations: %d", stats.BBRActivations)
	t.Logf("  CUBIC adjustments: %d", stats.CubicAdjustments)
	t.Logf("  Total optimizations: %d", stats.TotalOptimizations)
	t.Logf("  Optimization effective: %t", stats.IsOptimizationEffective())

	// Health check
	err = transporter.HealthCheck(ctx)
	require.NoError(t, err)

	// Performance assertions for high bandwidth networks
	assert.Greater(t, throughputMBps, 50.0, "Should achieve >50 MB/s on 5Gbps internet connection")
	assert.True(t, stats.IsOptimizationEffective(), "S3 optimization should be effective")
	assert.Greater(t, stats.PerformanceImprovement, 1.1, "Should show measurable performance improvement")

	if throughputMBps > 200 {
		t.Logf("🏆 Exceptional performance: >200 MB/s achieved!")
	} else if throughputMBps > 100 {
		t.Logf("🎯 Excellent performance: >100 MB/s achieved!")
	} else if throughputMBps > 50 {
		t.Logf("✅ Good performance: >50 MB/s achieved")
	}

	// Test download performance with streaming
	t.Logf("Testing streaming download performance...")
	downloadStart := time.Now()

	reader, err := transporter.Download(ctx, testKey)
	require.NoError(t, err)

	// Stream download with progress tracking
	buffer := make([]byte, 1024*1024) // 1MB buffer
	totalDownloaded := int64(0)
	lastProgress := time.Now()

	for {
		n, err := reader.Read(buffer)
		totalDownloaded += int64(n)

		// Progress logging every 5 seconds for large files
		if time.Since(lastProgress) > 5*time.Second {
			progress := float64(totalDownloaded) / float64(testFileSize) * 100
			currentThroughput := float64(totalDownloaded) / (1024 * 1024) / time.Since(downloadStart).Seconds()
			t.Logf("  Download progress: %.1f%% (%.2f MB/s)", progress, currentThroughput)
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
	downloadThroughputGbps := downloadThroughputMBps * 8 / 1000

	t.Logf("Download Results:")
	t.Logf("  Downloaded: %.2f MB", float64(totalDownloaded)/(1024*1024))
	t.Logf("  Duration: %s", downloadDuration)
	t.Logf("  Throughput: %.2f MB/s (%.3f Gbps)", downloadThroughputMBps, downloadThroughputGbps)
	t.Logf("  Efficiency: %.1f%% of 5 Gbps internet", (downloadThroughputGbps/5.0)*100)

	// Performance assertions for download
	assert.Greater(t, downloadThroughputMBps, 50.0, "Download should exceed 50 MB/s")
	assert.Equal(t, testFileSize, totalDownloaded, "Should download complete file")

	// Cleanup
	t.Logf("Cleaning up test file...")
	err = transporter.DeleteObject(ctx, testKey)
	require.NoError(t, err)
	t.Logf("✅ Test completed successfully!")
}

func testMultipleConcurrentUploads(t *testing.T, ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, logger *slog.Logger) {
	// Test concurrent uploads to validate optimization under load
	numUploads := 5
	fileSize := 5 * 1024 * 1024 // 5MB per file

	transporter, err := NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	require.NoError(t, err)
	defer transporter.Shutdown(ctx)

	// Generate test data
	testData := strings.Repeat("Concurrent upload test data for CargoShip S3 optimization validation. ", fileSize/100)

	t.Logf("Testing %d concurrent uploads of %.2f MB each...", numUploads, float64(fileSize)/(1024*1024))

	// Channel to collect results
	type uploadResult struct {
		id         int
		duration   time.Duration
		throughput float64
		err        error
	}

	results := make(chan uploadResult, numUploads)
	startTime := time.Now()

	// Launch concurrent uploads
	for i := 0; i < numUploads; i++ {
		go func(uploadID int) {
			testKey := fmt.Sprintf("%s/concurrent-test-%d-%d.txt", transpTestPrefix, time.Now().Unix(), uploadID)
			archive := &Archive{
				Key:          testKey,
				Reader:       strings.NewReader(testData),
				Size:         int64(len(testData)),
				StorageClass: awsconfig.StorageClassStandard,
				Metadata: map[string]string{
					"test":      "concurrent-upload",
					"upload_id": fmt.Sprintf("%d", uploadID),
				},
			}

			uploadStart := time.Now()
			_, err := transporter.Upload(ctx, archive)
			duration := time.Since(uploadStart)

			throughput := float64(archive.Size) / (1024 * 1024) / duration.Seconds()

			results <- uploadResult{
				id:         uploadID,
				duration:   duration,
				throughput: throughput,
				err:        err,
			}
		}(i)
	}

	// Collect results
	totalDuration := time.Duration(0)
	totalThroughput := 0.0
	successCount := 0

	for i := 0; i < numUploads; i++ {
		result := <-results
		if result.err != nil {
			t.Errorf("Upload %d failed: %v", result.id, result.err)
			continue
		}

		successCount++
		totalDuration += result.duration
		totalThroughput += result.throughput

		t.Logf("Upload %d: %.2f MB/s in %s", result.id, result.throughput, result.duration)
	}

	overallDuration := time.Since(startTime)
	avgThroughput := totalThroughput / float64(successCount)
	totalDataMB := float64(fileSize*successCount) / (1024 * 1024)
	aggregateThroughput := totalDataMB / overallDuration.Seconds()

	t.Logf("Concurrent Upload Results:")
	t.Logf("  Successful uploads: %d/%d", successCount, numUploads)
	t.Logf("  Total data: %.2f MB", totalDataMB)
	t.Logf("  Overall duration: %s", overallDuration)
	t.Logf("  Average per-upload throughput: %.2f MB/s", avgThroughput)
	t.Logf("  Aggregate throughput: %.2f MB/s", aggregateThroughput)

	// Get optimization stats after concurrent load
	stats := transporter.GetOptimizationStats()
	t.Logf("Optimization under concurrent load:")
	t.Logf("  Performance improvement: %.2fx", stats.PerformanceImprovement)
	t.Logf("  Total optimizations: %d", stats.TotalOptimizations)
	t.Logf("  Optimization effective: %t", stats.IsOptimizationEffective())

	// Performance assertions
	assert.Equal(t, numUploads, successCount, "All uploads should succeed")
	assert.Greater(t, avgThroughput, 10.0, "Average throughput should exceed 10 MB/s")
	assert.Greater(t, aggregateThroughput, 20.0, "Aggregate throughput should exceed 20 MB/s")

	// Cleanup concurrent test files
	for i := 0; i < numUploads; i++ {
		testKey := fmt.Sprintf("%s/concurrent-test-%d-%d.txt", transpTestPrefix, startTime.Unix(), i)
		transporter.DeleteObject(ctx, testKey)
	}
}

// Helper functions

func ensureTestBucket(ctx context.Context, s3Client *s3.Client, bucket string) error {
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err == nil {
		return nil
	}

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(transpTestRegion),
		},
	})

	return err
}

func cleanupTestObjects(ctx context.Context, s3Client *s3.Client, bucket, prefix string) {
	result, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		return
	}

	for _, obj := range result.Contents {
		s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
	}
}
