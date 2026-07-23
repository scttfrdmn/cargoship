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

// maxLedgerRecords bounds the persisted cost ledger. When the recorded history
// exceeds this, the oldest records are dropped (FIFO) before a save so the store
// file can't grow without limit (#246).
const maxLedgerRecords = 10000

// Manager provides integrated cost management functionality
type Manager struct {
	config        *config.CostControlConfig
	pricingMgr    *PricingManager
	reporter      *CostReporter
	budgetTracker *BudgetTracker
	notifier      *BudgetAlertNotifier // Issue #133: Budget alert notifier
	store         BudgetStore          // #246: durable budget limits + cost ledger
	logger        *slog.Logger
	awsConfig     aws.Config // Issue #147 Phase 4: Needed for alert notification
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

// NewManager creates a new cost management manager backed by the default local
// budget store (~/.cargoship/budgets.json).
func NewManager(cfg *config.CostControlConfig, awsCfg aws.Config, logger *slog.Logger) (*Manager, error) {
	return newManagerWithStore(cfg, awsCfg, logger, localStore{})
}

// newManagerWithStore is the injectable constructor used by tests to supply a
// custom BudgetStore. Production code uses NewManager.
func newManagerWithStore(cfg *config.CostControlConfig, awsCfg aws.Config, logger *slog.Logger, store BudgetStore) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cost control config cannot be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}
	if store == nil {
		store = localStore{}
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

	// Issue #133: Initialize budget alert notifier
	alertConfig := DefaultBudgetAlertConfig()
	notifier := NewBudgetAlertNotifier(alertConfig, awsCfg)

	// Load persisted budget state so `budget set` survives across CLI
	// invocations (#241) and recorded spend is rehydrated across restarts (#246).
	// Budgets from the config file (if any) take precedence over the persisted
	// store for the same project ID; the recorded ledger seeds the reporter so
	// `budget status` reflects prior uploads.
	if state, _, err := store.Load(); err != nil {
		logger.Warn("failed to load persisted budget store", "error", err)
	} else {
		if len(state.ProjectBudgets) > 0 {
			if cfg.ProjectBudgets == nil {
				cfg.ProjectBudgets = make(map[string]config.ProjectBudget)
			}
			for id, b := range state.ProjectBudgets {
				if _, exists := cfg.ProjectBudgets[id]; !exists {
					cfg.ProjectBudgets[id] = b
				}
			}
		}
		reporter.SeedRecords(state.Records)
	}

	return &Manager{
		config:        cfg,
		pricingMgr:    pricingMgr,
		reporter:      reporter,
		budgetTracker: budgetTracker,
		notifier:      notifier,
		store:         store,
		logger:        logger.With("component", "cost-manager"),
		awsConfig:     awsCfg, // Issue #147 Phase 4: For alert notification
	}, nil
}

// saveState writes the current project budgets and recorded cost ledger to the
// store as one document (both halves together, so a limits update never clobbers
// the recorded ledger and vice versa — #246). FIFO rotation bounds the ledger.
func (m *Manager) saveState() error {
	if m.store == nil {
		m.store = localStore{}
	}
	records := m.reporter.SnapshotRecords()
	if len(records) > maxLedgerRecords {
		records = records[len(records)-maxLedgerRecords:]
	}
	state := LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: m.config.ProjectBudgets,
		Records:        records,
	}
	// Load the current token first so a Phase B S3 store can do a CAS write; the
	// local store ignores it. A load error still attempts an unconditional save.
	_, tok, err := m.store.Load()
	if err != nil {
		m.logger.Warn("budget store load before save failed", "error", err)
		tok = ""
	}
	return m.store.Save(state, tok)
}

// persistLedger is the best-effort variant of saveState used on the
// operation-recording path: a storage failure is logged, not returned, so it
// can't fail an operation whose work already completed (#246).
func (m *Manager) persistLedger() {
	if err := m.saveState(); err != nil {
		m.logger.Warn("failed to persist budget ledger", "error", err)
	}
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
// It performs budget AND volume quota enforcement before recording
func (m *Manager) RecordOperationCost(ctx context.Context, operation string, fileName string, sizeBytes int64, storageClass config.StorageClass, region string, jobID string, projectID string, tags map[string]string) error {
	// Calculate size in GB
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

	// ENFORCEMENT CHECK 1: Volume quota (always check, independent of cost estimation)
	if projectID != "" {
		// Check project-specific volume quota
		if err := m.CheckProjectVolumeQuota(projectID, sizeGB); err != nil {
			m.logger.Warn("Operation blocked by volume quota enforcement",
				"project_id", projectID,
				"size_gb", sizeGB,
				"error", err)
			return fmt.Errorf("volume quota enforcement: %w", err)
		}
	} else {
		// Check global volume quota only
		if err := m.checkGlobalVolumeQuota(sizeGB); err != nil {
			m.logger.Warn("Operation blocked by volume quota enforcement",
				"size_gb", sizeGB,
				"error", err)
			return fmt.Errorf("volume quota enforcement: %w", err)
		}
	}

	// ENFORCEMENT CHECK 2: Cost budget
	estimate, err := m.pricingMgr.EstimateArchivalCost(ctx, sizeGB, storageClass, region)
	if err != nil {
		m.logger.Warn("Failed to get cost estimate for budget enforcement", "error", err)
		// Continue recording even if estimate fails (non-blocking)
	} else {
		// Check budget limits BEFORE recording (if cost estimation succeeded)
		if projectID != "" {
			// Check project-specific budget
			if err := m.CheckProjectBudget(projectID, estimate.TotalCost); err != nil {
				// Budget would be exceeded, return error
				m.logger.Warn("Operation blocked by budget enforcement",
					"project_id", projectID,
					"estimated_cost", estimate.TotalCost,
					"error", err)
				return fmt.Errorf("budget enforcement: %w", err)
			}
		} else {
			// Check global budget only
			if err := m.checkGlobalBudget(estimate.TotalCost); err != nil {
				m.logger.Warn("Operation blocked by budget enforcement",
					"estimated_cost", estimate.TotalCost,
					"error", err)
				return fmt.Errorf("budget enforcement: %w", err)
			}
		}
	}

	// Both checks passed, record cost with reporter
	err = m.reporter.RecordArchivalCost(ctx, fileName, sizeBytes, storageClass, region, jobID, projectID, tags)
	if err != nil {
		return fmt.Errorf("failed to record cost: %w", err)
	}

	// Persist the recorded ledger so `budget status` reflects this upload across
	// process restarts (#246). Best-effort.
	m.persistLedger()

	// Update legacy budget tracking (for backward compatibility)
	if estimate != nil {
		m.budgetTracker.currentSpend += estimate.TotalCost
	}

	// Check for budget alerts (Issue #133)
	if err := m.checkAndSendBudgetAlerts(ctx); err != nil {
		m.logger.Error("Failed to check budget alerts", "error", err)
	}

	return nil
}

// RecordCompletedUpload records the cost of an upload that has ALREADY finished,
// then persists the ledger. Unlike RecordOperationCost it does not enforce
// budgets (there is nothing left to block — the bytes are in S3), so an
// over-budget upload is still recorded rather than silently dropped (#246).
// It still fires budget alerts so an over-budget condition is surfaced. Returns
// an error only if the cost couldn't be recorded at all.
func (m *Manager) RecordCompletedUpload(ctx context.Context, fileName string, sizeBytes int64, storageClass config.StorageClass, region string, jobID string, projectID string, tags map[string]string) error {
	if err := m.reporter.RecordArchivalCost(ctx, fileName, sizeBytes, storageClass, region, jobID, projectID, tags); err != nil {
		return fmt.Errorf("failed to record cost: %w", err)
	}
	m.persistLedger()
	if err := m.checkAndSendBudgetAlerts(ctx); err != nil {
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
			"period_type":    budgetPeriod.Type,
			"period_start":   start.Format("2006-01-02"),
			"period_end":     end.Format("2006-01-02"),
			"days_elapsed":   daysElapsed,
			"days_remaining": daysRemaining,
			"total_days":     totalDays,

			// Budget information
			"max_budget":       budgetPeriod.MaxBudget,
			"current_spend":    currentSpend,
			"budget_used":      budgetUsed,
			"budget_remaining": remaining,
			"alert_threshold":  budgetPeriod.AlertThreshold,
			"currency":         m.config.Pricing.Currency,

			// Burn rate and projections
			"daily_burn_rate":     burnRate,
			"projected_eop_spend": projectedSpend,
			"will_exceed_budget":  willExceed,
			"target_daily_rate":   targetDailyRate,

			// Status flags
			"over_budget":     budgetUsed > 1.0,
			"alert_triggered": budgetUsed > budgetPeriod.AlertThreshold,

			// Grant-specific information
			"grant_name":      budgetPeriod.GrantName,
			"enable_rollover": budgetPeriod.EnableRollover,
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

// checkAndSendBudgetAlerts checks if budget alerts should be sent (Issue #133)
func (m *Manager) checkAndSendBudgetAlerts(ctx context.Context) error {
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

			// Issue #133: Send actual alert notifications via configured channels
			// Check global budget and send alert if needed
			if err := m.CheckAndNotifyBudgetStatus(ctx, "", m.notifier); err != nil {
				m.logger.Error("Failed to send budget alert", "error", err)
				return fmt.Errorf("failed to send budget alert: %w", err)
			}

			// Update last alert timestamp to respect cooldown
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

// QueryCostsByDVCStage returns an aggregated cost summary for a named DVC pipeline stage (Issue #186).
func (m *Manager) QueryCostsByDVCStage(stage string) (*DVCStageSummary, error) {
	return m.reporter.QueryCostsByDVCStage(stage)
}

// QueryCostsByGitCommit returns all cost records tagged with the given git commit SHA (Issue #186).
func (m *Manager) QueryCostsByGitCommit(commit string) ([]CostRecord, error) {
	return m.reporter.QueryCostsByGitCommit(commit)
}

// GenerateNSFComplianceReport creates an NSF data-management compliance report (Issue #187).
func (m *Manager) GenerateNSFComplianceReport(budgetID, grantNumber string) (*ComplianceReport, error) {
	return m.reporter.GenerateNSFComplianceReport(budgetID, grantNumber)
}

// GenerateNIHComplianceReport creates an NIH data-management compliance report (Issue #187).
func (m *Manager) GenerateNIHComplianceReport(budgetID, grantNumber string) (*ComplianceReport, error) {
	return m.reporter.GenerateNIHComplianceReport(budgetID, grantNumber)
}

// GetAlertConfig returns the current alert configuration (Issue #147 Phase 4)
func (m *Manager) GetAlertConfig() *BudgetAlertConfig {
	// TODO: Load from persistent storage (file, database, etc.)
	// For now, return default config
	return DefaultBudgetAlertConfig()
}

// UpdateAlertConfig updates the alert configuration (Issue #147 Phase 4)
func (m *Manager) UpdateAlertConfig(config *BudgetAlertConfig) error {
	if config == nil {
		return fmt.Errorf("alert config cannot be nil")
	}

	// TODO: Save to persistent storage (file, database, etc.)
	// For now, just validate the config
	if config.EmailEnabled {
		if config.SMTPHost == "" {
			return fmt.Errorf("SMTP host is required when email is enabled")
		}
		if len(config.EmailRecipients) == 0 {
			return fmt.Errorf("email recipients are required when email is enabled")
		}
	}

	if config.SlackEnabled {
		if config.SlackWebhookURL == "" {
			return fmt.Errorf("slack webhook URL is required when slack is enabled")
		}
	}

	return nil
}

// SendTestAlert sends a test alert through configured channels (Issue #147 Phase 4)
func (m *Manager) SendTestAlert(ctx context.Context, alert *BudgetAlert, channel string) error {
	config := m.GetAlertConfig()
	if config == nil {
		return fmt.Errorf("no alert configuration found")
	}

	// Create notifier
	notifier := NewBudgetAlertNotifier(config, m.awsConfig)

	// If channel specified, temporarily enable only that channel
	if channel != "" {
		testConfig := *config
		testConfig.EmailEnabled = false
		testConfig.SlackEnabled = false
		testConfig.WebhookEnabled = false
		testConfig.CloudWatchEnabled = false

		switch channel {
		case "email":
			testConfig.EmailEnabled = config.EmailEnabled
			testConfig.EmailRecipients = config.EmailRecipients
			testConfig.SMTPHost = config.SMTPHost
			testConfig.SMTPPort = config.SMTPPort
			testConfig.SMTPUsername = config.SMTPUsername
			testConfig.SMTPPassword = config.SMTPPassword
			testConfig.SMTPFrom = config.SMTPFrom
			testConfig.SMTPUseTLS = config.SMTPUseTLS
		case "slack":
			testConfig.SlackEnabled = config.SlackEnabled
			testConfig.SlackWebhookURL = config.SlackWebhookURL
			testConfig.SlackChannel = config.SlackChannel
			testConfig.SlackUsername = config.SlackUsername
		case "webhook":
			testConfig.WebhookEnabled = config.WebhookEnabled
			testConfig.WebhookURL = config.WebhookURL
			testConfig.WebhookHeaders = config.WebhookHeaders
			testConfig.WebhookTimeout = config.WebhookTimeout
		case "cloudwatch":
			testConfig.CloudWatchEnabled = config.CloudWatchEnabled
			testConfig.CloudWatchNamespace = config.CloudWatchNamespace
		default:
			return fmt.Errorf("unknown channel: %s", channel)
		}

		notifier = NewBudgetAlertNotifier(&testConfig, m.awsConfig)
	}

	// Send the alert
	return notifier.SendAlert(ctx, alert)
}
