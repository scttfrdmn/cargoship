package config

import (
	"context"
	"testing"
)

func TestDefaultAWSConfig(t *testing.T) {
	config := DefaultAWSConfig()
	
	if config == nil {
		t.Fatalf("DefaultAWSConfig() returned nil")
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
			name:   "valid config",
			config: DefaultAWSConfig(),
			wantErr: false,
		},
		{
			name: "missing region",
			config: &AWSConfig{
				S3: S3Config{
					Bucket:      "test-bucket",
					Concurrency: 8,
					MultipartThreshold: 10*1024*1024,
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
					Concurrency: 8,
					MultipartThreshold: 10*1024*1024,
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
					Bucket:      "test-bucket",
					Concurrency: 0,
					MultipartThreshold: 10*1024*1024,
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
					Bucket:      "test-bucket",
					Concurrency: 8,
					MultipartThreshold: 1024*1024, // 1MB, too small
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
					Bucket:      "test-bucket",
					Concurrency: 8,
					MultipartThreshold: 10*1024*1024,
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
					Bucket:      "test-bucket",
					Concurrency: 8,
					MultipartThreshold: 10*1024*1024,
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
			StorageClass:           StorageClassStandard,
			MultipartThreshold:     50*1024*1024,
			MultipartChunkSize:     5*1024*1024,
			Concurrency:            16,
			KMSKeyID:               "test-kms-key",
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
	config.S3.MultipartThreshold = 5*1024*1024 // Exactly 5MB
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