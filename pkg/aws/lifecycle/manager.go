// Package lifecycle provides S3 lifecycle policy management for CargoShip
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Client defines the interface for S3 lifecycle operations
type S3Client interface {
	PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
	GetBucketLifecycleConfiguration(ctx context.Context, params *s3.GetBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error)
	DeleteBucketLifecycle(ctx context.Context, params *s3.DeleteBucketLifecycleInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketLifecycleOutput, error)
}

// Manager handles S3 lifecycle policies for automated cost optimization
type Manager struct {
	s3Client S3Client
	bucket   string
}

// NewManager creates a new lifecycle policy manager
func NewManager(s3Client S3Client, bucket string) *Manager {
	return &Manager{
		s3Client: s3Client,
		bucket:   bucket,
	}
}

// PolicyTemplate defines a lifecycle policy template
type PolicyTemplate struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rules       []LifecycleRule  `json:"rules"`
	Savings     EstimatedSavings `json:"estimated_savings"`
}

// LifecycleRule represents a single lifecycle rule
type LifecycleRule struct {
	ID                     string       `json:"id"`
	Status                 string       `json:"status"` // "Enabled" or "Disabled"
	Filter                 RuleFilter   `json:"filter"`
	Transitions            []Transition `json:"transitions"`
	Expiration             *Expiration  `json:"expiration,omitempty"`
	AbortIncompleteUploads *int         `json:"abort_incomplete_uploads,omitempty"`
	NoncurrentTransitions  []Transition `json:"noncurrent_transitions,omitempty"`
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

// Object tag that marks CargoShip archive data (#993). Lifecycle transitions
// select objects by this tag, never by prefix alone: a manifest
// (<prefix>/uploads/<id>/manifest*.json[.gz]) sits under the same prefix as its
// chunks, and S3 lifecycle filters have no suffix match, so a prefix-only rule
// moved manifests to GLACIER/DEEP_ARCHIVE too and left every upload unreadable
// without a thaw. The uploader sets this tag on chunk objects only.
const (
	// ChunkTagKey is the object tag key CargoShip sets on archive chunks.
	ChunkTagKey = "cargoship-object"
	// ChunkTagValue is the ChunkTagKey value for an archive chunk.
	ChunkTagValue = "chunk"
)

// ComplianceRetentionDays is how long the compliance-retention template keeps
// objects before S3 deletes them: 2,555 days, about 7 years.
const ComplianceRetentionDays = 2555

// templateCompliance is the template whose rules delete data.
const templateCompliance = "compliance-retention"

// NormalizePrefix turns an upload prefix (the part after the bucket in
// s3://bucket/prefix) into a lifecycle filter prefix: surrounding slashes are
// trimmed and one trailing "/" is added, so "backup" cannot also match
// "backup2/". An empty prefix means the whole bucket.
func NormalizePrefix(prefix string) string {
	p := strings.Trim(prefix, "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

// dataRules builds a template's rules for one prefix. Transitions go in a rule
// that matches only tagged chunks under the prefix. Multipart-upload cleanup and
// any expiration go in a second rule that matches by prefix only: S3 rejects
// AbortIncompleteMultipartUpload in a tag-filtered rule, and an expiration must
// remove an upload's manifests together with its chunks, not strand them.
func dataRules(id, prefix string, transitions []Transition, expiration *Expiration, abortDays int) []LifecycleRule {
	return []LifecycleRule{
		{
			ID:          managedRulePrefix + id,
			Status:      "Enabled",
			Filter:      RuleFilter{Prefix: prefix, Tags: map[string]string{ChunkTagKey: ChunkTagValue}},
			Transitions: transitions,
		},
		{
			ID:                     managedRulePrefix + id + "-cleanup",
			Status:                 "Enabled",
			Filter:                 RuleFilter{Prefix: prefix},
			Expiration:             expiration,
			AbortIncompleteUploads: aws.Int(abortDays),
		},
	}
}

// PredefinedTemplates returns the built-in lifecycle templates scoped to an
// upload prefix (see NormalizePrefix; "" is the whole bucket). Each template
// transitions only CargoShip chunks (objects tagged ChunkTagKey=ChunkTagValue);
// manifests stay in the class they were uploaded in, so sync, list, info,
// verify and restore keep working (#993).
func PredefinedTemplates(prefix string) []PolicyTemplate {
	p := NormalizePrefix(prefix)
	return []PolicyTemplate{
		{
			ID:          "archive-optimization",
			Name:        "Archive Optimization",
			Description: "Aggressive cost optimization for long-term archival data",
			Rules: dataRules("archive-optimization", p, []Transition{
				{Days: 30, StorageClass: "STANDARD_IA"},
				{Days: 90, StorageClass: "GLACIER"},
				{Days: 365, StorageClass: "DEEP_ARCHIVE"},
			}, nil, 7), // clean up failed uploads after 7 days
			Savings: EstimatedSavings{
				MonthlyPercent: 60.0,
				AnnualUSD:      1200.0, // Example based on 100GB
			},
		},
		{
			ID:          "intelligent-tiering",
			Name:        "Intelligent Tiering",
			Description: "Automatic optimization with S3 Intelligent Tiering",
			Rules: dataRules("intelligent-tiering", p, []Transition{
				{Days: 0, StorageClass: "INTELLIGENT_TIERING"},
			}, nil, 7),
			Savings: EstimatedSavings{
				MonthlyPercent: 25.0,
				AnnualUSD:      600.0,
			},
		},
		{
			ID:   templateCompliance,
			Name: "Compliance Retention",
			Description: fmt.Sprintf("Archive tiering, then DELETES every object under the prefix (chunks and manifests) "+
				"%d days (about 7 years) after it was written", ComplianceRetentionDays),
			Rules: dataRules(templateCompliance, p, []Transition{
				{Days: 30, StorageClass: "STANDARD_IA"},
				{Days: 90, StorageClass: "GLACIER"},
				{Days: 365, StorageClass: "DEEP_ARCHIVE"},
			}, &Expiration{Days: ComplianceRetentionDays}, 1),
			Savings: EstimatedSavings{
				MonthlyPercent: 65.0,
				AnnualUSD:      1500.0,
			},
		},
		{
			ID:          "fast-access",
			Name:        "Fast Access",
			Description: "Optimized for data that needs quick retrieval",
			Rules: dataRules("fast-access", p, []Transition{
				{Days: 90, StorageClass: "STANDARD_IA"},
				{Days: 365, StorageClass: "GLACIER"},
			}, nil, 3),
			Savings: EstimatedSavings{
				MonthlyPercent: 30.0,
				AnnualUSD:      400.0,
			},
		},
	}
}

// GetPredefinedTemplates returns the built-in templates scoped to the whole
// bucket. Use PredefinedTemplates or TemplateFor to scope them to the prefix an
// upload was written to.
func GetPredefinedTemplates() []PolicyTemplate {
	return PredefinedTemplates("")
}

// TemplateFor returns the built-in template id scoped to prefix. A template
// that deletes data (compliance-retention) requires a non-empty prefix, so it
// can't expire every object in the bucket by accident.
func TemplateFor(id, prefix string) (PolicyTemplate, error) {
	for _, t := range PredefinedTemplates(prefix) {
		if t.ID != id {
			continue
		}
		if id == templateCompliance && NormalizePrefix(prefix) == "" {
			return PolicyTemplate{}, fmt.Errorf("template %q deletes data after %d days, so it needs the prefix your uploads use, not the whole bucket", id, ComplianceRetentionDays)
		}
		return t, nil
	}
	return PolicyTemplate{}, fmt.Errorf("template '%s' not found. Use --list-templates to see available templates", id)
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

	// #920: merge into the bucket's existing configuration instead of replacing
	// it. PutBucketLifecycleConfiguration replaces the WHOLE configuration, so
	// applying a template deleted every other team's rules on a shared bucket.
	// Rules CargoShip manages (ID prefix "cargoship-") and rules with the same ID
	// as one being applied are replaced; everything else is kept.
	existing, err := m.currentRules(ctx)
	if err != nil {
		return err
	}
	applying := make(map[string]bool, len(awsRules))
	for _, r := range awsRules {
		applying[aws.ToString(r.ID)] = true
	}
	merged := make([]types.LifecycleRule, 0, len(existing)+len(awsRules))
	for _, r := range existing {
		id := aws.ToString(r.ID)
		if applying[id] || isManagedRule(id) {
			continue
		}
		merged = append(merged, r)
	}
	merged = append(merged, awsRules...)

	_, err = m.s3Client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(m.bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: merged},
	})
	if err != nil {
		return fmt.Errorf("failed to apply lifecycle policy: %w", err)
	}
	return nil
}

// managedRulePrefix marks the lifecycle rules CargoShip's templates create.
const managedRulePrefix = "cargoship-"

func isManagedRule(id string) bool { return strings.HasPrefix(id, managedRulePrefix) }

// currentRules returns the bucket's lifecycle rules, or none if it has no
// configuration. Any other error stops the change, since writing without the
// current rules would drop them.
func (m *Manager) currentRules(ctx context.Context) ([]types.LifecycleRule, error) {
	out, err := m.s3Client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(m.bucket),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchLifecycleConfiguration" {
			return nil, nil
		}
		return nil, fmt.Errorf("read the bucket's current lifecycle rules (not changing them without): %w", err)
	}
	return out.Rules, nil
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
//
// #920: only CargoShip's own rules (ID prefix "cargoship-") are removed. This
// used to delete the bucket's entire lifecycle configuration, including rules
// other teams depend on. The configuration is deleted only when no other rule
// remains.
func (m *Manager) RemovePolicy(ctx context.Context) error {
	existing, err := m.currentRules(ctx)
	if err != nil {
		return err
	}
	var kept []types.LifecycleRule
	for _, r := range existing {
		if !isManagedRule(aws.ToString(r.ID)) {
			kept = append(kept, r)
		}
	}
	if len(kept) == len(existing) && len(existing) > 0 {
		return nil // nothing of ours to remove
	}
	if len(kept) == 0 {
		_, err = m.s3Client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: aws.String(m.bucket)})
		return err
	}
	_, err = m.s3Client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(m.bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: kept},
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
		// Generate transitions based on access pattern
		var transitions []Transition
		switch pattern.Frequency {
		case "frequent":
			transitions = []Transition{
				{Days: 90, StorageClass: "STANDARD_IA"},
				{Days: 365, StorageClass: "GLACIER"},
			}
		case "infrequent":
			transitions = []Transition{
				{Days: 30, StorageClass: "STANDARD_IA"},
				{Days: 90, StorageClass: "GLACIER"},
				{Days: 365, StorageClass: "DEEP_ARCHIVE"},
			}
		case "archive":
			transitions = []Transition{
				{Days: 0, StorageClass: "GLACIER"},
				{Days: 90, StorageClass: "DEEP_ARCHIVE"},
			}
		default: // unknown
			transitions = []Transition{
				{Days: 0, StorageClass: "INTELLIGENT_TIERING"},
			}
		}

		// Add expiration if specified
		var expiration *Expiration
		if pattern.RetentionDays > 0 {
			expiration = &Expiration{Days: pattern.RetentionDays}
		}

		// Chunks transition; manifests don't (#993).
		rules = append(rules, dataRules(fmt.Sprintf("custom-%d", ruleID), prefix, transitions, expiration, 7)...)
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
	Frequency     string  `json:"frequency"`      // "frequent", "infrequent", "archive", "unknown"
	RetentionDays int     `json:"retention_days"` // 0 = no expiration
	SizeGB        float64 `json:"size_gb"`
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
	} else {
		// No prefix and no tags: the whole bucket. S3 needs an explicit filter.
		awsRule.Filter = &types.LifecycleRuleFilter{Prefix: aws.String("")}
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

	// S3 rejects this combination; say so before the API call does.
	if len(rule.Filter.Tags) > 0 && rule.AbortIncompleteUploads != nil {
		return fmt.Errorf("a rule with a tag filter cannot abort incomplete multipart uploads; put that in a separate prefix-only rule")
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
		"STANDARD_IA":         true,
		"ONEZONE_IA":          true,
		"INTELLIGENT_TIERING": true,
		"GLACIER":             true,
		"DEEP_ARCHIVE":        true,
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
		PolicyID:      template.ID,
		PolicyName:    template.Name,
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
