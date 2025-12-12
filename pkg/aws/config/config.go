// Package config provides AWS configuration management for CargoShip
package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

	// HTTP transport configuration for network tuning (optional)
	HTTPTransport *HTTPTransportConfig `yaml:"http_transport,omitempty" json:"http_transport,omitempty"`
}

// CostControlConfig holds cost management settings
type CostControlConfig struct {
	// DEPRECATED: Maximum monthly budget (USD) - Use BudgetPeriods instead for flexible periods
	// Kept for backward compatibility. If set, creates a default monthly budget period.
	MaxMonthlyBudget float64 `yaml:"max_monthly_budget,omitempty" json:"max_monthly_budget,omitempty"`

	// DEPRECATED: Alert threshold (0.0-1.0, percentage of budget) - Use BudgetPeriods instead
	// Kept for backward compatibility. Applied to default monthly budget if BudgetPeriods not set.
	AlertThreshold float64 `yaml:"alert_threshold,omitempty" json:"alert_threshold,omitempty"`

	// Budget periods with flexible time ranges (daily, weekly, monthly, quarterly, yearly, custom, fiscal_year, grant)
	// If not set, falls back to MaxMonthlyBudget for backward compatibility
	BudgetPeriods []BudgetPeriod `yaml:"budget_periods,omitempty" json:"budget_periods,omitempty"`

	// Active budget period index (defaults to 0 if multiple periods defined)
	ActiveBudgetPeriodIndex int `yaml:"active_budget_period_index,omitempty" json:"active_budget_period_index,omitempty"`

	// Project-specific budgets (keyed by project ID / manifest upload ID)
	// If not set, projects use the global budget period
	ProjectBudgets map[string]ProjectBudget `yaml:"project_budgets,omitempty" json:"project_budgets,omitempty"`

	// Enable automatic cost optimization
	AutoOptimize bool `yaml:"auto_optimize" json:"auto_optimize"`

	// Require approval for uploads over this amount
	RequireApprovalOver float64 `yaml:"require_approval_over" json:"require_approval_over"`

	// Pricing configuration
	Pricing PricingConfig `yaml:"pricing" json:"pricing"`

	// Cost reporting settings
	Reporting CostReportingConfig `yaml:"reporting" json:"reporting"`
}

// BudgetPeriodType represents the type of budget period
type BudgetPeriodType string

const (
	// BudgetPeriodDaily represents a daily budget period
	BudgetPeriodDaily BudgetPeriodType = "daily"

	// BudgetPeriodWeekly represents a weekly budget period
	BudgetPeriodWeekly BudgetPeriodType = "weekly"

	// BudgetPeriodMonthly represents a monthly budget period (calendar month)
	BudgetPeriodMonthly BudgetPeriodType = "monthly"

	// BudgetPeriodQuarterly represents a quarterly budget period (3 calendar months)
	BudgetPeriodQuarterly BudgetPeriodType = "quarterly"

	// BudgetPeriodYearly represents a yearly budget period (calendar year)
	BudgetPeriodYearly BudgetPeriodType = "yearly"

	// BudgetPeriodCustom represents a custom date range budget period
	BudgetPeriodCustom BudgetPeriodType = "custom"

	// BudgetPeriodFiscalYear represents a fiscal year budget period (non-calendar year)
	BudgetPeriodFiscalYear BudgetPeriodType = "fiscal_year"

	// BudgetPeriodGrant represents a grant period budget (multi-year with rollover)
	BudgetPeriodGrant BudgetPeriodType = "grant"
)

// BudgetPeriod represents a flexible budget period with custom date ranges
type BudgetPeriod struct {
	// Type of budget period (daily, weekly, monthly, etc.)
	Type BudgetPeriodType `yaml:"type" json:"type"`

	// Start date of the budget period (required for custom, fiscal_year, grant types)
	StartDate *time.Time `yaml:"start_date,omitempty" json:"start_date,omitempty"`

	// End date of the budget period (required for custom, grant types)
	EndDate *time.Time `yaml:"end_date,omitempty" json:"end_date,omitempty"`

	// Fiscal year start month (1-12, required for fiscal_year type)
	// Example: 10 for October (fiscal year starting October 1)
	FiscalYearStartMonth int `yaml:"fiscal_year_start_month,omitempty" json:"fiscal_year_start_month,omitempty"`

	// Grant name or identifier (optional, for tracking grant budgets)
	GrantName string `yaml:"grant_name,omitempty" json:"grant_name,omitempty"`

	// Enable budget rollover (for grant periods that allow unspent funds to carry over)
	EnableRollover bool `yaml:"enable_rollover,omitempty" json:"enable_rollover,omitempty"`

	// Maximum budget amount for this period
	MaxBudget float64 `yaml:"max_budget" json:"max_budget"`

	// Maximum volume quota in GB (0 = unlimited)
	MaxVolumeGB float64 `yaml:"max_volume_gb,omitempty" json:"max_volume_gb,omitempty"`

	// Alert threshold (0.0-1.0, percentage of budget)
	AlertThreshold float64 `yaml:"alert_threshold" json:"alert_threshold"`

	// Volume alert threshold (0.0-1.0, percentage of volume quota)
	// If not set, uses AlertThreshold for both cost and volume
	VolumeAlertThreshold float64 `yaml:"volume_alert_threshold,omitempty" json:"volume_alert_threshold,omitempty"`
}

// ProjectBudget represents a budget for a specific project (manifest upload ID)
type ProjectBudget struct {
	// Project ID (manifest upload ID, e.g., "20251206-abc123")
	ProjectID string `yaml:"project_id" json:"project_id"`

	// Budget period (references a BudgetPeriod by type, or uses custom dates)
	// If not set, inherits from the global active budget period
	BudgetPeriod *BudgetPeriod `yaml:"budget_period,omitempty" json:"budget_period,omitempty"`

	// Maximum budget for this project
	MaxBudget float64 `yaml:"max_budget" json:"max_budget"`

	// Maximum volume quota in GB (0 = unlimited)
	MaxVolumeGB float64 `yaml:"max_volume_gb,omitempty" json:"max_volume_gb,omitempty"`

	// Alert threshold (0.0-1.0, percentage of budget)
	// If not set, uses budget period's alert threshold
	AlertThreshold float64 `yaml:"alert_threshold,omitempty" json:"alert_threshold,omitempty"`

	// Volume alert threshold (0.0-1.0, percentage of volume quota)
	// If not set, uses AlertThreshold or budget period's volume alert threshold
	VolumeAlertThreshold float64 `yaml:"volume_alert_threshold,omitempty" json:"volume_alert_threshold,omitempty"`

	// Description of the project (optional)
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
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

// GetPeriodBounds returns the start and end dates for the budget period
// relative to the given reference time (usually time.Now())
func (bp *BudgetPeriod) GetPeriodBounds(referenceTime time.Time) (start time.Time, end time.Time, err error) {
	switch bp.Type {
	case BudgetPeriodDaily:
		start = time.Date(referenceTime.Year(), referenceTime.Month(), referenceTime.Day(), 0, 0, 0, 0, referenceTime.Location())
		end = start.AddDate(0, 0, 1).Add(-time.Nanosecond)

	case BudgetPeriodWeekly:
		// Start on Sunday of the current week
		weekday := int(referenceTime.Weekday())
		start = time.Date(referenceTime.Year(), referenceTime.Month(), referenceTime.Day()-weekday, 0, 0, 0, 0, referenceTime.Location())
		end = start.AddDate(0, 0, 7).Add(-time.Nanosecond)

	case BudgetPeriodMonthly:
		// Start on first day of current month
		start = time.Date(referenceTime.Year(), referenceTime.Month(), 1, 0, 0, 0, 0, referenceTime.Location())
		end = start.AddDate(0, 1, 0).Add(-time.Nanosecond)

	case BudgetPeriodQuarterly:
		// Calculate quarter (Q1: Jan-Mar, Q2: Apr-Jun, Q3: Jul-Sep, Q4: Oct-Dec)
		month := int(referenceTime.Month())
		quarterStartMonth := ((month - 1) / 3) * 3 + 1
		start = time.Date(referenceTime.Year(), time.Month(quarterStartMonth), 1, 0, 0, 0, 0, referenceTime.Location())
		end = start.AddDate(0, 3, 0).Add(-time.Nanosecond)

	case BudgetPeriodYearly:
		// Calendar year
		start = time.Date(referenceTime.Year(), 1, 1, 0, 0, 0, 0, referenceTime.Location())
		end = start.AddDate(1, 0, 0).Add(-time.Nanosecond)

	case BudgetPeriodFiscalYear:
		if bp.FiscalYearStartMonth < 1 || bp.FiscalYearStartMonth > 12 {
			return time.Time{}, time.Time{}, fmt.Errorf("fiscal_year_start_month must be between 1 and 12")
		}

		// Determine fiscal year start based on reference time
		fiscalStartMonth := time.Month(bp.FiscalYearStartMonth)
		if referenceTime.Month() < fiscalStartMonth {
			// Current fiscal year started last calendar year
			start = time.Date(referenceTime.Year()-1, fiscalStartMonth, 1, 0, 0, 0, 0, referenceTime.Location())
		} else {
			// Current fiscal year started this calendar year
			start = time.Date(referenceTime.Year(), fiscalStartMonth, 1, 0, 0, 0, 0, referenceTime.Location())
		}
		end = start.AddDate(1, 0, 0).Add(-time.Nanosecond)

	case BudgetPeriodCustom:
		if bp.StartDate == nil || bp.EndDate == nil {
			return time.Time{}, time.Time{}, fmt.Errorf("custom budget period requires both start_date and end_date")
		}
		start = *bp.StartDate
		end = *bp.EndDate

	case BudgetPeriodGrant:
		if bp.StartDate == nil || bp.EndDate == nil {
			return time.Time{}, time.Time{}, fmt.Errorf("grant budget period requires both start_date and end_date")
		}
		start = *bp.StartDate
		end = *bp.EndDate

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unknown budget period type: %s", bp.Type)
	}

	return start, end, nil
}

// GetDaysElapsed returns the number of days elapsed in the current budget period
func (bp *BudgetPeriod) GetDaysElapsed(referenceTime time.Time) (int, error) {
	start, _, err := bp.GetPeriodBounds(referenceTime)
	if err != nil {
		return 0, err
	}

	elapsed := referenceTime.Sub(start)
	days := int(elapsed.Hours() / 24)

	// Always return at least 1 day to avoid division by zero
	if days < 1 {
		days = 1
	}

	return days, nil
}

// GetDaysRemaining returns the number of days remaining in the current budget period
func (bp *BudgetPeriod) GetDaysRemaining(referenceTime time.Time) (int, error) {
	_, end, err := bp.GetPeriodBounds(referenceTime)
	if err != nil {
		return 0, err
	}

	remaining := end.Sub(referenceTime)
	days := int(remaining.Hours() / 24)

	// Return 0 if period has ended
	if days < 0 {
		days = 0
	}

	return days, nil
}

// GetTotalDays returns the total number of days in the current budget period
func (bp *BudgetPeriod) GetTotalDays(referenceTime time.Time) (int, error) {
	start, end, err := bp.GetPeriodBounds(referenceTime)
	if err != nil {
		return 0, err
	}

	duration := end.Sub(start)
	days := int(duration.Hours() / 24)

	// Add 1 to include the end day
	return days + 1, nil
}

// CalculateBurnRate calculates the daily burn rate for the current budget period
// Returns daily burn rate in the same currency as maxBudget
func (bp *BudgetPeriod) CalculateBurnRate(currentSpend float64, referenceTime time.Time) (float64, error) {
	daysElapsed, err := bp.GetDaysElapsed(referenceTime)
	if err != nil {
		return 0, err
	}

	if daysElapsed == 0 {
		return 0, fmt.Errorf("cannot calculate burn rate: no days elapsed")
	}

	return currentSpend / float64(daysElapsed), nil
}

// ProjectEndOfPeriodSpend projects the total spending at the end of the budget period
// based on current burn rate
func (bp *BudgetPeriod) ProjectEndOfPeriodSpend(currentSpend float64, referenceTime time.Time) (float64, error) {
	burnRate, err := bp.CalculateBurnRate(currentSpend, referenceTime)
	if err != nil {
		return 0, err
	}

	daysRemaining, err := bp.GetDaysRemaining(referenceTime)
	if err != nil {
		return 0, err
	}

	projectedSpend := currentSpend + (burnRate * float64(daysRemaining))
	return projectedSpend, nil
}

// WillExceedBudget determines if current spending rate will exceed the budget
// Returns true if projected spending exceeds max budget, along with projected amount
func (bp *BudgetPeriod) WillExceedBudget(currentSpend float64, referenceTime time.Time) (bool, float64, error) {
	projectedSpend, err := bp.ProjectEndOfPeriodSpend(currentSpend, referenceTime)
	if err != nil {
		return false, 0, err
	}

	willExceed := projectedSpend > bp.MaxBudget
	return willExceed, projectedSpend, nil
}

// GetBudgetStatus returns comprehensive status information for the budget period
func (bp *BudgetPeriod) GetBudgetStatus(currentSpend float64, referenceTime time.Time) (map[string]interface{}, error) {
	start, end, err := bp.GetPeriodBounds(referenceTime)
	if err != nil {
		return nil, err
	}

	daysElapsed, err := bp.GetDaysElapsed(referenceTime)
	if err != nil {
		return nil, err
	}

	daysRemaining, err := bp.GetDaysRemaining(referenceTime)
	if err != nil {
		return nil, err
	}

	totalDays, err := bp.GetTotalDays(referenceTime)
	if err != nil {
		return nil, err
	}

	burnRate, err := bp.CalculateBurnRate(currentSpend, referenceTime)
	if err != nil {
		return nil, err
	}

	projectedSpend, err := bp.ProjectEndOfPeriodSpend(currentSpend, referenceTime)
	if err != nil {
		return nil, err
	}

	willExceed, _, err := bp.WillExceedBudget(currentSpend, referenceTime)
	if err != nil {
		return nil, err
	}

	budgetUsed := currentSpend / bp.MaxBudget
	remaining := bp.MaxBudget - currentSpend

	// Calculate target daily rate to stay within budget
	targetDailyRate := 0.0
	if daysRemaining > 0 {
		targetDailyRate = remaining / float64(daysRemaining)
	}

	status := map[string]interface{}{
		// Period information
		"period_type":          bp.Type,
		"period_start":         start.Format("2006-01-02"),
		"period_end":           end.Format("2006-01-02"),
		"days_elapsed":         daysElapsed,
		"days_remaining":       daysRemaining,
		"total_days":           totalDays,

		// Budget information
		"max_budget":           bp.MaxBudget,
		"current_spend":        currentSpend,
		"budget_used":          budgetUsed,
		"budget_remaining":     remaining,
		"alert_threshold":      bp.AlertThreshold,

		// Burn rate and projections
		"daily_burn_rate":      burnRate,
		"projected_eop_spend":  projectedSpend,
		"will_exceed_budget":   willExceed,
		"target_daily_rate":    targetDailyRate,

		// Status flags
		"over_budget":          budgetUsed > 1.0,
		"alert_triggered":      budgetUsed > bp.AlertThreshold,

		// Grant-specific information
		"grant_name":           bp.GrantName,
		"enable_rollover":      bp.EnableRollover,
	}

	// Add overage/savings information
	if willExceed {
		overage := projectedSpend - bp.MaxBudget
		status["projected_overage"] = overage
		status["projected_overage_percent"] = (overage / bp.MaxBudget) * 100
	} else {
		savings := bp.MaxBudget - projectedSpend
		status["projected_savings"] = savings
		status["projected_savings_percent"] = (savings / bp.MaxBudget) * 100
	}

	return status, nil
}

// Validate validates the budget period configuration
func (bp *BudgetPeriod) Validate() error {
	// Check max budget
	if bp.MaxBudget <= 0 {
		return fmt.Errorf("max_budget must be greater than 0")
	}

	// Check alert threshold
	if bp.AlertThreshold < 0 || bp.AlertThreshold > 1 {
		return fmt.Errorf("alert_threshold must be between 0.0 and 1.0")
	}

	// Validate type-specific requirements
	switch bp.Type {
	case BudgetPeriodCustom:
		if bp.StartDate == nil || bp.EndDate == nil {
			return fmt.Errorf("custom budget period requires both start_date and end_date")
		}
		if bp.EndDate.Before(*bp.StartDate) {
			return fmt.Errorf("end_date must be after start_date")
		}

	case BudgetPeriodGrant:
		if bp.StartDate == nil || bp.EndDate == nil {
			return fmt.Errorf("grant budget period requires both start_date and end_date")
		}
		if bp.EndDate.Before(*bp.StartDate) {
			return fmt.Errorf("end_date must be after start_date")
		}
		if bp.GrantName == "" {
			return fmt.Errorf("grant budget period requires grant_name")
		}

	case BudgetPeriodFiscalYear:
		if bp.FiscalYearStartMonth < 1 || bp.FiscalYearStartMonth > 12 {
			return fmt.Errorf("fiscal_year_start_month must be between 1 and 12")
		}

	case BudgetPeriodDaily, BudgetPeriodWeekly, BudgetPeriodMonthly, BudgetPeriodQuarterly, BudgetPeriodYearly:
		// No additional validation needed for these types

	default:
		return fmt.Errorf("unknown budget period type: %s", bp.Type)
	}

	return nil
}

// String returns a human-readable string representation of the budget period
func (bp *BudgetPeriod) String() string {
	switch bp.Type {
	case BudgetPeriodDaily:
		return "Daily Budget"
	case BudgetPeriodWeekly:
		return "Weekly Budget"
	case BudgetPeriodMonthly:
		return "Monthly Budget"
	case BudgetPeriodQuarterly:
		return "Quarterly Budget"
	case BudgetPeriodYearly:
		return "Yearly Budget"
	case BudgetPeriodFiscalYear:
		return fmt.Sprintf("Fiscal Year Budget (starts %s)", time.Month(bp.FiscalYearStartMonth).String())
	case BudgetPeriodCustom:
		if bp.StartDate != nil && bp.EndDate != nil {
			return fmt.Sprintf("Custom Budget (%s to %s)",
				bp.StartDate.Format("2006-01-02"), bp.EndDate.Format("2006-01-02"))
		}
		return "Custom Budget"
	case BudgetPeriodGrant:
		if bp.GrantName != "" {
			return fmt.Sprintf("Grant Budget: %s", bp.GrantName)
		}
		return "Grant Budget"
	default:
		return string(bp.Type)
	}
}

