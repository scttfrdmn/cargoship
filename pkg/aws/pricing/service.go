// Package pricing provides real-time AWS pricing integration for CargoShip
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// Service provides real-time AWS pricing information
type Service struct {
	client       *pricing.Client
	cache        map[string]*PriceData
	cacheMutex   sync.RWMutex
	cacheExpiry  time.Duration
	lastUpdate   time.Time
}

// PriceData contains pricing information for AWS services
type PriceData struct {
	StoragePrice  map[config.StorageClass]float64 `json:"storage_price"`
	TransferPrice float64                         `json:"transfer_price"`
	RequestPrice  map[config.StorageClass]float64 `json:"request_price"`
	Region        string                          `json:"region"`
	UpdatedAt     time.Time                       `json:"updated_at"`
}

// NewService creates a new pricing service
func NewService(client *pricing.Client) *Service {
	return &Service{
		client:      client,
		cache:       make(map[string]*PriceData),
		cacheExpiry: 24 * time.Hour, // Cache for 24 hours
	}
}

// GetPricing retrieves current pricing data for a region with caching
func (s *Service) GetPricing(ctx context.Context, region string) (*PriceData, error) {
	s.cacheMutex.RLock()
	if cached, exists := s.cache[region]; exists {
		if time.Since(cached.UpdatedAt) < s.cacheExpiry {
			s.cacheMutex.RUnlock()
			return cached, nil
		}
	}
	s.cacheMutex.RUnlock()

	// Fetch fresh pricing data
	priceData, err := s.fetchPricingData(ctx, region)
	if err != nil {
		// Return cached data if available and fetch failed
		s.cacheMutex.RLock()
		if cached, exists := s.cache[region]; exists {
			s.cacheMutex.RUnlock()
			return cached, fmt.Errorf("using cached pricing due to fetch error: %w", err)
		}
		s.cacheMutex.RUnlock()
		return nil, err
	}

	// Update cache
	s.cacheMutex.Lock()
	s.cache[region] = priceData
	s.cacheMutex.Unlock()

	return priceData, nil
}

// fetchPricingData retrieves real-time pricing from AWS Pricing API
func (s *Service) fetchPricingData(ctx context.Context, region string) (*PriceData, error) {
	priceData := &PriceData{
		StoragePrice:  make(map[config.StorageClass]float64),
		RequestPrice:  make(map[config.StorageClass]float64),
		Region:        region,
		UpdatedAt:     time.Now(),
	}

	// Fetch S3 storage pricing
	if err := s.fetchS3StoragePricing(ctx, region, priceData); err != nil {
		return nil, fmt.Errorf("failed to fetch S3 storage pricing: %w", err)
	}

	// Fetch data transfer pricing
	if err := s.fetchDataTransferPricing(ctx, region, priceData); err != nil {
		return nil, fmt.Errorf("failed to fetch data transfer pricing: %w", err)
	}

	// Fetch request pricing
	if err := s.fetchRequestPricing(ctx, region, priceData); err != nil {
		return nil, fmt.Errorf("failed to fetch request pricing: %w", err)
	}

	return priceData, nil
}

// fetchS3StoragePricing retrieves S3 storage pricing
func (s *Service) fetchS3StoragePricing(ctx context.Context, region string, priceData *PriceData) error {
	// Query S3 storage pricing
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonS3"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(s.getLocationFromRegion(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Storage"),
			},
		},
		MaxResults: aws.Int32(100),
	}

	result, err := s.client.GetProducts(ctx, input)
	if err != nil {
		return err
	}

	// Parse pricing data
	for _, product := range result.PriceList {
		if err := s.parseS3StorageProduct(product, priceData); err != nil {
			continue // Skip products we can't parse
		}
	}

	// Set fallback pricing if no data found
	if len(priceData.StoragePrice) == 0 {
		s.setFallbackStoragePricing(priceData, region)
	}

	return nil
}

// fetchDataTransferPricing retrieves data transfer pricing
func (s *Service) fetchDataTransferPricing(ctx context.Context, region string, priceData *PriceData) error {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonS3"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(s.getLocationFromRegion(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Data Transfer"),
			},
		},
		MaxResults: aws.Int32(50),
	}

	result, err := s.client.GetProducts(ctx, input)
	if err != nil {
		return err
	}

	// Parse transfer pricing (simplified - take first valid price)
	for _, product := range result.PriceList {
		if price := s.parseDataTransferProduct(product); price > 0 {
			priceData.TransferPrice = price
			break
		}
	}

	// Set fallback if no data found
	if priceData.TransferPrice == 0 {
		priceData.TransferPrice = 0.09 // Standard fallback
	}

	return nil
}

// fetchRequestPricing retrieves API request pricing
func (s *Service) fetchRequestPricing(ctx context.Context, region string, priceData *PriceData) error {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonS3"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(s.getLocationFromRegion(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("API Request"),
			},
		},
		MaxResults: aws.Int32(100),
	}

	result, err := s.client.GetProducts(ctx, input)
	if err != nil {
		return err
	}

	// Parse request pricing
	for _, product := range result.PriceList {
		if err := s.parseS3RequestProduct(product, priceData); err != nil {
			continue
		}
	}

	// Set fallback pricing if no data found
	if len(priceData.RequestPrice) == 0 {
		s.setFallbackRequestPricing(priceData)
	}

	return nil
}

// parseS3StorageProduct parses a storage product from pricing API
func (s *Service) parseS3StorageProduct(product string, priceData *PriceData) error {
	var productData map[string]interface{}
	if err := json.Unmarshal([]byte(product), &productData); err != nil {
		return err
	}

	// Extract storage class and pricing
	attributes, ok := productData["product"].(map[string]interface{})["attributes"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid product structure")
	}

	storageClass := s.extractStorageClass(attributes)
	if storageClass == "" {
		return fmt.Errorf("unknown storage class")
	}

	// Extract price from terms
	price := s.extractPriceFromTerms(productData)
	if price > 0 {
		priceData.StoragePrice[config.StorageClass(storageClass)] = price
	}

	return nil
}

// parseDataTransferProduct extracts data transfer pricing
func (s *Service) parseDataTransferProduct(product string) float64 {
	var productData map[string]interface{}
	if err := json.Unmarshal([]byte(product), &productData); err != nil {
		return 0
	}

	return s.extractPriceFromTerms(productData)
}

// parseS3RequestProduct parses request pricing
func (s *Service) parseS3RequestProduct(product string, priceData *PriceData) error {
	var productData map[string]interface{}
	if err := json.Unmarshal([]byte(product), &productData); err != nil {
		return err
	}

	// Extract request type and pricing
	attributes, ok := productData["product"].(map[string]interface{})["attributes"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid product structure")
	}

	storageClass := s.extractStorageClassFromRequest(attributes)
	if storageClass == "" {
		return fmt.Errorf("unknown request type")
	}

	price := s.extractPriceFromTerms(productData)
	if price > 0 {
		priceData.RequestPrice[config.StorageClass(storageClass)] = price
	}

	return nil
}

// extractStorageClass maps AWS storage class names to our types
func (s *Service) extractStorageClass(attributes map[string]interface{}) string {
	storageClass, _ := attributes["storageClass"].(string)
	
	switch storageClass {
	case "General Purpose":
		return string(config.StorageClassStandard)
	case "Infrequent Access":
		return string(config.StorageClassStandardIA)
	case "One Zone - Infrequent Access":
		return string(config.StorageClassOneZoneIA)
	case "Archive":
		return string(config.StorageClassGlacier)
	case "Deep Archive":
		return string(config.StorageClassDeepArchive)
	case "Intelligent-Tiering":
		return string(config.StorageClassIntelligentTiering)
	default:
		return ""
	}
}

// extractStorageClassFromRequest maps request types to storage classes
func (s *Service) extractStorageClassFromRequest(attributes map[string]interface{}) string {
	requestType, _ := attributes["requestType"].(string)
	
	if strings.Contains(requestType, "PUT") {
		storageClass, _ := attributes["storageClass"].(string)
		return s.extractStorageClass(map[string]interface{}{"storageClass": storageClass})
	}
	
	return string(config.StorageClassStandard) // Default for most requests
}

// extractPriceFromTerms extracts the actual price from pricing terms
func (s *Service) extractPriceFromTerms(productData map[string]interface{}) float64 {
	terms, ok := productData["terms"].(map[string]interface{})
	if !ok {
		return 0
	}

	onDemand, ok := terms["OnDemand"].(map[string]interface{})
	if !ok {
		return 0
	}

	// Navigate through the nested structure to find the price
	for _, termData := range onDemand {
		termMap, ok := termData.(map[string]interface{})
		if !ok {
			continue
		}

		priceDimensions, ok := termMap["priceDimensions"].(map[string]interface{})
		if !ok {
			continue
		}

		for _, dimension := range priceDimensions {
			dimMap, ok := dimension.(map[string]interface{})
			if !ok {
				continue
			}

			pricePerUnit, ok := dimMap["pricePerUnit"].(map[string]interface{})
			if !ok {
				continue
			}

			usdPrice, ok := pricePerUnit["USD"].(string)
			if !ok {
				continue
			}

			var price float64
			if _, err := fmt.Sscanf(usdPrice, "%f", &price); err == nil {
				return price
			}
		}
	}

	return 0
}

// getLocationFromRegion maps AWS region codes to pricing API location names
func (s *Service) getLocationFromRegion(region string) string {
	locationMap := map[string]string{
		"us-east-1":      "US East (N. Virginia)",
		"us-east-2":      "US East (Ohio)",
		"us-west-1":      "US West (N. California)",
		"us-west-2":      "US West (Oregon)",
		"eu-west-1":      "Europe (Ireland)",
		"eu-west-2":      "Europe (London)",
		"eu-west-3":      "Europe (Paris)",
		"eu-central-1":   "Europe (Frankfurt)",
		"ap-northeast-1": "Asia Pacific (Tokyo)",
		"ap-southeast-1": "Asia Pacific (Singapore)",
		"ap-southeast-2": "Asia Pacific (Sydney)",
	}

	if location, exists := locationMap[region]; exists {
		return location
	}

	return "US East (N. Virginia)" // Default fallback
}

// setFallbackStoragePricing sets fallback pricing when API data is unavailable
func (s *Service) setFallbackStoragePricing(priceData *PriceData, region string) {
	// Regional pricing multipliers (simplified)
	multiplier := 1.0
	if strings.HasPrefix(region, "eu-") || strings.HasPrefix(region, "ap-") {
		multiplier = 1.1 // 10% higher for most non-US regions
	}

	priceData.StoragePrice = map[config.StorageClass]float64{
		config.StorageClassStandard:           0.023 * multiplier,
		config.StorageClassStandardIA:         0.0125 * multiplier,
		config.StorageClassOneZoneIA:          0.01 * multiplier,
		config.StorageClassIntelligentTiering: 0.0225 * multiplier,
		config.StorageClassGlacier:            0.004 * multiplier,
		config.StorageClassDeepArchive:        0.00099 * multiplier,
	}
}

// setFallbackRequestPricing sets fallback request pricing
func (s *Service) setFallbackRequestPricing(priceData *PriceData) {
	priceData.RequestPrice = map[config.StorageClass]float64{
		config.StorageClassStandard:           0.0005,
		config.StorageClassStandardIA:         0.001,
		config.StorageClassOneZoneIA:          0.001,
		config.StorageClassIntelligentTiering: 0.0005,
		config.StorageClassGlacier:            0.003,
		config.StorageClassDeepArchive:        0.005,
	}
}

// InvalidateCache clears the pricing cache for a region
func (s *Service) InvalidateCache(region string) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	delete(s.cache, region)
}

// InvalidateAllCache clears all cached pricing data
func (s *Service) InvalidateAllCache() {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.cache = make(map[string]*PriceData)
}