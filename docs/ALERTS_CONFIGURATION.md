# CargoShip Budget Alert Configuration Guide

**Version:** v0.6.0
**Last Updated:** 2025-12-09

## Table of Contents

1. [Overview](#overview)
2. [Alert Types & Severities](#alert-types--severities)
3. [Email (SMTP) Configuration](#email-smtp-configuration)
4. [Slack Webhook Configuration](#slack-webhook-configuration)
5. [Custom Webhook Configuration](#custom-webhook-configuration)
6. [CloudWatch Integration](#cloudwatch-integration)
7. [Testing Alerts](#testing-alerts)
8. [Managing Channels](#managing-channels)
9. [Alert Payload Format](#alert-payload-format)
10. [Troubleshooting](#troubleshooting)
11. [Best Practices](#best-practices)

## Overview

CargoShip's budget alert system provides real-time notifications when budgets or volume quotas approach or exceed configured thresholds. Alerts are delivered through multiple channels with graceful failure handling - one channel failure doesn't block others.

### Supported Channels

- **Email (SMTP)**: TLS 1.2+ encrypted email notifications
- **Slack**: Rich webhook integration with color-coded messages
- **Custom Webhooks**: HTTP POST with JSON payload
- **CloudWatch**: AWS CloudWatch metrics and alarms

### Key Features

- **Multi-Channel**: Send alerts to multiple destinations simultaneously
- **Graceful Failure**: One channel failure doesn't block others
- **Alert Cooldown**: Prevents notification spam (configurable period)
- **Channel Filtering**: Test specific channels before enabling
- **Security**: All channels disabled by default (opt-in)

## Alert Types & Severities

### Alert Types

| Type | Description | Trigger Condition |
|------|-------------|-------------------|
| `cost_threshold` | Cost exceeded alert threshold | Current spend > (MaxBudget × AlertThreshold) |
| `volume_threshold` | Volume exceeded alert threshold | Current volume > (MaxVolume × VolumeAlertThreshold) |
| `cost_over_budget` | Cost exceeded maximum budget | Current spend > MaxBudget |
| `volume_over_quota` | Volume exceeded maximum quota | Current volume > MaxVolume |
| `budget_projection` | Projected costs will exceed budget | Forecast shows budget exhaustion |
| `volume_projection` | Projected volume will exceed quota | Forecast shows quota exhaustion |

### Severity Levels

| Severity | Color (Slack) | Condition |
|----------|---------------|-----------|
| `info` | Blue (#36a64f) | Below thresholds, informational |
| `warning` | Orange (#ff9900) | Between threshold and maximum |
| `critical` | Red (#ff0000) | Above maximum or immediate action required |

### Alert Flow

```
Operation → Budget Check → Threshold Evaluation → Alert Generation → Multi-Channel Delivery
                                    ↓
                              Alert Cooldown
                              (prevents spam)
```

## Email (SMTP) Configuration

### Quick Start

```bash
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --smtp-username alerts@example.com \
  --smtp-password "your-app-password" \
  --smtp-from "cargoship@example.com" \
  --recipients admin@example.com,finance@example.com
```

### Configuration Parameters

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `--smtp-host` | SMTP server hostname | `smtp.gmail.com` | Yes |
| `--smtp-port` | SMTP server port | `587` (TLS) or `465` (SSL) | Yes |
| `--smtp-username` | SMTP authentication username | `alerts@example.com` | Yes |
| `--smtp-password` | SMTP authentication password | `app-password` | Yes |
| `--smtp-from` | From email address | `cargoship@example.com` | Yes |
| `--recipients` | Comma-separated recipient list | `admin@example.com,ops@example.com` | Yes |
| `--smtp-use-tls` | Use TLS encryption | `true` (default) | No |

### Gmail Configuration

Gmail requires **App Passwords** for SMTP authentication (not your regular password).

**Step-by-Step Setup:**

1. **Enable 2-Step Verification**
   - Go to [Google Account Security](https://myaccount.google.com/security)
   - Enable "2-Step Verification"

2. **Generate App Password**
   - Go to [App Passwords](https://myaccount.google.com/apppasswords)
   - Select "Mail" as the app
   - Select "Other" as the device
   - Enter "CargoShip" as the name
   - Click "Generate"
   - Copy the 16-character password

3. **Configure CargoShip**
   ```bash
   cargoship alerts configure email \
     --smtp-host smtp.gmail.com \
     --smtp-port 587 \
     --smtp-username your-email@gmail.com \
     --smtp-password "abcd efgh ijkl mnop" \
     --smtp-from "your-email@gmail.com" \
     --recipients admin@company.com
   ```

4. **Test Configuration**
   ```bash
   cargoship alerts test --channel email --severity warning
   ```

### Office 365 Configuration

```bash
cargoship alerts configure email \
  --smtp-host smtp.office365.com \
  --smtp-port 587 \
  --smtp-username alerts@company.com \
  --smtp-password "your-password" \
  --smtp-from "alerts@company.com" \
  --recipients admin@company.com,finance@company.com
```

### AWS SES Configuration

```bash
cargoship alerts configure email \
  --smtp-host email-smtp.us-west-2.amazonaws.com \
  --smtp-port 587 \
  --smtp-username "AWS-SES-SMTP-USERNAME" \
  --smtp-password "AWS-SES-SMTP-PASSWORD" \
  --smtp-from "verified@example.com" \
  --recipients admin@example.com
```

**Prerequisites:**
- Verify email address or domain in SES
- Create SMTP credentials in SES console
- Move out of SES sandbox (for production)

### Custom SMTP Server

```bash
cargoship alerts configure email \
  --smtp-host mail.company.com \
  --smtp-port 465 \
  --smtp-username alerts \
  --smtp-password "password" \
  --smtp-from "cargoship@company.com" \
  --smtp-use-tls false \
  --recipients team@company.com
```

### Email Format

**Subject:**
```
[CargoShip Alert] CRITICAL: Project 20251206-abc123 exceeded maximum budget
```

**Body:**
```
Budget Alert - CRITICAL

Project: 20251206-abc123
Type: cost_over_budget
Severity: critical
Timestamp: 2025-12-09 10:30:00 UTC

Cost Budget Status:
  Maximum Budget:    $1,000.00
  Current Spending:  $1,050.00
  Budget Remaining:  -$50.00
  Usage:             105.0%

Recommendation: URGENT: Halt operations or increase budget

This alert was generated by CargoShip Budget Management System.
To configure alerts: cargoship alerts configure
```

## Slack Webhook Configuration

### Quick Start

```bash
cargoship alerts configure slack \
  --webhook-url "https://hooks.slack.com/services/T00/B00/abc123" \
  --channel "#cargoship-alerts" \
  --username "CargoShip Monitor"
```

### Slack App Setup

**Step-by-Step:**

1. **Create Slack App**
   - Go to [https://api.slack.com/apps](https://api.slack.com/apps)
   - Click "Create New App"
   - Select "From scratch"
   - Enter app name: "CargoShip Alerts"
   - Select your workspace
   - Click "Create App"

2. **Enable Incoming Webhooks**
   - In app settings, click "Incoming Webhooks"
   - Toggle "Activate Incoming Webhooks" to ON
   - Click "Add New Webhook to Workspace"
   - Select the channel (e.g., `#cargoship-alerts`)
   - Click "Allow"
   - Copy the webhook URL

3. **Configure CargoShip**
   ```bash
   cargoship alerts configure slack \
     --webhook-url "https://hooks.slack.com/services/YOUR/WEBHOOK/URL" \
     --channel "#cargoship-alerts" \
     --username "CargoShip"
   ```

4. **Test Configuration**
   ```bash
   cargoship alerts test --channel slack --severity critical
   ```

### Configuration Parameters

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `--webhook-url` | Slack webhook URL | `https://hooks.slack.com/services/...` | Yes |
| `--channel` | Channel override (optional) | `#cargoship-alerts` | No |
| `--username` | Bot username (optional) | `CargoShip Monitor` | No |

### Slack Message Format

**Example Alert:**

```
🚨 CRITICAL: Budget Exceeded

Project: 20251206-abc123
Type: cost_over_budget

Cost Budget:
• Maximum: $1,000.00
• Current: $1,050.00
• Overage: $50.00 (105.0%)

⚠️ URGENT: Halt operations or increase budget

Timestamp: 2025-12-09 10:30:00 UTC
```

**Color Coding:**
- **Info**: Green (`#36a64f`)
- **Warning**: Orange (`#ff9900`)
- **Critical**: Red (`#ff0000`)

### Advanced Slack Configuration

**Multiple Channels:**

Configure separate webhook URLs for different severity levels:

```bash
# Critical alerts to #ops-critical
cargoship alerts configure slack \
  --webhook-url "https://hooks.slack.com/services/T00/B00/critical" \
  --channel "#ops-critical"

# Info alerts to #monitoring
cargoship alerts configure slack \
  --webhook-url "https://hooks.slack.com/services/T00/B00/monitoring" \
  --channel "#monitoring"
```

## Custom Webhook Configuration

### Quick Start

```bash
cargoship alerts configure webhook \
  --webhook-url "https://your-server.com/api/alerts" \
  --headers "Authorization: Bearer token123,X-API-Key: key456"
```

### Configuration Parameters

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `--webhook-url` | Webhook endpoint URL | `https://api.company.com/alerts` | Yes |
| `--headers` | Custom HTTP headers (comma-separated) | `Authorization: Bearer xyz` | No |

### Webhook Payload

**HTTP Method:** `POST`

**Content-Type:** `application/json`

**Payload Structure:**

```json
{
  "id": "alert-1733745000",
  "timestamp": "2025-12-09T10:30:00Z",
  "type": "cost_over_budget",
  "severity": "critical",
  "project_id": "20251206-abc123",
  "description": "Project exceeded maximum budget",
  "is_global": false,
  "max_budget": 1000.00,
  "current_spend": 1050.00,
  "budget_remaining": -50.00,
  "budget_used_percent": 1.05,
  "threshold_percent": 0.80,
  "recommendation": "URGENT: Halt operations or increase budget",
  "action_required": true
}
```

### Example Webhook Server (Go)

```go
package main

import (
    "encoding/json"
    "log"
    "net/http"
    "github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

func handleAlert(w http.ResponseWriter, r *http.Request) {
    var alert cost.BudgetAlert
    if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    log.Printf("Received alert: %s - %s", alert.Severity, alert.Description)

    // Process alert (send to monitoring system, create ticket, etc.)

    w.WriteHeader(http.StatusOK)
}

func main() {
    http.HandleFunc("/api/alerts", handleAlert)
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### Example Webhook Server (Python)

```python
from flask import Flask, request, jsonify
import logging

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

@app.route('/api/alerts', methods=['POST'])
def handle_alert():
    alert = request.get_json()

    logging.info(f"Received alert: {alert['severity']} - {alert['description']}")

    # Process alert
    if alert['severity'] == 'critical':
        # Create incident ticket
        # Send to PagerDuty
        pass

    return jsonify({'status': 'received'}), 200

if __name__ == '__main__':
    app.run(port=8080)
```

## CloudWatch Integration

### Quick Start

```bash
cargoship alerts configure cloudwatch \
  --namespace "CargoShip/Production" \
  --metric-name "BudgetAlert"
```

### Configuration Parameters

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `--namespace` | CloudWatch namespace | `CargoShip/Production` | Yes |
| `--metric-name` | Metric name | `BudgetAlert` | Yes |

### CloudWatch Metrics

**Metric Dimensions:**

```
Namespace: CargoShip/Production
Metric: BudgetAlert
Dimensions:
  - ProjectID: 20251206-abc123
  - AlertType: cost_over_budget
  - Severity: critical
```

**Metric Value:** `1` for each alert

### Creating CloudWatch Alarms

```bash
aws cloudwatch put-metric-alarm \
  --alarm-name cargoship-critical-budget-alerts \
  --alarm-description "Alert when budget is critically exceeded" \
  --metric-name BudgetAlert \
  --namespace CargoShip/Production \
  --statistic Sum \
  --period 300 \
  --evaluation-periods 1 \
  --threshold 1 \
  --comparison-operator GreaterThanOrEqualToThreshold \
  --dimensions Name=Severity,Value=critical \
  --alarm-actions arn:aws:sns:us-west-2:123456789012:cargoship-alerts
```

### CloudWatch Logs Integration

```bash
# View budget alerts in CloudWatch Logs
aws logs tail /aws/cargoship/budget-alerts --follow
```

## Testing Alerts

### Test All Channels

```bash
cargoship alerts test
```

Output:
```
Testing budget alert system...

📧 Email (SMTP):           ✅ Sent successfully
💬 Slack:                  ✅ Sent successfully
🔗 Webhook:                ✅ Sent successfully (200 OK)
☁️  CloudWatch:             ✅ Metric published

✅ All alert channels tested successfully
```

### Test Specific Channel

```bash
# Test email only
cargoship alerts test --channel email

# Test Slack only
cargoship alerts test --channel slack

# Test with specific severity
cargoship alerts test --channel email --severity critical
```

### Test Output

**Success:**
```
Testing email alert channel...
✅ Email alert sent successfully to:
   - admin@example.com
   - finance@example.com
```

**Failure:**
```
Testing email alert channel...
❌ Failed to send email alert:
   Error: 535 5.7.8 Authentication failed

Troubleshooting tips:
- Verify SMTP credentials are correct
- Check that 2FA app password is used (for Gmail)
- Ensure SMTP port is correct (587 for TLS, 465 for SSL)
- Check firewall allows outbound SMTP connections
```

## Managing Channels

### View Configuration

```bash
cargoship alerts config
```

Output:
```
=== Budget Alert Configuration ===

Alert System: ✅ Enabled
Cooldown Period: 24h0m0s

=== Configured Channels ===

📧 Email (SMTP):
  Status:        ✅ Enabled
  Host:          smtp.gmail.com:587
  From:          cargoship@example.com
  Recipients:    admin@example.com, finance@example.com
  TLS:           ✅ Enabled

💬 Slack:
  Status:        ✅ Enabled
  Webhook URL:   https://hooks.slack.com/services/T00/B00/***
  Channel:       #cargoship-alerts
  Username:      CargoShip Monitor

🔗 Custom Webhook:
  Status:        ❌ Disabled

☁️  CloudWatch:
  Status:        ✅ Enabled
  Namespace:     CargoShip/Production
  Metric Name:   BudgetAlert
  Region:        us-west-2
```

### Enable Channel

```bash
cargoship alerts enable email
cargoship alerts enable slack
cargoship alerts enable webhook
cargoship alerts enable cloudwatch
```

### Disable Channel

```bash
cargoship alerts disable email
cargoship alerts disable slack
cargoship alerts disable webhook
cargoship alerts disable cloudwatch
```

### Update Configuration

To update a channel, simply reconfigure it:

```bash
# Update email recipients
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --smtp-username alerts@example.com \
  --smtp-password "new-app-password" \
  --smtp-from "cargoship@example.com" \
  --recipients newadmin@example.com
```

## Alert Payload Format

### Full JSON Structure

```json
{
  "id": "alert-1733745000",
  "timestamp": "2025-12-09T10:30:00Z",
  "type": "cost_threshold",
  "severity": "warning",
  "project_id": "20251206-abc123",
  "description": "Project exceeded 80% cost threshold",
  "is_global": false,

  "max_budget": 1000.00,
  "current_spend": 850.00,
  "budget_remaining": 150.00,
  "budget_used_percent": 0.85,
  "threshold_percent": 0.80,

  "max_volume_gb": 500.00,
  "current_volume_gb": 380.00,
  "volume_remaining": 120.00,
  "volume_used_percent": 0.76,
  "volume_threshold_percent": 0.75,

  "recommendation": "Review spending or increase budget",
  "action_required": false
}
```

## Troubleshooting

### Email Alerts Not Sending

**Symptom:** Email test fails with authentication error

**Solutions:**

1. **Gmail: Use App Password**
   ```bash
   # Verify you're using app password, not regular password
   cargoship alerts test --channel email
   ```

2. **Check SMTP Port**
   - Port 587: TLS (STARTTLS)
   - Port 465: SSL
   - Port 25: Unencrypted (not recommended)

3. **Verify Firewall Rules**
   ```bash
   # Test SMTP connectivity
   telnet smtp.gmail.com 587
   ```

4. **Check Credentials**
   ```bash
   # Reconfigure with correct credentials
   cargoship alerts configure email \
     --smtp-host smtp.gmail.com \
     --smtp-port 587 \
     --smtp-username your-email@gmail.com \
     --smtp-password "correct-app-password" \
     --smtp-from "your-email@gmail.com" \
     --recipients admin@example.com
   ```

### Slack Alerts Not Appearing

**Symptom:** Slack test succeeds but no message appears

**Solutions:**

1. **Verify Webhook URL**
   ```bash
   # Test webhook manually
   curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
     -H 'Content-Type: application/json' \
     -d '{"text":"Test from CargoShip"}'
   ```

2. **Check Channel Permissions**
   - Ensure app has permission to post to channel
   - Verify channel name includes `#` prefix

3. **Regenerate Webhook**
   - Go to Slack App settings
   - Incoming Webhooks → Regenerate
   - Update CargoShip configuration

### Webhook Returns 401/403

**Symptom:** Custom webhook test fails with authentication error

**Solutions:**

1. **Add Authentication Headers**
   ```bash
   cargoship alerts configure webhook \
     --webhook-url "https://api.company.com/alerts" \
     --headers "Authorization: Bearer YOUR_TOKEN,X-API-Key: YOUR_KEY"
   ```

2. **Verify Endpoint**
   ```bash
   # Test manually
   curl -X POST https://api.company.com/alerts \
     -H 'Content-Type: application/json' \
     -H 'Authorization: Bearer YOUR_TOKEN' \
     -d '{"test": true}'
   ```

### CloudWatch Metrics Not Appearing

**Symptom:** CloudWatch test succeeds but metrics not visible

**Solutions:**

1. **Check AWS Permissions**
   ```bash
   # Verify IAM permissions
   aws cloudwatch list-metrics --namespace CargoShip/Production
   ```

2. **Required IAM Permissions:**
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "cloudwatch:PutMetricData",
           "cloudwatch:GetMetricData",
           "cloudwatch:ListMetrics"
         ],
         "Resource": "*"
       }
     ]
   }
   ```

3. **Check Region**
   ```bash
   # Verify correct region
   export AWS_REGION=us-west-2
   cargoship alerts test --channel cloudwatch
   ```

## Best Practices

### Alert Configuration

1. **Use Multiple Channels**
   - Email for detailed records
   - Slack for team visibility
   - CloudWatch for monitoring integration

2. **Set Appropriate Thresholds**
   - Warning: 75-80% of budget
   - Critical: 90-95% of budget
   - Adjust based on burn rate volatility

3. **Configure Cooldown Period**
   - Default: 24 hours (prevents spam)
   - Reduce for critical projects: 1-6 hours
   - Increase for stable projects: 48-72 hours

4. **Test Regularly**
   ```bash
   # Monthly alert system health check
   cargoship alerts test
   ```

### Email Best Practices

1. **Use Dedicated Account**
   - Create `cargoship-alerts@company.com`
   - Don't use personal accounts

2. **Multiple Recipients**
   ```bash
   --recipients "admin@company.com,finance@company.com,ops@company.com"
   ```

3. **Configure Email Rules**
   - Create filters for alert severity
   - Set up auto-forwarding for critical alerts

### Slack Best Practices

1. **Dedicated Channel**
   - Create `#cargoship-budget-alerts`
   - Don't spam general channels

2. **Channel Notifications**
   - Enable desktop notifications for critical
   - Mute info-level alerts

3. **Threaded Conversations**
   - Use threads for alert follow-ups
   - Keep channel clean

### Webhook Best Practices

1. **Implement Retry Logic**
   ```go
   func handleAlert(alert *cost.BudgetAlert) error {
       maxRetries := 3
       for i := 0; i < maxRetries; i++ {
           if err := sendToTicketSystem(alert); err == nil {
               return nil
           }
           time.Sleep(time.Second * time.Duration(i+1))
       }
       return fmt.Errorf("failed after %d retries", maxRetries)
   }
   ```

2. **Validate Payload**
   ```go
   if alert.Severity == "critical" && alert.ActionRequired {
       createIncident(alert)
   }
   ```

3. **Log All Alerts**
   ```go
   log.Printf("[ALERT] %s - %s: %s",
       alert.Severity,
       alert.Type,
       alert.Description)
   ```

### Security Best Practices

1. **Protect Credentials**
   - Never commit credentials to git
   - Use environment variables
   - Consider secrets management (Vault, AWS Secrets Manager)

2. **Use TLS/SSL**
   - Always enable `--smtp-use-tls`
   - Use HTTPS for webhooks

3. **Restrict Webhook Access**
   - Implement webhook signature verification
   - Use API keys for authentication
   - Whitelist CargoShip IP addresses

4. **Rotate Credentials**
   - Rotate SMTP passwords quarterly
   - Regenerate Slack webhooks annually
   - Update API keys regularly

## Advanced Configuration

### Programmatic Configuration

```go
config := &cost.BudgetAlertConfig{
    Enabled:        true,
    CooldownPeriod: 24 * time.Hour,

    EmailEnabled:    true,
    EmailRecipients: []string{"admin@example.com"},
    SMTPHost:        "smtp.gmail.com",
    SMTPPort:        587,
    SMTPUsername:    "alerts@example.com",
    SMTPPassword:    "app-password",
    SMTPFrom:        "cargoship@example.com",
    SMTPUseTLS:      true,

    SlackEnabled:    true,
    SlackWebhookURL: "https://hooks.slack.com/services/T00/B00/abc",
    SlackChannel:    "#cargoship-alerts",
    SlackUsername:   "CargoShip",
}

notifier := cost.NewBudgetAlertNotifier(config, awsConfig)
```

### Custom Alert Handler

```go
type CustomAlertHandler struct {
    notifier *cost.BudgetAlertNotifier
}

func (h *CustomAlertHandler) HandleBudgetAlert(alert *cost.BudgetAlert) {
    // Log alert
    log.Printf("Budget alert: %s", alert.Description)

    // Send to notification channels
    if err := h.notifier.SendAlert(ctx, alert); err != nil {
        log.Printf("Failed to send alert: %v", err)
    }

    // Custom logic
    if alert.Severity == cost.SeverityCritical {
        h.createIncidentTicket(alert)
        h.notifyOnCall(alert)
    }
}
```

## See Also

- [Budget User Guide](BUDGET_USER_GUIDE.md) - End-user documentation
- [API Documentation](BUDGET_API.md) - Integrate alerts into your application
- [Troubleshooting Guide](TROUBLESHOOTING.md) - General troubleshooting

---

**Version**: v0.6.0
**Last Updated**: 2025-12-09
**License**: MIT
