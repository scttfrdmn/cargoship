# GitHub Organization Setup Guide

This document describes the GitHub organization structure for CargoShip, including Projects, milestones, labels, and workflows.

---

## Table of Contents

- [Labels](#labels)
- [Issue Templates](#issue-templates)
- [Pull Request Template](#pull-request-template)
- [GitHub Projects](#github-projects)
- [Milestones](#milestones)
- [Workflows](#workflows)
- [Setup Instructions](#setup-instructions)

---

## Labels

CargoShip uses a comprehensive label system to categorize and track issues. Labels are defined in `.github/labels.yml`.

### Label Categories

#### Type Labels
- `type: bug` - Something isn't working correctly
- `type: feature` - New feature or capability
- `type: enhancement` - Improvement to existing feature
- `type: documentation` - Documentation improvements
- `type: refactor` - Code refactoring
- `type: test` - Test improvements
- `type: chore` - Maintenance tasks
- `type: security` - Security-related issues

#### Priority Labels
- `priority: critical` - Blocking issue, must be fixed immediately
- `priority: high` - Important, should be addressed soon
- `priority: medium` - Normal priority
- `priority: low` - Nice to have, not urgent

#### Status Labels
- `status: needs-triage` - New issue that needs review
- `status: needs-info` - Waiting for more information
- `status: in-progress` - Currently being worked on
- `status: blocked` - Blocked by external dependency
- `status: ready` - Ready to be worked on
- `status: wontfix` - Will not be addressed

#### Area Labels
- `area: multiregion` - Multi-region coordination
- `area: staging` - Staging optimization
- `area: budget` - Budget tracking
- `area: cli` - Command-line interface
- `area: s3` - S3 operations
- `area: compression` - Compression algorithms
- `area: testing` - Test infrastructure
- `area: performance` - Performance optimization
- `area: docs` - Documentation
- `area: ci` - CI/CD pipeline

#### Effort Labels
- `effort: small` - < 4 hours
- `effort: medium` - 1-2 days
- `effort: large` - > 2 days

#### Persona Labels
- `persona: academic-researcher`
- `persona: lab-manager`
- `persona: graduate-student`
- `persona: pi`
- `persona: research-admin`

#### Release Labels
- `v0.6.0` - Planned for v0.6.0
- `v0.7.0` - Planned for v0.7.0
- `v0.8.0` - Planned for v0.8.0
- `v1.0.0` - Planned for v1.0.0

### Applying Labels

```bash
# Apply labels to repository
gh label create --repo scttfrdmn/cargoship --file .github/labels.yml

# Or manually sync labels using GitHub's UI
```

---

## Issue Templates

CargoShip provides four issue templates:

1. **Bug Report** (`.github/ISSUE_TEMPLATE/bug_report.yml`)
   - Structured form for reporting bugs
   - Includes system information, logs, and reproduction steps

2. **Feature Request** (`.github/ISSUE_TEMPLATE/feature_request.yml`)
   - Form for suggesting new features
   - Includes persona selection and use case details

3. **Documentation Improvement** (`.github/ISSUE_TEMPLATE/documentation.yml`)
   - Form for suggesting documentation improvements
   - Targets specific documentation types

4. **Performance Issue** (`.github/ISSUE_TEMPLATE/performance.yml`)
   - Form for reporting performance problems
   - Includes workload details and metrics

### Template Configuration

The `.github/ISSUE_TEMPLATE/config.yml` file:
- Disables blank issues (forces template selection)
- Links to GitHub Discussions
- Links to documentation and troubleshooting guides

---

## Pull Request Template

The pull request template (`.github/pull_request_template.md`) includes:

- Description and context
- Type of change selection
- Testing checklist
- Documentation updates
- Code quality checklist
- Security considerations
- Reviewer guidance

---

## GitHub Projects

CargoShip uses GitHub Projects (beta) for tracking work across releases.

### Main Project Board: CargoShip Development

**URL:** `https://github.com/users/scttfrdmn/projects/[PROJECT_ID]`

#### Board Views

1. **Kanban Board** (default view)
   - Columns: Backlog, Ready, In Progress, In Review, Done
   - Grouped by milestone
   - Filtered by priority

2. **Roadmap View**
   - Timeline view showing releases
   - Grouped by milestone
   - Shows dependencies between issues

3. **Priority View**
   - Sorted by priority labels
   - Filtered to show only open issues
   - Critical and high priority items at top

4. **Persona View**
   - Grouped by persona labels
   - Shows user-centric feature organization
   - Helps prioritize by user impact

#### Custom Fields

- **Status** (select): Backlog, Ready, In Progress, In Review, Done
- **Priority** (select): Critical, High, Medium, Low
- **Effort** (select): Small, Medium, Large
- **Target Release** (select): v0.6.0, v0.7.0, v0.8.0, v1.0.0, Backlog
- **Area** (select): Multiregion, Staging, Budget, CLI, S3, etc.
- **Progress** (number): 0-100% complete

#### Automation Rules

1. **Auto-add to project**
   - When: Issue or PR is created
   - Then: Add to project in "Backlog" status

2. **Status transitions**
   - When: PR is opened → Set status to "In Review"
   - When: PR is merged → Set status to "Done"
   - When: Issue is closed → Set status to "Done"

3. **Milestone sync**
   - When: Milestone is assigned → Set "Target Release" field
   - When: Milestone is removed → Set "Target Release" to "Backlog"

### Setting Up the Project Board

```bash
# Create project (via GitHub UI or gh CLI)
gh project create --owner scttfrdmn --title "CargoShip Development"

# Add custom fields
# (Currently requires GitHub UI - no CLI support yet)

# Link repository
gh project link scttfrdmn/cargoship --project [PROJECT_ID]
```

**Manual Steps (GitHub UI):**

1. Go to: https://github.com/users/scttfrdmn/projects
2. Click "New project"
3. Select "Board" template
4. Name: "CargoShip Development"
5. Add custom fields:
   - Priority (Single select: Critical, High, Medium, Low)
   - Effort (Single select: Small, Medium, Large)
   - Target Release (Single select: v0.6.0, v0.7.0, v0.8.0, v1.0.0, Backlog)
   - Area (Single select: All area labels)
   - Progress (Number, 0-100)

6. Configure views:
   - **Kanban**: Group by Status, Filter by milestone
   - **Roadmap**: Layout by Target Release, Timeline view
   - **Priority**: Sort by Priority (descending)
   - **Persona**: Group by Persona labels

7. Set up workflows:
   - Settings → Workflows → Enable automations

---

## Milestones

CargoShip uses milestones to track releases and major feature sets.

### Milestone: v0.6.0 - Budget & Cost Management

**Due Date:** 4 weeks from now
**Description:**

Implement comprehensive budget tracking and cost management for academic researchers managing grant funds.

**Goals:**
- Budget tracker with Prism-compatible structures
- Grant and project management
- Cost estimation and tracking
- Budget CLI commands
- Alert system for budget thresholds

**Features:**
- [ ] Core BudgetTracker implementation
- [ ] Grant and Project types
- [ ] Cost estimation for uploads
- [ ] Budget CLI commands (init, status, breakdown, etc.)
- [ ] Alert system with thresholds
- [ ] Integration with upload command

**Success Criteria:**
- All budget features implemented and tested
- CLI commands functional
- Documentation complete
- 5 persona walkthroughs written

### Milestone: v0.7.0 - Data Lifecycle Management

**Due Date:** 8 weeks from now
**Description:**

Automated data tiering, archival policies, and lifecycle management for long-term storage optimization.

**Goals:**
- Automated tiering to Glacier/Deep Archive
- Data retention policies
- Compliance and governance features
- Cost optimization through tiering

**Features:**
- [ ] Automated tiering engine
- [ ] Lifecycle policy configuration
- [ ] Compliance reporting
- [ ] Data retention enforcement
- [ ] Cost analysis and optimization suggestions

**Success Criteria:**
- Automatic tiering functional
- Policy engine complete
- Cost savings demonstrated
- Compliance features validated

### Milestone: v0.8.0 - Enterprise Features

**Due Date:** 12 weeks from now
**Description:**

Enterprise-grade features for multi-user environments, advanced security, and team collaboration.

**Goals:**
- Multi-tenancy support
- Advanced security features
- Team collaboration features
- Enterprise monitoring and compliance

**Features:**
- [ ] Multi-tenancy with resource isolation
- [ ] Advanced encryption (KMS, envelope encryption)
- [ ] Role-based access control (RBAC)
- [ ] Team collaboration features
- [ ] Enterprise monitoring dashboard
- [ ] Compliance and audit logging

**Success Criteria:**
- Multi-tenant architecture functional
- Security features audited
- Team features validated
- Enterprise customers engaged

### Milestone: v1.0.0 - Production Ready

**Due Date:** 16 weeks from now
**Description:**

First stable release with API stability guarantees, comprehensive documentation, and production validation.

**Goals:**
- API stability commitment
- Comprehensive documentation
- Production validation
- Enterprise support readiness

**Features:**
- [ ] API stability guarantees
- [ ] Comprehensive test coverage (>80%)
- [ ] Production case studies
- [ ] Performance benchmarks published
- [ ] Enterprise support documentation
- [ ] Migration guides from competitors

**Success Criteria:**
- API frozen and documented
- Production deployments validated
- Performance benchmarks published
- Enterprise customers on v1.0

### Creating Milestones

```bash
# Create milestone via gh CLI
gh milestone create "v0.6.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.6.0 - Budget & Cost Management" \
  --description "Implement comprehensive budget tracking for academic researchers" \
  --due-date "2025-12-02"

gh milestone create "v0.7.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.7.0 - Data Lifecycle Management" \
  --description "Automated data tiering and lifecycle management" \
  --due-date "2025-12-30"

gh milestone create "v0.8.0" \
  --repo scttfrdmn/cargoship \
  --title "v0.8.0 - Enterprise Features" \
  --description "Enterprise-grade multi-tenancy and security" \
  --due-date "2026-01-27"

gh milestone create "v1.0.0" \
  --repo scttfrdmn/cargoship \
  --title "v1.0.0 - Production Ready" \
  --description "First stable release with API stability" \
  --due-date "2026-02-24"
```

---

## Workflows

### Issue Triage Workflow

1. **New Issue Created**
   - Auto-labeled: `status: needs-triage`
   - Auto-added to project board in "Backlog"

2. **Maintainer Reviews**
   - Assigns appropriate labels (type, area, priority, effort)
   - Assigns to milestone (if planned)
   - Moves to "Ready" column if actionable
   - Requests more info if needed (`status: needs-info`)

3. **Developer Picks Up**
   - Assigns issue to themselves
   - Moves to "In Progress"
   - Updates `status: in-progress` label

4. **PR Created**
   - Links to issue with "Fixes #XXX"
   - Auto-moved to "In Review"
   - Requests reviews

5. **PR Merged**
   - Issue auto-closed
   - Moved to "Done"

### Release Workflow

1. **Milestone Creation**
   - Create milestone for upcoming release
   - Define goals and success criteria

2. **Feature Planning**
   - Add issues to milestone
   - Prioritize features
   - Assign effort estimates

3. **Development**
   - Track progress on project board
   - Regular updates on milestone progress

4. **Release Preparation**
   - Update CHANGELOG.md
   - Update version in code
   - Create release notes

5. **Release**
   - Tag release with `git tag v0.X.0`
   - GitHub Actions builds and publishes
   - Close milestone
   - Announce release

---

## Setup Instructions

### 1. Apply Labels

```bash
cd /Users/scttfrdmn/src/cargoship

# Sync labels to GitHub
gh label sync --repo scttfrdmn/cargoship --file .github/labels.yml
```

### 2. Create Milestones

```bash
# Create all milestones
gh milestone create "v0.6.0" --repo scttfrdmn/cargoship --title "v0.6.0 - Budget & Cost Management" --due-date "2025-12-02"
gh milestone create "v0.7.0" --repo scttfrdmn/cargoship --title "v0.7.0 - Data Lifecycle Management" --due-date "2025-12-30"
gh milestone create "v0.8.0" --repo scttfrdmn/cargoship --title "v0.8.0 - Enterprise Features" --due-date "2026-01-27"
gh milestone create "v1.0.0" --repo scttfrdmn/cargoship --title "v1.0.0 - Production Ready" --due-date "2026-02-24"
```

### 3. Create GitHub Project

1. Visit: https://github.com/users/scttfrdmn/projects
2. Click "New project" → "Board"
3. Name: "CargoShip Development"
4. Add custom fields (see GitHub Projects section)
5. Configure views
6. Enable workflows

### 4. Verify Setup

```bash
# List labels
gh label list --repo scttfrdmn/cargoship

# List milestones
gh milestone list --repo scttfrdmn/cargoship

# List projects
gh project list --owner scttfrdmn
```

### 5. Create Initial Issues

Create issues for planned v0.6.0 features:

```bash
# Budget tracker implementation
gh issue create --repo scttfrdmn/cargoship \
  --title "Implement core BudgetTracker" \
  --body "Create pkg/budget/tracker.go with core budget tracking functionality" \
  --label "type: feature,area: budget,priority: high,v0.6.0,effort: large" \
  --milestone "v0.6.0"

# Additional issues for budget CLI, cost estimation, etc.
```

---

## Maintenance

### Regular Tasks

**Weekly:**
- Triage new issues
- Review project board
- Update milestone progress

**Monthly:**
- Review and update labels if needed
- Analyze issue metrics
- Adjust priorities based on feedback

**Per Release:**
- Create new milestone
- Update release labels
- Plan features for milestone
- Create release notes

---

## References

- [GitHub Projects Documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects)
- [GitHub Labels Best Practices](https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/managing-labels)
- [GitHub Milestones](https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/about-milestones)
- [GitHub Issue Templates](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests)

---

**Last Updated:** November 4, 2025
**Maintainer:** CargoShip Team
