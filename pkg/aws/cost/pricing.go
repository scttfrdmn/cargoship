// Package cost provides AWS cost calculation and pricing management for CargoShip
package cost

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricingfallback"
)

// PricingManager handles cost calculations with AWS Pricing API integration
type PricingManager struct {
	config     *config.PricingConfig
	pricingAPI pricingAPIClient
	logger     *slog.Logger
	cache      *PricingCache
	mu         sync.RWMutex
}

// PricingCache holds cached pricing data
type PricingCache struct {
	data       map[string]CachedPrice
	lastUpdate time.Time
	duration   time.Duration
	mu         sync.RWMutex
}

// CachedPrice represents a cached price entry
type CachedPrice struct {
	Price     float64   `json:"price"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "aws_api", "custom", "fallback"
}

// CostEstimate represents a cost estimate for an operation
type CostEstimate struct {
	StorageCost      float64           `json:"storage_cost"`
	RequestCost      float64           `json:"request_cost"`
	DataTransferCost float64           `json:"data_transfer_cost"`
	TotalCost        float64           `json:"total_cost"`
	Currency         string            `json:"currency"`
	EstimatedAt      time.Time         `json:"estimated_at"`
	Discounts        DiscountBreakdown `json:"discounts"`
	Breakdown        CostBreakdown     `json:"breakdown"`
}

// DiscountBreakdown shows applied discounts
type DiscountBreakdown struct {
	GlobalDiscount       float64 `json:"global_discount"`
	ServiceDiscount      float64 `json:"service_discount"`
	VolumeDiscount       float64 `json:"volume_discount"`
	ReservedDiscount     float64 `json:"reserved_discount"`
	SavingsPlansDiscount float64 `json:"savings_plans_discount"`
	EnterpriseDiscount   float64 `json:"enterprise_discount"`
	TotalDiscount        float64 `json:"total_discount"`
	OriginalCost         float64 `json:"original_cost"`
	DiscountedCost       float64 `json:"discounted_cost"`
}

// CostBreakdown provides detailed cost breakdown
type CostBreakdown struct {
	StorageBreakdown  map[string]float64 `json:"storage_breakdown"`
	RequestBreakdown  map[string]float64 `json:"request_breakdown"`
	TransferBreakdown map[string]float64 `json:"transfer_breakdown"`
	ServiceBreakdown  map[string]float64 `json:"service_breakdown"`
	RegionBreakdown   map[string]float64 `json:"region_breakdown"`
}

// NewPricingManager creates a new pricing manager
func NewPricingManager(cfg *config.PricingConfig, awsCfg aws.Config, logger *slog.Logger) (*PricingManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("pricing config cannot be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	cacheDuration, err := time.ParseDuration(cfg.PricingCacheDuration)
	if err != nil {
		cacheDuration = 24 * time.Hour // Default to 24 hours
		logger.Warn("Invalid pricing cache duration, using default", "duration", "24h", "error", err)
	}

	pm := &PricingManager{
		config: cfg,
		logger: logger.With("component", "pricing-manager"),
		cache: &PricingCache{
			data:     make(map[string]CachedPrice),
			duration: cacheDuration,
		},
	}

	// Initialize AWS Pricing API client if enabled
	if cfg.UseAWSPricingAPI {
		// AWS Pricing API is only available in us-east-1
		pricingConfig := awsCfg.Copy()
		pricingConfig.Region = "us-east-1"
		pm.pricingAPI = pricing.NewFromConfig(pricingConfig)
	}

	return pm, nil
}

// EstimateArchivalCost estimates the cost for archiving data
func (pm *PricingManager) EstimateArchivalCost(ctx context.Context, sizeGB float64, storageClass config.StorageClass, region string) (*CostEstimate, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	estimate := &CostEstimate{
		Currency:    pm.config.Currency,
		EstimatedAt: time.Now(),
		Breakdown: CostBreakdown{
			StorageBreakdown:  make(map[string]float64),
			RequestBreakdown:  make(map[string]float64),
			TransferBreakdown: make(map[string]float64),
			ServiceBreakdown:  make(map[string]float64),
			RegionBreakdown:   make(map[string]float64),
		},
	}

	// Calculate storage cost
	storagePrice, err := pm.getStoragePrice(ctx, storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage price: %w", err)
	}

	estimate.StorageCost = sizeGB * storagePrice
	estimate.Breakdown.StorageBreakdown[string(storageClass)] = estimate.StorageCost

	// Calculate request cost (PUT requests for upload)
	requestPrice, err := pm.getRequestPrice(ctx, "PUT", storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get request price: %w", err)
	}

	// Estimate number of requests based on multipart upload (assume 100MB chunks)
	numRequests := sizeGB * 1024 / 100 // Convert GB to 100MB chunks
	if numRequests < 1 {
		numRequests = 1 // At least one request
	}

	estimate.RequestCost = (numRequests / 1000) * requestPrice // Pricing is per 1000 requests
	estimate.Breakdown.RequestBreakdown["PUT"] = estimate.RequestCost

	// Calculate data transfer cost (assume upload from internet)
	transferPrice, err := pm.getDataTransferPrice(ctx, "upload", region)
	if err != nil {
		pm.logger.Warn("Failed to get data transfer price, assuming free", "error", err)
		transferPrice = 0 // Uploads are typically free
	}

	estimate.DataTransferCost = sizeGB * transferPrice
	estimate.Breakdown.TransferBreakdown["upload"] = estimate.DataTransferCost

	// Calculate total before discounts
	originalCost := estimate.StorageCost + estimate.RequestCost + estimate.DataTransferCost

	// Apply discounts
	discounts := pm.calculateDiscounts(originalCost, "s3", region)
	estimate.Discounts = discounts

	// Set final costs
	estimate.TotalCost = discounts.DiscountedCost
	estimate.Breakdown.ServiceBreakdown["s3"] = estimate.TotalCost
	estimate.Breakdown.RegionBreakdown[region] = estimate.TotalCost

	return estimate, nil
}

// getStoragePrice retrieves storage pricing for the specified storage class and region
func (pm *PricingManager) getStoragePrice(ctx context.Context, storageClass config.StorageClass, region string) (float64, error) {
	cacheKey := fmt.Sprintf("storage_%s_%s", storageClass, region)

	// Check cache first
	if price, ok := pm.getCachedPrice(cacheKey); ok {
		return price.Price, nil
	}

	var price float64
	var err error

	// Check custom pricing first
	if customPricing, exists := pm.config.CustomPricing[region]; exists {
		if storagePrice, exists := customPricing.S3Storage[storageClass]; exists {
			price = storagePrice
			pm.setCachedPrice(cacheKey, price, "custom")
			return price, nil
		}
	}

	// Use AWS Pricing API if enabled
	if pm.config.UseAWSPricingAPI && pm.pricingAPI != nil {
		price, err = pm.getAWSStoragePrice(ctx, storageClass, region)
		if err != nil {
			pm.logger.Warn("Failed to get AWS pricing, using fallback", "error", err)
			price = pm.getFallbackStoragePrice(storageClass)
		} else {
			pm.setCachedPrice(cacheKey, price, "aws_api")
		}
	} else {
		price = pm.getFallbackStoragePrice(storageClass)
		pm.setCachedPrice(cacheKey, price, "fallback")
	}

	return price, nil
}

// getRequestPrice retrieves request pricing
func (pm *PricingManager) getRequestPrice(ctx context.Context, requestType string, storageClass config.StorageClass, region string) (float64, error) {
	cacheKey := fmt.Sprintf("request_%s_%s_%s", requestType, storageClass, region)

	// Check cache first
	if price, ok := pm.getCachedPrice(cacheKey); ok {
		return price.Price, nil
	}

	var price float64

	// Check custom pricing first
	if customPricing, exists := pm.config.CustomPricing[region]; exists {
		switch strings.ToUpper(requestType) {
		case "PUT", "POST", "COPY", "LIST":
			price = customPricing.S3Requests.PutRequests
		case "GET", "SELECT":
			price = customPricing.S3Requests.GetRequests
		case "DELETE":
			price = customPricing.S3Requests.DeleteRequests
		default:
			price = customPricing.S3Requests.PutRequests // Default to PUT pricing
		}
		pm.setCachedPrice(cacheKey, price, "custom")
		return price, nil
	}

	// Use AWS Pricing API if enabled, falling back to the static table on error.
	if pm.config.UseAWSPricingAPI && pm.pricingAPI != nil {
		apiPrice, err := pm.getAWSRequestPrice(ctx, strings.ToUpper(requestType), region)
		if err != nil {
			pm.logger.Warn("Failed to get AWS request pricing, using fallback", "error", err)
			price = pm.getFallbackRequestPrice(requestType, storageClass)
			pm.setCachedPrice(cacheKey, price, "fallback")
		} else {
			price = apiPrice
			pm.setCachedPrice(cacheKey, price, "aws_api")
		}
	} else {
		price = pm.getFallbackRequestPrice(requestType, storageClass)
		pm.setCachedPrice(cacheKey, price, "fallback")
	}

	return price, nil
}

// getDataTransferPrice retrieves data transfer pricing
func (pm *PricingManager) getDataTransferPrice(ctx context.Context, transferType, region string) (float64, error) {
	cacheKey := fmt.Sprintf("transfer_%s_%s", transferType, region)

	// Check cache first
	if price, ok := pm.getCachedPrice(cacheKey); ok {
		return price.Price, nil
	}

	var price float64

	// Check custom pricing first
	if customPricing, exists := pm.config.CustomPricing[region]; exists {
		switch transferType {
		case "upload":
			price = 0 // Uploads are typically free
		case "download":
			if outPrice, exists := customPricing.DataTransfer.OutToInternet[region]; exists {
				price = outPrice
			}
		}
		pm.setCachedPrice(cacheKey, price, "custom")
		return price, nil
	}

	// Use fallback pricing
	switch transferType {
	case "upload":
		price = 0 // Uploads are free
	case "download":
		price = 0.09 // Typical internet egress pricing
	default:
		price = 0
	}

	pm.setCachedPrice(cacheKey, price, "fallback")
	return price, nil
}

// calculateDiscounts applies all configured discounts
func (pm *PricingManager) calculateDiscounts(originalCost float64, service, region string) DiscountBreakdown {
	breakdown := DiscountBreakdown{
		OriginalCost: originalCost,
	}

	discountedCost := originalCost

	// Apply global discount
	if pm.config.GlobalDiscount > 0 {
		globalDiscount := originalCost * pm.config.GlobalDiscount
		breakdown.GlobalDiscount = globalDiscount
		discountedCost -= globalDiscount
	}

	// Apply service-specific discount
	if serviceDiscount, exists := pm.config.ServiceDiscounts[service]; exists && serviceDiscount > 0 {
		serviceDiscountAmount := originalCost * serviceDiscount
		breakdown.ServiceDiscount = serviceDiscountAmount
		discountedCost -= serviceDiscountAmount
	}

	// Apply enterprise discounts
	if pm.config.EnterpriseDiscount.Enabled {
		enterpriseDiscount := pm.calculateEnterpriseDiscount(originalCost, service)
		breakdown.EnterpriseDiscount = enterpriseDiscount
		discountedCost -= enterpriseDiscount
	}

	// Apply Reserved Instance discounts (for applicable services)
	if riDiscount, exists := pm.config.ReservedInstanceDiscounts[service]; exists {
		riDiscountAmount := originalCost * riDiscount.Discount
		breakdown.ReservedDiscount = riDiscountAmount
		discountedCost -= riDiscountAmount
	}

	// Apply Savings Plans discounts
	if spDiscount, exists := pm.config.SavingsPlansDiscounts[service]; exists {
		spDiscountAmount := originalCost * spDiscount.Discount
		breakdown.SavingsPlansDiscount = spDiscountAmount
		discountedCost -= spDiscountAmount
	}

	// Calculate total discount
	breakdown.TotalDiscount = originalCost - discountedCost
	breakdown.DiscountedCost = discountedCost

	// Ensure discounted cost is not negative
	if breakdown.DiscountedCost < 0 {
		breakdown.DiscountedCost = 0
	}

	return breakdown
}

// calculateEnterpriseDiscount calculates enterprise volume discounts
func (pm *PricingManager) calculateEnterpriseDiscount(cost float64, service string) float64 {
	if !pm.config.EnterpriseDiscount.Enabled {
		return 0
	}

	// Apply volume tier discounts
	var volumeDiscount float64
	for _, tier := range pm.config.EnterpriseDiscount.VolumeTiers {
		if cost >= tier.MinimumSpend {
			// Check if service is covered by this tier
			serviceCovered := len(tier.Services) == 0 // Empty means all services
			for _, svc := range tier.Services {
				if svc == service {
					serviceCovered = true
					break
				}
			}

			if serviceCovered && tier.Discount > volumeDiscount {
				volumeDiscount = tier.Discount
			}
		}
	}

	enterpriseDiscount := cost * volumeDiscount

	// Add annual commitment discount
	if pm.config.EnterpriseDiscount.AnnualCommitmentDiscount > 0 {
		enterpriseDiscount += cost * pm.config.EnterpriseDiscount.AnnualCommitmentDiscount
	}

	return enterpriseDiscount
}

// Helper methods for AWS Pricing API integration

// getAWSStoragePrice queries the AWS Price List API for the per-GB-month storage
// price of storageClass in region. Returns an error (so the caller falls back to
// the static table) when the storage class has no Price List mapping or the
// query yields no usable price.
func (pm *PricingManager) getAWSStoragePrice(ctx context.Context, storageClass config.StorageClass, region string) (float64, error) {
	volumeType, ok := s3StorageVolumeType(storageClass)
	if !ok {
		return 0, fmt.Errorf("no Price List volumeType for storage class %q", storageClass)
	}
	return pm.queryAWSPrice(ctx, region, map[string]string{
		"productFamily": "Storage",
		"volumeType":    volumeType,
	})
}

// getAWSRequestPrice queries the AWS Price List API for the per-request price of
// requestType in region, returned as a price per 1,000 requests to match the
// rest of the pricing code. Returns an error (caller falls back) when the
// request type has no Price List mapping or the query yields no usable price.
func (pm *PricingManager) getAWSRequestPrice(ctx context.Context, requestType, region string) (float64, error) {
	group, ok := s3RequestGroup(requestType)
	if !ok {
		return 0, fmt.Errorf("no Price List group for request type %q", requestType)
	}
	// The API returns a per-request price; the rest of the code works in
	// price-per-1,000-requests, so scale up.
	perRequest, err := pm.queryAWSPrice(ctx, region, map[string]string{
		"productFamily": "API Request",
		"group":         group,
	})
	if err != nil {
		return 0, err
	}
	return perRequest * 1000, nil
}

// Fallback pricing methods delegate to the canonical pricingfallback tables so
// the numbers are defined in exactly one place (#237).
func (pm *PricingManager) getFallbackStoragePrice(storageClass config.StorageClass) float64 {
	return pricingfallback.StoragePrice(storageClass)
}

// getFallbackRequestPrice returns the per-1000 request price for a verb and
// storage class. PUT/COPY/POST/LIST prices vary by storage class (archival
// classes cost far more per request than Standard); GET/SELECT and DELETE are
// verb-only. Previously this ignored storageClass and priced every PUT at the
// Standard rate, undercounting Glacier/Deep-Archive uploads (#237).
func (pm *PricingManager) getFallbackRequestPrice(requestType string, storageClass config.StorageClass) float64 {
	return pricingfallback.RequestPrice(requestType, storageClass)
}

// Cache management methods
func (pm *PricingManager) getCachedPrice(key string) (CachedPrice, bool) {
	pm.cache.mu.RLock()
	defer pm.cache.mu.RUnlock()

	price, exists := pm.cache.data[key]
	if !exists {
		return CachedPrice{}, false
	}

	// Check if cache entry is still valid
	if time.Since(price.Timestamp) > pm.cache.duration {
		return CachedPrice{}, false
	}

	return price, true
}

func (pm *PricingManager) setCachedPrice(key string, price float64, source string) {
	pm.cache.mu.Lock()
	defer pm.cache.mu.Unlock()

	pm.cache.data[key] = CachedPrice{
		Price:     price,
		Currency:  pm.config.Currency,
		Timestamp: time.Now(),
		Source:    source,
	}
}

// ClearCache clears the pricing cache
func (pm *PricingManager) ClearCache() {
	pm.cache.mu.Lock()
	defer pm.cache.mu.Unlock()

	pm.cache.data = make(map[string]CachedPrice)
	pm.logger.Info("Pricing cache cleared")
}

// GetCacheStats returns cache statistics
func (pm *PricingManager) GetCacheStats() map[string]interface{} {
	pm.cache.mu.RLock()
	defer pm.cache.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_entries"] = len(pm.cache.data)
	stats["cache_duration"] = pm.cache.duration.String()
	stats["last_update"] = pm.cache.lastUpdate

	sourceCounts := make(map[string]int)
	for _, entry := range pm.cache.data {
		sourceCounts[entry.Source]++
	}
	stats["sources"] = sourceCounts

	return stats
}

// ComparisonEstimate represents naive vs CargoShip chunking cost comparison
// Issue #169: Show chunking benefits in cost estimates
type ComparisonEstimate struct {
	NaiveUploadCost   *CostEstimate    `json:"naive_upload_cost"`
	ChunkedUploadCost *CostEstimate    `json:"chunked_upload_cost"`
	SavingsBreakdown  SavingsBreakdown `json:"savings_breakdown"`
	Recommendations   []string         `json:"recommendations"`
	FileStats         FileStatistics   `json:"file_stats"`
}

// SavingsBreakdown shows cost savings from chunking
type SavingsBreakdown struct {
	MinimumSizePenaltySaved float64 `json:"minimum_size_penalty_saved"`
	RequestCostSaved        float64 `json:"request_cost_saved"`
	MonitoringCostSaved     float64 `json:"monitoring_cost_saved"`
	TotalMonthlySavings     float64 `json:"total_monthly_savings"`
	SavingsPercentage       float64 `json:"savings_percentage"`
	AnnualSavings           float64 `json:"annual_savings"`
}

// FileStatistics provides file count and size information
type FileStatistics struct {
	TotalFiles        int     `json:"total_files"`
	TotalSizeGB       float64 `json:"total_size_gb"`
	AverageFileSizeKB float64 `json:"average_file_size_kb"`
	EstimatedChunks   int     `json:"estimated_chunks"`
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

// EstimateWithComparison calculates naive vs chunking cost comparison
// Issue #169: Show chunking benefits in cost estimates
func (pm *PricingManager) EstimateWithComparison(
	ctx context.Context,
	fileCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
	region string,
	chunkCount int,
) (*ComparisonEstimate, error) {
	// Calculate file statistics
	avgFileSizeKB := (totalSizeGB * 1024 * 1024) / float64(fileCount)

	fileStats := FileStatistics{
		TotalFiles:        fileCount,
		TotalSizeGB:       totalSizeGB,
		AverageFileSizeKB: avgFileSizeKB,
		EstimatedChunks:   chunkCount,
	}

	// Calculate naive upload cost (individual files without chunking)
	naiveCost, err := pm.estimateNaiveUploadCost(ctx, fileCount, totalSizeGB, storageClass, region, avgFileSizeKB)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate naive upload cost: %w", err)
	}

	// Calculate chunked upload cost (with CargoShip chunking)
	chunkedCost, err := pm.estimateChunkedUploadCost(ctx, chunkCount, totalSizeGB, storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate chunked upload cost: %w", err)
	}

	// Calculate savings breakdown
	savings := SavingsBreakdown{
		MinimumSizePenaltySaved: naiveCost.Breakdown.StorageBreakdown["minimum_size_penalty"],
		RequestCostSaved:        naiveCost.RequestCost - chunkedCost.RequestCost,
		MonitoringCostSaved:     naiveCost.Breakdown.ServiceBreakdown["monitoring"] - chunkedCost.Breakdown.ServiceBreakdown["monitoring"],
		TotalMonthlySavings:     naiveCost.TotalCost - chunkedCost.TotalCost,
	}

	if naiveCost.TotalCost > 0 {
		savings.SavingsPercentage = (savings.TotalMonthlySavings / naiveCost.TotalCost) * 100
	}
	savings.AnnualSavings = savings.TotalMonthlySavings * 12

	// Generate recommendations
	recommendations := pm.generateRecommendations(fileStats, storageClass, savings)

	comparison := &ComparisonEstimate{
		NaiveUploadCost:   naiveCost,
		ChunkedUploadCost: chunkedCost,
		SavingsBreakdown:  savings,
		Recommendations:   recommendations,
		FileStats:         fileStats,
	}

	return comparison, nil
}

// estimateNaiveUploadCost calculates cost for uploading individual files without chunking
func (pm *PricingManager) estimateNaiveUploadCost(
	ctx context.Context,
	fileCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
	region string,
	avgFileSizeKB float64,
) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Currency:    pm.config.Currency,
		EstimatedAt: time.Now(),
		Breakdown: CostBreakdown{
			StorageBreakdown:  make(map[string]float64),
			RequestBreakdown:  make(map[string]float64),
			TransferBreakdown: make(map[string]float64),
			ServiceBreakdown:  make(map[string]float64),
			RegionBreakdown:   make(map[string]float64),
		},
	}

	// Calculate storage cost with minimum object size penalty
	storagePrice, err := pm.getStoragePrice(ctx, storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage price: %w", err)
	}

	// Calculate charged size considering minimum object size
	chargedSizeGB := pm.calculateChargedSize(fileCount, totalSizeGB, storageClass, avgFileSizeKB)
	minSizePenalty := (chargedSizeGB - totalSizeGB) * storagePrice

	estimate.StorageCost = chargedSizeGB * storagePrice
	estimate.Breakdown.StorageBreakdown[string(storageClass)] = totalSizeGB * storagePrice
	estimate.Breakdown.StorageBreakdown["minimum_size_penalty"] = minSizePenalty

	// Calculate request cost (one PUT per file)
	requestPrice, err := pm.getRequestPrice(ctx, "PUT", storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get request price: %w", err)
	}

	estimate.RequestCost = (float64(fileCount) / 1000) * requestPrice
	estimate.Breakdown.RequestBreakdown["PUT"] = estimate.RequestCost

	// Calculate INTELLIGENT_TIERING monitoring fee if applicable
	if storageClass == config.StorageClassIntelligentTiering {
		monitoringCost := (float64(fileCount) / 1000) * IntelligentTieringMonitoringFee
		estimate.Breakdown.ServiceBreakdown["monitoring"] = monitoringCost
		estimate.TotalCost += monitoringCost
	}

	// Data transfer (uploads are free)
	estimate.DataTransferCost = 0
	estimate.Breakdown.TransferBreakdown["upload"] = 0

	// Calculate total
	estimate.TotalCost = estimate.StorageCost + estimate.RequestCost + estimate.DataTransferCost

	return estimate, nil
}

// estimateChunkedUploadCost calculates cost for uploading with CargoShip chunking
func (pm *PricingManager) estimateChunkedUploadCost(
	ctx context.Context,
	chunkCount int,
	totalSizeGB float64,
	storageClass config.StorageClass,
	region string,
) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Currency:    pm.config.Currency,
		EstimatedAt: time.Now(),
		Breakdown: CostBreakdown{
			StorageBreakdown:  make(map[string]float64),
			RequestBreakdown:  make(map[string]float64),
			TransferBreakdown: make(map[string]float64),
			ServiceBreakdown:  make(map[string]float64),
			RegionBreakdown:   make(map[string]float64),
		},
	}

	// Calculate storage cost (no minimum size penalty for large chunks)
	storagePrice, err := pm.getStoragePrice(ctx, storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage price: %w", err)
	}

	estimate.StorageCost = totalSizeGB * storagePrice
	estimate.Breakdown.StorageBreakdown[string(storageClass)] = estimate.StorageCost
	estimate.Breakdown.StorageBreakdown["minimum_size_penalty"] = 0 // No penalty with chunking

	// Calculate request cost (one PUT per chunk)
	requestPrice, err := pm.getRequestPrice(ctx, "PUT", storageClass, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get request price: %w", err)
	}

	estimate.RequestCost = (float64(chunkCount) / 1000) * requestPrice
	estimate.Breakdown.RequestBreakdown["PUT"] = estimate.RequestCost

	// Calculate INTELLIGENT_TIERING monitoring fee if applicable (for chunks, not files)
	if storageClass == config.StorageClassIntelligentTiering {
		monitoringCost := (float64(chunkCount) / 1000) * IntelligentTieringMonitoringFee
		estimate.Breakdown.ServiceBreakdown["monitoring"] = monitoringCost
		estimate.TotalCost += monitoringCost
	} else {
		estimate.Breakdown.ServiceBreakdown["monitoring"] = 0
	}

	// Data transfer (uploads are free)
	estimate.DataTransferCost = 0
	estimate.Breakdown.TransferBreakdown["upload"] = 0

	// Calculate total
	estimate.TotalCost = estimate.StorageCost + estimate.RequestCost + estimate.DataTransferCost

	return estimate, nil
}

// calculateChargedSize calculates actual charged size considering minimum object size
func (pm *PricingManager) calculateChargedSize(fileCount int, totalSizeGB float64, storageClass config.StorageClass, avgFileSizeKB float64) float64 {
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

// generateRecommendations generates cost optimization recommendations
func (pm *PricingManager) generateRecommendations(stats FileStatistics, storageClass config.StorageClass, savings SavingsBreakdown) []string {
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
