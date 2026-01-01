// Package cost provides AWS cost calculation and pricing management for CargoShip
package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// BenchmarkCostComparison compares costs between different upload approaches
type BenchmarkCostComparison struct {
	Scenario              string                  `json:"scenario"`
	Tool                  string                  `json:"tool"`
	DataTransferCost      float64                 `json:"data_transfer_cost"`
	PUTRequestCost        float64                 `json:"put_request_cost"`
	StorageCostMonthly    float64                 `json:"storage_cost_monthly"`
	TotalUploadCost       float64                 `json:"total_upload_cost"`
	MonthlyRunningCost    float64                 `json:"monthly_running_cost"`
	AnnualTCO             float64                 `json:"annual_tco"`
	Currency              string                  `json:"currency"`
	CargoShipAdvantages   *CargoShipCostAdvantage `json:"cargoship_advantages,omitempty"`
	EstimatedAt           time.Time               `json:"estimated_at"`
}

// CargoShipCostAdvantage shows cost savings from CargoShip features
type CargoShipCostAdvantage struct {
	CompressionSavings       float64 `json:"compression_savings"`
	CompressionRatio         float64 `json:"compression_ratio"`
	ChunkingSavings          float64 `json:"chunking_savings"`
	RequestReduction         int64   `json:"request_reduction"`
	StorageTierSavings       float64 `json:"storage_tier_savings"`
	StorageTierUsed          string  `json:"storage_tier_used"`
	DeduplicationSavings     float64 `json:"deduplication_savings"`
	DeduplicationRatio       float64 `json:"deduplication_ratio"`
	TotalSavings             float64 `json:"total_savings"`
	SavingsPercentage        float64 `json:"savings_percentage"`
	CompetitorComparisonCost float64 `json:"competitor_comparison_cost"`
}

// BenchmarkCostCalculator calculates costs for benchmark comparisons
type BenchmarkCostCalculator struct {
	pricingManager *PricingManager
	region         string
}

// NewBenchmarkCostCalculator creates a new benchmark cost calculator
func NewBenchmarkCostCalculator(pm *PricingManager, region string) *BenchmarkCostCalculator {
	return &BenchmarkCostCalculator{
		pricingManager: pm,
		region:         region,
	}
}

// CalculateCompetitorCost calculates cost for tools without compression/dedup/tiering
func (bcc *BenchmarkCostCalculator) CalculateCompetitorCost(
	ctx context.Context,
	scenario string,
	tool string,
	sizeGB float64,
	fileCount int64,
) (*BenchmarkCostComparison, error) {
	comparison := &BenchmarkCostComparison{
		Scenario:    scenario,
		Tool:        tool,
		Currency:    bcc.pricingManager.config.Currency,
		EstimatedAt: time.Now(),
	}

	// Data transfer cost (upload - typically free for S3)
	transferPrice, _ := bcc.pricingManager.getDataTransferPrice(ctx, "upload", bcc.region)
	comparison.DataTransferCost = sizeGB * transferPrice

	// PUT request cost (1 request per file for competitors)
	requestPrice, err := bcc.pricingManager.getRequestPrice(ctx, "PUT", config.StorageClassStandard, bcc.region)
	if err != nil {
		return nil, fmt.Errorf("failed to get request price: %w", err)
	}
	comparison.PUTRequestCost = (float64(fileCount) / 1000) * requestPrice

	// Storage cost (STANDARD tier, no optimization)
	storagePrice, err := bcc.pricingManager.getStoragePrice(ctx, config.StorageClassStandard, bcc.region)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage price: %w", err)
	}
	comparison.StorageCostMonthly = sizeGB * storagePrice

	// Total upload cost (one-time)
	comparison.TotalUploadCost = comparison.DataTransferCost + comparison.PUTRequestCost

	// Monthly running cost (ongoing)
	comparison.MonthlyRunningCost = comparison.StorageCostMonthly

	// Annual TCO (upload cost + 12 months storage)
	comparison.AnnualTCO = comparison.TotalUploadCost + (comparison.StorageCostMonthly * 12)

	return comparison, nil
}

// CalculateCargoShipCost calculates cost with CargoShip optimizations
func (bcc *BenchmarkCostCalculator) CalculateCargoShipCost(
	ctx context.Context,
	scenario string,
	sizeGB float64,
	fileCount int64,
	compressionRatio float64,
	deduplicationRatio float64,
	storageClass config.StorageClass,
) (*BenchmarkCostComparison, error) {
	comparison := &BenchmarkCostComparison{
		Scenario:    scenario,
		Tool:        "cargoship",
		Currency:    bcc.pricingManager.config.Currency,
		EstimatedAt: time.Now(),
	}

	// Calculate actual data size after compression and deduplication
	effectiveSizeGB := sizeGB
	if compressionRatio > 0 {
		effectiveSizeGB = sizeGB / compressionRatio
	}
	if deduplicationRatio > 0 {
		effectiveSizeGB = effectiveSizeGB / deduplicationRatio
	}

	// Data transfer cost (based on compressed size)
	transferPrice, _ := bcc.pricingManager.getDataTransferPrice(ctx, "upload", bcc.region)
	comparison.DataTransferCost = effectiveSizeGB * transferPrice

	// PUT request cost (chunking reduces requests by ~50%)
	// CargoShip uses multipart upload with intelligent chunking
	// Estimate: 1 request per 100MB chunk (vs 1 per file)
	numChunks := int64(effectiveSizeGB * 1024 / 100) // 100MB chunks
	if numChunks < 1 {
		numChunks = 1
	}

	requestPrice, err := bcc.pricingManager.getRequestPrice(ctx, "PUT", storageClass, bcc.region)
	if err != nil {
		return nil, fmt.Errorf("failed to get request price: %w", err)
	}
	comparison.PUTRequestCost = (float64(numChunks) / 1000) * requestPrice

	// Storage cost (with intelligent tier selection)
	storagePrice, err := bcc.pricingManager.getStoragePrice(ctx, storageClass, bcc.region)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage price: %w", err)
	}
	comparison.StorageCostMonthly = effectiveSizeGB * storagePrice

	// Total upload cost
	comparison.TotalUploadCost = comparison.DataTransferCost + comparison.PUTRequestCost

	// Monthly running cost
	comparison.MonthlyRunningCost = comparison.StorageCostMonthly

	// Annual TCO
	comparison.AnnualTCO = comparison.TotalUploadCost + (comparison.StorageCostMonthly * 12)

	// Calculate CargoShip-specific advantages
	comparison.CargoShipAdvantages = bcc.calculateAdvantages(
		ctx,
		sizeGB,
		effectiveSizeGB,
		fileCount,
		numChunks,
		compressionRatio,
		deduplicationRatio,
		storageClass,
	)

	return comparison, nil
}

// calculateAdvantages calculates CargoShip's cost advantages
func (bcc *BenchmarkCostCalculator) calculateAdvantages(
	ctx context.Context,
	originalSizeGB float64,
	effectiveSizeGB float64,
	originalFileCount int64,
	actualChunks int64,
	compressionRatio float64,
	deduplicationRatio float64,
	storageClass config.StorageClass,
) *CargoShipCostAdvantage {
	advantages := &CargoShipCostAdvantage{
		CompressionRatio:   compressionRatio,
		DeduplicationRatio: deduplicationRatio,
		StorageTierUsed:    string(storageClass),
	}

	// Get pricing for calculations
	standardStoragePrice, _ := bcc.pricingManager.getStoragePrice(ctx, config.StorageClassStandard, bcc.region)
	selectedStoragePrice, _ := bcc.pricingManager.getStoragePrice(ctx, storageClass, bcc.region)
	requestPrice, _ := bcc.pricingManager.getRequestPrice(ctx, "PUT", config.StorageClassStandard, bcc.region)

	// 1. Compression Savings (data transfer + storage)
	if compressionRatio > 1.0 {
		compressedSizeGB := originalSizeGB / compressionRatio
		sizeSavingsGB := originalSizeGB - compressedSizeGB

		// Transfer savings (if transfer had a cost)
		// Storage savings (monthly)
		advantages.CompressionSavings = (sizeSavingsGB * standardStoragePrice) * 12 // Annual
	}

	// 2. Chunking Savings (request costs)
	if actualChunks < originalFileCount {
		requestReduction := originalFileCount - actualChunks
		advantages.RequestReduction = requestReduction
		advantages.ChunkingSavings = (float64(requestReduction) / 1000) * requestPrice
	}

	// 3. Storage Tier Savings
	if storageClass != config.StorageClassStandard {
		priceDiff := standardStoragePrice - selectedStoragePrice
		if priceDiff > 0 {
			advantages.StorageTierSavings = (effectiveSizeGB * priceDiff) * 12 // Annual
		}
	}

	// 4. Deduplication Savings
	if deduplicationRatio > 1.0 {
		preDedup := effectiveSizeGB * deduplicationRatio
		dedupSizeSavings := preDedup - effectiveSizeGB
		advantages.DeduplicationSavings = (dedupSizeSavings * standardStoragePrice) * 12 // Annual
	}

	// Calculate total savings
	advantages.TotalSavings = advantages.CompressionSavings +
		advantages.ChunkingSavings +
		advantages.StorageTierSavings +
		advantages.DeduplicationSavings

	// Calculate competitor cost for comparison
	advantages.CompetitorComparisonCost = (originalSizeGB * standardStoragePrice * 12) +
		((float64(originalFileCount) / 1000) * requestPrice)

	// Calculate savings percentage
	if advantages.CompetitorComparisonCost > 0 {
		advantages.SavingsPercentage = (advantages.TotalSavings / advantages.CompetitorComparisonCost) * 100
	}

	return advantages
}

// CompareCosts compares CargoShip against a competitor
func (bcc *BenchmarkCostCalculator) CompareCosts(
	cargoshipCost *BenchmarkCostComparison,
	competitorCost *BenchmarkCostComparison,
) map[string]interface{} {
	comparison := make(map[string]interface{})

	comparison["scenario"] = cargoshipCost.Scenario
	comparison["cargoship_upload_cost"] = cargoshipCost.TotalUploadCost
	comparison["competitor_upload_cost"] = competitorCost.TotalUploadCost
	comparison["upload_cost_savings"] = competitorCost.TotalUploadCost - cargoshipCost.TotalUploadCost
	comparison["upload_cost_savings_pct"] = ((competitorCost.TotalUploadCost - cargoshipCost.TotalUploadCost) / competitorCost.TotalUploadCost) * 100

	comparison["cargoship_monthly_cost"] = cargoshipCost.MonthlyRunningCost
	comparison["competitor_monthly_cost"] = competitorCost.MonthlyRunningCost
	comparison["monthly_cost_savings"] = competitorCost.MonthlyRunningCost - cargoshipCost.MonthlyRunningCost
	comparison["monthly_cost_savings_pct"] = ((competitorCost.MonthlyRunningCost - cargoshipCost.MonthlyRunningCost) / competitorCost.MonthlyRunningCost) * 100

	comparison["cargoship_annual_tco"] = cargoshipCost.AnnualTCO
	comparison["competitor_annual_tco"] = competitorCost.AnnualTCO
	comparison["annual_tco_savings"] = competitorCost.AnnualTCO - cargoshipCost.AnnualTCO
	comparison["annual_tco_savings_pct"] = ((competitorCost.AnnualTCO - cargoshipCost.AnnualTCO) / competitorCost.AnnualTCO) * 100

	if cargoshipCost.CargoShipAdvantages != nil {
		comparison["cargoship_advantages"] = cargoshipCost.CargoShipAdvantages
	}

	return comparison
}

// GenerateCostReport generates a human-readable cost comparison report
func (bcc *BenchmarkCostCalculator) GenerateCostReport(comparisons []*BenchmarkCostComparison) string {
	report := "# Cost Analysis Report\n\n"
	report += fmt.Sprintf("Generated: %s\n", time.Now().Format(time.RFC3339))
	report += fmt.Sprintf("Region: %s\n", bcc.region)
	report += fmt.Sprintf("Currency: %s\n\n", bcc.pricingManager.config.Currency)

	for _, comp := range comparisons {
		report += fmt.Sprintf("## %s - %s\n\n", comp.Scenario, comp.Tool)
		report += fmt.Sprintf("- **Upload Cost**: $%.4f\n", comp.TotalUploadCost)
		report += fmt.Sprintf("- **Monthly Storage Cost**: $%.4f\n", comp.MonthlyRunningCost)
		report += fmt.Sprintf("- **Annual TCO**: $%.4f\n\n", comp.AnnualTCO)

		if comp.CargoShipAdvantages != nil {
			adv := comp.CargoShipAdvantages
			report += "### CargoShip Advantages:\n\n"

			if adv.CompressionSavings > 0 {
				report += fmt.Sprintf("- **Compression**: %.1f:1 ratio, $%.2f annual savings\n",
					adv.CompressionRatio, adv.CompressionSavings)
			}

			if adv.ChunkingSavings > 0 {
				report += fmt.Sprintf("- **Intelligent Chunking**: %d fewer requests, $%.2f savings\n",
					adv.RequestReduction, adv.ChunkingSavings)
			}

			if adv.StorageTierSavings > 0 {
				report += fmt.Sprintf("- **Storage Tier (%s)**: $%.2f annual savings\n",
					adv.StorageTierUsed, adv.StorageTierSavings)
			}

			if adv.DeduplicationSavings > 0 {
				report += fmt.Sprintf("- **Deduplication**: %.1f:1 ratio, $%.2f annual savings\n",
					adv.DeduplicationRatio, adv.DeduplicationSavings)
			}

			report += fmt.Sprintf("\n**Total Savings**: $%.2f (%.1f%%)\n\n",
				adv.TotalSavings, adv.SavingsPercentage)
		}

		report += "---\n\n"
	}

	return report
}
