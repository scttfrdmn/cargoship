package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RegressionReport compares current results against historical baseline
type RegressionReport struct {
	Timestamp time.Time `json:"timestamp"`
	Scenario  string    `json:"scenario"`
	Pass      bool      `json:"pass"`
	Comparisons []RegressionComparison `json:"comparisons"`
}

// RegressionComparison compares one tool's performance
type RegressionComparison struct {
	Tool            string  `json:"tool"`
	Strategy        string  `json:"strategy,omitempty"`
	BaselineTime    float64 `json:"baseline_time_seconds"`
	CurrentTime     float64 `json:"current_time_seconds"`
	ChangePercent   float64 `json:"change_percent"`
	Pass            bool    `json:"pass"`
	Threshold       float64 `json:"threshold"`
}

const (
	// Allow up to 10% regression
	defaultRegressionThreshold = 10.0
	// Baseline history directory
	baselineDir = "regression/baseline"
)

// checkRegression compares current results against baseline
func checkRegression(resultsDir, scenario string, results []BenchmarkResult) (*RegressionReport, error) {
	report := &RegressionReport{
		Timestamp:   time.Now(),
		Scenario:    scenario,
		Pass:        true,
		Comparisons: []RegressionComparison{},
	}

	// Load baseline
	baseline, err := loadBaseline(scenario)
	if err != nil {
		return nil, fmt.Errorf("failed to load baseline: %w", err)
	}

	if baseline == nil {
		// No baseline exists yet
		return report, nil
	}

	// Compare each result against baseline
	for _, current := range results {
		key := makeResultKey(current)
		baselineResult, found := baseline[key]

		if !found {
			// New tool/strategy, skip comparison
			continue
		}

		baselineTime := baselineResult.UploadDuration.Seconds()
		currentTime := current.UploadDuration.Seconds()

		changePercent := ((currentTime - baselineTime) / baselineTime) * 100.0

		comparison := RegressionComparison{
			Tool:          current.Tool,
			Strategy:      current.Strategy,
			BaselineTime:  baselineTime,
			CurrentTime:   currentTime,
			ChangePercent: changePercent,
			Pass:          changePercent <= defaultRegressionThreshold,
			Threshold:     defaultRegressionThreshold,
		}

		if !comparison.Pass {
			report.Pass = false
		}

		report.Comparisons = append(report.Comparisons, comparison)
	}

	// Save regression report
	if err := saveRegressionReport(resultsDir, report); err != nil {
		return nil, fmt.Errorf("failed to save regression report: %w", err)
	}

	return report, nil
}

// updateBaseline updates the baseline with current results if they're better
func updateBaseline(scenario string, results []BenchmarkResult) error {
	baseline, err := loadBaseline(scenario)
	if err != nil {
		return err
	}

	if baseline == nil {
		baseline = make(map[string]BenchmarkResult)
	}

	updated := false
	for _, result := range results {
		key := makeResultKey(result)
		existing, found := baseline[key]

		if !found || result.UploadDuration < existing.UploadDuration {
			// New result or better performance - update baseline
			baseline[key] = result
			updated = true
		}
	}

	if updated {
		return saveBaseline(scenario, baseline)
	}

	return nil
}

// loadBaseline loads baseline results from disk
func loadBaseline(scenario string) (map[string]BenchmarkResult, error) {
	filename := filepath.Join(baselineDir, fmt.Sprintf("%s.json", scenario))

	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil, nil // No baseline yet
	}
	if err != nil {
		return nil, err
	}

	var results []BenchmarkResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}

	// Convert to map for easy lookup
	baseline := make(map[string]BenchmarkResult)
	for _, result := range results {
		key := makeResultKey(result)
		baseline[key] = result
	}

	return baseline, nil
}

// saveBaseline saves baseline results to disk
func saveBaseline(scenario string, baseline map[string]BenchmarkResult) error {
	if err := os.MkdirAll(baselineDir, 0755); err != nil {
		return err
	}

	// Convert map back to slice
	results := make([]BenchmarkResult, 0, len(baseline))
	for _, result := range baseline {
		results = append(results, result)
	}

	// Sort for consistent output
	sort.Slice(results, func(i, j int) bool {
		if results[i].Tool != results[j].Tool {
			return results[i].Tool < results[j].Tool
		}
		return results[i].Strategy < results[j].Strategy
	})

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(baselineDir, fmt.Sprintf("%s.json", scenario))
	return os.WriteFile(filename, data, 0644)
}

// saveRegressionReport saves regression report to disk
func saveRegressionReport(resultsDir string, report *RegressionReport) error {
	reportDir := filepath.Join(resultsDir, "regression")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return err
	}

	filename := filepath.Join(reportDir, fmt.Sprintf("%s_%s.json",
		report.Scenario, time.Now().Format("20060102-150405")))

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// makeResultKey creates a unique key for a result
func makeResultKey(result BenchmarkResult) string {
	if result.Strategy != "" {
		return fmt.Sprintf("%s-%s", result.Tool, result.Strategy)
	}
	return result.Tool
}

// printRegressionReport prints regression report to stdout
func printRegressionReport(report *RegressionReport) {
	fmt.Printf("\n📊 Regression Analysis: %s\n", report.Scenario)
	fmt.Printf("=======================================\n\n")

	if len(report.Comparisons) == 0 {
		fmt.Printf("   ℹ️  No baseline found - this will be the first baseline\n")
		return
	}

	passed := 0
	failed := 0

	for _, comp := range report.Comparisons {
		toolName := comp.Tool
		if comp.Strategy != "" {
			toolName = fmt.Sprintf("%s (%s)", comp.Tool, comp.Strategy)
		}

		status := "✅"
		if !comp.Pass {
			status = "❌"
			failed++
		} else {
			passed++
		}

		fmt.Printf("%s %s\n", status, toolName)
		fmt.Printf("   Baseline: %.2fs\n", comp.BaselineTime)
		fmt.Printf("   Current:  %.2fs\n", comp.CurrentTime)

		if comp.ChangePercent > 0 {
			fmt.Printf("   Change:   +%.1f%% slower ⚠️\n", comp.ChangePercent)
		} else {
			fmt.Printf("   Change:   %.1f%% faster ⚡\n", math.Abs(comp.ChangePercent))
		}

		if !comp.Pass {
			fmt.Printf("   ❗ Regression detected (threshold: %.1f%%)\n", comp.Threshold)
		}
		fmt.Printf("\n")
	}

	fmt.Printf("Summary: %d passed, %d failed\n", passed, failed)

	if report.Pass {
		fmt.Printf("✅ No regressions detected\n")
	} else {
		fmt.Printf("❌ Performance regression detected!\n")
	}
}
