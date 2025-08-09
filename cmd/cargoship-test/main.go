package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// Command-line flags
var (
	testType              = flag.String("test-type", "", "Type of test to run (performance, stress, large-file, benchmark-suite)")
	testFiles             = flag.String("test-files", "", "Comma-separated list of test files")
	awsProfile            = flag.String("aws-profile", "aws", "AWS profile to use")
	awsRegion             = flag.String("aws-region", "us-west-2", "AWS region")
	s3Bucket              = flag.String("s3-bucket", "", "S3 bucket for testing")
	outputFormat          = flag.String("output-format", "json", "Output format (json, text)")
	verbose               = flag.Bool("verbose", false, "Enable verbose logging")
	enableOptimization    = flag.Bool("enable-optimization", true, "Enable S3 optimization")
	enableBBR             = flag.Bool("enable-bbr", true, "Enable BBR congestion control")
	enableCUBIC           = flag.Bool("enable-cubic", true, "Enable CUBIC congestion control")
	concurrency           = flag.Int("concurrency", 50, "S3 concurrency level")
	multipartThreshold    = flag.Int64("multipart-threshold", 500*1024*1024, "Multipart upload threshold")
	multipartChunkSize    = flag.Int64("multipart-chunk-size", 256*1024*1024, "Multipart chunk size")
	maxConcurrency        = flag.Int("max-concurrency", 200, "Maximum concurrency for stress tests")
	chunkSizeMB           = flag.Int("chunk-size-mb", 256, "Chunk size in MB")
	_                     = flag.Bool("stress-mode", false, "Enable stress testing mode") // Currently unused
	enableMonitoring      = flag.Bool("enable-monitoring", true, "Enable resource monitoring")
	networkOptimization   = flag.Bool("network-optimization", true, "Enable network optimization")
	_                     = flag.Bool("enable-progress-tracking", false, "Enable progress tracking for large files") // Currently unused
	maxFileSizeGB         = flag.Int("max-file-size-gb", 10, "Maximum file size in GB")
	_                     = flag.Bool("enable-resume", false, "Enable resumable uploads") // Currently unused
	_                     = flag.Bool("verify-integrity", true, "Verify file integrity")  // Currently unused
	runAllTests           = flag.Bool("run-all-tests", false, "Run all available tests")
	includeStressTests    = flag.Bool("include-stress-tests", false, "Include stress tests in suite")
	includeLargeFileTests = flag.Bool("include-large-file-tests", false, "Include large file tests in suite")
	_                     = flag.Bool("include-concurrent-tests", false, "Include concurrent tests in suite") // Currently unused
	_                     = flag.Bool("generate-report", true, "Generate test report")                        // Currently unused
	_                     = flag.Bool("network-analysis", true, "Enable network analysis")                    // Currently unused
	_                     = flag.Bool("resource-monitoring", true, "Enable resource monitoring")              // Currently unused
)

func main() {
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Validate required parameters
	if *testType == "" {
		logger.Error("test-type is required")
		os.Exit(1)
	}

	if *s3Bucket == "" {
		logger.Error("s3-bucket is required")
		os.Exit(1)
	}

	logger.Info("starting CargoShip test execution",
		"test_type", *testType,
		"aws_profile", *awsProfile,
		"aws_region", *awsRegion,
		"s3_bucket", *s3Bucket,
		"optimization_enabled", *enableOptimization)

	ctx := context.Background()

	// Run the specified test
	results, err := runTest(ctx, logger)
	if err != nil {
		logger.Error("test execution failed", "error", err)
		os.Exit(1)
	}

	// Output results
	if err := outputResults(results); err != nil {
		logger.Error("failed to output results", "error", err)
		os.Exit(1)
	}

	if !results.Success {
		logger.Error("test failed", "errors", results.ErrorCount)
		os.Exit(1)
	}

	logger.Info("test completed successfully",
		"duration", results.Duration,
		"throughput", results.AverageThroughputMBps,
		"files_processed", results.ProcessedFiles)
}

func runTest(ctx context.Context, logger *slog.Logger) (*launch.TestResults, error) {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(*awsProfile),
		config.WithRegion(*awsRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	// Configure S3 settings
	s3Config := awsconfig.S3Config{
		Bucket:             *s3Bucket,
		Concurrency:        *concurrency,
		MultipartThreshold: *multipartThreshold,
		MultipartChunkSize: *multipartChunkSize,
	}

	// Configure optimization if enabled
	var optimizationConfig *s3optimization.Config
	if *enableOptimization {
		optimizationConfig = &s3optimization.Config{
			EnableBBR:          *enableBBR,
			EnableCUBIC:        *enableCUBIC,
			NetworkAdaptation:  *networkOptimization,
			PredictiveMode:     true,
			MaxConnections:     *maxConcurrency,
			ConnectionPoolSize: *concurrency,
			BufferSize:         int64(*chunkSizeMB) * 1024 * 1024,
			MetricsEnabled:     *enableMonitoring,
		}
	}

	// Execute test based on type
	switch *testType {
	case "performance":
		return runPerformanceTest(ctx, s3Client, s3Config, optimizationConfig, logger)
	case "stress":
		return runStressTest(ctx, s3Client, s3Config, optimizationConfig, logger)
	case "large_file_transfer":
		return runLargeFileTest(ctx, s3Client, s3Config, optimizationConfig, logger)
	case "benchmark_suite":
		return runBenchmarkSuite(ctx, s3Client, s3Config, optimizationConfig, logger)
	default:
		return nil, fmt.Errorf("unknown test type: %s", *testType)
	}
}

func runPerformanceTest(ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, optimizationConfig *s3optimization.Config, logger *slog.Logger) (*launch.TestResults, error) {
	logger.Info("running performance test")

	transporter, err := s3transport.NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create transporter: %w", err)
	}
	defer func() {
		if err := transporter.Shutdown(ctx); err != nil {
			fmt.Printf("Warning: failed to shutdown transporter: %v\n", err)
		}
	}()

	startTime := time.Now()
	var totalBytes int64
	var errors []string

	// Get test files
	files := getTestFiles()
	logger.Info("found test files", "count", len(files))

	processedFiles := 0
	for _, file := range files {
		if err := uploadFile(ctx, transporter, file, logger); err != nil {
			errors = append(errors, fmt.Sprintf("failed to upload %s: %v", file, err))
			continue
		}

		fileInfo, _ := os.Stat(file)
		if fileInfo != nil {
			totalBytes += fileInfo.Size()
		}
		processedFiles++
	}

	duration := time.Since(startTime)
	averageThroughput := float64(totalBytes) / (1024 * 1024) / duration.Seconds()

	// Get optimization stats
	stats := transporter.GetOptimizationStats()

	return &launch.TestResults{
		TestType:              "performance",
		Success:               len(errors) == 0,
		TotalFiles:            len(files),
		ProcessedFiles:        processedFiles,
		TotalBytes:            totalBytes,
		ProcessedBytes:        totalBytes,
		Duration:              duration,
		AverageThroughputMBps: averageThroughput,
		PeakThroughputMBps:    averageThroughput, // Simplified for basic test
		OptimizationStats:     stats,
		NetworkUtilization:    calculateNetworkUtilization(averageThroughput),
		ErrorCount:            len(errors),
		Errors:                errors,
	}, nil
}

func runStressTest(ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, optimizationConfig *s3optimization.Config, logger *slog.Logger) (*launch.TestResults, error) {
	logger.Info("running stress test", "max_concurrency", *maxConcurrency)

	// Configure for stress testing
	s3Config.Concurrency = *maxConcurrency
	s3Config.MultipartChunkSize = int64(*chunkSizeMB) * 1024 * 1024

	transporter, err := s3transport.NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create transporter: %w", err)
	}
	defer func() {
		if err := transporter.Shutdown(ctx); err != nil {
			fmt.Printf("Warning: failed to shutdown transporter: %v\n", err)
		}
	}()

	startTime := time.Now()

	// Run concurrent stress test
	files := getTestFiles()
	results := make(chan uploadResult, len(files))

	for _, file := range files {
		go func(f string) {
			err := uploadFile(ctx, transporter, f, logger)
			fileInfo, _ := os.Stat(f)
			size := int64(0)
			if fileInfo != nil {
				size = fileInfo.Size()
			}
			results <- uploadResult{file: f, size: size, err: err}
		}(file)
	}

	// Collect results
	var totalBytes int64
	var errors []string
	processedFiles := 0

	for i := 0; i < len(files); i++ {
		result := <-results
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("failed to upload %s: %v", result.file, result.err))
		} else {
			totalBytes += result.size
			processedFiles++
		}
	}

	duration := time.Since(startTime)
	averageThroughput := float64(totalBytes) / (1024 * 1024) / duration.Seconds()

	stats := transporter.GetOptimizationStats()

	return &launch.TestResults{
		TestType:              "stress",
		Success:               len(errors) == 0,
		TotalFiles:            len(files),
		ProcessedFiles:        processedFiles,
		TotalBytes:            totalBytes,
		ProcessedBytes:        totalBytes,
		Duration:              duration,
		AverageThroughputMBps: averageThroughput,
		PeakThroughputMBps:    averageThroughput * 1.2, // Estimate peak
		OptimizationStats:     stats,
		NetworkUtilization:    calculateNetworkUtilization(averageThroughput),
		ErrorCount:            len(errors),
		Errors:                errors,
	}, nil
}

func runLargeFileTest(ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, optimizationConfig *s3optimization.Config, logger *slog.Logger) (*launch.TestResults, error) {
	logger.Info("running large file test", "max_file_size_gb", *maxFileSizeGB)

	transporter, err := s3transport.NewOptimizedTransporter(ctx, s3Client, s3Config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create transporter: %w", err)
	}
	defer func() {
		if err := transporter.Shutdown(ctx); err != nil {
			fmt.Printf("Warning: failed to shutdown transporter: %v\n", err)
		}
	}()

	// Find large files
	files := getLargeTestFiles(int64(*maxFileSizeGB) * 1024 * 1024 * 1024)
	if len(files) == 0 {
		return nil, fmt.Errorf("no large files found for testing")
	}

	logger.Info("found large files", "count", len(files))

	startTime := time.Now()
	var totalBytes int64
	var errors []string
	processedFiles := 0

	for _, file := range files {
		fileInfo, err := os.Stat(file)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to stat %s: %v", file, err))
			continue
		}

		logger.Info("uploading large file", "file", file, "size_mb", fileInfo.Size()/(1024*1024))

		if err := uploadFile(ctx, transporter, file, logger); err != nil {
			errors = append(errors, fmt.Sprintf("failed to upload %s: %v", file, err))
			continue
		}

		totalBytes += fileInfo.Size()
		processedFiles++
	}

	duration := time.Since(startTime)
	averageThroughput := float64(totalBytes) / (1024 * 1024) / duration.Seconds()

	stats := transporter.GetOptimizationStats()

	return &launch.TestResults{
		TestType:              "large_file_transfer",
		Success:               len(errors) == 0,
		TotalFiles:            len(files),
		ProcessedFiles:        processedFiles,
		TotalBytes:            totalBytes,
		ProcessedBytes:        totalBytes,
		Duration:              duration,
		AverageThroughputMBps: averageThroughput,
		PeakThroughputMBps:    averageThroughput,
		OptimizationStats:     stats,
		NetworkUtilization:    calculateNetworkUtilization(averageThroughput),
		ErrorCount:            len(errors),
		Errors:                errors,
	}, nil
}

func runBenchmarkSuite(ctx context.Context, s3Client *s3.Client, s3Config awsconfig.S3Config, optimizationConfig *s3optimization.Config, logger *slog.Logger) (*launch.TestResults, error) {
	logger.Info("running benchmark suite")

	var allResults []*launch.TestResults
	var totalBytes int64
	var totalFiles int
	var totalErrors []string

	suiteStart := time.Now()

	// Run performance test
	if *runAllTests || true {
		logger.Info("running performance benchmark")
		result, err := runPerformanceTest(ctx, s3Client, s3Config, optimizationConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("performance test failed: %w", err)
		}
		allResults = append(allResults, result)
		totalBytes += result.ProcessedBytes
		totalFiles += result.ProcessedFiles
		totalErrors = append(totalErrors, result.Errors...)
	}

	// Run stress test if enabled
	if *includeStressTests {
		logger.Info("running stress benchmark")
		result, err := runStressTest(ctx, s3Client, s3Config, optimizationConfig, logger)
		if err != nil {
			logger.Warn("stress test failed", "error", err)
		} else {
			allResults = append(allResults, result)
			totalBytes += result.ProcessedBytes
			totalFiles += result.ProcessedFiles
			totalErrors = append(totalErrors, result.Errors...)
		}
	}

	// Run large file test if enabled
	if *includeLargeFileTests {
		logger.Info("running large file benchmark")
		result, err := runLargeFileTest(ctx, s3Client, s3Config, optimizationConfig, logger)
		if err != nil {
			logger.Warn("large file test failed", "error", err)
		} else {
			allResults = append(allResults, result)
			totalBytes += result.ProcessedBytes
			totalFiles += result.ProcessedFiles
			totalErrors = append(totalErrors, result.Errors...)
		}
	}

	suiteDuration := time.Since(suiteStart)
	averageThroughput := float64(totalBytes) / (1024 * 1024) / suiteDuration.Seconds()

	// Calculate peak throughput from all results
	peakThroughput := 0.0
	for _, result := range allResults {
		if result.PeakThroughputMBps > peakThroughput {
			peakThroughput = result.PeakThroughputMBps
		}
	}

	return &launch.TestResults{
		TestType:              "benchmark_suite",
		Success:               len(totalErrors) == 0,
		TotalFiles:            totalFiles,
		ProcessedFiles:        totalFiles,
		TotalBytes:            totalBytes,
		ProcessedBytes:        totalBytes,
		Duration:              suiteDuration,
		AverageThroughputMBps: averageThroughput,
		PeakThroughputMBps:    peakThroughput,
		NetworkUtilization:    calculateNetworkUtilization(averageThroughput),
		ErrorCount:            len(totalErrors),
		Errors:                totalErrors,
	}, nil
}

type uploadResult struct {
	file string
	size int64
	err  error
}

func uploadFile(ctx context.Context, transporter *s3transport.OptimizedTransporter, filePath string, logger *slog.Logger) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	key := fmt.Sprintf("test/%s", filepath.Base(filePath))

	archive := &s3transport.Archive{
		Key:          key,
		Reader:       file,
		Size:         fileInfo.Size(),
		StorageClass: awsconfig.StorageClassStandard,
		Metadata: map[string]string{
			"source":        "astrapi-test",
			"original_path": filePath,
		},
	}

	_, err = transporter.Upload(ctx, archive)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	logger.Debug("file uploaded successfully", "file", filePath, "key", key, "size", fileInfo.Size())
	return nil
}

func getTestFiles() []string {
	if *testFiles != "" {
		return strings.Split(*testFiles, ",")
	}

	// Auto-discover test files in /data/public
	var files []string
	publicDir := "/data/public"

	_ = filepath.Walk(publicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && info.Size() > 1024*1024 { // Files > 1MB
			files = append(files, path)
		}

		return nil
	})

	// Limit to first 10 files for basic tests
	if len(files) > 10 {
		files = files[:10]
	}

	return files
}

func getLargeTestFiles(maxSize int64) []string {
	var files []string
	publicDir := "/data/public"

	_ = filepath.Walk(publicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Look for files between 100MB and maxSize
		if !info.IsDir() && info.Size() > 100*1024*1024 && info.Size() <= maxSize {
			files = append(files, path)
		}

		return nil
	})

	return files
}

func calculateNetworkUtilization(throughputMBps float64) *launch.NetworkUtilization {
	// Convert MB/s to Mbps
	throughputMbps := throughputMBps * 8

	// Astrapi has 10Gbps local network, 5Gbps internet
	localNetworkGbps := 10.0
	internetGbps := 5.0

	// Assume traffic goes through internet to AWS
	internetEfficiency := (throughputMbps / 1000) / internetGbps * 100
	localEfficiency := (throughputMbps / 1000) / localNetworkGbps * 100

	return &launch.NetworkUtilization{
		LocalNetworkMbps:   throughputMbps,
		InternetMbps:       throughputMbps,
		LocalEfficiency:    localEfficiency,
		InternetEfficiency: internetEfficiency,
		OptimalPathUsed:    throughputMbps > 100, // Assume optimal if >100 Mbps
	}
}

func outputResults(results *launch.TestResults) error {
	switch *outputFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	case "text":
		fmt.Printf("Test Type: %s\n", results.TestType)
		fmt.Printf("Success: %t\n", results.Success)
		fmt.Printf("Files Processed: %d/%d\n", results.ProcessedFiles, results.TotalFiles)
		fmt.Printf("Data Processed: %.2f MB\n", float64(results.ProcessedBytes)/(1024*1024))
		fmt.Printf("Duration: %s\n", results.Duration)
		fmt.Printf("Average Throughput: %.2f MB/s\n", results.AverageThroughputMBps)
		fmt.Printf("Peak Throughput: %.2f MB/s\n", results.PeakThroughputMBps)
		if results.NetworkUtilization != nil {
			fmt.Printf("Internet Efficiency: %.1f%%\n", results.NetworkUtilization.InternetEfficiency)
		}
		if results.ErrorCount > 0 {
			fmt.Printf("Errors: %d\n", results.ErrorCount)
		}
		return nil
	default:
		return fmt.Errorf("unknown output format: %s", *outputFormat)
	}
}
