// Package cost provides budget alert notification system
package cost

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
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
	Type     BudgetAlertType     `json:"type"`     // cost_threshold, volume_threshold, cost_over_budget, volume_over_quota
	Severity BudgetAlertSeverity `json:"severity"` // info, warning, critical

	// Budget/Project context
	ProjectID   string `json:"project_id,omitempty"` // Empty if global budget
	Description string `json:"description"`          // Human-readable description
	IsGlobal    bool   `json:"is_global"`            // True if global budget alert

	// Cost metrics (if cost alert)
	MaxBudget         float64 `json:"max_budget,omitempty"`
	CurrentSpend      float64 `json:"current_spend,omitempty"`
	BudgetRemaining   float64 `json:"budget_remaining,omitempty"`
	BudgetUsedPercent float64 `json:"budget_used_percent,omitempty"`
	ThresholdPercent  float64 `json:"threshold_percent,omitempty"`

	// Volume metrics (if volume alert)
	MaxVolumeGB            float64 `json:"max_volume_gb,omitempty"`
	CurrentVolumeGB        float64 `json:"current_volume_gb,omitempty"`
	VolumeRemaining        float64 `json:"volume_remaining,omitempty"`
	VolumeUsedPercent      float64 `json:"volume_used_percent,omitempty"`
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
	WebhookURL     string            `yaml:"webhook_url" json:"-"`     // excluded from JSON: contains secret URL
	WebhookHeaders map[string]string `yaml:"webhook_headers" json:"-"` // may contain auth headers
	WebhookTimeout time.Duration     `yaml:"webhook_timeout" json:"webhook_timeout"`

	// CloudWatch alarm configuration
	CloudWatchEnabled   bool   `yaml:"cloudwatch_enabled" json:"cloudwatch_enabled"`
	CloudWatchNamespace string `yaml:"cloudwatch_namespace" json:"cloudwatch_namespace"`
	CloudWatchRegion    string `yaml:"cloudwatch_region" json:"cloudwatch_region"`

	// Email notification configuration (Issue #147 Phase 4)
	EmailEnabled    bool     `yaml:"email_enabled" json:"email_enabled"`
	EmailRecipients []string `yaml:"email_recipients" json:"email_recipients,omitempty"`
	SMTPHost        string   `yaml:"smtp_host" json:"smtp_host,omitempty"`
	SMTPPort        int      `yaml:"smtp_port" json:"smtp_port,omitempty"`
	SMTPUsername    string   `yaml:"smtp_username" json:"smtp_username,omitempty"`
	SMTPPassword    string   `yaml:"smtp_password" json:"-"` // excluded from JSON: credential
	SMTPFrom        string   `yaml:"smtp_from" json:"smtp_from,omitempty"`
	SMTPUseTLS      bool     `yaml:"smtp_use_tls" json:"smtp_use_tls"`

	// Slack notification configuration (Issue #147 Phase 4)
	SlackEnabled    bool   `yaml:"slack_enabled" json:"slack_enabled"`
	SlackWebhookURL string `yaml:"slack_webhook_url" json:"-"`                     // excluded from JSON: secret webhook URL
	SlackChannel    string `yaml:"slack_channel" json:"slack_channel,omitempty"`   // Optional override
	SlackUsername   string `yaml:"slack_username" json:"slack_username,omitempty"` // Optional bot name

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
		EmailEnabled:        false, // Issue #147 Phase 4
		SMTPPort:            587,   // Default SMTP port with STARTTLS
		SMTPUseTLS:          true,  // Enable TLS by default
		SlackEnabled:        false, // Issue #147 Phase 4
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

	// Send email notification (Issue #147 Phase 4)
	if n.config.EmailEnabled && len(n.config.EmailRecipients) > 0 && n.config.SMTPHost != "" {
		if err := n.sendEmailAlert(ctx, alert); err != nil {
			if lastErr != nil {
				lastErr = fmt.Errorf("%v; email alert failed: %w", lastErr, err)
			} else {
				lastErr = fmt.Errorf("email alert failed: %w", err)
			}
		}
	}

	// Send Slack notification (Issue #147 Phase 4)
	if n.config.SlackEnabled && n.config.SlackWebhookURL != "" {
		if err := n.sendSlackAlert(ctx, alert); err != nil {
			if lastErr != nil {
				lastErr = fmt.Errorf("%v; slack alert failed: %w", lastErr, err)
			} else {
				lastErr = fmt.Errorf("slack alert failed: %w", err)
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

// sendEmailAlert sends an alert via email (Issue #147 Phase 4)
func (n *BudgetAlertNotifier) sendEmailAlert(ctx context.Context, alert *BudgetAlert) error {
	// Validate configuration
	if len(n.config.EmailRecipients) == 0 {
		return fmt.Errorf("no email recipients configured")
	}
	if n.config.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured")
	}
	if n.config.SMTPFrom == "" {
		return fmt.Errorf("SMTP from address not configured")
	}

	// Build email subject
	subject := fmt.Sprintf("[CargoShip Budget Alert] %s - %s", alert.Severity, alert.Type)
	if alert.ProjectID != "" {
		subject = fmt.Sprintf("%s - Project: %s", subject, alert.ProjectID)
	}

	// Build email body
	body := n.formatEmailBody(alert)

	// Construct email message
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		n.config.SMTPFrom,
		strings.Join(n.config.EmailRecipients, ", "),
		subject,
		body)

	// Send via SMTP
	addr := fmt.Sprintf("%s:%d", n.config.SMTPHost, n.config.SMTPPort)

	// Determine authentication
	var auth smtp.Auth
	if n.config.SMTPUsername != "" && n.config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", n.config.SMTPUsername, n.config.SMTPPassword, n.config.SMTPHost)
	}

	// Send with or without TLS
	if n.config.SMTPUseTLS {
		return n.sendEmailTLS(addr, auth, n.config.SMTPFrom, n.config.EmailRecipients, []byte(msg))
	}

	return smtp.SendMail(addr, auth, n.config.SMTPFrom, n.config.EmailRecipients, []byte(msg))
}

// sendEmailTLS sends email with TLS encryption
func (n *BudgetAlertNotifier) sendEmailTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	// Connect with TLS
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: n.config.SMTPHost,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect with TLS: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client, err := smtp.NewClient(conn, n.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	// Authenticate if configured
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", addr, err)
		}
	}

	// Send message body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to initiate data transfer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}

// formatEmailBody formats the alert as an email body
func (n *BudgetAlertNotifier) formatEmailBody(alert *BudgetAlert) string {
	var buf bytes.Buffer

	buf.WriteString("CargoShip Budget Alert\n")
	buf.WriteString("======================\n\n")
	fmt.Fprintf(&buf, "Severity: %s\n", alert.Severity)
	fmt.Fprintf(&buf, "Type: %s\n", alert.Type)
	fmt.Fprintf(&buf, "Time: %s\n\n", alert.Timestamp.Format(time.RFC3339))

	if alert.ProjectID != "" {
		fmt.Fprintf(&buf, "Project: %s\n", alert.ProjectID)
	} else {
		buf.WriteString("Scope: Global\n")
	}

	fmt.Fprintf(&buf, "\n%s\n\n", alert.Description)

	// Cost details
	if alert.MaxBudget > 0 {
		buf.WriteString("Budget Details:\n")
		fmt.Fprintf(&buf, "  Maximum Budget: $%.2f\n", alert.MaxBudget)
		fmt.Fprintf(&buf, "  Current Spend: $%.2f\n", alert.CurrentSpend)
		fmt.Fprintf(&buf, "  Remaining: $%.2f\n", alert.BudgetRemaining)
		fmt.Fprintf(&buf, "  Used: %.1f%%\n\n", alert.BudgetUsedPercent)
	}

	// Volume details
	if alert.MaxVolumeGB > 0 {
		buf.WriteString("Volume Details:\n")
		fmt.Fprintf(&buf, "  Maximum Quota: %.2f GB\n", alert.MaxVolumeGB)
		fmt.Fprintf(&buf, "  Current Volume: %.2f GB\n", alert.CurrentVolumeGB)
		fmt.Fprintf(&buf, "  Remaining: %.2f GB\n", alert.VolumeRemaining)
		fmt.Fprintf(&buf, "  Used: %.1f%%\n\n", alert.VolumeUsedPercent)
	}

	if alert.Recommendation != "" {
		fmt.Fprintf(&buf, "Recommendation:\n%s\n\n", alert.Recommendation)
	}

	if alert.ActionRequired {
		buf.WriteString("⚠️ IMMEDIATE ACTION REQUIRED ⚠️\n\n")
	}

	buf.WriteString("--\nCargoShip Budget Alert System\n")
	buf.WriteString("https://github.com/scttfrdmn/cargoship\n")

	return buf.String()
}

// sendSlackAlert sends an alert via Slack webhook (Issue #147 Phase 4)
func (n *BudgetAlertNotifier) sendSlackAlert(ctx context.Context, alert *BudgetAlert) error {
	if n.config.SlackWebhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	// Build Slack message payload
	payload := n.buildSlackPayload(alert)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", n.config.SlackWebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack webhook: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned status %d", resp.StatusCode)
	}

	return nil
}

// buildSlackPayload builds a Slack message payload
func (n *BudgetAlertNotifier) buildSlackPayload(alert *BudgetAlert) map[string]interface{} {
	// Determine color based on severity
	var color string
	var emoji string
	switch alert.Severity {
	case SeverityCritical:
		color = "danger"
		emoji = ":rotating_light:"
	case SeverityWarning:
		color = "warning"
		emoji = ":warning:"
	default:
		color = "good"
		emoji = ":information_source:"
	}

	// Build attachment fields
	fields := []map[string]interface{}{
		{
			"title": "Severity",
			"value": string(alert.Severity),
			"short": true,
		},
		{
			"title": "Alert Type",
			"value": string(alert.Type),
			"short": true,
		},
	}

	if alert.ProjectID != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Project",
			"value": alert.ProjectID,
			"short": true,
		})
	} else {
		fields = append(fields, map[string]interface{}{
			"title": "Scope",
			"value": "Global",
			"short": true,
		})
	}

	// Add budget details
	if alert.MaxBudget > 0 {
		fields = append(fields,
			map[string]interface{}{
				"title": "Current Spend",
				"value": fmt.Sprintf("$%.2f", alert.CurrentSpend),
				"short": true,
			},
			map[string]interface{}{
				"title": "Budget",
				"value": fmt.Sprintf("$%.2f", alert.MaxBudget),
				"short": true,
			},
			map[string]interface{}{
				"title": "Budget Used",
				"value": fmt.Sprintf("%.1f%%", alert.BudgetUsedPercent),
				"short": true,
			},
		)
	}

	// Add volume details
	if alert.MaxVolumeGB > 0 {
		fields = append(fields,
			map[string]interface{}{
				"title": "Current Volume",
				"value": fmt.Sprintf("%.2f GB", alert.CurrentVolumeGB),
				"short": true,
			},
			map[string]interface{}{
				"title": "Quota",
				"value": fmt.Sprintf("%.2f GB", alert.MaxVolumeGB),
				"short": true,
			},
			map[string]interface{}{
				"title": "Volume Used",
				"value": fmt.Sprintf("%.1f%%", alert.VolumeUsedPercent),
				"short": true,
			},
		)
	}

	// Build payload
	payload := map[string]interface{}{
		"username":   n.getSlackUsername(),
		"icon_emoji": ":moneybag:",
		"attachments": []map[string]interface{}{
			{
				"color":       color,
				"title":       fmt.Sprintf("%s %s", emoji, alert.Description),
				"text":        alert.Recommendation,
				"fields":      fields,
				"footer":      "CargoShip",
				"footer_icon": "https://github.com/scttfrdmn/cargoship/raw/main/docs/logo.png",
				"ts":          alert.Timestamp.Unix(),
			},
		},
	}

	if n.config.SlackChannel != "" {
		payload["channel"] = n.config.SlackChannel
	}

	if alert.ActionRequired {
		payload["text"] = "<!here> *IMMEDIATE ACTION REQUIRED*"
	}

	return payload
}

// getSlackUsername returns the configured Slack username or default
func (n *BudgetAlertNotifier) getSlackUsername() string {
	if n.config.SlackUsername != "" {
		return n.config.SlackUsername
	}
	return "CargoShip Budget Alerts"
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
				ID:                fmt.Sprintf("cost-over-%s-%d", projectID, time.Now().Unix()),
				Timestamp:         time.Now(),
				Type:              AlertTypeCostOverBudget,
				Severity:          SeverityCritical,
				ProjectID:         projectID,
				Description:       fmt.Sprintf("Project %s has exceeded its budget", projectID),
				IsGlobal:          projectID == "",
				MaxBudget:         status.MaxBudget,
				CurrentSpend:      status.CurrentSpend,
				BudgetRemaining:   status.BudgetRemaining,
				BudgetUsedPercent: status.BudgetUsed * 100,
				ThresholdPercent:  status.MaxBudget * 0.8,
				Recommendation:    "Immediate action required: Budget exceeded. Consider pausing uploads or requesting additional budget.",
				ActionRequired:    true,
			}, nil
		}

		// Check if alert threshold reached (warning)
		if status.AlertTriggered {
			return &BudgetAlert{
				ID:                fmt.Sprintf("cost-threshold-%s-%d", projectID, time.Now().Unix()),
				Timestamp:         time.Now(),
				Type:              AlertTypeCostThreshold,
				Severity:          SeverityWarning,
				ProjectID:         projectID,
				Description:       fmt.Sprintf("Project %s has reached its budget alert threshold", projectID),
				IsGlobal:          projectID == "",
				MaxBudget:         status.MaxBudget,
				CurrentSpend:      status.CurrentSpend,
				BudgetRemaining:   status.BudgetRemaining,
				BudgetUsedPercent: status.BudgetUsed * 100,
				ThresholdPercent:  status.MaxBudget * 0.8,
				Recommendation:    fmt.Sprintf("Budget threshold reached. %.2f%% of budget consumed. Consider optimizing uploads or requesting additional budget.", status.BudgetUsed*100),
				ActionRequired:    false,
			}, nil
		}

		// Check if projected to exceed (warning)
		if status.WillExceedBudget {
			return &BudgetAlert{
				ID:                fmt.Sprintf("cost-projection-%s-%d", projectID, time.Now().Unix()),
				Timestamp:         time.Now(),
				Type:              AlertTypeBudgetProjection,
				Severity:          SeverityWarning,
				ProjectID:         projectID,
				Description:       fmt.Sprintf("Project %s is projected to exceed its budget", projectID),
				IsGlobal:          projectID == "",
				MaxBudget:         status.MaxBudget,
				CurrentSpend:      status.CurrentSpend,
				BudgetRemaining:   status.BudgetRemaining,
				BudgetUsedPercent: status.BudgetUsed * 100,
				ThresholdPercent:  status.MaxBudget * 0.8,
				Recommendation:    fmt.Sprintf("Current burn rate ($%.2f/day) will exceed budget. Projected end-of-period spend: $%.2f", status.DailyBurnRate, status.ProjectedEOPSpend),
				ActionRequired:    false,
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
