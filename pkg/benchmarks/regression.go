package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// RegressionDetector compares current benchmark results against baseline
type RegressionDetector struct {
	baseline   *BaselineMetrics
	thresholds Thresholds
}

// NewRegressionDetector creates a new regression detector
func NewRegressionDetector(baseline *BaselineMetrics, thresholds Thresholds) *RegressionDetector {
	return &RegressionDetector{
		baseline:   baseline,
		thresholds: thresholds,
	}
}

// Check compares current results against baseline and detects regressions
func (rd *RegressionDetector) Check(results []*Result) (*RegressionReport, error) {
	if rd.baseline == nil {
		return nil, fmt.Errorf("no baseline loaded")
	}

	report := &RegressionReport{
		Timestamp:   time.Now(),
		Baseline:    rd.baseline,
		Results:     results,
		Regressions: make([]Regression, 0),
	}

	// Check each result against baseline
	for _, result := range results {
		baselineMetrics, exists := rd.baseline.Scenarios[result.ScenarioName]
		if !exists {
			// New scenario, no baseline to compare
			continue
		}

		// Check throughput regression
		if reg := rd.checkThroughput(result.ScenarioName, baselineMetrics, result.Metrics); reg != nil {
			report.Regressions = append(report.Regressions, *reg)
		}

		// Check latency regression
		if reg := rd.checkLatency(result.ScenarioName, baselineMetrics, result.Metrics); reg != nil {
			report.Regressions = append(report.Regressions, *reg)
		}

		// Check memory regression
		if reg := rd.checkMemory(result.ScenarioName, baselineMetrics, result.Metrics); reg != nil {
			report.Regressions = append(report.Regressions, *reg)
		}

		// Check allocation regression
		if reg := rd.checkAllocations(result.ScenarioName, baselineMetrics, result.Metrics); reg != nil {
			report.Regressions = append(report.Regressions, *reg)
		}
	}

	return report, nil
}

// checkThroughput detects throughput regressions
func (rd *RegressionDetector) checkThroughput(scenarioName string, baseline, current Metrics) *Regression {
	if baseline.Throughput == 0 {
		return nil // No baseline throughput
	}

	percentChange := (current.Throughput - baseline.Throughput) / baseline.Throughput

	// Check if throughput decreased beyond threshold (negative is bad)
	if percentChange < rd.thresholds.ThroughputDelta {
		return &Regression{
			ScenarioName:  scenarioName,
			Metric:        "Throughput",
			BaselineValue: baseline.Throughput,
			CurrentValue:  current.Throughput,
			PercentChange: percentChange * 100,
			Threshold:     rd.thresholds.ThroughputDelta * 100,
			Severity:      calculateSeverity(percentChange, rd.thresholds.ThroughputDelta),
		}
	}

	return nil
}

// checkLatency detects latency regressions
func (rd *RegressionDetector) checkLatency(scenarioName string, baseline, current Metrics) *Regression {
	if baseline.Latency.P95 == 0 {
		return nil // No baseline latency
	}

	// Use P95 latency for comparison
	baselineLatency := float64(baseline.Latency.P95.Milliseconds())
	currentLatency := float64(current.Latency.P95.Milliseconds())

	percentChange := (currentLatency - baselineLatency) / baselineLatency

	// Check if latency increased beyond threshold (positive is bad)
	if percentChange > rd.thresholds.LatencyDelta {
		return &Regression{
			ScenarioName:  scenarioName,
			Metric:        "Latency (P95)",
			BaselineValue: baselineLatency,
			CurrentValue:  currentLatency,
			PercentChange: percentChange * 100,
			Threshold:     rd.thresholds.LatencyDelta * 100,
			Severity:      calculateSeverity(percentChange, rd.thresholds.LatencyDelta),
		}
	}

	return nil
}

// checkMemory detects memory usage regressions
func (rd *RegressionDetector) checkMemory(scenarioName string, baseline, current Metrics) *Regression {
	if baseline.MemoryUsed == 0 {
		return nil // No baseline memory
	}

	baselineMem := float64(baseline.MemoryUsed)
	currentMem := float64(current.MemoryUsed)

	percentChange := (currentMem - baselineMem) / baselineMem

	// Check if memory usage increased beyond threshold
	if percentChange > rd.thresholds.MemoryDelta {
		return &Regression{
			ScenarioName:  scenarioName,
			Metric:        "Memory Usage",
			BaselineValue: baselineMem,
			CurrentValue:  currentMem,
			PercentChange: percentChange * 100,
			Threshold:     rd.thresholds.MemoryDelta * 100,
			Severity:      calculateSeverity(percentChange, rd.thresholds.MemoryDelta),
		}
	}

	return nil
}

// checkAllocations detects allocation regressions
func (rd *RegressionDetector) checkAllocations(scenarioName string, baseline, current Metrics) *Regression {
	if baseline.Allocations == 0 {
		return nil // No baseline allocations
	}

	baselineAllocs := float64(baseline.Allocations)
	currentAllocs := float64(current.Allocations)

	percentChange := (currentAllocs - baselineAllocs) / baselineAllocs

	// Check if allocations increased beyond threshold
	if percentChange > rd.thresholds.AllocationDelta {
		return &Regression{
			ScenarioName:  scenarioName,
			Metric:        "Allocations",
			BaselineValue: baselineAllocs,
			CurrentValue:  currentAllocs,
			PercentChange: percentChange * 100,
			Threshold:     rd.thresholds.AllocationDelta * 100,
			Severity:      calculateSeverity(percentChange, rd.thresholds.AllocationDelta),
		}
	}

	return nil
}

// calculateSeverity determines regression severity
func calculateSeverity(percentChange, threshold float64) RegressionSeverity {
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

// RegressionReport contains regression detection results
type RegressionReport struct {
	// Timestamp when report was generated
	Timestamp time.Time

	// Baseline used for comparison
	Baseline *BaselineMetrics

	// Results being checked
	Results []*Result

	// Regressions detected
	Regressions []Regression

	// Summary statistics
	Summary RegressionSummary
}

// Regression represents a detected performance regression
type Regression struct {
	// ScenarioName that regressed
	ScenarioName string

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

// RegressionSeverity indicates how severe a regression is
type RegressionSeverity string

const (
	// SeverityLow minor regression, just over threshold
	SeverityLow RegressionSeverity = "low"

	// SeverityMedium moderate regression
	SeverityMedium RegressionSeverity = "medium"

	// SeverityHigh significant regression
	SeverityHigh RegressionSeverity = "high"

	// SeverityCritical severe regression
	SeverityCritical RegressionSeverity = "critical"
)

// RegressionSummary provides aggregate regression statistics
type RegressionSummary struct {
	// TotalScenarios checked
	TotalScenarios int

	// TotalRegressions detected
	TotalRegressions int

	// RegressionsByMetric counts by metric type
	RegressionsByMetric map[string]int

	// RegressionsBySeverity counts by severity
	RegressionsBySeverity map[RegressionSeverity]int
}

// LoadBaseline loads baseline metrics from a JSON file
func LoadBaseline(path string) (*BaselineMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var baseline BaselineMetrics
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline file: %w", err)
	}

	return &baseline, nil
}

// SaveBaseline saves baseline metrics to a JSON file
func SaveBaseline(path string, baseline *BaselineMetrics) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}

	return nil
}

// CreateBaselineFromResults creates a baseline from benchmark results
func CreateBaselineFromResults(version string, results []*Result, env EnvironmentInfo) *BaselineMetrics {
	baseline := &BaselineMetrics{
		Version:     version,
		Timestamp:   time.Now(),
		Scenarios:   make(map[string]Metrics),
		Environment: env,
	}

	for _, result := range results {
		baseline.Scenarios[result.ScenarioName] = result.Metrics
	}

	return baseline
}

// FormatRegressionReport formats a regression report for display
func FormatRegressionReport(report *RegressionReport) string {
	output := "Regression Detection Report\n"
	output += "============================\n\n"
	output += fmt.Sprintf("Generated: %s\n", report.Timestamp.Format(time.RFC3339))
	output += fmt.Sprintf("Baseline: %s (%s)\n\n", report.Baseline.Version, report.Baseline.Timestamp.Format("2006-01-02"))

	if len(report.Regressions) == 0 {
		output += "✅ No regressions detected!\n"
		return output
	}

	output += fmt.Sprintf("⚠️  %d Regression(s) Detected:\n\n", len(report.Regressions))

	// Group by severity
	bySeverity := make(map[RegressionSeverity][]Regression)
	for _, reg := range report.Regressions {
		bySeverity[reg.Severity] = append(bySeverity[reg.Severity], reg)
	}

	// Report critical first
	severityOrder := []RegressionSeverity{
		SeverityCritical,
		SeverityHigh,
		SeverityMedium,
		SeverityLow,
	}

	for _, severity := range severityOrder {
		regressions := bySeverity[severity]
		if len(regressions) == 0 {
			continue
		}

		output += fmt.Sprintf("🔴 %s Severity (%d):\n", severity, len(regressions))
		for _, reg := range regressions {
			output += fmt.Sprintf("  • %s - %s\n", reg.ScenarioName, reg.Metric)
			output += fmt.Sprintf("    Baseline: %.2f → Current: %.2f\n", reg.BaselineValue, reg.CurrentValue)
			output += fmt.Sprintf("    Change: %+.2f%% (threshold: %+.2f%%)\n\n", reg.PercentChange, reg.Threshold)
		}
	}

	return output
}

// ExitCodeFromReport returns an appropriate exit code based on regression severity
func ExitCodeFromReport(report *RegressionReport) int {
	if len(report.Regressions) == 0 {
		return 0 // Success
	}

	// Check for critical regressions
	for _, reg := range report.Regressions {
		if reg.Severity == SeverityCritical {
			return 2 // Critical failure
		}
	}

	// Check for high severity
	for _, reg := range report.Regressions {
		if reg.Severity == SeverityHigh {
			return 1 // Warning
		}
	}

	return 0 // Low/medium severity, pass but warn
}

// GenerateSummary generates summary statistics for a regression report
func (rr *RegressionReport) GenerateSummary() RegressionSummary {
	summary := RegressionSummary{
		TotalScenarios:        len(rr.Results),
		TotalRegressions:      len(rr.Regressions),
		RegressionsByMetric:   make(map[string]int),
		RegressionsBySeverity: make(map[RegressionSeverity]int),
	}

	for _, reg := range rr.Regressions {
		summary.RegressionsByMetric[reg.Metric]++
		summary.RegressionsBySeverity[reg.Severity]++
	}

	return summary
}
