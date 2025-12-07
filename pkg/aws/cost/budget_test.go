package cost

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// TestSetProjectBudget tests setting project budgets with cost and volume quotas (Issue #147 Phase 4)
func TestSetProjectBudget(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		MaxMonthlyBudget: 10000.0,
		AlertThreshold:   0.8,
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets:          make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test setting project budget with both cost and volume
	err = manager.SetProjectBudget("project1", 1000.0, 500.0, 0.8, 0.75)
	if err != nil {
		t.Fatalf("Failed to set project budget: %v", err)
	}

	// Verify budget was set
	budgets := manager.ListProjectBudgets()
	if len(budgets) != 1 {
		t.Fatalf("Expected 1 budget, got %d", len(budgets))
	}

	budget, exists := cfg.ProjectBudgets["project1"]
	if !exists {
		t.Fatal("Project budget not found")
	}

	if budget.MaxBudget != 1000.0 {
		t.Errorf("Expected MaxBudget=1000.0, got %.2f", budget.MaxBudget)
	}
	if budget.MaxVolumeGB != 500.0 {
		t.Errorf("Expected MaxVolumeGB=500.0, got %.2f", budget.MaxVolumeGB)
	}
	if budget.AlertThreshold != 0.8 {
		t.Errorf("Expected AlertThreshold=0.8, got %.2f", budget.AlertThreshold)
	}
	if budget.VolumeAlertThreshold != 0.75 {
		t.Errorf("Expected VolumeAlertThreshold=0.75, got %.2f", budget.VolumeAlertThreshold)
	}
}

// TestSetProjectBudgetValidation tests budget validation (Issue #147 Phase 4)
func TestSetProjectBudgetValidation(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cfg := &config.CostControlConfig{
		ProjectBudgets: make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	tests := []struct {
		name          string
		projectID     string
		maxBudget     float64
		maxVolumeGB   float64
		costAlert     float64
		volumeAlert   float64
		expectError   bool
		errorContains string
	}{
		{
			name:          "Empty project ID",
			projectID:     "",
			maxBudget:     1000.0,
			maxVolumeGB:   500.0,
			costAlert:     0.8,
			volumeAlert:   0.75,
			expectError:   true,
			errorContains: "project ID cannot be empty",
		},
		{
			name:          "Negative budget",
			projectID:     "project1",
			maxBudget:     -100.0,
			maxVolumeGB:   500.0,
			costAlert:     0.8,
			volumeAlert:   0.75,
			expectError:   true,
			errorContains: "max budget cannot be negative",
		},
		{
			name:          "Negative volume",
			projectID:     "project1",
			maxBudget:     1000.0,
			maxVolumeGB:   -50.0,
			costAlert:     0.8,
			volumeAlert:   0.75,
			expectError:   true,
			errorContains: "max volume cannot be negative",
		},
		{
			name:          "Invalid cost alert threshold (> 1.0)",
			projectID:     "project1",
			maxBudget:     1000.0,
			maxVolumeGB:   500.0,
			costAlert:     1.5,
			volumeAlert:   0.75,
			expectError:   true,
			errorContains: "cost alert threshold must be between 0.0 and 1.0",
		},
		{
			name:          "Invalid cost alert threshold (< 0.0)",
			projectID:     "project1",
			maxBudget:     1000.0,
			maxVolumeGB:   500.0,
			costAlert:     -0.1,
			volumeAlert:   0.75,
			expectError:   true,
			errorContains: "cost alert threshold must be between 0.0 and 1.0",
		},
		{
			name:          "Invalid volume alert threshold (> 1.0)",
			projectID:     "project1",
			maxBudget:     1000.0,
			maxVolumeGB:   500.0,
			costAlert:     0.8,
			volumeAlert:   1.2,
			expectError:   true,
			errorContains: "volume alert threshold must be between 0.0 and 1.0",
		},
		{
			name:          "Invalid volume alert threshold (< 0.0)",
			projectID:     "project1",
			maxBudget:     1000.0,
			maxVolumeGB:   500.0,
			costAlert:     0.8,
			volumeAlert:   -0.5,
			expectError:   true,
			errorContains: "volume alert threshold must be between 0.0 and 1.0",
		},
		{
			name:        "Valid: zero budget (unlimited)",
			projectID:   "project1",
			maxBudget:   0,
			maxVolumeGB: 500.0,
			costAlert:   0.8,
			volumeAlert: 0.75,
			expectError: false,
		},
		{
			name:        "Valid: zero volume (unlimited)",
			projectID:   "project1",
			maxBudget:   1000.0,
			maxVolumeGB: 0,
			costAlert:   0.8,
			volumeAlert: 0.75,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.SetProjectBudget(tt.projectID, tt.maxBudget, tt.maxVolumeGB, tt.costAlert, tt.volumeAlert)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRemoveProjectBudget tests removing project budgets (Issue #147 Phase 4)
func TestRemoveProjectBudget(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cfg := &config.CostControlConfig{
		ProjectBudgets: make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Set a project budget
	err = manager.SetProjectBudget("project1", 1000.0, 500.0, 0.8, 0.75)
	if err != nil {
		t.Fatalf("Failed to set project budget: %v", err)
	}

	// Verify budget exists
	if len(cfg.ProjectBudgets) != 1 {
		t.Fatal("Budget was not set")
	}

	// Remove the budget
	err = manager.RemoveProjectBudget("project1")
	if err != nil {
		t.Fatalf("Failed to remove project budget: %v", err)
	}

	// Verify budget was removed
	if len(cfg.ProjectBudgets) != 0 {
		t.Fatalf("Expected 0 budgets, got %d", len(cfg.ProjectBudgets))
	}

	// Try to remove non-existent budget
	err = manager.RemoveProjectBudget("nonexistent")
	if err == nil {
		t.Fatal("Expected error removing non-existent budget")
	}
}

// TestListProjectBudgets tests listing all project budgets (Issue #147 Phase 4)
func TestListProjectBudgets(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cfg := &config.CostControlConfig{
		ProjectBudgets: make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Initially empty
	budgets := manager.ListProjectBudgets()
	if len(budgets) != 0 {
		t.Fatalf("Expected 0 budgets, got %d", len(budgets))
	}

	// Add multiple budgets
	projects := []string{"project1", "project2", "project3"}
	for i, projectID := range projects {
		budget := float64((i + 1) * 1000)
		volume := float64((i + 1) * 500)
		err := manager.SetProjectBudget(projectID, budget, volume, 0.8, 0.75)
		if err != nil {
			t.Fatalf("Failed to set budget for %s: %v", projectID, err)
		}
	}

	// List all budgets
	budgets = manager.ListProjectBudgets()
	if len(budgets) != 3 {
		t.Fatalf("Expected 3 budgets, got %d", len(budgets))
	}

	// Verify budgets
	for i, projectID := range projects {
		found := false
		for _, budget := range budgets {
			if budget.ProjectID == projectID {
				found = true
				expectedBudget := float64((i + 1) * 1000)
				expectedVolume := float64((i + 1) * 500)
				if budget.MaxBudget != expectedBudget {
					t.Errorf("Project %s: Expected budget=%.2f, got %.2f", projectID, expectedBudget, budget.MaxBudget)
				}
				if budget.MaxVolumeGB != expectedVolume {
					t.Errorf("Project %s: Expected volume=%.2f, got %.2f", projectID, expectedVolume, budget.MaxVolumeGB)
				}
				break
			}
		}
		if !found {
			t.Errorf("Budget for %s not found in list", projectID)
		}
	}
}

// TestCheckProjectBudgetEnforcement tests budget enforcement (Issue #147 Phase 4)
func TestCheckProjectBudgetEnforcement(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		MaxMonthlyBudget: 10000.0,
		AlertThreshold:   0.8,
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets:          make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Set project budget: $100, 50GB
	err = manager.SetProjectBudget("project1", 100.0, 50.0, 0.8, 0.75)
	if err != nil {
		t.Fatalf("Failed to set project budget: %v", err)
	}

	// Test cost budget enforcement - under budget
	err = manager.CheckProjectBudget("project1", 50.0)
	if err != nil {
		t.Errorf("Unexpected error for operation under budget: %v", err)
	}

	// Test cost budget enforcement - would exceed
	err = manager.CheckProjectBudget("project1", 101.0)
	if err == nil {
		t.Error("Expected error for operation exceeding budget")
	}
	if _, ok := err.(*BudgetExceededError); !ok {
		t.Errorf("Expected BudgetExceededError, got %T", err)
	}

	// Test volume quota enforcement - under quota
	err = manager.CheckProjectVolumeQuota("project1", 30.0)
	if err != nil {
		t.Errorf("Unexpected error for operation under quota: %v", err)
	}

	// Test volume quota enforcement - would exceed
	err = manager.CheckProjectVolumeQuota("project1", 51.0)
	if err == nil {
		t.Error("Expected error for operation exceeding quota")
	}
	if _, ok := err.(*VolumeQuotaExceededError); !ok {
		t.Errorf("Expected VolumeQuotaExceededError, got %T", err)
	}
}

// TestGetProjectBudgetStatus tests getting project budget status (Issue #147 Phase 4)
func TestGetProjectBudgetStatus(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		MaxMonthlyBudget: 10000.0,
		AlertThreshold:   0.8,
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets:          make(map[string]config.ProjectBudget),
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Set project budget
	err = manager.SetProjectBudget("project1", 1000.0, 500.0, 0.8, 0.75)
	if err != nil {
		t.Fatalf("Failed to set project budget: %v", err)
	}

	// Record some costs
	reporter := manager.GetReporter()
	for i := 0; i < 10; i++ {
		reporter.RecordCost(CostRecord{
			ProjectID: "project1",
			Cost:      10.0,
			SizeGB:    5.0,
			Timestamp: now.Add(time.Duration(-i) * time.Hour),
			Currency:  "USD",
		})
	}

	// Get status
	status, err := manager.GetProjectBudgetStatus("project1")
	if err != nil {
		t.Fatalf("Failed to get project budget status: %v", err)
	}

	// Verify status fields
	if status == nil {
		t.Fatal("Status is nil")
	}
	if status.ProjectID != "project1" {
		t.Errorf("Expected ProjectID=project1, got %s", status.ProjectID)
	}
	if status.MaxBudget != 1000.0 {
		t.Errorf("Expected MaxBudget=1000.0, got %.2f", status.MaxBudget)
	}
	if status.MaxVolumeGB != 500.0 {
		t.Errorf("Expected MaxVolumeGB=500.0, got %.2f", status.MaxVolumeGB)
	}
	if status.CurrentSpend != 100.0 {
		t.Errorf("Expected CurrentSpend=100.0 (10 records * $10), got %.2f", status.CurrentSpend)
	}
	if status.CurrentVolumeGB != 50.0 {
		t.Errorf("Expected CurrentVolumeGB=50.0 (10 records * 5GB), got %.2f", status.CurrentVolumeGB)
	}

	// Check calculations
	expectedBudgetUsed := 100.0 / 1000.0 // 0.1 or 10%
	if status.BudgetUsed != expectedBudgetUsed {
		t.Errorf("Expected BudgetUsed=%.2f, got %.2f", expectedBudgetUsed, status.BudgetUsed)
	}

	expectedVolumeUsed := 50.0 / 500.0 // 0.1 or 10%
	if status.VolumeUsed != expectedVolumeUsed {
		t.Errorf("Expected VolumeUsed=%.2f, got %.2f", expectedVolumeUsed, status.VolumeUsed)
	}

	// Should not be over budget or over quota
	if status.OverBudget {
		t.Error("Status incorrectly shows over budget")
	}
	if status.OverVolume {
		t.Error("Status incorrectly shows over volume quota")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestRecordOperationCostWithBudgetEnforcement tests full integration (Issue #147 Phase 4)
func TestRecordOperationCostWithBudgetEnforcement(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	ctx := context.Background()
	now := time.Now()

	cfg := &config.CostControlConfig{
		MaxMonthlyBudget: 10000.0,
		AlertThreshold:   0.8,
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets:          make(map[string]config.ProjectBudget),
	}

	manager, err := NewManager(cfg, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Set project budget: $100, 10GB
	err = manager.SetProjectBudget("project1", 100.0, 10.0, 0.8, 0.75)
	if err != nil {
		t.Fatalf("Failed to set project budget: %v", err)
	}

	// Test 1: Record operation under both limits - should succeed
	err = manager.RecordOperationCost(
		ctx,
		"upload",
		"file1.txt",
		1024*1024*1024, // 1GB
		config.StorageClassStandard,
		"us-west-2",
		"job1",
		"project1",
		nil,
	)
	if err != nil {
		t.Errorf("Unexpected error for operation under limits: %v", err)
	}

	// Test 2: Record operation that would exceed volume quota - should fail
	err = manager.RecordOperationCost(
		ctx,
		"upload",
		"largefile.txt",
		11*1024*1024*1024, // 11GB (exceeds 10GB quota)
		config.StorageClassStandard,
		"us-west-2",
		"job2",
		"project1",
		nil,
	)
	if err == nil {
		t.Error("Expected error for operation exceeding volume quota")
	}
	if !contains(err.Error(), "volume quota enforcement") {
		t.Errorf("Expected volume quota error, got: %v", err)
	}

	// Test 3: Operation without project ID should use global limits
	err = manager.RecordOperationCost(
		ctx,
		"upload",
		"file3.txt",
		1024*1024*1024, // 1GB
		config.StorageClassStandard,
		"us-west-2",
		"job3",
		"", // No project ID
		nil,
	)
	// This should succeed as global limits are much higher
	if err != nil && !contains(err.Error(), "cost estimation") {
		t.Errorf("Unexpected error for operation with no project ID: %v", err)
	}
}
