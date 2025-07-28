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
)

// PricingManager handles cost calculations with AWS Pricing API integration
type PricingManager struct {
	config       *config.PricingConfig
	pricingAPI   *pricing.Client
	logger       *slog.Logger
	cache        *PricingCache
	mu           sync.RWMutex
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
	StorageCost       float64           `json:"storage_cost"`
	RequestCost       float64           `json:"request_cost"`
	DataTransferCost  float64           `json:"data_transfer_cost"`
	TotalCost         float64           `json:"total_cost"`
	Currency          string            `json:"currency"`
	EstimatedAt       time.Time         `json:"estimated_at"`
	Discounts         DiscountBreakdown `json:"discounts"`
	Breakdown         CostBreakdown     `json:"breakdown"`
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
	StorageBreakdown     map[string]float64 `json:"storage_breakdown"`
	RequestBreakdown     map[string]float64 `json:"request_breakdown"`
	TransferBreakdown    map[string]float64 `json:"transfer_breakdown"`
	ServiceBreakdown     map[string]float64 `json:"service_breakdown"`
	RegionBreakdown      map[string]float64 `json:"region_breakdown"`
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
		config:     cfg,
		logger:     logger.With("component", "pricing-manager"),
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
	
	// Use AWS Pricing API or fallback
	if pm.config.UseAWSPricingAPI && pm.pricingAPI != nil {
		// AWS Pricing API implementation would go here
		// For now, use fallback prices
		price = pm.getFallbackRequestPrice(requestType)
	} else {
		price = pm.getFallbackRequestPrice(requestType)
	}
	
	pm.setCachedPrice(cacheKey, price, "fallback")
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
func (pm *PricingManager) getAWSStoragePrice(ctx context.Context, storageClass config.StorageClass, region string) (float64, error) {
	// This would implement actual AWS Pricing API calls
	// For now, return fallback prices
	return pm.getFallbackStoragePrice(storageClass), nil
}

// Fallback pricing methods
func (pm *PricingManager) getFallbackStoragePrice(storageClass config.StorageClass) float64 {
	// These are approximate AWS S3 prices as of 2024 (USD per GB per month)
	switch storageClass {
	case config.StorageClassStandard:
		return 0.023
	case config.StorageClassStandardIA:
		return 0.0125
	case config.StorageClassOneZoneIA:
		return 0.01
	case config.StorageClassIntelligentTiering:
		return 0.023 // Same as Standard + monitoring
	case config.StorageClassGlacier:
		return 0.004
	case config.StorageClassDeepArchive:
		return 0.00099
	default:
		return 0.023
	}
}

func (pm *PricingManager) getFallbackRequestPrice(requestType string) float64 {
	// Approximate AWS S3 request prices (USD per 1000 requests)
	switch strings.ToUpper(requestType) {
	case "PUT", "POST", "COPY", "LIST":
		return 0.0005
	case "GET", "SELECT":
		return 0.0004
	case "DELETE":
		return 0.0
	default:
		return 0.0005
	}
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