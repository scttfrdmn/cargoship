// Package main provides comprehensive CargoHold performance benchmarking
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/corpus"
)

// BenchmarkConfig holds configuration for a benchmark run
type BenchmarkConfig struct {
	Scenario        string   // small, medium, large, xlarge
	Tools           []string // cargohold, s5cmd, mc, tar
	ShardStrategies []string // hash, size, type, adaptive (CargoHold only)
	Bucket          string   // S3 bucket for testing
	Prefix          string   // S3 prefix
	DataDir         string   // Local data directory
	ResultsDir      string   // Results output directory
	Concurrency     int      // Parallel operations
	Iterations      int      // Number of runs per test
	Corpus          string   // shared pkg/corpus profile name; "" == legacy scenario generator

	CloudWatchMetrics bool          // read competitor server-side request counts from CloudWatch (real-AWS only)
	MetricsSettle     time.Duration // how long to wait after runs for metrics to publish
}

// BenchmarkResult stores results from a single benchmark run
type BenchmarkResult struct {
	Tool                   string        `json:"tool"`
	Strategy               string        `json:"strategy,omitempty"`
	Scenario               string        `json:"scenario"`
	FileCount              int           `json:"file_count"`
	TotalSizeBytes         int64         `json:"total_size_bytes"`
	UploadDuration         time.Duration `json:"upload_duration"`
	DownloadDuration       time.Duration `json:"download_duration,omitempty"`
	UploadThroughputMBps   float64       `json:"upload_throughput_mbps"`
	DownloadThroughputMBps float64       `json:"download_throughput_mbps,omitempty"`
	PeakMemoryMB           float64       `json:"peak_memory_mb"`
	AvgCPUPercent          float64       `json:"avg_cpu_percent"`
	RequestCount           int           `json:"request_count,omitempty"`
	ErrorCount             int           `json:"error_count"`
	Verified               bool          `json:"verified,omitempty"`       // upload byte-verified against the source corpus
	VerifiedFiles          int           `json:"verified_files,omitempty"` // files confirmed byte-identical
	RequestsUSD            float64       `json:"requests_usd,omitempty"`   // itemized S3 request cost (from server-side counts)
	Timestamp              time.Time     `json:"timestamp"`

	// windowStart/windowEnd bound the run in wall-clock time, so a post-run
	// CloudWatch read can attribute server-side request counts to it, and
	// metricsLabel identifies its prefix-filtered metrics configuration. Not
	// serialized — internal to the metrics pass.
	windowStart  time.Time
	windowEnd    time.Time
	metricsLabel string
}

// ScenarioSpec defines a test scenario
type ScenarioSpec struct {
	Name        string
	FileCount   int
	TotalSize   int64
	MinFileSize int64
	MaxFileSize int64
}

var scenarios = map[string]ScenarioSpec{
	"small": {
		Name:        "Small",
		FileCount:   10000,
		TotalSize:   1 * 1024 * 1024 * 1024, // 1 GB
		MinFileSize: 10 * 1024,              // 10 KB
		MaxFileSize: 200 * 1024,             // 200 KB
	},
	"medium": {
		Name:        "Medium",
		FileCount:   100000,
		TotalSize:   10 * 1024 * 1024 * 1024, // 10 GB
		MinFileSize: 10 * 1024,               // 10 KB
		MaxFileSize: 200 * 1024,              // 200 KB
	},
	"large": {
		Name:        "Large",
		FileCount:   1000000,
		TotalSize:   100 * 1024 * 1024 * 1024, // 100 GB
		MinFileSize: 10 * 1024,                // 10 KB
		MaxFileSize: 200 * 1024,               // 200 KB
	},
	"xlarge": {
		Name:        "XLarge",
		FileCount:   10000000,
		TotalSize:   1024 * 1024 * 1024 * 1024, // 1 TB
		MinFileSize: 10 * 1024,                 // 10 KB
		MaxFileSize: 200 * 1024,                // 200 KB
	},
}

func main() {
	config := parseFlags()

	if err := validateConfig(config); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("🚀 Starting CargoHold Performance Benchmark Suite")
	log.Printf("   Scenario: %s", config.Scenario)
	log.Printf("   Tools: %v", config.Tools)
	log.Printf("   Bucket: %s", config.Bucket)
	log.Printf("   Results: %s\n", config.ResultsDir)

	// Create results directory
	if err := os.MkdirAll(config.ResultsDir, 0755); err != nil {
		log.Fatalf("Failed to create results directory: %v", err)
	}

	// Prepare test data — either the shared, reproducible pkg/corpus profile
	// (enables byte-verify) or the legacy scenario generator.
	spec := scenarios[config.Scenario]
	dataDir := filepath.Join(config.DataDir, config.Scenario)
	var corpusFiles []corpus.File
	if config.Corpus != "" {
		files, planted, dir, err := plantCorpus(config)
		if err != nil {
			log.Fatalf("Failed to plant corpus %q: %v", config.Corpus, err)
		}
		corpusFiles, dataDir = files, dir
		spec = planted
	} else {
		if err := ensureTestData(dataDir, spec); err != nil {
			log.Fatalf("Failed to prepare test data: %v", err)
		}
	}
	log.Printf("📊 Test Scenario: %s", spec.Name)
	log.Printf("   Files: %d", spec.FileCount)
	log.Printf("   Total Size: %s", formatBytes(spec.TotalSize))
	if config.Corpus != "" {
		log.Printf("   Corpus: %s (reproducible; uploads will be byte-verified)\n", config.Corpus)
	}

	// Optionally enable CloudWatch S3 request metrics up front (they take up to
	// ~15 min to activate the first time on a bucket), so a post-run read can
	// attribute each tool's server-side request counts and itemized request cost.
	var mc *metricsClients
	if config.CloudWatchMetrics {
		var err error
		if mc, err = enableRequestMetrics(context.Background(), config); err != nil {
			log.Fatalf("Failed to enable CloudWatch request metrics: %v", err)
		}
	}

	// Run benchmarks
	var results []BenchmarkResult

	for _, tool := range config.Tools {
		if tool == "cargohold" {
			// Test all shard strategies for CargoHold
			for _, strategy := range config.ShardStrategies {
				log.Printf("\n🔬 Benchmarking CargoHold with %s strategy (%d iteration(s))...", strategy, config.Iterations)
				result, err := runBest(config.Iterations, func() (BenchmarkResult, error) {
					return runCargoHoldBenchmark(config, spec, dataDir, strategy)
				})
				if err != nil {
					log.Printf("❌ CargoHold %s failed: %v", strategy, err)
					continue
				}
				result.metricsLabel = resultLabel("cargohold", strategy, config.Scenario)
				results = append(results, result)
			}
		} else {
			log.Printf("\n🔬 Benchmarking %s (%d iteration(s))...", tool, config.Iterations)
			result, err := runBest(config.Iterations, func() (BenchmarkResult, error) {
				return runToolBenchmark(tool, config, spec, dataDir)
			})
			if err != nil {
				log.Printf("❌ %s failed: %v", tool, err)
				continue
			}
			result.metricsLabel = resultLabel(tool, "", config.Scenario)
			results = append(results, result)
		}
	}

	// Read server-side request counts + itemized request cost from CloudWatch
	// BEFORE byte-verify: verify downloads the objects (aws s3 sync), which would
	// otherwise land in the metric window and inflate a competitor's GET count.
	if mc != nil {
		populateRequestCounts(context.Background(), mc, config, results)
	}

	// Byte-verify each upload (safe now that the metric read is done) and print.
	for i := range results {
		verifyIfCorpus(config, results[i].Tool, corpusFiles, &results[i])
		printResult(results[i])
	}

	// Save results
	if err := saveResults(config.ResultsDir, config.Scenario, results); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}

	// Generate comparison report
	log.Printf("\n📈 Generating comparison report...")
	if err := generateReport(config.ResultsDir, config.Scenario, results); err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	log.Printf("\n✅ Benchmark complete!")
	log.Printf("   Results saved to: %s", config.ResultsDir)
}

// runBest runs a benchmark up to iterations times and returns the run with the
// highest upload throughput — best-of-N factors out transient contention, and
// (unlike the old code) actually honors -iterations. Each attempt is logged so
// the raw spread stays visible; an error is returned only if every run failed.
func runBest(iterations int, run func() (BenchmarkResult, error)) (BenchmarkResult, error) {
	if iterations < 1 {
		iterations = 1
	}
	var (
		best    BenchmarkResult
		haveOne bool
		lastErr error
	)
	for i := 1; i <= iterations; i++ {
		res, err := run()
		if err != nil {
			log.Printf("      iteration %d/%d failed: %v", i, iterations, err)
			lastErr = err
			continue
		}
		log.Printf("      iteration %d/%d: %.1f MB/s", i, iterations, res.UploadThroughputMBps)
		if !haveOne || res.UploadThroughputMBps > best.UploadThroughputMBps {
			best, haveOne = res, true
		}
	}
	if !haveOne {
		return BenchmarkResult{}, lastErr
	}
	return best, nil
}

// plantCorpus materializes a shared, reproducible pkg/corpus profile and returns
// its files (with sha256 sums, for byte-verify), a spec describing it, and the
// directory it was planted in. It re-plants fresh each run so the corpus is
// deterministic regardless of prior state.
func plantCorpus(config *BenchmarkConfig) ([]corpus.File, ScenarioSpec, string, error) {
	prof, ok := corpus.ProfileByName(config.Corpus)
	if !ok {
		return nil, ScenarioSpec{}, "", fmt.Errorf("unknown profile (want many-tiny|few-large|mixed|hostile)")
	}
	dir := filepath.Join(config.DataDir, "corpus-"+config.Corpus)
	if err := os.RemoveAll(dir); err != nil {
		return nil, ScenarioSpec{}, "", fmt.Errorf("clear corpus dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, ScenarioSpec{}, "", fmt.Errorf("create corpus dir: %w", err)
	}
	files, err := prof.Plant(dir)
	if err != nil {
		return nil, ScenarioSpec{}, "", fmt.Errorf("plant: %w", err)
	}
	var total int64
	for _, f := range files {
		total += int64(f.Size)
	}
	return files, ScenarioSpec{Name: prof.Name, FileCount: len(files), TotalSize: total}, dir, nil
}

func parseFlags() *BenchmarkConfig {
	config := &BenchmarkConfig{}

	flag.StringVar(&config.Scenario, "scenario", "small", "Benchmark scenario (small, medium, large, xlarge)")
	flag.StringVar(&config.Bucket, "bucket", "", "S3 bucket for testing (required)")
	flag.StringVar(&config.Prefix, "prefix", "cargohold-benchmark", "S3 prefix")
	flag.StringVar(&config.DataDir, "data-dir", "./test-data", "Local data directory")
	flag.StringVar(&config.ResultsDir, "results-dir", "./results", "Results output directory")
	flag.IntVar(&config.Concurrency, "concurrency", 10, "Parallel operations")
	flag.IntVar(&config.Iterations, "iterations", 3, "Number of runs per test")
	flag.StringVar(&config.Corpus, "corpus", "", "shared pkg/corpus profile (many-tiny|few-large|mixed|hostile); enables byte-verify. Empty uses the legacy scenario generator")
	flag.BoolVar(&config.CloudWatchMetrics, "cloudwatch-metrics", false, "read each tool's server-side S3 request counts (and itemized request cost) from CloudWatch. Real-AWS only; best with -iterations 1")
	flag.DurationVar(&config.MetricsSettle, "metrics-settle", 10*time.Minute, "how long to wait after the runs for CloudWatch metrics to publish before reading")

	var tools string
	var strategies string
	flag.StringVar(&tools, "tools", "cargohold,s5cmd,mc,tar", "Comma-separated list of tools to benchmark")
	flag.StringVar(&strategies, "strategies", "hash,size,type,adaptive", "Comma-separated shard strategies (CargoHold only)")

	flag.Parse()

	// Parse comma-separated values
	config.Tools = parseCommaSeparated(tools)
	config.ShardStrategies = parseCommaSeparated(strategies)

	return config
}

func validateConfig(config *BenchmarkConfig) error {
	if config.Bucket == "" {
		return fmt.Errorf("bucket is required (use -bucket flag)")
	}

	if _, ok := scenarios[config.Scenario]; !ok {
		return fmt.Errorf("unknown scenario: %s (must be small, medium, large, or xlarge)", config.Scenario)
	}

	return nil
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}

	var result []string
	for _, part := range splitComma(s) {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitComma(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func saveResults(resultsDir, scenario string, results []BenchmarkResult) error {
	filename := filepath.Join(resultsDir, fmt.Sprintf("%s_%s.json", scenario, time.Now().Format("20060102-150405")))

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	log.Printf("💾 Results saved to: %s", filename)
	return nil
}

func printResult(result BenchmarkResult) {
	log.Printf("   ✅ %s completed", result.Tool)
	if result.Strategy != "" {
		log.Printf("      Strategy: %s", result.Strategy)
	}
	log.Printf("      Upload Time: %s", result.UploadDuration.Round(time.Millisecond))
	log.Printf("      Throughput: %.1f MB/s", result.UploadThroughputMBps)
	log.Printf("      Memory: %.1f MB", result.PeakMemoryMB)
	log.Printf("      CPU: %.1f%%", result.AvgCPUPercent)
	if result.Verified {
		log.Printf("      Verified: ✅ %d files byte-identical", result.VerifiedFiles)
	}
	if result.ErrorCount > 0 {
		log.Printf("      Errors: %d", result.ErrorCount)
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
