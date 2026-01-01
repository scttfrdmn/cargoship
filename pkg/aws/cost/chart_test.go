package cost

import (
	"strings"
	"testing"
)

func TestNewASCIIBarChart(t *testing.T) {
	chart := NewASCIIBarChart("Test Chart", 60)

	if chart.Title != "Test Chart" {
		t.Errorf("Expected title 'Test Chart', got %q", chart.Title)
	}

	if chart.Width != 60 {
		t.Errorf("Expected width 60, got %d", chart.Width)
	}

	if len(chart.Bars) != 0 {
		t.Errorf("Expected 0 bars, got %d", len(chart.Bars))
	}
}

func TestNewASCIIBarChart_DefaultWidth(t *testing.T) {
	chart := NewASCIIBarChart("Test", 0)

	if chart.Width != 60 {
		t.Errorf("Expected default width 60, got %d", chart.Width)
	}
}

func TestAddBar(t *testing.T) {
	chart := NewASCIIBarChart("Test", 60)

	chart.AddBar("Item 1", 100.0, "")
	chart.AddBar("Item 2", 50.0, "")

	if len(chart.Bars) != 2 {
		t.Fatalf("Expected 2 bars, got %d", len(chart.Bars))
	}

	if chart.Bars[0].Label != "Item 1" {
		t.Errorf("Expected label 'Item 1', got %q", chart.Bars[0].Label)
	}

	if chart.Bars[0].Value != 100.0 {
		t.Errorf("Expected value 100.0, got %f", chart.Bars[0].Value)
	}

	if chart.Bars[1].Value != 50.0 {
		t.Errorf("Expected value 50.0, got %f", chart.Bars[1].Value)
	}
}

func TestRender_EmptyChart(t *testing.T) {
	chart := NewASCIIBarChart("Empty", 60)

	output := chart.Render()

	if output != "" {
		t.Errorf("Expected empty output for chart with no bars, got %q", output)
	}
}

func TestRender_WithBars(t *testing.T) {
	chart := NewASCIIBarChart("Cost Comparison", 60)

	chart.AddBar("CargoShip", 10.0, "")
	chart.AddBar("Competitor", 50.0, "")

	output := chart.Render()

	// Verify title is present
	if !strings.Contains(output, "Cost Comparison") {
		t.Error("Chart should contain title")
	}

	// Verify labels are present
	if !strings.Contains(output, "CargoShip") {
		t.Error("Chart should contain 'CargoShip' label")
	}

	if !strings.Contains(output, "Competitor") {
		t.Error("Chart should contain 'Competitor' label")
	}

	// Verify values are present
	if !strings.Contains(output, "$10.00") {
		t.Error("Chart should contain CargoShip value $10.00")
	}

	if !strings.Contains(output, "$50.00") {
		t.Error("Chart should contain Competitor value $50.00")
	}

	// Verify bars are present (█ character)
	if !strings.Contains(output, "█") {
		t.Error("Chart should contain bar characters")
	}
}

func TestRender_ZeroMaxValue(t *testing.T) {
	chart := NewASCIIBarChart("Zero Values", 60)

	chart.AddBar("Item 1", 0.0, "")
	chart.AddBar("Item 2", 0.0, "")

	// Should not panic with zero max value
	output := chart.Render()

	if output == "" {
		t.Error("Should render chart even with zero values")
	}
}

func TestCostComparisonChart(t *testing.T) {
	output := CostComparisonChart(10.50, 50.75, "CargoShip", "s5cmd")

	// Verify it contains key elements
	if !strings.Contains(output, "CargoShip") {
		t.Error("Should contain CargoShip name")
	}

	if !strings.Contains(output, "s5cmd") {
		t.Error("Should contain competitor name")
	}

	if !strings.Contains(output, "$10.50") {
		t.Error("Should contain CargoShip cost")
	}

	if !strings.Contains(output, "$50.75") {
		t.Error("Should contain competitor cost")
	}

	// Verify savings calculation
	if !strings.Contains(output, "Savings") {
		t.Error("Should contain savings summary")
	}

	// Expected savings: $40.25 (79.3%)
	if !strings.Contains(output, "$40.25") {
		t.Error("Should contain correct savings amount")
	}
}

func TestSavingsBreakdownChart(t *testing.T) {
	advantages := &CargoShipCostAdvantage{
		CompressionSavings:   25.50,
		CompressionRatio:     3.0,
		ChunkingSavings:      2.00,
		RequestReduction:     9500,
		StorageTierSavings:   10.00,
		StorageTierUsed:      "GLACIER",
		DeduplicationSavings: 5.00,
		DeduplicationRatio:   2.0,
		TotalSavings:         42.50,
		SavingsPercentage:    85.0,
	}

	output := SavingsBreakdownChart(advantages)

	// Verify it contains all savings components
	if !strings.Contains(output, "Compression") {
		t.Error("Should contain compression savings")
	}

	if !strings.Contains(output, "Chunking") {
		t.Error("Should contain chunking savings")
	}

	if !strings.Contains(output, "Tier") {
		t.Error("Should contain tier savings")
	}

	if !strings.Contains(output, "Dedup") {
		t.Error("Should contain deduplication savings")
	}

	// Verify totals
	if !strings.Contains(output, "$42.50") {
		t.Error("Should contain total savings")
	}

	if !strings.Contains(output, "85.0%") {
		t.Error("Should contain savings percentage")
	}
}

func TestSavingsBreakdownChart_Nil(t *testing.T) {
	output := SavingsBreakdownChart(nil)

	if output != "" {
		t.Error("Should return empty string for nil advantages")
	}
}

func TestSavingsBreakdownChart_MinimalSavings(t *testing.T) {
	// Test with very small savings (should be filtered out)
	advantages := &CargoShipCostAdvantage{
		CompressionSavings: 0.005, // Below 0.01 threshold
		CompressionRatio:   1.1,
		ChunkingSavings:    2.50,
		RequestReduction:   100,
		TotalSavings:       2.505,
		SavingsPercentage:  5.0,
	}

	output := SavingsBreakdownChart(advantages)

	// Compression should not appear (< 0.01)
	if strings.Contains(output, "Compression") {
		t.Error("Should not show compression savings below threshold")
	}

	// Chunking should appear
	if !strings.Contains(output, "Chunking") {
		t.Error("Should show chunking savings above threshold")
	}
}

func TestMonthlyVsAnnualChart(t *testing.T) {
	output := MonthlyVsAnnualChart(2.50, 0.10, 30.10)

	// Verify labels
	if !strings.Contains(output, "One-time Upload") {
		t.Error("Should contain upload cost label")
	}

	if !strings.Contains(output, "Monthly Storage") {
		t.Error("Should contain monthly storage label")
	}

	if !strings.Contains(output, "Annual TCO") {
		t.Error("Should contain annual TCO label")
	}

	// Verify values
	if !strings.Contains(output, "$0.10") {
		t.Error("Should contain upload cost value")
	}

	if !strings.Contains(output, "$2.50") {
		t.Error("Should contain monthly storage value")
	}

	if !strings.Contains(output, "$30.10") {
		t.Error("Should contain annual TCO value")
	}

	// Verify breakdown summary
	if !strings.Contains(output, "After 12 months") {
		t.Error("Should contain monthly breakdown")
	}
}

func TestComparisonTable(t *testing.T) {
	cargoship := &BenchmarkCostComparison{
		TotalUploadCost:    0.10,
		MonthlyRunningCost: 1.50,
		AnnualTCO:          18.10,
	}

	competitor := &BenchmarkCostComparison{
		TotalUploadCost:    0.50,
		MonthlyRunningCost: 5.00,
		AnnualTCO:          60.50,
	}

	output := ComparisonTable(cargoship, competitor)

	// Verify table structure
	if !strings.Contains(output, "╔") || !strings.Contains(output, "╗") {
		t.Error("Should contain table borders")
	}

	// Verify headers
	if !strings.Contains(output, "Cost Category") {
		t.Error("Should contain 'Cost Category' header")
	}

	if !strings.Contains(output, "CargoShip") {
		t.Error("Should contain 'CargoShip' header")
	}

	if !strings.Contains(output, "Competitor") {
		t.Error("Should contain 'Competitor' header")
	}

	if !strings.Contains(output, "Savings") {
		t.Error("Should contain 'Savings' header")
	}

	// Verify row labels
	if !strings.Contains(output, "Upload Cost") {
		t.Error("Should contain 'Upload Cost' row")
	}

	if !strings.Contains(output, "Monthly Storage") {
		t.Error("Should contain 'Monthly Storage' row")
	}

	if !strings.Contains(output, "Annual TCO") {
		t.Error("Should contain 'Annual TCO' row")
	}

	// Verify values are present (specific amounts)
	if !strings.Contains(output, "18.10") {
		t.Error("Should contain CargoShip annual TCO")
	}

	if !strings.Contains(output, "60.50") {
		t.Error("Should contain competitor annual TCO")
	}

	// Verify savings calculation present
	if !strings.Contains(output, "Savings Percentage") {
		t.Error("Should contain savings percentage row")
	}
}

func TestComparisonTable_ZeroCompetitor(t *testing.T) {
	cargoship := &BenchmarkCostComparison{
		TotalUploadCost:    0.10,
		MonthlyRunningCost: 1.50,
		AnnualTCO:          18.10,
	}

	competitor := &BenchmarkCostComparison{
		TotalUploadCost:    0.0,
		MonthlyRunningCost: 0.0,
		AnnualTCO:          0.0,
	}

	// Should not panic with zero competitor costs
	output := ComparisonTable(cargoship, competitor)

	if output == "" {
		t.Error("Should render table even with zero competitor costs")
	}

	// Savings percentage should be 0 or handled gracefully
	if !strings.Contains(output, "Savings Percentage") {
		t.Error("Should contain savings percentage row")
	}
}

func TestComparisonTable_Integration(t *testing.T) {
	// Integration test with realistic data
	cargoship := &BenchmarkCostComparison{
		Scenario:           "100GB benchmark",
		Tool:               "cargoship",
		DataTransferCost:   0.00,
		PUTRequestCost:     0.00034,
		StorageCostMonthly: 0.092,
		TotalUploadCost:    0.00034,
		MonthlyRunningCost: 0.092,
		AnnualTCO:          1.10,
		Currency:           "USD",
		CargoShipAdvantages: &CargoShipCostAdvantage{
			CompressionSavings: 18.40,
			CompressionRatio:   3.0,
			ChunkingSavings:    0.005,
			RequestReduction:   9666,
			StorageTierSavings: 7.60,
			StorageTierUsed:    "GLACIER",
			TotalSavings:       26.00,
			SavingsPercentage:  96.0,
		},
	}

	competitor := &BenchmarkCostComparison{
		Scenario:           "100GB benchmark",
		Tool:               "s5cmd",
		DataTransferCost:   0.00,
		PUTRequestCost:     0.005,
		StorageCostMonthly: 2.30,
		TotalUploadCost:    0.005,
		MonthlyRunningCost: 2.30,
		AnnualTCO:          27.60,
		Currency:           "USD",
	}

	output := ComparisonTable(cargoship, competitor)

	// Verify realistic savings are calculated
	// Expected: $26.50 savings (96% reduction)
	if !strings.Contains(output, "1.10") {
		t.Error("Should show CargoShip annual TCO")
	}

	if !strings.Contains(output, "27.60") {
		t.Error("Should show competitor annual TCO")
	}

	// Table should be well-formatted
	lines := strings.Split(output, "\n")
	if len(lines) < 5 {
		t.Error("Table should have multiple lines")
	}
}
