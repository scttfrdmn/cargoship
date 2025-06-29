// Package lifecycle provides S3 lifecycle policy management for CargoShip
package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Manager handles S3 lifecycle policies for automated cost optimization
type Manager struct {
	s3Client *s3.Client
	bucket   string
}

// NewManager creates a new lifecycle policy manager
func NewManager(s3Client *s3.Client, bucket string) *Manager {
	return &Manager{
		s3Client: s3Client,
		bucket:   bucket,
	}
}

// PolicyTemplate defines a lifecycle policy template
type PolicyTemplate struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Rules       []LifecycleRule          `json:"rules"`
	Savings     EstimatedSavings         `json:"estimated_savings"`
}

// LifecycleRule represents a single lifecycle rule
type LifecycleRule struct {
	ID                     string            `json:"id"`
	Status                 string            `json:"status"` // "Enabled" or "Disabled"
	Filter                 RuleFilter        `json:"filter"`
	Transitions            []Transition      `json:"transitions"`
	Expiration             *Expiration       `json:"expiration,omitempty"`
	AbortIncompleteUploads *int              `json:"abort_incomplete_uploads,omitempty"`
	NoncurrentTransitions  []Transition      `json:"noncurrent_transitions,omitempty"`
}

// RuleFilter defines which objects the rule applies to
type RuleFilter struct {
	Prefix string            `json:"prefix,omitempty"`
	Tags   map[string]string `json:"tags,omitempty"`
}

// Transition defines a storage class transition
type Transition struct {
	Days         int    `json:"days"`
	StorageClass string `json:"storage_class"`
}

// Expiration defines object expiration settings
type Expiration struct {
	Days int `json:"days"`
}

// EstimatedSavings shows potential cost savings
type EstimatedSavings struct {
	MonthlyPercent float64 `json:"monthly_percent"`
	AnnualUSD      float64 `json:"annual_usd"`
}

// GetPredefinedTemplates returns common lifecycle policy templates
func GetPredefinedTemplates() []PolicyTemplate {
	return []PolicyTemplate{
		{
			ID:          "archive-optimization",
			Name:        "Archive Optimization",
			Description: "Aggressive cost optimization for long-term archival data",
			Rules: []LifecycleRule{
				{
					ID:     "cargoship-archive-optimization",
					Status: "Enabled",
					Filter: RuleFilter{
						Prefix: "archives/",
						Tags: map[string]string{
							"cargoship-created-by": "cargoship",
						},
					},
					Transitions: []Transition{
						{Days: 30, StorageClass: "STANDARD_IA"},
						{Days: 90, StorageClass: "GLACIER"},
						{Days: 365, StorageClass: "DEEP_ARCHIVE"},
					},
					AbortIncompleteUploads: aws.Int(7), // Clean up failed uploads after 7 days
				},
			},
			Savings: EstimatedSavings{
				MonthlyPercent: 60.0,
				AnnualUSD:      1200.0, // Example based on 100GB
			},
		},
		{
			ID:          "intelligent-tiering",
			Name:        "Intelligent Tiering",
			Description: "Automatic optimization with S3 Intelligent Tiering",
			Rules: []LifecycleRule{
				{
					ID:     "cargoship-intelligent-tiering",
					Status: "Enabled",
					Filter: RuleFilter{
						Prefix: "archives/",
					},
					Transitions: []Transition{
						{Days: 0, StorageClass: "INTELLIGENT_TIERING"},
					},
					AbortIncompleteUploads: aws.Int(7),
				},
			},
			Savings: EstimatedSavings{
				MonthlyPercent: 25.0,
				AnnualUSD:      600.0,
			},
		},
		{
			ID:          "compliance-retention",
			Name:        "Compliance Retention",
			Description: "7-year retention with automatic deletion for compliance",
			Rules: []LifecycleRule{
				{
					ID:     "cargoship-compliance-retention",
					Status: "Enabled",
					Filter: RuleFilter{
						Prefix: "archives/compliance/",
					},
					Transitions: []Transition{
						{Days: 30, StorageClass: "STANDARD_IA"},
						{Days: 90, StorageClass: "GLACIER"},
						{Days: 365, StorageClass: "DEEP_ARCHIVE"},
					},
					Expiration: &Expiration{
						Days: 2555, // 7 years
					},
					AbortIncompleteUploads: aws.Int(1),
				},
			},
			Savings: EstimatedSavings{
				MonthlyPercent: 65.0,
				AnnualUSD:      1500.0,
			},
		},
		{
			ID:          "fast-access",
			Name:        "Fast Access",
			Description: "Optimized for data that needs quick retrieval",
			Rules: []LifecycleRule{
				{
					ID:     "cargoship-fast-access",
					Status: "Enabled",
					Filter: RuleFilter{
						Prefix: "archives/",
						Tags: map[string]string{
							"cargoship-access-pattern": "frequent",
						},
					},
					Transitions: []Transition{
						{Days: 90, StorageClass: "STANDARD_IA"},
						{Days: 365, StorageClass: "GLACIER"},
					},
					AbortIncompleteUploads: aws.Int(3),
				},
			},
			Savings: EstimatedSavings{
				MonthlyPercent: 30.0,
				AnnualUSD:      400.0,
			},
		},
	}
}

// ApplyPolicy applies a lifecycle policy to the S3 bucket
func (m *Manager) ApplyPolicy(ctx context.Context, template PolicyTemplate) error {
	// Convert our policy template to AWS S3 lifecycle rules
	awsRules := make([]types.LifecycleRule, 0, len(template.Rules))

	for _, rule := range template.Rules {
		awsRule, err := m.convertToAWSRule(rule)
		if err != nil {
			return fmt.Errorf("failed to convert rule %s: %w", rule.ID, err)
		}
		awsRules = append(awsRules, awsRule)
	}

	// Create the lifecycle configuration
	config := &types.BucketLifecycleConfiguration{
		Rules: awsRules,
	}

	// Apply the policy
	_, err := m.s3Client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(m.bucket),
		LifecycleConfiguration: config,
	})

	if err != nil {
		return fmt.Errorf("failed to apply lifecycle policy: %w", err)
	}

	return nil
}

// GetCurrentPolicy retrieves the current lifecycle policy
func (m *Manager) GetCurrentPolicy(ctx context.Context) (*types.BucketLifecycleConfiguration, error) {
	result, err := m.s3Client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(m.bucket),
	})

	if err != nil {
		return nil, err
	}

	return &types.BucketLifecycleConfiguration{
		Rules: result.Rules,
	}, nil
}

// RemovePolicy removes the lifecycle policy from the bucket
func (m *Manager) RemovePolicy(ctx context.Context) error {
	_, err := m.s3Client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{
		Bucket: aws.String(m.bucket),
	})

	return err
}

// ValidatePolicy validates a lifecycle policy template
func (m *Manager) ValidatePolicy(template PolicyTemplate) error {
	if template.ID == "" {
		return fmt.Errorf("policy ID cannot be empty")
	}

	if len(template.Rules) == 0 {
		return fmt.Errorf("policy must have at least one rule")
	}

	for _, rule := range template.Rules {
		if err := m.validateRule(rule); err != nil {
			return fmt.Errorf("invalid rule %s: %w", rule.ID, err)
		}
	}

	return nil
}

// GenerateCustomPolicy generates a custom lifecycle policy based on access patterns
func (m *Manager) GenerateCustomPolicy(patterns map[string]AccessPattern) PolicyTemplate {
	rules := make([]LifecycleRule, 0)
	ruleID := 1

	for prefix, pattern := range patterns {
		rule := LifecycleRule{
			ID:     fmt.Sprintf("cargoship-custom-%d", ruleID),
			Status: "Enabled",
			Filter: RuleFilter{
				Prefix: prefix,
			},
			AbortIncompleteUploads: aws.Int(7),
		}

		// Generate transitions based on access pattern
		switch pattern.Frequency {
		case "frequent":
			rule.Transitions = []Transition{
				{Days: 90, StorageClass: "STANDARD_IA"},
				{Days: 365, StorageClass: "GLACIER"},
			}
		case "infrequent":
			rule.Transitions = []Transition{
				{Days: 30, StorageClass: "STANDARD_IA"},
				{Days: 90, StorageClass: "GLACIER"},
				{Days: 365, StorageClass: "DEEP_ARCHIVE"},
			}
		case "archive":
			rule.Transitions = []Transition{
				{Days: 0, StorageClass: "GLACIER"},
				{Days: 90, StorageClass: "DEEP_ARCHIVE"},
			}
		default: // unknown
			rule.Transitions = []Transition{
				{Days: 0, StorageClass: "INTELLIGENT_TIERING"},
			}
		}

		// Add expiration if specified
		if pattern.RetentionDays > 0 {
			rule.Expiration = &Expiration{
				Days: pattern.RetentionDays,
			}
		}

		rules = append(rules, rule)
		ruleID++
	}

	return PolicyTemplate{
		ID:          "cargoship-custom-generated",
		Name:        "Custom Generated Policy",
		Description: "Automatically generated based on access patterns",
		Rules:       rules,
		Savings: EstimatedSavings{
			MonthlyPercent: 45.0, // Conservative estimate
			AnnualUSD:      0,    // Would need actual data size to calculate
		},
	}
}

// AccessPattern defines how data is expected to be accessed
type AccessPattern struct {
	Frequency      string `json:"frequency"`       // "frequent", "infrequent", "archive", "unknown"
	RetentionDays  int    `json:"retention_days"`  // 0 = no expiration
	SizeGB         float64 `json:"size_gb"`
}

// convertToAWSRule converts our lifecycle rule to AWS S3 format
func (m *Manager) convertToAWSRule(rule LifecycleRule) (types.LifecycleRule, error) {
	awsRule := types.LifecycleRule{
		ID:     aws.String(rule.ID),
		Status: types.ExpirationStatus(rule.Status),
	}

	// Convert filter
	if rule.Filter.Prefix != "" || len(rule.Filter.Tags) > 0 {
		filter := &types.LifecycleRuleFilter{}

		if rule.Filter.Prefix != "" && len(rule.Filter.Tags) == 0 {
			// Simple prefix filter
			filter = &types.LifecycleRuleFilter{
				Prefix: aws.String(rule.Filter.Prefix),
			}
		} else if rule.Filter.Prefix == "" && len(rule.Filter.Tags) == 1 {
			// Simple tag filter
			for key, value := range rule.Filter.Tags {
				filter = &types.LifecycleRuleFilter{
					Tag: &types.Tag{
						Key:   aws.String(key),
						Value: aws.String(value),
					},
				}
				break
			}
		} else {
			// Complex filter with AND
			and := &types.LifecycleRuleAndOperator{}

			if rule.Filter.Prefix != "" {
				and.Prefix = aws.String(rule.Filter.Prefix)
			}

			if len(rule.Filter.Tags) > 0 {
				tags := make([]types.Tag, 0, len(rule.Filter.Tags))
				for key, value := range rule.Filter.Tags {
					tags = append(tags, types.Tag{
						Key:   aws.String(key),
						Value: aws.String(value),
					})
				}
				and.Tags = tags
			}

			filter = &types.LifecycleRuleFilter{
				And: and,
			}
		}

		awsRule.Filter = filter
	}

	// Convert transitions
	if len(rule.Transitions) > 0 {
		transitions := make([]types.Transition, 0, len(rule.Transitions))
		for _, transition := range rule.Transitions {
			transitions = append(transitions, types.Transition{
				Days:         aws.Int32(int32(transition.Days)),
				StorageClass: types.TransitionStorageClass(transition.StorageClass),
			})
		}
		awsRule.Transitions = transitions
	}

	// Convert expiration
	if rule.Expiration != nil {
		awsRule.Expiration = &types.LifecycleExpiration{
			Days: aws.Int32(int32(rule.Expiration.Days)),
		}
	}

	// Convert abort incomplete uploads
	if rule.AbortIncompleteUploads != nil {
		awsRule.AbortIncompleteMultipartUpload = &types.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: aws.Int32(int32(*rule.AbortIncompleteUploads)),
		}
	}

	return awsRule, nil
}

// validateRule validates a single lifecycle rule
func (m *Manager) validateRule(rule LifecycleRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	if rule.Status != "Enabled" && rule.Status != "Disabled" {
		return fmt.Errorf("rule status must be 'Enabled' or 'Disabled'")
	}

	// Validate transitions are in chronological order
	if len(rule.Transitions) > 1 {
		for i := 1; i < len(rule.Transitions); i++ {
			if rule.Transitions[i].Days <= rule.Transitions[i-1].Days {
				return fmt.Errorf("transitions must be in chronological order")
			}
		}
	}

	// Validate storage classes
	validClasses := map[string]bool{
		"STANDARD_IA":        true,
		"ONEZONE_IA":         true,
		"INTELLIGENT_TIERING": true,
		"GLACIER":            true,
		"DEEP_ARCHIVE":       true,
	}

	for _, transition := range rule.Transitions {
		if !validClasses[transition.StorageClass] {
			return fmt.Errorf("invalid storage class: %s", transition.StorageClass)
		}
	}

	return nil
}

// EstimateSavings calculates potential savings from a lifecycle policy
func (m *Manager) EstimateSavings(ctx context.Context, template PolicyTemplate, currentSizeGB float64) (*SavingsEstimate, error) {
	// This is a simplified calculation - real implementation would consider:
	// - Current storage class distribution
	// - Access patterns
	// - Retrieval costs
	// - Regional pricing differences

	estimate := &SavingsEstimate{
		PolicyID:     template.ID,
		PolicyName:   template.Name,
		CurrentSizeGB: currentSizeGB,
	}

	// Calculate current monthly cost (assuming Standard storage)
	standardCostPerGB := 0.023 // $0.023/GB/month for us-east-1
	estimate.CurrentMonthlyCost = currentSizeGB * standardCostPerGB

	// Calculate optimized cost based on lifecycle rules
	optimizedCost := estimate.CurrentMonthlyCost

	for _, rule := range template.Rules {
		// Apply transition savings based on timeline
		if len(rule.Transitions) > 0 {
			// Simplified: assume data follows the lifecycle transitions
			finalStorageClass := rule.Transitions[len(rule.Transitions)-1].StorageClass
			
			var finalCostPerGB float64
			switch finalStorageClass {
			case "STANDARD_IA":
				finalCostPerGB = 0.0125
			case "ONEZONE_IA":
				finalCostPerGB = 0.01
			case "INTELLIGENT_TIERING":
				finalCostPerGB = 0.0225
			case "GLACIER":
				finalCostPerGB = 0.004
			case "DEEP_ARCHIVE":
				finalCostPerGB = 0.00099
			default:
				finalCostPerGB = standardCostPerGB
			}

			// Calculate weighted cost reduction
			optimizedCost = currentSizeGB * finalCostPerGB
		}
	}

	estimate.OptimizedMonthlyCost = optimizedCost
	estimate.MonthlySavings = estimate.CurrentMonthlyCost - estimate.OptimizedMonthlyCost
	estimate.AnnualSavings = estimate.MonthlySavings * 12
	estimate.SavingsPercent = (estimate.MonthlySavings / estimate.CurrentMonthlyCost) * 100

	return estimate, nil
}

// SavingsEstimate contains lifecycle policy savings analysis
type SavingsEstimate struct {
	PolicyID             string  `json:"policy_id"`
	PolicyName           string  `json:"policy_name"`
	CurrentSizeGB        float64 `json:"current_size_gb"`
	CurrentMonthlyCost   float64 `json:"current_monthly_cost"`
	OptimizedMonthlyCost float64 `json:"optimized_monthly_cost"`
	MonthlySavings       float64 `json:"monthly_savings"`
	AnnualSavings        float64 `json:"annual_savings"`
	SavingsPercent       float64 `json:"savings_percent"`
}

// ExportPolicy exports a lifecycle policy to JSON
func (m *Manager) ExportPolicy(template PolicyTemplate) (string, error) {
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ImportPolicy imports a lifecycle policy from JSON
func (m *Manager) ImportPolicy(jsonData string) (*PolicyTemplate, error) {
	var template PolicyTemplate
	if err := json.Unmarshal([]byte(jsonData), &template); err != nil {
		return nil, err
	}

	if err := m.ValidatePolicy(template); err != nil {
		return nil, err
	}

	return &template, nil
}