// Package cost provides integrated cost management for CargoShip
package cost

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// Manager provides integrated cost management functionality
type Manager struct {
	config        *config.CostControlConfig
	pricingMgr    *PricingManager
	reporter      *CostReporter
	budgetTracker *BudgetTracker
	logger        *slog.Logger
}

// BudgetTracker tracks budget usage and alerts
type BudgetTracker struct {
	maxBudget      float64
	alertThreshold float64
	currentSpend   float64
	lastAlertSent  time.Time
	alertCooldown  time.Duration
}

// CostApprovalRequest represents a request for cost approval
type CostApprovalRequest struct {
	Operation      string     `json:"operation"`
	EstimatedCost  float64    `json:"estimated_cost"`
	Currency       string     `json:"currency"`
	Justification  string     `json:"justification"`
	RequestedBy    string     `json:"requested_by"`
	RequestedAt    time.Time  `json:"requested_at"`
	ApprovalStatus string     `json:"approval_status"` // "pending", "approved", "denied"
	ApprovedBy     string     `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	DeniedReason   string     `json:"denied_reason,omitempty"`
}

// NewManager creates a new cost management manager
func NewManager(cfg *config.CostControlConfig, awsCfg aws.Config, logger *slog.Logger) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cost control config cannot be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Initialize pricing manager
	pricingMgr, err := NewPricingManager(&cfg.Pricing, awsCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create pricing manager: %w", err)
	}

	// Initialize S3 client for reporting (if configured)
	var s3Client *s3.Client
	if cfg.Reporting.Enabled && cfg.Reporting.ReportBucket != "" {
		s3Client = s3.NewFromConfig(awsCfg)
	}

	// Initialize cost reporter
	reporter := NewCostReporter(&cfg.Reporting, pricingMgr, s3Client, logger)

	// Initialize budget tracker
	budgetTracker := &BudgetTracker{
		maxBudget:      cfg.MaxMonthlyBudget,
		alertThreshold: cfg.AlertThreshold,
		alertCooldown:  24 * time.Hour, // Don't spam alerts
	}

	return &Manager{
		config:        cfg,
		pricingMgr:    pricingMgr,
		reporter:      reporter,
		budgetTracker: budgetTracker,
		logger:        logger.With("component", "cost-manager"),
	}, nil
}

// EstimateOperationCost estimates the cost of an operation before execution
func (m *Manager) EstimateOperationCost(ctx context.Context, operation string, sizeGB float64, storageClass config.StorageClass, region string) (*CostEstimate, error) {
	switch operation {
	case "upload", "archive":
		return m.pricingMgr.EstimateArchivalCost(ctx, sizeGB, storageClass, region)
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}
}

// CheckCostApproval checks if an operation requires cost approval
func (m *Manager) CheckCostApproval(ctx context.Context, operation string, estimatedCost float64) (bool, error) {
	// Check if cost exceeds approval threshold
	if estimatedCost > m.config.RequireApprovalOver {
		m.logger.Info("Operation requires cost approval",
			"operation", operation,
			"estimated_cost", estimatedCost,
			"threshold", m.config.RequireApprovalOver)
		return true, nil
	}

	// Check budget limits
	if err := m.checkBudgetLimits(estimatedCost); err != nil {
		return true, err
	}

	return false, nil
}

// RequestCostApproval creates a cost approval request
func (m *Manager) RequestCostApproval(operation string, estimatedCost float64, justification string, requestedBy string) *CostApprovalRequest {
	return &CostApprovalRequest{
		Operation:      operation,
		EstimatedCost:  estimatedCost,
		Currency:       m.config.Pricing.Currency,
		Justification:  justification,
		RequestedBy:    requestedBy,
		RequestedAt:    time.Now(),
		ApprovalStatus: "pending",
	}
}

// RecordOperationCost records the actual cost of a completed operation
func (m *Manager) RecordOperationCost(ctx context.Context, operation string, fileName string, sizeBytes int64, storageClass config.StorageClass, region string, jobID string, projectID string, tags map[string]string) error {
	// Record cost with reporter
	err := m.reporter.RecordArchivalCost(ctx, fileName, sizeBytes, storageClass, region, jobID, projectID, tags)
	if err != nil {
		return fmt.Errorf("failed to record cost: %w", err)
	}

	// Update budget tracking
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
	estimate, err := m.pricingMgr.EstimateArchivalCost(ctx, sizeGB, storageClass, region)
	if err != nil {
		m.logger.Warn("Failed to get cost estimate for budget tracking", "error", err)
		return nil
	}

	m.budgetTracker.currentSpend += estimate.TotalCost

	// Check for budget alerts
	if err := m.checkAndSendBudgetAlerts(); err != nil {
		m.logger.Error("Failed to check budget alerts", "error", err)
	}

	return nil
}

// GenerateCostReport generates a cost report for the specified period
func (m *Manager) GenerateCostReport(ctx context.Context, period string) (*CostSummary, error) {
	return m.reporter.GenerateReport(ctx, period)
}

// ExportCostReport exports a cost report
func (m *Manager) ExportCostReport(ctx context.Context, summary *CostSummary, format string, outputPath string) error {
	return m.reporter.ExportReport(ctx, summary, format, outputPath)
}

// GetCurrentMonthSpend returns the current month's spending
func (m *Manager) GetCurrentMonthSpend() float64 {
	return m.reporter.GetCurrentMonthCosts()
}

// GetBudgetStatus returns current budget status
// Supports both new budget periods and legacy monthly budget (backward compatible)
func (m *Manager) GetBudgetStatus() map[string]interface{} {
	currentSpend := m.GetCurrentMonthSpend()
	now := time.Now()

	// Check if budget periods are configured (new system)
	if len(m.config.BudgetPeriods) > 0 {
		// Use active budget period (default to index 0)
		activeIndex := m.config.ActiveBudgetPeriodIndex
		if activeIndex < 0 || activeIndex >= len(m.config.BudgetPeriods) {
			activeIndex = 0
		}

		budgetPeriod := m.config.BudgetPeriods[activeIndex]

		// Get period bounds
		start, end, err := budgetPeriod.GetPeriodBounds(now)
		if err != nil {
			m.logger.Error("Failed to get budget period bounds", "error", err)
			// Fall back to legacy system
			return m.getLegacyBudgetStatus(currentSpend)
		}

		daysElapsed, _ := budgetPeriod.GetDaysElapsed(now)
		daysRemaining, _ := budgetPeriod.GetDaysRemaining(now)
		totalDays, _ := budgetPeriod.GetTotalDays(now)
		burnRate, _ := budgetPeriod.CalculateBurnRate(currentSpend, now)
		projectedSpend, _ := budgetPeriod.ProjectEndOfPeriodSpend(currentSpend, now)
		willExceed, _, _ := budgetPeriod.WillExceedBudget(currentSpend, now)

		budgetUsed := currentSpend / budgetPeriod.MaxBudget
		remaining := budgetPeriod.MaxBudget - currentSpend

		// Calculate target daily rate to stay within budget
		targetDailyRate := 0.0
		if daysRemaining > 0 {
			targetDailyRate = remaining / float64(daysRemaining)
		}

		status := map[string]interface{}{
			// Period information
			"period_type":          budgetPeriod.Type,
			"period_start":         start.Format("2006-01-02"),
			"period_end":           end.Format("2006-01-02"),
			"days_elapsed":         daysElapsed,
			"days_remaining":       daysRemaining,
			"total_days":           totalDays,

			// Budget information
			"max_budget":           budgetPeriod.MaxBudget,
			"current_spend":        currentSpend,
			"budget_used":          budgetUsed,
			"budget_remaining":     remaining,
			"alert_threshold":      budgetPeriod.AlertThreshold,
			"currency":             m.config.Pricing.Currency,

			// Burn rate and projections
			"daily_burn_rate":      burnRate,
			"projected_eop_spend":  projectedSpend,
			"will_exceed_budget":   willExceed,
			"target_daily_rate":    targetDailyRate,

			// Status flags
			"over_budget":          budgetUsed > 1.0,
			"alert_triggered":      budgetUsed > budgetPeriod.AlertThreshold,

			// Grant-specific information
			"grant_name":           budgetPeriod.GrantName,
			"enable_rollover":      budgetPeriod.EnableRollover,
		}

		// Add overage/savings information
		if willExceed {
			overage := projectedSpend - budgetPeriod.MaxBudget
			status["projected_overage"] = overage
			status["projected_overage_percent"] = (overage / budgetPeriod.MaxBudget) * 100
		} else {
			savings := budgetPeriod.MaxBudget - projectedSpend
			status["projected_savings"] = savings
			status["projected_savings_percent"] = (savings / budgetPeriod.MaxBudget) * 100
		}

		return status
	}

	// Fall back to legacy monthly budget system
	return m.getLegacyBudgetStatus(currentSpend)
}

// getLegacyBudgetStatus provides backward compatibility with old monthly budget system
func (m *Manager) getLegacyBudgetStatus(currentSpend float64) map[string]interface{} {
	budgetUsed := currentSpend / m.config.MaxMonthlyBudget
	remaining := m.config.MaxMonthlyBudget - currentSpend

	return map[string]interface{}{
		"max_budget":       m.config.MaxMonthlyBudget,
		"current_spend":    currentSpend,
		"budget_used":      budgetUsed,
		"budget_remaining": remaining,
		"alert_threshold":  m.config.AlertThreshold,
		"currency":         m.config.Pricing.Currency,
		"over_budget":      budgetUsed > 1.0,
		"alert_triggered":  budgetUsed > m.config.AlertThreshold,
	}
}

// OptimizeCosts suggests cost optimizations
func (m *Manager) OptimizeCosts(ctx context.Context) ([]CostRecommendation, error) {
	summary, err := m.reporter.GenerateReport(ctx, "month")
	if err != nil {
		return nil, fmt.Errorf("failed to generate cost summary: %w", err)
	}

	return summary.Recommendations, nil
}

// PerformScheduledTasks performs scheduled cost management tasks
func (m *Manager) PerformScheduledTasks(ctx context.Context) error {
	// Generate and export reports if configured
	if m.config.Reporting.Enabled {
		if err := m.generateScheduledReports(ctx); err != nil {
			m.logger.Error("Failed to generate scheduled reports", "error", err)
		}
	}

	// Purge old cost records (keep 1 year by default)
	purged := m.reporter.PurgeCosts(365 * 24 * time.Hour)
	if purged > 0 {
		m.logger.Info("Purged old cost records", "count", purged)
	}

	// Clear pricing cache if needed
	cacheStats := m.pricingMgr.GetCacheStats()
	if totalEntries, ok := cacheStats["total_entries"].(int); ok && totalEntries > 10000 {
		m.pricingMgr.ClearCache()
		m.logger.Info("Cleared pricing cache due to size", "entries", totalEntries)
	}

	return nil
}

// checkBudgetLimits checks if the operation would exceed budget limits
func (m *Manager) checkBudgetLimits(additionalCost float64) error {
	currentSpend := m.GetCurrentMonthSpend()
	projectedSpend := currentSpend + additionalCost

	if projectedSpend > m.config.MaxMonthlyBudget {
		return fmt.Errorf("operation would exceed monthly budget: projected $%.2f, budget $%.2f",
			projectedSpend, m.config.MaxMonthlyBudget)
	}

	return nil
}

// checkAndSendBudgetAlerts checks if budget alerts should be sent
func (m *Manager) checkAndSendBudgetAlerts() error {
	currentSpend := m.GetCurrentMonthSpend()
	budgetUsed := currentSpend / m.config.MaxMonthlyBudget

	// Check if alert threshold exceeded and cooldown period passed
	if budgetUsed > m.config.AlertThreshold {
		timeSinceLastAlert := time.Since(m.budgetTracker.lastAlertSent)
		if timeSinceLastAlert > m.budgetTracker.alertCooldown {
			m.logger.Warn("Budget alert triggered",
				"current_spend", currentSpend,
				"max_budget", m.config.MaxMonthlyBudget,
				"budget_used_percent", budgetUsed*100,
				"alert_threshold_percent", m.config.AlertThreshold*100)

			// TODO: Implement actual alert sending (email, Slack, etc.)
			// For now, just log and update timestamp
			m.budgetTracker.lastAlertSent = time.Now()
		}
	}

	return nil
}

// generateScheduledReports generates reports according to schedule
func (m *Manager) generateScheduledReports(ctx context.Context) error {
	var period string

	switch m.config.Reporting.Frequency {
	case "daily":
		period = "today"
	case "weekly":
		period = "week"
	case "monthly":
		period = "month"
	default:
		period = "week"
	}

	summary, err := m.reporter.GenerateReport(ctx, period)
	if err != nil {
		return fmt.Errorf("failed to generate %s report: %w", period, err)
	}

	// Export report
	timestamp := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("cost-report-%s-%s.%s", period, timestamp, m.config.Reporting.ExportFormat)
	outputPath := fmt.Sprintf("/tmp/%s", filename)

	if err := m.reporter.ExportReport(ctx, summary, m.config.Reporting.ExportFormat, outputPath); err != nil {
		return fmt.Errorf("failed to export report: %w", err)
	}

	// Upload to S3 if configured
	if m.config.Reporting.ReportBucket != "" {
		s3Key := fmt.Sprintf("cost-reports/%d/%02d/%s",
			time.Now().Year(), time.Now().Month(), filename)

		if err := m.reporter.UploadReportToS3(ctx, outputPath, m.config.Reporting.ReportBucket, s3Key); err != nil {
			return fmt.Errorf("failed to upload report to S3: %w", err)
		}
	}

	m.logger.Info("Scheduled cost report generated", "period", period, "file", filename)
	return nil
}

// GetPricingStats returns pricing cache statistics
func (m *Manager) GetPricingStats() map[string]interface{} {
	return m.pricingMgr.GetCacheStats()
}

// ValidateConfig validates the cost configuration
func (m *Manager) ValidateConfig() error {
	if m.config.MaxMonthlyBudget <= 0 {
		return fmt.Errorf("max_monthly_budget must be greater than 0")
	}

	if m.config.AlertThreshold < 0 || m.config.AlertThreshold > 1 {
		return fmt.Errorf("alert_threshold must be between 0.0 and 1.0")
	}

	if m.config.RequireApprovalOver < 0 {
		return fmt.Errorf("require_approval_over cannot be negative")
	}

	if m.config.Pricing.GlobalDiscount < 0 || m.config.Pricing.GlobalDiscount > 1 {
		return fmt.Errorf("global_discount must be between 0.0 and 1.0")
	}

	// Validate service discounts
	for service, discount := range m.config.Pricing.ServiceDiscounts {
		if discount < 0 || discount > 1 {
			return fmt.Errorf("service_discount for %s must be between 0.0 and 1.0", service)
		}
	}

	return nil
}

// GetCurrentPricing returns current pricing for common operations
func (m *Manager) GetCurrentPricing(ctx context.Context, region string) (map[string]interface{}, error) {
	pricing := make(map[string]interface{})

	// Get storage pricing for different classes
	storagePricing := make(map[string]float64)
	storageClasses := []config.StorageClass{
		config.StorageClassStandard,
		config.StorageClassStandardIA,
		config.StorageClassIntelligentTiering,
		config.StorageClassGlacier,
		config.StorageClassDeepArchive,
	}

	for _, class := range storageClasses {
		price, err := m.pricingMgr.getStoragePrice(ctx, class, region)
		if err != nil {
			m.logger.Warn("Failed to get pricing for storage class", "class", class, "error", err)
			continue
		}
		storagePricing[string(class)] = price
	}

	pricing["storage_per_gb_month"] = storagePricing

	// Get request pricing
	requestPricing := make(map[string]float64)
	putPrice, _ := m.pricingMgr.getRequestPrice(ctx, "PUT", config.StorageClassStandard, region)
	getPrice, _ := m.pricingMgr.getRequestPrice(ctx, "GET", config.StorageClassStandard, region)

	requestPricing["put_per_1000"] = putPrice
	requestPricing["get_per_1000"] = getPrice

	pricing["requests"] = requestPricing

	// Add metadata
	pricing["region"] = region
	pricing["currency"] = m.config.Pricing.Currency
	pricing["last_updated"] = time.Now().Format(time.RFC3339)
	pricing["source"] = "aws_pricing_api"
	if !m.config.Pricing.UseAWSPricingAPI {
		pricing["source"] = "fallback_pricing"
	}

	return pricing, nil
}

// GetReporter returns the cost reporter (Issue #147 Phase 2)
func (m *Manager) GetReporter() *CostReporter {
	return m.reporter
}
