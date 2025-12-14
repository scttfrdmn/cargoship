// Package main provides comprehensive CargoHold performance benchmarking
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
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
	Timestamp              time.Time     `json:"timestamp"`
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

	// Generate or verify test data
	spec := scenarios[config.Scenario]
	log.Printf("📊 Test Scenario: %s", spec.Name)
	log.Printf("   Files: %d", spec.FileCount)
	log.Printf("   Total Size: %s\n", formatBytes(spec.TotalSize))

	dataDir := filepath.Join(config.DataDir, config.Scenario)
	if err := ensureTestData(dataDir, spec); err != nil {
		log.Fatalf("Failed to prepare test data: %v", err)
	}

	// Run benchmarks
	var results []BenchmarkResult

	for _, tool := range config.Tools {
		if tool == "cargohold" {
			// Test all shard strategies for CargoHold
			for _, strategy := range config.ShardStrategies {
				log.Printf("\n🔬 Benchmarking CargoHold with %s strategy...", strategy)
				result, err := runCargoHoldBenchmark(config, spec, dataDir, strategy)
				if err != nil {
					log.Printf("❌ CargoHold %s failed: %v", strategy, err)
					continue
				}
				results = append(results, result)
				printResult(result)
			}
		} else {
			log.Printf("\n🔬 Benchmarking %s...", tool)
			result, err := runToolBenchmark(tool, config, spec, dataDir)
			if err != nil {
				log.Printf("❌ %s failed: %v", tool, err)
				continue
			}
			results = append(results, result)
			printResult(result)
		}
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

func parseFlags() *BenchmarkConfig {
	config := &BenchmarkConfig{}

	flag.StringVar(&config.Scenario, "scenario", "small", "Benchmark scenario (small, medium, large, xlarge)")
	flag.StringVar(&config.Bucket, "bucket", "", "S3 bucket for testing (required)")
	flag.StringVar(&config.Prefix, "prefix", "cargohold-benchmark", "S3 prefix")
	flag.StringVar(&config.DataDir, "data-dir", "./test-data", "Local data directory")
	flag.StringVar(&config.ResultsDir, "results-dir", "./results", "Results output directory")
	flag.IntVar(&config.Concurrency, "concurrency", 10, "Parallel operations")
	flag.IntVar(&config.Iterations, "iterations", 3, "Number of runs per test")

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
