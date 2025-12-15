# Budget Management Guide

**CargoShip v0.6.0+**
**Last Updated**: December 2025

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Concepts](#concepts)
4. [Configuration](#configuration)
5. [CLI Reference](#cli-reference)
6. [Use Cases](#use-cases)
7. [Integration with Uploads](#integration-with-uploads)
8. [Alert Configuration](#alert-configuration)
9. [Troubleshooting](#troubleshooting)
10. [FAQ](#faq)

---

## Overview

CargoShip's budget management system provides **dual-control cost governance** through both cost budgets (USD) and volume quotas (GB). This enables granular control where operations can be blocked if they would exceed EITHER cost budgets OR volume quotas.

### Key Features

- **Project-Specific Budgets**: Set unique budgets per project (manifest upload)
- **Dual Controls**: Separate cost budgets (USD) AND volume quotas (GB)
- **Hierarchical Enforcement**: Project limits checked first, then global limits
- **Real-Time Tracking**: Live budget status with burn rate monitoring
- **Predictive Alerts**: Warn when projected usage will exceed limits
- **Flexible Thresholds**: Configure alert levels independently for cost vs volume

### Who Should Use This?

- **Academic Researchers**: Track grant spending and ensure compliance with funding limits
- **Lab Managers**: Allocate budgets across multiple research projects
- **Principal Investigators**: Monitor grant utilization and prevent overages
- **Enterprise Teams**: Control cloud spending per business unit or cost center
- **DevOps Teams**: Prevent runaway cloud costs from automated pipelines

---

## Quick Start

### 1. Set a Project Budget

```bash
# Set cost budget and volume quota for a project
cargoship budget set genomics-2025 --cost 5000 --volume 2000

# Output:
# ✅ Budget set for project: genomics-2025
#    Cost Budget:      $5000.00 (alert at 80%)
#    Volume Quota:     2000.00 GB (alert at 75%)
```

### 2. Check Budget Status

```bash
# View current budget status
cargoship budget status genomics-2025

# Output:
# Budget Status for Project: genomics-2025
#
# === Cost Budget ===
#   Maximum Budget:    $5000.00
#   Current Spending:  $1250.50
#   Remaining:         $3749.50
#   Usage:             25.0%
#   Daily Burn Rate:   $125.05/day
#   Projected Total:   $3751.50
#   Status:            ✅ OK
#
# === Volume Quota ===
#   Maximum Volume:    2000.00 GB
#   Current Volume:    480.25 GB
#   Remaining:         1519.75 GB
#   Usage:             24.0%
#   Daily Rate:        48.02 GB/day
#   Projected Total:   1440.60 GB
#   Status:            ✅ OK
```

### 3. Upload with Budget Enforcement

```bash
# Upload will be checked against budget before starting
cargoship create upload ./dataset \
  --bucket research-data \
  --project genomics-2025

# If budget would be exceeded:
# ❌ Error: project budget exceeded
#    Project:         genomics-2025
#    Max Budget:      $5000.00
#    Current Spend:   $4850.00
#    Operation Cost:  $200.00
#    Projected:       $5050.00
#    Overage:         $50.00
```

---

## Concepts

### Project-Based Budgets

Each **project** represents a logical grouping of uploads, typically corresponding to:
- Research grant or funding source
- Business unit or cost center
- Experiment or study
- Client or customer

**Project ID** is typically the manifest upload ID, but can be any identifier.

### Dual Control System

CargoShip enforces budgets using **two independent controls**:

#### 1. Cost Budget (USD)
- Maximum spending limit in dollars
- Tracks actual S3 costs (storage, requests, transfer)
- Alert when approaching limit (default: 80%)
- **Block uploads** that would exceed budget

#### 2. Volume Quota (GB)
- Maximum data volume limit in gigabytes
- Tracks total uploaded data size (uncompressed)
- Alert when approaching limit (default: 75%)
- **Block uploads** that would exceed quota

**Both limits are enforced independently** - an operation is blocked if it would exceed EITHER limit.

### Hierarchical Enforcement

Budget checks follow this order:

1. **Project Budget** (if configured)
   - Check if upload would exceed project cost budget
   - Check if upload would exceed project volume quota

2. **Global Budget** (if no project budget)
   - Check if upload would exceed global cost budget
   - Check if upload would exceed global volume quota

**Note**: Global budgets are configured in `~/.cargoship.yaml`, while project budgets are managed via CLI.

### Burn Rate Tracking

CargoShip calculates **daily burn rates** for both cost and volume:

- **Cost Burn Rate**: Average spending per day in current period
- **Volume Burn Rate**: Average data uploaded per day in current period
- **Projected End-of-Period (EOP)**: Estimated total cost/volume if burn rate continues

**Example**:
```
Period: 30 days
Days Elapsed: 10 days
Current Spend: $1,000
Daily Burn Rate: $100/day
Projected EOP Spend: $3,000
Budget: $2,500
Status: ⚠️  WILL EXCEED
```

---

## Configuration

### Global Budget Configuration

Configure global budgets in `~/.cargoship.yaml`:

```yaml
aws:
  region: us-west-2
  profile: default

cost_control:
  enabled: true

  # Global cost budget
  max_budget: 10000.0  # USD
  alert_threshold: 0.80  # Alert at 80%

  # Global volume quota
  max_volume_gb: 5000.0  # GB
  volume_alert_threshold: 0.75  # Alert at 75%

  # Budget period
  period_type: "monthly"  # monthly, quarterly, grant
  period_start: "2025-01-01"
  period_end: "2025-12-31"

  # Optional: Grant information
  grant_name: "NIH R01 123456"
  enable_rollover: false
```

### Project Budget Configuration

Project budgets are configured via CLI (stored in cost management database):

```bash
# Set project budget with cost and volume limits
cargoship budget set <project-id> \
  --cost <max-usd> \
  --volume <max-gb> \
  --cost-alert <threshold> \
  --volume-alert <threshold>
```

**Parameters**:
- `--cost`: Maximum cost budget in USD (0 = unlimited)
- `--volume`: Maximum volume quota in GB (0 = unlimited)
- `--cost-alert`: Cost alert threshold (0.0-1.0, default: 0.8)
- `--volume-alert`: Volume alert threshold (0.0-1.0, default: 0.75)

---

## CLI Reference

### `cargoship budget status`

Show detailed budget status for a project.

```bash
cargoship budget status <project-id> [flags]
```

**Flags**:
- `--json`: Output as JSON

**Examples**:
```bash
# Show budget status
cargoship budget status genomics-2025

# Get JSON output
cargoship budget status genomics-2025 --json
```

**Output Fields**:
- Current spending and volume usage
- Remaining budget and quota
- Usage percentages
- Daily burn rates
- Projected end-of-period totals
- Alert status

---

### `cargoship budget set`

Set or update budget and quota for a project.

```bash
cargoship budget set <project-id> [flags]
```

**Flags**:
- `--cost <float>`: Maximum cost budget in USD (0 = unlimited)
- `--volume <float>`: Maximum volume quota in GB (0 = unlimited)
- `--cost-alert <float>`: Cost alert threshold (0.0-1.0, default: 0.8)
- `--volume-alert <float>`: Volume alert threshold (0.0-1.0, default: 0.75)

**Examples**:
```bash
# Set cost budget only
cargoship budget set genomics-2025 --cost 5000

# Set volume quota only
cargoship budget set genomics-2025 --volume 2000

# Set both cost and volume limits
cargoship budget set genomics-2025 --cost 5000 --volume 2000

# Set with custom alert thresholds
cargoship budget set genomics-2025 \
  --cost 5000 \
  --volume 2000 \
  --cost-alert 0.85 \
  --volume-alert 0.70

# Set unlimited (removes limits)
cargoship budget set genomics-2025 --cost 0 --volume 0
```

---

### `cargoship budget list`

List all configured project budgets.

```bash
cargoship budget list [flags]
```

**Flags**:
- `--json`: Output as JSON

**Examples**:
```bash
# List all project budgets
cargoship budget list

# Output:
# Project Budgets (3 total)
#
# Project: genomics-2025
#   Cost Budget:    $5000.00
#   Volume Quota:   2000.00 GB
#
# Project: proteomics-2025
#   Cost Budget:    $3000.00
#   Volume Quota:   1500.00 GB
#
# Project: imaging-2025
#   Cost Budget:    unlimited
#   Volume Quota:   5000.00 GB
```

---

### `cargoship budget remove`

Remove budget configuration for a project.

```bash
cargoship budget remove <project-id>
```

**Examples**:
```bash
# Remove project budget
cargoship budget remove genomics-2025

# Output:
# ✅ Budget removed for project: genomics-2025
#    Project will now use global budget and quota settings
```

After removal, the project will be subject to global budget and quota limits (if configured).

---

## Use Cases

### Use Case 1: Academic Researcher with NIH Grant

**Scenario**: Dr. Smith has a 5-year NIH R01 grant with $250,000 total budget. She needs to track S3 costs to ensure compliance with funding limits.

**Configuration**:

```yaml
# ~/.cargoship.yaml
cost_control:
  enabled: true
  max_budget: 250000  # Total grant amount
  period_type: "grant"
  period_start: "2025-01-01"
  period_end: "2029-12-31"
  grant_name: "NIH R01 GM123456"
  enable_rollover: true
```

**Per-Project Budgets**:

```bash
# Year 1: Genomics sequencing
cargoship budget set nih-year1-genomics --cost 10000 --volume 5000

# Year 1: Imaging data
cargoship budget set nih-year1-imaging --cost 5000 --volume 2000

# Track spending
cargoship budget status nih-year1-genomics
```

**Benefits**:
- Track spending against grant budget
- Prevent accidental overspending
- Generate reports for grant administration
- Allocate funds across experiments

---

### Use Case 2: Lab Manager with Multiple Projects

**Scenario**: Lab manager oversees 5 research projects with different funding sources. Each project needs independent budget tracking.

**Setup**:

```bash
# Project A: Cancer research (Foundation grant)
cargoship budget set project-cancer-2025 \
  --cost 15000 \
  --volume 8000 \
  --cost-alert 0.75

# Project B: Neuroscience (NIH grant)
cargoship budget set project-neuro-2025 \
  --cost 20000 \
  --volume 10000 \
  --cost-alert 0.80

# Project C: Cardiovascular (Industry sponsored)
cargoship budget set project-cardio-2025 \
  --cost 30000 \
  --volume 15000 \
  --cost-alert 0.85

# Project D: Pilot study (internal funds)
cargoship budget set project-pilot-2025 \
  --cost 3000 \
  --volume 1500 \
  --cost-alert 0.70

# Project E: Multi-year cohort (federal grant)
cargoship budget set project-cohort-2025 \
  --cost 50000 \
  --volume 25000 \
  --cost-alert 0.90
```

**Monthly Review**:

```bash
# List all projects
cargoship budget list

# Check specific project
cargoship budget status project-cancer-2025

# Generate JSON report for all projects
for project in project-cancer-2025 project-neuro-2025 project-cardio-2025 project-pilot-2025 project-cohort-2025; do
  cargoship budget status $project --json > reports/${project}-budget.json
done
```

**Benefits**:
- Independent tracking per funding source
- Custom alert thresholds per project risk tolerance
- Easy reporting for grant administration
- Prevent cross-project fund mixing

---

### Use Case 3: PI Reviewing Grant Spending

**Scenario**: Principal Investigator needs quarterly spending reports to ensure grant compliance and plan future experiments.

**Quarterly Check**:

```bash
# Q1 Review (January - March)
cargoship budget status nih-r01-2025 --json | jq '{
  project_id,
  current_spend,
  budget_remaining,
  budget_used,
  daily_burn_rate,
  projected_eop_spend
}'

# Output:
# {
#   "project_id": "nih-r01-2025",
#   "current_spend": 12500.50,
#   "budget_remaining": 37499.50,
#   "budget_used": 0.25,
#   "daily_burn_rate": 138.89,
#   "projected_eop_spend": 50000.00
# }
```

**Analysis**:
- Q1: Spent $12,500 (25% of annual $50K budget)
- Burn rate: $138.89/day
- Projected annual spend: $50,000 (on track)
- Action: Continue current usage patterns

**Adjustment Example**:

```bash
# Reduce budget for Q2 based on Q1 spending
cargoship budget set nih-r01-q2 \
  --cost 10000 \
  --cost-alert 0.75  # More conservative alert
```

---

### Use Case 4: Enterprise Cost Center Allocation

**Scenario**: DevOps team manages S3 uploads for 10 business units with allocated cloud budgets.

**Setup by Cost Center**:

```bash
# Engineering
cargoship budget set costcenter-engineering \
  --cost 50000 \
  --volume 25000

# Research & Development
cargoship budget set costcenter-rnd \
  --cost 75000 \
  --volume 40000

# Data Science
cargoship budget set costcenter-datascience \
  --cost 100000 \
  --volume 50000

# Marketing Analytics
cargoship budget set costcenter-marketing \
  --cost 25000 \
  --volume 10000

# Finance
cargoship budget set costcenter-finance \
  --cost 15000 \
  --volume 5000
```

**Monthly Reporting**:

```bash
#!/bin/bash
# monthly-budget-report.sh

echo "Monthly Budget Report - $(date +%Y-%m)"
echo "=========================================="
echo

for cc in engineering rnd datascience marketing finance; do
  cargoship budget status costcenter-${cc} --json | jq -r '
    "Cost Center: " + .project_id,
    "  Budget:      $" + (.max_budget | tostring),
    "  Spent:       $" + (.current_spend | tostring),
    "  Remaining:   $" + (.budget_remaining | tostring),
    "  Usage:       " + ((.budget_used * 100) | tostring) + "%",
    ""
  '
done
```

---

### Use Case 5: Preventing Budget Overruns

**Scenario**: Research project approaching budget limit needs automatic blocking to prevent overage.

**Setup with Strict Enforcement**:

```bash
# Set budget with low alert threshold
cargoship budget set clinical-trial-2025 \
  --cost 25000 \
  --volume 15000 \
  --cost-alert 0.90 \
  --volume-alert 0.85
```

**Upload Attempt Near Limit**:

```bash
# Check budget before large upload
cargoship budget status clinical-trial-2025

# Output:
# === Cost Budget ===
#   Current Spending:  $24,500.00
#   Remaining:         $500.00
#   Usage:             98.0%
#   Status:            ⚠️  ALERT TRIGGERED

# Attempt upload (will be blocked)
cargoship create upload ./new-data \
  --bucket clinical-data \
  --project clinical-trial-2025

# Error:
# ❌ Error: project budget exceeded
#    Project:         clinical-trial-2025
#    Max Budget:      $25000.00
#    Current Spend:   $24500.00
#    Operation Cost:  $750.00
#    Projected:       $25250.00
#    Overage:         $250.00
#
# Operation blocked to prevent budget overage.
```

**Resolution**:

```bash
# Option 1: Increase budget (if funds available)
cargoship budget set clinical-trial-2025 --cost 26000

# Option 2: Remove some data to stay within budget
# Option 3: Request budget allocation from another project
```

---

## Integration with Uploads

### Automatic Budget Checks

Every upload automatically checks budget before starting:

```bash
cargoship create upload ./data \
  --bucket my-bucket \
  --project my-project
```

**Check Sequence**:
1. **Estimate upload cost** based on data size, storage class, region
2. **Check project budget** (if `--project` specified)
3. **Check volume quota** (if configured)
4. **Proceed or block** based on results

### Project Assignment

Specify project ID to associate upload with a budget:

```bash
# Upload with project assignment
cargoship create upload ./data \
  --bucket research-data \
  --project genomics-2025

# Project ID is stored in manifest and tracked in cost database
```

**Without `--project` flag**: Upload is tracked against global budget only.

### Cost Estimation

CargoShip estimates costs before upload:

```bash
# Estimate upload costs
cargoship estimate ./data \
  --storage-class GLACIER_IR \
  --show-breakdown

# Output includes:
# - Upload request costs
# - Storage costs (monthly)
# - Data transfer costs
# - Total estimated cost
```

Use estimates to verify budget before uploading.

---

## Alert Configuration

### Alert Thresholds

Configure when alerts trigger:

```bash
# Conservative alerts (75% cost, 70% volume)
cargoship budget set project1 \
  --cost 10000 \
  --volume 5000 \
  --cost-alert 0.75 \
  --volume-alert 0.70

# Moderate alerts (80% cost, 75% volume) - DEFAULT
cargoship budget set project2 \
  --cost 10000 \
  --volume 5000 \
  --cost-alert 0.80 \
  --volume-alert 0.75

# Aggressive alerts (90% cost, 85% volume)
cargoship budget set project3 \
  --cost 10000 \
  --volume 5000 \
  --cost-alert 0.90 \
  --volume-alert 0.85
```

### Alert Triggers

Alerts trigger when:
1. **Current usage exceeds threshold** (e.g., spent >80% of budget)
2. **Projected usage will exceed limit** (based on burn rate)
3. **Single operation would exceed limit** (upload blocked)

### Alert Outputs

Budget status shows alerts:

```bash
cargoship budget status project1

# Output includes:
# Status: ⚠️  ALERT TRIGGERED (when threshold exceeded)
# Status: ⚠️  WILL EXCEED (when projection exceeds limit)
# Status: ⚠️  OVER BUDGET (when limit already exceeded)
```

### Custom Alert Integration

Export budget data for custom alerting:

```bash
# Get JSON status
cargoship budget status project1 --json > budget-status.json

# Parse and send alerts
cat budget-status.json | jq -r '
  if .alert_triggered or .volume_alert_triggered then
    "ALERT: Project " + .project_id + " at " + (.budget_used * 100 | tostring) + "% of budget"
  else
    "OK"
  end
'

# Integrate with monitoring systems
# - Email alerts via SMTP
# - Slack notifications
# - PagerDuty integration
# - CloudWatch alarms
```

---

## Troubleshooting

### Issue: Budget Not Enforcing

**Symptoms**: Uploads proceed despite exceeding budget limits

**Diagnosis**:
```bash
# Check if cost control is enabled
cargoship budget status <project-id> --json | jq '.max_budget'

# Check global config
cat ~/.cargoship.yaml | grep -A 10 cost_control
```

**Solutions**:
1. Enable cost control in `~/.cargoship.yaml`:
   ```yaml
   cost_control:
     enabled: true
   ```

2. Verify budget is set:
   ```bash
   cargoship budget list
   ```

3. Ensure project ID matches:
   ```bash
   cargoship create upload ./data --project <correct-project-id>
   ```

---

### Issue: Incorrect Cost Estimates

**Symptoms**: Projected costs don't match actual AWS bills

**Diagnosis**:
```bash
# Compare estimated vs actual costs
cargoship estimate ./data --show-breakdown
cargoship cost projects  # Actual costs
```

**Solutions**:
1. Update pricing data (cached for 24h):
   ```bash
   # Force pricing refresh
   rm ~/.cargoship/cache/pricing.json
   cargoship estimate ./data --real-time-pricing
   ```

2. Verify correct region:
   ```bash
   cargoship estimate ./data --region us-west-2
   ```

3. Check storage class costs:
   ```bash
   cargoship estimate ./data --storage-class GLACIER_IR
   ```

---

### Issue: Budget Reset Mid-Period

**Symptoms**: Budget usage shows 0% after uploads

**Diagnosis**:
```bash
# Check budget period configuration
cargoship budget status <project-id> --json | jq '{period_start, period_end, period_type}'
```

**Solutions**:
1. Verify period dates in `~/.cargoship.yaml`:
   ```yaml
   cost_control:
     period_start: "2025-01-01"  # Start of budget period
     period_end: "2025-12-31"    # End of budget period
   ```

2. For monthly budgets, ensure period aligns with calendar month
3. For grant budgets, set exact grant start/end dates

---

### Issue: Volume Quota Not Working

**Symptoms**: Volume quota not enforced even when exceeded

**Diagnosis**:
```bash
# Check volume quota configuration
cargoship budget status <project-id> | grep "Volume Quota"
```

**Solutions**:
1. Set volume quota (default is 0 = unlimited):
   ```bash
   cargoship budget set <project-id> --volume 5000
   ```

2. Verify volume tracking is enabled:
   ```yaml
   # ~/.cargoship.yaml
   cost_control:
     enabled: true
     max_volume_gb: 10000  # Global quota
   ```

---

## FAQ

### Q: How are costs calculated?

**A**: CargoShip uses AWS Pricing API to calculate costs based on:
- Storage class and region
- Data volume (compressed size)
- Request counts (PUTs, GETs, LISTs)
- Data transfer (if cross-region)

Costs are estimated before upload and tracked after completion.

---

### Q: Can I have different budgets per region?

**A**: Not directly. Budgets are per-project, not per-region. However, you can create separate projects per region:

```bash
cargoship budget set project-uswest2 --cost 10000
cargoship budget set project-useast1 --cost 15000

cargoship create upload ./data --bucket us-west-2-bucket --project project-uswest2
cargoship create upload ./data --bucket us-east-1-bucket --project project-useast1
```

---

### Q: What happens when budget is exceeded?

**A**: Uploads are **blocked before starting**. You'll receive an error message with:
- Current spending
- Operation cost
- Projected total
- Overage amount

No charges are incurred for blocked uploads.

---

### Q: Can I track budgets retroactively?

**A**: Budget tracking starts when you set a budget. Historical uploads before budget configuration are not included in budget calculations.

To include historical data, you would need to manually add entries to the cost database.

---

### Q: How do I generate budget reports?

**A**: Use JSON output for programmatic reporting:

```bash
# Single project
cargoship budget status project1 --json

# All projects
cargoship budget list --json

# Custom report
cargoship budget list --json | jq -r '
  .[] |
  "\(.project_id): $\(.current_spend) / $\(.max_budget) (\((.budget_used * 100) | floor)%)"
'
```

---

### Q: Can I set daily spending limits?

**A**: Not directly, but you can calculate daily limits from monthly budgets:

```bash
# Monthly budget: $3000
# Daily limit: $3000 / 30 = $100/day

# Monitor daily spending
cargoship budget status project1 | grep "Daily Burn Rate"

# Alert if exceeds daily target
# Daily Burn Rate: $125.00/day (25% over target)
```

---

### Q: How do I handle budget overruns?

**A**: Options:
1. **Increase budget** (if funds available):
   ```bash
   cargoship budget set project1 --cost 12000
   ```

2. **Remove non-critical data** to stay within budget

3. **Defer uploads** to next budget period

4. **Transfer budget** from another project:
   ```bash
   cargoship budget set project1 --cost 12000  # Increase
   cargoship budget set project2 --cost 8000   # Decrease
   ```

---

### Q: Can I export budget data to CloudWatch?

**A**: Yes, integrate with CloudWatch using JSON output:

```bash
# Get budget metrics
BUDGET_USED=$(cargoship budget status project1 --json | jq '.budget_used')

# Publish to CloudWatch
aws cloudwatch put-metric-data \
  --namespace CargoShip/Budget \
  --metric-name BudgetUsed \
  --value $BUDGET_USED \
  --dimensions Project=project1
```

---

## Additional Resources

- [Cost Management Documentation](cost-management.md)
- [CLI Reference](CLI_REFERENCE.md)
- [Deployment Guide - Budget Configuration](DEPLOYMENT_GUIDE.md#budget-monitoring)
- [S3 Pricing](https://aws.amazon.com/s3/pricing/)

---

**Need Help?**
- GitHub Issues: https://github.com/scttfrdmn/cargoship/issues
- Discussions: https://github.com/scttfrdmn/cargoship/discussions
- Documentation: https://cargoship.app
