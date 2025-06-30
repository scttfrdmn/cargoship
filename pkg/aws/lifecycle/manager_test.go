package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// MockS3Client implements the S3 client interface for testing
type MockS3Client struct {
	putLifecycleConfigCalls   []s3.PutBucketLifecycleConfigurationInput
	getLifecycleConfigCalls   []s3.GetBucketLifecycleConfigurationInput
	deleteLifecycleCalls      []s3.DeleteBucketLifecycleInput
	lifecycleConfig           *types.BucketLifecycleConfiguration
	putError                  error
	getError                  error
	deleteError               error
}

func (m *MockS3Client) PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	if m.putLifecycleConfigCalls == nil {
		m.putLifecycleConfigCalls = make([]s3.PutBucketLifecycleConfigurationInput, 0)
	}
	m.putLifecycleConfigCalls = append(m.putLifecycleConfigCalls, *params)
	
	if m.putError != nil {
		return nil, m.putError
	}
	
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (m *MockS3Client) GetBucketLifecycleConfiguration(ctx context.Context, params *s3.GetBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error) {
	if m.getLifecycleConfigCalls == nil {
		m.getLifecycleConfigCalls = make([]s3.GetBucketLifecycleConfigurationInput, 0)
	}
	m.getLifecycleConfigCalls = append(m.getLifecycleConfigCalls, *params)
	
	if m.getError != nil {
		return nil, m.getError
	}
	
	if m.lifecycleConfig != nil {
		return &s3.GetBucketLifecycleConfigurationOutput{
			Rules: m.lifecycleConfig.Rules,
		}, nil
	}
	
	return &s3.GetBucketLifecycleConfigurationOutput{}, nil
}

func (m *MockS3Client) DeleteBucketLifecycle(ctx context.Context, params *s3.DeleteBucketLifecycleInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketLifecycleOutput, error) {
	if m.deleteLifecycleCalls == nil {
		m.deleteLifecycleCalls = make([]s3.DeleteBucketLifecycleInput, 0)
	}
	m.deleteLifecycleCalls = append(m.deleteLifecycleCalls, *params)
	
	if m.deleteError != nil {
		return nil, m.deleteError
	}
	
	return &s3.DeleteBucketLifecycleOutput{}, nil
}

func TestNewManager(t *testing.T) {
	mockS3 := &MockS3Client{}
	bucket := "test-bucket"
	
	manager := NewManager(mockS3, bucket)
	
	if manager == nil {
		t.Fatalf("NewManager() returned nil")
	}
	
	if manager.bucket != bucket {
		t.Errorf("Manager bucket = %v, want %v", manager.bucket, bucket)
	}
	
	// Note: We can't directly compare interface implementations
}

func TestGetPredefinedTemplates(t *testing.T) {
	templates := GetPredefinedTemplates()
	
	if len(templates) == 0 {
		t.Fatalf("GetPredefinedTemplates() returned no templates")
	}
	
	expectedTemplates := []string{
		"archive-optimization",
		"intelligent-tiering", 
		"compliance-retention",
		"fast-access",
	}
	
	templateIDs := make(map[string]bool)
	for _, template := range templates {
		templateIDs[template.ID] = true
		
		// Validate template structure
		if template.ID == "" {
			t.Errorf("Template has empty ID")
		}
		if template.Name == "" {
			t.Errorf("Template %s has empty name", template.ID)
		}
		if len(template.Rules) == 0 {
			t.Errorf("Template %s has no rules", template.ID)
		}
	}
	
	for _, expectedID := range expectedTemplates {
		if !templateIDs[expectedID] {
			t.Errorf("Missing expected template: %s", expectedID)
		}
	}
}

func TestManager_ApplyPolicy(t *testing.T) {
	mockS3 := &MockS3Client{}
	manager := NewManager(mockS3, "test-bucket")
	
	template := PolicyTemplate{
		ID:   "test-policy",
		Name: "Test Policy",
		Rules: []LifecycleRule{
			{
				ID:     "test-rule",
				Status: "Enabled",
				Filter: RuleFilter{
					Prefix: "archives/",
				},
				Transitions: []Transition{
					{Days: 30, StorageClass: "STANDARD_IA"},
					{Days: 90, StorageClass: "GLACIER"},
				},
				AbortIncompleteUploads: aws.Int(7),
			},
		},
	}
	
	ctx := context.Background()
	err := manager.ApplyPolicy(ctx, template)
	
	if err != nil {
		t.Errorf("ApplyPolicy() error = %v", err)
	}
	
	if len(mockS3.putLifecycleConfigCalls) != 1 {
		t.Errorf("Expected 1 PutBucketLifecycleConfiguration call, got %d", len(mockS3.putLifecycleConfigCalls))
	}
	
	call := mockS3.putLifecycleConfigCalls[0]
	if *call.Bucket != "test-bucket" {
		t.Errorf("PutBucketLifecycleConfiguration bucket = %v, want test-bucket", *call.Bucket)
	}
	
	if len(call.LifecycleConfiguration.Rules) != 1 {
		t.Errorf("Expected 1 rule in lifecycle configuration, got %d", len(call.LifecycleConfiguration.Rules))
	}
}

func TestManager_ApplyPolicy_Error(t *testing.T) {
	mockS3 := &MockS3Client{
		putError: fmt.Errorf("S3 error"),
	}
	manager := NewManager(mockS3, "test-bucket")
	
	template := PolicyTemplate{
		ID:   "test-policy",
		Rules: []LifecycleRule{
			{
				ID:     "test-rule",
				Status: "Enabled",
			},
		},
	}
	
	ctx := context.Background()
	err := manager.ApplyPolicy(ctx, template)
	
	if err == nil {
		t.Errorf("ApplyPolicy() should return error when S3 fails")
	}
}

func TestManager_GetCurrentPolicy(t *testing.T) {
	expectedRules := []types.LifecycleRule{
		{
			ID:     aws.String("existing-rule"),
			Status: types.ExpirationStatusEnabled,
		},
	}
	
	mockS3 := &MockS3Client{
		lifecycleConfig: &types.BucketLifecycleConfiguration{
			Rules: expectedRules,
		},
	}
	manager := NewManager(mockS3, "test-bucket")
	
	ctx := context.Background()
	config, err := manager.GetCurrentPolicy(ctx)
	
	if err != nil {
		t.Errorf("GetCurrentPolicy() error = %v", err)
	}
	
	if config == nil {
		t.Fatalf("GetCurrentPolicy() returned nil config")
	}
	
	if len(config.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(config.Rules))
	}
	
	if *config.Rules[0].ID != "existing-rule" {
		t.Errorf("Rule ID = %v, want existing-rule", *config.Rules[0].ID)
	}
}

func TestManager_GetCurrentPolicy_Error(t *testing.T) {
	mockS3 := &MockS3Client{
		getError: fmt.Errorf("No lifecycle configuration"),
	}
	manager := NewManager(mockS3, "test-bucket")
	
	ctx := context.Background()
	_, err := manager.GetCurrentPolicy(ctx)
	
	if err == nil {
		t.Errorf("GetCurrentPolicy() should return error when S3 fails")
	}
}

func TestManager_RemovePolicy(t *testing.T) {
	mockS3 := &MockS3Client{}
	manager := NewManager(mockS3, "test-bucket")
	
	ctx := context.Background()
	err := manager.RemovePolicy(ctx)
	
	if err != nil {
		t.Errorf("RemovePolicy() error = %v", err)
	}
	
	if len(mockS3.deleteLifecycleCalls) != 1 {
		t.Errorf("Expected 1 DeleteBucketLifecycle call, got %d", len(mockS3.deleteLifecycleCalls))
	}
	
	call := mockS3.deleteLifecycleCalls[0]
	if *call.Bucket != "test-bucket" {
		t.Errorf("DeleteBucketLifecycle bucket = %v, want test-bucket", *call.Bucket)
	}
}

func TestManager_ValidatePolicy(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	tests := []struct {
		name     string
		template PolicyTemplate
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid policy",
			template: PolicyTemplate{
				ID: "valid-policy",
				Rules: []LifecycleRule{
					{
						ID:     "valid-rule",
						Status: "Enabled",
						Transitions: []Transition{
							{Days: 30, StorageClass: "STANDARD_IA"},
							{Days: 90, StorageClass: "GLACIER"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty policy ID",
			template: PolicyTemplate{
				ID: "",
				Rules: []LifecycleRule{
					{ID: "rule1", Status: "Enabled"},
				},
			},
			wantErr: true,
			errMsg:  "policy ID cannot be empty",
		},
		{
			name: "no rules",
			template: PolicyTemplate{
				ID:    "test-policy",
				Rules: []LifecycleRule{},
			},
			wantErr: true,
			errMsg:  "policy must have at least one rule",
		},
		{
			name: "invalid rule status",
			template: PolicyTemplate{
				ID: "test-policy",
				Rules: []LifecycleRule{
					{
						ID:     "invalid-rule",
						Status: "Invalid",
					},
				},
			},
			wantErr: true,
			errMsg:  "rule status must be 'Enabled' or 'Disabled'",
		},
		{
			name: "transitions not in chronological order",
			template: PolicyTemplate{
				ID: "test-policy",
				Rules: []LifecycleRule{
					{
						ID:     "rule-with-bad-transitions",
						Status: "Enabled",
						Transitions: []Transition{
							{Days: 90, StorageClass: "GLACIER"},
							{Days: 30, StorageClass: "STANDARD_IA"}, // Out of order
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "transitions must be in chronological order",
		},
		{
			name: "invalid storage class",
			template: PolicyTemplate{
				ID: "test-policy",
				Rules: []LifecycleRule{
					{
						ID:     "rule-with-invalid-class",
						Status: "Enabled",
						Transitions: []Transition{
							{Days: 30, StorageClass: "INVALID_CLASS"},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid storage class: INVALID_CLASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidatePolicy(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePolicy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidatePolicy() error = %v, want error containing %v", err, tt.errMsg)
			}
		})
	}
}

func TestManager_GenerateCustomPolicy(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	patterns := map[string]AccessPattern{
		"archives/frequent/": {
			Frequency:     "frequent",
			RetentionDays: 0,
			SizeGB:        100.0,
		},
		"archives/backup/": {
			Frequency:     "infrequent",
			RetentionDays: 2555, // 7 years
			SizeGB:        500.0,
		},
		"archives/cold/": {
			Frequency:     "archive",
			RetentionDays: 0,
			SizeGB:        1000.0,
		},
		"archives/unknown/": {
			Frequency:     "unknown",
			RetentionDays: 0,
			SizeGB:        50.0,
		},
	}
	
	policy := manager.GenerateCustomPolicy(patterns)
	
	if policy.ID != "cargoship-custom-generated" {
		t.Errorf("Generated policy ID = %v, want cargoship-custom-generated", policy.ID)
	}
	
	if len(policy.Rules) != len(patterns) {
		t.Errorf("Generated policy has %d rules, want %d", len(policy.Rules), len(patterns))
	}
	
	// Check that rules were generated for each pattern
	rulesByPrefix := make(map[string]LifecycleRule)
	for _, rule := range policy.Rules {
		rulesByPrefix[rule.Filter.Prefix] = rule
	}
	
	// Verify frequent access pattern
	frequentRule, exists := rulesByPrefix["archives/frequent/"]
	if !exists {
		t.Errorf("Missing rule for frequent access pattern")
	} else {
		if len(frequentRule.Transitions) != 2 {
			t.Errorf("Frequent access rule has %d transitions, want 2", len(frequentRule.Transitions))
		}
		if frequentRule.Expiration != nil {
			t.Errorf("Frequent access rule should not have expiration")
		}
	}
	
	// Verify backup pattern with retention
	backupRule, exists := rulesByPrefix["archives/backup/"]
	if !exists {
		t.Errorf("Missing rule for backup pattern")
	} else {
		if backupRule.Expiration == nil {
			t.Errorf("Backup rule should have expiration")
		} else if backupRule.Expiration.Days != 2555 {
			t.Errorf("Backup rule expiration = %d days, want 2555", backupRule.Expiration.Days)
		}
	}
	
	// Verify archive pattern
	archiveRule, exists := rulesByPrefix["archives/cold/"]
	if !exists {
		t.Errorf("Missing rule for archive pattern")
	} else {
		if len(archiveRule.Transitions) == 0 {
			t.Errorf("Archive rule should have transitions")
		} else {
			// Should start with GLACIER
			if archiveRule.Transitions[0].StorageClass != "GLACIER" {
				t.Errorf("Archive rule first transition = %s, want GLACIER", archiveRule.Transitions[0].StorageClass)
			}
		}
	}
	
	// Verify unknown pattern gets intelligent tiering
	unknownRule, exists := rulesByPrefix["archives/unknown/"]
	if !exists {
		t.Errorf("Missing rule for unknown pattern")
	} else {
		if len(unknownRule.Transitions) != 1 {
			t.Errorf("Unknown pattern rule has %d transitions, want 1", len(unknownRule.Transitions))
		} else if unknownRule.Transitions[0].StorageClass != "INTELLIGENT_TIERING" {
			t.Errorf("Unknown pattern transition = %s, want INTELLIGENT_TIERING", unknownRule.Transitions[0].StorageClass)
		}
	}
}

func TestManager_EstimateSavings(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	template := PolicyTemplate{
		ID:   "test-savings",
		Name: "Test Savings Policy",
		Rules: []LifecycleRule{
			{
				ID:     "savings-rule",
				Status: "Enabled",
				Transitions: []Transition{
					{Days: 30, StorageClass: "STANDARD_IA"},
					{Days: 90, StorageClass: "DEEP_ARCHIVE"},
				},
			},
		},
	}
	
	currentSizeGB := 1000.0
	ctx := context.Background()
	
	estimate, err := manager.EstimateSavings(ctx, template, currentSizeGB)
	if err != nil {
		t.Errorf("EstimateSavings() error = %v", err)
	}
	
	if estimate == nil {
		t.Fatalf("EstimateSavings() returned nil estimate")
	}
	
	if estimate.PolicyID != template.ID {
		t.Errorf("Estimate PolicyID = %v, want %v", estimate.PolicyID, template.ID)
	}
	
	if estimate.CurrentSizeGB != currentSizeGB {
		t.Errorf("Estimate CurrentSizeGB = %v, want %v", estimate.CurrentSizeGB, currentSizeGB)
	}
	
	// Should have calculated costs
	if estimate.CurrentMonthlyCost <= 0 {
		t.Errorf("CurrentMonthlyCost should be > 0, got %v", estimate.CurrentMonthlyCost)
	}
	
	if estimate.OptimizedMonthlyCost <= 0 {
		t.Errorf("OptimizedMonthlyCost should be > 0, got %v", estimate.OptimizedMonthlyCost)
	}
	
	// Optimized cost should be less than current (for DEEP_ARCHIVE)
	if estimate.OptimizedMonthlyCost >= estimate.CurrentMonthlyCost {
		t.Errorf("OptimizedMonthlyCost (%v) should be less than CurrentMonthlyCost (%v)", 
			estimate.OptimizedMonthlyCost, estimate.CurrentMonthlyCost)
	}
	
	// Savings should be positive
	if estimate.MonthlySavings <= 0 {
		t.Errorf("MonthlySavings should be > 0, got %v", estimate.MonthlySavings)
	}
	
	if estimate.SavingsPercent <= 0 {
		t.Errorf("SavingsPercent should be > 0, got %v", estimate.SavingsPercent)
	}
}

func TestManager_ExportPolicy(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	template := PolicyTemplate{
		ID:          "export-test",
		Name:        "Export Test Policy",
		Description: "Test policy for export",
		Rules: []LifecycleRule{
			{
				ID:     "export-rule",
				Status: "Enabled",
				Filter: RuleFilter{
					Prefix: "test/",
				},
				Transitions: []Transition{
					{Days: 30, StorageClass: "GLACIER"},
				},
			},
		},
		Savings: EstimatedSavings{
			MonthlyPercent: 50.0,
			AnnualUSD:      1000.0,
		},
	}
	
	jsonStr, err := manager.ExportPolicy(template)
	if err != nil {
		t.Errorf("ExportPolicy() error = %v", err)
	}
	
	if jsonStr == "" {
		t.Errorf("ExportPolicy() returned empty string")
	}
	
	// Verify it's valid JSON by unmarshaling
	var exported PolicyTemplate
	err = json.Unmarshal([]byte(jsonStr), &exported)
	if err != nil {
		t.Errorf("Exported JSON is invalid: %v", err)
	}
	
	if exported.ID != template.ID {
		t.Errorf("Exported policy ID = %v, want %v", exported.ID, template.ID)
	}
}

func TestManager_ImportPolicy(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	validJSON := `{
		"id": "imported-policy",
		"name": "Imported Policy",
		"description": "Test import",
		"rules": [
			{
				"id": "imported-rule",
				"status": "Enabled",
				"filter": {
					"prefix": "imported/"
				},
				"transitions": [
					{"days": 30, "storage_class": "GLACIER"}
				]
			}
		],
		"estimated_savings": {
			"monthly_percent": 40.0,
			"annual_usd": 800.0
		}
	}`
	
	template, err := manager.ImportPolicy(validJSON)
	if err != nil {
		t.Errorf("ImportPolicy() error = %v", err)
	}
	
	if template == nil {
		t.Fatalf("ImportPolicy() returned nil template")
	}
	
	if template.ID != "imported-policy" {
		t.Errorf("Imported policy ID = %v, want imported-policy", template.ID)
	}
	
	if len(template.Rules) != 1 {
		t.Errorf("Imported policy has %d rules, want 1", len(template.Rules))
	}
}

func TestManager_ImportPolicy_InvalidJSON(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	invalidJSON := `{"id": "test", "invalid": json}`
	
	_, err := manager.ImportPolicy(invalidJSON)
	if err == nil {
		t.Errorf("ImportPolicy() should return error for invalid JSON")
	}
}

func TestManager_ImportPolicy_InvalidPolicy(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	// Valid JSON but invalid policy (no rules)
	invalidPolicyJSON := `{
		"id": "invalid-policy",
		"name": "Invalid Policy",
		"rules": []
	}`
	
	_, err := manager.ImportPolicy(invalidPolicyJSON)
	if err == nil {
		t.Errorf("ImportPolicy() should return error for invalid policy")
	}
}

func TestStructFields(t *testing.T) {
	// Test PolicyTemplate
	policy := PolicyTemplate{
		ID:          "test-id",
		Name:        "Test Policy",
		Description: "Test description",
		Rules:       []LifecycleRule{},
		Savings: EstimatedSavings{
			MonthlyPercent: 50.0,
			AnnualUSD:      1000.0,
		},
	}
	
	if policy.ID != "test-id" {
		t.Errorf("PolicyTemplate.ID = %v, want test-id", policy.ID)
	}
	
	// Test LifecycleRule
	rule := LifecycleRule{
		ID:     "test-rule",
		Status: "Enabled",
		Filter: RuleFilter{
			Prefix: "test/",
			Tags:   map[string]string{"env": "prod"},
		},
		Transitions: []Transition{
			{Days: 30, StorageClass: "GLACIER"},
		},
		Expiration: &Expiration{Days: 365},
		AbortIncompleteUploads: aws.Int(7),
	}
	
	if rule.ID != "test-rule" {
		t.Errorf("LifecycleRule.ID = %v, want test-rule", rule.ID)
	}
	
	// Test AccessPattern
	pattern := AccessPattern{
		Frequency:     "frequent",
		RetentionDays: 365,
		SizeGB:        100.0,
	}
	
	if pattern.Frequency != "frequent" {
		t.Errorf("AccessPattern.Frequency = %v, want frequent", pattern.Frequency)
	}
	
	// Test SavingsEstimate
	estimate := SavingsEstimate{
		PolicyID:             "test-policy",
		PolicyName:           "Test Policy",
		CurrentSizeGB:        1000.0,
		CurrentMonthlyCost:   23.0,
		OptimizedMonthlyCost: 10.0,
		MonthlySavings:       13.0,
		AnnualSavings:        156.0,
		SavingsPercent:       56.5,
	}
	
	if estimate.PolicyID != "test-policy" {
		t.Errorf("SavingsEstimate.PolicyID = %v, want test-policy", estimate.PolicyID)
	}
}

func TestManager_convertToAWSRule(t *testing.T) {
	manager := NewManager(&MockS3Client{}, "test-bucket")
	
	tests := []struct {
		name string
		rule LifecycleRule
		want func(types.LifecycleRule) bool
	}{
		{
			name: "simple prefix filter",
			rule: LifecycleRule{
				ID:     "test-rule",
				Status: "Enabled",
				Filter: RuleFilter{
					Prefix: "archives/",
				},
				Transitions: []Transition{
					{Days: 30, StorageClass: "GLACIER"},
				},
			},
			want: func(awsRule types.LifecycleRule) bool {
				return *awsRule.ID == "test-rule" &&
					awsRule.Status == types.ExpirationStatusEnabled &&
					awsRule.Filter.Prefix != nil &&
					*awsRule.Filter.Prefix == "archives/" &&
					len(awsRule.Transitions) == 1
			},
		},
		{
			name: "complex filter with prefix and tags",
			rule: LifecycleRule{
				ID:     "complex-rule",
				Status: "Disabled",
				Filter: RuleFilter{
					Prefix: "data/",
					Tags:   map[string]string{"env": "prod", "type": "archive"},
				},
				Expiration: &Expiration{Days: 365},
			},
			want: func(awsRule types.LifecycleRule) bool {
				return *awsRule.ID == "complex-rule" &&
					awsRule.Status == types.ExpirationStatusDisabled &&
					awsRule.Filter.And != nil &&
					awsRule.Expiration != nil &&
					*awsRule.Expiration.Days == 365
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			awsRule, err := manager.convertToAWSRule(tt.rule)
			if err != nil {
				t.Errorf("convertToAWSRule() error = %v", err)
				return
			}
			
			if !tt.want(awsRule) {
				t.Errorf("convertToAWSRule() result validation failed")
			}
		})
	}
}