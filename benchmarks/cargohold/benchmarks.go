package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runCargoHoldBenchmark runs a benchmark using CargoHold
func runCargoHoldBenchmark(config *BenchmarkConfig, spec ScenarioSpec, dataDir, strategy string) (BenchmarkResult, error) {
	result := BenchmarkResult{
		Tool:           "cargohold",
		Strategy:       strategy,
		Scenario:       config.Scenario,
		FileCount:      spec.FileCount,
		TotalSizeBytes: spec.TotalSize,
		Timestamp:      time.Now(),
	}

	// Prepare command
	cargoshipPath, err := exec.LookPath("cargoship")
	if err != nil {
		// Try local build
		cargoshipPath = "./cargoship"
		if _, err := os.Stat(cargoshipPath); err != nil {
			return result, fmt.Errorf("cargoship binary not found")
		}
	}

	args := []string{
		"create", "upload",
		dataDir,
		"--bucket", config.Bucket,
		"--prefix", fmt.Sprintf("%s/cargohold-%s-%s", config.Prefix, strategy, config.Scenario),
		"--shard-strategy", strategy,
		"--shard-count", "10",
	}

	// Start metrics collection
	metrics := startMetricsCollection()

	// Run upload
	startTime := time.Now()
	cmd := exec.Command(cargoshipPath, args...)
	output, err := cmd.CombinedOutput()
	uploadDuration := time.Since(startTime)

	// Stop metrics collection
	stopMetricsCollection(metrics, &result)

	if err != nil {
		return result, fmt.Errorf("cargoship upload failed: %w\nOutput: %s", err, string(output))
	}

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// runToolBenchmark runs a benchmark using an external tool
func runToolBenchmark(tool string, config *BenchmarkConfig, spec ScenarioSpec, dataDir string) (BenchmarkResult, error) {
	result := BenchmarkResult{
		Tool:           tool,
		Scenario:       config.Scenario,
		FileCount:      spec.FileCount,
		TotalSizeBytes: spec.TotalSize,
		Timestamp:      time.Now(),
	}

	switch tool {
	case "s5cmd":
		return runS5cmdBenchmark(config, spec, dataDir, result)
	case "mc":
		return runMinIOMcBenchmark(config, spec, dataDir, result)
	case "tar":
		return runTarBenchmark(config, spec, dataDir, result)
	default:
		return result, fmt.Errorf("unsupported tool: %s", tool)
	}
}

// runS5cmdBenchmark runs benchmark using s5cmd
func runS5cmdBenchmark(config *BenchmarkConfig, spec ScenarioSpec, dataDir string, result BenchmarkResult) (BenchmarkResult, error) {
	// Check if s5cmd is installed
	s5cmdPath, err := exec.LookPath("s5cmd")
	if err != nil {
		return result, fmt.Errorf("s5cmd not found in PATH")
	}

	s3Dest := fmt.Sprintf("s3://%s/%s/s5cmd-%s/*", config.Bucket, config.Prefix, config.Scenario)

	// Start metrics collection
	metrics := startMetricsCollection()

	// Run upload using s5cmd's parallel cp
	startTime := time.Now()
	cmd := exec.Command(s5cmdPath,
		"--numworkers", fmt.Sprintf("%d", config.Concurrency),
		"cp",
		filepath.Join(dataDir, "*"),
		s3Dest,
	)

	output, err := cmd.CombinedOutput()
	uploadDuration := time.Since(startTime)

	// Stop metrics collection
	stopMetricsCollection(metrics, &result)

	if err != nil {
		return result, fmt.Errorf("s5cmd failed: %w\nOutput: %s", err, string(output))
	}

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// runMinIOMcBenchmark runs benchmark using MinIO mc
func runMinIOMcBenchmark(config *BenchmarkConfig, spec ScenarioSpec, dataDir string, result BenchmarkResult) (BenchmarkResult, error) {
	// Check if mc is installed
	mcPath, err := exec.LookPath("mc")
	if err != nil {
		return result, fmt.Errorf("mc not found in PATH")
	}

	// Configure alias (assuming aws profile is configured)
	aliasCmd := exec.Command(mcPath, "alias", "set", "s3bench", "https://s3.amazonaws.com", "", "")
	if output, err := aliasCmd.CombinedOutput(); err != nil {
		return result, fmt.Errorf("mc alias setup failed: %w\nOutput: %s", err, string(output))
	}

	s3Dest := fmt.Sprintf("s3bench/%s/%s/mc-%s/", config.Bucket, config.Prefix, config.Scenario)

	// Start metrics collection
	metrics := startMetricsCollection()

	// Run upload using mc mirror (parallel)
	startTime := time.Now()
	cmd := exec.Command(mcPath,
		"mirror",
		"--parallel", fmt.Sprintf("%d", config.Concurrency),
		dataDir,
		s3Dest,
	)

	output, err := cmd.CombinedOutput()
	uploadDuration := time.Since(startTime)

	// Stop metrics collection
	stopMetricsCollection(metrics, &result)

	if err != nil {
		return result, fmt.Errorf("mc mirror failed: %w\nOutput: %s", err, string(output))
	}

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// runTarBenchmark runs benchmark using traditional tar+zstd+aws-cli approach
func runTarBenchmark(config *BenchmarkConfig, spec ScenarioSpec, dataDir string, result BenchmarkResult) (BenchmarkResult, error) {
	// Check required tools
	for _, tool := range []string{"tar", "zstd", "aws"} {
		if _, err := exec.LookPath(tool); err != nil {
			return result, fmt.Errorf("%s not found in PATH", tool)
		}
	}

	// Create temporary tar.zst file
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("benchmark-%s.tar.zst", config.Scenario))
	defer os.Remove(tmpFile)

	// Start metrics collection
	metrics := startMetricsCollection()
	startTime := time.Now()

	// Step 1: Create tar.zst archive
	tarCmd := exec.Command("tar",
		"-cf", "-",
		"-C", filepath.Dir(dataDir),
		filepath.Base(dataDir),
	)

	zstdCmd := exec.Command("zstd",
		"-3", // Level 3 (fast)
		"-o", tmpFile,
	)

	// Pipe tar output to zstd
	pipe, err := tarCmd.StdoutPipe()
	if err != nil {
		stopMetricsCollection(metrics, &result)
		return result, fmt.Errorf("failed to create pipe: %w", err)
	}
	zstdCmd.Stdin = pipe

	if err := zstdCmd.Start(); err != nil {
		stopMetricsCollection(metrics, &result)
		return result, fmt.Errorf("failed to start zstd: %w", err)
	}

	if err := tarCmd.Run(); err != nil {
		stopMetricsCollection(metrics, &result)
		return result, fmt.Errorf("tar failed: %w", err)
	}

	if err := zstdCmd.Wait(); err != nil {
		stopMetricsCollection(metrics, &result)
		return result, fmt.Errorf("zstd failed: %w", err)
	}

	// Step 2: Upload to S3 using aws cli
	s3Dest := fmt.Sprintf("s3://%s/%s/tar-%s/archive.tar.zst",
		config.Bucket, config.Prefix, config.Scenario)

	awsCmd := exec.Command("aws", "s3", "cp", tmpFile, s3Dest)
	if output, err := awsCmd.CombinedOutput(); err != nil {
		stopMetricsCollection(metrics, &result)
		return result, fmt.Errorf("aws s3 cp failed: %w\nOutput: %s", err, string(output))
	}

	uploadDuration := time.Since(startTime)

	// Stop metrics collection
	stopMetricsCollection(metrics, &result)

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// MetricsCollector tracks resource usage during benchmark
type MetricsCollector struct {
	ctx       context.Context
	cancel    context.CancelFunc
	pid       int
	samples   []resourceSample
	startTime time.Time
}

type resourceSample struct {
	timestamp  time.Time
	cpuPercent float64
	memoryMB   float64
}

func startMetricsCollection() *MetricsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &MetricsCollector{
		ctx:       ctx,
		cancel:    cancel,
		pid:       os.Getpid(),
		samples:   make([]resourceSample, 0),
		startTime: time.Now(),
	}

	// Start background sampling
	go collector.collectSamples()

	return collector
}

func (mc *MetricsCollector) collectSamples() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			sample := mc.takeSample()
			mc.samples = append(mc.samples, sample)
		}
	}
}

func (mc *MetricsCollector) takeSample() resourceSample {
	// Use ps command to get resource usage
	// This is a simplified implementation - production would use proper process monitoring
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", mc.pid), "-o", "%cpu,%mem")
	output, err := cmd.Output()
	if err != nil {
		return resourceSample{timestamp: time.Now()}
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return resourceSample{timestamp: time.Now()}
	}

	var cpu, mem float64
	fmt.Sscanf(lines[1], "%f %f", &cpu, &mem)

	return resourceSample{
		timestamp:  time.Now(),
		cpuPercent: cpu,
		memoryMB:   mem,
	}
}

func stopMetricsCollection(collector *MetricsCollector, result *BenchmarkResult) {
	if collector == nil {
		return
	}

	collector.cancel()
	time.Sleep(200 * time.Millisecond) // Let final samples complete

	// Calculate peak and average metrics
	if len(collector.samples) == 0 {
		return
	}

	var totalCPU, peakMem float64
	for _, sample := range collector.samples {
		totalCPU += sample.cpuPercent
		if sample.memoryMB > peakMem {
			peakMem = sample.memoryMB
		}
	}

	result.AvgCPUPercent = totalCPU / float64(len(collector.samples))
	result.PeakMemoryMB = peakMem
}
