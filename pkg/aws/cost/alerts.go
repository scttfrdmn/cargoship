// Package cost provides budget alert notification system
package cost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// BudgetAlert represents a budget threshold alert
type BudgetAlert struct {
	// Alert identification
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`

	// Alert type
	Type     BudgetAlertType     `json:"type"`      // cost_threshold, volume_threshold, cost_over_budget, volume_over_quota
	Severity BudgetAlertSeverity `json:"severity"` // info, warning, critical

	// Budget/Project context
	ProjectID   string  `json:"project_id,omitempty"`   // Empty if global budget
	Description string  `json:"description"`            // Human-readable description
	IsGlobal    bool    `json:"is_global"`              // True if global budget alert

	// Cost metrics (if cost alert)
	MaxBudget          float64 `json:"max_budget,omitempty"`
	CurrentSpend       float64 `json:"current_spend,omitempty"`
	BudgetRemaining    float64 `json:"budget_remaining,omitempty"`
	BudgetUsedPercent  float64 `json:"budget_used_percent,omitempty"`
	ThresholdPercent   float64 `json:"threshold_percent,omitempty"`

	// Volume metrics (if volume alert)
	MaxVolumeGB          float64 `json:"max_volume_gb,omitempty"`
	CurrentVolumeGB      float64 `json:"current_volume_gb,omitempty"`
	VolumeRemaining      float64 `json:"volume_remaining,omitempty"`
	VolumeUsedPercent    float64 `json:"volume_used_percent,omitempty"`
	VolumeThresholdPercent float64 `json:"volume_threshold_percent,omitempty"`

	// Actions and recommendations
	Recommendation string `json:"recommendation,omitempty"`
	ActionRequired bool   `json:"action_required"`
}

// BudgetAlertType represents the type of budget alert
type BudgetAlertType string

const (
	// AlertTypeCostThreshold indicates cost has exceeded the alert threshold
	AlertTypeCostThreshold BudgetAlertType = "cost_threshold"

	// AlertTypeVolumeThreshold indicates volume has exceeded the alert threshold
	AlertTypeVolumeThreshold BudgetAlertType = "volume_threshold"

	// AlertTypeCostOverBudget indicates cost has exceeded the maximum budget
	AlertTypeCostOverBudget BudgetAlertType = "cost_over_budget"

	// AlertTypeVolumeOverQuota indicates volume has exceeded the maximum quota
	AlertTypeVolumeOverQuota BudgetAlertType = "volume_over_quota"

	// AlertTypeBudgetProjection indicates projected costs will exceed budget
	AlertTypeBudgetProjection BudgetAlertType = "budget_projection"

	// AlertTypeVolumeProjection indicates projected volume will exceed quota
	AlertTypeVolumeProjection BudgetAlertType = "volume_projection"
)

// BudgetAlertSeverity represents the severity of a budget alert
type BudgetAlertSeverity string

const (
	// SeverityInfo is for informational alerts (< threshold)
	SeverityInfo BudgetAlertSeverity = "info"

	// SeverityWarning is for warning alerts (> threshold, < max)
	SeverityWarning BudgetAlertSeverity = "warning"

	// SeverityCritical is for critical alerts (> max)
	SeverityCritical BudgetAlertSeverity = "critical"
)

// BudgetAlertConfig configures the budget alert system
type BudgetAlertConfig struct {
	// Enable/disable alert system
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Alert cooldown period (prevents spam)
	CooldownPeriod time.Duration `yaml:"cooldown_period" json:"cooldown_period"`

	// Webhook configuration
	WebhookEnabled bool              `yaml:"webhook_enabled" json:"webhook_enabled"`
	WebhookURL     string            `yaml:"webhook_url" json:"webhook_url"`
	WebhookHeaders map[string]string `yaml:"webhook_headers" json:"webhook_headers,omitempty"`
	WebhookTimeout time.Duration     `yaml:"webhook_timeout" json:"webhook_timeout"`

	// CloudWatch alarm configuration
	CloudWatchEnabled   bool   `yaml:"cloudwatch_enabled" json:"cloudwatch_enabled"`
	CloudWatchNamespace string `yaml:"cloudwatch_namespace" json:"cloudwatch_namespace"`
	CloudWatchRegion    string `yaml:"cloudwatch_region" json:"cloudwatch_region"`

	// Alert delivery options
	SendProjectAlerts bool `yaml:"send_project_alerts" json:"send_project_alerts"`
	SendGlobalAlerts  bool `yaml:"send_global_alerts" json:"send_global_alerts"`
}

// BudgetAlertNotifier manages budget alert notifications
type BudgetAlertNotifier struct {
	config         *BudgetAlertConfig
	awsConfig      aws.Config
	cloudwatchSvc  *cloudwatch.Client
	httpClient     *http.Client
	lastAlertTimes map[string]time.Time // key: projectID or "global"
}

// NewBudgetAlertNotifier creates a new budget alert notifier
func NewBudgetAlertNotifier(config *BudgetAlertConfig, awsConfig aws.Config) *BudgetAlertNotifier {
	if config == nil {
		config = DefaultBudgetAlertConfig()
	}

	notifier := &BudgetAlertNotifier{
		config:         config,
		awsConfig:      awsConfig,
		lastAlertTimes: make(map[string]time.Time),
		httpClient: &http.Client{
			Timeout: config.WebhookTimeout,
		},
	}

	// Initialize CloudWatch client if enabled
	if config.CloudWatchEnabled {
		notifier.cloudwatchSvc = cloudwatch.NewFromConfig(awsConfig, func(o *cloudwatch.Options) {
			if config.CloudWatchRegion != "" {
				o.Region = config.CloudWatchRegion
			}
		})
	}

	return notifier
}

// DefaultBudgetAlertConfig returns default budget alert configuration
func DefaultBudgetAlertConfig() *BudgetAlertConfig {
	return &BudgetAlertConfig{
		Enabled:             true,
		CooldownPeriod:      24 * time.Hour, // Don't spam alerts
		WebhookEnabled:      false,
		WebhookTimeout:      10 * time.Second,
		CloudWatchEnabled:   false,
		CloudWatchNamespace: "CargoShip/Budget",
		SendProjectAlerts:   true,
		SendGlobalAlerts:    true,
	}
}

// SendAlert sends a budget alert through configured channels
func (n *BudgetAlertNotifier) SendAlert(ctx context.Context, alert *BudgetAlert) error {
	if !n.config.Enabled {
		return nil
	}

	// Check if alert should be sent based on type
	if alert.IsGlobal && !n.config.SendGlobalAlerts {
		return nil
	}
	if !alert.IsGlobal && !n.config.SendProjectAlerts {
		return nil
	}

	// Check cooldown period
	key := "global"
	if alert.ProjectID != "" {
		key = alert.ProjectID
	}

	if lastAlert, ok := n.lastAlertTimes[key]; ok {
		if time.Since(lastAlert) < n.config.CooldownPeriod {
			return nil // Skip alert, in cooldown period
		}
	}

	// Send alert through configured channels
	var lastErr error

	// Send webhook notification
	if n.config.WebhookEnabled && n.config.WebhookURL != "" {
		if err := n.sendWebhookAlert(ctx, alert); err != nil {
			lastErr = fmt.Errorf("webhook alert failed: %w", err)
		}
	}

	// Send CloudWatch alarm
	if n.config.CloudWatchEnabled && n.cloudwatchSvc != nil {
		if err := n.sendCloudWatchAlert(ctx, alert); err != nil {
			if lastErr != nil {
				lastErr = fmt.Errorf("%v; cloudwatch alert failed: %w", lastErr, err)
			} else {
				lastErr = fmt.Errorf("cloudwatch alert failed: %w", err)
			}
		}
	}

	// Update last alert time
	n.lastAlertTimes[key] = time.Now()

	return lastErr
}

// sendWebhookAlert sends an alert via webhook
func (n *BudgetAlertNotifier) sendWebhookAlert(ctx context.Context, alert *BudgetAlert) error {
	// Marshal alert to JSON
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", n.config.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range n.config.WebhookHeaders {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// sendCloudWatchAlert sends an alert to CloudWatch as a custom metric alarm
func (n *BudgetAlertNotifier) sendCloudWatchAlert(ctx context.Context, alert *BudgetAlert) error {
	// Determine metric name based on alert type
	var metricName string
	var metricValue float64
	var dimensions []types.Dimension

	switch alert.Type {
	case AlertTypeCostThreshold, AlertTypeCostOverBudget:
		metricName = "BudgetUsagePercent"
		metricValue = alert.BudgetUsedPercent
		dimensions = []types.Dimension{
			{
				Name:  aws.String("AlertType"),
				Value: aws.String(string(alert.Type)),
			},
		}
		if alert.ProjectID != "" {
			dimensions = append(dimensions, types.Dimension{
				Name:  aws.String("ProjectID"),
				Value: aws.String(alert.ProjectID),
			})
		}

	case AlertTypeVolumeThreshold, AlertTypeVolumeOverQuota:
		metricName = "VolumeUsagePercent"
		metricValue = alert.VolumeUsedPercent
		dimensions = []types.Dimension{
			{
				Name:  aws.String("AlertType"),
				Value: aws.String(string(alert.Type)),
			},
		}
		if alert.ProjectID != "" {
			dimensions = append(dimensions, types.Dimension{
				Name:  aws.String("ProjectID"),
				Value: aws.String(alert.ProjectID),
			})
		}

	default:
		return fmt.Errorf("unsupported alert type for CloudWatch: %s", alert.Type)
	}

	// Put metric data
	_, err := n.cloudwatchSvc.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(n.config.CloudWatchNamespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(metricName),
				Value:      aws.Float64(metricValue),
				Timestamp:  aws.Time(alert.Timestamp),
				Unit:       types.StandardUnitPercent,
				Dimensions: dimensions,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to put CloudWatch metric: %w", err)
	}

	return nil
}

// CheckBudgetStatus evaluates budget status and generates alerts if needed
func (m *Manager) CheckBudgetStatus(ctx context.Context, projectID string) (*BudgetAlert, error) {
	// Get budget status
	status, err := m.GetProjectBudgetStatus(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get budget status: %w", err)
	}

	// Check for cost alerts
	if status.MaxBudget > 0 {
		// Check if over budget (critical)
		if status.OverBudget {
			return &BudgetAlert{
				ID:                 fmt.Sprintf("cost-over-%s-%d", projectID, time.Now().Unix()),
				Timestamp:          time.Now(),
				Type:               AlertTypeCostOverBudget,
				Severity:           SeverityCritical,
				ProjectID:          projectID,
				Description:        fmt.Sprintf("Project %s has exceeded its budget", projectID),
				IsGlobal:           projectID == "",
				MaxBudget:          status.MaxBudget,
				CurrentSpend:       status.CurrentSpend,
				BudgetRemaining:    status.BudgetRemaining,
				BudgetUsedPercent:  status.BudgetUsed * 100,
				ThresholdPercent:   status.MaxBudget * 0.8,
				Recommendation:     "Immediate action required: Budget exceeded. Consider pausing uploads or requesting additional budget.",
				ActionRequired:     true,
			}, nil
		}

		// Check if alert threshold reached (warning)
		if status.AlertTriggered {
			return &BudgetAlert{
				ID:                 fmt.Sprintf("cost-threshold-%s-%d", projectID, time.Now().Unix()),
				Timestamp:          time.Now(),
				Type:               AlertTypeCostThreshold,
				Severity:           SeverityWarning,
				ProjectID:          projectID,
				Description:        fmt.Sprintf("Project %s has reached its budget alert threshold", projectID),
				IsGlobal:           projectID == "",
				MaxBudget:          status.MaxBudget,
				CurrentSpend:       status.CurrentSpend,
				BudgetRemaining:    status.BudgetRemaining,
				BudgetUsedPercent:  status.BudgetUsed * 100,
				ThresholdPercent:   status.MaxBudget * 0.8,
				Recommendation:     fmt.Sprintf("Budget threshold reached. %.2f%% of budget consumed. Consider optimizing uploads or requesting additional budget.", status.BudgetUsed*100),
				ActionRequired:     false,
			}, nil
		}

		// Check if projected to exceed (warning)
		if status.WillExceedBudget {
			return &BudgetAlert{
				ID:                 fmt.Sprintf("cost-projection-%s-%d", projectID, time.Now().Unix()),
				Timestamp:          time.Now(),
				Type:               AlertTypeBudgetProjection,
				Severity:           SeverityWarning,
				ProjectID:          projectID,
				Description:        fmt.Sprintf("Project %s is projected to exceed its budget", projectID),
				IsGlobal:           projectID == "",
				MaxBudget:          status.MaxBudget,
				CurrentSpend:       status.CurrentSpend,
				BudgetRemaining:    status.BudgetRemaining,
				BudgetUsedPercent:  status.BudgetUsed * 100,
				ThresholdPercent:   status.MaxBudget * 0.8,
				Recommendation:     fmt.Sprintf("Current burn rate ($%.2f/day) will exceed budget. Projected end-of-period spend: $%.2f", status.DailyBurnRate, status.ProjectedEOPSpend),
				ActionRequired:     false,
			}, nil
		}
	}

	// Check for volume alerts
	if status.MaxVolumeGB > 0 {
		// Check if over quota (critical)
		if status.OverVolume {
			return &BudgetAlert{
				ID:                     fmt.Sprintf("volume-over-%s-%d", projectID, time.Now().Unix()),
				Timestamp:              time.Now(),
				Type:                   AlertTypeVolumeOverQuota,
				Severity:               SeverityCritical,
				ProjectID:              projectID,
				Description:            fmt.Sprintf("Project %s has exceeded its volume quota", projectID),
				IsGlobal:               projectID == "",
				MaxVolumeGB:            status.MaxVolumeGB,
				CurrentVolumeGB:        status.CurrentVolumeGB,
				VolumeRemaining:        status.VolumeRemaining,
				VolumeUsedPercent:      status.VolumeUsed * 100,
				VolumeThresholdPercent: status.MaxVolumeGB * 0.75,
				Recommendation:         "Immediate action required: Volume quota exceeded. Consider pausing uploads or requesting additional quota.",
				ActionRequired:         true,
			}, nil
		}

		// Check if alert threshold reached (warning)
		if status.VolumeAlertTriggered {
			return &BudgetAlert{
				ID:                     fmt.Sprintf("volume-threshold-%s-%d", projectID, time.Now().Unix()),
				Timestamp:              time.Now(),
				Type:                   AlertTypeVolumeThreshold,
				Severity:               SeverityWarning,
				ProjectID:              projectID,
				Description:            fmt.Sprintf("Project %s has reached its volume alert threshold", projectID),
				IsGlobal:               projectID == "",
				MaxVolumeGB:            status.MaxVolumeGB,
				CurrentVolumeGB:        status.CurrentVolumeGB,
				VolumeRemaining:        status.VolumeRemaining,
				VolumeUsedPercent:      status.VolumeUsed * 100,
				VolumeThresholdPercent: status.MaxVolumeGB * 0.75,
				Recommendation:         fmt.Sprintf("Volume threshold reached. %.2f%% of quota consumed. Consider optimizing data volume or requesting additional quota.", status.VolumeUsed*100),
				ActionRequired:         false,
			}, nil
		}

		// Check if projected to exceed (warning)
		if status.WillExceedVolume {
			return &BudgetAlert{
				ID:                     fmt.Sprintf("volume-projection-%s-%d", projectID, time.Now().Unix()),
				Timestamp:              time.Now(),
				Type:                   AlertTypeVolumeProjection,
				Severity:               SeverityWarning,
				ProjectID:              projectID,
				Description:            fmt.Sprintf("Project %s is projected to exceed its volume quota", projectID),
				IsGlobal:               projectID == "",
				MaxVolumeGB:            status.MaxVolumeGB,
				CurrentVolumeGB:        status.CurrentVolumeGB,
				VolumeRemaining:        status.VolumeRemaining,
				VolumeUsedPercent:      status.VolumeUsed * 100,
				VolumeThresholdPercent: status.MaxVolumeGB * 0.75,
				Recommendation:         fmt.Sprintf("Current volume rate (%.2f GB/day) will exceed quota. Projected end-of-period volume: %.2f GB", status.DailyVolumeBurnRate, status.ProjectedEOPVolume),
				ActionRequired:         false,
			}, nil
		}
	}

	// No alerts triggered
	return nil, nil
}

// CheckAndNotifyBudgetStatus checks budget status and sends alerts if needed
func (m *Manager) CheckAndNotifyBudgetStatus(ctx context.Context, projectID string, notifier *BudgetAlertNotifier) error {
	// Check budget status
	alert, err := m.CheckBudgetStatus(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to check budget status: %w", err)
	}

	// No alert needed
	if alert == nil {
		return nil
	}

	// Send alert
	if err := notifier.SendAlert(ctx, alert); err != nil {
		return fmt.Errorf("failed to send alert: %w", err)
	}

	return nil
}

// MonitorAllBudgets monitors all project budgets and sends alerts as needed
func (m *Manager) MonitorAllBudgets(ctx context.Context, notifier *BudgetAlertNotifier) error {
	// Check global budget
	if err := m.CheckAndNotifyBudgetStatus(ctx, "", notifier); err != nil {
		m.logger.Error("failed to check global budget", "error", err)
	}

	// Check all project budgets
	for projectID := range m.config.ProjectBudgets {
		if err := m.CheckAndNotifyBudgetStatus(ctx, projectID, notifier); err != nil {
			m.logger.Error("failed to check project budget", "project_id", projectID, "error", err)
		}

		// Respect context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}
