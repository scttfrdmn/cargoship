#!/bin/bash
# Apply GitHub Organization Setup
# This script applies all the GitHub organization infrastructure to the repository

set -e  # Exit on error

REPO="scttfrdmn/cargoship"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🚀 Applying GitHub Organization Setup to $REPO"
echo "=============================================="
echo ""

# Check prerequisites
echo "📋 Checking prerequisites..."
if ! command -v gh &> /dev/null; then
    echo "❌ Error: gh CLI not found. Please install: brew install gh"
    exit 1
fi

if ! gh auth status &> /dev/null; then
    echo "❌ Error: Not authenticated with GitHub. Please run: gh auth login"
    exit 1
fi

echo "✅ Prerequisites OK"
echo ""

# Step 1: Push changes to GitHub
echo "📤 Step 1: Pushing changes to GitHub..."
cd "$REPO_ROOT"
if git push origin main; then
    echo "✅ Pushed to GitHub"
else
    echo "⚠️  Push failed - you may need to push manually:"
    echo "   cd $REPO_ROOT"
    echo "   git push origin main"
    echo ""
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi
echo ""

# Step 2: Sync labels
echo "🏷️  Step 2: Syncing labels..."
if gh label sync --repo "$REPO" --file .github/labels.yml; then
    echo "✅ Labels synced successfully"

    # Count labels
    LABEL_COUNT=$(gh label list --repo "$REPO" --limit 100 | wc -l)
    echo "   Total labels: $LABEL_COUNT"
else
    echo "❌ Failed to sync labels"
    exit 1
fi
echo ""

# Step 3: Create milestones
echo "📅 Step 3: Creating milestones..."

# Check if milestones already exist
EXISTING_MILESTONES=$(gh milestone list --repo "$REPO" --json title -q '.[].title')

create_milestone() {
    local version=$1
    local title=$2
    local description=$3
    local due_date=$4

    if echo "$EXISTING_MILESTONES" | grep -q "^$title$"; then
        echo "⏭️  Milestone $version already exists, skipping"
    else
        if gh milestone create "$version" --repo "$REPO" --title "$title" --description "$description" --due-date "$due_date"; then
            echo "✅ Created milestone: $version"
        else
            echo "❌ Failed to create milestone: $version"
            return 1
        fi
    fi
}

create_milestone "v0.6.0" \
    "v0.6.0 - Budget & Cost Management" \
    "Implement comprehensive budget tracking and cost management for academic researchers managing grant funds. Core BudgetTracker, Grant/Project types, cost estimation, budget CLI commands, alert system." \
    "2025-12-02"

create_milestone "v0.7.0" \
    "v0.7.0 - Data Lifecycle Management" \
    "Automated data tiering, archival policies, and lifecycle management for long-term storage optimization. Automated tiering to Glacier/Deep Archive, retention policies, compliance features." \
    "2025-12-30"

create_milestone "v0.8.0" \
    "v0.8.0 - Enterprise Features" \
    "Enterprise-grade features for multi-user environments. Multi-tenancy, advanced security (KMS, RBAC), team collaboration, enterprise monitoring and compliance." \
    "2026-01-27"

create_milestone "v1.0.0" \
    "v1.0.0 - Production Ready" \
    "First stable release with API stability guarantees, comprehensive documentation, and production validation. API stability commitment, >80% test coverage, production case studies, performance benchmarks." \
    "2026-02-24"

echo ""
echo "📊 Current milestones:"
gh milestone list --repo "$REPO"
echo ""

# Step 4: Create initial v0.6.0 issues
echo "📝 Step 4: Creating initial v0.6.0 issues..."
echo "   (This will create 6 issues for budget feature tracking)"
echo ""

create_issue() {
    local title=$1
    local body=$2
    local labels=$3

    # Check if issue already exists
    if gh issue list --repo "$REPO" --search "\"$title\"" --json title -q '.[].title' | grep -q "^$title$"; then
        echo "⏭️  Issue already exists: $title"
        return 0
    fi

    if gh issue create --repo "$REPO" \
        --title "$title" \
        --body "$body" \
        --label "$labels" \
        --milestone "v0.6.0"; then
        echo "✅ Created issue: $title"
    else
        echo "⚠️  Failed to create issue: $title"
        return 1
    fi
}

# Issue 1: Core BudgetTracker
create_issue \
    "Implement core BudgetTracker" \
    "Create pkg/budget/tracker.go with core budget tracking functionality including:
- ProjectBudgetData storage
- CostDataPoint history tracking
- Integration with upload tracking
- Persistence to ~/.cargoship/budget_data.json
- Thread-safe operations with proper locking

References:
- Design doc: docs/BUDGET_INTEGRATION_DESIGN.md
- Prism compatibility: Use compatible data structures

Implementation checklist:
- [ ] Define Tracker struct and methods
- [ ] Implement storage and retrieval
- [ ] Add cost tracking for uploads
- [ ] Write unit tests (>80% coverage)
- [ ] Document API" \
    "type: feature,area: budget,priority: high,effort: large"

# Issue 2: Grant and Project Types
create_issue \
    "Define Grant and Project types" \
    "Create pkg/budget/types.go with Grant, Project, and supporting types:

Types to implement:
- Grant (multi-year research grant tracking)
- Project (sub-allocation within a grant)
- BudgetAlert (threshold-based alerts)
- BudgetAutoAction (automated responses)
- StorageCostPoint (S3 cost tracking)
- BudgetData (persistent storage format)

Requirements:
- Make compatible with Prism budget system
- Include JSON tags for serialization
- Add validation methods
- Document all fields

Reference: docs/BUDGET_INTEGRATION_DESIGN.md (lines 126-244)" \
    "type: feature,area: budget,priority: high,effort: medium"

# Issue 3: Cost Estimator
create_issue \
    "Implement cost estimator for S3 operations" \
    "Create pkg/budget/estimator.go to estimate upload costs before execution.

Features:
- EstimateUpload(size, storageClass) - estimate single upload cost
- EstimateStorage(size, storageClass, months) - estimate long-term storage cost
- Compare(size, classes[]) - compare costs across storage classes
- AWS pricing data caching

Implementation:
- Use AWS pricing API or static pricing data
- Cache pricing data locally (~/.cargoship/cost_cache/)
- Handle different regions and storage classes
- Include PUT/POST request costs
- Add data transfer costs

Testing:
- Unit tests with mock pricing data
- Integration test with real AWS pricing (optional)
- Cost estimation accuracy validation" \
    "type: feature,area: budget,priority: high,effort: medium"

# Issue 4: Budget CLI Commands
create_issue \
    "Implement budget CLI commands" \
    "Create budget subcommands matching Prism patterns:

Commands to implement:
1. cargoship budget init - Initialize grant/project
2. cargoship budget status - Current spending status
3. cargoship budget list - List all grants/projects
4. cargoship budget breakdown - Detailed cost breakdown
5. cargoship budget history - Spending history over time

Integration:
- Integrate with upload command to track costs automatically
- Add --project flag to upload command
- Update estimate command to include budget impact

CLI framework:
- Use Cobra commands (cmd/cargoship/cmd/budget.go)
- Consistent output formatting
- Support --json flag for programmatic use
- Add progress indicators for long operations

Documentation:
- Add command help text
- Create examples in docs/BUDGET.md" \
    "type: feature,area: budget,area: cli,priority: high,effort: large"

# Issue 5: Alert System
create_issue \
    "Implement budget alert system" \
    "Create pkg/budget/alerts.go to trigger alerts when budget thresholds are reached.

Features:
- Threshold management (%, absolute amounts)
- Multiple alert channels (email, webhook, stdout)
- Alert history and deduplication
- Configurable alert cooldown periods
- Alert severity levels (warning, critical)

Notification channels:
- Email via SMTP
- Webhook (POST to configurable URL)
- CLI output (for interactive use)
- Log file

Configuration:
- Define alerts in ~/.cargoship.yaml
- Support per-grant and per-project alerts
- Allow disabling specific alert types

Implementation:
- AlertManager struct
- ThresholdChecker
- NotificationDispatcher
- AlertHistory tracking

Testing:
- Unit tests for threshold detection
- Integration tests for notifications
- Mock notification channels" \
    "type: feature,area: budget,priority: medium,effort: medium"

# Issue 6: Documentation
create_issue \
    "Write budget system documentation" \
    "Create comprehensive budget tracking documentation:

Documentation to create/update:
1. docs/BUDGET.md - Main budget guide
   - Getting started
   - Configuration reference
   - CLI command reference
   - Integration with uploads
   - Alert configuration
   - Prism integration notes

2. Update docs/getting-started.md
   - Add budget setup section
   - Link to budget guide

3. Update README.md
   - Mention budget tracking feature
   - Link to documentation

Content to include:
- Configuration examples for common scenarios
- Academic researcher use cases (grant tracking)
- Multi-project cost allocation
- Alert setup examples
- Troubleshooting common issues
- FAQ section

Persona-specific examples:
- Academic researcher with NIH grant
- Lab manager tracking multiple projects
- PI reviewing grant spending

Code examples:
- YAML configuration snippets
- CLI usage examples
- Integration patterns" \
    "type: documentation,area: budget,area: docs,priority: medium,effort: small"

echo ""
echo "✅ All steps completed successfully!"
echo ""
echo "📊 Summary:"
echo "   - Labels synced"
echo "   - 4 milestones created"
echo "   - 6 initial issues created for v0.6.0"
echo ""
echo "🔗 Next steps:"
echo "   1. Create GitHub Project board (requires UI):"
echo "      https://github.com/users/scttfrdmn/projects"
echo "   2. Review and refine the issues:"
echo "      https://github.com/$REPO/issues?q=milestone:v0.6.0"
echo "   3. Start working on budget features!"
echo ""
echo "📖 Reference:"
echo "   - Setup guide: docs/GITHUB_ORGANIZATION_SETUP.md"
echo "   - Budget design: docs/BUDGET_INTEGRATION_DESIGN.md"
echo ""
