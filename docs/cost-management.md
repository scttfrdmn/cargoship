# CargoShip AWS Cost Management

CargoShip provides comprehensive AWS cost management features that help you track, estimate, and optimize your archival costs. The system supports various discount scenarios including Reserved Instances, Savings Plans, Enterprise Agreements, and custom negotiated rates.

## Features

- **Real-time Cost Estimation**: Get accurate cost estimates before performing operations
- **Flexible Discount Configuration**: Support for all AWS discount programs
- **Budget Tracking**: Monitor spending against configured budgets with alerts
- **Cost Reporting**: Generate detailed cost reports with trends and recommendations
- **AWS Pricing API Integration**: Automatically fetch current AWS pricing
- **Custom Pricing Support**: Override AWS pricing with your negotiated rates
- **Approval Workflows**: Require approval for operations exceeding cost thresholds

## Configuration

### Basic Configuration

```yaml
cost_control:
  max_monthly_budget: 5000.0      # USD
  alert_threshold: 0.85           # Alert at 85% of budget
  auto_optimize: true
  require_approval_over: 1000.0   # Require approval for operations > $1000

  pricing:
    use_aws_pricing_api: true
    currency: "USD"
    pricing_cache_duration: "24h"
```

### Discount Configuration

#### Global Discounts

Apply a percentage discount to all services:

```yaml
pricing:
  global_discount: 0.15  # 15% enterprise discount
```

#### Service-Specific Discounts

Configure different discounts for different AWS services:

```yaml
pricing:
  service_discounts:
    s3: 0.20          # 20% discount on S3
    ec2: 0.25         # 25% discount on EC2
    glacier: 0.10     # 10% discount on Glacier
```

#### Reserved Instance Discounts

Configure Reserved Instance savings:

```yaml
pricing:
  reserved_instance_discounts:
    s3:
      discount: 0.30             # 30% discount
      term: "3 years"
      payment_option: "All Upfront"
      instance_types: ["standard", "ia"]
```

#### Savings Plans Discounts

Configure Compute Savings Plans:

```yaml
pricing:
  savings_plans_discounts:
    compute:
      discount: 0.28             # 28% discount
      commitment: 500.0          # $500/hour commitment
      term: "3 years"
      plan_type: "Compute"
```

#### Enterprise Discounts

Configure volume-based enterprise discounts:

```yaml
pricing:
  enterprise_discount:
    enabled: true
    annual_commitment_discount: 0.05  # 5% for annual commitment
    
    volume_tiers:
      - minimum_spend: 1000.0    # $1000/month minimum
        discount: 0.05           # 5% additional discount
        services: ["s3", "glacier"]
      - minimum_spend: 25000.0   # $25000/month minimum
        discount: 0.18           # 18% additional discount
        services: []             # All services
```

### Custom Pricing

Override AWS pricing with your negotiated rates:

```yaml
pricing:
  custom_pricing:
    us-east-1:
      s3_storage:
        STANDARD: 0.019          # Custom rate instead of $0.023/GB
        GLACIER: 0.003           # Custom rate instead of $0.004/GB
      s3_requests:
        put_requests: 0.0004     # Per 1000 PUT requests
        get_requests: 0.0003     # Per 1000 GET requests
      data_transfer:
        out_to_internet:
          first_1gb: 0.0         # First 1GB free
          over_50gb: 0.07        # Over 50GB
```

### Cost Reporting

Configure automated cost reporting:

```yaml
reporting:
  enabled: true
  frequency: "weekly"          # daily, weekly, monthly
  detailed_breakdown: true
  export_format: "json"        # json, csv, pdf
  report_bucket: "cost-reports-bucket"
  
  email_recipients:
    - "finance@company.com"
    - "ops-team@company.com"
```

## Usage

### CLI Tool

CargoShip includes a dedicated cost management CLI tool:

```bash
# Build the CLI tool
go build -o cargoship-cost ./cmd/cargoship-cost

# Estimate cost for uploading 1GB to Glacier
./cargoship-cost -command=estimate -size=1GB -storage-class=GLACIER -region=us-east-1

# Generate monthly cost report
./cargoship-cost -command=report -period=month -format=json

# Check budget status
./cargoship-cost -command=budget -format=table

# Get current AWS pricing
./cargoship-cost -command=pricing -region=us-east-1 -format=table

# Validate configuration
./cargoship-cost -command=validate -config=config.yaml
```

### Programmatic Usage

```go
package main

import (
    "context"
    "log"
    
    "github.com/scttfrdmn/cargoship/pkg/aws/cost"
    "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

func main() {
    ctx := context.Background()
    
    // Load configuration
    cfg := config.DefaultAWSConfig()
    
    // Load AWS config
    awsCfg, err := config.LoadAWSConfig(ctx, cfg.Profile, cfg.Region)
    if err != nil {
        log.Fatal(err)
    }
    
    // Create cost manager
    costManager, err := cost.NewManager(&cfg.CostControl, awsCfg, nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Estimate cost for 1GB upload to S3 Standard
    estimate, err := costManager.EstimateOperationCost(
        ctx, "upload", 1.0, config.StorageClassStandard, "us-east-1")
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Estimated cost: $%.4f", estimate.TotalCost)
    
    // Check if operation requires approval
    needsApproval, err := costManager.CheckCostApproval(ctx, "upload", estimate.TotalCost)
    if err != nil {
        log.Fatal(err)
    }
    
    if needsApproval {
        log.Println("Operation requires cost approval")
    }
    
    // Record actual cost after operation
    err = costManager.RecordOperationCost(
        ctx, "upload", "large-file.zip", 1024*1024*1024, 
        config.StorageClassStandard, "us-east-1", "job-123", nil)
    if err != nil {
        log.Fatal(err)
    }
}
```

## Cost Estimation

The cost estimation system provides detailed breakdowns:

```json
{
  "storage_cost": 0.023,
  "request_cost": 0.0005,
  "data_transfer_cost": 0.0,
  "total_cost": 0.0235,
  "currency": "USD",
  "estimated_at": "2024-01-15T10:30:00Z",
  "discounts": {
    "global_discount": 0.00352,
    "service_discount": 0.0047,
    "volume_discount": 0.0,
    "total_discount": 0.00822,
    "original_cost": 0.03172,
    "discounted_cost": 0.0235
  },
  "breakdown": {
    "storage_breakdown": {
      "STANDARD": 0.023
    },
    "request_breakdown": {
      "PUT": 0.0005
    }
  }
}
```

## Budget Management

### Budget Alerts

Configure budget alerts to monitor spending:

```yaml
cost_control:
  max_monthly_budget: 5000.0
  alert_threshold: 0.85  # Alert at 85% of budget
```

### Budget Status

Check current budget status:

```bash
./cargoship-cost -command=budget
```

```
Budget Status
=============
Max Budget:      $5000.00
Current Spend:   $3247.52
Remaining:       $1752.48
Usage:           64.9%
Alert Threshold: 85.0%

✅ Within budget
```

## Cost Reports

### Report Types

- **Daily Reports**: Track daily spending patterns
- **Weekly Reports**: Weekly summaries with trends
- **Monthly Reports**: Comprehensive monthly analysis

### Report Contents

- Total costs and savings
- Breakdown by service, region, and storage class
- Cost trends and projections
- Top cost files
- Optimization recommendations

### Example Report

```json
{
  "period": "month",
  "total_cost": 1247.52,
  "total_savings": 312.15,
  "currency": "USD",
  "by_service": {
    "s3": 1098.23,
    "glacier": 149.29
  },
  "trends": {
    "daily_average": 41.58,
    "monthly_projection": 1247.52,
    "growth_rate": 15.2,
    "cost_per_gb": 0.0234
  },
  "recommendations": [
    {
      "type": "storage_class",
      "priority": "high",
      "description": "Archive data older than 90 days to Glacier",
      "potential_saving": 200.15,
      "implementation": "Set up S3 lifecycle rules"
    }
  ]
}
```

## Integration with CargoShip Operations

Cost management is automatically integrated with CargoShip operations:

### Archival Operations

```go
// Cost is automatically calculated and recorded
err := archiver.ArchiveFile(
    "/path/to/file", 
    "s3://bucket/key", 
    config.StorageClassGlacier, 
    metadata)
```

### Approval Workflows

Operations exceeding cost thresholds automatically trigger approval workflows:

```go
// Check if approval is needed
needsApproval, err := costManager.CheckCostApproval(ctx, "upload", estimatedCost)
if needsApproval {
    // Create approval request
    request := costManager.RequestCostApproval(
        "upload", estimatedCost, "Monthly backup", "ops-team")
    
    // Wait for approval...
}
```

## Best Practices

### 1. Configure Realistic Budgets

Set budgets based on historical usage and business requirements:

```yaml
cost_control:
  max_monthly_budget: 5000.0    # Based on historical data
  alert_threshold: 0.80         # Early warning
```

### 2. Use Appropriate Storage Classes

Choose storage classes based on access patterns:

- **STANDARD**: Frequently accessed data
- **STANDARD_IA**: Infrequently accessed data (>30 days)
- **GLACIER**: Archive data (>90 days)
- **DEEP_ARCHIVE**: Long-term archive (>180 days)

### 3. Configure Volume Discounts

Set up volume tiers to automatically apply enterprise discounts:

```yaml
enterprise_discount:
  volume_tiers:
    - minimum_spend: 10000.0
      discount: 0.15
      services: []  # All services
```

### 4. Regular Cost Reviews

Enable automated reporting to track spending trends:

```yaml
reporting:
  enabled: true
  frequency: "weekly"
  email_recipients:
    - "finance@company.com"
```

### 5. Optimize Storage Lifecycle

Use lifecycle policies to automatically transition data to cheaper storage classes.

## Troubleshooting

### AWS Pricing API Issues

If the AWS Pricing API is unavailable, CargoShip falls back to cached or default pricing:

```yaml
pricing:
  use_aws_pricing_api: true
  pricing_cache_duration: "48h"  # Longer cache during outages
```

### Discount Calculation

Discounts are applied in this order:
1. Global discount
2. Service-specific discount
3. Reserved Instance discount
4. Savings Plans discount
5. Enterprise volume discount

### Budget Alerts

Budget alerts have a 24-hour cooldown to prevent spam. Configure email notifications or integrate with your alerting system.

## API Reference

### CostManager

- `EstimateOperationCost(ctx, operation, sizeGB, storageClass, region)`: Estimate operation cost
- `CheckCostApproval(ctx, operation, cost)`: Check if approval required
- `RecordOperationCost(ctx, ...)`: Record actual operation cost
- `GenerateCostReport(ctx, period)`: Generate cost report
- `GetBudgetStatus()`: Get current budget status

### Configuration Types

- `PricingConfig`: Pricing and discount configuration
- `CostControlConfig`: Budget and approval settings
- `CostReportingConfig`: Report generation settings
- `EnterpriseDiscountConfig`: Enterprise discount configuration

For complete API documentation, see the generated Go docs.