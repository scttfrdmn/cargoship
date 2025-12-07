// Package cost provides budget enforcement functionality
package cost

import (
	"fmt"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// BudgetStatus represents the current status of a budget
type BudgetStatus struct {
	// Budget identification
	ProjectID   string `json:"project_id,omitempty"`   // Empty for global budget
	BudgetType  string `json:"budget_type"`            // "global" or "project"
	PeriodType  string `json:"period_type"`            // e.g., "monthly", "quarterly", "grant"
	PeriodStart string `json:"period_start"`           // ISO 8601 date
	PeriodEnd   string `json:"period_end"`             // ISO 8601 date

	// Budget limits and usage
	MaxBudget       float64 `json:"max_budget"`
	CurrentSpend    float64 `json:"current_spend"`
	BudgetRemaining float64 `json:"budget_remaining"`
	BudgetUsed      float64 `json:"budget_used"`           // Percentage (0.0-1.0)
	AlertThreshold  float64 `json:"alert_threshold"`       // Percentage (0.0-1.0)

	// Time tracking
	DaysElapsed   int `json:"days_elapsed"`
	DaysRemaining int `json:"days_remaining"`
	TotalDays     int `json:"total_days"`

	// Burn rate and projections
	DailyBurnRate      float64 `json:"daily_burn_rate"`
	ProjectedEOPSpend  float64 `json:"projected_eop_spend"`  // End of period
	WillExceedBudget   bool    `json:"will_exceed_budget"`
	TargetDailyRate    float64 `json:"target_daily_rate"`    // To stay within budget

	// Status flags
	OverBudget      bool   `json:"over_budget"`
	AlertTriggered  bool   `json:"alert_triggered"`
	Currency        string `json:"currency"`

	// Volume quota tracking (if enabled)
	MaxVolumeGB          float64 `json:"max_volume_gb,omitempty"`           // 0 = unlimited
	CurrentVolumeGB      float64 `json:"current_volume_gb,omitempty"`
	VolumeRemaining      float64 `json:"volume_remaining,omitempty"`
	VolumeUsed           float64 `json:"volume_used,omitempty"`              // Percentage (0.0-1.0)
	VolumeAlertThreshold float64 `json:"volume_alert_threshold,omitempty"`   // Percentage (0.0-1.0)
	DailyVolumeBurnRate  float64 `json:"daily_volume_burn_rate,omitempty"`   // GB/day
	ProjectedEOPVolume   float64 `json:"projected_eop_volume,omitempty"`     // GB
	WillExceedVolume     bool    `json:"will_exceed_volume,omitempty"`
	OverVolume           bool    `json:"over_volume,omitempty"`
	VolumeAlertTriggered bool    `json:"volume_alert_triggered,omitempty"`

	// Optional grant information
	GrantName      string `json:"grant_name,omitempty"`
	EnableRollover bool   `json:"enable_rollover,omitempty"`
}

// BudgetExceededError represents an error when budget would be exceeded
type BudgetExceededError struct {
	ProjectID       string
	BudgetType      string  // "global" or "project"
	MaxBudget       float64
	CurrentSpend    float64
	AdditionalCost  float64
	ProjectedSpend  float64
	Overage         float64
	Currency        string
}

func (e *BudgetExceededError) Error() string {
	if e.ProjectID != "" {
		return fmt.Sprintf(
			"project budget exceeded: project=%s, max_budget=%.2f %s, current_spend=%.2f %s, "+
			"additional_cost=%.2f %s, projected_spend=%.2f %s, overage=%.2f %s",
			e.ProjectID, e.MaxBudget, e.Currency, e.CurrentSpend, e.Currency,
			e.AdditionalCost, e.Currency, e.ProjectedSpend, e.Currency, e.Overage, e.Currency,
		)
	}
	return fmt.Sprintf(
		"global budget exceeded: max_budget=%.2f %s, current_spend=%.2f %s, "+
		"additional_cost=%.2f %s, projected_spend=%.2f %s, overage=%.2f %s",
		e.MaxBudget, e.Currency, e.CurrentSpend, e.Currency,
		e.AdditionalCost, e.Currency, e.ProjectedSpend, e.Currency, e.Overage, e.Currency,
	)
}

// VolumeQuotaExceededError represents an error when volume quota would be exceeded
type VolumeQuotaExceededError struct {
	ProjectID        string
	QuotaType        string  // "global" or "project"
	MaxVolumeGB      float64
	CurrentVolumeGB  float64
	AdditionalGB     float64
	ProjectedVolume  float64
	Overage          float64
}

func (e *VolumeQuotaExceededError) Error() string {
	if e.ProjectID != "" {
		return fmt.Sprintf(
			"project volume quota exceeded: project=%s, max_volume=%.2f GB, current_volume=%.2f GB, "+
			"additional_volume=%.2f GB, projected_volume=%.2f GB, overage=%.2f GB",
			e.ProjectID, e.MaxVolumeGB, e.CurrentVolumeGB,
			e.AdditionalGB, e.ProjectedVolume, e.Overage,
		)
	}
	return fmt.Sprintf(
		"global volume quota exceeded: max_volume=%.2f GB, current_volume=%.2f GB, "+
		"additional_volume=%.2f GB, projected_volume=%.2f GB, overage=%.2f GB",
		e.MaxVolumeGB, e.CurrentVolumeGB,
		e.AdditionalGB, e.ProjectedVolume, e.Overage,
	)
}

// CheckProjectBudget checks if a project would exceed its budget
// Returns nil if within budget, BudgetExceededError otherwise
func (m *Manager) CheckProjectBudget(projectID string, additionalCost float64) error {
	// Check if project has a specific budget configured
	projectBudget, hasProjectBudget := m.config.ProjectBudgets[projectID]

	if !hasProjectBudget {
		// No project-specific budget, check against global budget
		return m.checkGlobalBudget(additionalCost)
	}

	// Get current project spending
	currentSpend := m.reporter.GetProjectCosts(projectID)
	projectedSpend := currentSpend + additionalCost

	// Check if projected spend would exceed project budget
	if projectedSpend > projectBudget.MaxBudget {
		overage := projectedSpend - projectBudget.MaxBudget
		return &BudgetExceededError{
			ProjectID:      projectID,
			BudgetType:     "project",
			MaxBudget:      projectBudget.MaxBudget,
			CurrentSpend:   currentSpend,
			AdditionalCost: additionalCost,
			ProjectedSpend: projectedSpend,
			Overage:        overage,
			Currency:       m.config.Pricing.Currency,
		}
	}

	// Project budget check passed, also check global budget
	return m.checkGlobalBudget(additionalCost)
}

// checkGlobalBudget checks if an operation would exceed the global budget
func (m *Manager) checkGlobalBudget(additionalCost float64) error {
	// Check if budget periods are configured (new system)
	if len(m.config.BudgetPeriods) > 0 {
		return m.checkBudgetPeriod(additionalCost)
	}

	// Fall back to legacy monthly budget system
	currentSpend := m.GetCurrentMonthSpend()
	projectedSpend := currentSpend + additionalCost

	if projectedSpend > m.config.MaxMonthlyBudget {
		overage := projectedSpend - m.config.MaxMonthlyBudget
		return &BudgetExceededError{
			BudgetType:     "global",
			MaxBudget:      m.config.MaxMonthlyBudget,
			CurrentSpend:   currentSpend,
			AdditionalCost: additionalCost,
			ProjectedSpend: projectedSpend,
			Overage:        overage,
			Currency:       m.config.Pricing.Currency,
		}
	}

	return nil
}

// checkBudgetPeriod checks against the active budget period
func (m *Manager) checkBudgetPeriod(additionalCost float64) error {
	activeIndex := m.config.ActiveBudgetPeriodIndex
	if activeIndex < 0 || activeIndex >= len(m.config.BudgetPeriods) {
		activeIndex = 0
	}

	budgetPeriod := m.config.BudgetPeriods[activeIndex]
	currentSpend := m.GetCurrentMonthSpend() // TODO: Should filter by period, not just month
	projectedSpend := currentSpend + additionalCost

	if projectedSpend > budgetPeriod.MaxBudget {
		overage := projectedSpend - budgetPeriod.MaxBudget
		return &BudgetExceededError{
			BudgetType:     "global",
			MaxBudget:      budgetPeriod.MaxBudget,
			CurrentSpend:   currentSpend,
			AdditionalCost: additionalCost,
			ProjectedSpend: projectedSpend,
			Overage:        overage,
			Currency:       m.config.Pricing.Currency,
		}
	}

	return nil
}

// GetProjectBudgetStatus returns the current budget status for a project
func (m *Manager) GetProjectBudgetStatus(projectID string) (*BudgetStatus, error) {
	// Check if project has a specific budget configured
	projectBudget, hasProjectBudget := m.config.ProjectBudgets[projectID]

	if !hasProjectBudget {
		return nil, fmt.Errorf("no budget configured for project: %s", projectID)
	}

	// Get current project spending
	currentSpend := m.reporter.GetProjectCosts(projectID)
	now := time.Now()

	// Determine which budget period to use
	var budgetPeriod *config.BudgetPeriod
	if projectBudget.BudgetPeriod != nil {
		// Project has its own budget period
		budgetPeriod = projectBudget.BudgetPeriod
	} else if len(m.config.BudgetPeriods) > 0 {
		// Inherit from global active budget period
		activeIndex := m.config.ActiveBudgetPeriodIndex
		if activeIndex < 0 || activeIndex >= len(m.config.BudgetPeriods) {
			activeIndex = 0
		}
		budgetPeriod = &m.config.BudgetPeriods[activeIndex]
	} else {
		// No budget period available, create a default monthly period
		budgetPeriod = &config.BudgetPeriod{
			Type:           "monthly",
			MaxBudget:      projectBudget.MaxBudget,
			AlertThreshold: projectBudget.AlertThreshold,
		}
	}

	// Get period bounds
	start, end, err := budgetPeriod.GetPeriodBounds(now)
	if err != nil {
		return nil, fmt.Errorf("failed to get budget period bounds: %w", err)
	}

	daysElapsed, _ := budgetPeriod.GetDaysElapsed(now)
	daysRemaining, _ := budgetPeriod.GetDaysRemaining(now)
	totalDays, _ := budgetPeriod.GetTotalDays(now)
	burnRate, _ := budgetPeriod.CalculateBurnRate(currentSpend, now)
	projectedSpend, _ := budgetPeriod.ProjectEndOfPeriodSpend(currentSpend, now)
	willExceed, _, _ := budgetPeriod.WillExceedBudget(currentSpend, now)

	budgetUsed := currentSpend / projectBudget.MaxBudget
	remaining := projectBudget.MaxBudget - currentSpend

	// Calculate target daily rate to stay within budget
	targetDailyRate := 0.0
	if daysRemaining > 0 {
		targetDailyRate = remaining / float64(daysRemaining)
	}

	// Determine alert threshold (project-specific or period default)
	alertThreshold := projectBudget.AlertThreshold
	if alertThreshold == 0 {
		alertThreshold = budgetPeriod.AlertThreshold
	}

	status := &BudgetStatus{
		ProjectID:   projectID,
		BudgetType:  "project",
		PeriodType:  string(budgetPeriod.Type),
		PeriodStart: start.Format("2006-01-02"),
		PeriodEnd:   end.Format("2006-01-02"),

		MaxBudget:       projectBudget.MaxBudget,
		CurrentSpend:    currentSpend,
		BudgetRemaining: remaining,
		BudgetUsed:      budgetUsed,
		AlertThreshold:  alertThreshold,
		Currency:        m.config.Pricing.Currency,

		DaysElapsed:   daysElapsed,
		DaysRemaining: daysRemaining,
		TotalDays:     totalDays,

		DailyBurnRate:     burnRate,
		ProjectedEOPSpend: projectedSpend,
		WillExceedBudget:  willExceed,
		TargetDailyRate:   targetDailyRate,

		OverBudget:     budgetUsed > 1.0,
		AlertTriggered: budgetUsed > alertThreshold,

		GrantName:      budgetPeriod.GrantName,
		EnableRollover: budgetPeriod.EnableRollover,
	}

	return status, nil
}

// GetGlobalBudgetStatus returns the current global budget status
// This is a convenience wrapper around the existing GetBudgetStatus method
func (m *Manager) GetGlobalBudgetStatus() *BudgetStatus {
	legacyStatus := m.GetBudgetStatus()

	// Convert legacy map to BudgetStatus struct
	status := &BudgetStatus{
		BudgetType: "global",
		Currency:   m.config.Pricing.Currency,
	}

	// Extract values from map
	if v, ok := legacyStatus["period_type"].(string); ok {
		status.PeriodType = v
	}
	if v, ok := legacyStatus["period_start"].(string); ok {
		status.PeriodStart = v
	}
	if v, ok := legacyStatus["period_end"].(string); ok {
		status.PeriodEnd = v
	}
	if v, ok := legacyStatus["max_budget"].(float64); ok {
		status.MaxBudget = v
	}
	if v, ok := legacyStatus["current_spend"].(float64); ok {
		status.CurrentSpend = v
	}
	if v, ok := legacyStatus["budget_remaining"].(float64); ok {
		status.BudgetRemaining = v
	}
	if v, ok := legacyStatus["budget_used"].(float64); ok {
		status.BudgetUsed = v
	}
	if v, ok := legacyStatus["alert_threshold"].(float64); ok {
		status.AlertThreshold = v
	}
	if v, ok := legacyStatus["days_elapsed"].(int); ok {
		status.DaysElapsed = v
	}
	if v, ok := legacyStatus["days_remaining"].(int); ok {
		status.DaysRemaining = v
	}
	if v, ok := legacyStatus["total_days"].(int); ok {
		status.TotalDays = v
	}
	if v, ok := legacyStatus["daily_burn_rate"].(float64); ok {
		status.DailyBurnRate = v
	}
	if v, ok := legacyStatus["projected_eop_spend"].(float64); ok {
		status.ProjectedEOPSpend = v
	}
	if v, ok := legacyStatus["will_exceed_budget"].(bool); ok {
		status.WillExceedBudget = v
	}
	if v, ok := legacyStatus["target_daily_rate"].(float64); ok {
		status.TargetDailyRate = v
	}
	if v, ok := legacyStatus["over_budget"].(bool); ok {
		status.OverBudget = v
	}
	if v, ok := legacyStatus["alert_triggered"].(bool); ok {
		status.AlertTriggered = v
	}
	if v, ok := legacyStatus["grant_name"].(string); ok {
		status.GrantName = v
	}
	if v, ok := legacyStatus["enable_rollover"].(bool); ok {
		status.EnableRollover = v
	}

	return status
}

// ListProjectBudgets returns all configured project budgets
func (m *Manager) ListProjectBudgets() []config.ProjectBudget {
	budgets := make([]config.ProjectBudget, 0, len(m.config.ProjectBudgets))
	for _, budget := range m.config.ProjectBudgets {
		budgets = append(budgets, budget)
	}
	return budgets
}

// SetProjectBudget creates or updates a project budget
func (m *Manager) SetProjectBudget(projectID string, maxBudget float64, alertThreshold float64, description string) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}
	if maxBudget <= 0 {
		return fmt.Errorf("max budget must be greater than 0")
	}
	if alertThreshold < 0 || alertThreshold > 1 {
		return fmt.Errorf("alert threshold must be between 0.0 and 1.0")
	}

	if m.config.ProjectBudgets == nil {
		m.config.ProjectBudgets = make(map[string]config.ProjectBudget)
	}

	m.config.ProjectBudgets[projectID] = config.ProjectBudget{
		ProjectID:      projectID,
		MaxBudget:      maxBudget,
		AlertThreshold: alertThreshold,
		Description:    description,
	}

	m.logger.Info("Project budget updated",
		"project_id", projectID,
		"max_budget", maxBudget,
		"alert_threshold", alertThreshold)

	return nil
}

// DeleteProjectBudget removes a project budget
func (m *Manager) DeleteProjectBudget(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	if _, exists := m.config.ProjectBudgets[projectID]; !exists {
		return fmt.Errorf("no budget found for project: %s", projectID)
	}

	delete(m.config.ProjectBudgets, projectID)

	m.logger.Info("Project budget deleted", "project_id", projectID)
	return nil
}
