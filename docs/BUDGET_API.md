# CargoShip Budget & Cost Management API Documentation

**Version:** v0.6.0
**Package:** `github.com/scttfrdmn/cargoship/pkg/aws/cost`
**Last Updated:** 2025-12-09

## Table of Contents

1. [Overview](#overview)
2. [Package Architecture](#package-architecture)
3. [Core Types](#core-types)
4. [Manager API](#manager-api)
5. [Budget Tracking](#budget-tracking)
6. [Cost Reporting](#cost-reporting)
7. [Forecasting](#forecasting)
8. [Alert Notifications](#alert-notifications)
9. [Integration Examples](#integration-examples)
10. [Error Handling](#error-handling)

## Overview

The `pkg/aws/cost` package provides comprehensive budget and cost management functionality for CargoShip. It supports:

- **Budget Enforcement**: Cost budgets and volume quotas per project
- **Cost Tracking**: Real-time cost recording with project-based tracking
- **Forecasting**: ML-based cost prediction with 4 models
- **Burn Rate Analysis**: Historical trends and future predictions
- **Multi-Channel Alerts**: Email, Slack, webhooks, CloudWatch

### Key Features

- Thread-safe operations with `sync.RWMutex`
- Project-based cost isolation
- Hierarchical budget enforcement (project → global)
- ML forecasting with confidence intervals
- Graceful alert delivery (one channel failure doesn't block others)

## Package Architecture

```
pkg/aws/cost/
├── manager.go         - Central cost management orchestrator
├── budget.go          - Budget enforcement and quota checking
├── reporting.go       - Cost recording and reporting
├── forecasting.go     - ML forecasting and burn rate analysis
├── alerts.go          - Multi-channel alert notifications
└── pricing.go         - AWS pricing calculations
```

### Component Relationships

```
Manager
├── PricingManager    (cost estimation)
├── CostReporter      (cost recording)
├── BudgetTracker     (budget enforcement)
└── ForecastEngine    (predictions)
    └── BudgetAlertNotifier (notifications)
```

## Core Types

### Manager

Central orchestrator for all cost management operations.

```go
type Manager struct {
    config        *config.CostControlConfig
    pricingMgr    *PricingManager
    reporter      *CostReporter
    budgetTracker *BudgetTracker
    logger        *slog.Logger
    awsConfig     aws.Config
}
```

**Constructor:**

```go
func NewManager(
    cfg *config.CostControlConfig,
    awsCfg aws.Config,
    logger *slog.Logger,
) (*Manager, error)
```

**Parameters:**
- `cfg`: Cost control configuration (required)
- `awsCfg`: AWS SDK configuration for S3/CloudWatch (required)
- `logger`: Structured logger (`nil` uses default)

**Returns:**
- `*Manager`: Initialized manager instance
- `error`: Configuration validation errors

**Example:**

```go
import (
    "context"
    "log/slog"

    "github.com/aws/aws-sdk-go-v2/config"
    cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
    "github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

func main() {
    ctx := context.Background()

    // Load AWS config
    awsCfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Create CargoShip config
    cargoCfg := cargoconfig.DefaultAWSConfig()

    // Initialize cost manager
    manager, err := cost.NewManager(
        &cargoCfg.CostControl,
        awsCfg,
        slog.Default(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Use manager...
}
```

### BudgetStatus

Represents current budget status with projections.

```go
type BudgetStatus struct {
    // Identification
    ProjectID   string
    BudgetType  string  // "global" or "project"
    PeriodType  string  // "monthly", "quarterly", "grant"
    PeriodStart string  // ISO 8601
    PeriodEnd   string  // ISO 8601

    // Cost budget
    MaxBudget       float64  // USD
    CurrentSpend    float64  // USD
    BudgetRemaining float64  // USD
    BudgetUsed      float64  // 0.0-1.0 (percentage)
    AlertThreshold  float64  // 0.0-1.0 (percentage)

    // Time tracking
    DaysElapsed   int
    DaysRemaining int
    TotalDays     int

    // Burn rate and projections
    DailyBurnRate     float64  // USD/day
    ProjectedEOPSpend float64  // End-of-period
    WillExceedBudget  bool
    TargetDailyRate   float64  // To stay within budget

    // Status flags
    OverBudget     bool
    AlertTriggered bool
    Currency       string

    // Volume quota (optional)
    MaxVolumeGB          float64
    CurrentVolumeGB      float64
    VolumeRemaining      float64
    VolumeUsed           float64  // 0.0-1.0
    VolumeAlertThreshold float64  // 0.0-1.0
    DailyVolumeBurnRate  float64  // GB/day
    ProjectedEOPVolume   float64  // GB
    WillExceedVolume     bool
    OverVolume           bool
    VolumeAlertTriggered bool
}
```

### CostRecord

Individual cost entry.

```go
type CostRecord struct {
    Timestamp       time.Time
    Operation       string            // "upload", "download", "storage"
    Service         string            // "s3", "ec2", etc.
    Region          string
    StorageClass    string
    SizeBytes       int64
    SizeGB          float64
    Cost            float64
    OriginalCost    float64
    DiscountApplied float64
    Currency        string
    FileName        string
    JobID           string
    ProjectID       string            // Manifest upload ID
    Tags            map[string]string
}
```

### CostForecast

ML-based cost prediction.

```go
type CostForecast struct {
    Model           ForecastModel  // linear, exponential, moving_average, ensemble
    GeneratedAt     time.Time
    ForecastDays    int
    BaseCost        float64
    BaseDate        time.Time
    HistoricalDays  int

    // Predictions
    Predicted7Days  float64
    Predicted14Days float64
    Predicted30Days float64
    Predicted60Days float64
    Predicted90Days float64

    // Daily forecasts (day -> cost)
    DailyForecasts map[int]float64

    // Confidence intervals
    Confidence7Days  *ConfidenceInterval
    Confidence30Days *ConfidenceInterval
    Confidence90Days *ConfidenceInterval

    // Model performance
    ModelAccuracy         float64  // 0.0-1.0
    MeanAbsoluteError     float64  // MAE
    RootMeanSquaredError  float64  // RMSE
    R2Score               float64  // Coefficient of determination

    // Budget impact
    BudgetExhaustionDate  *time.Time
    DaysUntilExhaustion   int
    ExhaustionProbability float64  // 0.0-1.0
}
```

### BudgetAlert

Budget threshold alert.

```go
type BudgetAlert struct {
    ID        string
    Timestamp time.Time
    Type      BudgetAlertType      // cost_threshold, volume_threshold, etc.
    Severity  BudgetAlertSeverity  // info, warning, critical

    ProjectID   string
    Description string
    IsGlobal    bool

    // Cost metrics
    MaxBudget         float64
    CurrentSpend      float64
    BudgetRemaining   float64
    BudgetUsedPercent float64
    ThresholdPercent  float64

    // Volume metrics
    MaxVolumeGB           float64
    CurrentVolumeGB       float64
    VolumeRemaining       float64
    VolumeUsedPercent     float64
    VolumeThresholdPercent float64

    Recommendation string
    ActionRequired bool
}
```

## Manager API

### Cost Estimation

#### EstimateOperationCost

Estimates cost before operation execution.

```go
func (m *Manager) EstimateOperationCost(
    ctx context.Context,
    operation string,
    sizeGB float64,
    storageClass config.StorageClass,
    region string,
) (*CostEstimate, error)
```

**Parameters:**
- `operation`: "upload", "archive", "download"
- `sizeGB`: Data size in gigabytes
- `storageClass`: `STANDARD`, `INTELLIGENT_TIERING`, `GLACIER`, etc.
- `region`: AWS region (e.g., "us-west-2")

**Returns:**
- `*CostEstimate`: Detailed cost breakdown
- `error`: Pricing lookup errors

**Example:**

```go
estimate, err := manager.EstimateOperationCost(
    ctx,
    "upload",
    100.0,  // 100 GB
    config.StorageClassINTELLIGENT_TIERING,
    "us-west-2",
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Estimated monthly cost: $%.2f\n", estimate.TotalCost)
```

### Budget Enforcement

#### CheckProjectBudget

Checks if operation would exceed project budget.

```go
func (m *Manager) CheckProjectBudget(
    projectID string,
    additionalCost float64,
) error
```

**Parameters:**
- `projectID`: Project identifier (manifest upload ID)
- `additionalCost`: Estimated operation cost in USD

**Returns:**
- `nil`: Within budget limits
- `*BudgetExceededError`: Budget would be exceeded
- `error`: Other errors

**Example:**

```go
// Check before expensive operation
err := manager.CheckProjectBudget("20251206-abc123", 50.00)
if err != nil {
    var budgetErr *cost.BudgetExceededError
    if errors.As(err, &budgetErr) {
        fmt.Printf("Budget exceeded: overage $%.2f\n", budgetErr.Overage)
        // Request approval or abort
    }
    return err
}

// Proceed with operation
```

#### CheckProjectVolumeQuota

Checks if operation would exceed volume quota.

```go
func (m *Manager) CheckProjectVolumeQuota(
    projectID string,
    additionalGB float64,
) error
```

**Parameters:**
- `projectID`: Project identifier
- `additionalGB`: Data volume in gigabytes

**Returns:**
- `nil`: Within quota limits
- `*VolumeQuotaExceededError`: Quota would be exceeded
- `error`: Other errors

**Example:**

```go
// Check volume quota
sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
err := manager.CheckProjectVolumeQuota("20251206-abc123", sizeGB)
if err != nil {
    var quotaErr *cost.VolumeQuotaExceededError
    if errors.As(err, &quotaErr) {
        fmt.Printf("Quota exceeded: overage %.2f GB\n", quotaErr.Overage)
    }
    return err
}
```

#### SetProjectBudget

Creates or updates project budget.

```go
func (m *Manager) SetProjectBudget(
    projectID string,
    maxBudget float64,
    maxVolumeGB float64,
    costAlertThreshold float64,
    volumeAlertThreshold float64,
) error
```

**Parameters:**
- `projectID`: Project identifier
- `maxBudget`: Maximum cost in USD (0 = unlimited)
- `maxVolumeGB`: Maximum volume in GB (0 = unlimited)
- `costAlertThreshold`: Alert percentage (0.0-1.0)
- `volumeAlertThreshold`: Alert percentage (0.0-1.0)

**Example:**

```go
// Set budget: $1000, 500GB, 80% cost alert, 75% volume alert
err := manager.SetProjectBudget(
    "20251206-abc123",
    1000.0,
    500.0,
    0.80,
    0.75,
)
```

#### GetProjectBudgetStatus

Retrieves current budget status.

```go
func (m *Manager) GetProjectBudgetStatus(
    projectID string,
) (*BudgetStatus, error)
```

**Returns:**
- `*BudgetStatus`: Complete status with projections
- `error`: Lookup errors

**Example:**

```go
status, err := manager.GetProjectBudgetStatus("20251206-abc123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Budget: $%.2f / $%.2f (%.1f%%)\n",
    status.CurrentSpend,
    status.MaxBudget,
    status.BudgetUsed*100)

if status.WillExceedBudget {
    fmt.Println("Warning: Projected to exceed budget")
    fmt.Printf("Projected EOP spend: $%.2f\n", status.ProjectedEOPSpend)
}
```

### Cost Recording

#### RecordOperationCost

Records actual cost of completed operation.

```go
func (m *Manager) RecordOperationCost(
    ctx context.Context,
    operation string,
    fileName string,
    sizeBytes int64,
    storageClass config.StorageClass,
    region string,
    jobID string,
    projectID string,
    tags map[string]string,
) error
```

**Parameters:**
- All previous parameters plus:
- `fileName`: File name for tracking
- `jobID`: Job identifier (optional)
- `projectID`: Project identifier (manifest upload ID)
- `tags`: Custom metadata tags

**Returns:**
- `error`: Budget enforcement or recording errors

**Note:** This function enforces budgets **before** recording costs.

**Example:**

```go
err := manager.RecordOperationCost(
    ctx,
    "upload",
    "data.tar.gz",
    524288000,  // 500 MB
    config.StorageClassSTANDARD,
    "us-west-2",
    "job-123",
    "20251206-abc123",  // Project ID
    map[string]string{
        "department": "engineering",
        "team": "data",
    },
)
if err != nil {
    // Operation blocked by budget enforcement
    return err
}
```

## Cost Reporting

### Getting the Reporter

```go
func (m *Manager) GetReporter() *CostReporter
```

Returns the internal `CostReporter` for advanced operations.

### Project Cost Tracking

#### GetProjectCosts

Returns total costs for a project.

```go
func (cr *CostReporter) GetProjectCosts(projectID string) float64
```

#### GetProjectSummary

Returns detailed project metrics.

```go
func (cr *CostReporter) GetProjectSummary(
    projectID string,
) (*ProjectSummary, error)
```

**Returns:**

```go
type ProjectSummary struct {
    ProjectID       string
    TotalCost       float64
    TotalSavings    float64
    FilesProcessed  int
    TotalVolumeGB   float64
    AvgFileSize     float64
    PeriodStart     time.Time
    PeriodEnd       time.Time
    ByRegion        map[string]float64
    ByStorageClass  map[string]float64
    DailyCosts      map[string]float64
}
```

**Example:**

```go
reporter := manager.GetReporter()
summary, err := reporter.GetProjectSummary("20251206-abc123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Project: %s\n", summary.ProjectID)
fmt.Printf("Total cost: $%.2f\n", summary.TotalCost)
fmt.Printf("Savings: $%.2f (compression)\n", summary.TotalSavings)
fmt.Printf("Files: %d\n", summary.FilesProcessed)
fmt.Printf("Volume: %.2f GB\n", summary.TotalVolumeGB)
```

#### ListProjects

Returns all project IDs with costs.

```go
func (cr *CostReporter) ListProjects() []string
```

### Cost Summaries

#### GenerateCostSummary

Generates cost summary for a time period.

```go
func (cr *CostReporter) GenerateCostSummary(
    period string,
) (*CostSummary, error)
```

**Parameters:**
- `period`: "day", "week", "month", "year", or "custom"

**Returns:**

```go
type CostSummary struct {
    Period          string
    TotalCost       float64
    TotalSavings    float64
    Currency        string
    ByService       map[string]float64
    ByRegion        map[string]float64
    ByStorageClass  map[string]float64
    ByOperation     map[string]float64
    ByProject       map[string]float64
    TopFiles        []CostRecord
    DailyCosts      map[string]float64
    Trends          CostTrends
    Recommendations []CostRecommendation
}
```

**Example:**

```go
summary, err := reporter.GenerateCostSummary("month")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("=== Cost Summary (%s) ===\n", summary.Period)
fmt.Printf("Total: $%.2f\n", summary.TotalCost)

fmt.Println("\n=== By Region ===")
for region, cost := range summary.ByRegion {
    pct := (cost / summary.TotalCost) * 100
    fmt.Printf("%s: $%.2f (%.1f%%)\n", region, cost, pct)
}

fmt.Println("\n=== By Project ===")
for proj, cost := range summary.ByProject {
    fmt.Printf("%s: $%.2f\n", proj, cost)
}
```

## Forecasting

### ForecastEngine

ML-based cost forecasting.

```go
type ForecastEngine struct {
    reporter *CostReporter
}

func NewForecastEngine(reporter *CostReporter) *ForecastEngine
```

### Generating Forecasts

#### GenerateForecast

Creates cost forecast using specified model.

```go
func (fe *ForecastEngine) GenerateForecast(
    projectID string,
    days int,
    model ForecastModel,
) (*CostForecast, error)
```

**Parameters:**
- `projectID`: Project ID (empty string for all projects)
- `days`: Days to forecast (typically 7-90)
- `model`: `ForecastModelLinear`, `ForecastModelExponential`, `ForecastModelMovingAverage`, `ForecastModelEnsemble`

**Example:**

```go
reporter := manager.GetReporter()
engine := cost.NewForecastEngine(reporter)

// Generate 30-day forecast using ensemble model
forecast, err := engine.GenerateForecast(
    "",  // All projects
    30,
    cost.ForecastModelEnsemble,
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("=== Cost Forecast (%s model) ===\n", forecast.Model)
fmt.Printf("Historical data: %d days\n", forecast.HistoricalDays)
fmt.Printf("Model accuracy: R²=%.3f\n", forecast.R2Score)

fmt.Println("\n=== Predictions ===")
fmt.Printf("7 days:  $%.2f (±$%.2f)\n",
    forecast.Predicted7Days,
    forecast.Confidence7Days.UpperBound - forecast.Predicted7Days)
fmt.Printf("30 days: $%.2f (±$%.2f)\n",
    forecast.Predicted30Days,
    forecast.Confidence30Days.UpperBound - forecast.Predicted30Days)
fmt.Printf("90 days: $%.2f (±$%.2f)\n",
    forecast.Predicted90Days,
    forecast.Confidence90Days.UpperBound - forecast.Predicted90Days)

if forecast.BudgetExhaustionDate != nil {
    fmt.Printf("\nBudget exhaustion: %s (%d days)\n",
        forecast.BudgetExhaustionDate.Format("2006-01-02"),
        forecast.DaysUntilExhaustion)
}
```

### Burn Rate Analysis

#### AnalyzeBurnRate

Analyzes spending velocity and trends.

```go
func (fe *ForecastEngine) AnalyzeBurnRate(
    projectID string,
    days int,
) (*BurnRateAnalysis, error)
```

**Example:**

```go
analysis, err := engine.AnalyzeBurnRate("", 30)
if err != nil {
    log.Fatal(err)
}

fmt.Println("=== Burn Rate Analysis ===")
fmt.Printf("Current rate: $%.2f/day\n", analysis.CurrentDailyRate)
fmt.Printf("Average rate: $%.2f/day\n", analysis.AverageDailyRate)
fmt.Printf("Volatility: %.1f%%\n", analysis.Volatility*100)

fmt.Printf("\nTrend: %s (%s strength)\n",
    analysis.TrendDirection,
    strengthLabel(analysis.TrendStrength))

if analysis.TrendDirection == "increasing" {
    fmt.Printf("Acceleration: +$%.2f/day²\n", analysis.AccelerationRate)
}

fmt.Println("\n=== Predicted Future Rates ===")
fmt.Printf("30 days: $%.2f/day\n", analysis.PredictedDailyRate30Days)
fmt.Printf("60 days: $%.2f/day\n", analysis.PredictedDailyRate60Days)
fmt.Printf("90 days: $%.2f/day\n", analysis.PredictedDailyRate90Days)
```

## Alert Notifications

### Configuration

```go
type BudgetAlertConfig struct {
    Enabled        bool
    CooldownPeriod time.Duration

    // Webhooks
    WebhookEnabled bool
    WebhookURL     string
    WebhookHeaders map[string]string

    // CloudWatch
    CloudWatchEnabled   bool
    CloudWatchNamespace string
    CloudWatchRegion    string

    // Email (SMTP)
    EmailEnabled    bool
    EmailRecipients []string
    SMTPHost        string
    SMTPPort        int
    SMTPUsername    string
    SMTPPassword    string
    SMTPFrom        string
    SMTPUseTLS      bool

    // Slack
    SlackEnabled    bool
    SlackWebhookURL string
    SlackChannel    string
    SlackUsername   string
}
```

### Creating Alert Notifier

```go
notifier := cost.NewBudgetAlertNotifier(config, awsConfig)
```

### Sending Alerts

#### SendAlert

Delivers alert to all configured channels.

```go
func (n *BudgetAlertNotifier) SendAlert(
    ctx context.Context,
    alert *BudgetAlert,
) error
```

**Example:**

```go
config := &cost.BudgetAlertConfig{
    Enabled:         true,
    CooldownPeriod:  24 * time.Hour,
    EmailEnabled:    true,
    EmailRecipients: []string{"admin@example.com"},
    SMTPHost:        "smtp.gmail.com",
    SMTPPort:        587,
    SMTPUsername:    "alerts@example.com",
    SMTPPassword:    "app-password",
    SMTPFrom:        "cargoship@example.com",
    SMTPUseTLS:      true,
}

notifier := cost.NewBudgetAlertNotifier(config, awsConfig)

alert := &cost.BudgetAlert{
    ID:          "alert-123",
    Timestamp:   time.Now(),
    Type:        cost.AlertTypeCostThreshold,
    Severity:    cost.SeverityWarning,
    ProjectID:   "20251206-abc123",
    Description: "Project exceeded 80% cost threshold",
    MaxBudget:   1000.00,
    CurrentSpend: 850.00,
    BudgetRemaining: 150.00,
    BudgetUsedPercent: 0.85,
    ThresholdPercent: 0.80,
    Recommendation: "Review spending or increase budget",
    ActionRequired: true,
}

err := notifier.SendAlert(ctx, alert)
if err != nil {
    log.Printf("Failed to send alert: %v", err)
}
```

## Integration Examples

### Complete Upload with Budget Enforcement

```go
func UploadWithBudgetEnforcement(
    ctx context.Context,
    manager *cost.Manager,
    fileName string,
    data []byte,
    projectID string,
) error {
    sizeBytes := int64(len(data))
    sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

    // 1. Estimate cost
    estimate, err := manager.EstimateOperationCost(
        ctx,
        "upload",
        sizeGB,
        config.StorageClassINTELLIGENT_TIERING,
        "us-west-2",
    )
    if err != nil {
        return fmt.Errorf("cost estimation failed: %w", err)
    }

    // 2. Check budget (cost AND volume)
    if err := manager.CheckProjectBudget(projectID, estimate.TotalCost); err != nil {
        return fmt.Errorf("budget check failed: %w", err)
    }

    if err := manager.CheckProjectVolumeQuota(projectID, sizeGB); err != nil {
        return fmt.Errorf("volume quota check failed: %w", err)
    }

    // 3. Perform upload
    if err := performS3Upload(ctx, fileName, data); err != nil {
        return fmt.Errorf("upload failed: %w", err)
    }

    // 4. Record actual cost
    err = manager.RecordOperationCost(
        ctx,
        "upload",
        fileName,
        sizeBytes,
        config.StorageClassINTELLIGENT_TIERING,
        "us-west-2",
        "",  // jobID
        projectID,
        nil,  // tags
    )
    if err != nil {
        log.Printf("Warning: failed to record cost: %v", err)
        // Don't fail upload if cost recording fails
    }

    return nil
}
```

### Budget Monitoring Service

```go
func RunBudgetMonitor(
    ctx context.Context,
    manager *cost.Manager,
    notifier *cost.BudgetAlertNotifier,
    interval time.Duration,
) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            checkAllProjects(ctx, manager, notifier)
        }
    }
}

func checkAllProjects(
    ctx context.Context,
    manager *cost.Manager,
    notifier *cost.BudgetAlertNotifier,
) {
    reporter := manager.GetReporter()
    projects := reporter.ListProjects()

    for _, projectID := range projects {
        status, err := manager.GetProjectBudgetStatus(projectID)
        if err != nil {
            log.Printf("Failed to get status for %s: %v", projectID, err)
            continue
        }

        // Check cost threshold
        if status.AlertTriggered && !status.OverBudget {
            alert := &cost.BudgetAlert{
                ID:                 fmt.Sprintf("alert-%d", time.Now().Unix()),
                Timestamp:          time.Now(),
                Type:               cost.AlertTypeCostThreshold,
                Severity:           cost.SeverityWarning,
                ProjectID:          projectID,
                Description:        fmt.Sprintf("Project exceeded %.0f%% cost threshold", status.AlertThreshold*100),
                MaxBudget:          status.MaxBudget,
                CurrentSpend:       status.CurrentSpend,
                BudgetRemaining:    status.BudgetRemaining,
                BudgetUsedPercent:  status.BudgetUsed,
                ThresholdPercent:   status.AlertThreshold,
                Recommendation:     "Review spending or increase budget",
                ActionRequired:     false,
            }

            if err := notifier.SendAlert(ctx, alert); err != nil {
                log.Printf("Failed to send alert for %s: %v", projectID, err)
            }
        }

        // Check budget exceeded
        if status.OverBudget {
            alert := &cost.BudgetAlert{
                ID:                 fmt.Sprintf("alert-%d", time.Now().Unix()),
                Timestamp:          time.Now(),
                Type:               cost.AlertTypeCostOverBudget,
                Severity:           cost.SeverityCritical,
                ProjectID:          projectID,
                Description:        "Project exceeded maximum budget",
                MaxBudget:          status.MaxBudget,
                CurrentSpend:       status.CurrentSpend,
                BudgetRemaining:    status.BudgetRemaining,
                BudgetUsedPercent:  status.BudgetUsed,
                Recommendation:     "URGENT: Halt operations or increase budget",
                ActionRequired:     true,
            }

            if err := notifier.SendAlert(ctx, alert); err != nil {
                log.Printf("Failed to send alert for %s: %v", projectID, err)
            }
        }
    }
}
```

## Error Handling

### Budget Errors

#### BudgetExceededError

```go
type BudgetExceededError struct {
    ProjectID      string
    BudgetType     string   // "global" or "project"
    MaxBudget      float64
    CurrentSpend   float64
    AdditionalCost float64
    ProjectedSpend float64
    Overage        float64
    Currency       string
}
```

**Checking:**

```go
err := manager.CheckProjectBudget(projectID, cost)
if err != nil {
    var budgetErr *cost.BudgetExceededError
    if errors.As(err, &budgetErr) {
        fmt.Printf("Budget exceeded by $%.2f\n", budgetErr.Overage)
        // Handle budget exceeded
    } else {
        // Other error
        return err
    }
}
```

#### VolumeQuotaExceededError

```go
type VolumeQuotaExceededError struct {
    ProjectID       string
    QuotaType       string   // "global" or "project"
    MaxVolumeGB     float64
    CurrentVolumeGB float64
    AdditionalGB    float64
    ProjectedVolume float64
    Overage         float64
}
```

**Checking:**

```go
err := manager.CheckProjectVolumeQuota(projectID, sizeGB)
if err != nil {
    var quotaErr *cost.VolumeQuotaExceededError
    if errors.As(err, &quotaErr) {
        fmt.Printf("Quota exceeded by %.2f GB\n", quotaErr.Overage)
        // Handle quota exceeded
    }
}
```

### Best Practices

1. **Always check budgets before operations**
2. **Handle budget errors gracefully** (don't fail silently)
3. **Use context for cancellation** in long-running operations
4. **Log cost recording failures** but don't fail operations
5. **Implement retry logic** for transient failures
6. **Monitor alert delivery** health
7. **Use ensemble model** for production forecasts
8. **Check R² score** before trusting forecasts (>0.8 is good)

## Thread Safety

All public methods are thread-safe:
- `Manager`: Uses internal locks in components
- `CostReporter`: Protected by `sync.RWMutex`
- `ForecastEngine`: Read-only access to reporter
- `BudgetAlertNotifier`: Stateless alert delivery

## Performance Considerations

- **Cost recording**: O(1) append to slice
- **Budget checks**: O(1) map lookup
- **Forecasting**: O(n) where n = historical days
- **Alert delivery**: Parallel, graceful failure

## See Also

- [Budget User Guide](BUDGET_USER_GUIDE.md) - End-user documentation
- [Alert Configuration Guide](ALERTS_CONFIGURATION.md) - Alert setup
- [Forecasting Guide](FORECASTING_GUIDE.md) - ML model details

---

**Version**: v0.6.0
**Last Updated**: 2025-12-09
**License**: MIT
