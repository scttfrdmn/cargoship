# Budget System Integration Design
**Date**: November 2025
**Purpose**: Design CargoShip budget system for future integration with Prism

---

## 🎯 Integration Goals

### Primary Objectives
1. **Shared Budget Pool** - Prism (compute) and CargoShip (storage) draw from same grant budget
2. **Unified Reporting** - Single cost view across compute + storage
3. **Coordinated Enforcement** - Budget limits apply to combined spending
4. **Data Compatibility** - Similar data structures for easy integration

### Use Case Example
```
PI has $10,000 NIH grant for 3 years:
- Prism EC2 instances: $6,500 spent
- CargoShip S3 storage: $2,200 spent
- Total spent: $8,700 / $10,000 (87%)
- Remaining: $1,300

Alert: "Approaching 90% of grant budget across compute + storage"
```

---

## 📊 Prism Budget Architecture Analysis

### Key Components (from /Users/scttfrdmn/src/prism)

#### 1. Core Types (`pkg/types/types.go`)
```go
type ProjectBudget struct {
    ID              string
    ProjectID       string
    Amount          float64
    Currency        string
    Period          BudgetPeriod  // Monthly, Quarterly, Yearly, Grant
    StartDate       time.Time
    EndDate         *time.Time    // Nil for ongoing

    // Alerts
    Alerts          []BudgetAlert
    AlertChannels   []AlertChannel  // Email, Slack, Webhook

    // Auto Actions
    AutoActions     []BudgetAutoAction

    // Tracking
    CurrentSpending float64
    LastUpdated     time.Time
}

type BudgetPeriod string
const (
    BudgetPeriodMonthly    BudgetPeriod = "monthly"
    BudgetPeriodQuarterly  BudgetPeriod = "quarterly"
    BudgetPeriodYearly     BudgetPeriod = "yearly"
    BudgetPeriodGrant      BudgetPeriod = "grant"      // NEW for academic
    BudgetPeriodCustom     BudgetPeriod = "custom"
)

type BudgetAlert struct {
    Threshold       float64  // Percentage (e.g., 75, 90, 95)
    Triggered       bool
    LastTriggered   *time.Time
}

type BudgetAutoAction struct {
    Threshold       float64
    Action          AutoActionType  // Warn, Hibernate, Stop, PreventLaunch
    Enabled         bool
}
```

#### 2. Budget Tracker (`pkg/project/budget_tracker.go`)
```go
type BudgetTracker struct {
    budgetPath     string                          // ~/.prism/budget_data.json
    budgetData     map[string]*ProjectBudgetData   // ProjectID -> Data
    costCalculator *CostCalculator
    actionExecutor ActionExecutor                  // Execute auto actions
}

type ProjectBudgetData struct {
    ProjectID    string
    Budget       *types.ProjectBudget
    CostHistory  []CostDataPoint      // Time series data
    AlertHistory []AlertEvent         // Alert log
    LastUpdated  time.Time
}

type CostDataPoint struct {
    Timestamp     time.Time
    TotalCost     float64
    InstanceCosts []types.InstanceCost   // EC2 instances
    StorageCosts  []types.StorageCost    // EBS volumes
    DailyCost     float64
}
```

#### 3. CLI Commands (`internal/cli/budget_commands.go`)
```bash
prism budget list                    # List all budgets
prism budget create <project> <amt>  # Create budget
prism budget status <budget-id>      # Current status
prism budget usage <budget-id>       # Detailed usage
prism budget history <budget-id>     # Spending history
prism budget forecast <budget-id>    # Future projection
prism budget breakdown <budget-id>   # Cost breakdown
prism budget savings <budget-id>     # Hibernation savings
```

---

## 🏗️ CargoShip Budget System Design

### Design Principles
1. **Structural Compatibility** - Use similar types as Prism
2. **Namespace Separation** - CargoShip tracks S3, Prism tracks EC2
3. **Shared Budget ID** - Same ProjectID references shared grant
4. **Future Merge Path** - Easy to combine into unified system

### Data Structures

#### pkg/budget/types.go
```go
package budget

import (
    "time"
)

// Grant represents a multi-year research grant
// Compatible with Prism's ProjectBudget but extended for academic needs
type Grant struct {
    ID              string        `json:"id"`
    Name            string        `json:"name"`
    TotalAmount     float64       `json:"total_amount"`
    Currency        string        `json:"currency"`
    StartDate       time.Time     `json:"start_date"`
    EndDate         time.Time     `json:"end_date"`

    // Grant-specific fields (not in Prism yet)
    GrantNumber     string        `json:"grant_number"`     // e.g., "NIH R01 GM123456"
    FundingAgency   string        `json:"funding_agency"`   // e.g., "NIH", "NSF"
    PI              string        `json:"pi"`               // Principal Investigator
    Institution     string        `json:"institution"`

    // Budget controls (compatible with Prism)
    Alerts          []BudgetAlert     `json:"alerts"`
    AutoActions     []BudgetAutoAction `json:"auto_actions"`

    // Tracking
    CurrentSpending float64       `json:"current_spending"`  // CargoShip S3 costs only
    LastUpdated     time.Time     `json:"last_updated"`
}

// Project represents a sub-allocation within a grant
// Maps to Prism's Project concept
type Project struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    GrantID         string    `json:"grant_id"`        // Links to Grant
    AllocationPct   float64   `json:"allocation_pct"`  // Percentage of grant (0-100)

    // Metadata for organization
    Tags            []string  `json:"tags"`            // e.g., ["genomics", "sequencing"]
    PI              string    `json:"pi"`
    StartDate       time.Time `json:"start_date"`
    EndDate         *time.Time `json:"end_date"`       // Nil if ongoing

    // Tracking (CargoShip S3 costs for this project)
    CurrentSpending float64   `json:"current_spending"`
    LastUpdated     time.Time `json:"last_updated"`
}

// BudgetAlert represents a spending threshold alert
// IDENTICAL to Prism's structure for compatibility
type BudgetAlert struct {
    Threshold       float64    `json:"threshold"`        // Percentage (e.g., 75.0)
    Triggered       bool       `json:"triggered"`
    LastTriggered   *time.Time `json:"last_triggered"`
    NotifyChannels  []string   `json:"notify_channels"`  // ["email", "slack"]
}

// BudgetAutoAction defines automated responses to budget thresholds
// IDENTICAL to Prism's structure for compatibility
type BudgetAutoAction struct {
    Threshold       float64        `json:"threshold"`
    Action          AutoActionType `json:"action"`
    Enabled         bool           `json:"enabled"`
}

type AutoActionType string
const (
    ActionWarn           AutoActionType = "warn"
    ActionPreventUpload  AutoActionType = "prevent_upload"   // CargoShip-specific
    ActionNotify         AutoActionType = "notify"
    // Future: ActionArchiveToGlacier, ActionDeleteOldData
)

// StorageCostPoint represents S3 storage costs at a point in time
// Parallel to Prism's CostDataPoint but for S3
type StorageCostPoint struct {
    Timestamp       time.Time           `json:"timestamp"`
    TotalCost       float64             `json:"total_cost"`
    BucketCosts     []BucketCost        `json:"bucket_costs"`
    StorageClasses  map[string]float64  `json:"storage_classes"`  // STANDARD: $X, GLACIER: $Y
    UploadCosts     float64             `json:"upload_costs"`     // PUT/POST requests
    DownloadCosts   float64             `json:"download_costs"`   // GET requests
    TransferCosts   float64             `json:"transfer_costs"`   // Data transfer out
}

type BucketCost struct {
    Bucket          string    `json:"bucket"`
    Region          string    `json:"region"`
    StorageGB       float64   `json:"storage_gb"`
    Cost            float64   `json:"cost"`
    StorageClass    string    `json:"storage_class"`
}

// BudgetData is the persistent storage format
// Parallel to Prism's ProjectBudgetData
type BudgetData struct {
    Grants          map[string]*Grant              `json:"grants"`           // GrantID -> Grant
    Projects        map[string]*Project            `json:"projects"`         // ProjectID -> Project
    CostHistory     map[string][]StorageCostPoint  `json:"cost_history"`     // ProjectID -> History
    AlertHistory    map[string][]AlertEvent        `json:"alert_history"`    // ProjectID -> Alerts
    LastUpdated     time.Time                      `json:"last_updated"`
}

type AlertEvent struct {
    Timestamp       time.Time     `json:"timestamp"`
    AlertType       string        `json:"alert_type"`
    Threshold       float64       `json:"threshold"`
    SpentAmount     float64       `json:"spent_amount"`
    Message         string        `json:"message"`
    ProjectID       string        `json:"project_id"`
    GrantID         string        `json:"grant_id"`
    Resolved        bool          `json:"resolved"`
}
```

### Storage Location
```
~/.cargoship/
  budget_data.json          # Budget state (parallel to Prism's)
  cost_cache/               # AWS pricing cache
    pricing_us-west-2.json
    pricing_us-east-1.json
```

### Integration Points

#### Shared Budget Scenario
```json
// Prism's budget_data.json (~/.prism/budget_data.json)
{
  "projects": {
    "nih-r01-2023": {
      "project_id": "nih-r01-2023",
      "budget": {
        "amount": 10000,
        "current_spending": 6500  // EC2 compute costs
      }
    }
  }
}

// CargoShip's budget_data.json (~/.cargoship/budget_data.json)
{
  "grants": {
    "nih-r01-2023": {
      "id": "nih-r01-2023",
      "total_amount": 10000,
      "current_spending": 2200  // S3 storage costs
    }
  }
}

// Future: Unified view (in either Prism or CargoShip daemon)
{
  "grants": {
    "nih-r01-2023": {
      "total": 10000,
      "compute_spending": 6500,  // From Prism
      "storage_spending": 2200,  // From CargoShip
      "total_spending": 8700,
      "remaining": 1300
    }
  }
}
```

---

## 🔌 Integration Architecture

### Phase 1: Independent Operation (v0.6.0 - Now)
```
┌─────────────────┐        ┌─────────────────┐
│  Prism          │        │  CargoShip      │
│  - EC2 costs    │        │  - S3 costs     │
│  - Budget DB    │        │  - Budget DB    │
│  ~/.prism/      │        │  ~/.cargoship/  │
└─────────────────┘        └─────────────────┘
      ↓                           ↓
   User manually combines costs
```

### Phase 2: Shared Daemon Integration (v0.8.0 - Future)
```
┌──────────────────────────────────────┐
│  Unified Budget Daemon               │
│  - Reads both Prism + CargoShip data │
│  - Combines costs                    │
│  - Unified alerts                    │
└──────────────────────────────────────┘
         ↓              ↓
   ┌─────────┐    ┌──────────┐
   │ Prism   │    │CargoShip │
   │ CLI     │    │ CLI      │
   └─────────┘    └──────────┘
```

### Phase 3: Fully Unified System (v1.0.0 - Future)
```
┌─────────────────────────────────────┐
│  Research Computing Budget Manager  │
│  - Compute (Prism)                  │
│  - Storage (CargoShip)              │
│  - Unified grant tracking           │
│  - Combined reporting               │
└─────────────────────────────────────┘
```

---

## 🛠️ Implementation Strategy

### CargoShip v0.6.0 (Now)
**Goal**: Independent budget system with Prism-compatible structures

**Components**:
1. **pkg/budget/** - Core budget tracking
   - Use Prism-compatible types
   - Store in `~/.cargoship/budget_data.json`
   - S3 cost tracking only

2. **CLI Commands** - Match Prism patterns
   ```bash
   cargoship budget list
   cargoship budget create <grant-name> <amount>
   cargoship budget status <grant-id>
   cargoship budget breakdown <grant-id>
   ```

3. **Upload Integration**
   ```bash
   cargoship upload --project rnaseq data/
   # Automatically tracks cost to "rnaseq" project
   # Checks budget before upload
   ```

### CargoShip v0.7.0 (3-6 months)
**Goal**: Multi-project support, better reporting

**Additions**:
- Project-level cost allocation
- Export budget data for external tools
- Shared configuration for lab teams

### Integration v0.8.0 (6-12 months)
**Goal**: Basic Prism integration

**Approach**:
1. **Shared Budget API** - Both tools can query combined spending
2. **Data Export** - CargoShip exports costs in Prism-compatible format
3. **Unified Reporting** - Single report combining compute + storage

### Full Integration v1.0.0+ (12+ months)
**Goal**: Single unified budget system

**Approach**:
- Merge budget tracking into shared daemon
- Single CLI for both tools
- Unified grant management

---

## 📝 Design Decisions

### ✅ Use Compatible Data Structures
- CargoShip types mirror Prism types
- Easy to merge later
- Users can manually combine data if needed

### ✅ Separate Storage Locations (For Now)
- `~/.prism/budget_data.json` - Prism compute costs
- `~/.cargoship/budget_data.json` - CargoShip storage costs
- Prevents conflicts, allows independent operation

### ✅ Shared Project/Grant IDs
- Same ID (`"nih-r01-2023"`) can be used in both tools
- Manual aggregation possible
- Future automated aggregation easy

### ✅ Compatible CLI Patterns
- Similar command structure
- Similar output formats
- Users comfortable with one tool can use the other

### ⚠️ Defer Full Integration
- Don't try to solve everything now
- Focus on CargoShip's core mission (S3 archival)
- Build integration hooks for future

---

## 🎯 Immediate Implementation Plan

### Week 1: Core Budget Tracker
```go
// pkg/budget/tracker.go
type Tracker struct {
    dataPath    string
    data        *BudgetData
    costCache   *CostCache
}

func NewTracker() (*Tracker, error)
func (t *Tracker) InitGrant(grant *Grant) error
func (t *Tracker) TrackUpload(projectID string, cost float64) error
func (t *Tracker) CheckBudget(projectID string, estimatedCost float64) error
func (t *Tracker) GetStatus(grantID string) (*GrantStatus, error)
```

### Week 2: Cost Estimation
```go
// pkg/budget/estimator.go
type Estimator struct {
    pricingCache *PricingCache
}

func (e *Estimator) EstimateUpload(size int64, storageClass string) (float64, error)
func (e *Estimator) EstimateStorage(size int64, storageClass string, months int) (float64, error)
func (e *Estimator) Compare(size int64, classes []string) (map[string]float64, error)
```

### Week 3: CLI Integration
```bash
cargoship budget init --grant "NIH R01" --amount 10000 --duration 5y
cargoship budget project add rnaseq --allocation 40%
cargoship upload --project rnaseq data/
cargoship budget status
```

---

## 🔍 Integration Testing Scenarios

### Scenario 1: Independent Usage
```bash
# User uses both tools separately
prism launch ml-instance    # Tracks to Prism budget
cargoship upload data/      # Tracks to CargoShip budget

# User manually adds costs from both tools
```

### Scenario 2: Shared Grant ID
```bash
# Both tools use same grant ID
prism budget create "NIH-R01" 10000
cargoship budget init --grant "NIH-R01" --amount 10000

# Each tracks their own costs
# Future: Daemon aggregates both
```

### Scenario 3: Export/Import
```bash
# CargoShip exports costs
cargoship budget export --format json > cargoship-costs.json

# Import into Prism (future)
prism budget import cargoship-costs.json --merge

# Combined view
prism budget status --show-storage
```

---

## 📊 Success Metrics

### Technical Compatibility
- [ ] CargoShip types compatible with Prism types
- [ ] Data can be exported in Prism-compatible format
- [ ] Shared project IDs work across tools
- [ ] Similar CLI command patterns

### User Experience
- [ ] Users can track grant budgets in CargoShip
- [ ] Budget enforcement works for S3 uploads
- [ ] Reports useful for PIs and grant management
- [ ] Easy to understand coming from Prism

### Future Integration Readiness
- [ ] Data migration path documented
- [ ] API hooks for external budget aggregation
- [ ] Daemon integration architecture defined
- [ ] No blocking technical debt

---

## 🚀 Next Steps

1. **Implement Core Tracker** (this week)
   - BudgetTracker with Prism-compatible structures
   - Grant and Project types
   - Persistence to JSON

2. **Test Compatibility** (week 2)
   - Export CargoShip budget data
   - Verify structure matches Prism expectations
   - Document any differences

3. **CLI Integration** (week 3)
   - Commands matching Prism patterns
   - Test with beta users familiar with Prism

4. **Integration Planning** (week 4)
   - Document detailed integration architecture
   - Prototype shared daemon concept
   - Plan v0.8.0 integration milestone

---

**Key Insight**: By using Prism-compatible data structures NOW, we make future integration straightforward while maintaining CargoShip's independence and focus on S3 archival optimization.
