# CargoShip: Comprehensive Project Review & Strategic Plan
**Date**: November 2025
**Version**: Post-v0.4.6, Planning v0.5.0+
**Review Type**: Holistic assessment with organizational transformation

---

## Executive Summary

CargoShip is a high-performance S3 file upload optimization tool designed for **academic researchers and labs** who need to efficiently archive large datasets to object storage while managing costs across grant periods (3-5 years). This review proposes a strategic shift to persona-driven development, adopting organizational best practices from the Prism project, and streamlining the release process.

### Current State
- **325 Go files** across **52 packages** with **63 pkg subdirectories**
- **v0.4.6 released** with developer experience improvements
- **100% test pass rate** achieved in v0.5.0 Phase 1
- **Advanced features**: Multi-region, zero-copy I/O, NUMA-aware allocation, adaptive compression
- **Technical debt**: Resolved 19 test failures, comprehensive audit complete

### Strategic Goals
1. **Adopt persona-driven development** - Define 5 academic researcher personas
2. **Implement GitHub Projects/Issues/Milestones** - Professional project management
3. **Streamline releases with goreleaser** - Homebrew, Scoop, RPM, DEB, binaries
4. **Maintain Go Report Card A+** - Quality gates in CI/CD
5. **Prepare for GUI evolution** - Daemon architecture planning

---

## 🎯 Part 1: Academic Researcher Personas

Modeled after Prism's successful persona-driven approach, these personas guide feature prioritization and UX decisions.

### Persona 1: Solo Grad Student (Budget-Conscious Archiver)

**Name**: Maya Rodriguez
**Role**: PhD Candidate in Bioinformatics
**Institution**: Mid-sized public university

**Background**:
- Working on RNA-seq analysis for dissertation
- Personal lab budget: $200/month AWS credits from advisor's grant
- Generates 500GB-2TB of data per experiment (weekly)
- **Primary concern**: Not exceeding budget - must justify every dollar to advisor
- Technical level: Comfortable with command line, basic AWS knowledge
- Works from laptop, sometimes on campus HPC

**Pain Points**:
- Has accidentally left files in expensive storage classes
- Doesn't know which compression works best for her data types
- Needs to provide monthly cost reports to PI
- Anxious about multi-region transfers (might cost more?)
- Current solution: Manually calculates costs in spreadsheet before uploads

**Core Needs**:
1. **Budget enforcement** - Hard stops before exceeding monthly limit
2. **Cost estimation** - "What will this upload cost?" before executing
3. **Intelligent compression** - Automatic selection for genomics data
4. **Simple CLI commands** - `cargoship upload --budget $200/month data/`
5. **Grant period budgeting** - "I have $2,400 over 12 months, track burn rate"

**Success Metrics**:
- Never exceeds monthly budget
- Reduces storage costs by 40% through intelligent compression
- Spends <5 minutes per week on cost management

---

### Persona 2: Lab Data Manager (Multi-Project Coordinator)

**Name**: Dr. Sarah Chen
**Role**: Lab Manager / Data Steward
**Institution**: Large R1 research university

**Background**:
- Manages data for 8 active projects across 15 lab members
- Lab budget: $2,000/month AWS (from multiple grants)
- Handles 5-20TB of uploads per month (microscopy, sequencing, simulations)
- **Primary concern**: Cost allocation across projects and grant tracking
- Technical level: Advanced Linux user, AWS certified
- Responsible for compliance and data retention policies

**Pain Points**:
- Needs to track costs per project/grant for billing
- Different projects have different retention requirements
- Lab members upload data inconsistently (no standards)
- Manual tagging is error-prone
- Needs to generate quarterly reports for PIs

**Core Needs**:
1. **Project-based tagging** - Automatic metadata for grant allocation
2. **Retention policies** - Different lifecycles per project type
3. **Team coordination** - Shared configuration, role-based access
4. **Reporting** - Per-project cost breakdowns, trend analysis
5. **Automation** - Pipeline integration for reproducible workflows

**Success Metrics**:
- Accurate cost allocation to 8+ projects
- Automated compliance with institutional data policies
- 90% reduction in manual data management tasks

---

### Persona 3: Principal Investigator (Grant Budget Owner)

**Name**: Prof. James Wilson
**Role**: PI, Computational Neuroscience Lab
**Institution**: Medical school research center

**Background**:
- Manages 3 NIH grants totaling $1.2M over 5 years
- Lab generates 100TB+ of data (fMRI, EEG, behavioral)
- **Primary concern**: Grant budget oversight and institutional compliance
- Technical level: Delegates to lab manager, reviews high-level reports
- Needs auditable records for grant close-outs

**Pain Points**:
- Grant periods don't align with fiscal years
- Needs to justify storage costs to program officers
- Must demonstrate cost-effective data management
- Institutional policies require data retention for 7 years post-publication
- Current solution: Spreadsheet tracking, manual audit trails

**Core Needs**:
1. **Grant-period budgeting** - 3-5 year budget tracking with milestones
2. **Audit trails** - Complete logs for institutional compliance
3. **Cost justification reports** - For grant renewals and close-outs
4. **Data retention enforcement** - Automatic lifecycle based on publication dates
5. **Dashboard view** - High-level overview without CLI usage

**Success Metrics**:
- Pass institutional audits with zero findings
- Demonstrate 50% cost savings vs. standard S3
- Zero data loss incidents over grant period

---

### Persona 4: HPC/Research Computing Staff

**Name**: Alex Thompson
**Role**: Research Computing Specialist
**Institution**: University central IT / Research Computing Center

**Background**:
- Supports 50+ research groups across disciplines
- Manages institutional HPC cluster and data archival
- **Primary concern**: Reliability, automation, and support burden reduction
- Technical level: Expert systems administrator, infrastructure-as-code
- Needs to minimize support tickets from researchers

**Pain Points**:
- Each lab has different workflows and requirements
- Manual support for data transfers is time-consuming
- Inconsistent data organization makes support difficult
- Needs to demonstrate ROI for institutional AWS investment
- Current solution: Custom scripts per lab, not scalable

**Core Needs**:
1. **Multi-tenancy** - Support multiple labs with isolation
2. **Automation** - Scripted workflows, cron integration
3. **Monitoring** - Centralized dashboards for all labs
4. **Documentation** - Self-service guides to reduce tickets
5. **Reliability** - 99.9% uptime, automatic retries

**Success Metrics**:
- Support 50+ labs with 1 FTE
- 80% reduction in support tickets
- Zero data loss across all supported labs

---

### Persona 5: Institutional Data Manager (Policy & Compliance)

**Name**: Dr. Linda Park
**Role**: Director of Research Data Services
**Institution**: University central administration

**Background**:
- Sets data management policy for entire institution
- Manages institutional AWS agreements and billing
- **Primary concern**: Compliance, cost control, security
- Technical level: Policy expert, understands technical requirements
- Reports to CIO and research administration

**Pain Points**:
- Fragmented data management across departments
- No visibility into research data spending
- Difficult to enforce institutional policies
- Security audit findings require centralized controls
- Current solution: Annual surveys, manual policy enforcement

**Core Needs**:
1. **Policy enforcement** - Centralized controls for compliance
2. **Cost visibility** - Institution-wide spending dashboards
3. **Security controls** - Encryption, access logging, DLP
4. **Standardization** - Common workflows across departments
5. **Reporting** - For executive leadership and auditors

**Success Metrics**:
- 100% compliance with federal data management requirements
- 30% reduction in institutional AWS spending
- Zero security incidents in research data storage

---

## 📊 Part 2: GitHub Organizational Structure

Adopt Prism's proven approach with CargoShip-specific adaptations.

### 2.1 Label System

Create `.github/labels.yml`:

```yaml
# CargoShip Label System
# Apply with: gh label sync --labels .github/labels.yml

# ============================================================================
# Type Labels
# ============================================================================
- name: "bug"
  color: "d73a4a"
  description: "Something isn't working correctly"

- name: "enhancement"
  color: "a2eeef"
  description: "New feature or request"

- name: "documentation"
  color: "0075ca"
  description: "Documentation improvements"

- name: "technical-debt"
  color: "fbca04"
  description: "Code refactoring or improvement"

- name: "performance"
  color: "1d76db"
  description: "Performance optimization"

# ============================================================================
# Priority Labels
# ============================================================================
- name: "priority: critical"
  color: "b60205"
  description: "Data loss risk or blocking issue"

- name: "priority: high"
  color: "d93f0b"
  description: "High priority - affects core mission"

- name: "priority: medium"
  color: "fbca04"
  description: "Important but not urgent"

- name: "priority: low"
  color: "0e8a16"
  description: "Nice to have"

# ============================================================================
# Area Labels
# ============================================================================
- name: "area: cli"
  color: "c5def5"
  description: "Command-line interface"

- name: "area: staging"
  color: "c5def5"
  description: "Staging and optimization system"

- name: "area: multiregion"
  color: "c5def5"
  description: "Multi-region coordination"

- name: "area: compression"
  color: "c5def5"
  description: "Compression and deduplication"

- name: "area: cost-management"
  color: "c5def5"
  description: "Budget tracking and cost optimization"

- name: "area: aws"
  color: "c5def5"
  description: "AWS integration and S3 operations"

- name: "area: config"
  color: "c5def5"
  description: "Configuration and setup"

- name: "area: daemon"
  color: "c5def5"
  description: "Daemon architecture (future)"

# ============================================================================
# Persona Labels
# ============================================================================
- name: "persona: grad-student"
  color: "e99695"
  description: "Benefits solo grad student workflow"

- name: "persona: lab-manager"
  color: "e99695"
  description: "Benefits lab data manager workflow"

- name: "persona: pi"
  color: "e99695"
  description: "Benefits PI/grant owner workflow"

- name: "persona: research-computing"
  color: "e99695"
  description: "Benefits HPC/research computing staff"

- name: "persona: institutional"
  color: "e99695"
  description: "Benefits institutional data management"

# ============================================================================
# Status Labels
# ============================================================================
- name: "triage"
  color: "ededed"
  description: "Needs initial review"

- name: "ready"
  color: "0e8a16"
  description: "Ready to be worked on"

- name: "in-progress"
  color: "fbca04"
  description: "Currently being worked on"

- name: "blocked"
  color: "b60205"
  description: "Blocked by external dependency"

# ============================================================================
# Special Labels
# ============================================================================
- name: "good first issue"
  color: "7057ff"
  description: "Good for newcomers"

- name: "help wanted"
  color: "008672"
  description: "Community contributions welcome"

- name: "breaking-change"
  color: "d73a4a"
  description: "Breaks backward compatibility"

- name: "security"
  color: "ee0701"
  description: "Security-related"

# ============================================================================
# Release/Milestone Labels
# ============================================================================
- name: "milestone: v0.5.0"
  color: "bfd4f2"
  description: "Test quality & performance"

- name: "milestone: v0.6.0"
  color: "bfd4f2"
  description: "Budget management & grant tracking"

- name: "milestone: v0.7.0"
  color: "bfd4f2"
  description: "Team collaboration features"

- name: "milestone: v1.0.0"
  color: "bfd4f2"
  description: "Production-ready stable release"
```

### 2.2 Issue Templates

Create `.github/ISSUE_TEMPLATE/`:

#### feature_request.yml
```yaml
name: ✨ Feature Request
description: Suggest a new feature or enhancement
title: "[Feature]: "
labels: ["enhancement", "triage"]
body:
  - type: markdown
    attributes:
      value: |
        Thanks for suggesting a feature! Consider:
        - Which academic researcher persona does this help?
        - Does it align with our core mission (efficient archival to S3)?

  - type: dropdown
    id: persona
    attributes:
      label: Which Persona Benefits?
      options:
        - Grad Student (Budget-Conscious)
        - Lab Data Manager
        - PI/Grant Owner
        - Research Computing Staff
        - Institutional Data Manager
        - All Personas
        - Not Sure
    validations:
      required: true

  - type: dropdown
    id: component
    attributes:
      label: Component
      options:
        - CLI Commands
        - Cost Management
        - Compression/Staging
        - Multi-region
        - Configuration
        - Documentation
    validations:
      required: true

  - type: textarea
    id: problem
    attributes:
      label: Problem Statement
      description: What research workflow problem does this solve?
      placeholder: As a [persona], I need to [do something] because [reason]...
    validations:
      required: true

  - type: textarea
    id: solution
    attributes:
      label: Proposed Solution
      placeholder: |
        I'd like to be able to...
        Example: `cargoship upload --budget-limit $200/month data/`
    validations:
      required: true

  - type: textarea
    id: workflow
    attributes:
      label: Current Workflow vs. Improved
      placeholder: |
        Current: Manual cost calculation, anxious about budget
        With feature: Automatic enforcement, peace of mind
    validations:
      required: true
```

#### bug_report.yml
```yaml
name: 🐛 Bug Report
description: Report something that isn't working
title: "[Bug]: "
labels: ["bug", "triage"]
body:
  - type: textarea
    id: description
    attributes:
      label: Bug Description
      description: Clear description of the bug
    validations:
      required: true

  - type: textarea
    id: reproduce
    attributes:
      label: Steps to Reproduce
      placeholder: |
        1. Run `cargoship upload ...`
        2. See error...
    validations:
      required: true

  - type: textarea
    id: expected
    attributes:
      label: Expected Behavior
    validations:
      required: true

  - type: textarea
    id: actual
    attributes:
      label: Actual Behavior
    validations:
      required: true

  - type: input
    id: version
    attributes:
      label: CargoShip Version
      placeholder: v0.4.6
    validations:
      required: true

  - type: dropdown
    id: severity
    attributes:
      label: Severity
      options:
        - Critical - Data loss or security issue
        - High - Blocks core functionality
        - Medium - Workaround exists
        - Low - Minor inconvenience
    validations:
      required: true
```

### 2.3 GitHub Projects Structure

**Project 1: CargoShip Roadmap**
- View: Kanban board
- Columns: Backlog, Next Up, In Progress, Review, Done
- Automation: Auto-move to "Done" when PR merged

**Project 2: Release v0.6.0 - Budget Management**
- View: Table with custom fields
- Fields: Persona, Priority, Effort (T-shirt sizes), Status
- Filter views per persona

**Project 3: Technical Debt Tracker**
- View: List sorted by priority
- Fields: Area, Impact, Effort, Blockers

### 2.4 Milestones

**v0.5.0** - Test Quality & Performance (Current)
- Duration: 3-4 weeks
- Focus: Phase 1 test fixes, Phase 2 performance optimization
- Success: 100% test pass rate, 20% performance improvement

**v0.6.0** - Budget Management & Grant Tracking
- Duration: 4-6 weeks
- Focus: Grant period budgeting, cost allocation, reporting
- Primary personas: Grad Student, Lab Manager, PI
- Success: Budget enforcement, cost forecasting, audit trails

**v0.7.0** - Team Collaboration Features
- Duration: 4-6 weeks
- Focus: Multi-user support, shared configs, project tagging
- Primary personas: Lab Manager, Research Computing Staff
- Success: Multi-project support, cost allocation, team workflows

**v1.0.0** - Production Stable Release
- Duration: 8-10 weeks
- Focus: Stability, documentation, onboarding
- All personas supported
- Success: A+ Go Report Card, comprehensive docs, 500+ users

---

## 🚀 Part 3: goreleaser Configuration

Create `.goreleaser.yaml`:

```yaml
# CargoShip goreleaser configuration
version: 2

project_name: cargoship

before:
  hooks:
    - go mod tidy
    - go generate ./...
    - go test -short ./...

builds:
  - id: cargoship
    binary: cargoship
    main: ./cmd/cargoship
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X github.com/scttfrdmn/cargoship/pkg/version.Version={{.Version}}
      - -X github.com/scttfrdmn/cargoship/pkg/version.BuildDate={{.Date}}
      - -X github.com/scttfrdmn/cargoship/pkg/version.GitCommit={{.Commit}}

archives:
  - id: cargoship
    format: tar.gz
    name_template: >-
      {{ .ProjectName }}_
      {{- .Version }}_
      {{- .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else }}{{ .Arch }}{{ end }}
    format_overrides:
      - goos: windows
        format: zip
    files:
      - README.md
      - CHANGELOG.md
      - LICENSE
      - docs/USER_GUIDE.md
      - docs/TROUBLESHOOTING.md
      - docs/DEVELOPER_TOOLS.md

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  use: github
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'
      - typo
  groups:
    - title: 'New Features'
      regexp: "^.*feat[(\\w)]*:+.*$"
      order: 0
    - title: 'Bug Fixes'
      regexp: "^.*fix[(\\w)]*:+.*$"
      order: 1
    - title: 'Performance Improvements'
      regexp: "^.*perf[(\\w)]*:+.*$"
      order: 2
    - title: 'Documentation'
      regexp: "^.*docs[(\\w)]*:+.*$"
      order: 3
    - title: 'Other'
      order: 999

# Homebrew tap
brews:
  - name: cargoship
    repository:
      owner: scttfrdmn
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    commit_author:
      name: goreleaserbot
      email: bot@goreleaser.com
    homepage: "https://github.com/scttfrdmn/cargoship"
    description: "High-performance S3 archival tool for academic research data"
    license: "Apache-2.0"
    test: |
      system "#{bin}/cargoship --version"
    install: |
      bin.install "cargoship"

# Scoop bucket (Windows)
scoop:
  bucket:
    owner: scttfrdmn
    name: scoop-bucket
    token: "{{ .Env.SCOOP_BUCKET_GITHUB_TOKEN }}"
  commit_author:
    name: goreleaserbot
    email: bot@goreleaser.com
  homepage: "https://github.com/scttfrdmn/cargoship"
  description: "High-performance S3 archival tool for academic research data"
  license: "Apache-2.0"

# Linux packages
nfpms:
  - id: cargoship
    package_name: cargoship
    homepage: "https://github.com/scttfrdmn/cargoship"
    maintainer: "Scott Freedman <scott@example.com>"
    description: "High-performance S3 archival tool for academic research data"
    license: "Apache-2.0"
    formats:
      - deb
      - rpm
    bindir: /usr/bin
    contents:
      - src: ./docs/USER_GUIDE.md
        dst: /usr/share/doc/cargoship/USER_GUIDE.md
      - src: ./LICENSE
        dst: /usr/share/doc/cargoship/LICENSE

# Docker images (optional for future)
dockers:
  - image_templates:
      - "ghcr.io/scttfrdmn/cargoship:{{ .Tag }}"
      - "ghcr.io/scttfrdmn/cargoship:v{{ .Major }}"
      - "ghcr.io/scttfrdmn/cargoship:latest"
    dockerfile: Dockerfile
    build_flag_templates:
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"

# GitHub Release
release:
  github:
    owner: scttfrdmn
    name: cargoship
  draft: false
  prerelease: auto
  name_template: "Release {{.Version}}"
  header: |
    ## CargoShip {{.Version}}

    High-performance S3 archival for academic research data.
  footer: |
    ## Installation

    ### Homebrew (macOS/Linux)
    ```bash
    brew install scttfrdmn/tap/cargoship
    ```

    ### Scoop (Windows)
    ```powershell
    scoop bucket add scttfrdmn https://github.com/scttfrdmn/scoop-bucket
    scoop install cargoship
    ```

    ### Debian/Ubuntu
    ```bash
    wget https://github.com/scttfrdmn/cargoship/releases/download/{{.Version}}/cargoship_{{.Version}}_linux_x86_64.deb
    sudo dpkg -i cargoship_{{.Version}}_linux_x86_64.deb
    ```

    ### Binary
    Download from [releases page](https://github.com/scttfrdmn/cargoship/releases)

# Announce (optional - future)
announce:
  skip: true
```

### Supporting Files Needed

#### pkg/version/version.go
```go
package version

var (
    Version   = "dev"
    BuildDate = "unknown"
    GitCommit = "unknown"
)

func GetVersion() string {
    return Version
}

func GetBuildInfo() map[string]string {
    return map[string]string{
        "version":   Version,
        "buildDate": BuildDate,
        "gitCommit": GitCommit,
    }
}
```

#### .github/workflows/release.yml
```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write
  packages: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v5
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
          SCOOP_BUCKET_GITHUB_TOKEN: ${{ secrets.SCOOP_BUCKET_GITHUB_TOKEN }}
```

---

## 🔧 Part 4: Refactoring Opportunities

### 4.1 Code Complexity Analysis

#### Current Package Structure (63 subdirectories in pkg/)
**Assessment**: Good separation of concerns, but some packages may be over-engineered.

**High-Complexity Areas**:
1. **pkg/staging/** - Multiple complex optimization systems
   - SimpleAdvancedStagingOptimizer
   - ChunkDeduplicator
   - AdaptiveCompressionSelector
   - PerformanceMonitor
   - PredictivePrefetcher

   **Question**: Are all these optimizations used by academic researchers? Or premature optimization?

2. **pkg/s3optimization/** - Predictive prefetching system
   - AccessPatternAnalyzer
   - RequestPredictor
   - PrefetchCache

   **Question**: Do researchers need this complexity? Or simple upload is enough?

3. **pkg/multiregion/** - Multi-region coordination
   - DefaultCoordinator
   - LoadBalancer (8 strategies)
   - FailoverManager

   **Question**: Is multi-region a core use case for labs? Or nice-to-have?

### 4.2 Refactoring Priorities

#### Priority 1: Simplify for Core Use Case (Week 1-2)

**Focus**: What do grad students and lab managers actually need?

1. **Audit actual usage** vs. implemented features
   - Profile: Which packages are imported by cmd/cargoship?
   - Measure: What % of code is actually executed in typical workflows?
   - Decision: Keep core, deprecate unused complexity

2. **Create "Essential" vs. "Advanced" feature tiers**
   - Essential: Upload, compression, cost estimation
   - Advanced: Multi-region, predictive optimization, NUMA
   - Action: Make advanced features opt-in

3. **Simplify staging system**
   - Current: 5+ optimization layers
   - Proposed: 1-2 simple, predictable optimization modes
   - Benefit: Easier to debug, maintain, explain

#### Priority 2: Documentation Restructuring (Week 3)

**Problem**: 625-line CLAUDE.md is developer-focused, not user-focused

**Proposed Structure**:
```
docs/
├── USER_SCENARIOS/          # NEW - Prism-style walkthroughs
│   ├── 01_GRAD_STUDENT_WALKTHROUGH.md
│   ├── 02_LAB_MANAGER_WALKTHROUGH.md
│   ├── 03_PI_WALKTHROUGH.md
│   ├── 04_RESEARCH_COMPUTING_WALKTHROUGH.md
│   └── 05_INSTITUTIONAL_WALKTHROUGH.md
├── user-guides/             # NEW - End-user documentation
│   ├── GETTING_STARTED.md
│   ├── BUDGET_MANAGEMENT.md
│   ├── COMPRESSION_GUIDE.md
│   └── TROUBLESHOOTING.md   # Already exists
├── admin-guides/            # NEW - IT/admin docs
│   ├── INSTALLATION.md
│   ├── AWS_SETUP.md
│   ├── MULTI_LAB_DEPLOYMENT.md
│   └── COST_ALLOCATION.md
├── development/             # Developer docs
│   ├── ARCHITECTURE.md
│   ├── CONTRIBUTING.md
│   ├── CLAUDE.md           # Keep for dev context
│   └── DEVELOPER_TOOLS.md  # Already exists
└── api/                     # API reference
    └── api-stability.md    # Already exists
```

**Action Items**:
1. Split CLAUDE.md into user-facing and dev sections
2. Create persona walkthroughs (5 files, ~300 lines each)
3. Write GETTING_STARTED.md for new users
4. Write BUDGET_MANAGEMENT.md for grant tracking

#### Priority 3: Configuration Simplification (Week 4)

**Current**: Complex YAML configuration with many options

**Proposed**: Tiered configuration system
```yaml
# Simple mode (grad student)
mode: simple
budget:
  monthly_limit: 200
  currency: USD
compression: auto

# Advanced mode (lab manager)
mode: advanced
budget:
  grant_periods:
    - name: "NIH R01 2023-2028"
      total: 12000
      start: "2023-09-01"
      end: "2028-08-31"
  projects:
    - name: "RNA-seq"
      grant: "NIH R01 2023-2028"
      allocation: 40%
compression:
  genomics: zstd
  imaging: brotli
multiregion:
  enabled: true
  regions: [us-west-2, us-east-1]
```

### 4.3 Technical Debt Resolution

From TEST_FAILURES_AUDIT.md, technical debt is well-managed. Continue:

1. **Maintain test quality**
   - Keep 100% pass rate
   - Add integration tests for new features
   - Benchmark all performance claims

2. **Monitor code complexity**
   - Run `gocyclo` on new PRs
   - Set complexity threshold: 15
   - Refactor functions above threshold

3. **Dependency hygiene**
   - Regular `go mod tidy`
   - Security scanning (govulncheck)
   - Update dependencies quarterly

---

## 📋 Part 5: Feature Prioritization Framework

### Prioritization Matrix

**Criteria** (weighted):
1. **Persona Impact** (40%) - How many personas benefit?
2. **Core Mission Alignment** (30%) - Does it improve archival efficiency or cost management?
3. **Implementation Effort** (20%) - T-shirt size (S/M/L/XL)
4. **User Feedback** (10%) - Requested by real users?

### Feature Categorization

#### Tier 1: Must Have (Core Mission)
- ✅ Upload optimization and compression
- ✅ Cost estimation before upload
- 🔄 Budget enforcement (hard stops)
- 🔄 Grant period tracking (3-5 years)
- 🔄 Cost allocation per project

#### Tier 2: Should Have (Enhances Core)
- 🔄 Interactive setup wizard (v0.4.6 started this)
- 🔄 Compression auto-selection by file type
- 🔄 Retention policy enforcement
- ⏸️ Multi-region (only if users request)
- ⏸️ GUI (future, not urgent)

#### Tier 3: Nice to Have (Advanced)
- ⏸️ Predictive prefetching
- ⏸️ NUMA-aware allocation
- ⏸️ Zero-copy I/O optimizations
- ⏸️ Advanced staging optimization

**Recommendation**: Pause Tier 3 work until Tier 1-2 are complete and validated with users.

---

## 🗺️ Part 6: 6-Month Roadmap

### Phase 1: Foundation & Organization (Weeks 1-2)
**Goal**: Establish persona-driven development and release infrastructure

**Tasks**:
- [ ] Create 5 persona walkthrough documents
- [ ] Implement GitHub labels/issues/projects structure
- [ ] Set up goreleaser and test release process
- [ ] Create homebrew tap and scoop bucket repositories
- [ ] Write GETTING_STARTED.md for new users

**Deliverables**:
- `docs/USER_SCENARIOS/*.md` (5 files)
- `.github/labels.yml`, issue templates
- `.goreleaser.yaml` configured
- First automated release (v0.5.0)

---

### Phase 2: v0.6.0 - Budget Management (Weeks 3-8)
**Goal**: Address top pain point for grad students and PIs

**Primary Personas**: Grad Student, PI

**Features**:
1. **Budget Enforcement**
   - Command: `cargoship config set-budget --monthly 200 --currency USD`
   - Behavior: Refuse uploads that would exceed limit
   - CLI: Show remaining budget in status

2. **Grant Period Tracking**
   - Command: `cargoship grant add "NIH R01" --total 12000 --start 2023-09-01 --duration 5y`
   - Dashboard: Show burn rate, projected end-of-grant balance
   - Alerts: Warning at 80%, 90%, 95% of budget

3. **Cost Estimation**
   - Command: `cargoship estimate /path/to/data`
   - Output: Upload cost, monthly storage cost, 5-year total
   - Options: Compare storage classes (Standard vs. Glacier)

4. **Reporting**
   - Command: `cargoship report --grant "NIH R01" --format pdf`
   - Output: Monthly/quarterly cost reports for PI
   - Sections: Upload costs, storage costs, cost trends

**Success Metrics**:
- Zero budget overruns for early adopter labs
- 90% reduction in "budget anxiety" (user survey)
- Adoption by 10+ research groups

---

### Phase 3: v0.7.0 - Team Collaboration (Weeks 9-14)
**Goal**: Support lab data managers coordinating multiple projects

**Primary Personas**: Lab Manager, Research Computing Staff

**Features**:
1. **Project Tagging**
   - Command: `cargoship upload --project rnaseq-2024 --grant "NIH R01" data/`
   - Automatic: Tag all uploads with metadata
   - Reporting: Per-project cost breakdowns

2. **Shared Configuration**
   - Location: Lab-wide config at `/lab/shared/.cargoship.yaml`
   - Personal: User overrides in `~/.cargoship.yaml`
   - Inheritance: Personal inherits from shared

3. **Cost Allocation**
   - Command: `cargoship allocate --project rnaseq-2024 --percent 40`
   - Dashboard: Show allocation vs. actual for each project
   - Alerts: Project over-budget warnings

4. **Automation Support**
   - Script-friendly: JSON output for all commands
   - Pipeline: Integration guides for Nextflow, Snakemake
   - Cron: Scheduled uploads with budget checks

**Success Metrics**:
- 5+ labs using multi-project features
- Accurate cost allocation (within 5% error)
- 50% reduction in manual cost tracking time

---

### Phase 4: v1.0.0 - Production Stable (Weeks 15-24)
**Goal**: Achieve production-ready stability for broad adoption

**All Personas**

**Features**:
1. **Comprehensive Documentation**
   - 5 persona walkthroughs complete with screenshots
   - Video tutorials (5-10 min each)
   - FAQ covering 50+ common questions

2. **Onboarding Experience**
   - Interactive `cargoship init` wizard
   - Sample workflows for each persona
   - Configuration validator with helpful errors

3. **Quality Gates**
   - A+ on Go Report Card
   - 90%+ test coverage
   - Zero critical bugs open
   - Security audit complete

4. **Community**
   - GitHub Discussions enabled
   - User showcase (5+ case studies)
   - Monthly office hours for Q&A

**Success Metrics**:
- 500+ active installations
- <2% bug report rate
- 90% user satisfaction (NPS >50)
- Featured by AWS, HPC blogs, research computing newsletters

---

## 🎬 Part 7: Immediate Action Plan

### Week 1: Quick Wins

**Day 1-2: GitHub Organization**
```bash
# Create labels
gh label sync --labels .github/labels.yml

# Create issue templates
mkdir -p .github/ISSUE_TEMPLATE
# Copy templates from this document

# Create first project
gh project create "CargoShip Roadmap" --owner scttfrdmn

# Create milestones
gh milestone create "v0.6.0 - Budget Management" --due-date 2025-12-15
gh milestone create "v0.7.0 - Team Collaboration" --due-date 2026-02-01
gh milestone create "v1.0.0 - Production Stable" --due-date 2026-04-01
```

**Day 3-5: goreleaser Setup**
```bash
# Create version package
mkdir -p pkg/version
# Copy version.go from this document

# Create .goreleaser.yaml
# Copy config from this document

# Test release (dry-run)
goreleaser release --snapshot --clean

# Create homebrew tap repo
gh repo create homebrew-tap --public

# Create scoop bucket repo
gh repo create scoop-bucket --public
```

### Week 2: Persona Documentation

**Day 1-5: Create Persona Walkthroughs**
```bash
mkdir -p docs/USER_SCENARIOS

# Write 5 persona walkthrough documents
# Each 300-500 lines with:
# - Persona background and pain points
# - Step-by-step workflows
# - Before/after comparisons
# - Success metrics
```

**Deliverable**: 5 markdown files ready for user feedback

### Week 3: User-Facing Documentation

**Day 1-3: Getting Started Guide**
- Installation (all platforms)
- AWS setup
- First upload
- Cost estimation
- Budget configuration

**Day 4-5: Budget Management Guide**
- Setting monthly budgets
- Grant period tracking
- Cost reports
- Troubleshooting budget issues

---

## 🤔 Critical Questions for Decision

Before proceeding, please confirm:

### 1. Feature Scope
**Question**: Should we pause/deprecate advanced features (predictive prefetching, NUMA, multi-region) to focus on core budget management?

**Options**:
- A) Keep all features, document as "advanced/experimental"
- B) Deprecate unused features in v0.6.0
- C) Make advanced features opt-in via config flag

**Recommendation**: Option C - opt-in for complexity

### 2. Documentation Priority
**Question**: Should persona walkthroughs be written before or after implementing v0.6.0 features?

**Options**:
- A) Write walkthroughs now (guides feature design)
- B) Write walkthroughs after implementation (documents reality)

**Recommendation**: Option A - personas guide features

### 3. Release Cadence
**Question**: How often should we release?

**Options**:
- A) Weekly (rapid iteration, more overhead)
- B) Every 2-4 weeks (balanced)
- C) Monthly (stable, slower feedback)

**Recommendation**: Option B for now, move to A for v1.0+

### 4. Community Building
**Question**: Should we recruit beta testers from academic labs now?

**Options**:
- A) Yes, recruit 5-10 labs immediately for v0.6.0 beta
- B) Wait until v0.7.0 for broader testing
- C) Launch v1.0.0 first, then market

**Recommendation**: Option A - early feedback is critical

---

## 📊 Appendix A: Go Report Card Checklist

To maintain A+:

- [ ] `go fmt` on all files
- [ ] `go vet` passes
- [ ] `golint` passes with zero warnings
- [ ] `gocyclo` under 15 for all functions
- [ ] `gofmt -s` simplifications applied
- [ ] `ineffassign` zero ineffectual assignments
- [ ] `misspell` zero spelling errors
- [ ] Test coverage >80%
- [ ] `go doc` for all exported symbols
- [ ] `examples/` directory with runnable examples

**CI Integration**:
```yaml
# .github/workflows/quality.yml
name: Quality Checks
on: [push, pull_request]
jobs:
  quality:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: go fmt
        run: test -z "$(gofmt -l .)"

      - name: go vet
        run: go vet ./...

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v3
        with:
          version: latest

      - name: Test coverage
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | awk '{if ($1 < 80) exit 1}'
```

---

## 📚 Appendix B: Reference Material

### Prism Organizational Assets to Replicate
1. ✅ `.github/labels.yml` - Comprehensive label system
2. ✅ `.github/ISSUE_TEMPLATE/*.yml` - Structured issue templates
3. ✅ `docs/USER_SCENARIOS/*.md` - Persona walkthroughs
4. ✅ `.goreleaser.yaml` - Automated releases
5. ✅ `Makefile` with quality checks
6. ⏸️ GUI architecture (future)

### CargoShip Unique Strengths to Preserve
1. ✅ Advanced performance optimizations
2. ✅ Comprehensive testing infrastructure
3. ✅ Multi-region capabilities
4. ✅ Zero-copy I/O innovations
5. ✅ Detailed CLAUDE.md development history

---

## 🎯 Success Metrics Dashboard

Track these metrics monthly:

### User Adoption
- [ ] Active installations: ___ / 500 (v1.0 goal)
- [ ] GitHub stars: ___ / 100
- [ ] Issues opened: ___ / resolved ___
- [ ] Community contributions: ___ PRs merged

### Quality
- [ ] Go Report Card: ___ / A+
- [ ] Test coverage: ___% / 80%
- [ ] Open bugs: ___ / <10 target
- [ ] Security vulnerabilities: ___ / 0

### Persona Satisfaction
- [ ] Grad Student NPS: ___ / 50+
- [ ] Lab Manager NPS: ___ / 50+
- [ ] PI NPS: ___ / 50+

### Financial Impact (for users)
- [ ] Average cost reduction: ___% / 40%
- [ ] Budget overruns prevented: ___ / 0
- [ ] Labs tracking grants: ___ / 20+

---

## 🚀 Let's Get Started!

This review provides:
1. ✅ 5 academic researcher personas
2. ✅ Complete GitHub organizational structure
3. ✅ goreleaser configuration for automated releases
4. ✅ Refactoring priorities and technical debt plan
5. ✅ 6-month roadmap with clear milestones
6. ✅ Immediate action plan for Week 1

**Next Steps**:
1. Review and approve personas
2. Implement GitHub labels and issue templates
3. Set up goreleaser and test first automated release
4. Begin v0.6.0 budget management feature development

**Questions?** Let's discuss any section that needs clarification or adjustment.
