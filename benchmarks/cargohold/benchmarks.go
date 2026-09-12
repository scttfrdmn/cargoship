package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/corpus"
)

// verifyIfCorpus byte-verifies a completed upload against the source corpus when
// running in corpus mode. It downloads what the tool wrote to S3 and confirms
// every file matches by sha256. Mirror-style tools (s5cmd, rclone, mc) preserve
// the file tree and are verified here; cargohold writes chunked archives
// (byte-verified separately by pkg/benchharness) and tar writes a single
// archive, so those log a skip rather than a false failure.
func verifyIfCorpus(config *BenchmarkConfig, tool string, files []corpus.File, result *BenchmarkResult) {
	if config.Corpus == "" || len(files) == 0 {
		return
	}
	prefix, ok := mirrorPrefix(tool, config)
	if !ok {
		log.Printf("      Verify: skipped for %s (chunked/archive layout — cargoship is verified by pkg/benchharness)", tool)
		return
	}
	n, err := verifyMirrorUpload(config.Bucket, prefix, files)
	if err != nil {
		log.Printf("      ❌ Verify FAILED for %s: %v", tool, err)
		result.ErrorCount++
		return
	}
	result.Verified = true
	result.VerifiedFiles = n
}

// mirrorPrefix returns the S3 key prefix a mirror-style tool wrote its file tree
// under, matching the dest each runner builds. Non-mirror tools return ok=false.
func mirrorPrefix(tool string, config *BenchmarkConfig) (string, bool) {
	switch tool {
	case "s5cmd", "rclone", "mc":
		return fmt.Sprintf("%s/%s-%s", config.Prefix, tool, config.Scenario), true
	default: // cargohold (chunked), tar (single archive)
		return "", false
	}
}

// verifyMirrorUpload downloads everything under s3://bucket/prefix/ and confirms
// every source file is present with byte-identical content (matched by basename,
// as pkg/benchharness does). Returns the number of files verified.
func verifyMirrorUpload(bucket, prefix string, files []corpus.File) (int, error) {
	tmp, err := os.MkdirTemp("", "cargohold-verify-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)

	src := fmt.Sprintf("s3://%s/%s/", bucket, prefix)
	if out, err := exec.Command("aws", "s3", "sync", src, tmp).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("aws s3 sync %s: %w\n%s", src, err, out)
	}
	return compareDownloaded(tmp, files)
}

// compareDownloaded confirms every source file appears in dir (matched by
// basename) with a byte-identical sha256. Returns the count verified.
func compareDownloaded(dir string, files []corpus.File) (int, error) {
	idx, err := corpus.IndexByBase(dir)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		path, ok := idx[f.Base]
		if !ok {
			return 0, fmt.Errorf("file %q missing from upload", f.Base)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != f.Sum {
			return 0, fmt.Errorf("file %q content mismatch (sha256)", f.Base)
		}
	}
	return len(files), nil
}

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

	// `cargoship create upload` takes --shards (there is no --shard-strategy /
	// --shard-count in the current CLI); the strategy is retained only as a label
	// for the S3 prefix and results. --quiet disables the TUI for non-interactive
	// exec.
	args := []string{
		"create", "upload",
		dataDir,
		"--bucket", config.Bucket,
		"--prefix", fmt.Sprintf("%s/cargohold-%s-%s", config.Prefix, strategy, config.Scenario),
		"--shards", "10",
		"--quiet",
	}

	cmd := exec.Command(cargoshipPath, args...)
	output, uploadDuration, err := runInstrumented(cmd, &result)
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
	case "rclone":
		return runRcloneBenchmark(config, spec, dataDir, result)
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

	// Target must be a plain prefix (s5cmd rejects a glob in the destination);
	// the source "dir/*" recurses subdirectories.
	s3Dest := fmt.Sprintf("s3://%s/%s/s5cmd-%s/", config.Bucket, config.Prefix, config.Scenario)

	// Run upload using s5cmd's parallel cp
	cmd := exec.Command(s5cmdPath,
		"--numworkers", fmt.Sprintf("%d", config.Concurrency),
		"cp",
		filepath.Join(dataDir, "*"),
		s3Dest,
	)

	output, uploadDuration, err := runInstrumented(cmd, &result)
	if err != nil {
		return result, fmt.Errorf("s5cmd failed: %w\nOutput: %s", err, string(output))
	}

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// runRcloneBenchmark runs an upload benchmark using rclone's S3 backend. It uses
// an inline `:s3:` connection string (creds from the environment via env_auth,
// region from AWS_REGION) so no pre-configured rclone remote is required.
func runRcloneBenchmark(config *BenchmarkConfig, spec ScenarioSpec, dataDir string, result BenchmarkResult) (BenchmarkResult, error) {
	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		return result, fmt.Errorf("rclone not found in PATH")
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	dest := fmt.Sprintf(":s3,provider=AWS,env_auth=true,region=%s:%s/%s/rclone-%s",
		region, config.Bucket, config.Prefix, config.Scenario)

	cmd := exec.Command(rclonePath, "copy", dataDir, dest,
		"--transfers", fmt.Sprintf("%d", config.Concurrency),
		"--s3-no-check-bucket",
	)
	output, uploadDuration, err := runInstrumented(cmd, &result)
	if err != nil {
		return result, fmt.Errorf("rclone failed: %w\nOutput: %s", err, string(output))
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

	// Configure alias against the region's S3 endpoint (creds from the standard
	// AWS env vars). Falls back to the global endpoint when AWS_REGION is unset.
	endpoint := "https://s3.amazonaws.com"
	if region := os.Getenv("AWS_REGION"); region != "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
	}
	aliasCmd := exec.Command(mcPath, "alias", "set", "s3bench", endpoint,
		os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"))
	if output, err := aliasCmd.CombinedOutput(); err != nil {
		return result, fmt.Errorf("mc alias setup failed: %w\nOutput: %s", err, string(output))
	}

	s3Dest := fmt.Sprintf("s3bench/%s/%s/mc-%s/", config.Bucket, config.Prefix, config.Scenario)

	// Run upload using mc mirror (parallel)
	cmd := exec.Command(mcPath,
		"mirror",
		"--parallel", fmt.Sprintf("%d", config.Concurrency),
		dataDir,
		s3Dest,
	)

	output, uploadDuration, err := runInstrumented(cmd, &result)
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

	// Whole-pipeline timing (tar → zstd → aws cp). Resource metrics are sampled
	// on the aws-cp upload child (runInstrumented); the tar|zstd compression runs
	// as separate child processes whose CPU/mem this simple sampler doesn't track.
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
		return result, fmt.Errorf("failed to create pipe: %w", err)
	}
	zstdCmd.Stdin = pipe

	if err := zstdCmd.Start(); err != nil {
		return result, fmt.Errorf("failed to start zstd: %w", err)
	}

	if err := tarCmd.Run(); err != nil {
		return result, fmt.Errorf("tar failed: %w", err)
	}

	if err := zstdCmd.Wait(); err != nil {
		return result, fmt.Errorf("zstd failed: %w", err)
	}

	// Step 2: Upload to S3 using aws cli
	s3Dest := fmt.Sprintf("s3://%s/%s/tar-%s/archive.tar.zst",
		config.Bucket, config.Prefix, config.Scenario)

	awsCmd := exec.Command("aws", "s3", "cp", tmpFile, s3Dest)
	if output, _, err := runInstrumented(awsCmd, &result); err != nil {
		return result, fmt.Errorf("aws s3 cp failed: %w\nOutput: %s", err, string(output))
	}

	uploadDuration := time.Since(startTime)

	result.UploadDuration = uploadDuration
	result.UploadThroughputMBps = float64(spec.TotalSize) / (1024 * 1024) / uploadDuration.Seconds()

	return result, nil
}

// MetricsCollector tracks resource usage during a benchmark. It samples the
// benchmarked CHILD process (set via begin), not the harness itself.
type MetricsCollector struct {
	ctx       context.Context
	cancel    context.CancelFunc
	pid       atomic.Int64 // the child PID to sample; 0 until begin() is called
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
	return &MetricsCollector{
		ctx:       ctx,
		cancel:    cancel,
		samples:   make([]resourceSample, 0),
		startTime: time.Now(),
	}
}

// begin points the collector at pid (the child process to measure) and starts
// background sampling. Sampling before begin is a no-op, so the goroutine never
// races on the PID.
func (mc *MetricsCollector) begin(pid int) {
	mc.pid.Store(int64(pid))
	go mc.collectSamples()
}

// runInstrumented runs cmd to completion while sampling the CHILD process's
// CPU/memory (the fix for measuring os.Getpid(), the harness, instead of the
// tool under test). It returns the combined stdout+stderr, the wall-clock
// duration, and rolls the peak/avg metrics into result.
func runInstrumented(cmd *exec.Cmd, result *BenchmarkResult) ([]byte, time.Duration, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	mc := startMetricsCollection()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		stopMetricsCollection(mc, result)
		return buf.Bytes(), time.Since(start), err
	}
	mc.begin(cmd.Process.Pid)
	err := cmd.Wait()
	dur := time.Since(start)
	stopMetricsCollection(mc, result)
	result.windowStart, result.windowEnd = start, time.Now()
	return buf.Bytes(), dur, err
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
	pid := mc.pid.Load()
	if pid == 0 { // begin() not called yet — nothing to sample
		return resourceSample{timestamp: time.Now()}
	}
	// Use ps command to get resource usage
	// This is a simplified implementation - production would use proper process monitoring
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "%cpu,%mem")
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
