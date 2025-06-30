package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/compression"
)

var (
	benchmarkSize     string
	benchmarkDataType string
	benchmarkFormat   string
	benchmarkFile     string
)

// NewBenchmarkCmd creates the benchmark command for compression algorithms
func NewBenchmarkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark compression algorithms",
		Long: `Benchmark different compression algorithms to find the optimal one for your data.

This command tests various compression algorithms (gzip, zlib, zstd, lz4, s2) 
with different compression levels to help you choose the best algorithm based
on your performance and size requirements.

Examples:
  # Benchmark with random data
  cargoship benchmark --size 10MB
  
  # Benchmark with specific data type simulation
  cargoship benchmark --size 50MB --data-type text
  
  # Benchmark using a real file
  cargoship benchmark --file /path/to/data.tar
  
  # Output results in JSON format
  cargoship benchmark --size 1GB --format json`,
		RunE: runBenchmark,
	}

	cmd.Flags().StringVar(&benchmarkSize, "size", "10MB", "Size of test data to generate (e.g., 1MB, 10MB, 1GB)")
	cmd.Flags().StringVar(&benchmarkDataType, "data-type", "mixed", "Type of data to simulate (text, binary, mixed, random)")
	cmd.Flags().StringVar(&benchmarkFormat, "format", "table", "Output format (table, json)")
	cmd.Flags().StringVar(&benchmarkFile, "file", "", "Use real file instead of generated data")

	return cmd
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	fmt.Printf("🚀 Starting compression algorithm benchmark...\n\n")

	var data []byte
	var dataSize int64
	var err error

	if benchmarkFile != "" {
		// Use real file
		fmt.Printf("📁 Loading data from file: %s\n", benchmarkFile)
		data, err = os.ReadFile(benchmarkFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		dataSize = int64(len(data))
		fmt.Printf("   File size: %s\n\n", formatBytes(dataSize))
	} else {
		// Generate test data
		dataSize, err = parseBytes(benchmarkSize)
		if err != nil {
			return fmt.Errorf("invalid size: %w", err)
		}

		fmt.Printf("📊 Generating %s of %s data...\n", benchmarkSize, benchmarkDataType)
		data, err = generateTestData(dataSize, benchmarkDataType)
		if err != nil {
			return fmt.Errorf("failed to generate test data: %w", err)
		}
		fmt.Printf("   Generated: %s\n\n", formatBytes(int64(len(data))))
	}

	// Run benchmark
	dataReader := bytes.NewReader(data)
	results, err := compression.BenchmarkCompression(dataReader, dataSize)
	if err != nil {
		return fmt.Errorf("benchmark failed: %w", err)
	}

	// Output results
	switch benchmarkFormat {
	case "json":
		return outputBenchmarkJSON(results)
	case "table":
		return outputBenchmarkTable(results, dataSize)
	default:
		return fmt.Errorf("unsupported format: %s", benchmarkFormat)
	}
}

func generateTestData(size int64, dataType string) ([]byte, error) {
	data := make([]byte, size)

	switch dataType {
	case "random":
		// Completely random data (worst case for compression)
		if _, err := rand.Read(data); err != nil {
			return nil, fmt.Errorf("failed to generate random data: %w", err)
		}

	case "text":
		// Simulate text data with repeated patterns
		pattern := []byte("The quick brown fox jumps over the lazy dog. This is a test of compression algorithms with repeated text patterns. ")
		for i := int64(0); i < size; i++ {
			data[i] = pattern[i%int64(len(pattern))]
		}

	case "binary":
		// Simulate binary data with some patterns
		for i := int64(0); i < size; i++ {
			if i%1024 < 100 {
				data[i] = byte(i % 256) // Some pattern
			} else {
				data[i] = byte((i * 31) % 256) // Pseudo-random but deterministic
			}
		}

	case "mixed":
		// Mix of text and binary
		textPattern := []byte("Sample text content with some repetition. ")
		for i := int64(0); i < size; i++ {
			if i%100 < 80 {
				// 80% text-like data
				data[i] = textPattern[i%int64(len(textPattern))]
			} else {
				// 20% binary-like data
				data[i] = byte((i * 13) % 256)
			}
		}

	default:
		return nil, fmt.Errorf("unsupported data type: %s", dataType)
	}

	return data, nil
}

func outputBenchmarkTable(results []compression.CompressionResult, originalSize int64) error {
	fmt.Printf("📈 Compression Benchmark Results\n")
	fmt.Printf("   Original size: %s\n\n", formatBytes(originalSize))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header(
		"Algorithm",
		"Level", 
		"Compressed Size",
		"Ratio",
		"Time (ms)",
		"Speed (MB/s)",
		"Efficiency Score",
	)

	// Calculate efficiency scores and sort
	for i := range results {
		// Efficiency score: ratio / (time_factor) - higher is better
		timeFactor := float64(results[i].CompressionTime) / 1000.0 // Convert to seconds
		if timeFactor == 0 {
			timeFactor = 0.001 // Prevent division by zero
		}
		results[i].Throughput = results[i].CompressionRatio / timeFactor
	}

	// Add recommendation
	best := findBestAlgorithm(results)
	
	for _, result := range results {
		efficiency := fmt.Sprintf("%.1f", result.Throughput)
		if result.Algorithm == best.Algorithm && result.Level == best.Level {
			efficiency += " ⭐"
		}

		_ = table.Append(
			string(result.Algorithm),
			fmt.Sprintf("%d", result.Level),
			formatBytes(result.CompressedSize),
			fmt.Sprintf("%.2fx", result.CompressionRatio),
			fmt.Sprintf("%d", result.CompressionTime),
			fmt.Sprintf("%.1f", result.Throughput),
			efficiency,
		)
	}

	_ = table.Render()

	// Show recommendations
	fmt.Printf("\n🎯 Recommendations:\n")
	fmt.Printf("   Best Overall: %s (level %d) - %.2fx compression in %dms\n", 
		best.Algorithm, best.Level, best.CompressionRatio, best.CompressionTime)

	fastest := findFastestAlgorithm(results)
	fmt.Printf("   Fastest: %s (level %d) - %.1f MB/s\n", 
		fastest.Algorithm, fastest.Level, fastest.Throughput)

	bestRatio := findBestRatioAlgorithm(results) 
	fmt.Printf("   Best Compression: %s (level %d) - %.2fx ratio\n", 
		bestRatio.Algorithm, bestRatio.Level, bestRatio.CompressionRatio)

	return nil
}

func outputBenchmarkJSON(results []compression.CompressionResult) error {
	output := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"results":   results,
		"recommendations": map[string]interface{}{
			"best_overall":     findBestAlgorithm(results),
			"fastest":         findFastestAlgorithm(results),
			"best_compression": findBestRatioAlgorithm(results),
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func findBestAlgorithm(results []compression.CompressionResult) compression.CompressionResult {
	if len(results) == 0 {
		return compression.CompressionResult{}
	}

	best := results[0]
	bestScore := calculateScore(best)

	for _, result := range results[1:] {
		score := calculateScore(result)
		if score > bestScore {
			best = result
			bestScore = score
		}
	}

	return best
}

func findFastestAlgorithm(results []compression.CompressionResult) compression.CompressionResult {
	if len(results) == 0 {
		return compression.CompressionResult{}
	}

	fastest := results[0]
	for _, result := range results[1:] {
		if result.Throughput > fastest.Throughput {
			fastest = result
		}
	}

	return fastest
}

func findBestRatioAlgorithm(results []compression.CompressionResult) compression.CompressionResult {
	if len(results) == 0 {
		return compression.CompressionResult{}
	}

	best := results[0]
	for _, result := range results[1:] {
		if result.CompressionRatio > best.CompressionRatio {
			best = result
		}
	}

	return best
}

func calculateScore(result compression.CompressionResult) float64 {
	// Balance compression ratio and speed
	// Higher compression ratio is better, lower time is better
	ratioScore := result.CompressionRatio * 10 // Weight compression ratio
	speedScore := result.Throughput           // Weight speed
	
	return ratioScore + speedScore
}

func parseBytes(s string) (int64, error) {
	s = strings.ToUpper(s)
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	var value float64
	var unit string

	n, err := fmt.Sscanf(s, "%f%s", &value, &unit)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("invalid size format: %s", s)
	}

	multiplier, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}

	return int64(value * float64(multiplier)), nil
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

func init() {
	// This command will be added to root in root.go
}