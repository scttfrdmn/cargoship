// Package cost provides AWS cost calculation and pricing management for CargoShip
package cost

import (
	"fmt"
	"math"
	"strings"
)

// ASCIIBarChart generates a simple ASCII bar chart for cost comparison
type ASCIIBarChart struct {
	Title string
	Width int // Total width of the chart in characters
	Bars  []BarData
}

// BarData represents a single bar in the chart
type BarData struct {
	Label string
	Value float64
	Color string // ANSI color code (optional)
}

// NewASCIIBarChart creates a new ASCII bar chart
func NewASCIIBarChart(title string, width int) *ASCIIBarChart {
	if width <= 0 {
		width = 60
	}
	return &ASCIIBarChart{
		Title: title,
		Width: width,
		Bars:  make([]BarData, 0),
	}
}

// AddBar adds a bar to the chart
func (c *ASCIIBarChart) AddBar(label string, value float64, color string) {
	c.Bars = append(c.Bars, BarData{
		Label: label,
		Value: value,
		Color: color,
	})
}

// Render generates the ASCII bar chart string
func (c *ASCIIBarChart) Render() string {
	if len(c.Bars) == 0 {
		return ""
	}

	var output strings.Builder

	// Title
	if c.Title != "" {
		output.WriteString(fmt.Sprintf("\n%s\n", c.Title))
		output.WriteString(strings.Repeat("=", len(c.Title)) + "\n\n")
	}

	// Find max value for scaling
	maxValue := 0.0
	for _, bar := range c.Bars {
		if bar.Value > maxValue {
			maxValue = bar.Value
		}
	}

	// Prevent division by zero
	if maxValue == 0 {
		maxValue = 1.0
	}

	// Find longest label for alignment
	maxLabelLen := 0
	for _, bar := range c.Bars {
		if len(bar.Label) > maxLabelLen {
			maxLabelLen = len(bar.Label)
		}
	}

	// Reserve space for label, separator, and value
	barAreaWidth := c.Width - maxLabelLen - 15 // 15 chars for " | $XXX.XX"

	// Render each bar
	for _, bar := range c.Bars {
		// Calculate bar length
		barLen := int((bar.Value / maxValue) * float64(barAreaWidth))
		if barLen < 0 {
			barLen = 0
		}
		if barLen > barAreaWidth {
			barLen = barAreaWidth
		}

		// Format label (left-aligned, padded)
		label := fmt.Sprintf("%-*s", maxLabelLen, bar.Label)

		// Create bar string
		barStr := strings.Repeat("█", barLen)

		// Apply color if provided
		if bar.Color != "" {
			barStr = bar.Color + barStr + "\033[0m" // Reset color after
		}

		// Format value
		valueStr := fmt.Sprintf("$%.2f", bar.Value)

		// Render line
		output.WriteString(fmt.Sprintf("%s │ %-*s %s\n", label, barAreaWidth, barStr, valueStr))
	}

	output.WriteString("\n")

	return output.String()
}

// CostComparisonChart creates a cost comparison chart between CargoShip and competitors
func CostComparisonChart(cargoshipCost, competitorCost float64, cargoshipName, competitorName string) string {
	chart := NewASCIIBarChart("Annual TCO Comparison", 70)

	// Color codes: green for CargoShip, red for competitor
	greenColor := "\033[32m" // Green
	redColor := "\033[31m"   // Red

	chart.AddBar(cargoshipName, cargoshipCost, greenColor)
	chart.AddBar(competitorName, competitorCost, redColor)

	output := chart.Render()

	// Add savings summary
	savings := competitorCost - cargoshipCost
	savingsPct := 0.0
	if competitorCost > 0 {
		savingsPct = (savings / competitorCost) * 100
	}

	output += fmt.Sprintf("💰 Savings: $%.2f/year (%.1f%% reduction)\n\n", savings, savingsPct)

	return output
}

// SavingsBreakdownChart shows detailed savings breakdown
func SavingsBreakdownChart(advantages *CargoShipCostAdvantage) string {
	if advantages == nil {
		return ""
	}

	chart := NewASCIIBarChart("CargoShip Savings Breakdown", 70)

	// Color for all bars
	blueColor := "\033[34m" // Blue

	if advantages.CompressionSavings > 0.01 {
		label := fmt.Sprintf("Compression (%.1fx)", advantages.CompressionRatio)
		chart.AddBar(label, advantages.CompressionSavings, blueColor)
	}

	if advantages.ChunkingSavings > 0.01 {
		label := fmt.Sprintf("Chunking (%d fewer reqs)", advantages.RequestReduction)
		chart.AddBar(label, advantages.ChunkingSavings, blueColor)
	}

	if advantages.StorageTierSavings > 0.01 {
		label := fmt.Sprintf("Tier (%s)", advantages.StorageTierUsed)
		chart.AddBar(label, advantages.StorageTierSavings, blueColor)
	}

	if advantages.DeduplicationSavings > 0.01 {
		label := fmt.Sprintf("Dedup (%.1fx)", advantages.DeduplicationRatio)
		chart.AddBar(label, advantages.DeduplicationSavings, blueColor)
	}

	output := chart.Render()

	// Add total
	output += fmt.Sprintf("✨ Total Annual Savings: $%.2f\n", advantages.TotalSavings)
	output += fmt.Sprintf("📊 Savings Rate: %.1f%%\n\n", advantages.SavingsPercentage)

	return output
}

// MonthlyVsAnnualChart shows monthly vs annual cost projection
func MonthlyVsAnnualChart(monthlyStorageCost, uploadCost, annualTCO float64) string {
	chart := NewASCIIBarChart("Cost Projection", 70)

	chart.AddBar("One-time Upload", uploadCost, "\033[33m")         // Yellow
	chart.AddBar("Monthly Storage", monthlyStorageCost, "\033[36m") // Cyan
	chart.AddBar("Annual TCO", annualTCO, "\033[35m")               // Magenta

	output := chart.Render()

	// Add breakdown
	storagePercentage := 0.0
	if annualTCO > 0 {
		storagePercentage = (monthlyStorageCost * 12 / annualTCO) * 100
	}

	output += fmt.Sprintf("Storage represents %.1f%% of annual TCO\n", storagePercentage)
	output += fmt.Sprintf("After 12 months: $%.2f upload + $%.2f storage = $%.2f total\n\n",
		uploadCost, monthlyStorageCost*12, annualTCO)

	return output
}

// ComparisonTable generates a text table comparing costs
func ComparisonTable(cargoship, competitor *BenchmarkCostComparison) string {
	var output strings.Builder

	output.WriteString("\n")
	output.WriteString("╔════════════════════════════╤════════════════╤════════════════╤════════════════╗\n")
	output.WriteString("║ Cost Category              │   CargoShip    │   Competitor   │    Savings     ║\n")
	output.WriteString("╠════════════════════════════╪════════════════╪════════════════╪════════════════╣\n")

	// Upload cost row
	uploadSavings := competitor.TotalUploadCost - cargoship.TotalUploadCost
	uploadSavingsPct := 0.0
	if competitor.TotalUploadCost > 0 {
		uploadSavingsPct = (uploadSavings / competitor.TotalUploadCost) * 100
	}
	output.WriteString(fmt.Sprintf("║ Upload Cost                │   $%10.2f  │   $%10.2f  │  $%10.2f   ║\n",
		cargoship.TotalUploadCost, competitor.TotalUploadCost, uploadSavings))

	// Monthly storage cost row
	monthlySavings := competitor.MonthlyRunningCost - cargoship.MonthlyRunningCost
	monthlySavingsPct := 0.0
	if competitor.MonthlyRunningCost > 0 {
		monthlySavingsPct = (monthlySavings / competitor.MonthlyRunningCost) * 100
	}
	output.WriteString(fmt.Sprintf("║ Monthly Storage            │   $%10.2f  │   $%10.2f  │  $%10.2f   ║\n",
		cargoship.MonthlyRunningCost, competitor.MonthlyRunningCost, monthlySavings))

	// Annual TCO row
	annualSavings := competitor.AnnualTCO - cargoship.AnnualTCO
	annualSavingsPct := 0.0
	if competitor.AnnualTCO > 0 {
		annualSavingsPct = (annualSavings / competitor.AnnualTCO) * 100
	}
	output.WriteString("╠════════════════════════════╪════════════════╪════════════════╪════════════════╣\n")
	output.WriteString(fmt.Sprintf("║ Annual TCO                 │   $%10.2f  │   $%10.2f  │  $%10.2f   ║\n",
		cargoship.AnnualTCO, competitor.AnnualTCO, annualSavings))

	output.WriteString("╠════════════════════════════╧════════════════╧════════════════╧════════════════╣\n")
	output.WriteString(fmt.Sprintf("║ Savings Percentage:  Upload: %.1f%%  |  Monthly: %.1f%%  |  Annual: %.1f%%      ║\n",
		math.Max(0, uploadSavingsPct),
		math.Max(0, monthlySavingsPct),
		math.Max(0, annualSavingsPct)))
	output.WriteString("╚════════════════════════════════════════════════════════════════════════════════╝\n\n")

	return output.String()
}
