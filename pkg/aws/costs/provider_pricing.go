// Package costs provides S3-compatible storage provider pricing models
// Issue #170 Phase 3: Support for Wasabi, Backblaze B2, MinIO, etc.
package costs

import (
	"fmt"
)

// StorageProvider represents an S3-compatible storage provider
type StorageProvider string

const (
	ProviderAWS         StorageProvider = "aws"
	ProviderWasabi      StorageProvider = "wasabi"
	ProviderBackblazeB2 StorageProvider = "b2"
	ProviderMinIO       StorageProvider = "minio"
	ProviderCustom      StorageProvider = "custom"
)

// ProviderPricing defines pricing structure for a storage provider
type ProviderPricing struct {
	Provider              StorageProvider
	Name                  string
	StoragePricePerTB     float64 // Monthly storage cost per TB
	HasMonitoringFees     bool    // Whether provider charges monitoring fees
	MonitoringFeePerK     float64 // Monitoring fee per 1000 objects (if applicable)
	HasMinimumSizePenalty bool    // Whether provider has minimum object size penalties
	EgressPricePerGB      float64 // Data transfer out price per GB
	GetRequestCostPerK    float64 // GET request cost per 1000 requests
	PutRequestCostPerK    float64 // PUT request cost per 1000 requests
	ListRequestCostPerK   float64 // LIST request cost per 1000 requests
}

// GetProviderPricing returns pricing information for a storage provider
func GetProviderPricing(provider StorageProvider, region string) *ProviderPricing {
	switch provider {
	case ProviderWasabi:
		return &ProviderPricing{
			Provider:              ProviderWasabi,
			Name:                  "Wasabi",
			StoragePricePerTB:     5.99,
			HasMonitoringFees:     false,
			MonitoringFeePerK:     0,
			HasMinimumSizePenalty: false,
			EgressPricePerGB:      0, // Free egress
			GetRequestCostPerK:    0, // Included
			PutRequestCostPerK:    0, // Included
			ListRequestCostPerK:   0, // Included
		}

	case ProviderBackblazeB2:
		return &ProviderPricing{
			Provider:              ProviderBackblazeB2,
			Name:                  "Backblaze B2",
			StoragePricePerTB:     5.00,
			HasMonitoringFees:     false,
			MonitoringFeePerK:     0,
			HasMinimumSizePenalty: false,
			EgressPricePerGB:      0.01,  // $0.01/GB egress
			GetRequestCostPerK:    0.004, // $0.004 per 1K (Class B transactions)
			PutRequestCostPerK:    0.004, // $0.004 per 1K (Class C transactions)
			ListRequestCostPerK:   0.004, // $0.004 per 1K (Class B transactions)
		}

	case ProviderMinIO:
		return &ProviderPricing{
			Provider:              ProviderMinIO,
			Name:                  "MinIO (Self-Hosted)",
			StoragePricePerTB:     0, // Self-hosted, no cloud storage costs
			HasMonitoringFees:     false,
			MonitoringFeePerK:     0,
			HasMinimumSizePenalty: false,
			EgressPricePerGB:      0, // Self-hosted
			GetRequestCostPerK:    0, // Self-hosted
			PutRequestCostPerK:    0, // Self-hosted
			ListRequestCostPerK:   0, // Self-hosted
		}

	case ProviderAWS:
		fallthrough
	default:
		// AWS pricing is handled by the existing Calculator
		return &ProviderPricing{
			Provider:              ProviderAWS,
			Name:                  "AWS S3",
			StoragePricePerTB:     0, // Varies by region and tier
			HasMonitoringFees:     true,
			MonitoringFeePerK:     0.0025,
			HasMinimumSizePenalty: true,
			EgressPricePerGB:      0.09, // First 10 TB/month
			GetRequestCostPerK:    0.0004,
			PutRequestCostPerK:    0.005,
			ListRequestCostPerK:   0.005,
		}
	}
}

// CalculateProviderStorageCost calculates storage cost for a provider
func (p *ProviderPricing) CalculateProviderStorageCost(sizeGB float64) float64 {
	if p.Provider == ProviderAWS {
		// AWS pricing is handled by Calculator (region-specific, tier-specific)
		return 0
	}

	sizeTB := sizeGB / 1024
	return sizeTB * p.StoragePricePerTB
}

// CalculateProviderRequestCost calculates request costs for a provider
func (p *ProviderPricing) CalculateProviderRequestCost(getRequests, putRequests, listRequests int) float64 {
	getCost := (float64(getRequests) / 1000.0) * p.GetRequestCostPerK
	putCost := (float64(putRequests) / 1000.0) * p.PutRequestCostPerK
	listCost := (float64(listRequests) / 1000.0) * p.ListRequestCostPerK
	return getCost + putCost + listCost
}

// CalculateProviderMonitoringCost calculates monitoring fees for a provider
func (p *ProviderPricing) CalculateProviderMonitoringCost(objectCount int64) float64 {
	if !p.HasMonitoringFees {
		return 0
	}
	return (float64(objectCount) / 1000.0) * p.MonitoringFeePerK
}

// GetProviderBenefitMessage returns a message about CargoShip benefits for this provider
func (p *ProviderPricing) GetProviderBenefitMessage() string {
	switch p.Provider {
	case ProviderWasabi:
		return "Wasabi has no monitoring fees, but chunking still reduces object count for better management and faster listing"
	case ProviderBackblazeB2:
		return "Backblaze B2 has no monitoring fees, but chunking reduces request costs and simplifies object management"
	case ProviderMinIO:
		return "MinIO has no cloud costs, but chunking provides organizational benefits (fewer objects to manage, faster operations)"
	default:
		return "CargoShip chunking reduces costs through monitoring fee elimination and minimum size penalty removal"
	}
}

// ValidateProvider checks if a provider string is valid
func ValidateProvider(provider string) (StorageProvider, error) {
	p := StorageProvider(provider)
	switch p {
	case ProviderAWS, ProviderWasabi, ProviderBackblazeB2, ProviderMinIO, ProviderCustom:
		return p, nil
	default:
		return "", fmt.Errorf("unsupported provider: %s (supported: aws, wasabi, b2, minio, custom)", provider)
	}
}
