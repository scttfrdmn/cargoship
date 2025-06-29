// Package costs provides cost estimation and optimization for CargoShip AWS operations
package costs

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/scttfrdmn/cargoship-cli/pkg/aws/config"
	"github.com/scttfrdmn/cargoship-cli/pkg/aws/pricing"
	"github.com/scttfrdmn/cargoship-cli/pkg/aws/s3"
)

// Calculator provides cost estimation for S3 operations
type Calculator struct {
	region        string
	pricingService *pricing.Service
	lastUpdate    time.Time
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
	TotalUploadCost   float64 `json:"total_upload_cost"`   // One-time upload cost
	TotalMonthlyCost  float64 `json:"total_monthly_cost"`  // Monthly storage cost
	TotalAnnualCost   float64 `json:"total_annual_cost"`   // Annual cost projection
	
	// Optimization recommendations
	Recommendations []Recommendation `json:"recommendations"`
	
	// Metadata
	Region        string    `json:"region"`
	CalculatedAt  time.Time `json:"calculated_at"`
	TotalSizeGB   float64   `json:"total_size_gb"`
	ArchiveCount  int       `json:"archive_count"`
}

// CostBreakdown shows costs by storage class
type CostBreakdown struct {
	Standard          float64 `json:"standard"`
	StandardIA        float64 `json:"standard_ia"`
	OneZoneIA         float64 `json:"onezone_ia"`
	IntelligentTiering float64 `json:"intelligent_tiering"`
	Glacier           float64 `json:"glacier"`
	DeepArchive       float64 `json:"deep_archive"`
	Total             float64 `json:"total"`
}

// Recommendation represents a cost optimization suggestion
type Recommendation struct {
	Type            string  `json:"type"`             // "storage_class", "lifecycle", "compression"
	Description     string  `json:"description"`      // Human-readable description
	EstimatedSavings float64 `json:"estimated_savings"` // Monthly savings in USD
	Confidence      float64 `json:"confidence"`       // Confidence level (0.0-1.0)
	Impact          string  `json:"impact"`           // "low", "medium", "high"
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
		config.StorageClassStandard:           0.023,  // $0.023/GB
		config.StorageClassStandardIA:         0.0125, // $0.0125/GB
		config.StorageClassOneZoneIA:          0.01,   // $0.01/GB
		config.StorageClassIntelligentTiering: 0.0225, // $0.0225/GB + monitoring
		config.StorageClassGlacier:            0.004,  // $0.004/GB
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
		config.StorageClassStandard:           0.005,  // $0.005/1K requests
		config.StorageClassStandardIA:         0.01,   // $0.01/1K requests
		config.StorageClassOneZoneIA:          0.01,   // $0.01/1K requests
		config.StorageClassIntelligentTiering: 0.005,  // $0.005/1K requests
		config.StorageClassGlacier:            0.03,   // $0.03/1K requests
		config.StorageClassDeepArchive:        0.05,   // $0.05/1K requests
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
					Type:            "storage_class",
					Description:     fmt.Sprintf("Move large archive to Deep Archive (%.1f GB)", sizeGB),
					EstimatedSavings: monthlySavings,
					Confidence:      0.9,
					Impact:          "high",
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
				Type:            "storage_class",
				Description:     "Enable Intelligent Tiering for automatic cost optimization",
				EstimatedSavings: estimate.TotalSizeGB * 0.005, // Estimated 5% savings
				Confidence:      0.7,
				Impact:          "medium",
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
			Type:            "lifecycle",
			Description:     "Set up lifecycle policies to automatically transition to cheaper storage",
			EstimatedSavings: estimate.TotalSizeGB * 0.01, // Estimated 10% savings
			Confidence:      0.8,
			Impact:          "high",
		})
	}
	
	return recommendations
}