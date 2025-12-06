# Scenario 4: Principal Investigator with Grant Budget Management

## Persona: Prof. James Wilson

**Background**:
- Principal Investigator (Full Professor) at Research University
- Leads lab of 20 researchers across 8 active grants (total: $450K/year in cloud budget)
- Research focus: Multi-omics integration (genomics, proteomics, metabolomics)
- Primary concern: **Staying within grant budgets and providing audit trails for sponsors**
- Technical level: Strategic oversight - delegates technical details to Lab Manager
- Time constraints: Very busy - needs dashboard views and exception alerts only

**Pain Points**:
- Grant sponsors (NIH, NSF, DOE, industry) require detailed cost justifications
- Different grants have different indirect cost rates (F&A: 25%-60%)
- Multi-year grants need budget forecasting (will we have enough for Year 3?)
- Annual renewals require demonstrating efficient resource usage
- Current solution: Finance office provides quarterly AWS reports (too late to correct course!)
- Worry: A runaway upload could blow through an entire year's cloud budget

**Grant Portfolio**:
```
Wilson Lab Grant Portfolio ($450K/year cloud budget)

├── NIH R01-HL-2023 (Cardiovascular Genomics)
│   │   Budget: $180K/year cloud ($15K/month)
│   │   Duration: 5 years (Year 3 of 5)
│   │   F&A: 60% (research university rate)
│   │   Team: 2 postdocs + 4 PhD students
│   │   Data: WGS, RNA-seq, ATAC-seq (50-100 TB/year)
│   │
│   ├── NSF DBI-2024 (Machine Learning for Proteomics)
│   │   Budget: $90K/year cloud ($7.5K/month)
│   │   Duration: 3 years (Year 1 of 3)
│   │   F&A: 25% (NSF rate)
│   │   Team: 1 postdoc + 2 PhD students
│   │   Data: Mass spec, protein structures (20-30 TB/year)
│   │
│   ├── DOE Office of Science (Metabolomics)
│   │   Budget: $75K/year cloud ($6.25K/month)
│   │   Duration: 3 years (Year 2 of 3)
│   │   F&A: 35% (DOE rate)
│   │   Team: 1 senior scientist + 2 students
│   │   Data: Metabolomics, imaging (15-25 TB/year)
│   │
│   ├── Industry Partnership (Pharma)
│   │   Budget: $60K/year cloud ($5K/month)
│   │   Duration: 2 years (Year 1 of 2)
│   │   F&A: 0% (negotiated, no overhead)
│   │   Team: 1 postdoc dedicated
│   │   Data: Drug screening, patient data (30-40 TB/year)
│   │
│   ├── Foundation Grant (Cancer Research)
│   │   Budget: $30K/year cloud ($2.5K/month)
│   │   Duration: 2 years (Year 2 of 2)
│   │   F&A: 15% (foundation rate)
│   │   Team: 1 PhD student
│   │   Data: Single-cell, spatial transcriptomics (10-15 TB/year)
│   │
│   └── Discretionary / Startup
│       Budget: $15K/year cloud ($1.25K/month)
│       Purpose: New projects, pilot studies, training
│       Team: Rotating students, new lab members
```

---

## Version Legend
- ✅ **v0.5.0 (Current)**: Features available today
- 🔄 **v0.6.0+ (Planned)**: Features in development (see linked GitHub issues)

## Current State (v0.5.0): What Works Today

### ✅ Initial Setup: PI Dashboard Configuration (Day 0)

Prof. Wilson works with Lab Manager (Dr. Sarah Chen) to set up PI-level oversight.

```bash
# Lab Manager configures PI dashboard
cargoship config pi-dashboard \
  --pi james-wilson \
  --email james.wilson@university.edu \
  --notification-threshold 80  # Alert at 80% of any budget
  --digest-frequency weekly \
  --include-forecasts \
  --output-format executive-summary

# Output:
# ✅ PI Dashboard configured for Prof. James Wilson
#
# 📊 Dashboard Settings:
#   Email:              james.wilson@university.edu
#   Alert threshold:    80% of budget (any grant)
#   Digest emails:      Every Monday 8:00 AM
#   Include:            Budget forecasts, trend analysis
#   Format:             Executive summary (high-level metrics)
#
# 📧 Notifications:
#   ✅ Budget warnings (>80%)
#   ✅ Large uploads (>$1,000)
#   ✅ Unusual activity (spending spikes)
#   ✅ Grant renewals (60 days before end)
#
# 💡 Prof. Wilson will receive weekly digests and immediate alerts for exceptions
```

**What Prof. Wilson thinks**: *"Good. I don't need to see every upload, just exceptions and high-level trends. Weekly digests will keep me informed without overwhelming my inbox. If something goes wrong, I'll hear about it immediately."*

### ✅ Weekly Digest Email (Typical Week)

Prof. Wilson receives automated weekly summary every Monday morning.

```
From: CargoShip Lab Management <cargoship@university.edu>
To: james.wilson@university.edu
Subject: Wilson Lab Weekly Digest - Week of November 11, 2024
Date: Monday, November 11, 2024 8:00 AM

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Wilson Lab Cloud Storage Summary
Week of November 11, 2024
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Executive Summary

Lab-wide spend: $8,247 this week ($35,891 month-to-date)
Network impact: <2% of 10 Gbps link (no congestion)
Upload success rate: 99.4% (3 failed uploads, auto-recovered)
Status: ✅ All grants within budget, on track for year

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💰 Budget Status by Grant

1️⃣  NIH R01-HL-2023 (Cardiovascular Genomics)
    Monthly budget: $15,000
    Spent (MTD):    $11,247 (75% of monthly budget)
    Remaining:      $3,753 (25%)
    Days left:      19 days in November
    Burn rate:      $937/day (within normal range)
    Forecast:       $13,560 total for November (90% of budget) ✅
    Status:         ON TRACK

    Top users: dr-maria-garcia ($4,892), james-kim ($3,201)
    Largest upload: cardiovascular-cohort-500 (2.1 TB, $187)

2️⃣  NSF DBI-2024 (Machine Learning for Proteomics)
    Monthly budget: $7,500
    Spent (MTD):    $6,892 (92% of monthly budget)
    Remaining:      $608 (8%)
    Days left:      19 days in November
    Burn rate:      $574/day (elevated)
    Forecast:       $8,238 for November (110% of budget) ⚠️
    Status:         APPROACHING LIMIT

    ⚠️  WARNING: Forecasted to exceed monthly budget by $738
    Recommendation: Review with Lab Manager (Dr. Chen)

    Top user: dr-alex-thompson ($6,892 - 100%)
    Largest upload: proteomics-timecourse-full (1.2 TB, $106)

3️⃣  DOE Office of Science (Metabolomics)
    Monthly budget: $6,250
    Spent (MTD):    $2,847 (46% of monthly budget)
    Remaining:      $3,403 (54%)
    Days left:      19 days in November
    Status:         WELL UNDER BUDGET ✅

    Note: Under-utilization may indicate project delays
    Recommend: Check with project lead

4️⃣  Industry Partnership (Pharma)
    Monthly budget: $5,000
    Spent (MTD):    $4,892 (98% of monthly budget)
    Remaining:      $108 (2%)
    Status:         NEAR LIMIT (within normal for this grant) ✅

    Note: This grant consistently runs at 95-100% budget
    Pattern: Large uploads early-month, then minimal activity

5️⃣  Foundation Grant (Cancer Research)
    Monthly budget: $2,500
    Spent (MTD):    $1,847 (74% of monthly budget)
    Remaining:      $653 (26%)
    Status:         ON TRACK ✅

6️⃣  Discretionary / Startup
    Monthly budget: $1,250
    Spent (MTD):    $166 (13% of monthly budget)
    Status:         UNDER-UTILIZED ✅

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📈 Trends (vs Last Week)

Total spending:        +$1,247 (+17.8%) - within normal variance
Upload volume:         +1.8 TB (+12.3%) - seasonal increase
Cost per GB:           $1.12/GB (consistent)
Number of uploads:     67 uploads (vs 59 last week)

⚠️  NSF grant spending accelerated (+$2,100 vs last week)
    Action: Lab Manager notified, reviewing with project lead

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔮 Annual Forecast (Based on Current Trends)

Total annual budget:   $450,000 (across all grants)
Projected spend:       $438,000 (97% utilization)
Expected savings:      $12,000 (3% under budget)

Grant-by-grant forecast:
  ✅ NIH R01:          $178,800 / $180,000 (99%)
  ⚠️  NSF:            $92,400 / $90,000 (103%) - OVER BUDGET
  ✅ DOE:             $68,000 / $75,000 (91%)
  ✅ Industry:        $59,500 / $60,000 (99%)
  ✅ Foundation:      $28,300 / $30,000 (94%)
  ✅ Discretionary:   $11,000 / $15,000 (73%)

Action items:
  1. NSF grant trending 3% over budget - reduce Q4 uploads
  2. DOE grant under-utilized - discuss with project lead
  3. Overall: On track to come in under budget ($12K savings)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🎯 Action Items for PI

1. ⚠️  NSF grant approaching monthly limit ($738 over forecast)
   → Lab Manager (Dr. Chen) reviewing with Dr. Thompson
   → Consider budget reallocation or defer non-critical uploads

2. ✅ NIH grant on track (75% with 60% of month elapsed)
   → No action needed

3. 💡 DOE grant under-utilized (46% with 40% of month elapsed)
   → May indicate project delays - check with team lead
   → Under-spend could be flagged in progress reports

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📄 Detailed Reports Available

View full reports at: https://cargoship.lab.university.edu/pi-dashboard

  • Grant accounting report (PDF) - for sponsors
  • Per-user breakdown (CSV) - for lab management
  • Upload audit log (JSON) - for compliance
  • Cost trends (charts) - for presentations

Questions? Contact Dr. Sarah Chen (Lab Manager)
  Email: sarah.chen@university.edu
  Slack: #wilson-lab-it

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**What Prof. Wilson thinks**: *"Perfect. This is exactly what I need - high-level summary with actionable items. The NSF budget warning is caught early enough to correct (not a month-end surprise). The annual forecast shows we're on track to come in under budget, which sponsors like to see. I can forward this directly to our grant administrator. 5 minutes of review and I'm done."*

### ✅ Budget Alert: NSF Grant Approaching Limit (Day 12)

Mid-month, Prof. Wilson receives alert about NSF budget trajectory.

```
From: CargoShip Lab Management <cargoship@university.edu>
To: james.wilson@university.edu
Cc: sarah.chen@university.edu
Subject: ⚠️ ALERT: NSF DBI-2024 Budget Warning (92% of monthly limit)
Date: Wednesday, November 13, 2024 10:47 AM
Priority: High

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🚨 Budget Warning: NSF DBI-2024 Grant
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Grant:          NSF DBI-2024 (Machine Learning for Proteomics)
Monthly budget: $7,500
Current spend:  $6,892 (92% of monthly budget)
Days remaining: 17 days in November

⚠️  Your NSF grant has reached 92% of monthly budget with 17 days remaining.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Recent Activity

Dr. Alex Thompson uploaded 1.2 TB of proteomics data yesterday:
  • Upload: proteomics-timecourse-full
  • Cost: $106 (12x higher than typical daily spend)
  • Status: Completed successfully

This large upload triggered the budget warning threshold (92% > 80%).

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔮 Forecast

If current spending continues:
  • Projected November total: $8,238
  • Budget overrun: $738 (9.8% over)
  • Annual impact: NSF grant will be 3% over budget by year-end

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💡 Recommended Actions

1. IMMEDIATE: Review necessity of remaining uploads this month
   → Dr. Thompson has 3 planned uploads (est. $340)
   → Defer non-critical uploads to December?

2. SHORT-TERM: Implement upload approval for NSF grant (rest of month)
   → Uploads >$50 require Lab Manager approval
   → Prevents accidental budget overruns

3. LONG-TERM: Reallocate budget across grants
   → Transfer $2,000 from under-utilized DOE grant?
   → Or reduce NSF uploads in Q4 to stay within annual budget

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Auto-Response Taken

Lab Manager (Dr. Chen) has been notified and is:
  1. Discussing with Dr. Thompson (NSF project lead)
  2. Reviewing planned uploads for remainder of month
  3. Will provide recommendation by EOD today

You will receive update within 24 hours.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

View detailed breakdown:
https://cargoship.lab.university.edu/grants/nsf-dbi-2024

Questions? Reply to this email or contact:
Dr. Sarah Chen (Lab Manager): sarah.chen@university.edu
```

**What Prof. Wilson thinks**: *"Good catch! The 92% alert triggered with enough time to correct course. Dr. Chen is already on it - I don't need to micromanage. The forecast shows the annual impact (3% over by year-end), which helps me decide if this is worth reallocating budget. I'll wait for Sarah's recommendation by end of day."*

### ✅ Lab Manager Response: Budget Mitigation Plan (Same Day)

Dr. Chen responds with mitigation plan.

```
From: Dr. Sarah Chen <sarah.chen@university.edu>
To: james.wilson@university.edu
Subject: RE: NSF Budget Warning - Mitigation Plan
Date: Wednesday, November 13, 2024 3:42 PM

Prof. Wilson,

I've reviewed the NSF budget situation with Dr. Thompson. Here's the plan:

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📋 Root Cause

Dr. Thompson's large upload (1.2 TB, $106) was for the proteomics time-course
paper submission (deadline: Nov 30). This was necessary and couldn't be deferred.

The upload was 12x larger than normal because it includes:
  • Raw mass spec data (required by journal)
  • Processed protein structures
  • Supplementary datasets

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Mitigation Plan (Approved by Dr. Thompson)

1. DEFER 2 planned uploads to December:
   • Replication dataset #2 (est. $180) → Dec 1
   • Method validation data (est. $90) → Dec 15
   • Savings: $270

2. COMPRESS remaining upload more aggressively:
   • Training data for ML models (planned: $70)
   • Use zstd-level-6 instead of zstd-3
   • Expected cost: $60 (save $10)

3. TOTAL SAVINGS: $280

Result: November spend will be $7,500 (exactly on budget) ✅

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔮 Annual Budget Impact

With this mitigation:
  • November: $7,500 / $7,500 (100% - on budget)
  • December: Will be ~$300 over (deferred uploads)
  • Q1 2025: Reduce by $300 to compensate
  • Annual NSF budget: $90,000 / $90,000 (100% - on track) ✅

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔧 Implemented Controls

I've enabled upload approval for NSF grant (rest of November):
  • Uploads >$50 require my approval
  • Dr. Thompson has been notified
  • Will prevent accidental overruns

This will auto-disable on December 1.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📄 Documentation

Full analysis attached (PDF) for grant records.
No action required from you - situation under control.

Best,
Sarah

Dr. Sarah Chen
Lab Data Manager
Wilson Computational Biology Lab
```

**What Prof. Wilson thinks**: *"Excellent. Sarah handled this proactively, worked with Alex to defer non-critical uploads, and we'll stay on budget. The upload approval control (>$50) for rest of month is smart - prevents any surprises. Annual budget impact is zero. This is exactly how I want my lab managed - exceptions caught early and resolved by the team. I can focus on science."*

### ✅ Grant Renewal: Annual Budget Justification (Month 11)

NSF grant is up for annual renewal. Prof. Wilson needs to justify cloud spending.

```bash
# Generate annual grant report for NSF renewal
cargoship report annual \
  --grant nsf-dbi-2024 \
  --year 2024 \
  --format sponsor-report \
  --include-efficiency-metrics \
  --include-publications \
  --output nsf-renewal-2024.pdf

# Output:
# 📊 Generating Annual Grant Report for NSF DBI-2024...
#
# Report includes:
#   ✅ Monthly spending breakdown
#   ✅ Budget utilization (100% target, 99.8% actual)
#   ✅ Cost optimization metrics (15% compression savings)
#   ✅ Data management practices (duplicate elimination)
#   ✅ Publications enabled (3 papers, 1 preprint)
#   ✅ Data sharing compliance (2 public datasets)
#
# Report saved to: nsf-renewal-2024.pdf (47 pages)
```

**Report excerpt (Executive Summary)**:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
NSF DBI-2024 Annual Cloud Storage Report
Year 1 (December 2023 - November 2024)
Wilson Computational Biology Lab
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💰 Budget Utilization

Allocated budget:     $90,000 / year
Actual spend:         $89,784 (99.8% utilization)
Variance:             -$216 (0.2% under budget)

Quarterly breakdown:
  Q1 (Dec-Feb):       $22,100 (98% of quarterly target)
  Q2 (Mar-May):       $23,500 (104% of quarterly target)
  Q3 (Jun-Aug):       $21,800 (97% of quarterly target)
  Q4 (Sep-Nov):       $22,384 (99% of quarterly target)

Assessment: ✅ Excellent budget management
  • Consistent quarterly spending (21-23K per quarter)
  • No significant overruns or under-utilization
  • Demonstrates reliable forecasting and control

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Data Management Metrics

Total data uploaded:  28.7 TB (proteomics, ML training data)
Total uploads:        842 uploads (avg 34 GB per upload)
Upload success rate:  99.7% (3 failures, auto-recovered)
Data shared publicly: 2 datasets (5.2 TB) - NSF data sharing compliance

Cost efficiency:
  Compression savings: 15% average (zstd-3 lossless)
  Duplicate elimination: $420/year saved
  Network efficiency: <2% of 10 Gbps link (no congestion)
  Cost per GB: $1.12/GB (15% below AWS baseline)

Assessment: ✅ Efficient resource utilization
  • Lossless compression reduced costs without data loss
  • Eliminated duplicate reference files (shared bucket)
  • Network-friendly uploads (no lab disruption)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔬 Scientific Impact

Publications enabled:
  1. Thompson et al. (2024). "Machine learning for proteomics"
     Nature Methods. DOI: 10.1038/s41592-024-xxxxx
     Data: 2.1 TB deposited in ProteomeXchange (PXD034567)

  2. Thompson & Chen (2024). "Protein structure prediction"
     Bioinformatics. DOI: 10.1093/bioinformatics/btxxxx
     Data: 1.8 TB in Protein Data Bank

  3. Thompson et al. (2024). "Time-course proteomics"
     Molecular Systems Biology. DOI: 10.15252/msb.202412345
     Data: 1.2 TB (currently in review, public upon acceptance)

Preprints:
  4. Thompson & Wilson (2024). "Deep learning for mass spec"
     bioRxiv. DOI: 10.1101/2024.10.xxxxx
     Data: 3.5 TB (public via Zenodo)

Total citations: 47 (across 3 published papers)
Data downloads: 1,247 (from public repositories)

Assessment: ✅ High scientific productivity
  • 3 published papers + 1 preprint in Year 1
  • All required data publicly shared (NSF compliance)
  • Data reuse by community (1,247 downloads)
  • Cost per publication: $29,928 (excellent ROI)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📋 Compliance & Audit Trail

NSF Data Management Plan: ✅ COMPLIANT
  • All data uploaded with metadata tags
  • Public datasets deposited within 6 months of publication
  • Persistent identifiers (DOIs) for all shared data
  • README files and documentation for reproducibility

Cost allocation: ✅ ACCURATE
  • Per-upload cost tracking with grant tags
  • Quarterly reports submitted to university finance
  • Audit trail for all expenditures (842 uploads documented)
  • No commingling with other grants

Assessment: ✅ Exemplary grant management
  • Full compliance with NSF data sharing policy
  • Transparent cost tracking and reporting
  • Audit-ready documentation

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🎯 Year 2 Budget Request: $90,000 (same as Year 1)

Justification:
  • Year 1 demonstrated reliable spending (99.8% utilization)
  • Consistent quarterly burn rate ($22-23K per quarter)
  • High scientific productivity (3 papers, 1 preprint)
  • Efficient resource usage (15% compression savings)
  • Full NSF compliance (data sharing, reporting)

Projected Year 2 activities:
  • 4-5 publications (based on current pipeline)
  • 30-35 TB new data (proteomics, ML models)
  • Continued public data sharing (NSF compliance)
  • Integration with multi-omics datasets (new collaboration)

Assessment: ✅ RECOMMEND RENEWAL at requested level

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**What Prof. Wilson thinks**: *"This is grant-renewal gold. The report shows 99.8% budget utilization (not under-spending or over-spending), 3 published papers with public data (NSF loves that), and full compliance with data sharing policy. The efficiency metrics (15% compression, duplicate elimination) show we're good stewards of taxpayer money. I can attach this PDF directly to the renewal application. The cost per publication ($29,928) is excellent ROI. This took 5 minutes to generate and would have taken days to compile manually from AWS billing data."*

### ✅ Multi-Grant Coordination: Budget Reallocation (Mid-Year)

Prof. Wilson realizes DOE grant is under-utilized, wants to reallocate to NSF grant.

```bash
# Check budget utilization across all grants (mid-year)
cargoship budget summary --all-grants --year 2024 --format table

# Output:
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Wilson Lab Multi-Grant Budget Summary (Jan-Jun 2024)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Grant              | Annual Budget | H1 Actual | H1 Target | Variance |  %Util
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# NIH R01-HL         | $180,000      | $89,400   | $90,000   | -$600    | 99.3% ✅
# NSF DBI            | $90,000       | $47,800   | $45,000   | +$2,800  | 106.2% ⚠️
# DOE Science        | $75,000       | $28,200   | $37,500   | -$9,300  | 75.2% ⚠️
# Industry Pharma    | $60,000       | $29,800   | $30,000   | -$200    | 99.3% ✅
# Foundation Cancer  | $30,000       | $14,900   | $15,000   | -$100    | 99.3% ✅
# Discretionary      | $15,000       | $6,100    | $7,500    | -$1,400  | 81.3% ✅
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TOTAL              | $450,000      | $216,200  | $225,000  | -$8,800  | 96.1% ✅
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📊 Analysis:
#
# ⚠️  NSF grant is 6.2% over H1 target (trending to 112% by year-end)
#     → Projected annual spend: $100,800 (12% over budget)
#     → Recommendation: Reduce H2 spending or reallocate budget
#
# ⚠️  DOE grant is 24.8% under H1 target (trending to 75% by year-end)
#     → Projected annual spend: $56,400 (25% under budget)
#     → Recommendation: Investigate under-utilization or reallocate
#
# 💡 Reallocation opportunity: Transfer $10,000 from DOE to NSF
#     → DOE would be at 80% utilization (acceptable for sponsor)
#     → NSF would be at 100% utilization (on budget)
#     → Requires PI and university grants office approval

# Prof. Wilson decides to reallocate
cargoship budget reallocate \
  --from doe-science \
  --to nsf-dbi \
  --amount 10000 \
  --reason "DOE project delayed 6 months, NSF project accelerated" \
  --approval-required \
  --notify-grants-office

# Output:
# 📋 Budget Reallocation Request Submitted
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# From:    DOE Office of Science (Metabolomics)
# To:      NSF DBI-2024 (Machine Learning for Proteomics)
# Amount:  $10,000
# Reason:  DOE project delayed 6 months, NSF project accelerated
#
# Approvals required:
#   ✅ PI (Prof. James Wilson): Auto-approved (you)
#   ⏳ University Grants Office: Pending review
#   ⏳ DOE Program Manager: Notification sent
#   ⏳ NSF Program Manager: Notification sent
#
# Status: PENDING APPROVAL (estimated 5-7 business days)
#
# Once approved:
#   DOE budget: $75,000 → $65,000 (87% util projected)
#   NSF budget: $90,000 → $100,000 (100% util projected)
#
# 💡 You will receive email when all approvals are complete
```

**What Prof. Wilson thinks**: *"The mid-year analysis caught the DOE under-utilization (75%) and NSF overrun trend (112%). The reallocation suggestion ($10K from DOE to NSF) makes sense - DOE project hit delays, NSF is accelerating. CargoShip initiated the approval workflow automatically (university grants office + sponsor program managers). This would have taken weeks of back-and-forth emails without automation. Now I just wait for approvals."*

## PI Dashboard Features

### 1. **High-Level Budget Monitoring**
```
Weekly Digest:
  ✅ Executive summary (not detailed uploads)
  ✅ Budget status by grant (traffic light indicators)
  ✅ Trend analysis (vs last week/month/quarter)
  ✅ Annual forecast (will we stay on budget?)
  ✅ Action items only (exceptions requiring attention)

Real-time Alerts:
  ⚠️  Budget warnings (80%, 90%, 100% thresholds)
  ⚠️  Large uploads (>$1,000)
  ⚠️  Unusual activity (spending spikes)
  ⚠️  Grant renewals (60 days before end)
```

### 2. **Grant Accounting Reports**
```
Annual Reports (for renewals):
  ✅ Budget utilization (monthly, quarterly, annual)
  ✅ Scientific productivity (publications, datasets)
  ✅ Compliance metrics (data sharing, reporting)
  ✅ Efficiency metrics (compression, optimization)
  ✅ Audit trail (all expenditures documented)

Formats:
  📄 PDF (for sponsor submission)
  📊 Excel/CSV (for analysis)
  🔗 URL (for online dashboard)
```

### 3. **Multi-Grant Coordination**
```
Cross-Grant Analysis:
  ✅ Budget utilization comparison
  ✅ Under/over-spend identification
  ✅ Reallocation opportunities
  ✅ Forecasting (will grants finish on budget?)

Automated Workflows:
  ✅ Budget reallocation requests
  ✅ Sponsor notifications
  ✅ University grants office approvals
  ✅ Audit trail documentation
```

### 4. **Strategic Decision Support**
```
Forecasting:
  🔮 Annual spend projection (by grant)
  🔮 Multi-year budget planning
  🔮 Renewal budget justification
  🔮 New grant budget estimation

What-if Analysis:
  💡 What if we reallocate $10K from DOE to NSF?
  💡 What if NSF project accelerates next quarter?
  💡 What if we add 2 more students to NIH grant?
```

## Real-World PI Scenarios

### Scenario A: Grant Renewal Preparation
```
Task:        Prepare NSF renewal application (due in 30 days)
Requirement: Justify cloud budget for Year 2 ($90K)
Challenge:   Need to show efficient use of Year 1 budget

CargoShip solution:
  1. Generate annual report (1 command, 5 minutes)
  2. Report shows: 99.8% budget utilization, 3 publications
  3. Include efficiency metrics (15% compression savings)
  4. Demonstrate compliance (data sharing, reporting)
  5. Result: Strong renewal application with data-backed justification ✅

Time saved: 20+ hours (vs manual AWS billing analysis)
```

### Scenario B: Mid-Year Budget Crisis
```
Scenario:    NSF grant trending 12% over budget at mid-year
Timeline:    Need to correct course before year-end
Challenge:   DOE grant under-utilized (25% under), can we reallocate?

CargoShip solution:
  1. Mid-year analysis identifies under/over-spend
  2. Recommend reallocation ($10K from DOE to NSF)
  3. Automated approval workflow (PI + grants office + sponsors)
  4. Budget reallocation approved in 7 days
  5. Result: NSF stays on budget, DOE at acceptable 87% utilization ✅

Without CargoShip: Month-end crisis, emergency PI approval required
```

### Scenario C: Multi-Grant Portfolio Management
```
Lab size:    20 researchers across 8 active grants
Challenge:   Ensure all grants stay within budget
Concern:     A runaway upload could blow through entire grant budget

CargoShip solution:
  - Weekly digest: High-level summary of all 8 grants
  - Budget alerts: Catch overruns early (80% threshold)
  - Lab Manager handles day-to-day (PI sees exceptions only)
  - Annual forecasting: Plan Year 2+ budgets
  - Result: All 8 grants stay within budget, zero surprises ✅

PI time investment: 5-10 minutes/week (review digest)
Lab Manager time: 2-3 hours/week (detailed management)
```

## PI Best Practices

### 1. **Delegate Day-to-Day, Monitor Exceptions**
```bash
# PI sees: Weekly digest + exception alerts
# Lab Manager sees: Real-time upload monitoring + budget tracking

# Prof. Wilson's approach:
#   - Weekly digest review (5 minutes Monday morning)
#   - Respond only to exception alerts (budget warnings, large uploads)
#   - Trust Lab Manager (Dr. Chen) for daily operations
#   - Focus on science, not cloud billing
```

### 2. **Use Budget Forecasting for Planning**
```bash
# Check annual forecast quarterly
cargoship budget forecast --all-grants --quarters 4

# Output shows:
#   - Will each grant finish on budget?
#   - Are any grants trending over/under?
#   - Should we reallocate mid-year?
#   - What to budget for Year 2 renewals?
```

### 3. **Document Everything for Sponsors**
```bash
# Generate grant reports for renewals
cargoship report annual --grant nsf-dbi-2024 --format sponsor-report

# Reports include:
#   ✅ Budget utilization
#   ✅ Publications enabled
#   ✅ Data sharing compliance
#   ✅ Efficiency metrics
#   ✅ Audit trail

# Attach PDF to renewal application (accepted by NIH, NSF, DOE)
```

### 4. **Set Thresholds for Alerts**
```bash
# Configure alert threshold (default: 80%)
cargoship config pi-dashboard --notification-threshold 80

# Receive alerts when:
#   ⚠️  Any grant reaches 80% of monthly budget
#   ⚠️  Large uploads (>$1,000)
#   ⚠️  Unusual spending spikes (>2x normal)
#   ⚠️  Grant renewals approaching (60 days)
```

## Cost Savings Summary

### What Prof. Wilson Achieved

**Before CargoShip** (Manual AWS Management):
- Budget tracking: Manual AWS billing review (quarterly)
- Grant reports: Days of manual work per renewal
- Cost attribution: Difficult (per-grant breakdown unclear)
- Surprises: Month-end budget overruns (too late to fix)
- Multi-grant coordination: Spreadsheets and estimates
- Audit trail: Reconstruct from AWS logs (painful)
- Efficiency: No visibility into optimization opportunities

**After CargoShip** (Automated Grant Management):
- Budget tracking: Real-time monitoring with weekly digests
- Grant reports: One command (5 minutes, PDF ready)
- Cost attribution: Automatic per-grant tagging
- Surprises: Zero (80% threshold catches issues early)
- Multi-grant coordination: Dashboard + forecasting + reallocation
- Audit trail: Complete documentation (842 uploads logged)
- Efficiency: 15% compression savings, duplicate elimination

**Key Benefits for PIs**:
1. ✅ **20+ hours saved per grant renewal** (automated reports)
2. ✅ **Zero budget surprises** (early warning system)
3. ✅ **Multi-grant visibility** (all 8 grants in one dashboard)
4. ✅ **Sponsor-ready reports** (PDF attachments for renewals)
5. ✅ **Efficient budget use** (15% compression savings, reallocation)
6. ✅ **Full compliance** (data sharing, audit trail, reporting)
7. ✅ **Strategic planning** (forecasting, what-if analysis)

## Next Steps for Principal Investigators

### Immediate (v0.5.0 - Available Today)
- ✅ Configure PI dashboard with weekly digests
- ✅ Set budget alert thresholds (80% recommended)
- ✅ Delegate day-to-day management to Lab Manager
- ✅ Review weekly digests (5 minutes/week)
- ✅ Use annual reports for grant renewals
- ✅ Implement multi-grant budget monitoring

### Coming Soon (v0.6.0+)
- 🔄 **Multi-year forecasting** (plan 3-5 year grant budgets)
- 🔄 **Budget rollover** (unused budget carries forward)
- 🔄 **Grant portfolio optimization** (optimal budget allocation)
- 🔄 **NSF/NIH integration** (submit reports directly to sponsor)
- 🔄 **Institutional dashboards** (university-wide grant oversight)
- 🔄 **Predictive analytics** (identify cost trends, recommend actions)

**What Prof. Wilson thinks**: *"CargoShip transformed how I manage my grant portfolio. The weekly digest gives me high-level visibility without drowning in details. Budget alerts catch issues early enough to fix them. The annual reports for renewals are sponsor-ready PDFs that would have taken days to compile manually. My Lab Manager handles day-to-day operations, I see only the exceptions. We're running 8 grants simultaneously with zero budget surprises. The ROI is incredible - $450K in cloud budget managed efficiently, 99.8% utilization across grants, full audit trail for sponsors. This is how modern grant-funded research should work."*
