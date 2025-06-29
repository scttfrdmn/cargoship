// Package config provides AWS configuration management for CargoShip
package config

import (
	"context"
	"fmt"

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
}

// StorageClass represents S3 storage classes
type StorageClass string

const (
	StorageClassStandard          StorageClass = "STANDARD"
	StorageClassStandardIA        StorageClass = "STANDARD_IA"
	StorageClassOneZoneIA         StorageClass = "ONEZONE_IA"
	StorageClassIntelligentTiering StorageClass = "INTELLIGENT_TIERING"
	StorageClassGlacier           StorageClass = "GLACIER"
	StorageClassDeepArchive       StorageClass = "DEEP_ARCHIVE"
)

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

// LoadAWSConfig loads AWS configuration with CargoShip defaults
func LoadAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error
	
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}