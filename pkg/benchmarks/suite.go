// Package benchmarks provides a comprehensive benchmarking framework for CargoShip
// with automated regression detection and performance tracking.
package benchmarks

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Suite orchestrates benchmark execution and result collection
type Suite struct {
	scenarios []Scenario
	config    SuiteConfig
}

// SuiteConfig configures benchmark suite behavior
type SuiteConfig struct {
	// OutputFormat specifies output format (text, json, markdown)
	OutputFormat string

	// BaselineFile path to baseline metrics file
	BaselineFile string

	// EnableRegression enables regression detection
	EnableRegression bool

	// RegressionThresholds defines acceptable performance degradation
	RegressionThresholds Thresholds
}

// Scenario represents a single benchmark test case
type Scenario struct {
	// Name identifies the scenario
	Name string

	// Description explains what this scenario tests
	Description string

	// FileSize in bytes for the upload test
	FileSize int64

	// Concurrency level for parallel operations
	Concurrency int

	// ChunkSize for multipart uploads
	ChunkSize int64

	// Setup prepares the test environment (optional)
	Setup func(ctx context.Context) error

	// Teardown cleans up after the test (optional)
	Teardown func(ctx context.Context) error

	// Validate checks if the result is valid (optional)
	Validate func(result *Result) error
}

// Result contains benchmark execution results
type Result struct {
	// Scenario that produced this result
	ScenarioName string

	// Metrics collected during execution
	Metrics Metrics

	// Timestamp when benchmark ran
	Timestamp time.Time

	// Iterations performed
	Iterations int

	// Error if benchmark failed
	Error error
}

// Metrics tracks performance measurements
type Metrics struct {
	// Throughput in MB/s
	Throughput float64

	// Latency measurements
	Latency LatencyMetrics

	// Memory usage in bytes
	MemoryUsed uint64

	// Memory allocations count
	Allocations int64

	// Bytes allocated
	BytesAllocated uint64

	// Goroutines active
	Goroutines int

	// GC statistics
	GCStats GCMetrics
}

// LatencyMetrics tracks latency percentiles
type LatencyMetrics struct {
	// P50 median latency
	P50 time.Duration

	// P95 95th percentile
	P95 time.Duration

	// P99 99th percentile
	P99 time.Duration

	// Min minimum observed
	Min time.Duration

	// Max maximum observed
	Max time.Duration

	// Mean average latency
	Mean time.Duration
}

// GCMetrics tracks garbage collection statistics
type GCMetrics struct {
	// NumGC number of GC cycles
	NumGC uint32

	// PauseTotal total GC pause time
	PauseTotal time.Duration

	// PauseMean average pause time
	PauseMean time.Duration
}

// BaselineMetrics stores historical baseline for comparison
type BaselineMetrics struct {
	// Version of CargoShip when baseline was established
	Version string

	// Timestamp when baseline was recorded
	Timestamp time.Time

	// Scenarios maps scenario name to baseline metrics
	Scenarios map[string]Metrics

	// Environment information
	Environment EnvironmentInfo
}

// EnvironmentInfo captures test environment details
type EnvironmentInfo struct {
	// GoVersion Go runtime version
	GoVersion string

	// OS operating system
	OS string

	// Arch CPU architecture
	Arch string

	// NumCPU number of CPUs
	NumCPU int

	// MemoryTotal total system memory
	MemoryTotal uint64
}

// Thresholds defines acceptable performance degradation
type Thresholds struct {
	// ThroughputDelta acceptable throughput decrease (e.g., -0.05 = -5%)
	ThroughputDelta float64

	// LatencyDelta acceptable latency increase (e.g., 0.10 = +10%)
	LatencyDelta float64

	// MemoryDelta acceptable memory increase (e.g., 0.15 = +15%)
	MemoryDelta float64

	// AllocationDelta acceptable allocation increase (e.g., 0.20 = +20%)
	AllocationDelta float64
}

// NewSuite creates a new benchmark suite
func NewSuite(config SuiteConfig) *Suite {
	return &Suite{
		scenarios: make([]Scenario, 0),
		config:    config,
	}
}

// AddScenario adds a benchmark scenario to the suite
func (s *Suite) AddScenario(scenario Scenario) {
	s.scenarios = append(s.scenarios, scenario)
}

// Run executes all scenarios and collects results
func (s *Suite) Run(ctx context.Context) ([]*Result, error) {
	results := make([]*Result, 0, len(s.scenarios))

	for _, scenario := range s.scenarios {
		result, err := s.runScenario(ctx, scenario)
		if err != nil {
			return nil, fmt.Errorf("scenario %s failed: %w", scenario.Name, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// runScenario executes a single scenario
func (s *Suite) runScenario(ctx context.Context, scenario Scenario) (*Result, error) {
	result := &Result{
		ScenarioName: scenario.Name,
		Timestamp:    time.Now(),
	}

	// Setup
	if scenario.Setup != nil {
		if err := scenario.Setup(ctx); err != nil {
			result.Error = err
			return result, err
		}
	}

	// Defer teardown
	if scenario.Teardown != nil {
		defer func() {
			if err := scenario.Teardown(ctx); err != nil {
				// Log teardown error but don't fail the benchmark
				fmt.Printf("Warning: teardown failed for %s: %v\n", scenario.Name, err)
			}
		}()
	}

	// Execute benchmark
	// Note: Actual benchmark execution will be implemented in specific benchmark tests
	// This is the orchestration framework

	return result, nil
}

// LoadBaseline loads baseline metrics from file
func (s *Suite) LoadBaseline(path string) error {
	// Implementation will be added with JSON marshaling
	return fmt.Errorf("not implemented")
}

// SaveBaseline saves current metrics as baseline
func (s *Suite) SaveBaseline(path string, results []*Result) error {
	// Implementation will be added with JSON marshaling
	return fmt.Errorf("not implemented")
}

// Report generates a formatted report of results
type Report struct {
	// Results from benchmark execution
	Results []*Result

	// Baseline for comparison (optional)
	Baseline *BaselineMetrics

	// Regressions detected (if enabled)
	Regressions []Regression

	// Summary statistics
	Summary Summary
}

// Summary provides aggregate statistics
type Summary struct {
	// TotalScenarios executed
	TotalScenarios int

	// PassedScenarios count
	PassedScenarios int

	// FailedScenarios count
	FailedScenarios int

	// Regressions detected
	Regressions int

	// ExecutionTime total time
	ExecutionTime time.Duration
}

// Reporter generates formatted reports
type Reporter interface {
	// Generate creates a report from results
	Generate(results []*Result, baseline *BaselineMetrics) (*Report, error)

	// Write outputs the report
	Write(w io.Writer, report *Report) error
}

// TextReporter generates plain text reports
type TextReporter struct{}

// Generate implements Reporter
func (tr *TextReporter) Generate(results []*Result, baseline *BaselineMetrics) (*Report, error) {
	return &Report{
		Results:  results,
		Baseline: baseline,
	}, nil
}

// Write implements Reporter
func (tr *TextReporter) Write(w io.Writer, report *Report) error {
	_, _ = fmt.Fprintf(w, "CargoShip Benchmark Report\n")
	_, _ = fmt.Fprintf(w, "==========================\n\n")

	for _, result := range report.Results {
		_, _ = fmt.Fprintf(w, "Scenario: %s\n", result.ScenarioName)
		_, _ = fmt.Fprintf(w, "  Throughput: %.2f MB/s\n", result.Metrics.Throughput)
		_, _ = fmt.Fprintf(w, "  Latency (p50/p95/p99): %v / %v / %v\n",
			result.Metrics.Latency.P50,
			result.Metrics.Latency.P95,
			result.Metrics.Latency.P99)
		_, _ = fmt.Fprintf(w, "  Memory: %d bytes\n", result.Metrics.MemoryUsed)
		_, _ = fmt.Fprintf(w, "  Allocations: %d\n", result.Metrics.Allocations)
		_, _ = fmt.Fprintf(w, "\n")
	}

	if len(report.Regressions) > 0 {
		_, _ = fmt.Fprintf(w, "⚠️  Regressions Detected: %d\n", len(report.Regressions))
		for _, reg := range report.Regressions {
			_, _ = fmt.Fprintf(w, "  - %s: %s changed by %.2f%% (%.2f → %.2f, threshold: %.2f%%)\n",
				reg.ScenarioName, reg.Metric, reg.PercentChange,
				reg.BaselineValue, reg.CurrentValue, reg.Threshold*100)
		}
	}

	return nil
}

// DefaultThresholds returns sensible default regression thresholds
func DefaultThresholds() Thresholds {
	return Thresholds{
		ThroughputDelta: -0.05, // -5% throughput loss acceptable
		LatencyDelta:    0.10,  // +10% latency increase acceptable
		MemoryDelta:     0.15,  // +15% memory increase acceptable
		AllocationDelta: 0.20,  // +20% allocation increase acceptable
	}
}
