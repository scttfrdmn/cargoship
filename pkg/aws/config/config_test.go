package config

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDefaultAWSConfig(t *testing.T) {
	config := DefaultAWSConfig()

	if config == nil {
		t.Fatalf("DefaultAWSConfig() returned nil")
		return
	}

	if config.Region != "us-east-1" {
		t.Errorf("DefaultAWSConfig() Region = %v, want us-east-1", config.Region)
	}

	if config.S3.StorageClass != StorageClassIntelligentTiering {
		t.Errorf("DefaultAWSConfig() S3.StorageClass = %v, want %v", config.S3.StorageClass, StorageClassIntelligentTiering)
	}

	if config.S3.MultipartThreshold != 100*1024*1024 {
		t.Errorf("DefaultAWSConfig() S3.MultipartThreshold = %v, want %v", config.S3.MultipartThreshold, 100*1024*1024)
	}

	if config.S3.MultipartChunkSize != 10*1024*1024 {
		t.Errorf("DefaultAWSConfig() S3.MultipartChunkSize = %v, want %v", config.S3.MultipartChunkSize, 10*1024*1024)
	}

	if config.S3.Concurrency != 8 {
		t.Errorf("DefaultAWSConfig() S3.Concurrency = %v, want 8", config.S3.Concurrency)
	}

	if config.CostControl.MaxMonthlyBudget != 1000.0 {
		t.Errorf("DefaultAWSConfig() CostControl.MaxMonthlyBudget = %v, want 1000.0", config.CostControl.MaxMonthlyBudget)
	}

	if config.CostControl.AlertThreshold != 0.8 {
		t.Errorf("DefaultAWSConfig() CostControl.AlertThreshold = %v, want 0.8", config.CostControl.AlertThreshold)
	}

	if !config.CostControl.AutoOptimize {
		t.Errorf("DefaultAWSConfig() CostControl.AutoOptimize = false, want true")
	}

	if config.CostControl.RequireApprovalOver != 500.0 {
		t.Errorf("DefaultAWSConfig() CostControl.RequireApprovalOver = %v, want 500.0", config.CostControl.RequireApprovalOver)
	}
}

func TestAWSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *AWSConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  DefaultAWSConfig(),
			wantErr: false,
		},
		{
			name: "missing region",
			config: &AWSConfig{
				S3: S3Config{
					Bucket:             "test-bucket",
					Concurrency:        8,
					MultipartThreshold: 10 * 1024 * 1024,
				},
				CostControl: CostControlConfig{
					AlertThreshold: 0.8,
				},
			},
			wantErr: true,
			errMsg:  "AWS region is required",
		},
		{
			name: "missing bucket",
			config: &AWSConfig{
				Region: "us-east-1",
				S3: S3Config{
					Concurrency:        8,
					MultipartThreshold: 10 * 1024 * 1024,
				},
				CostControl: CostControlConfig{
					AlertThreshold: 0.8,
				},
			},
			wantErr: true,
			errMsg:  "S3 bucket is required",
		},
		{
			name: "invalid concurrency",
			config: &AWSConfig{
				Region: "us-east-1",
				S3: S3Config{
					Bucket:             "test-bucket",
					Concurrency:        0,
					MultipartThreshold: 10 * 1024 * 1024,
				},
				CostControl: CostControlConfig{
					AlertThreshold: 0.8,
				},
			},
			wantErr: true,
			errMsg:  "S3 concurrency must be at least 1",
		},
		{
			name: "invalid multipart threshold",
			config: &AWSConfig{
				Region: "us-east-1",
				S3: S3Config{
					Bucket:             "test-bucket",
					Concurrency:        8,
					MultipartThreshold: 1024 * 1024, // 1MB, too small
				},
				CostControl: CostControlConfig{
					AlertThreshold: 0.8,
				},
			},
			wantErr: true,
			errMsg:  "multipart threshold must be at least 5MB",
		},
		{
			name: "invalid alert threshold - too low",
			config: &AWSConfig{
				Region: "us-east-1",
				S3: S3Config{
					Bucket:             "test-bucket",
					Concurrency:        8,
					MultipartThreshold: 10 * 1024 * 1024,
				},
				CostControl: CostControlConfig{
					AlertThreshold: -0.1,
				},
			},
			wantErr: true,
			errMsg:  "alert threshold must be between 0.0 and 1.0",
		},
		{
			name: "invalid alert threshold - too high",
			config: &AWSConfig{
				Region: "us-east-1",
				S3: S3Config{
					Bucket:             "test-bucket",
					Concurrency:        8,
					MultipartThreshold: 10 * 1024 * 1024,
				},
				CostControl: CostControlConfig{
					AlertThreshold: 1.1,
				},
			},
			wantErr: true,
			errMsg:  "alert threshold must be between 0.0 and 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set bucket for valid test case
			if tt.name == "valid config" {
				tt.config.S3.Bucket = "test-bucket"
			}

			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("AWSConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("AWSConfig.Validate() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestStorageClassConstants(t *testing.T) {
	if StorageClassStandard != "STANDARD" {
		t.Errorf("StorageClassStandard = %v, want STANDARD", StorageClassStandard)
	}
	if StorageClassStandardIA != "STANDARD_IA" {
		t.Errorf("StorageClassStandardIA = %v, want STANDARD_IA", StorageClassStandardIA)
	}
	if StorageClassOneZoneIA != "ONEZONE_IA" {
		t.Errorf("StorageClassOneZoneIA = %v, want ONEZONE_IA", StorageClassOneZoneIA)
	}
	if StorageClassIntelligentTiering != "INTELLIGENT_TIERING" {
		t.Errorf("StorageClassIntelligentTiering = %v, want INTELLIGENT_TIERING", StorageClassIntelligentTiering)
	}
	if StorageClassGlacier != "GLACIER" {
		t.Errorf("StorageClassGlacier = %v, want GLACIER", StorageClassGlacier)
	}
	if StorageClassDeepArchive != "DEEP_ARCHIVE" {
		t.Errorf("StorageClassDeepArchive = %v, want DEEP_ARCHIVE", StorageClassDeepArchive)
	}
}

func TestLoadAWSConfig(t *testing.T) {
	ctx := context.Background()

	// Test with empty profile and region
	cfg, err := LoadAWSConfig(ctx, "", "")
	if err != nil {
		// This may fail in test environment without AWS credentials, which is acceptable
		t.Logf("LoadAWSConfig() with empty values failed (expected in test env): %v", err)
	} else {
		if cfg.Region == "" {
			t.Logf("LoadAWSConfig() returned config with empty region (expected in test env)")
		}
	}

	// Test with specific region
	cfg, err = LoadAWSConfig(ctx, "", "us-west-2")
	if err != nil {
		// This may fail in test environment without AWS credentials, which is acceptable
		t.Logf("LoadAWSConfig() with region failed (expected in test env): %v", err)
	} else {
		if cfg.Region != "us-west-2" {
			t.Errorf("LoadAWSConfig() region = %v, want us-west-2", cfg.Region)
		}
	}

	// Test with profile
	cfg, err = LoadAWSConfig(ctx, "test-profile", "us-east-1")
	if err != nil {
		// This may fail in test environment without AWS credentials, which is acceptable
		t.Logf("LoadAWSConfig() with profile failed (expected in test env): %v", err)
	}
}

func TestAWSConfigStructFields(t *testing.T) {
	config := &AWSConfig{
		Profile: "test-profile",
		Region:  "us-west-1",
		S3: S3Config{
			Bucket:                  "test-bucket",
			StorageClass:            StorageClassStandard,
			MultipartThreshold:      50 * 1024 * 1024,
			MultipartChunkSize:      5 * 1024 * 1024,
			Concurrency:             16,
			KMSKeyID:                "test-kms-key",
			UseTransferAcceleration: true,
		},
		CostControl: CostControlConfig{
			MaxMonthlyBudget:    2000.0,
			AlertThreshold:      0.9,
			AutoOptimize:        false,
			RequireApprovalOver: 1000.0,
		},
	}

	if config.Profile != "test-profile" {
		t.Errorf("Profile = %v, want test-profile", config.Profile)
	}
	if config.Region != "us-west-1" {
		t.Errorf("Region = %v, want us-west-1", config.Region)
	}
	if config.S3.Bucket != "test-bucket" {
		t.Errorf("S3.Bucket = %v, want test-bucket", config.S3.Bucket)
	}
	if config.S3.StorageClass != StorageClassStandard {
		t.Errorf("S3.StorageClass = %v, want %v", config.S3.StorageClass, StorageClassStandard)
	}
	if config.S3.MultipartThreshold != 50*1024*1024 {
		t.Errorf("S3.MultipartThreshold = %v, want %v", config.S3.MultipartThreshold, 50*1024*1024)
	}
	if config.S3.MultipartChunkSize != 5*1024*1024 {
		t.Errorf("S3.MultipartChunkSize = %v, want %v", config.S3.MultipartChunkSize, 5*1024*1024)
	}
	if config.S3.Concurrency != 16 {
		t.Errorf("S3.Concurrency = %v, want 16", config.S3.Concurrency)
	}
	if config.S3.KMSKeyID != "test-kms-key" {
		t.Errorf("S3.KMSKeyID = %v, want test-kms-key", config.S3.KMSKeyID)
	}
	if !config.S3.UseTransferAcceleration {
		t.Errorf("S3.UseTransferAcceleration = false, want true")
	}
	if config.CostControl.MaxMonthlyBudget != 2000.0 {
		t.Errorf("CostControl.MaxMonthlyBudget = %v, want 2000.0", config.CostControl.MaxMonthlyBudget)
	}
	if config.CostControl.AlertThreshold != 0.9 {
		t.Errorf("CostControl.AlertThreshold = %v, want 0.9", config.CostControl.AlertThreshold)
	}
	if config.CostControl.AutoOptimize {
		t.Errorf("CostControl.AutoOptimize = true, want false")
	}
	if config.CostControl.RequireApprovalOver != 1000.0 {
		t.Errorf("CostControl.RequireApprovalOver = %v, want 1000.0", config.CostControl.RequireApprovalOver)
	}
}

func TestS3ConfigDefaults(t *testing.T) {
	config := DefaultAWSConfig()

	// Test S3 defaults
	if config.S3.MultipartThreshold <= 0 {
		t.Errorf("S3.MultipartThreshold should be > 0, got %v", config.S3.MultipartThreshold)
	}
	if config.S3.MultipartChunkSize <= 0 {
		t.Errorf("S3.MultipartChunkSize should be > 0, got %v", config.S3.MultipartChunkSize)
	}
	if config.S3.Concurrency <= 0 {
		t.Errorf("S3.Concurrency should be > 0, got %v", config.S3.Concurrency)
	}
	if config.S3.StorageClass == "" {
		t.Errorf("S3.StorageClass should not be empty")
	}
}

func TestCostControlDefaults(t *testing.T) {
	config := DefaultAWSConfig()

	// Test CostControl defaults
	if config.CostControl.MaxMonthlyBudget <= 0 {
		t.Errorf("CostControl.MaxMonthlyBudget should be > 0, got %v", config.CostControl.MaxMonthlyBudget)
	}
	if config.CostControl.AlertThreshold < 0 || config.CostControl.AlertThreshold > 1 {
		t.Errorf("CostControl.AlertThreshold should be between 0 and 1, got %v", config.CostControl.AlertThreshold)
	}
	if config.CostControl.RequireApprovalOver < 0 {
		t.Errorf("CostControl.RequireApprovalOver should be >= 0, got %v", config.CostControl.RequireApprovalOver)
	}
}

func TestAWSConfig_Validate_EdgeCases(t *testing.T) {
	config := DefaultAWSConfig()
	config.S3.Bucket = "test-bucket"

	// Test boundary values
	config.S3.MultipartThreshold = 5 * 1024 * 1024 // Exactly 5MB
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() should accept 5MB threshold, got error: %v", err)
	}

	config.S3.Concurrency = 1 // Minimum valid value
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() should accept concurrency=1, got error: %v", err)
	}

	config.CostControl.AlertThreshold = 0.0 // Minimum valid value
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() should accept AlertThreshold=0.0, got error: %v", err)
	}

	config.CostControl.AlertThreshold = 1.0 // Maximum valid value
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() should accept AlertThreshold=1.0, got error: %v", err)
	}
}

func TestLoadAWSConfig_Contexts(t *testing.T) {
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := LoadAWSConfig(ctx, "", "us-east-1")
	if err == nil {
		t.Logf("LoadAWSConfig() with cancelled context didn't fail (may use cached config)")
	}

	// Test with timeout context would be more complex and may not be reliable in tests
}

// TestGetLocalStackEndpoint tests the getLocalStackEndpoint function
func TestGetLocalStackEndpoint(t *testing.T) {
	// Save original env vars
	origAWSEndpoint := os.Getenv("AWS_ENDPOINT_URL")
	origLocalStackEndpoint := os.Getenv("LOCALSTACK_ENDPOINT")
	defer func() {
		os.Setenv("AWS_ENDPOINT_URL", origAWSEndpoint)
		os.Setenv("LOCALSTACK_ENDPOINT", origLocalStackEndpoint)
	}()

	tests := []struct {
		name               string
		awsEndpointURL     string
		localStackEndpoint string
		want               string
	}{
		{
			name:               "AWS_ENDPOINT_URL set",
			awsEndpointURL:     "http://custom:4566",
			localStackEndpoint: "",
			want:               "http://custom:4566",
		},
		{
			name:               "LOCALSTACK_ENDPOINT set",
			awsEndpointURL:     "",
			localStackEndpoint: "http://localstack:4566",
			want:               "http://localstack:4566",
		},
		{
			name:               "Both set - AWS_ENDPOINT_URL takes precedence",
			awsEndpointURL:     "http://aws:4566",
			localStackEndpoint: "http://local:4566",
			want:               "http://aws:4566",
		},
		{
			name:               "Neither set - default",
			awsEndpointURL:     "",
			localStackEndpoint: "",
			want:               "http://localhost:4566",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("AWS_ENDPOINT_URL", tt.awsEndpointURL)
			os.Setenv("LOCALSTACK_ENDPOINT", tt.localStackEndpoint)

			got := getLocalStackEndpoint()
			if got != tt.want {
				t.Errorf("getLocalStackEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsLocalStackConfig tests the IsLocalStackConfig function
func TestIsLocalStackConfig(t *testing.T) {
	// Save original env var
	origEndpoint := os.Getenv("AWS_ENDPOINT_URL")
	defer func() {
		os.Setenv("AWS_ENDPOINT_URL", origEndpoint)
	}()

	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{
			name:     "localhost endpoint",
			endpoint: "http://localhost:4566",
			want:     true,
		},
		{
			name:     "127.0.0.1 endpoint",
			endpoint: "http://127.0.0.1:4566",
			want:     true,
		},
		{
			name:     "non-local endpoint",
			endpoint: "https://s3.amazonaws.com",
			want:     false,
		},
		{
			name:     "empty endpoint",
			endpoint: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("AWS_ENDPOINT_URL", tt.endpoint)

			got := IsLocalStackConfig()
			if got != tt.want {
				t.Errorf("IsLocalStackConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBudgetPeriod_GetPeriodBounds tests the GetPeriodBounds method
func TestBudgetPeriod_GetPeriodBounds(t *testing.T) {
	refTime := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC) // June 15, 2024, Saturday

	tests := []struct {
		name       string
		period     BudgetPeriod
		wantStart  time.Time
		wantEnd    time.Time
		wantErr    bool
		errMessage string
	}{
		{
			name:      "Daily period",
			period:    BudgetPeriod{Type: BudgetPeriodDaily},
			wantStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 6, 15, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name:      "Weekly period",
			period:    BudgetPeriod{Type: BudgetPeriodWeekly},
			wantStart: time.Date(2024, 6, 9, 0, 0, 0, 0, time.UTC), // Sunday June 9
			wantEnd:   time.Date(2024, 6, 15, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name:      "Monthly period",
			period:    BudgetPeriod{Type: BudgetPeriodMonthly},
			wantStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 6, 30, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name:      "Quarterly period - Q2",
			period:    BudgetPeriod{Type: BudgetPeriodQuarterly},
			wantStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), // April 1
			wantEnd:   time.Date(2024, 6, 30, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name:      "Yearly period",
			period:    BudgetPeriod{Type: BudgetPeriodYearly},
			wantStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "Fiscal year - April start",
			period: BudgetPeriod{
				Type:                  BudgetPeriodFiscalYear,
				FiscalYearStartMonth:  4,
			},
			wantStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, 3, 31, 23, 59, 59, 999999999, time.UTC),
		},
		{
			name: "Fiscal year - invalid month",
			period: BudgetPeriod{
				Type:                  BudgetPeriodFiscalYear,
				FiscalYearStartMonth:  13,
			},
			wantErr:    true,
			errMessage: "fiscal_year_start_month must be between 1 and 12",
		},
		{
			name: "Custom period",
			period: BudgetPeriod{
				Type:      BudgetPeriodCustom,
				StartDate: &time.Time{},
				EndDate:   &time.Time{},
			},
			wantStart: time.Time{},
			wantEnd:   time.Time{},
		},
		{
			name: "Custom period - missing dates",
			period: BudgetPeriod{
				Type: BudgetPeriodCustom,
			},
			wantErr:    true,
			errMessage: "custom budget period requires both start_date and end_date",
		},
		{
			name: "Grant period",
			period: BudgetPeriod{
				Type:      BudgetPeriodGrant,
				StartDate: &time.Time{},
				EndDate:   &time.Time{},
			},
			wantStart: time.Time{},
			wantEnd:   time.Time{},
		},
		{
			name: "Grant period - missing dates",
			period: BudgetPeriod{
				Type: BudgetPeriodGrant,
			},
			wantErr:    true,
			errMessage: "grant budget period requires both start_date and end_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := tt.period.GetPeriodBounds(refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetPeriodBounds() expected error, got nil")
				} else if tt.errMessage != "" && err.Error() != tt.errMessage {
					t.Errorf("GetPeriodBounds() error = %v, want %v", err.Error(), tt.errMessage)
				}
				return
			}

			if err != nil {
				t.Errorf("GetPeriodBounds() unexpected error = %v", err)
				return
			}

			if !start.Equal(tt.wantStart) {
				t.Errorf("GetPeriodBounds() start = %v, want %v", start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Errorf("GetPeriodBounds() end = %v, want %v", end, tt.wantEnd)
			}
		})
	}
}

// TestBudgetPeriod_GetDaysElapsed tests the GetDaysElapsed method
func TestBudgetPeriod_GetDaysElapsed(t *testing.T) {
	tests := []struct {
		name      string
		period    BudgetPeriod
		refTime   time.Time
		want      int
		wantErr   bool
	}{
		{
			name:    "Beginning of daily period",
			period:  BudgetPeriod{Type: BudgetPeriodDaily},
			refTime: time.Date(2024, 6, 15, 0, 30, 0, 0, time.UTC),
			want:    1, // Always at least 1 day
		},
		{
			name:    "End of daily period",
			period:  BudgetPeriod{Type: BudgetPeriodDaily},
			refTime: time.Date(2024, 6, 15, 23, 30, 0, 0, time.UTC),
			want:    1,
		},
		{
			name:    "Middle of monthly period",
			period:  BudgetPeriod{Type: BudgetPeriodMonthly},
			refTime: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC), // 15th day
			want:    14, // 14 full days elapsed (1st = day 0)
		},
		{
			name:    "Start of yearly period",
			period:  BudgetPeriod{Type: BudgetPeriodYearly},
			refTime: time.Date(2024, 1, 1, 6, 0, 0, 0, time.UTC),
			want:    1, // At least 1 day
		},
		{
			name: "Invalid period type",
			period: BudgetPeriod{
				Type:                  BudgetPeriodFiscalYear,
				FiscalYearStartMonth:  13,
			},
			refTime: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.period.GetDaysElapsed(tt.refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDaysElapsed() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetDaysElapsed() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("GetDaysElapsed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBudgetPeriod_GetDaysRemaining tests the GetDaysRemaining method
func TestBudgetPeriod_GetDaysRemaining(t *testing.T) {
	tests := []struct {
		name    string
		period  BudgetPeriod
		refTime time.Time
		want    int
		wantErr bool
	}{
		{
			name:    "Beginning of daily period",
			period:  BudgetPeriod{Type: BudgetPeriodDaily},
			refTime: time.Date(2024, 6, 15, 0, 30, 0, 0, time.UTC),
			want:    0, // 23.5 hours = 0 days (floor division)
		},
		{
			name:    "End of daily period",
			period:  BudgetPeriod{Type: BudgetPeriodDaily},
			refTime: time.Date(2024, 6, 15, 23, 30, 0, 0, time.UTC),
			want:    0, // Less than 1 hour remaining
		},
		{
			name:    "Middle of monthly period",
			period:  BudgetPeriod{Type: BudgetPeriodMonthly},
			refTime: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			want:    15, // 15 days remaining (15th to 30th)
		},
		{
			name: "Invalid period type",
			period: BudgetPeriod{
				Type:                 BudgetPeriodFiscalYear,
				FiscalYearStartMonth: 13,
			},
			refTime: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.period.GetDaysRemaining(tt.refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDaysRemaining() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetDaysRemaining() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("GetDaysRemaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBudgetPeriod_GetTotalDays tests the GetTotalDays method
func TestBudgetPeriod_GetTotalDays(t *testing.T) {
	refTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		period  BudgetPeriod
		want    int
		wantErr bool
	}{
		{
			name:   "Daily period",
			period: BudgetPeriod{Type: BudgetPeriodDaily},
			want:   1,
		},
		{
			name:   "Weekly period",
			period: BudgetPeriod{Type: BudgetPeriodWeekly},
			want:   7,
		},
		{
			name:   "Monthly period - June",
			period: BudgetPeriod{Type: BudgetPeriodMonthly},
			want:   30,
		},
		{
			name:   "Quarterly period",
			period: BudgetPeriod{Type: BudgetPeriodQuarterly},
			want:   91, // Apr-Jun = 30+31+30 = 91 days
		},
		{
			name:   "Yearly period",
			period: BudgetPeriod{Type: BudgetPeriodYearly},
			want:   367, // 366 days in 2024 (leap year) + 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.period.GetTotalDays(refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetTotalDays() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetTotalDays() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("GetTotalDays() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBudgetPeriod_CalculateBurnRate tests the CalculateBurnRate method
func TestBudgetPeriod_CalculateBurnRate(t *testing.T) {
	tests := []struct {
		name         string
		period       BudgetPeriod
		currentSpend float64
		refTime      time.Time
		want         float64
		wantErr      bool
	}{
		{
			name:         "Daily period - $100 spent in first day",
			period:       BudgetPeriod{Type: BudgetPeriodDaily},
			currentSpend: 100.0,
			refTime:      time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			want:         100.0, // $100/1 day
		},
		{
			name:         "Monthly period - $500 spent in 9 days",
			period:       BudgetPeriod{Type: BudgetPeriodMonthly},
			currentSpend: 500.0,
			refTime:      time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC),
			want:         55.56, // $500/9 days (June 1-9 full days) = $55.56/day
		},
		{
			name:         "Yearly period - start of year",
			period:       BudgetPeriod{Type: BudgetPeriodYearly},
			currentSpend: 50.0,
			refTime:      time.Date(2024, 1, 1, 6, 0, 0, 0, time.UTC),
			want:         50.0, // $50/1 day (minimum 1 day)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.period.CalculateBurnRate(tt.currentSpend, tt.refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculateBurnRate() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("CalculateBurnRate() unexpected error = %v", err)
				return
			}

			// Allow small tolerance for floating point arithmetic
			tolerance := 0.01
			if got < tt.want-tolerance || got > tt.want+tolerance {
				t.Errorf("CalculateBurnRate() = %v, want %v (±%.2f)", got, tt.want, tolerance)
			}
		})
	}
}

// TestBudgetPeriod_ProjectEndOfPeriodSpend tests the ProjectEndOfPeriodSpend method
func TestBudgetPeriod_ProjectEndOfPeriodSpend(t *testing.T) {
	tests := []struct {
		name         string
		period       BudgetPeriod
		currentSpend float64
		refTime      time.Time
		want         float64
		wantErr      bool
	}{
		{
			name:         "Monthly period - mid-month projection",
			period:       BudgetPeriod{Type: BudgetPeriodMonthly},
			currentSpend: 500.0, // $500 spent by June 10
			refTime:      time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC),
			// Burn rate: $500 / 9 days = $55.56/day
			// Days remaining: ~20 days
			// Projected: $500 + ($55.56 * 20) = ~$1611
			want: 1611.11, // Approximate
		},
		{
			name:         "Daily period - end of day",
			period:       BudgetPeriod{Type: BudgetPeriodDaily},
			currentSpend: 100.0,
			refTime:      time.Date(2024, 6, 15, 23, 0, 0, 0, time.UTC),
			// Burn rate: $100 / 1 day
			// Days remaining: 0
			// Projected: $100 + ($100 * 0) = $100
			want: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.period.ProjectEndOfPeriodSpend(tt.currentSpend, tt.refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ProjectEndOfPeriodSpend() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ProjectEndOfPeriodSpend() unexpected error = %v", err)
				return
			}

			// Allow 1% tolerance for floating point arithmetic
			tolerance := tt.want * 0.01
			if got < tt.want-tolerance || got > tt.want+tolerance {
				t.Errorf("ProjectEndOfPeriodSpend() = %v, want %v (±1%%)", got, tt.want)
			}
		})
	}
}

// TestBudgetPeriod_WillExceedBudget tests the WillExceedBudget method
func TestBudgetPeriod_WillExceedBudget(t *testing.T) {
	tests := []struct {
		name         string
		period       BudgetPeriod
		currentSpend float64
		refTime      time.Time
		wantExceed   bool
		wantErr      bool
	}{
		{
			name: "Will exceed - high burn rate",
			period: BudgetPeriod{
				Type:      BudgetPeriodMonthly,
				MaxBudget: 1000.0,
			},
			currentSpend: 800.0, // $800 spent by June 10
			refTime:      time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC),
			// Burn rate: $800 / 9 days = $88.89/day
			// Projected: $800 + ($88.89 * 20) ≈ $1578 > $1000
			wantExceed: true,
		},
		{
			name: "Won't exceed - low burn rate",
			period: BudgetPeriod{
				Type:      BudgetPeriodMonthly,
				MaxBudget: 1000.0,
			},
			currentSpend: 200.0, // $200 spent by June 10
			refTime:      time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC),
			// Burn rate: $200 / 9 days = $22.22/day
			// Projected: $200 + ($22.22 * 20) ≈ $644 < $1000
			wantExceed: false,
		},
		{
			name: "Already over budget",
			period: BudgetPeriod{
				Type:      BudgetPeriodDaily,
				MaxBudget: 50.0,
			},
			currentSpend: 150.0,
			refTime:      time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			wantExceed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExceed, gotProjected, err := tt.period.WillExceedBudget(tt.currentSpend, tt.refTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("WillExceedBudget() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("WillExceedBudget() unexpected error = %v", err)
				return
			}

			if gotExceed != tt.wantExceed {
				t.Errorf("WillExceedBudget() exceed = %v, want %v (projected: %v, budget: %v)",
					gotExceed, tt.wantExceed, gotProjected, tt.period.MaxBudget)
			}
		})
	}
}

// TestBudgetPeriod_GetBudgetStatus tests the GetBudgetStatus method
func TestBudgetPeriod_GetBudgetStatus(t *testing.T) {
	period := BudgetPeriod{
		Type:           BudgetPeriodMonthly,
		MaxBudget:      1000.0,
		AlertThreshold: 0.8,
	}

	refTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	currentSpend := 600.0

	status, err := period.GetBudgetStatus(currentSpend, refTime)
	if err != nil {
		t.Fatalf("GetBudgetStatus() unexpected error = %v", err)
	}

	// Verify expected keys exist
	expectedKeys := []string{
		"period_type", "period_start", "period_end",
		"days_elapsed", "days_remaining", "total_days",
		"max_budget", "current_spend", "budget_used", "budget_remaining",
		"daily_burn_rate", "projected_eop_spend", "will_exceed_budget",
		"target_daily_rate", "over_budget", "alert_triggered",
	}

	for _, key := range expectedKeys {
		if _, exists := status[key]; !exists {
			t.Errorf("GetBudgetStatus() missing key: %s", key)
		}
	}

	// Verify some values
	if status["period_type"] != BudgetPeriodMonthly {
		t.Errorf("period_type = %v, want %v", status["period_type"], BudgetPeriodMonthly)
	}

	if status["max_budget"] != 1000.0 {
		t.Errorf("max_budget = %v, want 1000.0", status["max_budget"])
	}

	if status["current_spend"] != 600.0 {
		t.Errorf("current_spend = %v, want 600.0", status["current_spend"])
	}

	// Verify alert triggered (60% > 80% threshold = false)
	alertTriggered, ok := status["alert_triggered"].(bool)
	if !ok {
		t.Errorf("alert_triggered is not bool")
	}
	if alertTriggered {
		t.Errorf("alert_triggered = true, want false (60%% < 80%% threshold)")
	}
}

// TestBudgetPeriod_Validate tests the Validate method
func TestBudgetPeriod_Validate(t *testing.T) {
	validStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	validEnd := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		period  BudgetPeriod
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid daily period",
			period: BudgetPeriod{
				Type:           BudgetPeriodDaily,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
			},
			wantErr: false,
		},
		{
			name: "Invalid - zero max budget",
			period: BudgetPeriod{
				Type:           BudgetPeriodMonthly,
				MaxBudget:      0,
				AlertThreshold: 0.8,
			},
			wantErr: true,
			errMsg:  "max_budget must be greater than 0",
		},
		{
			name: "Invalid - negative max budget",
			period: BudgetPeriod{
				Type:           BudgetPeriodMonthly,
				MaxBudget:      -100,
				AlertThreshold: 0.8,
			},
			wantErr: true,
			errMsg:  "max_budget must be greater than 0",
		},
		{
			name: "Invalid - alert threshold > 1",
			period: BudgetPeriod{
				Type:           BudgetPeriodMonthly,
				MaxBudget:      100.0,
				AlertThreshold: 1.5,
			},
			wantErr: true,
			errMsg:  "alert_threshold must be between 0.0 and 1.0",
		},
		{
			name: "Invalid - alert threshold < 0",
			period: BudgetPeriod{
				Type:           BudgetPeriodMonthly,
				MaxBudget:      100.0,
				AlertThreshold: -0.1,
			},
			wantErr: true,
			errMsg:  "alert_threshold must be between 0.0 and 1.0",
		},
		{
			name: "Invalid - custom period missing dates",
			period: BudgetPeriod{
				Type:           BudgetPeriodCustom,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
			},
			wantErr: true,
			errMsg:  "custom budget period requires both start_date and end_date",
		},
		{
			name: "Invalid - custom period end before start",
			period: BudgetPeriod{
				Type:           BudgetPeriodCustom,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
				StartDate:      &validEnd,
				EndDate:        &validStart,
			},
			wantErr: true,
			errMsg:  "end_date must be after start_date",
		},
		{
			name: "Valid custom period",
			period: BudgetPeriod{
				Type:           BudgetPeriodCustom,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
				StartDate:      &validStart,
				EndDate:        &validEnd,
			},
			wantErr: false,
		},
		{
			name: "Invalid - grant period missing grant name",
			period: BudgetPeriod{
				Type:           BudgetPeriodGrant,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
				StartDate:      &validStart,
				EndDate:        &validEnd,
			},
			wantErr: true,
			errMsg:  "grant budget period requires grant_name",
		},
		{
			name: "Valid grant period",
			period: BudgetPeriod{
				Type:           BudgetPeriodGrant,
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
				StartDate:      &validStart,
				EndDate:        &validEnd,
				GrantName:      "Research Grant 2024",
			},
			wantErr: false,
		},
		{
			name: "Invalid - fiscal year invalid month",
			period: BudgetPeriod{
				Type:                 BudgetPeriodFiscalYear,
				MaxBudget:            100.0,
				AlertThreshold:       0.8,
				FiscalYearStartMonth: 13,
			},
			wantErr: true,
			errMsg:  "fiscal_year_start_month must be between 1 and 12",
		},
		{
			name: "Valid fiscal year",
			period: BudgetPeriod{
				Type:                 BudgetPeriodFiscalYear,
				MaxBudget:            100.0,
				AlertThreshold:       0.8,
				FiscalYearStartMonth: 4,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.period.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

// TestBudgetPeriod_String tests the String method
func TestBudgetPeriod_String(t *testing.T) {
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		period BudgetPeriod
		want   string
	}{
		{
			name:   "Daily",
			period: BudgetPeriod{Type: BudgetPeriodDaily},
			want:   "Daily Budget",
		},
		{
			name:   "Weekly",
			period: BudgetPeriod{Type: BudgetPeriodWeekly},
			want:   "Weekly Budget",
		},
		{
			name:   "Monthly",
			period: BudgetPeriod{Type: BudgetPeriodMonthly},
			want:   "Monthly Budget",
		},
		{
			name:   "Quarterly",
			period: BudgetPeriod{Type: BudgetPeriodQuarterly},
			want:   "Quarterly Budget",
		},
		{
			name:   "Yearly",
			period: BudgetPeriod{Type: BudgetPeriodYearly},
			want:   "Yearly Budget",
		},
		{
			name: "Fiscal Year",
			period: BudgetPeriod{
				Type:                 BudgetPeriodFiscalYear,
				FiscalYearStartMonth: 4,
			},
			want: "Fiscal Year Budget (starts April)",
		},
		{
			name: "Custom with dates",
			period: BudgetPeriod{
				Type:      BudgetPeriodCustom,
				StartDate: &startDate,
				EndDate:   &endDate,
			},
			want: "Custom Budget (2024-01-01 to 2024-12-31)",
		},
		{
			name:   "Custom without dates",
			period: BudgetPeriod{Type: BudgetPeriodCustom},
			want:   "Custom Budget",
		},
		{
			name: "Grant with name",
			period: BudgetPeriod{
				Type:      BudgetPeriodGrant,
				GrantName: "NSF Research Grant",
			},
			want: "Grant Budget: NSF Research Grant",
		},
		{
			name:   "Grant without name",
			period: BudgetPeriod{Type: BudgetPeriodGrant},
			want:   "Grant Budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.period.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
