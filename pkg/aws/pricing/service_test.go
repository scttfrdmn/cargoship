package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// MockPricingClient implements the PricingClient interface for testing
type MockPricingClient struct {
	getProductsFunc func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
	returnError     error
	callHistory     []MockCall
}

type MockCall struct {
	Method      string
	ServiceCode string
	Filters     []types.Filter
}

func (m *MockPricingClient) GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	call := MockCall{
		Method:      "GetProducts",
		ServiceCode: aws.ToString(params.ServiceCode),
		Filters:     params.Filters,
	}
	m.callHistory = append(m.callHistory, call)

	if m.returnError != nil {
		return nil, m.returnError
	}

	if m.getProductsFunc != nil {
		return m.getProductsFunc(ctx, params, optFns...)
	}

	// Default mock response
	return &pricing.GetProductsOutput{
		PriceList: []string{
			createMockS3StorageProduct("General Purpose", "0.023"),
			createMockS3StorageProduct("Infrequent Access", "0.0125"),
			createMockDataTransferProduct("0.09"),
			createMockRequestProduct("PUT", "General Purpose", "0.0005"),
		},
	}, nil
}

// createMockS3StorageProduct creates a realistic mock S3 storage product JSON
func createMockS3StorageProduct(storageClass, price string) string {
	product := map[string]interface{}{
		"product": map[string]interface{}{
			"attributes": map[string]interface{}{
				"storageClass":  storageClass,
				"productFamily": "Storage",
				"location":      "US East (N. Virginia)",
			},
		},
		"terms": map[string]interface{}{
			"OnDemand": map[string]interface{}{
				"term1": map[string]interface{}{
					"priceDimensions": map[string]interface{}{
						"dim1": map[string]interface{}{
							"pricePerUnit": map[string]interface{}{
								"USD": price,
							},
						},
					},
				},
			},
		},
	}
	
	jsonData, _ := json.Marshal(product)
	return string(jsonData)
}

// createMockDataTransferProduct creates a mock data transfer product
func createMockDataTransferProduct(price string) string {
	product := map[string]interface{}{
		"product": map[string]interface{}{
			"attributes": map[string]interface{}{
				"productFamily": "Data Transfer",
				"location":      "US East (N. Virginia)",
			},
		},
		"terms": map[string]interface{}{
			"OnDemand": map[string]interface{}{
				"term1": map[string]interface{}{
					"priceDimensions": map[string]interface{}{
						"dim1": map[string]interface{}{
							"pricePerUnit": map[string]interface{}{
								"USD": price,
							},
						},
					},
				},
			},
		},
	}
	
	jsonData, _ := json.Marshal(product)
	return string(jsonData)
}

// createMockRequestProduct creates a mock API request product
func createMockRequestProduct(requestType, storageClass, price string) string {
	product := map[string]interface{}{
		"product": map[string]interface{}{
			"attributes": map[string]interface{}{
				"requestType":   requestType,
				"storageClass":  storageClass,
				"productFamily": "API Request",
				"location":      "US East (N. Virginia)",
			},
		},
		"terms": map[string]interface{}{
			"OnDemand": map[string]interface{}{
				"term1": map[string]interface{}{
					"priceDimensions": map[string]interface{}{
						"dim1": map[string]interface{}{
							"pricePerUnit": map[string]interface{}{
								"USD": price,
							},
						},
					},
				},
			},
		},
	}
	
	jsonData, _ := json.Marshal(product)
	return string(jsonData)
}

func TestNewService(t *testing.T) {
	mockClient := &MockPricingClient{}
	service := NewService(mockClient)

	if service.client != mockClient {
		t.Error("NewService() should set client correctly")
	}

	if service.cache == nil {
		t.Error("NewService() cache should be initialized")
	}

	if service.cacheExpiry != 24*time.Hour {
		t.Errorf("NewService() cacheExpiry = %v, want %v", service.cacheExpiry, 24*time.Hour)
	}
}

func TestService_GetPricing_CacheHit(t *testing.T) {
	mockClient := &MockPricingClient{}
	service := NewService(mockClient)
	
	// Pre-populate cache with fresh data
	cachedData := &PriceData{
		StoragePrice: map[config.StorageClass]float64{
			config.StorageClassStandard: 0.023,
		},
		TransferPrice: 0.09,
		RequestPrice: map[config.StorageClass]float64{
			config.StorageClassStandard: 0.0005,
		},
		Region:    "us-east-1",
		UpdatedAt: time.Now(),
	}
	service.cache["us-east-1"] = cachedData

	// Test cache hit
	result, err := service.GetPricing(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("GetPricing() error = %v, want nil", err)
	}

	if result != cachedData {
		t.Error("GetPricing() should return cached data")
	}

	// Verify no API calls were made
	if len(mockClient.callHistory) > 0 {
		t.Error("GetPricing() should not call API when cache is fresh")
	}
}

func TestService_GetPricing_CacheExpired(t *testing.T) {
	mockClient := &MockPricingClient{}
	service := NewService(mockClient)

	// Pre-populate cache with expired data
	expiredData := &PriceData{
		StoragePrice: map[config.StorageClass]float64{
			config.StorageClassStandard: 0.023,
		},
		Region:    "us-east-1",
		UpdatedAt: time.Now().Add(-25 * time.Hour), // Expired (older than 24h)
	}
	service.cache["us-east-1"] = expiredData

	// Test cache miss due to expiration
	result, err := service.GetPricing(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("GetPricing() error = %v, want nil", err)
	}

	// Should get fresh data, not cached
	if result == expiredData {
		t.Error("GetPricing() should fetch fresh data when cache is expired")
	}

	// Verify API calls were made
	if len(mockClient.callHistory) == 0 {
		t.Error("GetPricing() should call API when cache is expired")
	}
}

func TestService_GetPricing_FetchError_FallbackToCache(t *testing.T) {
	mockClient := &MockPricingClient{
		returnError: fmt.Errorf("API error"),
	}
	service := NewService(mockClient)
	
	// Pre-populate cache with expired data
	cachedData := &PriceData{
		StoragePrice: map[config.StorageClass]float64{
			config.StorageClassStandard: 0.023,
		},
		Region:    "us-east-1",
		UpdatedAt: time.Now().Add(-25 * time.Hour), // Expired
	}
	service.cache["us-east-1"] = cachedData

	result, err := service.GetPricing(context.Background(), "us-east-1")
	if err == nil {
		t.Error("GetPricing() should return error when fetch fails")
	}

	if result != cachedData {
		t.Error("GetPricing() should return cached data when fetch fails")
	}

	if !strings.Contains(err.Error(), "using cached pricing due to fetch error") {
		t.Errorf("GetPricing() error should indicate fallback to cache, got: %v", err)
	}
}

func TestService_GetPricing_NoCache_FetchError(t *testing.T) {
	mockClient := &MockPricingClient{
		returnError: fmt.Errorf("API error"),
	}
	service := NewService(mockClient)

	result, err := service.GetPricing(context.Background(), "us-east-1")
	if err == nil {
		t.Error("GetPricing() should return error when fetch fails and no cache")
	}

	if result != nil {
		t.Error("GetPricing() should return nil when fetch fails and no cache")
	}
}

func TestService_fetchPricingData_Success(t *testing.T) {
	mockClient := &MockPricingClient{}
	service := NewService(mockClient)

	result, err := service.fetchPricingData(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("fetchPricingData() error = %v, want nil", err)
	}

	if result.Region != "us-east-1" {
		t.Errorf("fetchPricingData() region = %v, want us-east-1", result.Region)
	}

	// Should have called API for storage, transfer, and request pricing
	if len(mockClient.callHistory) < 3 {
		t.Errorf("fetchPricingData() should make 3 API calls, got %d", len(mockClient.callHistory))
	}

	// Verify service codes
	serviceCodes := make(map[string]int)
	for _, call := range mockClient.callHistory {
		serviceCodes[call.ServiceCode]++
	}
	
	if serviceCodes["AmazonS3"] != 3 {
		t.Errorf("fetchPricingData() should call AmazonS3 service 3 times, got %d", serviceCodes["AmazonS3"])
	}

	if time.Since(result.UpdatedAt) > time.Minute {
		t.Error("fetchPricingData() should set recent UpdatedAt")
	}
}

func TestService_extractStorageClass(t *testing.T) {
	service := NewService(&MockPricingClient{})

	tests := []struct {
		name        string
		attributes  map[string]interface{}
		expected    string
	}{
		{
			name:       "General Purpose",
			attributes: map[string]interface{}{"storageClass": "General Purpose"},
			expected:   string(config.StorageClassStandard),
		},
		{
			name:       "Infrequent Access",
			attributes: map[string]interface{}{"storageClass": "Infrequent Access"},
			expected:   string(config.StorageClassStandardIA),
		},
		{
			name:       "One Zone - Infrequent Access",
			attributes: map[string]interface{}{"storageClass": "One Zone - Infrequent Access"},
			expected:   string(config.StorageClassOneZoneIA),
		},
		{
			name:       "Archive",
			attributes: map[string]interface{}{"storageClass": "Archive"},
			expected:   string(config.StorageClassGlacier),
		},
		{
			name:       "Deep Archive",
			attributes: map[string]interface{}{"storageClass": "Deep Archive"},
			expected:   string(config.StorageClassDeepArchive),
		},
		{
			name:       "Intelligent-Tiering",
			attributes: map[string]interface{}{"storageClass": "Intelligent-Tiering"},
			expected:   string(config.StorageClassIntelligentTiering),
		},
		{
			name:       "Unknown storage class",
			attributes: map[string]interface{}{"storageClass": "Unknown Class"},
			expected:   "",
		},
		{
			name:       "Missing storage class",
			attributes: map[string]interface{}{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.extractStorageClass(tt.attributes)
			if result != tt.expected {
				t.Errorf("extractStorageClass() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestService_extractStorageClassFromRequest(t *testing.T) {
	service := NewService(&MockPricingClient{})

	tests := []struct {
		name        string
		attributes  map[string]interface{}
		expected    string
	}{
		{
			name: "PUT request with General Purpose",
			attributes: map[string]interface{}{
				"requestType":  "PUT",
				"storageClass": "General Purpose",
			},
			expected: string(config.StorageClassStandard),
		},
		{
			name: "PUT request with Glacier",
			attributes: map[string]interface{}{
				"requestType":  "PUT",
				"storageClass": "Archive",
			},
			expected: string(config.StorageClassGlacier),
		},
		{
			name: "GET request",
			attributes: map[string]interface{}{
				"requestType": "GET",
			},
			expected: string(config.StorageClassStandard),
		},
		{
			name: "LIST request",
			attributes: map[string]interface{}{
				"requestType": "LIST",
			},
			expected: string(config.StorageClassStandard),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.extractStorageClassFromRequest(tt.attributes)
			if result != tt.expected {
				t.Errorf("extractStorageClassFromRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestService_extractPriceFromTerms(t *testing.T) {
	service := NewService(&MockPricingClient{})

	tests := []struct {
		name        string
		productData map[string]interface{}
		expected    float64
	}{
		{
			name: "Valid pricing structure",
			productData: map[string]interface{}{
				"terms": map[string]interface{}{
					"OnDemand": map[string]interface{}{
						"term1": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"dim1": map[string]interface{}{
									"pricePerUnit": map[string]interface{}{
										"USD": "0.023",
									},
								},
							},
						},
					},
				},
			},
			expected: 0.023,
		},
		{
			name: "Multiple terms - use first valid",
			productData: map[string]interface{}{
				"terms": map[string]interface{}{
					"OnDemand": map[string]interface{}{
						"term1": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"dim1": map[string]interface{}{
									"pricePerUnit": map[string]interface{}{
										"USD": "0.025",
									},
								},
							},
						},
						"term2": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"dim2": map[string]interface{}{
									"pricePerUnit": map[string]interface{}{
										"USD": "0.030",
									},
								},
							},
						},
					},
				},
			},
			expected: 0.025, // Should get first valid price
		},
		{
			name: "Missing terms",
			productData: map[string]interface{}{
				"product": map[string]interface{}{},
			},
			expected: 0,
		},
		{
			name: "Missing OnDemand",
			productData: map[string]interface{}{
				"terms": map[string]interface{}{
					"Reserved": map[string]interface{}{},
				},
			},
			expected: 0,
		},
		{
			name: "Invalid price format",
			productData: map[string]interface{}{
				"terms": map[string]interface{}{
					"OnDemand": map[string]interface{}{
						"term1": map[string]interface{}{
							"priceDimensions": map[string]interface{}{
								"dim1": map[string]interface{}{
									"pricePerUnit": map[string]interface{}{
										"USD": "invalid",
									},
								},
							},
						},
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.extractPriceFromTerms(tt.productData)
			if result != tt.expected {
				t.Errorf("extractPriceFromTerms() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestService_getLocationFromRegion(t *testing.T) {
	service := NewService(&MockPricingClient{})

	tests := []struct {
		region   string
		expected string
	}{
		{"us-east-1", "US East (N. Virginia)"},
		{"us-east-2", "US East (Ohio)"},
		{"us-west-1", "US West (N. California)"},
		{"us-west-2", "US West (Oregon)"},
		{"eu-west-1", "Europe (Ireland)"},
		{"eu-west-2", "Europe (London)"},
		{"eu-west-3", "Europe (Paris)"},
		{"eu-central-1", "Europe (Frankfurt)"},
		{"ap-northeast-1", "Asia Pacific (Tokyo)"},
		{"ap-southeast-1", "Asia Pacific (Singapore)"},
		{"ap-southeast-2", "Asia Pacific (Sydney)"},
		{"unknown-region", "US East (N. Virginia)"}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			result := service.getLocationFromRegion(tt.region)
			if result != tt.expected {
				t.Errorf("getLocationFromRegion(%v) = %v, want %v", tt.region, result, tt.expected)
			}
		})
	}
}

func TestService_setFallbackStoragePricing(t *testing.T) {
	service := NewService(&MockPricingClient{})
	priceData := &PriceData{
		StoragePrice: make(map[config.StorageClass]float64),
	}

	tests := []struct {
		name             string
		region           string
		expectedMultiplier float64
	}{
		{"US region", "us-east-1", 1.0},
		{"EU region", "eu-west-1", 1.1},
		{"Asia Pacific region", "ap-southeast-1", 1.1},
		{"Unknown region", "unknown-1", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset price data
			priceData.StoragePrice = make(map[config.StorageClass]float64)
			
			service.setFallbackStoragePricing(priceData, tt.region)

			// Check that all storage classes are set
			expectedClasses := []config.StorageClass{
				config.StorageClassStandard,
				config.StorageClassStandardIA,
				config.StorageClassOneZoneIA,
				config.StorageClassIntelligentTiering,
				config.StorageClassGlacier,
				config.StorageClassDeepArchive,
			}

			for _, class := range expectedClasses {
				price, exists := priceData.StoragePrice[class]
				if !exists {
					t.Errorf("setFallbackStoragePricing() missing price for %v", class)
					continue
				}
				if price <= 0 {
					t.Errorf("setFallbackStoragePricing() invalid price %v for %v", price, class)
				}
			}

			// Verify regional pricing multiplier is applied
			standardPrice := priceData.StoragePrice[config.StorageClassStandard]
			expectedStandardPrice := 0.023 * tt.expectedMultiplier
			if standardPrice != expectedStandardPrice {
				t.Errorf("setFallbackStoragePricing() standard price = %v, want %v", standardPrice, expectedStandardPrice)
			}
		})
	}
}

func TestService_setFallbackRequestPricing(t *testing.T) {
	service := NewService(&MockPricingClient{})
	priceData := &PriceData{
		RequestPrice: make(map[config.StorageClass]float64),
	}

	service.setFallbackRequestPricing(priceData)

	// Check that all storage classes have request pricing
	expectedClasses := []config.StorageClass{
		config.StorageClassStandard,
		config.StorageClassStandardIA,
		config.StorageClassOneZoneIA,
		config.StorageClassIntelligentTiering,
		config.StorageClassGlacier,
		config.StorageClassDeepArchive,
	}

	for _, class := range expectedClasses {
		price, exists := priceData.RequestPrice[class]
		if !exists {
			t.Errorf("setFallbackRequestPricing() missing price for %v", class)
			continue
		}
		if price <= 0 {
			t.Errorf("setFallbackRequestPricing() invalid price %v for %v", price, class)
		}
	}
}

func TestService_InvalidateCache(t *testing.T) {
	service := NewService(&MockPricingClient{})
	
	// Populate cache
	service.cache["us-east-1"] = &PriceData{Region: "us-east-1"}
	service.cache["eu-west-1"] = &PriceData{Region: "eu-west-1"}

	// Invalidate one region
	service.InvalidateCache("us-east-1")

	if _, exists := service.cache["us-east-1"]; exists {
		t.Error("InvalidateCache() should remove specified region")
	}

	if _, exists := service.cache["eu-west-1"]; !exists {
		t.Error("InvalidateCache() should not affect other regions")
	}
}

func TestService_InvalidateAllCache(t *testing.T) {
	service := NewService(&MockPricingClient{})
	
	// Populate cache
	service.cache["us-east-1"] = &PriceData{Region: "us-east-1"}
	service.cache["eu-west-1"] = &PriceData{Region: "eu-west-1"}

	// Invalidate all
	service.InvalidateAllCache()

	if len(service.cache) != 0 {
		t.Errorf("InvalidateAllCache() should clear all cache, got %d items", len(service.cache))
	}
}

func TestService_parseS3StorageProduct(t *testing.T) {
	service := NewService(&MockPricingClient{})
	priceData := &PriceData{
		StoragePrice: make(map[config.StorageClass]float64),
	}

	tests := []struct {
		name        string
		product     string
		expectError bool
		expected    map[config.StorageClass]float64
	}{
		{
			name:        "Valid General Purpose storage product",
			product:     createMockS3StorageProduct("General Purpose", "0.023"),
			expectError: false,
			expected: map[config.StorageClass]float64{
				config.StorageClassStandard: 0.023,
			},
		},
		{
			name:        "Valid Glacier storage product",
			product:     createMockS3StorageProduct("Archive", "0.004"),
			expectError: false,
			expected: map[config.StorageClass]float64{
				config.StorageClassGlacier: 0.004,
			},
		},
		{
			name:        "Invalid JSON",
			product:     "invalid json",
			expectError: true,
			expected:    map[config.StorageClass]float64{},
		},
		{
			name:        "Unknown storage class",
			product:     createMockS3StorageProduct("Unknown Class", "0.001"),
			expectError: true,
			expected:    map[config.StorageClass]float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset price data
			priceData.StoragePrice = make(map[config.StorageClass]float64)
			
			err := service.parseS3StorageProduct(tt.product, priceData)
			
			if tt.expectError && err == nil {
				t.Errorf("parseS3StorageProduct() expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("parseS3StorageProduct() unexpected error: %v", err)
			}

			if !tt.expectError {
				for class, expectedPrice := range tt.expected {
					if actualPrice, exists := priceData.StoragePrice[class]; !exists || actualPrice != expectedPrice {
						t.Errorf("parseS3StorageProduct() price for %v = %v, want %v", class, actualPrice, expectedPrice)
					}
				}
			}
		})
	}
}

func TestService_parseDataTransferProduct(t *testing.T) {
	service := NewService(&MockPricingClient{})

	tests := []struct {
		name     string
		product  string
		expected float64
	}{
		{
			name:     "Valid data transfer product",
			product:  createMockDataTransferProduct("0.09"),
			expected: 0.09,
		},
		{
			name:     "Invalid JSON",
			product:  "invalid json",
			expected: 0,
		},
		{
			name:     "Missing price",
			product:  `{"product": {"attributes": {}}}`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.parseDataTransferProduct(tt.product)
			if result != tt.expected {
				t.Errorf("parseDataTransferProduct() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestService_parseS3RequestProduct(t *testing.T) {
	service := NewService(&MockPricingClient{})
	priceData := &PriceData{
		RequestPrice: make(map[config.StorageClass]float64),
	}

	tests := []struct {
		name        string
		product     string
		expectError bool
		expected    map[config.StorageClass]float64
	}{
		{
			name:        "Valid PUT request product",
			product:     createMockRequestProduct("PUT", "General Purpose", "0.0005"),
			expectError: false,
			expected: map[config.StorageClass]float64{
				config.StorageClassStandard: 0.0005,
			},
		},
		{
			name:        "Invalid JSON",
			product:     "invalid json",
			expectError: true,
			expected:    map[config.StorageClass]float64{},
		},
		{
			name:        "Unknown request type",
			product:     createMockRequestProduct("UNKNOWN", "General Purpose", "0.0005"),
			expectError: false, // Should default to standard
			expected: map[config.StorageClass]float64{
				config.StorageClassStandard: 0.0005,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset price data
			priceData.RequestPrice = make(map[config.StorageClass]float64)
			
			err := service.parseS3RequestProduct(tt.product, priceData)
			
			if tt.expectError && err == nil {
				t.Errorf("parseS3RequestProduct() expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("parseS3RequestProduct() unexpected error: %v", err)
			}

			if !tt.expectError {
				for class, expectedPrice := range tt.expected {
					if actualPrice, exists := priceData.RequestPrice[class]; !exists || actualPrice != expectedPrice {
						t.Errorf("parseS3RequestProduct() price for %v = %v, want %v", class, actualPrice, expectedPrice)
					}
				}
			}
		})
	}
}

// Edge case tests
func TestService_fetchS3StoragePricing_APIError(t *testing.T) {
	mockClient := &MockPricingClient{
		returnError: fmt.Errorf("API rate limit exceeded"),
	}
	service := NewService(mockClient)
	priceData := &PriceData{
		StoragePrice: make(map[config.StorageClass]float64),
	}

	err := service.fetchS3StoragePricing(context.Background(), "us-east-1", priceData)
	if err == nil {
		t.Error("fetchS3StoragePricing() should return error when API fails")
	}

	// Should have fallback pricing when API returns no data
	if len(priceData.StoragePrice) == 0 {
		t.Error("fetchS3StoragePricing() should set fallback pricing when API fails")
	}
}

func TestService_fetchDataTransferPricing_APIError(t *testing.T) {
	mockClient := &MockPricingClient{
		returnError: fmt.Errorf("API error"),
	}
	service := NewService(mockClient)
	priceData := &PriceData{}

	err := service.fetchDataTransferPricing(context.Background(), "us-east-1", priceData)
	if err == nil {
		t.Error("fetchDataTransferPricing() should return error when API fails")
	}

	// Should have fallback pricing
	if priceData.TransferPrice != 0.09 {
		t.Errorf("fetchDataTransferPricing() should set fallback price, got %v", priceData.TransferPrice)
	}
}

func TestService_fetchRequestPricing_APIError(t *testing.T) {
	mockClient := &MockPricingClient{
		returnError: fmt.Errorf("API error"),
	}
	service := NewService(mockClient)
	priceData := &PriceData{
		RequestPrice: make(map[config.StorageClass]float64),
	}

	err := service.fetchRequestPricing(context.Background(), "us-east-1", priceData)
	if err == nil {
		t.Error("fetchRequestPricing() should return error when API fails")
	}

	// Should have fallback pricing
	if len(priceData.RequestPrice) == 0 {
		t.Error("fetchRequestPricing() should set fallback pricing when API fails")
	}
}

// Concurrent access tests
func TestService_ConcurrentCacheAccess(t *testing.T) {
	service := NewService(&MockPricingClient{})
	
	// Test concurrent cache access
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			region := fmt.Sprintf("us-east-%d", id%2+1)
			service.cache[region] = &PriceData{
				Region:    region,
				UpdatedAt: time.Now(),
			}
			
			_, err := service.GetPricing(context.Background(), region)
			if err != nil {
				t.Errorf("Concurrent GetPricing() failed: %v", err)
			}
			
			service.InvalidateCache(region)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmark tests for performance verification
func BenchmarkService_GetPricing_CacheHit(b *testing.B) {
	service := NewService(&MockPricingClient{})
	
	// Pre-populate cache
	service.cache["us-east-1"] = &PriceData{
		StoragePrice: map[config.StorageClass]float64{
			config.StorageClassStandard: 0.023,
		},
		TransferPrice: 0.09,
		Region:        "us-east-1",
		UpdatedAt:     time.Now(),
	}

	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GetPricing(ctx, "us-east-1")
	}
}

func BenchmarkService_extractStorageClass(b *testing.B) {
	service := NewService(&MockPricingClient{})
	attributes := map[string]interface{}{
		"storageClass": "General Purpose",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = service.extractStorageClass(attributes)
	}
}