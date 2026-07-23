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
	ProjectID   string `json:"project_id,omitempty"` // Empty for global budget
	BudgetType  string `json:"budget_type"`          // "global" or "project"
	PeriodType  string `json:"period_type"`          // e.g., "monthly", "quarterly", "grant"
	PeriodStart string `json:"period_start"`         // ISO 8601 date
	PeriodEnd   string `json:"period_end"`           // ISO 8601 date

	// Budget limits and usage
	MaxBudget       float64 `json:"max_budget"`
	CurrentSpend    float64 `json:"current_spend"`
	BudgetRemaining float64 `json:"budget_remaining"`
	BudgetUsed      float64 `json:"budget_used"`     // Percentage (0.0-1.0)
	AlertThreshold  float64 `json:"alert_threshold"` // Percentage (0.0-1.0)

	// Time tracking
	DaysElapsed   int `json:"days_elapsed"`
	DaysRemaining int `json:"days_remaining"`
	TotalDays     int `json:"total_days"`

	// Burn rate and projections
	DailyBurnRate     float64 `json:"daily_burn_rate"`
	ProjectedEOPSpend float64 `json:"projected_eop_spend"` // End of period
	WillExceedBudget  bool    `json:"will_exceed_budget"`
	TargetDailyRate   float64 `json:"target_daily_rate"` // To stay within budget

	// Status flags
	OverBudget     bool   `json:"over_budget"`
	AlertTriggered bool   `json:"alert_triggered"`
	Currency       string `json:"currency"`

	// Volume quota tracking (if enabled)
	MaxVolumeGB          float64 `json:"max_volume_gb,omitempty"` // 0 = unlimited
	CurrentVolumeGB      float64 `json:"current_volume_gb,omitempty"`
	VolumeRemaining      float64 `json:"volume_remaining,omitempty"`
	VolumeUsed           float64 `json:"volume_used,omitempty"`            // Percentage (0.0-1.0)
	VolumeAlertThreshold float64 `json:"volume_alert_threshold,omitempty"` // Percentage (0.0-1.0)
	DailyVolumeBurnRate  float64 `json:"daily_volume_burn_rate,omitempty"` // GB/day
	ProjectedEOPVolume   float64 `json:"projected_eop_volume,omitempty"`   // GB
	WillExceedVolume     bool    `json:"will_exceed_volume,omitempty"`
	OverVolume           bool    `json:"over_volume,omitempty"`
	VolumeAlertTriggered bool    `json:"volume_alert_triggered,omitempty"`

	// Optional grant information
	GrantName      string `json:"grant_name,omitempty"`
	EnableRollover bool   `json:"enable_rollover,omitempty"`
}

// BudgetExceededError represents an error when budget would be exceeded
type BudgetExceededError struct {
	ProjectID      string
	BudgetType     string // "global" or "project"
	MaxBudget      float64
	CurrentSpend   float64
	AdditionalCost float64
	ProjectedSpend float64
	Overage        float64
	Currency       string
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
	ProjectID       string
	QuotaType       string // "global" or "project"
	MaxVolumeGB     float64
	CurrentVolumeGB float64
	AdditionalGB    float64
	ProjectedVolume float64
	Overage         float64
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

// CheckProjectBudget checks if a project would exceed its budget OR volume quota
// Returns nil if within limits, BudgetExceededError or VolumeQuotaExceededError otherwise
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

// CheckProjectVolumeQuota checks if a project would exceed its volume quota
// Returns nil if within quota, VolumeQuotaExceededError otherwise
func (m *Manager) CheckProjectVolumeQuota(projectID string, additionalGB float64) error {
	// Check if project has a specific volume quota configured
	projectBudget, hasProjectBudget := m.config.ProjectBudgets[projectID]

	if !hasProjectBudget || projectBudget.MaxVolumeGB == 0 {
		// No project-specific volume quota, check against global quota
		return m.checkGlobalVolumeQuota(additionalGB)
	}

	// Get current project volume usage
	currentVolume := m.reporter.GetProjectVolume(projectID)
	projectedVolume := currentVolume + additionalGB

	// Check if projected volume would exceed project quota
	if projectedVolume > projectBudget.MaxVolumeGB {
		overage := projectedVolume - projectBudget.MaxVolumeGB
		return &VolumeQuotaExceededError{
			ProjectID:       projectID,
			QuotaType:       "project",
			MaxVolumeGB:     projectBudget.MaxVolumeGB,
			CurrentVolumeGB: currentVolume,
			AdditionalGB:    additionalGB,
			ProjectedVolume: projectedVolume,
			Overage:         overage,
		}
	}

	// Project quota check passed, also check global quota
	return m.checkGlobalVolumeQuota(additionalGB)
}

// checkGlobalVolumeQuota checks if an operation would exceed the global volume quota
func (m *Manager) checkGlobalVolumeQuota(additionalGB float64) error {
	// Persisted global/team volume ceiling across all projects (#246 PR2),
	// enforced in addition to any budget-period quota.
	if err := m.checkGlobalVolumeCeiling(additionalGB); err != nil {
		return err
	}

	// Check if budget periods are configured (new system)
	if len(m.config.BudgetPeriods) > 0 {
		return m.checkBudgetPeriodVolume(additionalGB)
	}

	// No global volume quota configured (legacy system has no volume quotas)
	return nil
}

// checkBudgetPeriodVolume checks against the active budget period's volume quota
func (m *Manager) checkBudgetPeriodVolume(additionalGB float64) error {
	activeIndex := m.config.ActiveBudgetPeriodIndex
	if activeIndex < 0 || activeIndex >= len(m.config.BudgetPeriods) {
		activeIndex = 0
	}

	budgetPeriod := m.config.BudgetPeriods[activeIndex]

	// If MaxVolumeGB is 0, volume quota is unlimited
	if budgetPeriod.MaxVolumeGB == 0 {
		return nil
	}

	// Get period bounds and current volume
	now := time.Now()
	start, end, err := budgetPeriod.GetPeriodBounds(now)
	if err != nil {
		m.logger.Warn("Failed to get budget period bounds for volume check", "error", err)
		return nil // Don't block on error
	}

	currentVolume := m.reporter.GetCurrentPeriodVolume(start, end)
	projectedVolume := currentVolume + additionalGB

	if projectedVolume > budgetPeriod.MaxVolumeGB {
		overage := projectedVolume - budgetPeriod.MaxVolumeGB
		return &VolumeQuotaExceededError{
			QuotaType:       "global",
			MaxVolumeGB:     budgetPeriod.MaxVolumeGB,
			CurrentVolumeGB: currentVolume,
			AdditionalGB:    additionalGB,
			ProjectedVolume: projectedVolume,
			Overage:         overage,
		}
	}

	return nil
}

// checkGlobalBudget checks if an operation would exceed the global budget
func (m *Manager) checkGlobalBudget(additionalCost float64) error {
	// Persisted global/team cost ceiling across all projects (#246 PR2),
	// enforced in addition to any budget-period cap.
	if err := m.checkGlobalBudgetCeiling(additionalCost); err != nil {
		return err
	}

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

	// Get current project spending and volume
	currentSpend := m.reporter.GetProjectCosts(projectID)

	// Calculate current volume for project. A budget can be set before any spend
	// is recorded (fresh `budget set`), in which case there are no cost records
	// yet — treat that as zero volume rather than an error. (#241)
	var currentVolumeGB float64
	if projectSummary, err := m.reporter.GetProjectSummary(projectID); err == nil {
		currentVolumeGB = projectSummary.TotalSizeGB
	}

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

	// Calculate volume metrics
	volumeUsed := 0.0
	volumeRemaining := 0.0
	dailyVolumeBurnRate := 0.0
	projectedEOPVolume := 0.0
	volumeAlertTriggered := false
	overVolume := false
	willExceedVolume := false

	if projectBudget.MaxVolumeGB > 0 {
		volumeUsed = currentVolumeGB / projectBudget.MaxVolumeGB
		volumeRemaining = projectBudget.MaxVolumeGB - currentVolumeGB
		overVolume = volumeUsed > 1.0
		volumeAlertTriggered = volumeUsed > projectBudget.VolumeAlertThreshold

		// Calculate daily volume burn rate
		if daysElapsed > 0 {
			dailyVolumeBurnRate = currentVolumeGB / float64(daysElapsed)
		}

		// Project end-of-period volume
		if totalDays > 0 {
			projectedEOPVolume = (currentVolumeGB / float64(daysElapsed)) * float64(totalDays)
			willExceedVolume = projectedEOPVolume > projectBudget.MaxVolumeGB
		}
	}

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

		// Volume quota fields
		MaxVolumeGB:          projectBudget.MaxVolumeGB,
		CurrentVolumeGB:      currentVolumeGB,
		VolumeRemaining:      volumeRemaining,
		VolumeUsed:           volumeUsed,
		DailyVolumeBurnRate:  dailyVolumeBurnRate,
		ProjectedEOPVolume:   projectedEOPVolume,
		OverVolume:           overVolume,
		VolumeAlertTriggered: volumeAlertTriggered,
		WillExceedVolume:     willExceedVolume,

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

// SetProjectBudget creates or updates a project budget with cost and volume quota support
func (m *Manager) SetProjectBudget(projectID string, maxBudget float64, maxVolumeGB float64, costAlertThreshold float64, volumeAlertThreshold float64) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}
	// The project ID becomes part of an S3 object key when using the S3-backed
	// store (#246); reject separators and control characters so it can't inject
	// a key prefix or a malformed key.
	if err := validateProjectID(projectID); err != nil {
		return err
	}
	if maxBudget < 0 {
		return fmt.Errorf("max budget cannot be negative (use 0 for unlimited)")
	}
	if maxVolumeGB < 0 {
		return fmt.Errorf("max volume cannot be negative (use 0 for unlimited)")
	}
	if costAlertThreshold < 0 || costAlertThreshold > 1 {
		return fmt.Errorf("cost alert threshold must be between 0.0 and 1.0")
	}
	if volumeAlertThreshold < 0 || volumeAlertThreshold > 1 {
		return fmt.Errorf("volume alert threshold must be between 0.0 and 1.0")
	}

	if m.config.ProjectBudgets == nil {
		m.config.ProjectBudgets = make(map[string]config.ProjectBudget)
	}

	m.config.ProjectBudgets[projectID] = config.ProjectBudget{
		ProjectID:            projectID,
		MaxBudget:            maxBudget,
		MaxVolumeGB:          maxVolumeGB,
		AlertThreshold:       costAlertThreshold,
		VolumeAlertThreshold: volumeAlertThreshold,
	}

	// Persist so the budget survives across CLI invocations (#241), carrying the
	// recorded ledger through untouched so a limits update can't clobber it (#246).
	if err := m.saveState(); err != nil {
		return fmt.Errorf("persist project budget: %w", err)
	}

	m.logger.Info("Project budget updated",
		"project_id", projectID,
		"max_budget", maxBudget,
		"max_volume_gb", maxVolumeGB,
		"cost_alert_threshold", costAlertThreshold,
		"volume_alert_threshold", volumeAlertThreshold)

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

	// Persist the removal (#241), preserving the recorded ledger (#246).
	if err := m.saveState(); err != nil {
		return fmt.Errorf("persist project budget removal: %w", err)
	}

	m.logger.Info("Project budget deleted", "project_id", projectID)
	return nil
}

// RemoveProjectBudget is an alias for DeleteProjectBudget for CLI compatibility
func (m *Manager) RemoveProjectBudget(projectID string) error {
	return m.DeleteProjectBudget(projectID)
}

// SetGlobalBudget creates or updates the persisted org/team-wide budget ceiling
// (#246 Phase B PR2). It caps aggregate spend/volume across ALL projects and is
// enforced in addition to any per-project cap. maxBudget/maxVolumeGB of 0 mean
// unlimited for that dimension.
func (m *Manager) SetGlobalBudget(maxBudget, maxVolumeGB, costAlertThreshold, volumeAlertThreshold float64) error {
	if maxBudget < 0 {
		return fmt.Errorf("max budget cannot be negative (use 0 for unlimited)")
	}
	if maxVolumeGB < 0 {
		return fmt.Errorf("max volume cannot be negative (use 0 for unlimited)")
	}
	if costAlertThreshold < 0 || costAlertThreshold > 1 {
		return fmt.Errorf("cost alert threshold must be between 0.0 and 1.0")
	}
	if volumeAlertThreshold < 0 || volumeAlertThreshold > 1 {
		return fmt.Errorf("volume alert threshold must be between 0.0 and 1.0")
	}

	m.config.GlobalBudget = &config.GlobalBudget{
		MaxBudget:            maxBudget,
		MaxVolumeGB:          maxVolumeGB,
		AlertThreshold:       costAlertThreshold,
		VolumeAlertThreshold: volumeAlertThreshold,
	}

	if err := m.saveState(); err != nil {
		return fmt.Errorf("persist global budget: %w", err)
	}

	m.logger.Info("Global budget updated",
		"max_budget", maxBudget,
		"max_volume_gb", maxVolumeGB,
		"cost_alert_threshold", costAlertThreshold,
		"volume_alert_threshold", volumeAlertThreshold)
	return nil
}

// GetGlobalTeamBudgetStatus returns the status of the persisted global/team
// budget ceiling (#246 PR2) — aggregate spend/volume across all projects
// against the GlobalBudget caps. Distinct from GetGlobalBudgetStatus, which
// reports the legacy BudgetPeriods-based global status. Returns a status with
// zero maxima when no global budget is set.
func (m *Manager) GetGlobalTeamBudgetStatus() *BudgetStatus {
	status := &BudgetStatus{
		BudgetType:   "global",
		Currency:     m.config.Pricing.Currency,
		CurrentSpend: m.totalSpend(),
	}
	gb := m.config.GlobalBudget
	if gb == nil {
		return status
	}
	status.MaxBudget = gb.MaxBudget
	status.AlertThreshold = gb.AlertThreshold
	if gb.MaxBudget > 0 {
		status.BudgetRemaining = gb.MaxBudget - status.CurrentSpend
		status.BudgetUsed = status.CurrentSpend / gb.MaxBudget
		status.OverBudget = status.CurrentSpend > gb.MaxBudget
		status.AlertTriggered = gb.AlertThreshold > 0 && status.BudgetUsed >= gb.AlertThreshold
	}
	return status
}

// totalSpend returns the aggregate recorded cost across all projects (and
// un-projected records) — the basis for the global/team ceiling.
func (m *Manager) totalSpend() float64 {
	total := 0.0
	for _, r := range m.reporter.SnapshotRecords() {
		total += r.Cost
	}
	return total
}

// totalVolumeGB returns the aggregate recorded volume across all projects.
func (m *Manager) totalVolumeGB() float64 {
	total := 0.0
	for _, r := range m.reporter.SnapshotRecords() {
		total += r.SizeGB
	}
	return total
}

// checkGlobalBudgetCeiling enforces the persisted GlobalBudget cost ceiling
// across all projects (#246 PR2). Returns a BudgetExceededError when the
// additional cost would push aggregate spend over the ceiling.
func (m *Manager) checkGlobalBudgetCeiling(additionalCost float64) error {
	gb := m.config.GlobalBudget
	if gb == nil || gb.MaxBudget <= 0 {
		return nil // no global cost ceiling set
	}
	current := m.totalSpend()
	projected := current + additionalCost
	if projected > gb.MaxBudget {
		return &BudgetExceededError{
			BudgetType:     "global",
			MaxBudget:      gb.MaxBudget,
			CurrentSpend:   current,
			AdditionalCost: additionalCost,
			ProjectedSpend: projected,
			Overage:        projected - gb.MaxBudget,
			Currency:       m.config.Pricing.Currency,
		}
	}
	return nil
}

// checkGlobalVolumeCeiling enforces the persisted GlobalBudget volume ceiling
// across all projects (#246 PR2).
func (m *Manager) checkGlobalVolumeCeiling(additionalGB float64) error {
	gb := m.config.GlobalBudget
	if gb == nil || gb.MaxVolumeGB <= 0 {
		return nil // no global volume ceiling set
	}
	current := m.totalVolumeGB()
	projected := current + additionalGB
	if projected > gb.MaxVolumeGB {
		return &VolumeQuotaExceededError{
			QuotaType:       "global",
			MaxVolumeGB:     gb.MaxVolumeGB,
			CurrentVolumeGB: current,
			AdditionalGB:    additionalGB,
			ProjectedVolume: projected,
			Overage:         projected - gb.MaxVolumeGB,
		}
	}
	return nil
}
