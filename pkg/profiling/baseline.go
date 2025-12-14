package profiling

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Baseline represents a performance baseline for regression detection
type Baseline struct {
	// Version of the software
	Version string `json:"version"`

	// Timestamp when baseline was captured
	Timestamp time.Time `json:"timestamp"`

	// Environment information
	Environment EnvironmentInfo `json:"environment"`

	// Benchmarks maps benchmark name to metrics
	Benchmarks map[string]BenchmarkMetrics `json:"benchmarks"`
}

// BenchmarkMetrics contains performance metrics for a benchmark
type BenchmarkMetrics struct {
	// NsPerOp nanoseconds per operation
	NsPerOp float64 `json:"ns_per_op"`

	// AllocedBytesPerOp bytes allocated per operation
	AllocedBytesPerOp float64 `json:"alloced_bytes_per_op"`

	// AllocsPerOp allocations per operation
	AllocsPerOp float64 `json:"allocs_per_op"`

	// MBPerSec throughput in MB/s (if applicable)
	MBPerSec float64 `json:"mb_per_sec,omitempty"`
}

// EnvironmentInfo captures system information
type EnvironmentInfo struct {
	// GoVersion Go runtime version
	GoVersion string `json:"go_version"`

	// OS operating system
	OS string `json:"os"`

	// Arch CPU architecture
	Arch string `json:"arch"`

	// NumCPU number of CPUs
	NumCPU int `json:"num_cpu"`

	// Hostname machine hostname
	Hostname string `json:"hostname"`
}

// CaptureEnvironment captures current environment information
func CaptureEnvironment() EnvironmentInfo {
	hostname, _ := os.Hostname()

	return EnvironmentInfo{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		Hostname:  hostname,
	}
}

// NewBaseline creates a new baseline
func NewBaseline(version string) *Baseline {
	return &Baseline{
		Version:     version,
		Timestamp:   time.Now(),
		Environment: CaptureEnvironment(),
		Benchmarks:  make(map[string]BenchmarkMetrics),
	}
}

// AddBenchmark adds benchmark metrics to the baseline
func (b *Baseline) AddBenchmark(name string, metrics BenchmarkMetrics) {
	b.Benchmarks[name] = metrics
}

// SaveBaseline saves baseline to a JSON file
func SaveBaseline(baseline *Baseline, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}

	return nil
}

// LoadBaseline loads baseline from a JSON file
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to unmarshal baseline: %w", err)
	}

	return &baseline, nil
}

// Regression represents a detected performance regression
type Regression struct {
	// BenchmarkName that regressed
	BenchmarkName string

	// Metric that regressed
	Metric string

	// BaselineValue previous value
	BaselineValue float64

	// CurrentValue new value
	CurrentValue float64

	// PercentChange percentage change
	PercentChange float64

	// Threshold that was exceeded
	Threshold float64

	// Severity of the regression
	Severity RegressionSeverity
}

// RegressionSeverity indicates severity level
type RegressionSeverity string

const (
	SeverityLow      RegressionSeverity = "low"
	SeverityMedium   RegressionSeverity = "medium"
	SeverityHigh     RegressionSeverity = "high"
	SeverityCritical RegressionSeverity = "critical"
)

// RegressionThresholds defines acceptable degradation
type RegressionThresholds struct {
	// TimeThreshold acceptable time increase (e.g., 0.10 = +10%)
	TimeThreshold float64

	// MemoryThreshold acceptable memory increase (e.g., 0.15 = +15%)
	MemoryThreshold float64

	// AllocThreshold acceptable allocation increase (e.g., 0.20 = +20%)
	AllocThreshold float64

	// ThroughputThreshold acceptable throughput decrease (e.g., -0.05 = -5%)
	ThroughputThreshold float64
}

// DefaultThresholds returns sensible default thresholds
func DefaultThresholds() RegressionThresholds {
	return RegressionThresholds{
		TimeThreshold:       0.10,  // +10% time increase
		MemoryThreshold:     0.15,  // +15% memory increase
		AllocThreshold:      0.20,  // +20% allocation increase
		ThroughputThreshold: -0.05, // -5% throughput decrease
	}
}

// DetectRegressions compares current metrics against baseline
func DetectRegressions(baseline *Baseline, current *Baseline, thresholds RegressionThresholds) []Regression {
	var regressions []Regression

	// Compare each benchmark
	for name, currentMetrics := range current.Benchmarks {
		baselineMetrics, exists := baseline.Benchmarks[name]
		if !exists {
			// New benchmark, skip
			continue
		}

		// Check time regression
		if baselineMetrics.NsPerOp > 0 {
			percentChange := (currentMetrics.NsPerOp - baselineMetrics.NsPerOp) / baselineMetrics.NsPerOp
			if percentChange > thresholds.TimeThreshold {
				regressions = append(regressions, Regression{
					BenchmarkName: name,
					Metric:        "Time",
					BaselineValue: baselineMetrics.NsPerOp,
					CurrentValue:  currentMetrics.NsPerOp,
					PercentChange: percentChange * 100,
					Threshold:     thresholds.TimeThreshold * 100,
					Severity:      calculateSeverity(percentChange, thresholds.TimeThreshold),
				})
			}
		}

		// Check memory regression
		if baselineMetrics.AllocedBytesPerOp > 0 {
			percentChange := (currentMetrics.AllocedBytesPerOp - baselineMetrics.AllocedBytesPerOp) / baselineMetrics.AllocedBytesPerOp
			if percentChange > thresholds.MemoryThreshold {
				regressions = append(regressions, Regression{
					BenchmarkName: name,
					Metric:        "Memory",
					BaselineValue: baselineMetrics.AllocedBytesPerOp,
					CurrentValue:  currentMetrics.AllocedBytesPerOp,
					PercentChange: percentChange * 100,
					Threshold:     thresholds.MemoryThreshold * 100,
					Severity:      calculateSeverity(percentChange, thresholds.MemoryThreshold),
				})
			}
		}

		// Check allocation regression
		if baselineMetrics.AllocsPerOp > 0 {
			percentChange := (currentMetrics.AllocsPerOp - baselineMetrics.AllocsPerOp) / baselineMetrics.AllocsPerOp
			if percentChange > thresholds.AllocThreshold {
				regressions = append(regressions, Regression{
					BenchmarkName: name,
					Metric:        "Allocations",
					BaselineValue: baselineMetrics.AllocsPerOp,
					CurrentValue:  currentMetrics.AllocsPerOp,
					PercentChange: percentChange * 100,
					Threshold:     thresholds.AllocThreshold * 100,
					Severity:      calculateSeverity(percentChange, thresholds.AllocThreshold),
				})
			}
		}

		// Check throughput regression (inverse - lower is worse)
		if baselineMetrics.MBPerSec > 0 && currentMetrics.MBPerSec > 0 {
			percentChange := (currentMetrics.MBPerSec - baselineMetrics.MBPerSec) / baselineMetrics.MBPerSec
			if percentChange < thresholds.ThroughputThreshold {
				regressions = append(regressions, Regression{
					BenchmarkName: name,
					Metric:        "Throughput",
					BaselineValue: baselineMetrics.MBPerSec,
					CurrentValue:  currentMetrics.MBPerSec,
					PercentChange: percentChange * 100,
					Threshold:     thresholds.ThroughputThreshold * 100,
					Severity:      calculateSeverity(-percentChange, -thresholds.ThroughputThreshold),
				})
			}
		}
	}

	return regressions
}

// calculateSeverity determines regression severity
func calculateSeverity(percentChange, threshold float64) RegressionSeverity {
	// Normalize to positive for comparison
	absChange := percentChange
	if absChange < 0 {
		absChange = -absChange
	}
	absThreshold := threshold
	if absThreshold < 0 {
		absThreshold = -absThreshold
	}

	// Calculate how much the change exceeds the threshold
	exceedance := (absChange - absThreshold) / absThreshold

	switch {
	case exceedance > 1.0: // More than 2x threshold
		return SeverityCritical
	case exceedance > 0.5: // 1.5x threshold
		return SeverityHigh
	case exceedance > 0.2: // 1.2x threshold
		return SeverityMedium
	default:
		return SeverityLow
	}
}

// FormatRegressions formats regressions for display
func FormatRegressions(regressions []Regression) string {
	if len(regressions) == 0 {
		return "✅ No regressions detected!\n"
	}

	output := fmt.Sprintf("⚠️  %d Regression(s) Detected:\n\n", len(regressions))

	// Group by severity
	bySeverity := make(map[RegressionSeverity][]Regression)
	for _, reg := range regressions {
		bySeverity[reg.Severity] = append(bySeverity[reg.Severity], reg)
	}

	// Report in severity order
	severityOrder := []RegressionSeverity{
		SeverityCritical,
		SeverityHigh,
		SeverityMedium,
		SeverityLow,
	}

	for _, severity := range severityOrder {
		regs := bySeverity[severity]
		if len(regs) == 0 {
			continue
		}

		output += fmt.Sprintf("🔴 %s Severity (%d):\n", severity, len(regs))
		for _, reg := range regs {
			output += fmt.Sprintf("  • %s - %s\n", reg.BenchmarkName, reg.Metric)
			output += fmt.Sprintf("    Baseline: %.2f → Current: %.2f\n", reg.BaselineValue, reg.CurrentValue)
			output += fmt.Sprintf("    Change: %+.2f%% (threshold: %+.2f%%)\n\n", reg.PercentChange, reg.Threshold)
		}
	}

	return output
}

// ExitCodeFromRegressions returns appropriate exit code based on severity
func ExitCodeFromRegressions(regressions []Regression) int {
	if len(regressions) == 0 {
		return 0
	}

	// Check for critical regressions
	for _, reg := range regressions {
		if reg.Severity == SeverityCritical {
			return 2 // Critical failure
		}
	}

	// Check for high severity
	for _, reg := range regressions {
		if reg.Severity == SeverityHigh {
			return 1 // Warning
		}
	}

	return 0 // Low/medium severity, pass but warn
}
