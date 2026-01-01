// Package costs provides cost estimation and optimization for CargoShip AWS operations
package costs

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricing"
	"github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// Calculator provides cost estimation for S3 operations
type Calculator struct {
	region         string
	pricingService *pricing.Service
}

// NewCalculator creates a new cost calculator for the specified region
func NewCalculator(region string) *Calculator {
	return &Calculator{
		region: region,
	}
}

// NewCalculatorWithPricing creates a calculator with real-time pricing
func NewCalculatorWithPricing(region string, pricingService *pricing.Service) *Calculator {
	return &Calculator{
		region:         region,
		pricingService: pricingService,
	}
}

// CostEstimate represents a comprehensive cost breakdown
type CostEstimate struct {
	// Storage costs (monthly)
	StorageCosts CostBreakdown `json:"storage_costs"`

	// Data transfer costs (one-time)
	TransferCosts CostBreakdown `json:"transfer_costs"`

	// Request costs (one-time)
	RequestCosts CostBreakdown `json:"request_costs"`

	// Total costs
	TotalUploadCost  float64 `json:"total_upload_cost"`  // One-time upload cost
	TotalMonthlyCost float64 `json:"total_monthly_cost"` // Monthly storage cost
	TotalAnnualCost  float64 `json:"total_annual_cost"`  // Annual cost projection

	// Optimization recommendations
	Recommendations []Recommendation `json:"recommendations"`

	// Metadata
	Region       string    `json:"region"`
	CalculatedAt time.Time `json:"calculated_at"`
	TotalSizeGB  float64   `json:"total_size_gb"`
	ArchiveCount int       `json:"archive_count"`
}

// CostBreakdown shows costs by storage class
type CostBreakdown struct {
	Standard           float64 `json:"standard"`
	StandardIA         float64 `json:"standard_ia"`
	OneZoneIA          float64 `json:"onezone_ia"`
	IntelligentTiering float64 `json:"intelligent_tiering"`
	Glacier            float64 `json:"glacier"`
	DeepArchive        float64 `json:"deep_archive"`
	Total              float64 `json:"total"`
}

// Recommendation represents a cost optimization suggestion
type Recommendation struct {
	Type             string  `json:"type"`              // "storage_class", "lifecycle", "compression"
	Description      string  `json:"description"`       // Human-readable description
	EstimatedSavings float64 `json:"estimated_savings"` // Monthly savings in USD
	Confidence       float64 `json:"confidence"`        // Confidence level (0.0-1.0)
	Impact           string  `json:"impact"`            // "low", "medium", "high"
}

// EstimateArchives calculates costs for a set of archives
func (c *Calculator) EstimateArchives(ctx context.Context, archives []s3.Archive) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Region:       c.region,
		CalculatedAt: time.Now(),
		ArchiveCount: len(archives),
	}

	var totalSizeBytes int64
	for _, archive := range archives {
		totalSizeBytes += archive.Size
	}
	estimate.TotalSizeGB = float64(totalSizeBytes) / (1024 * 1024 * 1024)

	// Calculate costs for each storage class
	for _, storageClass := range []config.StorageClass{
		config.StorageClassStandard,
		config.StorageClassStandardIA,
		config.StorageClassOneZoneIA,
		config.StorageClassIntelligentTiering,
		config.StorageClassGlacier,
		config.StorageClassDeepArchive,
	} {
		storageCost := c.calculateStorageCost(ctx, estimate.TotalSizeGB, storageClass)
		transferCost := c.calculateTransferCost(ctx, estimate.TotalSizeGB)
		requestCost := c.calculateRequestCost(ctx, len(archives), storageClass)

		switch storageClass {
		case config.StorageClassStandard:
			estimate.StorageCosts.Standard = storageCost
			estimate.TransferCosts.Standard = transferCost
			estimate.RequestCosts.Standard = requestCost
		case config.StorageClassStandardIA:
			estimate.StorageCosts.StandardIA = storageCost
			estimate.TransferCosts.StandardIA = transferCost
			estimate.RequestCosts.StandardIA = requestCost
		case config.StorageClassOneZoneIA:
			estimate.StorageCosts.OneZoneIA = storageCost
			estimate.TransferCosts.OneZoneIA = transferCost
			estimate.RequestCosts.OneZoneIA = requestCost
		case config.StorageClassIntelligentTiering:
			estimate.StorageCosts.IntelligentTiering = storageCost
			estimate.TransferCosts.IntelligentTiering = transferCost
			estimate.RequestCosts.IntelligentTiering = requestCost
		case config.StorageClassGlacier:
			estimate.StorageCosts.Glacier = storageCost
			estimate.TransferCosts.Glacier = transferCost
			estimate.RequestCosts.Glacier = requestCost
		case config.StorageClassDeepArchive:
			estimate.StorageCosts.DeepArchive = storageCost
			estimate.TransferCosts.DeepArchive = transferCost
			estimate.RequestCosts.DeepArchive = requestCost
		}
	}

	// Calculate totals
	estimate.StorageCosts.Total = estimate.StorageCosts.Standard + estimate.StorageCosts.StandardIA +
		estimate.StorageCosts.OneZoneIA + estimate.StorageCosts.IntelligentTiering +
		estimate.StorageCosts.Glacier + estimate.StorageCosts.DeepArchive

	estimate.TransferCosts.Total = estimate.TransferCosts.Standard + estimate.TransferCosts.StandardIA +
		estimate.TransferCosts.OneZoneIA + estimate.TransferCosts.IntelligentTiering +
		estimate.TransferCosts.Glacier + estimate.TransferCosts.DeepArchive

	estimate.RequestCosts.Total = estimate.RequestCosts.Standard + estimate.RequestCosts.StandardIA +
		estimate.RequestCosts.OneZoneIA + estimate.RequestCosts.IntelligentTiering +
		estimate.RequestCosts.Glacier + estimate.RequestCosts.DeepArchive

	// For intelligent estimation, use the most cost-effective storage class
	minMonthlyCost := math.Min(
		math.Min(estimate.StorageCosts.Standard, estimate.StorageCosts.StandardIA),
		math.Min(estimate.StorageCosts.Glacier, estimate.StorageCosts.DeepArchive),
	)

	estimate.TotalUploadCost = estimate.TransferCosts.Standard + estimate.RequestCosts.Standard
	estimate.TotalMonthlyCost = minMonthlyCost
	estimate.TotalAnnualCost = estimate.TotalMonthlyCost * 12

	// Generate recommendations
	estimate.Recommendations = c.generateRecommendations(archives, estimate)

	return estimate, nil
}

// calculateStorageCost calculates monthly storage cost for given size and storage class
func (c *Calculator) calculateStorageCost(ctx context.Context, sizeGB float64, storageClass config.StorageClass) float64 {
	// Use real-time pricing if available
	if c.pricingService != nil {
		priceData, err := c.pricingService.GetPricing(ctx, c.region)
		if err == nil {
			if price, exists := priceData.StoragePrice[storageClass]; exists {
				return sizeGB * price
			}
		}
		// Log warning but continue with fallback
		// Note: In production, consider more sophisticated error handling
	}

	// Fallback pricing (original static prices)
	pricePerGB := map[config.StorageClass]float64{
		config.StorageClassStandard:           0.023,   // $0.023/GB
		config.StorageClassStandardIA:         0.0125,  // $0.0125/GB
		config.StorageClassOneZoneIA:          0.01,    // $0.01/GB
		config.StorageClassIntelligentTiering: 0.0225,  // $0.0225/GB + monitoring
		config.StorageClassGlacier:            0.004,   // $0.004/GB
		config.StorageClassDeepArchive:        0.00099, // $0.00099/GB
	}

	price, exists := pricePerGB[storageClass]
	if !exists {
		price = pricePerGB[config.StorageClassStandard] // Default fallback
	}

	return sizeGB * price
}

// calculateTransferCost calculates data transfer cost (first 1GB free)
func (c *Calculator) calculateTransferCost(ctx context.Context, sizeGB float64) float64 {
	if sizeGB <= 1.0 {
		return 0.0 // First 1GB is free
	}

	chargeableGB := sizeGB - 1.0

	// Use real-time pricing if available
	if c.pricingService != nil {
		priceData, err := c.pricingService.GetPricing(ctx, c.region)
		if err == nil && priceData.TransferPrice > 0 {
			return chargeableGB * priceData.TransferPrice
		}
	}

	// Fallback pricing
	return chargeableGB * 0.09 // $0.09/GB for data transfer out
}

// calculateRequestCost calculates PUT request costs
func (c *Calculator) calculateRequestCost(ctx context.Context, numRequests int, storageClass config.StorageClass) float64 {
	// Use real-time pricing if available
	if c.pricingService != nil {
		priceData, err := c.pricingService.GetPricing(ctx, c.region)
		if err == nil {
			if price, exists := priceData.RequestPrice[storageClass]; exists {
				return (float64(numRequests) / 1000.0) * price
			}
		}
	}

	// Fallback pricing per 1,000 requests
	pricePerThousand := map[config.StorageClass]float64{
		config.StorageClassStandard:           0.005, // $0.005/1K requests
		config.StorageClassStandardIA:         0.01,  // $0.01/1K requests
		config.StorageClassOneZoneIA:          0.01,  // $0.01/1K requests
		config.StorageClassIntelligentTiering: 0.005, // $0.005/1K requests
		config.StorageClassGlacier:            0.03,  // $0.03/1K requests
		config.StorageClassDeepArchive:        0.05,  // $0.05/1K requests
	}

	price, exists := pricePerThousand[storageClass]
	if !exists {
		price = pricePerThousand[config.StorageClassStandard]
	}

	return (float64(numRequests) / 1000.0) * price
}

// generateRecommendations creates cost optimization recommendations
func (c *Calculator) generateRecommendations(archives []s3.Archive, estimate *CostEstimate) []Recommendation {
	var recommendations []Recommendation

	// Recommend Deep Archive for large archives with archival access pattern
	for _, archive := range archives {
		if archive.Size > 1024*1024*1024 && archive.AccessPattern == "archive" { // > 1GB
			sizeGB := float64(archive.Size) / (1024 * 1024 * 1024)
			monthlySavings := sizeGB * (0.023 - 0.00099) // Standard vs Deep Archive

			if monthlySavings > 1.0 { // Only recommend if saves > $1/month
				recommendations = append(recommendations, Recommendation{
					Type:             "storage_class",
					Description:      fmt.Sprintf("Move large archive to Deep Archive (%.1f GB)", sizeGB),
					EstimatedSavings: monthlySavings,
					Confidence:       0.9,
					Impact:           "high",
				})
			}
		}
	}

	// Recommend Intelligent Tiering for unknown access patterns
	if estimate.TotalSizeGB > 10 { // Only for substantial data
		unknownPatternCount := 0
		for _, archive := range archives {
			if archive.AccessPattern == "" || archive.AccessPattern == "unknown" {
				unknownPatternCount++
			}
		}

		if float64(unknownPatternCount)/float64(len(archives)) > 0.5 { // > 50% unknown patterns
			recommendations = append(recommendations, Recommendation{
				Type:             "storage_class",
				Description:      "Enable Intelligent Tiering for automatic cost optimization",
				EstimatedSavings: estimate.TotalSizeGB * 0.005, // Estimated 5% savings
				Confidence:       0.7,
				Impact:           "medium",
			})
		}
	}

	// Recommend lifecycle policies for long-term retention
	longTermCount := 0
	for _, archive := range archives {
		if archive.RetentionDays > 365 {
			longTermCount++
		}
	}

	if longTermCount > 0 {
		recommendations = append(recommendations, Recommendation{
			Type:             "lifecycle",
			Description:      "Set up lifecycle policies to automatically transition to cheaper storage",
			EstimatedSavings: estimate.TotalSizeGB * 0.01, // Estimated 10% savings
			Confidence:       0.8,
			Impact:           "high",
		})
	}

	return recommendations
}

// ComparisonEstimate represents naive vs CargoShip chunking cost comparison
// Issue #169: Show chunking benefits in cost estimates
type ComparisonEstimate struct {
	NaiveUploadCost   *CostEstimate    `json:"naive_upload_cost"`
	ChunkedUploadCost *CostEstimate    `json:"chunked_upload_cost"`
	SavingsBreakdown  SavingsBreakdown `json:"savings_breakdown"`
	Recommendations   []string         `json:"recommendations"`
	FileStats         FileStatistics   `json:"file_stats"`
	TCOScenarios      []TCOScenario    `json:"tco_scenarios,omitempty"` // Issue #169 Phase 3: Archive tier TCO scenarios
}

// SavingsBreakdown shows cost savings from chunking
type SavingsBreakdown struct {
	MinimumSizePenaltySaved float64 `json:"minimum_size_penalty_saved"`
	RequestCostSaved        float64 `json:"request_cost_saved"`
	MonitoringCostSaved     float64 `json:"monitoring_cost_saved"`
	TotalMonthlySavings     float64 `json:"total_monthly_savings"`
	SavingsPercentage       float64 `json:"savings_percentage"`
	AnnualSavings           float64 `json:"annual_savings"`

	// Issue #169 Phase 2: Detailed request cost savings
	PutRequestsSaved     int     `json:"put_requests_saved"`
	GetRequestsSaved     int     `json:"get_requests_saved"`
	ListRequestsSaved    int     `json:"list_requests_saved"`
	GetRequestCostSaved  float64 `json:"get_request_cost_saved"`
	ListRequestCostSaved float64 `json:"list_request_cost_saved"`
}

// FileStatistics provides file count and size information
type FileStatistics struct {
	TotalFiles        int     `json:"total_files"`
	TotalSizeGB       float64 `json:"total_size_gb"`
	AverageFileSizeKB float64 `json:"average_file_size_kb"`
	EstimatedChunks   int     `json:"estimated_chunks"`
}

// RequestCostBreakdown shows costs by request type
// Issue #169 Phase 2: Comprehensive request tracking
type RequestCostBreakdown struct {
	// Upload costs
	PutRequests    int     `json:"put_requests"`
	PutRequestCost float64 `json:"put_request_cost"`

	// Retrieval costs (for TCO analysis)
	GetRequests    int     `json:"get_requests"`
	GetRequestCost float64 `json:"get_request_cost"`

	// Listing costs
	ListRequests    int     `json:"list_requests"`
	ListRequestCost float64 `json:"list_request_cost"`

	// Lifecycle costs
	LifecycleTransitions    int     `json:"lifecycle_transitions"`
	LifecycleTransitionCost float64 `json:"lifecycle_transition_cost"`

	// Deletion tracking (free, but informative)
	DeleteRequests int `json:"delete_requests"`

	// Total request cost
	TotalRequestCost float64 `json:"total_request_cost"`
}

// RestoreCostBreakdown shows Glacier/Deep Archive restore costs
// Issue #169 Phase 3: Archive tier cost modeling
type RestoreCostBreakdown struct {
	// Restore tier (Expedited, Standard, Bulk)
	RestoreTier string `json:"restore_tier"`

	// Number of restore requests
	RestoreRequests int `json:"restore_requests"`

	// Restore request cost (per-request fee)
	RestoreRequestCost float64 `json:"restore_request_cost"`

	// Data size being restored (GB)
	RestoreSizeGB float64 `json:"restore_size_gb"`

	// Restore per-GB cost
	RestorePerGBCost float64 `json:"restore_per_gb_cost"`

	// Days the restored copy is kept
	RestoreDays int `json:"restore_days"`

	// Cost of temporary storage for restored data
	TemporaryStorageCost float64 `json:"temporary_storage_cost"`

	// Total restore cost
	TotalRestoreCost float64 `json:"total_restore_cost"`
}

// EarlyDeletionPenalty calculates costs for deleting before minimum duration
// Issue #169 Phase 3: Early deletion penalties
type EarlyDeletionPenalty struct {
	// Storage class being deleted from
	StorageClass config.StorageClass `json:"storage_class"`

	// Minimum storage duration (days)
	MinimumDays int `json:"minimum_days"`

	// Actual days stored before deletion
	ActualDays int `json:"actual_days"`

	// Remaining days (charged as penalty)
	RemainingDays int `json:"remaining_days"`

	// Data size (GB)
	SizeGB float64 `json:"size_gb"`

	// Penalty cost (prorated for remaining days)
	PenaltyCost float64 `json:"penalty_cost"`
}

// TCOScenario represents a Total Cost of Ownership scenario with different retrieval patterns
// Issue #169 Phase 3: Archive tier cost modeling
type TCOScenario struct {
	// Scenario name
	Name string `json:"name"`

	// Scenario description
	Description string `json:"description"`

	// Number of retrievals per year
	RetrievalsPerYear int `json:"retrievals_per_year"`

	// Percentage of data retrieved each time (0.0-1.0)
	RetrievalPercentage float64 `json:"retrieval_percentage"`

	// Restore tier to use (Expedited, Standard, Bulk)
	RestoreTier string `json:"restore_tier"`

	// Total annual cost (storage + all operations)
	AnnualCost float64 `json:"annual_cost"`

	// Cost breakdown
	StorageCost    float64 `json:"storage_cost"`    // Monthly storage
	UploadCost     float64 `json:"upload_cost"`     // One-time upload
	RetrievalCost  float64 `json:"retrieval_cost"`  // Annual retrieval
	MonitoringCost float64 `json:"monitoring_cost"` // Monthly monitoring (if applicable)
}

// Minimum object size requirements per storage tier (in bytes)
// Issue #169: Minimum object size penalties
const (
	MinObjectSizeStandardIA  = 128 * 1024 // 128 KB
	MinObjectSizeGlacier     = 40 * 1024  // 40 KB
	MinObjectSizeDeepArchive = 40 * 1024  // 40 KB
)

// INTELLIGENT_TIERING monitoring fee (per 1000 objects per month)
// Issue #169: INTELLIGENT_TIERING monitoring costs
const IntelligentTieringMonitoringFee = 0.0025

// Request cost constants (per 1000 requests)
// Issue #169 Phase 2: Comprehensive request tracking
const (
	// GET/SELECT request costs per 1000 requests
	GetRequestCostStandard           = 0.0004
	GetRequestCostStandardIA         = 0.001
	GetRequestCostOneZoneIA          = 0.001
	GetRequestCostIntelligentTiering = 0.0004
	GetRequestCostGlacier            = 0.0004 // After restore
	GetRequestCostDeepArchive        = 0.0002 // After restore

	// LIST request cost per 1000 requests (same across all tiers)
	ListRequestCost = 0.005

	// Lifecycle transition request cost per 1000 requests
	LifecycleTransitionCost = 0.01

	// DELETE request cost (free, but we track count for completeness)
	DeleteRequestCost = 0.0
)

// Glacier restore costs (Issue #169 Phase 3)
const (
	// Glacier restore request costs (per 1000 requests)
	GlacierRestoreRequestExpedited = 10.0  // $10 per 1000 requests
	GlacierRestoreRequestStandard  = 0.05  // $0.05 per 1000 requests
	GlacierRestoreRequestBulk      = 0.025 // $0.025 per 1000 requests

	// Glacier restore per-GB costs (for temporary copy)
	GlacierRestorePerGBExpedited = 0.03   // $0.03 per GB
	GlacierRestorePerGBStandard  = 0.01   // $0.01 per GB
	GlacierRestorePerGBBulk      = 0.0025 // $0.0025 per GB

	// Deep Archive restore request costs (per 1000 requests)
	DeepArchiveRestoreRequestStandard = 0.025  // $0.025 per 1000 requests
	DeepArchiveRestoreRequestBulk     = 0.0025 // $0.0025 per 1000 requests

	// Deep Archive restore per-GB costs
	DeepArchiveRestorePerGBStandard = 0.02   // $0.02 per GB
	DeepArchiveRestorePerGBBulk     = 0.0025 // $0.0025 per GB
)

// Minimum storage duration requirements (in days)
// Early deletion incurs prorated charges for remaining days
// Issue #169 Phase 3: Early deletion penalties
const (
	MinStorageDurationStandardIA  = 30  // 30 days
	MinStorageDurationOneZoneIA   = 30  // 30 days
	MinStorageDurationGlacierIR   = 90  // 90 days
	MinStorageDurationGlacier     = 90  // 90 days
	MinStorageDurationDeepArchive = 180 // 180 days
)

// EstimateWithComparison calculates naive vs chunking cost comparison
// Issue #169: Show chunking benefits in cost estimates
func (c *Calculator) EstimateWithComparison(
	ctx context.Context,
	fileCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
) (*ComparisonEstimate, error) {
	// Calculate file statistics
	avgFileSizeKB := (totalSizeGB * 1024 * 1024) / float64(fileCount)

	// Estimate chunk count (assume 100 files per chunk or 10MB per chunk, whichever is smaller)
	filesPerChunk := 100
	chunkCount := (fileCount + filesPerChunk - 1) / filesPerChunk
	if chunkCount < 1 {
		chunkCount = 1
	}

	fileStats := FileStatistics{
		TotalFiles:        fileCount,
		TotalSizeGB:       totalSizeGB,
		AverageFileSizeKB: avgFileSizeKB,
		EstimatedChunks:   chunkCount,
	}

	// Calculate naive upload cost (individual files without chunking)
	naiveCost, err := c.estimateNaiveUploadCost(ctx, fileCount, totalSizeGB, storageClass, avgFileSizeKB)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate naive upload cost: %w", err)
	}

	// Calculate chunked upload cost (with CargoShip chunking)
	chunkedCost, err := c.estimateChunkedUploadCost(ctx, chunkCount, totalSizeGB, storageClass)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate chunked upload cost: %w", err)
	}

	// Issue #169 Phase 2: Calculate GET and LIST request savings for TCO analysis
	// Assume 10% of files are accessed per month (conservative estimate for cost comparison)
	monthlyAccessRate := 0.1
	naiveGetRequests := int(float64(fileCount) * monthlyAccessRate)
	chunkedGetRequests := int(float64(chunkCount) * monthlyAccessRate)

	// Assume 20 LIST operations per month for bucket management
	naiveListRequests := int(float64(fileCount) / 1000) // 1 LIST per 1000 objects
	if naiveListRequests < 20 {
		naiveListRequests = 20
	}
	chunkedListRequests := int(float64(chunkCount) / 1000) // 1 LIST per 1000 objects
	if chunkedListRequests < 20 {
		chunkedListRequests = 20
	}

	// Calculate GET and LIST costs
	naiveGetCost := c.calculateGetRequestCost(naiveGetRequests, storageClass)
	chunkedGetCost := c.calculateGetRequestCost(chunkedGetRequests, storageClass)
	naiveListCost := c.calculateListRequestCost(naiveListRequests)
	chunkedListCost := c.calculateListRequestCost(chunkedListRequests)

	// Calculate savings breakdown
	naiveTotal := naiveCost.TotalMonthlyCost + naiveCost.TotalUploadCost
	chunkedTotal := chunkedCost.TotalMonthlyCost + chunkedCost.TotalUploadCost

	savings := SavingsBreakdown{
		MinimumSizePenaltySaved: naiveCost.TotalMonthlyCost - chunkedCost.TotalMonthlyCost,
		RequestCostSaved:        naiveCost.TotalUploadCost - chunkedCost.TotalUploadCost,
		MonitoringCostSaved:     0, // Calculated separately for INTELLIGENT_TIERING
		TotalMonthlySavings:     naiveTotal - chunkedTotal,

		// Issue #169 Phase 2: Detailed request savings
		PutRequestsSaved:     fileCount - chunkCount,
		GetRequestsSaved:     naiveGetRequests - chunkedGetRequests,
		ListRequestsSaved:    naiveListRequests - chunkedListRequests,
		GetRequestCostSaved:  naiveGetCost - chunkedGetCost,
		ListRequestCostSaved: naiveListCost - chunkedListCost,
	}

	if naiveTotal > 0 {
		savings.SavingsPercentage = (savings.TotalMonthlySavings / naiveTotal) * 100
	}
	savings.AnnualSavings = savings.TotalMonthlySavings * 12

	// Generate recommendations
	recommendations := c.generateChunkingRecommendations(fileStats, storageClass, savings)

	// Issue #169 Phase 3: Generate TCO scenarios for archive tiers
	var tcoScenarios []TCOScenario
	if storageClass == config.StorageClassGlacier || storageClass == config.StorageClassDeepArchive {
		tcoScenarios = c.generateTCOScenarios(ctx, storageClass, totalSizeGB, fileCount, naiveCost.TotalUploadCost)
	}

	comparison := &ComparisonEstimate{
		NaiveUploadCost:   naiveCost,
		ChunkedUploadCost: chunkedCost,
		SavingsBreakdown:  savings,
		Recommendations:   recommendations,
		FileStats:         fileStats,
		TCOScenarios:      tcoScenarios,
	}

	return comparison, nil
}

// estimateNaiveUploadCost calculates cost for uploading individual files without chunking
func (c *Calculator) estimateNaiveUploadCost(
	ctx context.Context,
	fileCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
	avgFileSizeKB float64,
) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Region:       c.region,
		CalculatedAt: time.Now(),
		ArchiveCount: fileCount,
		TotalSizeGB:  totalSizeGB,
	}

	// Calculate storage cost with minimum object size penalty
	chargedSizeGB := c.calculateChargedSize(fileCount, totalSizeGB, storageClass, avgFileSizeKB)
	storageCost := c.calculateStorageCost(ctx, chargedSizeGB, storageClass)

	// Calculate request cost (one PUT per file)
	requestCost := c.calculateRequestCost(ctx, fileCount, storageClass)

	// Calculate INTELLIGENT_TIERING monitoring fee if applicable
	monitoringCost := 0.0
	if storageClass == config.StorageClassIntelligentTiering {
		monitoringCost = (float64(fileCount) / 1000) * IntelligentTieringMonitoringFee
	}

	// Assign costs to estimate
	estimate.TotalMonthlyCost = storageCost + monitoringCost
	estimate.TotalUploadCost = requestCost
	estimate.TotalAnnualCost = estimate.TotalMonthlyCost * 12

	return estimate, nil
}

// estimateChunkedUploadCost calculates cost for uploading with CargoShip chunking
func (c *Calculator) estimateChunkedUploadCost(
	ctx context.Context,
	chunkCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Region:       c.region,
		CalculatedAt: time.Now(),
		ArchiveCount: chunkCount,
		TotalSizeGB:  totalSizeGB,
	}

	// Calculate storage cost (no minimum size penalty for large chunks)
	storageCost := c.calculateStorageCost(ctx, totalSizeGB, storageClass)

	// Calculate request cost (one PUT per chunk)
	requestCost := c.calculateRequestCost(ctx, chunkCount, storageClass)

	// Calculate INTELLIGENT_TIERING monitoring fee if applicable (for chunks, not files)
	monitoringCost := 0.0
	if storageClass == config.StorageClassIntelligentTiering {
		monitoringCost = (float64(chunkCount) / 1000) * IntelligentTieringMonitoringFee
	}

	// Assign costs to estimate
	estimate.TotalMonthlyCost = storageCost + monitoringCost
	estimate.TotalUploadCost = requestCost
	estimate.TotalAnnualCost = estimate.TotalMonthlyCost * 12

	return estimate, nil
}

// calculateChargedSize calculates actual charged size considering minimum object size
func (c *Calculator) calculateChargedSize(fileCount int, totalSizeGB float64, storageClass config.StorageClass, avgFileSizeKB float64) float64 {
	// Get minimum object size for storage tier
	var minSizeBytes int64
	switch storageClass {
	case config.StorageClassStandardIA:
		minSizeBytes = MinObjectSizeStandardIA
	case config.StorageClassGlacier:
		minSizeBytes = MinObjectSizeGlacier
	case config.StorageClassDeepArchive:
		minSizeBytes = MinObjectSizeDeepArchive
	default:
		// STANDARD, ONEZONE_IA, and INTELLIGENT_TIERING have no minimum
		return totalSizeGB
	}

	minSizeKB := float64(minSizeBytes) / 1024

	// If average file size is smaller than minimum, calculate penalty
	if avgFileSizeKB < minSizeKB {
		// Each file is charged for minimum size
		chargedSizeKB := float64(fileCount) * minSizeKB
		return chargedSizeKB / (1024 * 1024) // Convert to GB
	}

	// No penalty if files are larger than minimum
	return totalSizeGB
}

// calculateGetRequestCost calculates GET request costs based on storage class
// Issue #169 Phase 2: Comprehensive request tracking
func (c *Calculator) calculateGetRequestCost(numRequests int, storageClass config.StorageClass) float64 {
	costPerThousand := map[config.StorageClass]float64{
		config.StorageClassStandard:           GetRequestCostStandard,
		config.StorageClassStandardIA:         GetRequestCostStandardIA,
		config.StorageClassOneZoneIA:          GetRequestCostOneZoneIA,
		config.StorageClassIntelligentTiering: GetRequestCostIntelligentTiering,
		config.StorageClassGlacier:            GetRequestCostGlacier,
		config.StorageClassDeepArchive:        GetRequestCostDeepArchive,
	}

	cost, exists := costPerThousand[storageClass]
	if !exists {
		cost = GetRequestCostStandard // Default to STANDARD pricing
	}

	return (float64(numRequests) / 1000.0) * cost
}

// calculateListRequestCost calculates LIST request costs
// Issue #169 Phase 2: Comprehensive request tracking
func (c *Calculator) calculateListRequestCost(numRequests int) float64 {
	return (float64(numRequests) / 1000.0) * ListRequestCost
}

// calculateLifecycleTransitionCost calculates lifecycle transition request costs
// Issue #169 Phase 2: Comprehensive request tracking
// This is a utility method exported for external use by applications that need
// to calculate lifecycle transition costs independently.
//
//nolint:unused // Exported utility method for external callers
func (c *Calculator) calculateLifecycleTransitionCost(numTransitions int) float64 {
	return (float64(numTransitions) / 1000.0) * LifecycleTransitionCost
}

// generateChunkingRecommendations generates cost optimization recommendations for chunking
func (c *Calculator) generateChunkingRecommendations(stats FileStatistics, storageClass config.StorageClass, savings SavingsBreakdown) []string {
	var recommendations []string

	// Savings percentage recommendations
	if savings.SavingsPercentage > 75 {
		recommendations = append(recommendations, "🎉 Excellent! CargoShip chunking provides >75% cost savings for this workload")
	} else if savings.SavingsPercentage > 50 {
		recommendations = append(recommendations, "✅ Great! CargoShip chunking provides >50% cost savings for this workload")
	} else if savings.SavingsPercentage > 25 {
		recommendations = append(recommendations, "👍 Good! CargoShip chunking provides >25% cost savings for this workload")
	}

	// File size recommendations
	if stats.AverageFileSizeKB < 40 {
		recommendations = append(recommendations, "💡 Small files (<40 KB average) - chunking eliminates 4x minimum size penalty on archive tiers")
	} else if stats.AverageFileSizeKB < 128 {
		recommendations = append(recommendations, "💡 Small files (<128 KB average) - chunking eliminates 2-4x minimum size penalty on some tiers")
	}

	// Request cost recommendations
	if stats.TotalFiles > 10000 {
		recommendations = append(recommendations, "💡 High file count - chunking reduces PUT requests by 99%, saving on request costs")
	}

	// INTELLIGENT_TIERING recommendations
	if storageClass == config.StorageClassIntelligentTiering {
		if stats.TotalFiles > 100000 {
			recommendations = append(recommendations, "💡 INTELLIGENT_TIERING with 100k+ files - chunking reduces monitoring fees by 99.9%")
		}
	}

	// Annual savings recommendations
	if savings.AnnualSavings > 1000 {
		recommendations = append(recommendations, fmt.Sprintf("💰 Annual savings: $%.2f - significant cost reduction!", savings.AnnualSavings))
	} else if savings.AnnualSavings > 100 {
		recommendations = append(recommendations, fmt.Sprintf("💰 Annual savings: $%.2f - meaningful cost reduction", savings.AnnualSavings))
	}

	return recommendations
}

// calculateRestoreCost calculates Glacier or Deep Archive restore costs
// Issue #169 Phase 3: Archive tier cost modeling
func (c *Calculator) calculateRestoreCost(
	ctx context.Context,
	storageClass config.StorageClass,
	restoreTier string,
	numObjects int,
	sizeGB float64,
	restoreDays int,
) *RestoreCostBreakdown {
	breakdown := &RestoreCostBreakdown{
		RestoreTier:     restoreTier,
		RestoreRequests: numObjects,
		RestoreSizeGB:   sizeGB,
	}

	// Calculate request costs and per-GB costs based on storage class and tier
	switch storageClass {
	case config.StorageClassGlacier:
		switch restoreTier {
		case "Expedited":
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * GlacierRestoreRequestExpedited
			breakdown.RestorePerGBCost = sizeGB * GlacierRestorePerGBExpedited
		case "Standard":
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * GlacierRestoreRequestStandard
			breakdown.RestorePerGBCost = sizeGB * GlacierRestorePerGBStandard
		case "Bulk":
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * GlacierRestoreRequestBulk
			breakdown.RestorePerGBCost = sizeGB * GlacierRestorePerGBBulk
		default:
			// Default to Standard
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * GlacierRestoreRequestStandard
			breakdown.RestorePerGBCost = sizeGB * GlacierRestorePerGBStandard
		}

	case config.StorageClassDeepArchive:
		switch restoreTier {
		case "Standard":
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * DeepArchiveRestoreRequestStandard
			breakdown.RestorePerGBCost = sizeGB * DeepArchiveRestorePerGBStandard
		case "Bulk":
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * DeepArchiveRestoreRequestBulk
			breakdown.RestorePerGBCost = sizeGB * DeepArchiveRestorePerGBBulk
		default:
			// Default to Standard
			breakdown.RestoreRequestCost = (float64(numObjects) / 1000) * DeepArchiveRestoreRequestStandard
			breakdown.RestorePerGBCost = sizeGB * DeepArchiveRestorePerGBStandard
		}

	default:
		// Non-archive tiers don't have restore costs
		return breakdown
	}

	// Calculate temporary storage cost (restored objects stored in STANDARD for access)
	// Assume restored data is kept for specified number of days
	breakdown.RestoreDays = restoreDays
	if restoreDays > 0 {
		standardStorageCost := c.calculateStorageCost(ctx, sizeGB, config.StorageClassStandard)
		breakdown.TemporaryStorageCost = standardStorageCost * (float64(restoreDays) / 30.0)
	}

	// Total restore cost
	breakdown.TotalRestoreCost = breakdown.RestoreRequestCost +
		breakdown.RestorePerGBCost +
		breakdown.TemporaryStorageCost

	return breakdown
}

// calculateEarlyDeletionPenalty calculates prorated charges for deleting before minimum storage duration
// Issue #169 Phase 3: Archive tier cost modeling
// This is a utility method exported for external use by applications that need
// to calculate early deletion penalties for archive tiers independently.
//
//nolint:unused // Exported utility method for external callers
func (c *Calculator) calculateEarlyDeletionPenalty(
	ctx context.Context,
	storageClass config.StorageClass,
	actualDays int,
	sizeGB float64,
) *EarlyDeletionPenalty {
	penalty := &EarlyDeletionPenalty{
		StorageClass: storageClass,
		ActualDays:   actualDays,
		SizeGB:       sizeGB,
	}

	// Get minimum storage duration for this storage class
	switch storageClass {
	case config.StorageClassStandardIA:
		penalty.MinimumDays = MinStorageDurationStandardIA
	case config.StorageClassOneZoneIA:
		penalty.MinimumDays = MinStorageDurationOneZoneIA
	case config.StorageClassGlacier:
		penalty.MinimumDays = MinStorageDurationGlacier
	case config.StorageClassDeepArchive:
		penalty.MinimumDays = MinStorageDurationDeepArchive
	default:
		// No minimum storage duration for STANDARD or INTELLIGENT_TIERING
		return penalty
	}

	// If deleted before minimum duration, calculate penalty
	if actualDays < penalty.MinimumDays {
		penalty.RemainingDays = penalty.MinimumDays - actualDays

		// Calculate monthly storage cost for this tier
		monthlyStorageCost := c.calculateStorageCost(ctx, sizeGB, storageClass)

		// Prorated penalty for remaining days
		penalty.PenaltyCost = monthlyStorageCost * (float64(penalty.RemainingDays) / 30.0)
	}

	return penalty
}

// generateTCOScenarios generates Total Cost of Ownership scenarios with different retrieval patterns
// Issue #169 Phase 3: Archive tier cost modeling
func (c *Calculator) generateTCOScenarios(
	ctx context.Context,
	storageClass config.StorageClass,
	sizeGB float64,
	numObjects int,
	uploadCost float64,
) []TCOScenario {
	var scenarios []TCOScenario

	// Only generate scenarios for archive tiers
	if storageClass != config.StorageClassGlacier && storageClass != config.StorageClassDeepArchive {
		return scenarios
	}

	// Base storage cost (monthly)
	monthlyStorageCost := c.calculateStorageCost(ctx, sizeGB, storageClass)

	// Monitoring fees (if INTELLIGENT_TIERING)
	monthlyMonitoringCost := 0.0
	if storageClass == config.StorageClassIntelligentTiering {
		monthlyMonitoringCost = (float64(numObjects) / 1000) * IntelligentTieringMonitoringFee
	}

	// Scenario 1: Zero retrievals (pure archival)
	scenarios = append(scenarios, TCOScenario{
		Name:                "Zero Retrievals (Pure Archival)",
		Description:         "Data stored long-term with no retrievals",
		RetrievalsPerYear:   0,
		RetrievalPercentage: 0.0,
		RestoreTier:         "N/A",
		AnnualCost:          (monthlyStorageCost+monthlyMonitoringCost)*12 + uploadCost,
		StorageCost:         monthlyStorageCost * 12,
		UploadCost:          uploadCost,
		RetrievalCost:       0.0,
		MonitoringCost:      monthlyMonitoringCost * 12,
	})

	// Scenario 2: Occasional retrievals (2x/year, 10% of data, Standard restore)
	switch storageClass {
	case config.StorageClassGlacier:
		retrievalSize := sizeGB * 0.1
		retrievalObjects := int(float64(numObjects) * 0.1)
		restoreCost := c.calculateRestoreCost(ctx, storageClass, "Standard", retrievalObjects, retrievalSize, 7)
		annualRetrievalCost := restoreCost.TotalRestoreCost * 2

		scenarios = append(scenarios, TCOScenario{
			Name:                "Occasional Retrievals (2x/year)",
			Description:         "Retrieve 10% of data twice yearly (Standard restore, 7-day access)",
			RetrievalsPerYear:   2,
			RetrievalPercentage: 0.1,
			RestoreTier:         "Standard",
			AnnualCost:          (monthlyStorageCost+monthlyMonitoringCost)*12 + uploadCost + annualRetrievalCost,
			StorageCost:         monthlyStorageCost * 12,
			UploadCost:          uploadCost,
			RetrievalCost:       annualRetrievalCost,
			MonitoringCost:      monthlyMonitoringCost * 12,
		})
	case config.StorageClassDeepArchive:
		retrievalSize := sizeGB * 0.05
		retrievalObjects := int(float64(numObjects) * 0.05)
		restoreCost := c.calculateRestoreCost(ctx, storageClass, "Standard", retrievalObjects, retrievalSize, 7)
		annualRetrievalCost := restoreCost.TotalRestoreCost * 1

		scenarios = append(scenarios, TCOScenario{
			Name:                "Rare Retrievals (1x/year)",
			Description:         "Retrieve 5% of data once yearly (Standard restore, 7-day access)",
			RetrievalsPerYear:   1,
			RetrievalPercentage: 0.05,
			RestoreTier:         "Standard",
			AnnualCost:          (monthlyStorageCost+monthlyMonitoringCost)*12 + uploadCost + annualRetrievalCost,
			StorageCost:         monthlyStorageCost * 12,
			UploadCost:          uploadCost,
			RetrievalCost:       annualRetrievalCost,
			MonitoringCost:      monthlyMonitoringCost * 12,
		})
	}

	// Scenario 3: Frequent retrievals (warning: archive tiers not suitable)
	if storageClass == config.StorageClassGlacier {
		retrievalSize := sizeGB * 0.2
		retrievalObjects := int(float64(numObjects) * 0.2)
		restoreCost := c.calculateRestoreCost(ctx, storageClass, "Bulk", retrievalObjects, retrievalSize, 3)
		annualRetrievalCost := restoreCost.TotalRestoreCost * 10

		scenarios = append(scenarios, TCOScenario{
			Name:                "Frequent Retrievals (10x/year) ⚠️",
			Description:         "Retrieve 20% of data 10x yearly (Bulk restore, 3-day access) - NOT RECOMMENDED",
			RetrievalsPerYear:   10,
			RetrievalPercentage: 0.2,
			RestoreTier:         "Bulk",
			AnnualCost:          (monthlyStorageCost+monthlyMonitoringCost)*12 + uploadCost + annualRetrievalCost,
			StorageCost:         monthlyStorageCost * 12,
			UploadCost:          uploadCost,
			RetrievalCost:       annualRetrievalCost,
			MonitoringCost:      monthlyMonitoringCost * 12,
		})
	}

	return scenarios
}
