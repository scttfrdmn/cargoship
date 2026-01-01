// Package costs provides S3 bucket cost analysis for existing storage
package costs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// S3Analyzer analyzes existing S3 buckets and calculates cost savings from CargoShip chunking
type S3Analyzer struct {
	calculator *Calculator
	scanner    *s3.BucketScanner
	region     string
}

// NewS3Analyzer creates a new S3 analyzer
func NewS3Analyzer(calculator *Calculator, scanner *s3.BucketScanner, region string) *S3Analyzer {
	return &S3Analyzer{
		calculator: calculator,
		scanner:    scanner,
		region:     region,
	}
}

// S3AnalysisResult contains the analysis results for an existing S3 bucket
type S3AnalysisResult struct {
	// Bucket information
	BucketName string          `json:"bucket_name"`
	Prefix     string          `json:"prefix"`
	Region     string          `json:"region"`
	ScanTime   time.Time       `json:"scan_time"`
	IsSampled  bool            `json:"is_sampled"`

	// Current bucket statistics
	BucketStats *s3.BucketStats `json:"bucket_stats"`

	// Current cost breakdown
	CurrentCosts *CurrentCostBreakdown `json:"current_costs"`

	// Projected costs with CargoShip
	ProjectedCosts *ProjectedCostBreakdown `json:"projected_costs"`

	// Savings analysis
	Savings *SavingsAnalysis `json:"savings"`

	// Migration cost estimate
	MigrationCost *MigrationCostEstimate `json:"migration_cost"`

	// Recommendations
	Recommendations []string `json:"recommendations"`
}

// CurrentCostBreakdown shows costs for existing S3 storage
type CurrentCostBreakdown struct {
	// Monthly storage costs by tier
	StorageCost float64 `json:"storage_cost"`

	// INTELLIGENT_TIERING monitoring fees
	MonitoringFees float64 `json:"monitoring_fees"`

	// Estimated monthly request costs
	RequestCosts float64 `json:"request_costs"`

	// Total monthly cost
	TotalMonthlyCost float64 `json:"total_monthly_cost"`

	// Annual projected cost
	TotalAnnualCost float64 `json:"total_annual_cost"`

	// Breakdown by storage class
	ByStorageClass map[string]float64 `json:"by_storage_class"`
}

// ProjectedCostBreakdown shows costs after CargoShip re-gration
type ProjectedCostBreakdown struct {
	// Estimated chunk count
	EstimatedChunks int `json:"estimated_chunks"`

	// Monthly storage costs (same as before)
	StorageCost float64 `json:"storage_cost"`

	// INTELLIGENT_TIERING monitoring fees (reduced)
	MonitoringFees float64 `json:"monitoring_fees"`

	// Request costs (amortized, effectively zero)
	RequestCosts float64 `json:"request_costs"`

	// Total monthly cost
	TotalMonthlyCost float64 `json:"total_monthly_cost"`

	// Annual projected cost
	TotalAnnualCost float64 `json:"total_annual_cost"`
}

// SavingsAnalysis shows savings from CargoShip re-gration
type SavingsAnalysis struct {
	// Monthly savings
	MonthlySavings float64 `json:"monthly_savings"`

	// Annual savings
	AnnualSavings float64 `json:"annual_savings"`

	// Savings percentage
	SavingsPercentage float64 `json:"savings_percentage"`

	// Savings breakdown
	MinimumSizeSavings float64 `json:"minimum_size_savings"`
	MonitoringFeeSavings float64 `json:"monitoring_fee_savings"`
	RequestCostSavings float64 `json:"request_cost_savings"`

	// Payback period in days
	PaybackPeriodDays float64 `json:"payback_period_days"`
}

// MigrationCostEstimate estimates one-time cost to migrate to CargoShip format
type MigrationCostEstimate struct {
	// GET requests to read source objects
	GetRequestCost float64 `json:"get_request_cost"`

	// PUT requests to write chunks
	PutRequestCost float64 `json:"put_request_cost"`

	// Data transfer costs (if cross-region)
	TransferCost float64 `json:"transfer_cost"`

	// Total one-time migration cost
	TotalCost float64 `json:"total_cost"`
}

// Analyze performs cost analysis on an existing S3 bucket
func (a *S3Analyzer) Analyze(ctx context.Context) (*S3AnalysisResult, error) {
	// Scan the bucket
	stats, err := a.scanner.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to scan bucket: %w", err)
	}

	// Calculate current costs
	currentCosts, err := a.calculateCurrentCosts(ctx, stats)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate current costs: %w", err)
	}

	// Calculate projected costs with CargoShip chunking
	projectedCosts, err := a.calculateProjectedCosts(ctx, stats)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate projected costs: %w", err)
	}

	// Calculate savings
	savings := a.calculateSavings(currentCosts, projectedCosts)

	// Estimate migration cost
	migrationCost := a.estimateMigrationCost(ctx, stats, projectedCosts.EstimatedChunks)

	// Calculate payback period
	if savings.MonthlySavings > 0 {
		savings.PaybackPeriodDays = (migrationCost.TotalCost / savings.MonthlySavings) * 30
	}

	// Generate recommendations
	recommendations := a.generateRecommendations(stats, savings, migrationCost)

	result := &S3AnalysisResult{
		Region:          a.region,
		ScanTime:        time.Now(),
		IsSampled:       stats.IsSampled,
		BucketStats:     stats,
		CurrentCosts:    currentCosts,
		ProjectedCosts:  projectedCosts,
		Savings:         savings,
		MigrationCost:   migrationCost,
		Recommendations: recommendations,
	}

	return result, nil
}

// calculateCurrentCosts calculates current monthly costs for existing S3 storage
func (a *S3Analyzer) calculateCurrentCosts(ctx context.Context, stats *s3.BucketStats) (*CurrentCostBreakdown, error) {
	breakdown := &CurrentCostBreakdown{
		ByStorageClass: make(map[string]float64),
	}

	// Calculate storage costs by tier
	for storageClass, sizeBytes := range stats.StorageClassSizes {
		sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
		objectCount := stats.StorageClassCounts[storageClass]

		// Convert to CargoShip config storage class
		configClass := s3.ConvertToStorageClass(storageClass)

		// Calculate storage cost with minimum size penalties
		avgFileSizeKB := (sizeGB * 1024 * 1024) / float64(objectCount)
		chargedSizeGB := a.calculator.calculateChargedSize(int(objectCount), sizeGB, configClass, avgFileSizeKB)
		storageCost := a.calculator.calculateStorageCost(ctx, chargedSizeGB, configClass)

		breakdown.ByStorageClass[string(storageClass)] = storageCost
		breakdown.StorageCost += storageCost

		// Add INTELLIGENT_TIERING monitoring fees
		if storageClass == types.ObjectStorageClassIntelligentTiering {
			monitoringFee := (float64(objectCount) / 1000) * IntelligentTieringMonitoringFee
			breakdown.MonitoringFees += monitoringFee
		}
	}

	// Estimate monthly request costs (assume moderate access pattern)
	// Conservative estimate: 10% of objects accessed per month
	accessedObjects := int(float64(stats.ObjectCount) * 0.1)
	getRequestCostPerMonth := (float64(accessedObjects) / 1000) * 0.0004 // $0.0004 per 1K GET requests

	breakdown.RequestCosts = getRequestCostPerMonth

	// Calculate totals
	breakdown.TotalMonthlyCost = breakdown.StorageCost + breakdown.MonitoringFees + breakdown.RequestCosts
	breakdown.TotalAnnualCost = breakdown.TotalMonthlyCost * 12

	return breakdown, nil
}

// calculateProjectedCosts calculates projected costs after CargoShip re-gration
func (a *S3Analyzer) calculateProjectedCosts(ctx context.Context, stats *s3.BucketStats) (*ProjectedCostBreakdown, error) {
	breakdown := &ProjectedCostBreakdown{}

	// Estimate chunk count (assume 100 files per chunk or 10MB per chunk, whichever gives larger chunks)
	filesPerChunk := 100
	chunkCount := int(stats.ObjectCount) / filesPerChunk
	if chunkCount < 1 {
		chunkCount = 1
	}

	// Alternative: size-based chunking (10MB per chunk)
	totalSizeGB := float64(stats.TotalSize) / (1024 * 1024 * 1024)
	sizeBasedChunkCount := int(totalSizeGB * 1024 / 10) // 10MB per chunk
	if sizeBasedChunkCount > chunkCount {
		chunkCount = sizeBasedChunkCount
	}

	// Cap at reasonable maximum (1M chunks)
	if chunkCount > 1000000 {
		chunkCount = 1000000
	}

	breakdown.EstimatedChunks = chunkCount

	// Calculate storage costs by tier (no minimum size penalty for large chunks)
	for storageClass, sizeBytes := range stats.StorageClassSizes {
		sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

		// Convert to CargoShip config storage class
		configClass := s3.ConvertToStorageClass(storageClass)

		// Calculate storage cost (no minimum size penalty)
		storageCost := a.calculator.calculateStorageCost(ctx, sizeGB, configClass)
		breakdown.StorageCost += storageCost

		// Add INTELLIGENT_TIERING monitoring fees (for chunks, not files)
		if storageClass == types.ObjectStorageClassIntelligentTiering {
			// Calculate proportional chunk count for this tier
			tierProportion := float64(sizeBytes) / float64(stats.TotalSize)
			tierChunks := int(float64(chunkCount) * tierProportion)
			if tierChunks < 1 {
				tierChunks = 1
			}
			monitoringFee := (float64(tierChunks) / 1000) * IntelligentTieringMonitoringFee
			breakdown.MonitoringFees += monitoringFee
		}
	}

	// Request costs are effectively zero after chunking (amortized over time)
	breakdown.RequestCosts = 0.0

	// Calculate totals
	breakdown.TotalMonthlyCost = breakdown.StorageCost + breakdown.MonitoringFees + breakdown.RequestCosts
	breakdown.TotalAnnualCost = breakdown.TotalMonthlyCost * 12

	return breakdown, nil
}

// calculateSavings calculates savings from CargoShip re-gration
func (a *S3Analyzer) calculateSavings(current *CurrentCostBreakdown, projected *ProjectedCostBreakdown) *SavingsAnalysis {
	savings := &SavingsAnalysis{}

	// Calculate total savings
	savings.MonthlySavings = current.TotalMonthlyCost - projected.TotalMonthlyCost
	savings.AnnualSavings = savings.MonthlySavings * 12

	// Calculate percentage
	if current.TotalMonthlyCost > 0 {
		savings.SavingsPercentage = (savings.MonthlySavings / current.TotalMonthlyCost) * 100
	}

	// Breakdown savings
	savings.MinimumSizeSavings = (current.StorageCost - projected.StorageCost)
	savings.MonitoringFeeSavings = current.MonitoringFees - projected.MonitoringFees
	savings.RequestCostSavings = current.RequestCosts - projected.RequestCosts

	return savings
}

// estimateMigrationCost estimates one-time cost to migrate to CargoShip format
func (a *S3Analyzer) estimateMigrationCost(ctx context.Context, stats *s3.BucketStats, chunkCount int) *MigrationCostEstimate {
	estimate := &MigrationCostEstimate{}

	// GET requests to read source objects
	// $0.0004 per 1000 GET requests for STANDARD
	estimate.GetRequestCost = (float64(stats.ObjectCount) / 1000) * 0.0004

	// PUT requests to write chunks
	// $0.005 per 1000 PUT requests
	estimate.PutRequestCost = (float64(chunkCount) / 1000) * 0.005

	// Data transfer is FREE within the same region (no cross-region by default)
	estimate.TransferCost = 0.0

	// Total migration cost
	estimate.TotalCost = estimate.GetRequestCost + estimate.PutRequestCost + estimate.TransferCost

	return estimate
}

// generateRecommendations generates actionable recommendations based on analysis
func (a *S3Analyzer) generateRecommendations(stats *s3.BucketStats, savings *SavingsAnalysis, migration *MigrationCostEstimate) []string {
	var recommendations []string

	// Recommendation based on savings
	if savings.SavingsPercentage > 90 {
		recommendations = append(recommendations, fmt.Sprintf(
			"⚡ CRITICAL: Re-grate immediately! Savings: $%.2f/month (%.1f%% reduction)",
			savings.MonthlySavings, savings.SavingsPercentage))
	} else if savings.SavingsPercentage > 50 {
		recommendations = append(recommendations, fmt.Sprintf(
			"💰 HIGH: Significant savings available: $%.2f/month (%.1f%% reduction)",
			savings.MonthlySavings, savings.SavingsPercentage))
	} else if savings.SavingsPercentage > 20 {
		recommendations = append(recommendations, fmt.Sprintf(
			"✅ MEDIUM: Moderate savings available: $%.2f/month (%.1f%% reduction)",
			savings.MonthlySavings, savings.SavingsPercentage))
	}

	// Payback period recommendation
	if savings.PaybackPeriodDays < 1 {
		recommendations = append(recommendations, fmt.Sprintf(
			"⏱️  IMMEDIATE ROI: Payback in %.1f hours (migration cost: $%.2f)",
			savings.PaybackPeriodDays*24, migration.TotalCost))
	} else if savings.PaybackPeriodDays < 30 {
		recommendations = append(recommendations, fmt.Sprintf(
			"⏱️  FAST ROI: Payback in %.0f days (migration cost: $%.2f)",
			savings.PaybackPeriodDays, migration.TotalCost))
	}

	// Primary savings source
	if savings.MonitoringFeeSavings > savings.MonthlySavings*0.8 {
		recommendations = append(recommendations,
			"📊 PRIMARY SAVINGS: INTELLIGENT_TIERING monitoring fee elimination (99.9% reduction)")
	}

	if savings.MinimumSizeSavings > savings.MonthlySavings*0.5 {
		recommendations = append(recommendations,
			"📦 PRIMARY SAVINGS: Minimum object size penalty elimination (small files costing 2-4x more)")
	}

	// File distribution recommendations
	avgFileSize := stats.AverageSize / (1024 * 1024) // MB
	if avgFileSize < 1.0 {
		recommendations = append(recommendations,
			fmt.Sprintf("🔍 IDEAL CANDIDATE: Small average file size (%.1f KB) benefits most from chunking", avgFileSize*1024))
	}

	// Object count recommendations
	if stats.ObjectCount > 1000000 {
		recommendations = append(recommendations,
			fmt.Sprintf("🎯 PERFECT FIT: Large object count (%d objects) = massive monitoring fee savings", stats.ObjectCount))
	}

	// Migration strategy
	if stats.TotalSize > 1024*1024*1024*1024 { // > 1 TB
		recommendations = append(recommendations,
			"⚠️  MIGRATION: Large dataset - consider incremental migration or parallel re-gration tool")
	} else {
		recommendations = append(recommendations,
			"✅ MIGRATION: Dataset size supports quick migration (< 1 day expected)")
	}

	// Zero-downtime migration
	recommendations = append(recommendations,
		"🔒 ZERO DOWNTIME: Create new bucket, re-grate in parallel, cutover when ready")

	return recommendations
}
