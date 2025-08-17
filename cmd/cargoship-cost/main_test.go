package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// Helper function to create temporary files (compatibility with older Go versions)
func createTempFile(dir, pattern, content string) (string, error) {
	tmpFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := tmpFile.Close(); closeErr != nil {
			_ = closeErr // Ignore close error
		}
	}()

	if content != "" {
		if _, err := tmpFile.WriteString(content); err != nil {
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{
			name:     "bytes",
			input:    "1024",
			expected: 1024,
		},
		{
			name:     "bytes with B suffix",
			input:    "1024B",
			expected: 1024,
		},
		{
			name:     "kilobytes",
			input:    "1KB",
			expected: 1024,
		},
		{
			name:     "megabytes",
			input:    "1MB",
			expected: 1024 * 1024,
		},
		{
			name:     "gigabytes",
			input:    "1GB",
			expected: 1024 * 1024 * 1024,
		},
		{
			name:     "terabytes",
			input:    "1TB",
			expected: 1024 * 1024 * 1024 * 1024,
		},
		{
			name:     "fractional gigabytes",
			input:    "1.5GB",
			expected: int64(1.5 * 1024 * 1024 * 1024),
		},
		{
			name:     "lowercase",
			input:    "1gb",
			expected: 1024 * 1024 * 1024,
		},
		{
			name:     "with spaces",
			input:    " 2GB ",
			expected: 2 * 1024 * 1024 * 1024,
		},
		{
			name:    "invalid number",
			input:   "invalidGB",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSize(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseStorageClass(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected cargoconfig.StorageClass
		wantErr  bool
	}{
		{
			name:     "standard",
			input:    "STANDARD",
			expected: cargoconfig.StorageClassStandard,
		},
		{
			name:     "standard_ia",
			input:    "STANDARD_IA",
			expected: cargoconfig.StorageClassStandardIA,
		},
		{
			name:     "onezone_ia",
			input:    "ONEZONE_IA",
			expected: cargoconfig.StorageClassOneZoneIA,
		},
		{
			name:     "intelligent_tiering",
			input:    "INTELLIGENT_TIERING",
			expected: cargoconfig.StorageClassIntelligentTiering,
		},
		{
			name:     "glacier",
			input:    "GLACIER",
			expected: cargoconfig.StorageClassGlacier,
		},
		{
			name:     "deep_archive",
			input:    "DEEP_ARCHIVE",
			expected: cargoconfig.StorageClassDeepArchive,
		},
		{
			name:     "lowercase",
			input:    "glacier",
			expected: cargoconfig.StorageClassGlacier,
		},
		{
			name:    "invalid",
			input:   "INVALID_CLASS",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseStorageClass(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create a temporary config file
	configContent := `
profile: "test-profile"
region: "us-west-2"
cost_control:
  max_monthly_budget: 1000.0
  alert_threshold: 0.8
`

	tmpFile, err := createTempFile("", "cargoship-config-*.yaml", configContent)
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	// Test loading the config
	config, err := loadConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "test-profile", config.Profile)
	assert.Equal(t, "us-west-2", config.Region)
	assert.Equal(t, float64(1000), config.CostControl.MaxMonthlyBudget)
	assert.Equal(t, float64(0.8), config.CostControl.AlertThreshold)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	_, err := loadConfig("nonexistent-file.yaml")
	assert.Error(t, err)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create a file with invalid YAML
	tmpFile, err := createTempFile("", "invalid-config-*.yaml", "invalid: yaml: content: [")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	_, err = loadConfig(tmpFile)
	assert.Error(t, err)
}

func TestOutputJSON(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	testData := map[string]interface{}{
		"test":  "value",
		"count": 42,
	}

	err := outputJSON(testData)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	// Verify JSON output
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["test"])
	assert.Equal(t, float64(42), result["count"])
}

func TestOutputEstimateTable(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	estimate := &cost.CostEstimate{
		Currency:         "USD",
		StorageCost:      10.50,
		RequestCost:      0.25,
		DataTransferCost: 1.00,
		TotalCost:        11.75,
		Discounts: cost.DiscountBreakdown{
			OriginalCost:  11.75,
			TotalDiscount: 1.75,
		},
	}

	err := outputEstimateTable(estimate, 100.0, cargoconfig.StorageClassStandard)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Cost Estimate")
	assert.Contains(t, output, "100.00 GB")
	assert.Contains(t, output, "STANDARD")
	assert.Contains(t, output, "$10.5000")
	assert.Contains(t, output, "$11.7500")
}

func TestOutputBudgetTable(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	status := map[string]interface{}{
		"max_budget":       1000.0,
		"current_spend":    750.0,
		"budget_remaining": 250.0,
		"budget_used":      0.75,
		"alert_threshold":  0.8,
		"over_budget":      false,
		"alert_triggered":  false,
	}

	err := outputBudgetTable(status)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Budget Status")
	assert.Contains(t, output, "$1000.00")
	assert.Contains(t, output, "$750.00")
	assert.Contains(t, output, "75.0%")
	assert.Contains(t, output, "✅ Within budget")
}

func TestOutputBudgetTable_OverBudget(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	status := map[string]interface{}{
		"max_budget":       1000.0,
		"current_spend":    1200.0,
		"budget_remaining": -200.0,
		"budget_used":      1.2,
		"alert_threshold":  0.8,
		"over_budget":      true,
		"alert_triggered":  true,
	}

	err := outputBudgetTable(status)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "⚠️  OVER BUDGET!")
}

func TestOutputReportTable(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	summary := &cost.CostSummary{
		Period:       "2023-12",
		TotalCost:    1500.00,
		TotalSavings: 300.00,
		Currency:     "USD",
		ByService: map[string]float64{
			"S3":         800.00,
			"EC2":        500.00,
			"CloudWatch": 200.00,
		},
		ByRegion: map[string]float64{
			"us-east-1": 900.00,
			"us-west-2": 600.00,
		},
		Trends: cost.CostTrends{
			DailyAverage:      50.00,
			WeeklyAverage:     350.00,
			MonthlyProjection: 1600.00,
			CostPerGB:         0.023,
		},
		Recommendations: []cost.CostRecommendation{
			{
				Priority:        "HIGH",
				Description:     "Consider using Intelligent Tiering",
				PotentialSaving: 120.00,
			},
		},
	}

	err := outputReportTable(summary)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Cost Report - 2023-12")
	assert.Contains(t, output, "$1500.00")
	assert.Contains(t, output, "$300.00")
	assert.Contains(t, output, "S3")
	assert.Contains(t, output, "us-east-1")
	assert.Contains(t, output, "Intelligent Tiering")
	assert.Contains(t, output, "[HIGH]")
}

func TestOutputPricingTable(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	pricing := map[string]interface{}{
		"region":       "us-east-1",
		"currency":     "USD",
		"source":       "AWS Pricing API",
		"last_updated": "2023-12-15T10:30:00Z",
		"storage_per_gb_month": map[string]float64{
			"STANDARD":    0.023,
			"STANDARD_IA": 0.0125,
			"GLACIER":     0.004,
		},
		"requests": map[string]float64{
			"PUT": 0.0005,
			"GET": 0.0004,
		},
	}

	err := outputPricingTable(pricing)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Current AWS Pricing - us-east-1")
	assert.Contains(t, output, "USD")
	assert.Contains(t, output, "STANDARD")
	assert.Contains(t, output, "$0.023000")
	assert.Contains(t, output, "PUT")
}

// Integration test for main function flow (without actually calling main)
func TestCommandLineArguments(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no command",
			args:        []string{"cargoship-cost"},
			expectError: true,
		},
		{
			name:        "estimate without size",
			args:        []string{"cargoship-cost", "-command", "estimate"},
			expectError: true,
		},
		{
			name:        "unknown command",
			args:        []string{"cargoship-cost", "-command", "unknown"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the individual functions rather than main() to avoid exit()
			if strings.Contains(strings.Join(tt.args, " "), "estimate") && !strings.Contains(strings.Join(tt.args, " "), "size") {
				// Test handleEstimate without size
				size = new(string) // Reset size flag
				*size = ""

				err := handleEstimate(context.Background(), nil, nil)
				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "size is required")
				}
			}
		})
	}
}

// Test helper functions edge cases
func TestParseSizeEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{
			name:     "zero",
			input:    "0GB",
			expected: 0,
		},
		{
			name:     "very large",
			input:    "999TB",
			expected: 999 * 1024 * 1024 * 1024 * 1024,
		},
		{
			name:     "decimal precision",
			input:    "0.001GB",
			expected: 1073741, // int64(0.001 * 1024 * 1024 * 1024)
		},
		{
			name:     "negative",
			input:    "-1GB",
			expected: -1 * 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSize(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Test various output formats
func TestOutputFormats(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Test JSON output with complex nested data
	complexData := map[string]interface{}{
		"nested": map[string]interface{}{
			"array": []int{1, 2, 3},
			"bool":  true,
			"null":  nil,
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSON(complexData)
	require.NoError(t, err)

	// Restore stdout and read output
	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer: %v", err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	// Verify the output is valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	nested := result["nested"].(map[string]interface{})
	assert.Equal(t, []interface{}{1.0, 2.0, 3.0}, nested["array"])
	assert.Equal(t, true, nested["bool"])
	assert.Nil(t, nested["null"])
}

// Benchmark tests for performance-sensitive operations
func BenchmarkParseSize(b *testing.B) {
	testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = parseSize("1.5GB")
		}
	})
}

func BenchmarkParseStorageClass(b *testing.B) {
	testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = parseStorageClass("STANDARD")
		}
	})
}
