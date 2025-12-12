# CargoShip Budget & Cost Management User Guide

**Version:** v0.6.0
**Last Updated:** 2025-12-09

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Budget Management](#budget-management)
4. [Cost Tracking & Reporting](#cost-tracking--reporting)
5. [Alert Notifications](#alert-notifications)
6. [Forecasting & Burn Rate Analysis](#forecasting--burn-rate-analysis)
7. [Best Practices](#best-practices)
8. [Troubleshooting](#troubleshooting)

## Overview

CargoShip's budget and cost management system provides comprehensive tools for tracking, forecasting, and controlling your AWS S3 storage costs. The system is designed around **projects** - each upload operation creates a unique project identified by its manifest upload ID (e.g., `20251206-abc123`).

### Key Features

- **Project-Based Cost Tracking**: Granular cost analysis per upload
- **Dual Budget Control**: Cost budgets (USD) AND volume quotas (GB)
- **Multi-Channel Alerts**: Email, Slack, webhooks, and CloudWatch integration
- **Advanced Forecasting**: 4 ML models with confidence intervals
- **Burn Rate Analysis**: Historical trends and predictions
- **Real-Time Monitoring**: Track spending as it happens

### Architecture Overview

```
Upload Operation
    ↓
Manifest Created (20251206-abc123)
    ↓
Cost Records Captured
    ↓
Budget Enforcement
    ↓
Alerts (if thresholds exceeded)
```

## Quick Start

### 1. View Cost Summary

Get an overview of your current month's spending:

```bash
cargoship cost summary
```

Output:
```
=== Cost Summary (month) ===
Period:              Dec 2025 (9 days)
Total Cost:          $45.67
Total Savings:       $12.34 (from compression)
Files Processed:     1,234
Data Volume:         567.89 GB

=== Breakdown by Region ===
us-west-2:           $30.45 (67%)
eu-west-1:           $15.22 (33%)
```

### 2. List All Projects

See all your upload projects and their costs:

```bash
cargoship cost projects
```

Output:
```
Projects (5 total)

Project: 20251206-abc123
  Period:         2025-12-06 to 2025-12-09 (3 days)
  Total Cost:     $12.34
  Savings:        $3.45
  Files:          456
  Volume:         123.45 GB

Project: 20251205-def456
  Period:         2025-12-05 to 2025-12-09 (4 days)
  Total Cost:     $8.90
  ...
```

### 3. Set a Project Budget

Create a budget for a specific project:

```bash
# Cost budget only
cargoship budget set 20251206-abc123 --cost 100

# Cost + volume quota
cargoship budget set 20251206-abc123 --cost 100 --volume 500

# With custom alert thresholds
cargoship budget set 20251206-abc123 \
  --cost 100 \
  --volume 500 \
  --cost-alert 0.85 \
  --volume-alert 0.75
```

### 4. Configure Email Alerts

Set up email notifications for budget alerts:

```bash
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --smtp-username alerts@example.com \
  --smtp-password "your-app-password" \
  --smtp-from "cargoship@example.com" \
  --recipients admin@example.com,finance@example.com
```

### 5. Forecast Future Costs

Predict your spending for the next 30 days:

```bash
cargoship cost forecast --days 30 --model ensemble
```

## Budget Management

### Understanding Budgets

CargoShip supports **two types of limits** per project:

1. **Cost Budgets** (USD): Maximum amount to spend
2. **Volume Quotas** (GB): Maximum data volume to upload

Both limits work independently - operations are blocked if **either** limit is exceeded.

### Creating Budgets

#### Basic Budget (Cost Only)

```bash
cargoship budget set PROJECT_ID --cost 1000
```

This creates a $1000 cost budget with:
- Alert at 80% ($800)
- Hard limit at 100% ($1000)

#### Volume Quota Only

```bash
cargoship budget set PROJECT_ID --volume 500
```

This creates a 500GB volume quota with:
- Alert at 75% (375GB)
- Hard limit at 100% (500GB)

#### Combined Budget

```bash
cargoship budget set PROJECT_ID --cost 1000 --volume 500
```

This creates both limits. Operations are blocked if:
- Cost exceeds $1000 **OR**
- Volume exceeds 500GB

#### Custom Alert Thresholds

```bash
cargoship budget set PROJECT_ID \
  --cost 1000 \
  --cost-alert 0.85 \
  --volume 500 \
  --volume-alert 0.70
```

This sets:
- Cost alert at 85% ($850)
- Volume alert at 70% (350GB)

### Checking Budget Status

View detailed budget status for a project:

```bash
cargoship budget status PROJECT_ID
```

Output:
```
Budget Status for Project: 20251206-abc123

=== Cost Budget ===
  Maximum Budget:    $1000.00
  Current Spending:  $450.23
  Remaining:         $549.77
  Usage:             45.0%
  Daily Burn Rate:   $50.02/day
  Projected Total:   $950.38
  Status:            ✅ OK

=== Volume Quota ===
  Maximum Volume:    500.00 GB
  Current Volume:    234.56 GB
  Remaining:         265.44 GB
  Usage:             46.9%
  Daily Rate:        26.07 GB/day
  Projected Total:   495.33 GB
  Status:            ⚠️  WILL EXCEED
```

### Listing All Budgets

```bash
cargoship budget list
```

### Removing Budgets

```bash
cargoship budget remove PROJECT_ID
```

After removal, the project uses global budget settings.

## Cost Tracking & Reporting

### Project-Based Cost Tracking

Every upload operation creates a project with a unique manifest ID. All costs are tracked against this project.

#### View Project Costs

```bash
# Basic project summary
cargoship cost project 20251206-abc123

# With specific time period
cargoship cost project 20251206-abc123 --period week

# JSON output for programmatic access
cargoship cost project 20251206-abc123 --json
```

Output:
```
=== Project Summary ===
Project ID:          20251206-abc123
Period:              2025-12-06 to 2025-12-09 (3 days)
Total Cost:          $45.67
Total Savings:       $12.34 (compression)
Files Processed:     1,234
Data Volume:         567.89 GB
Avg File Size:       471.35 MB

=== Cost Breakdown by Region ===
us-west-2:           $30.45 (67%)
eu-west-1:           $15.22 (33%)

=== Cost Breakdown by Storage Class ===
INTELLIGENT_TIERING: $35.50 (78%)
STANDARD:            $10.17 (22%)

=== Timeline ===
2025-12-06:          $15.23
2025-12-07:          $18.45
2025-12-08:          $11.99
```

### Cost Summaries

#### Current Month

```bash
cargoship cost summary
```

#### Specific Time Period

```bash
# This week
cargoship cost summary --period week

# This year
cargoship cost summary --period year

# Custom date range
cargoship cost summary --from 2025-12-01 --to 2025-12-31
```

#### Generate Cost Report

```bash
# Human-readable report
cargoship cost report --period month

# Save to file
cargoship cost report --period month --output report.txt

# JSON format
cargoship cost report --period month --json --output report.json
```

### Cost Estimation

#### Estimate Upload Costs

Before uploading, estimate storage costs:

```bash
# Basic estimate
cargoship cost estimate --size 100GB

# Specific storage class
cargoship cost estimate --size 500GB --storage-class GLACIER

# Different region
cargoship cost estimate --size 1TB --region eu-west-1
```

Output:
```
=== Cost Estimate ===
Data Size:           100.00 GB
Storage Class:       STANDARD
Region:              us-west-2

Monthly Storage:     $2.30
PUT Requests:        $0.05
GET Requests:        $0.01
Data Transfer:       $0.90

Total Monthly:       $3.26
```

## Alert Notifications

CargoShip supports **4 notification channels**:

1. **Email (SMTP)**: TLS-encrypted email notifications
2. **Slack**: Rich webhook integration with color-coded messages
3. **Webhooks**: Custom HTTP POST notifications
4. **CloudWatch**: AWS CloudWatch metrics and alarms

### Alert Types

- `cost_threshold`: Cost exceeded alert threshold
- `volume_threshold`: Volume exceeded alert threshold
- `cost_over_budget`: Cost exceeded maximum budget
- `volume_over_quota`: Volume exceeded maximum quota
- `budget_projection`: Projected costs will exceed budget
- `volume_projection`: Projected volume will exceed quota

### Alert Severities

- `info`: Informational (< threshold)
- `warning`: Warning (> threshold, < max)
- `critical`: Critical (> max)

### Email Configuration

```bash
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --smtp-username alerts@example.com \
  --smtp-password "your-app-password" \
  --smtp-from "cargoship@example.com" \
  --recipients admin@example.com,finance@example.com
```

**Gmail App Password Setup:**

1. Go to Google Account Settings
2. Security → 2-Step Verification → App passwords
3. Generate password for "Mail"
4. Use generated password in `--smtp-password`

### Slack Configuration

```bash
cargoship alerts configure slack \
  --webhook-url "https://hooks.slack.com/services/T00/B00/abc123" \
  --channel "#cargoship-alerts" \
  --username "CargoShip Monitor"
```

**Slack Webhook Setup:**

1. Go to https://api.slack.com/apps
2. Create new app or select existing
3. Enable "Incoming Webhooks"
4. Create webhook for specific channel
5. Copy webhook URL

### Webhook Configuration

```bash
cargoship alerts configure webhook \
  --webhook-url "https://your-server.com/api/alerts" \
  --headers "Authorization: Bearer token123,X-API-Key: key456"
```

Webhook payload example:
```json
{
  "alert_type": "cost_threshold",
  "severity": "warning",
  "project_id": "20251206-abc123",
  "current_spend": 850.00,
  "budget": 1000.00,
  "threshold": 800.00,
  "message": "Project 20251206-abc123 exceeded 85% cost threshold",
  "timestamp": "2025-12-09T10:30:00Z"
}
```

### CloudWatch Configuration

```bash
cargoship alerts configure cloudwatch \
  --namespace "CargoShip/Production" \
  --metric-name "BudgetAlert"
```

### Testing Alerts

```bash
# Test all channels
cargoship alerts test

# Test specific channel
cargoship alerts test --channel email --severity critical

# Test Slack only
cargoship alerts test --channel slack --severity warning
```

### Managing Alert Channels

```bash
# Enable channel
cargoship alerts enable email
cargoship alerts enable slack

# Disable channel
cargoship alerts disable cloudwatch

# View configuration
cargoship alerts config
```

## Forecasting & Burn Rate Analysis

### Cost Forecasting

CargoShip includes **4 forecasting models**:

1. **Linear**: Simple linear regression
2. **Exponential**: Exponential growth model
3. **Moving Average**: Time-series moving average
4. **Ensemble**: Combines all models with weighted average

#### Generate Forecast

```bash
# Default forecast (30 days, ensemble model)
cargoship cost forecast

# Specific time period
cargoship cost forecast --days 60

# Specific model
cargoship cost forecast --model linear

# For specific project
cargoship cost forecast --project 20251206-abc123

# With confidence intervals
cargoship cost forecast --confidence 0.95
```

Output:
```
=== Cost Forecast (ensemble model) ===
Historical Period:   2025-11-09 to 2025-12-09 (30 days)
Forecast Period:     2025-12-09 to 2026-01-08 (30 days)

Model Accuracy:
  R² Score:          0.92 (excellent fit)
  MAE:               $2.34
  RMSE:              $3.12

Predicted Costs:
  7 days:            $350.23 (±$15.67)
  14 days:           $725.45 (±$32.18)
  30 days:           $1,545.89 (±$68.92)
  60 days:           $3,120.45 (±$145.23)
  90 days:           $4,723.12 (±$225.67)

Confidence Level:    95%
Warning:             ⚠️  Projected to exceed $1000 budget in 21 days
```

### Burn Rate Analysis

Track your daily, weekly, and monthly spending rates:

```bash
# Current burn rate
cargoship cost burnrate

# Extended analysis period
cargoship cost burnrate --days 60

# For specific project
cargoship cost burnrate --project 20251206-abc123
```

Output:
```
=== Burn Rate Analysis ===
Analysis Period:     30 days

Current Burn Rate:
  Daily:             $50.23/day
  Weekly:            $351.61/week
  Monthly:           $1,506.90/month

Historical Statistics:
  Average:           $45.67/day
  Minimum:           $12.34/day (2025-11-15)
  Maximum:           $89.12/day (2025-12-01)
  Std Deviation:     $15.23
  Volatility:        33.4%

Trend Analysis:
  Direction:         ⬆️  Increasing
  Strength:          Moderate
  Acceleration:      +$0.45/day²

Predicted Future Burn Rate:
  30 days:           $55.67/day (±$8.23)
  60 days:           $62.34/day (±$12.45)
  90 days:           $70.12/day (±$18.67)
```

### Budget Exhaustion Prediction

Predict when your budget will run out:

```bash
# For current budget
cargoship cost exhaustion --budget 1000

# With current spending
cargoship cost exhaustion --budget 1000 --spent 450

# For specific project
cargoship cost exhaustion --budget 1000 --project 20251206-abc123
```

Output:
```
=== Budget Exhaustion Prediction ===
Budget:              $1,000.00
Current Spending:    $450.23
Remaining:           $549.77

Exhaustion Date:     2025-12-20
Days Until Empty:    11 days

Burn Rate:           $50.02/day

Confidence Intervals:
  90%:               2025-12-18 to 2025-12-22
  95%:               2025-12-17 to 2025-12-23
  99%:               2025-12-15 to 2025-12-25

Budget Usage Forecast:
  Today:             $450.23 (45%)
  +7 days:           $800.37 (80%)  ⚠️  Warning
  +14 days:          $1,150.51 (115%)  🚨 Over Budget

Warning Level:       🚨 CRITICAL (<14 days)
Recommendation:      Reduce spending or increase budget
```

## Best Practices

### Budget Planning

1. **Start with Monitoring**: Track costs for 2-4 weeks before setting budgets
2. **Use Forecasting**: Generate forecasts to understand spending trends
3. **Set Alerts Early**: Configure alerts at 75-80% of budget
4. **Review Regularly**: Check budget status weekly
5. **Adjust Dynamically**: Update budgets based on actual usage

### Cost Optimization

1. **Use Compression**: CargoShip automatically compresses data (typically 30-50% savings)
2. **Choose Storage Classes Wisely**:
   - `INTELLIGENT_TIERING`: Best for unpredictable access patterns
   - `STANDARD_IA`: For data accessed monthly
   - `GLACIER`: For long-term archival (cheapest)
3. **Monitor Compression ROI**: Check `cost project` savings
4. **Batch Uploads**: Combine small files to reduce request costs

### Alert Configuration

1. **Use Multiple Channels**: Email + Slack for redundancy
2. **Test Regularly**: Run `alerts test` monthly
3. **Set Appropriate Thresholds**:
   - Warning: 75-80%
   - Critical: 90-95%
4. **Use Project-Specific Alerts**: Critical projects get lower thresholds

### Forecasting Strategy

1. **Use Ensemble Model**: Most accurate for production
2. **Check Model Accuracy**: R² > 0.8 is good, > 0.9 is excellent
3. **Consider Confidence Intervals**: Plan for 95% confidence upper bound
4. **Update Forecasts Weekly**: Trends change over time
5. **Factor in Seasonality**: Q4 might have different patterns than Q1

## Troubleshooting

### Budget Not Enforcing

**Symptom**: Operations continue despite budget being exceeded

**Solutions**:
1. Check budget is set: `cargoship budget status PROJECT_ID`
2. Verify budget value: `--cost 0` means unlimited
3. Check logs for enforcement errors
4. Ensure cost tracking is enabled in config

### Alerts Not Sending

**Symptom**: No alert notifications received

**Solutions**:

1. **Test Alert Delivery**:
   ```bash
   cargoship alerts test --channel email --severity critical
   ```

2. **Check Configuration**:
   ```bash
   cargoship alerts config
   ```

3. **Verify Channel is Enabled**:
   ```bash
   cargoship alerts enable email
   ```

4. **Email Issues**:
   - Verify SMTP credentials
   - Check spam folder
   - Test with Gmail app password
   - Verify TLS/SSL settings

5. **Slack Issues**:
   - Verify webhook URL is correct
   - Check channel name includes `#`
   - Verify app has webhook permissions

### Inaccurate Forecasts

**Symptom**: Forecasts don't match actual spending

**Solutions**:
1. Check historical data: Need at least 7 days
2. Try different models: `--model linear` vs `--model ensemble`
3. Check for data anomalies: Large one-time uploads
4. Increase analysis period: `--days 60` instead of 30
5. Review model accuracy: R² < 0.7 means poor fit

### Cost Tracking Not Recording

**Symptom**: `cost summary` shows $0.00

**Solutions**:
1. Verify cost tracking is enabled in config
2. Check upload succeeded: Look for manifest ID
3. Run upload with cost tracking: Ensure project ID is set
4. Check permissions: S3 access required for cost calculation

### Budget Status Shows Incorrect Values

**Symptom**: Budget remaining doesn't match expected values

**Solutions**:
1. Check time period: Default is current month
2. Verify project ID is correct
3. Check for duplicate cost records
4. Re-run cost calculation: `cargoship cost summary --refresh`

## Command Reference

### Budget Commands

| Command | Description |
|---------|-------------|
| `budget status PROJECT_ID` | Show budget status |
| `budget set PROJECT_ID` | Set budget and quota |
| `budget list` | List all budgets |
| `budget remove PROJECT_ID` | Remove budget |

### Cost Commands

| Command | Description |
|---------|-------------|
| `cost summary` | Cost summary for period |
| `cost projects` | List all projects |
| `cost project ID` | Project cost details |
| `cost estimate` | Estimate future costs |
| `cost report` | Generate cost report |
| `cost forecast` | Forecast future spending |
| `cost burnrate` | Analyze burn rate |
| `cost exhaustion` | Predict budget exhaustion |

### Alert Commands

| Command | Description |
|---------|-------------|
| `alerts config` | Show alert config |
| `alerts configure CHANNEL` | Configure channel |
| `alerts test` | Test alert delivery |
| `alerts enable CHANNEL` | Enable channel |
| `alerts disable CHANNEL` | Disable channel |

## Getting Help

- **GitHub Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Documentation**: https://github.com/scttfrdmn/cargoship/tree/main/docs
- **API Reference**: See `docs/BUDGET_API.md`
- **Alert Configuration**: See `docs/ALERTS_CONFIGURATION.md`

## Next Steps

1. [API Documentation](BUDGET_API.md) - Integrate budgets into your applications
2. [Alert Configuration Guide](ALERTS_CONFIGURATION.md) - Advanced alert setup
3. [Forecasting Guide](FORECASTING_GUIDE.md) - Deep dive into forecasting models
4. [Troubleshooting Guide](TROUBLESHOOTING.md) - Comprehensive troubleshooting

---

**Version**: v0.6.0
**Last Updated**: 2025-12-09
**License**: MIT
