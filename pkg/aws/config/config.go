// Package config provides AWS configuration management for CargoShip
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// AWSConfig holds CargoShip-specific AWS configuration
type AWSConfig struct {
	// AWS Profile to use
	Profile string `yaml:"profile" json:"profile"`

	// AWS Region
	Region string `yaml:"region" json:"region"`

	// S3 Configuration
	S3 S3Config `yaml:"s3" json:"s3"`

	// Cost Control Configuration
	CostControl CostControlConfig `yaml:"cost_control" json:"cost_control"`
}

// S3Config holds S3-specific configuration
type S3Config struct {
	// Default bucket for uploads
	Bucket string `yaml:"bucket" json:"bucket"`

	// Default storage class
	StorageClass StorageClass `yaml:"storage_class" json:"storage_class"`

	// Multipart upload threshold (default: 100MB)
	MultipartThreshold int64 `yaml:"multipart_threshold" json:"multipart_threshold"`

	// Multipart chunk size (default: 10MB)
	MultipartChunkSize int64 `yaml:"multipart_chunk_size" json:"multipart_chunk_size"`

	// Upload concurrency (default: 8)
	Concurrency int `yaml:"concurrency" json:"concurrency"`

	// KMS Key ID for encryption
	KMSKeyID string `yaml:"kms_key_id" json:"kms_key_id"`

	// Enable transfer acceleration
	UseTransferAcceleration bool `yaml:"use_transfer_acceleration" json:"use_transfer_acceleration"`
}

// CostControlConfig holds cost management settings
type CostControlConfig struct {
	// Maximum monthly budget (USD)
	MaxMonthlyBudget float64 `yaml:"max_monthly_budget" json:"max_monthly_budget"`

	// Alert threshold (0.0-1.0, percentage of budget)
	AlertThreshold float64 `yaml:"alert_threshold" json:"alert_threshold"`

	// Enable automatic cost optimization
	AutoOptimize bool `yaml:"auto_optimize" json:"auto_optimize"`

	// Require approval for uploads over this amount
	RequireApprovalOver float64 `yaml:"require_approval_over" json:"require_approval_over"`

	// Pricing configuration
	Pricing PricingConfig `yaml:"pricing" json:"pricing"`

	// Cost reporting settings
	Reporting CostReportingConfig `yaml:"reporting" json:"reporting"`
}

// StorageClass represents S3 storage classes
type StorageClass string

const (
	StorageClassStandard           StorageClass = "STANDARD"
	StorageClassStandardIA         StorageClass = "STANDARD_IA"
	StorageClassOneZoneIA          StorageClass = "ONEZONE_IA"
	StorageClassIntelligentTiering StorageClass = "INTELLIGENT_TIERING"
	StorageClassGlacier            StorageClass = "GLACIER"
	StorageClassDeepArchive        StorageClass = "DEEP_ARCHIVE"
)

// PricingConfig holds pricing and discount configuration
type PricingConfig struct {
	// Use AWS Pricing API for current rates
	UseAWSPricingAPI bool `yaml:"use_aws_pricing_api" json:"use_aws_pricing_api"`

	// Custom pricing overrides (overrides AWS API if set)
	CustomPricing map[string]ServicePricing `yaml:"custom_pricing" json:"custom_pricing"`

	// Global discount percentage (0.0-1.0)
	GlobalDiscount float64 `yaml:"global_discount" json:"global_discount"`

	// Service-specific discounts
	ServiceDiscounts map[string]float64 `yaml:"service_discounts" json:"service_discounts"`

	// Reserved Instance discounts
	ReservedInstanceDiscounts map[string]ReservedInstanceDiscount `yaml:"reserved_instance_discounts" json:"reserved_instance_discounts"`

	// Savings Plans discounts
	SavingsPlansDiscounts map[string]SavingsPlansDiscount `yaml:"savings_plans_discounts" json:"savings_plans_discounts"`

	// Enterprise discount (for large volume customers)
	EnterpriseDiscount EnterpriseDiscountConfig `yaml:"enterprise_discount" json:"enterprise_discount"`

	// Currency for cost calculations (default: USD)
	Currency string `yaml:"currency" json:"currency"`

	// AWS Pricing API cache duration (default: 24h)
	PricingCacheDuration string `yaml:"pricing_cache_duration" json:"pricing_cache_duration"`
}

// ServicePricing holds pricing for a specific AWS service
type ServicePricing struct {
	// S3 Storage pricing per GB per month
	S3Storage map[StorageClass]float64 `yaml:"s3_storage" json:"s3_storage"`

	// S3 Request pricing
	S3Requests S3RequestPricing `yaml:"s3_requests" json:"s3_requests"`

	// Data transfer pricing
	DataTransfer DataTransferPricing `yaml:"data_transfer" json:"data_transfer"`

	// Glacier retrieval pricing
	GlacierRetrieval GlacierRetrievalPricing `yaml:"glacier_retrieval" json:"glacier_retrieval"`
}

// S3RequestPricing holds S3 request pricing
type S3RequestPricing struct {
	// PUT, COPY, POST, LIST requests per 1000
	PutRequests float64 `yaml:"put_requests" json:"put_requests"`

	// GET, SELECT requests per 1000
	GetRequests float64 `yaml:"get_requests" json:"get_requests"`

	// DELETE requests (usually free)
	DeleteRequests float64 `yaml:"delete_requests" json:"delete_requests"`
}

// DataTransferPricing holds data transfer pricing
type DataTransferPricing struct {
	// Data transfer out to internet (per GB)
	OutToInternet map[string]float64 `yaml:"out_to_internet" json:"out_to_internet"`

	// Data transfer between regions (per GB)
	BetweenRegions map[string]float64 `yaml:"between_regions" json:"between_regions"`

	// CloudFront distribution (per GB)
	CloudFront float64 `yaml:"cloudfront" json:"cloudfront"`
}

// GlacierRetrievalPricing holds Glacier retrieval pricing
type GlacierRetrievalPricing struct {
	// Expedited retrieval (per GB)
	Expedited float64 `yaml:"expedited" json:"expedited"`

	// Standard retrieval (per GB)
	Standard float64 `yaml:"standard" json:"standard"`

	// Bulk retrieval (per GB)
	Bulk float64 `yaml:"bulk" json:"bulk"`
}

// ReservedInstanceDiscount holds Reserved Instance discount configuration
type ReservedInstanceDiscount struct {
	// Discount percentage (0.0-1.0)
	Discount float64 `yaml:"discount" json:"discount"`

	// Term length (1 year, 3 years)
	Term string `yaml:"term" json:"term"`

	// Payment option (No Upfront, Partial Upfront, All Upfront)
	PaymentOption string `yaml:"payment_option" json:"payment_option"`

	// Instance types covered
	InstanceTypes []string `yaml:"instance_types" json:"instance_types"`
}

// SavingsPlansDiscount holds Savings Plans discount configuration
type SavingsPlansDiscount struct {
	// Discount percentage (0.0-1.0)
	Discount float64 `yaml:"discount" json:"discount"`

	// Commitment amount (USD per hour)
	Commitment float64 `yaml:"commitment" json:"commitment"`

	// Term length (1 year, 3 years)
	Term string `yaml:"term" json:"term"`

	// Plan type (Compute, EC2 Instance, SageMaker)
	PlanType string `yaml:"plan_type" json:"plan_type"`
}

// EnterpriseDiscountConfig holds enterprise discount configuration
type EnterpriseDiscountConfig struct {
	// Enable enterprise discounts
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Volume discount tiers
	VolumeTiers []VolumeDiscountTier `yaml:"volume_tiers" json:"volume_tiers"`

	// Annual commitment discounts
	AnnualCommitmentDiscount float64 `yaml:"annual_commitment_discount" json:"annual_commitment_discount"`

	// Custom negotiated rates
	CustomRates map[string]float64 `yaml:"custom_rates" json:"custom_rates"`
}

// VolumeDiscountTier represents volume-based discount tiers
type VolumeDiscountTier struct {
	// Minimum monthly spend to qualify
	MinimumSpend float64 `yaml:"minimum_spend" json:"minimum_spend"`

	// Discount percentage for this tier
	Discount float64 `yaml:"discount" json:"discount"`

	// Services this tier applies to
	Services []string `yaml:"services" json:"services"`
}

// CostReportingConfig holds cost reporting settings
type CostReportingConfig struct {
	// Enable cost reporting
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Report frequency (daily, weekly, monthly)
	Frequency string `yaml:"frequency" json:"frequency"`

	// Email recipients for cost reports
	EmailRecipients []string `yaml:"email_recipients" json:"email_recipients"`

	// Include detailed breakdowns
	DetailedBreakdown bool `yaml:"detailed_breakdown" json:"detailed_breakdown"`

	// Export format (json, csv, pdf)
	ExportFormat string `yaml:"export_format" json:"export_format"`

	// S3 bucket for report storage
	ReportBucket string `yaml:"report_bucket" json:"report_bucket"`
}

// DefaultAWSConfig returns a sensible default configuration
func DefaultAWSConfig() *AWSConfig {
	return &AWSConfig{
		Region: "us-east-1",
		S3: S3Config{
			StorageClass:       StorageClassIntelligentTiering,
			MultipartThreshold: 100 * 1024 * 1024, // 100MB
			MultipartChunkSize: 10 * 1024 * 1024,  // 10MB
			Concurrency:        8,
		},
		CostControl: CostControlConfig{
			MaxMonthlyBudget:    1000.0,
			AlertThreshold:      0.8,
			AutoOptimize:        true,
			RequireApprovalOver: 500.0,
			Pricing: PricingConfig{
				UseAWSPricingAPI:          true,
				Currency:                  "USD",
				PricingCacheDuration:      "24h",
				CustomPricing:             make(map[string]ServicePricing),
				ServiceDiscounts:          make(map[string]float64),
				ReservedInstanceDiscounts: make(map[string]ReservedInstanceDiscount),
				SavingsPlansDiscounts:     make(map[string]SavingsPlansDiscount),
			},
			Reporting: CostReportingConfig{
				Enabled:           false,
				Frequency:         "monthly",
				DetailedBreakdown: true,
				ExportFormat:      "json",
			},
		},
	}
}

// Validate checks the configuration for required fields and valid values
func (c *AWSConfig) Validate() error {
	if c.Region == "" {
		return fmt.Errorf("AWS region is required")
	}

	if c.S3.Bucket == "" {
		return fmt.Errorf("S3 bucket is required")
	}

	if c.S3.Concurrency < 1 {
		return fmt.Errorf("S3 concurrency must be at least 1")
	}

	if c.S3.MultipartThreshold < 5*1024*1024 {
		return fmt.Errorf("multipart threshold must be at least 5MB")
	}

	if c.CostControl.AlertThreshold < 0 || c.CostControl.AlertThreshold > 1 {
		return fmt.Errorf("alert threshold must be between 0.0 and 1.0")
	}

	return nil
}

// isLocalStackEndpoint checks if the current configuration is using LocalStack
func isLocalStackEndpoint() bool {
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")
	if endpointURL == "" {
		endpointURL = os.Getenv("LOCALSTACK_ENDPOINT")
	}

	// Check for common LocalStack endpoint patterns
	return strings.Contains(endpointURL, "localhost:4566") ||
		strings.Contains(endpointURL, "127.0.0.1:4566") ||
		strings.Contains(endpointURL, "localstack")
}

// LoadAWSConfig loads AWS configuration with CargoShip defaults
func LoadAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		if profile != "" {
			return cfg, fmt.Errorf("failed to load AWS config with profile '%s' and region '%s': %w", profile, region, err)
		}
		return cfg, fmt.Errorf("failed to load AWS config for region '%s': %w", region, err)
	}

	// Configure for LocalStack compatibility if detected
	if isLocalStackEndpoint() {
		// Create custom endpoint resolver for LocalStack
		cfg.BaseEndpoint = aws.String(getLocalStackEndpoint())

		// Note: S3 UsePathStyle must be configured on the S3 client options
		// See: IsLocalStackConfig() and CreateLocalStackS3Client() functions
	}

	return cfg, nil
}

// getLocalStackEndpoint returns the LocalStack endpoint URL
func getLocalStackEndpoint() string {
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")
	if endpointURL == "" {
		endpointURL = os.Getenv("LOCALSTACK_ENDPOINT")
	}
	if endpointURL == "" {
		endpointURL = "http://localhost:4566" // Default LocalStack endpoint
	}
	return endpointURL
}

// IsLocalStackConfig returns true if the current configuration is using LocalStack
// This function can be used by other packages to determine LocalStack usage
func IsLocalStackConfig() bool {
	return isLocalStackEndpoint()
}
