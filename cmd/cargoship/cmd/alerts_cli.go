package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// NewAlertsCmd creates the 'alerts' command for alert configuration
func NewAlertsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Manage budget alert notifications",
		Long: `Configure and test budget alert notifications.

Budget alerts support multiple notification channels:
- Webhooks (HTTP POST with JSON payload)
- CloudWatch (AWS CloudWatch metrics and alarms)
- Email (SMTP with TLS encryption)
- Slack (webhook integration with rich formatting)

Features:
- Configure multiple notification channels
- Test alert delivery before enabling
- View current alert configuration
- Enable/disable specific channels

Examples:
  # Show current alert configuration
  cargoship alerts config

  # Configure email notifications
  cargoship alerts configure email \\
    --smtp-host smtp.gmail.com \\
    --smtp-port 587 \\
    --smtp-username alerts@example.com \\
    --smtp-password "app-password" \\
    --smtp-from "cargoship@example.com" \\
    --recipients admin@example.com,ops@example.com

  # Configure Slack notifications
  cargoship alerts configure slack \\
    --webhook-url "https://hooks.slack.com/services/T00/B00/abc123" \\
    --channel "#cargoship-alerts" \\
    --username "CargoShip Monitor"

  # Test alert delivery
  cargoship alerts test

  # Enable/disable specific channels
  cargoship alerts enable email
  cargoship alerts disable slack
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAlertsConfigCmd())
	cmd.AddCommand(newAlertsConfigureCmd())
	cmd.AddCommand(newAlertsTestCmd())
	cmd.AddCommand(newAlertsEnableCmd())
	cmd.AddCommand(newAlertsDisableCmd())

	return cmd
}

func newAlertsConfigCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show current alert configuration",
		Long: `Display the current alert notification configuration.

Shows:
- Enabled notification channels
- Channel-specific configuration
- Alert thresholds and cooldown periods
- Last alert timestamps

Examples:
  # Show configuration
  cargoship alerts config

  # Show as JSON
  cargoship alerts config --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlertsConfig(cmd.Context(), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func newAlertsConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure [channel]",
		Short: "Configure alert notification channels",
		Long: `Configure specific alert notification channels.

Supported channels:
- email:    SMTP email notifications with TLS
- slack:    Slack webhook notifications
- webhook:  Custom HTTP webhook
- cloudwatch: AWS CloudWatch integration

Examples:
  # Configure email
  cargoship alerts configure email \\
    --smtp-host smtp.gmail.com \\
    --smtp-port 587 \\
    --smtp-username alerts@example.com \\
    --smtp-password "app-password" \\
    --smtp-from "cargoship@example.com" \\
    --recipients admin@example.com,ops@example.com

  # Configure Slack
  cargoship alerts configure slack \\
    --webhook-url "https://hooks.slack.com/services/..." \\
    --channel "#cargoship-alerts"
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]
			return runAlertsConfigure(cmd.Context(), channel, cmd)
		},
	}

	// Email flags
	cmd.Flags().String("smtp-host", "", "SMTP server hostname")
	cmd.Flags().Int("smtp-port", 587, "SMTP server port")
	cmd.Flags().String("smtp-username", "", "SMTP username")
	cmd.Flags().String("smtp-password", "", "SMTP password")
	cmd.Flags().String("smtp-from", "", "From email address")
	cmd.Flags().Bool("smtp-use-tls", true, "Use TLS encryption")
	cmd.Flags().StringSlice("recipients", nil, "Email recipients (comma-separated)")

	// Slack flags
	cmd.Flags().String("webhook-url", "", "Slack webhook URL")
	cmd.Flags().String("channel", "", "Slack channel (e.g., #cargoship-alerts)")
	cmd.Flags().String("username", "CargoShip Monitor", "Slack bot username")

	// CloudWatch flags
	cmd.Flags().String("namespace", "CargoShip/Budget", "CloudWatch namespace")

	return cmd
}

func newAlertsTestCmd() *cobra.Command {
	var (
		channel  string
		severity string
	)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test alert notification delivery",
		Long: `Send a test alert to verify notification configuration.

Tests all enabled channels or a specific channel if specified.
The test alert uses sample budget data to demonstrate formatting.

Examples:
  # Test all enabled channels
  cargoship alerts test

  # Test specific channel
  cargoship alerts test --channel email

  # Test with specific severity
  cargoship alerts test --severity critical
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlertsTest(cmd.Context(), channel, severity)
		},
	}

	cmd.Flags().StringVar(&channel, "channel", "", "Test specific channel (email, slack, webhook, cloudwatch)")
	cmd.Flags().StringVar(&severity, "severity", "warning", "Alert severity (info, warning, critical)")

	return cmd
}

func newAlertsEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable [channel]",
		Short: "Enable alert notification channel",
		Long: `Enable a specific alert notification channel.

Supported channels:
- email
- slack
- webhook
- cloudwatch

Examples:
  # Enable email notifications
  cargoship alerts enable email

  # Enable Slack notifications
  cargoship alerts enable slack
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]
			return runAlertsEnable(cmd.Context(), channel)
		},
	}

	return cmd
}

func newAlertsDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable [channel]",
		Short: "Disable alert notification channel",
		Long: `Disable a specific alert notification channel.

Supported channels:
- email
- slack
- webhook
- cloudwatch

Examples:
  # Disable email notifications
  cargoship alerts disable email

  # Disable Slack notifications
  cargoship alerts disable slack
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]
			return runAlertsDisable(cmd.Context(), channel)
		},
	}

	return cmd
}

// Implementation functions

func runAlertsConfig(ctx context.Context, jsonOutput bool) error {
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	config := manager.GetAlertConfig()
	if config == nil {
		fmt.Println("No alert configuration found")
		return nil
	}

	if jsonOutput {
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Format output
	fmt.Println("Budget Alert Configuration")
	fmt.Println("==========================")
	fmt.Println()

	// Overall status
	if config.Enabled {
		fmt.Println("Status:           ✅ Enabled")
	} else {
		fmt.Println("Status:           ⏸️  Disabled")
	}
	fmt.Printf("Cooldown Period:  %v\n", config.CooldownPeriod)
	fmt.Println()

	// Email configuration
	fmt.Println("=== Email Notifications ===")
	if config.EmailEnabled {
		fmt.Println("  Status:         ✅ Enabled")
		fmt.Printf("  SMTP Host:      %s:%d\n", config.SMTPHost, config.SMTPPort)
		fmt.Printf("  From Address:   %s\n", config.SMTPFrom)
		fmt.Printf("  Recipients:     %s\n", strings.Join(config.EmailRecipients, ", "))
		if config.SMTPUseTLS {
			fmt.Println("  TLS:            ✅ Enabled")
		} else {
			fmt.Println("  TLS:            ⚠️  Disabled")
		}
	} else {
		fmt.Println("  Status:         ⏸️  Disabled")
	}
	fmt.Println()

	// Slack configuration
	fmt.Println("=== Slack Notifications ===")
	if config.SlackEnabled {
		fmt.Println("  Status:         ✅ Enabled")
		if config.SlackChannel != "" {
			fmt.Printf("  Channel:        %s\n", config.SlackChannel)
		}
		if config.SlackUsername != "" {
			fmt.Printf("  Username:       %s\n", config.SlackUsername)
		}
		fmt.Println("  Webhook:        (configured)")
	} else {
		fmt.Println("  Status:         ⏸️  Disabled")
	}
	fmt.Println()

	// Webhook configuration
	fmt.Println("=== Webhook Notifications ===")
	if config.WebhookEnabled {
		fmt.Println("  Status:         ✅ Enabled")
		fmt.Printf("  Timeout:        %v\n", config.WebhookTimeout)
		if config.WebhookURL != "" {
			fmt.Printf("  URL:            %s\n", config.WebhookURL)
		}
	} else {
		fmt.Println("  Status:         ⏸️  Disabled")
	}
	fmt.Println()

	// CloudWatch configuration
	fmt.Println("=== CloudWatch Integration ===")
	if config.CloudWatchEnabled {
		fmt.Println("  Status:         ✅ Enabled")
		fmt.Printf("  Namespace:      %s\n", config.CloudWatchNamespace)
	} else {
		fmt.Println("  Status:         ⏸️  Disabled")
	}

	return nil
}

func runAlertsConfigure(ctx context.Context, channel string, cmd *cobra.Command) error {
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	config := manager.GetAlertConfig()
	if config == nil {
		config = cost.DefaultBudgetAlertConfig()
	}

	switch strings.ToLower(channel) {
	case "email":
		// Get email configuration from flags
		smtpHost, _ := cmd.Flags().GetString("smtp-host")
		smtpPort, _ := cmd.Flags().GetInt("smtp-port")
		smtpUsername, _ := cmd.Flags().GetString("smtp-username")
		smtpPassword, _ := cmd.Flags().GetString("smtp-password")
		smtpFrom, _ := cmd.Flags().GetString("smtp-from")
		smtpUseTLS, _ := cmd.Flags().GetBool("smtp-use-tls")
		recipients, _ := cmd.Flags().GetStringSlice("recipients")

		if smtpHost == "" {
			return fmt.Errorf("--smtp-host is required")
		}
		if smtpFrom == "" {
			return fmt.Errorf("--smtp-from is required")
		}
		if len(recipients) == 0 {
			return fmt.Errorf("--recipients is required")
		}

		config.EmailEnabled = true
		config.SMTPHost = smtpHost
		config.SMTPPort = smtpPort
		config.SMTPUsername = smtpUsername
		config.SMTPPassword = smtpPassword
		config.SMTPFrom = smtpFrom
		config.SMTPUseTLS = smtpUseTLS
		config.EmailRecipients = recipients

		fmt.Println("✅ Email notifications configured")
		fmt.Printf("   SMTP Host:      %s:%d\n", smtpHost, smtpPort)
		fmt.Printf("   From:           %s\n", smtpFrom)
		fmt.Printf("   Recipients:     %s\n", strings.Join(recipients, ", "))
		if smtpUseTLS {
			fmt.Println("   TLS:            ✅ Enabled")
		}

	case "slack":
		webhookURL, _ := cmd.Flags().GetString("webhook-url")
		slackChannel, _ := cmd.Flags().GetString("channel")
		username, _ := cmd.Flags().GetString("username")

		if webhookURL == "" {
			return fmt.Errorf("--webhook-url is required")
		}

		config.SlackEnabled = true
		config.SlackWebhookURL = webhookURL
		config.SlackChannel = slackChannel
		config.SlackUsername = username

		fmt.Println("✅ Slack notifications configured")
		if slackChannel != "" {
			fmt.Printf("   Channel:        %s\n", slackChannel)
		}
		if username != "" {
			fmt.Printf("   Username:       %s\n", username)
		}

	case "webhook":
		webhookURL, _ := cmd.Flags().GetString("webhook-url")
		if webhookURL == "" {
			return fmt.Errorf("--webhook-url is required")
		}

		config.WebhookEnabled = true
		config.WebhookURL = webhookURL

		fmt.Println("✅ Webhook notifications configured")
		fmt.Printf("   URL:            %s\n", webhookURL)

	case "cloudwatch":
		namespace, _ := cmd.Flags().GetString("namespace")
		config.CloudWatchEnabled = true
		config.CloudWatchNamespace = namespace

		fmt.Println("✅ CloudWatch integration configured")
		fmt.Printf("   Namespace:      %s\n", namespace)

	default:
		return fmt.Errorf("unknown channel: %s (supported: email, slack, webhook, cloudwatch)", channel)
	}

	// Save updated configuration
	if err := manager.UpdateAlertConfig(config); err != nil {
		return fmt.Errorf("failed to save alert configuration: %w", err)
	}

	return nil
}

func runAlertsTest(ctx context.Context, channel, severity string) error {
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Create test alert
	alert := &cost.BudgetAlert{
		Type:               cost.AlertTypeCostThreshold,
		Severity:           parseSeverity(severity),
		Description:        "This is a test alert from CargoShip",
		Timestamp:          time.Now(),
		ProjectID:          "test-project",
		IsGlobal:           false,
		MaxBudget:          1000.00,
		CurrentSpend:       850.00,
		BudgetRemaining:    150.00,
		BudgetUsedPercent:  85.0,
		ThresholdPercent:   80.0,
		ActionRequired:     true,
		Recommendation:     "Consider reducing spending or increasing budget allocation",
	}

	fmt.Println("Sending test alert...")
	fmt.Printf("  Severity:       %s\n", severity)
	if channel != "" {
		fmt.Printf("  Channel:        %s\n", channel)
	} else {
		fmt.Println("  Channels:       all enabled")
	}
	fmt.Println()

	// Send alert (filter by channel if specified)
	if err := manager.SendTestAlert(ctx, alert, channel); err != nil {
		return fmt.Errorf("failed to send test alert: %w", err)
	}

	fmt.Println("✅ Test alert sent successfully")
	fmt.Println()
	fmt.Println("Please check your configured notification channels:")
	fmt.Println("  - Email inbox")
	fmt.Println("  - Slack channel")
	fmt.Println("  - Webhook endpoint logs")
	fmt.Println("  - CloudWatch console")

	return nil
}

func runAlertsEnable(ctx context.Context, channel string) error {
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	config := manager.GetAlertConfig()
	if config == nil {
		return fmt.Errorf("no alert configuration found. Run 'cargoship alerts configure' first")
	}

	switch strings.ToLower(channel) {
	case "email":
		if config.SMTPHost == "" {
			return fmt.Errorf("email not configured. Run 'cargoship alerts configure email' first")
		}
		config.EmailEnabled = true
		fmt.Println("✅ Email notifications enabled")

	case "slack":
		if config.SlackWebhookURL == "" {
			return fmt.Errorf("slack not configured. Run 'cargoship alerts configure slack' first")
		}
		config.SlackEnabled = true
		fmt.Println("✅ Slack notifications enabled")

	case "webhook":
		if config.WebhookURL == "" {
			return fmt.Errorf("webhook not configured. Run 'cargoship alerts configure webhook' first")
		}
		config.WebhookEnabled = true
		fmt.Println("✅ Webhook notifications enabled")

	case "cloudwatch":
		config.CloudWatchEnabled = true
		fmt.Println("✅ CloudWatch integration enabled")

	default:
		return fmt.Errorf("unknown channel: %s (supported: email, slack, webhook, cloudwatch)", channel)
	}

	if err := manager.UpdateAlertConfig(config); err != nil {
		return fmt.Errorf("failed to save alert configuration: %w", err)
	}

	return nil
}

func runAlertsDisable(ctx context.Context, channel string) error {
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	config := manager.GetAlertConfig()
	if config == nil {
		return fmt.Errorf("no alert configuration found")
	}

	switch strings.ToLower(channel) {
	case "email":
		config.EmailEnabled = false
		fmt.Println("⏸️  Email notifications disabled")

	case "slack":
		config.SlackEnabled = false
		fmt.Println("⏸️  Slack notifications disabled")

	case "webhook":
		config.WebhookEnabled = false
		fmt.Println("⏸️  Webhook notifications disabled")

	case "cloudwatch":
		config.CloudWatchEnabled = false
		fmt.Println("⏸️  CloudWatch integration disabled")

	default:
		return fmt.Errorf("unknown channel: %s (supported: email, slack, webhook, cloudwatch)", channel)
	}

	if err := manager.UpdateAlertConfig(config); err != nil {
		return fmt.Errorf("failed to save alert configuration: %w", err)
	}

	return nil
}

// Helper functions

func parseSeverity(severity string) cost.BudgetAlertSeverity {
	switch strings.ToLower(severity) {
	case "critical":
		return cost.SeverityCritical
	case "warning":
		return cost.SeverityWarning
	case "info":
		return cost.SeverityInfo
	default:
		return cost.SeverityWarning
	}
}
