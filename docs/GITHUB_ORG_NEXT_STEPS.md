# GitHub Organization - Next Steps

**Status:** Infrastructure files committed, ready to apply to GitHub
**Commit:** 289cfa6 - "feat: Add GitHub organization infrastructure"

---

## ✅ Completed

### 1. Labels Configuration
- Created `.github/labels.yml` with 60+ labels
- Categories: type, priority, status, area, effort, persona, release
- Color-coded and well-documented

### 2. Issue Templates
- Bug report template (`.github/ISSUE_TEMPLATE/bug_report.yml`)
- Feature request template (`.github/ISSUE_TEMPLATE/feature_request.yml`)
- Documentation improvement template (`.github/ISSUE_TEMPLATE/documentation.yml`)
- Performance issue template (`.github/ISSUE_TEMPLATE/performance.yml`)
- Template chooser config (`.github/ISSUE_TEMPLATE/config.yml`)

### 3. Pull Request Template
- Comprehensive checklist (`.github/pull_request_template.md`)
- Code quality, testing, documentation sections
- Security and performance impact tracking

### 4. Documentation
- Complete setup guide (`docs/GITHUB_ORGANIZATION_SETUP.md`)
- GitHub Projects board configuration
- Milestone definitions for v0.6.0, v0.7.0, v0.8.0, v1.0.0
- Workflow documentation

---

## 📋 Next Steps - Apply to GitHub

### Step 1: Push Changes to GitHub

```bash
cd /Users/scttfrdmn/src/cargoship
git push origin main
```

### Step 2: Sync Labels

```bash
# Sync all labels defined in labels.yml to GitHub
gh label sync --repo scttfrdmn/cargoship --file .github/labels.yml
```

**Expected Output:**
```
✓ Synced 60+ labels
✓ Created new labels
✓ Updated existing labels
✓ No labels removed (default: keep existing)
```

### Step 3: Verify Issue Templates

1. Visit: https://github.com/scttfrdmn/cargoship/issues/new/choose
2. Verify all 4 templates appear
3. Test one template to ensure form works

### Step 4: Verify PR Template

1. Create a test branch and dummy PR (or wait for next real PR)
2. Verify template auto-loads in PR description
3. Test checklist functionality

### Step 5: Create Milestones

```bash
# Create v0.6.0 milestone
gh milestone create "v0.6.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.6.0 - Budget & Cost Management" \
  --description "Implement comprehensive budget tracking and cost management for academic researchers managing grant funds. Core BudgetTracker, Grant/Project types, cost estimation, budget CLI commands, alert system." \
  --due-date "2025-12-02"

# Create v0.7.0 milestone
gh milestone create "v0.7.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.7.0 - Data Lifecycle Management" \
  --description "Automated data tiering, archival policies, and lifecycle management for long-term storage optimization. Automated tiering to Glacier/Deep Archive, retention policies, compliance features." \
  --due-date "2025-12-30"

# Create v0.8.0 milestone
gh milestone create "v0.8.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.8.0 - Enterprise Features" \
  --description "Enterprise-grade features for multi-user environments. Multi-tenancy, advanced security (KMS, RBAC), team collaboration, enterprise monitoring and compliance." \
  --due-date "2026-01-27"

# Create v1.0.0 milestone
gh milestone create "v1.0.0" \
  --repo scttfrdmn/cargoship \
  --title "v1.0.0 - Production Ready" \
  --description "First stable release with API stability guarantees, comprehensive documentation, and production validation. API stability commitment, >80% test coverage, production case studies, performance benchmarks." \
  --due-date "2026-02-24"
```

**Verify:**
```bash
gh milestone list --repo scttfrdmn/cargoship
```

### Step 6: Create GitHub Project Board

**Manual Steps (GitHub UI required):**

1. Visit: https://github.com/users/scttfrdmn/projects
2. Click "New project"
3. Select "Board" template
4. Name: "CargoShip Development"
5. Description: "Main project board for tracking CargoShip development across releases"

**Add Custom Fields:**
- Priority (Single select: Critical, High, Medium, Low)
- Effort (Single select: Small, Medium, Large)
- Target Release (Single select: v0.6.0, v0.7.0, v0.8.0, v1.0.0, Backlog)
- Area (Single select: multiregion, staging, budget, cli, s3, compression, testing, performance, docs, ci)
- Progress (Number: 0-100)

**Create Views:**
1. **Kanban** (default)
   - Layout: Board
   - Group by: Status
   - Columns: Backlog, Ready, In Progress, In Review, Done
   - Filter: `is:open`

2. **Roadmap**
   - Layout: Roadmap
   - Group by: Target Release
   - Date field: Milestone due date
   - Filter: `is:open`

3. **Priority**
   - Layout: Table
   - Sort: Priority (descending), then Target Release
   - Filter: `is:open`

4. **Persona**
   - Layout: Board
   - Group by: Persona labels
   - Filter: `is:open label:persona*`

**Configure Workflows:**
1. Settings → Workflows
2. Enable "Auto-add to project"
   - When: Item opened
   - Then: Add to project
   - Set: Status = Backlog

3. Enable "Auto-archive"
   - When: Item closed
   - Then: Archive item

4. Enable "Item reopened"
   - When: Item reopened
   - Then: Set Status = Ready

### Step 7: Create Initial Issues for v0.6.0

```bash
# Budget Tracker Core
gh issue create --repo scttfrdmn/cargoship \
  --title "Implement core BudgetTracker" \
  --body "Create pkg/budget/tracker.go with core budget tracking functionality including ProjectBudgetData storage, CostDataPoint history, and integration with upload tracking." \
  --label "type: feature,area: budget,priority: high,v0.6.0,effort: large" \
  --milestone "v0.6.0"

# Grant and Project Types
gh issue create --repo scttfrdmn/cargoship \
  --title "Define Grant and Project types" \
  --body "Create pkg/budget/types.go with Grant, Project, BudgetAlert, BudgetAutoAction, StorageCostPoint, and BudgetData structures. Make compatible with Prism budget system." \
  --label "type: feature,area: budget,priority: high,v0.6.0,effort: medium" \
  --milestone "v0.6.0"

# Cost Estimator
gh issue create --repo scttfrdmn/cargoship \
  --title "Implement cost estimator for S3 operations" \
  --body "Create pkg/budget/estimator.go to estimate upload costs before execution. Include EstimateUpload, EstimateStorage, and Compare functions with AWS pricing data caching." \
  --label "type: feature,area: budget,priority: high,v0.6.0,effort: medium" \
  --milestone "v0.6.0"

# Budget CLI Commands
gh issue create --repo scttfrdmn/cargoship \
  --title "Implement budget CLI commands" \
  --body "Create budget subcommands: init, status, list, breakdown, history. Match Prism budget command patterns. Integrate with upload command to track costs automatically." \
  --label "type: feature,area: budget,area: cli,priority: high,v0.6.0,effort: large" \
  --milestone "v0.6.0"

# Alert System
gh issue create --repo scttfrdmn/cargoship \
  --title "Implement budget alert system" \
  --body "Create pkg/budget/alerts.go to trigger alerts when budget thresholds are reached. Support email and webhook notifications. Include threshold management and alert history." \
  --label "type: feature,area: budget,priority: medium,v0.6.0,effort: medium" \
  --milestone "v0.6.0"

# Documentation
gh issue create --repo scttfrdmn/cargoship \
  --title "Write budget system documentation" \
  --body "Create docs/BUDGET.md with comprehensive budget tracking guide, configuration examples, CLI reference, and integration with Prism. Include persona-specific use cases." \
  --label "type: documentation,area: budget,area: docs,priority: medium,v0.6.0,effort: small" \
  --milestone "v0.6.0"
```

### Step 8: Verify Everything

**Checklist:**
- [ ] Labels synced (60+ labels visible)
- [ ] Issue templates working (4 templates available)
- [ ] PR template auto-loads
- [ ] All 4 milestones created
- [ ] GitHub Project created and configured
- [ ] Initial v0.6.0 issues created
- [ ] Repository settings updated (if needed)

---

## 📊 Current Status

### Labels
- ✅ Committed: Yes (`.github/labels.yml`)
- ⏳ Applied to GitHub: **Pending Step 2**
- Command: `gh label sync --repo scttfrdmn/cargoship --file .github/labels.yml`

### Issue Templates
- ✅ Committed: Yes (`.github/ISSUE_TEMPLATE/`)
- ✅ Auto-applied: Will work immediately after push

### PR Template
- ✅ Committed: Yes (`.github/pull_request_template.md`)
- ✅ Auto-applied: Will work immediately after push

### Milestones
- ✅ Documented: Yes (`docs/GITHUB_ORGANIZATION_SETUP.md`)
- ⏳ Created on GitHub: **Pending Step 5**
- Commands: Provided in Step 5

### GitHub Projects
- ✅ Documented: Yes (`docs/GITHUB_ORGANIZATION_SETUP.md`)
- ⏳ Created: **Pending Step 6** (requires GitHub UI)

### Initial Issues
- ✅ Documented: Issue creation commands provided
- ⏳ Created: **Pending Step 7**

---

## 🚀 Quick Start Command Sequence

```bash
# Push to GitHub
git push origin main

# Sync labels
gh label sync --repo scttfrdmn/cargoship --file .github/labels.yml

# Create milestones
gh milestone create "v0.6.0" --repo scttfrdmn/cargoship --title "v0.6.0 - Budget & Cost Management" --description "Budget tracking and cost management" --due-date "2025-12-02"
gh milestone create "v0.7.0" --repo scttfrdmn/cargoship --title "v0.7.0 - Data Lifecycle Management" --description "Automated tiering and archival" --due-date "2025-12-30"
gh milestone create "v0.8.0" --repo scttfrdmn/cargoship --title "v0.8.0 - Enterprise Features" --description "Multi-tenancy and security" --due-date "2026-01-27"
gh milestone create "v1.0.0" --repo scttfrdmn/cargoship --title "v1.0.0 - Production Ready" --description "API stability and production validation" --due-date "2026-02-24"

# Verify
gh milestone list --repo scttfrdmn/cargoship
gh label list --repo scttfrdmn/cargoship | wc -l  # Should show 60+

# Then create Project board via GitHub UI
# Then create initial issues
```

---

## 📝 Notes

- **Labels:** Automatically applied after sync, no manual work needed
- **Issue Templates:** Work immediately after push, GitHub auto-detects them
- **PR Template:** Works immediately after push
- **Milestones:** Must be created via CLI or UI, commands provided
- **GitHub Projects:** Currently requires UI (no full CLI support yet)
- **Initial Issues:** Can be bulk-created via CLI commands provided

---

**Last Updated:** November 4, 2025
**Status:** Ready to apply to GitHub
**Next Action:** Run Step 1 (push to GitHub)
