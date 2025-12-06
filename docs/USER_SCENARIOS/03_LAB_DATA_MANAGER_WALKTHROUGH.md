# Scenario 3: Lab Data Manager with Multi-Project Coordination

## Persona: Dr. Sarah Chen

**Background**:
- Lab Data Manager for the Chen Computational Biology Lab (15 researchers)
- Oversees data from 5 active grants with different budget allocations
- Manages ~50 TB of research data across genomics, proteomics, and imaging projects
- Primary concern: **Efficient resource allocation and preventing budget overruns**
- Technical level: Expert - manages HPC systems, optimizes workflows, trains lab members
- Authority: Approves large uploads, manages lab-wide S3 buckets, sets storage policies

**Pain Points**:
- Graduate students frequently upload redundant data (same FASTQ files multiple times)
- Different projects have different budget constraints (NIH vs NSF vs startup funds)
- No visibility into who uploaded what, when, and how much it cost
- Storage costs creeping up month-over-month ($800/month and rising)
- Hard to track which data belongs to which grant for accounting
- Current solution: Spreadsheet tracking + manual S3 cost allocation (error-prone!)

**Lab Structure**:
```
Chen Lab Organization
├── NIH R01 Grant ($2,000/month budget)
│   ├── Dr. Maria Garcia (Postdoc): RNA-seq, ATAC-seq
│   ├── James Kim (PhD student): Single-cell sequencing
│   └── Lisa Park (PhD student): Variant calling
│
├── NSF Grant ($1,200/month budget)
│   ├── Dr. Alex Thompson (Postdoc): Proteomics, mass spec
│   └── Robert Chen (PhD student): Imaging, confocal
│
├── Industry Collaboration ($800/month budget)
│   ├── Dr. Emily Wong (Postdoc): Drug screening, HTS
│   └── David Lee (PhD student): Compound library imaging
│
├── Startup Funds ($600/month budget - limited!)
│   └── New students: Training datasets, learning projects
│
└── Shared Resources ($400/month buffer)
    └── Dr. Sarah Chen: Emergency overages, shared datasets
```

---

## Version Legend
- ✅ **v0.5.0 (Current)**: Features available today
- 🔄 **v0.6.0+ (Planned)**: Features in development (see linked GitHub issues)

## Current State (v0.5.0): What Works Today

### ✅ Initial Setup: Lab-Wide Configuration (Day 0)

Sarah sets up CargoShip with lab-wide AWS credentials and per-project S3 buckets.

```bash
# Install CargoShip on shared HPC login node
sudo yum install cargoship

# Configure lab AWS profile
aws configure --profile chen-lab
# AWS Access Key ID: [Lab AWS account]
# AWS Secret Access Key: [provided by university IT]
# Default region: us-west-2
# Default output format: json

# Create configuration file for lab defaults
mkdir -p ~/.cargoship
cat > ~/.cargoship/lab-config.yaml <<EOF
# Chen Lab CargoShip Configuration
# Last updated: 2024-10-20 by Dr. Sarah Chen

default_profile: chen-lab
default_region: us-west-2

# Lab-wide settings
compression:
  algorithm: zstd
  level: 3

memory_limit: 8G  # HPC nodes have 128 GB, safe to use 8 GB

# S3 bucket mapping by project
buckets:
  nih-r01:
    name: chen-lab-nih-r01-us-west-2
    budget: 2000  # USD/month
    tags:
      grant: "NIH-R01-2023"
      pi: "dr-sarah-chen"

  nsf-grant:
    name: chen-lab-nsf-us-west-2
    budget: 1200
    tags:
      grant: "NSF-2024"
      pi: "dr-sarah-chen"

  industry:
    name: chen-lab-industry-collab-us-west-2
    budget: 800
    tags:
      grant: "Industry-Pharma-2024"
      pi: "dr-sarah-chen"

  startup:
    name: chen-lab-startup-us-west-2
    budget: 600
    tags:
      grant: "Startup-Funds"
      pi: "dr-sarah-chen"

  shared:
    name: chen-lab-shared-us-west-2
    budget: 400
    tags:
      project: "shared-resources"
      pi: "dr-sarah-chen"

# Cost allocation tags (for university accounting)
cost_allocation:
  department: "Computational-Biology"
  lab: "Chen-Lab"
  university: "Research-University"
EOF
```

**What Sarah thinks**: *"Good, now I have a centralized configuration. Lab members can reference project names instead of remembering bucket names. The budget tracking will help me catch overages before they happen."*

### ✅ Training Graduate Students (Day 1)

Sarah creates a training guide for lab members to use CargoShip correctly.

```bash
# Create lab wiki entry (shared documentation)
cat > /shared/chen-lab/wiki/cargoship-guide.md <<EOF
# Chen Lab CargoShip Usage Guide

## Quick Start for Lab Members

### 1. Uploading Genomics Data (NIH R01 Project)

\`\`\`bash
# Navigate to your data directory
cd /data/username/rnaseq-experiment

# Upload to NIH R01 bucket (most common for genomics)
cargoship upload \\
  --source . \\
  --project nih-r01 \\
  --prefix your-name/experiment-name/ \\
  --tags "user=your-name,experiment=rnaseq-001" \\
  --verbose

# The --project flag automatically uses the correct bucket and tags
\`\`\`

### 2. Uploading Imaging Data (NSF Project)

\`\`\`bash
cd /data/username/confocal-data

cargoship upload \\
  --source . \\
  --project nsf-grant \\
  --prefix your-name/imaging-session-date/ \\
  --tags "user=your-name,modality=confocal" \\
  --verbose
\`\`\`

### 3. Check Your Upload Costs

\`\`\`bash
# See your uploads this month
cargoship cost summary --user your-name --month current

# See project totals (Lab Manager only)
cargoship cost summary --project nih-r01 --month current
\`\`\`

## Important Rules

1. **Always use --project flag**: Don't upload to buckets directly
2. **Always tag with your name**: --tags "user=your-name"
3. **Use descriptive prefixes**: your-name/experiment-description/
4. **Check costs before large uploads**: cargoship estimate --source .
5. **Don't upload training data to grant buckets**: Use --project startup

## Need Help?

- Slack: #chen-lab-it channel
- Email: sarah.chen@university.edu
- Office hours: Tuesdays 2-4 PM
EOF
```

### ✅ First Multi-User Upload Day (Day 2)

Three lab members upload data simultaneously - Sarah monitors to ensure no conflicts.

**User 1: James (RNA-seq, NIH R01)**
```bash
# James uploads 25 GB of RNA-seq data
cd /data/james/rnaseq-liver-study

cargoship upload \
  --source . \
  --project nih-r01 \
  --prefix james-kim/liver-rnaseq-oct-2024/ \
  --tags "user=james-kim,tissue=liver,assay=rnaseq" \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Project:      nih-r01
#   Bucket:       chen-lab-nih-r01-us-west-2
#   Prefix:       james-kim/liver-rnaseq-oct-2024/
#   User:         james-kim
#   Grant:        NIH-R01-2023
#
# 📊 Scanning files...
# ✅ Found 12 FASTQ.gz files (25.0 GB uncompressed)
#
# 💰 Cost Estimate:
#   Upload size:      21.3 GB (compressed, 15% reduction)
#   Data transfer:    $1.92 (at $0.09/GB)
#   Storage/month:    $0.49 (at $0.023/GB-month)
#   Project budget:   $1,847 / $2,000 remaining (92% available)
#
# Proceed with upload? [Y/n]: y
#
# 🔄 Processing pipeline started...
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# 📈 Performance: 125 MB/s processing | 106 MB/s network
# ⏱️  Duration: 3m 21s
# 💰 Cost: $1.92 transfer + $0.49/month storage
# 🔗 Location: s3://chen-lab-nih-r01-us-west-2/james-kim/liver-rnaseq-oct-2024/
```

**User 2: Robert (Confocal imaging, NSF Grant)**
```bash
# Robert uploads 80 GB of confocal TIFF stacks
cd /data/robert/confocal-neurons

cargoship upload \
  --source . \
  --project nsf-grant \
  --prefix robert-chen/neuron-imaging-oct-2024/ \
  --tags "user=robert-chen,modality=confocal,sample=neurons" \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Project:      nsf-grant
#   Bucket:       chen-lab-nsf-us-west-2
#   Prefix:       robert-chen/neuron-imaging-oct-2024/
#   User:         robert-chen
#   Grant:        NSF-2024
#
# 📊 Scanning files...
# ✅ Found 4 TIFF stacks (80.0 GB uncompressed)
#
# 💰 Cost Estimate:
#   Upload size:      64.0 GB (compressed, 20% reduction, LOSSLESS)
#   Data transfer:    $5.76 (at $0.09/GB)
#   Storage/month:    $1.47 (at $0.023/GB-month)
#   Project budget:   $1,187 / $1,200 remaining (99% available)
#
# ⚠️  This upload will use 5% of monthly NSF budget
# Proceed with upload? [Y/n]: y
#
# 🔄 Processing pipeline started...
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# 📈 Performance: 142 MB/s processing | 114 MB/s network
# ⏱️  Duration: 9m 24s
# 💰 Cost: $5.76 transfer + $1.47/month storage
```

**User 3: Emily (High-throughput screening, Industry)**
```bash
# Emily uploads 400 GB of screening images
cd /data/emily/hts-drug-screen

cargoship upload \
  --source . \
  --project industry \
  --prefix emily-wong/drug-screen-q4-2024/ \
  --tags "user=emily-wong,assay=hts,compound-library=v3" \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Project:      industry
#   Bucket:       chen-lab-industry-collab-us-west-2
#   Prefix:       emily-wong/drug-screen-q4-2024/
#   User:         emily-wong
#   Grant:        Industry-Pharma-2024
#
# 📊 Scanning files...
# ✅ Found 50,000 PNG files (400 GB uncompressed)
#
# 💰 Cost Estimate:
#   Upload size:      352 GB (compressed, 12% reduction)
#   Data transfer:    $31.68 (at $0.09/GB)
#   Storage/month:    $8.10 (at $0.023/GB-month)
#   Project budget:   $762 / $800 remaining (95% available)
#
# ⚠️  This upload will use 40% of monthly Industry budget!
# ⚠️  Industry project has limited budget ($800/month)
# Proceed with upload? [Y/n]: y
#
# 🔄 Processing pipeline started...
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# 📈 Performance: 133 MB/s processing | 117 MB/s network
# ⏱️  Duration: 50m 18s
# 💰 Cost: $31.68 transfer + $8.10/month storage
```

**Sarah's monitoring dashboard**:
```bash
# Sarah checks lab-wide upload activity
cargoship status --lab-wide

# Output:
# 🔧 Chen Lab Upload Activity (Real-Time)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Active Uploads: 3
#
# 1️⃣  james-kim (NIH R01)
#    Status:     ✅ Completed
#    Data:       25 GB → 21.3 GB (15% compression)
#    Duration:   3m 21s
#    Cost:       $1.92 + $0.49/mo
#    Network:    106 MB/s (0% of 10 Gbps link - completed)
#
# 2️⃣  robert-chen (NSF Grant)
#    Status:     🔄 Uploading (78% complete)
#    Data:       62.4 GB / 80 GB processed
#    ETA:        2m 15s remaining
#    Cost:       $5.62 so far (est. $5.76 total)
#    Network:    114 MB/s (0.9% of 10 Gbps link)
#
# 3️⃣  emily-wong (Industry)
#    Status:     🔄 Uploading (45% complete)
#    Data:       180 GB / 400 GB processed
#    ETA:        27m 30s remaining
#    Cost:       $14.58 so far (est. $31.68 total)
#    Network:    117 MB/s (0.9% of 10 Gbps link)
#
# 📊 Lab Network Usage: 231 MB/s total (1.8% of 10 Gbps)
# 💰 Today's Total Cost: $22.12 (+ $10.06/month storage)
#
# 💡 All uploads running smoothly - no network congestion
```

**What Sarah thinks**: *"Perfect! Three simultaneous uploads and we're only using 1.8% of the network. The cost tracking is automatic and tagged by user and project. Emily's large upload triggered a budget warning, which is exactly what I wanted. I can see everything in real-time."*

### ✅ Monthly Budget Review (End of Month)

Sarah generates cost reports for grant accounting.

```bash
# Generate monthly cost report for all projects
cargoship cost report --month october --output-format pdf

# Output:
# 📊 Generating Chen Lab Cost Report for October 2024...
#
# Report saved to: /home/sarah/chen-lab-cost-report-2024-10.pdf
#
# Summary:
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# NIH R01 Project:
#   Budget:           $2,000 / month
#   Spent:            $1,847 (92% of budget)
#   Remaining:        $153 (8%)
#   Uploads:          47 (1.2 TB total)
#   Top users:        james-kim ($687), maria-garcia ($542), lisa-park ($418)
#   Largest upload:   james-kim/liver-rnaseq-oct-2024 (250 GB, $22.50)
#   Status:           ✅ Within budget
#
# NSF Grant Project:
#   Budget:           $1,200 / month
#   Spent:            $1,187 (99% of budget)
#   Remaining:        $13 (1%)
#   Uploads:          23 (680 GB total)
#   Top users:        robert-chen ($892), alex-thompson ($295)
#   Largest upload:   robert-chen/neuron-imaging-oct-2024 (320 GB, $28.80)
#   Status:           ⚠️  Near budget limit (99%)
#
# Industry Collaboration:
#   Budget:           $800 / month
#   Spent:            $762 (95% of budget)
#   Remaining:        $38 (5%)
#   Uploads:          8 (1.5 TB total)
#   Top users:        emily-wong ($762)
#   Largest upload:   emily-wong/drug-screen-q4-2024 (400 GB, $35.20)
#   Status:           ⚠️  Near budget limit (95%)
#
# Startup Funds:
#   Budget:           $600 / month
#   Spent:            $127 (21% of budget)
#   Remaining:        $473 (79%)
#   Uploads:          12 (85 GB total)
#   Top users:        new-student-1 ($87), new-student-2 ($40)
#   Status:           ✅ Well under budget
#
# Shared Resources:
#   Budget:           $400 / month (buffer)
#   Spent:            $0 (0%)
#   Remaining:        $400 (100%)
#   Uploads:          0
#   Status:           ✅ Unused (good!)
#
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Lab Total:
#   Combined budget:  $5,000 / month
#   Total spent:      $3,923 (78% of combined budget)
#   Total uploads:    90 uploads (3.4 TB)
#   Average cost:     $43.59 per upload
#   Network impact:   <2% utilization (lab-friendly)
#
# 💰 Compared to Last Month (September):
#   Spending:         +$342 (+9.6%) - seasonal increase expected
#   Upload volume:    +680 GB (+25%) - multiple large experiments
#   Cost per GB:      $1.15/GB (consistent with last month)
#
# 📈 Trends:
#   ✅ NIH R01: Consistent usage, well-managed
#   ⚠️  NSF: Approaching budget limit - review with team
#   ⚠️  Industry: Approaching budget limit - check with sponsor
#   ✅ Startup: Underutilized - good for new student training
```

**What Sarah thinks**: *"This is exactly what I need for grant reporting! NSF and Industry are at 95%+ budget usage - I need to talk with those teams about upcoming uploads. The per-user breakdown helps me identify who needs training on efficient data management. I can directly attach this PDF to grant progress reports."*

### ✅ Identifying Redundant Data (Data Deduplication Analysis)

Sarah notices storage costs creeping up and investigates.

```bash
# Analyze duplicate files across lab
cargoship analyze duplicates --lab-wide --min-size 1GB

# Output:
# 🔍 Chen Lab Duplicate File Analysis
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Scanning all lab S3 buckets for duplicate files (>1 GB)...
# ✅ Scan complete: 3,847 files analyzed (47.2 TB total)
#
# 📊 Duplicate Files Found: 47 files (682 GB wasted storage)
#
# Top Duplicates:
#
# 1️⃣  reference-genome-hg38.fa (32 GB)
#    Copies: 8
#    Locations:
#      - s3://chen-lab-nih-r01/.../james-kim/ref/hg38.fa
#      - s3://chen-lab-nih-r01/.../maria-garcia/ref/hg38.fa
#      - s3://chen-lab-nih-r01/.../lisa-park/reference/hg38.fa
#      - s3://chen-lab-nsf/.../alex-thompson/ref/hg38.fa
#      ... (4 more copies)
#    Wasted storage: 224 GB (7 redundant copies)
#    Cost: $5.15/month ($61.80/year)
#    Recommendation: Move to shared bucket, use single copy
#
# 2️⃣  sample_R1.fastq.gz (25 GB)
#    Copies: 3
#    Locations:
#      - s3://chen-lab-nih-r01/.../james-kim/experiment-001/sample_R1.fastq.gz
#      - s3://chen-lab-nih-r01/.../james-kim/experiment-002-rerun/sample_R1.fastq.gz
#      - s3://chen-lab-nih-r01/.../james-kim/backup/sample_R1.fastq.gz
#    Wasted storage: 50 GB (2 redundant copies)
#    Cost: $1.15/month ($13.80/year)
#    Recommendation: Keep only in experiment-001/, delete duplicates
#
# 3️⃣  compound-library-v3.zip (18 GB)
#    Copies: 2
#    Locations:
#      - s3://chen-lab-industry/.../emily-wong/screening-q3/library.zip
#      - s3://chen-lab-industry/.../emily-wong/screening-q4/library.zip
#    Wasted storage: 18 GB (1 redundant copy)
#    Cost: $0.41/month ($4.96/year)
#    Recommendation: Use shared reference, symlink or document location
#
# ... (44 more duplicates)
#
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Summary:
#   Total duplicates:     47 files
#   Wasted storage:       682 GB (1.4% of total lab storage)
#   Monthly waste:        $15.69/month
#   Annual waste:         $188.28/year
#
# 💡 Recommendations:
#   1. Create shared reference bucket for common files (genomes, libraries)
#   2. Train users to check for existing files before uploading
#   3. Implement lab policy: References in shared/, experiments in project buckets
#   4. Delete identified redundant copies (save $188/year)
#
# 🔧 Quick Actions:
#   cargoship deduplicate --dry-run    # Preview changes
#   cargoship deduplicate --execute    # Delete duplicates (requires approval)
```

**What Sarah thinks**: *"$188/year wasted on duplicate reference genomes! That's real money I can save. The analysis shows exactly which files are duplicated and where. I'll create a shared reference bucket and train the lab on proper data organization. The dry-run mode lets me preview changes before deleting anything."*

### ✅ Setting Up Shared Reference Data (Day 30)

Sarah creates a shared bucket for common reference files.

```bash
# Create shared reference configuration
cargoship bucket create-shared \
  --name chen-lab-shared-references \
  --region us-west-2 \
  --public-read-access within-lab \
  --description "Common reference genomes, annotations, and libraries"

# Output:
# ✅ Created shared bucket: chen-lab-shared-references
# 🔗 S3 location: s3://chen-lab-shared-references/
# 🔒 Access: Lab members can read, only Lab Manager can write

# Move reference genome to shared bucket
cargoship move \
  --source s3://chen-lab-nih-r01/james-kim/ref/hg38.fa \
  --destination s3://chen-lab-shared-references/genomes/hg38.fa \
  --delete-source

# Output:
# 🔄 Moving hg38.fa (32 GB) to shared references...
# ✅ Move complete
# 🗑️  Deleted source copy
#
# 💰 Savings: Will save $5.15/month once all 7 duplicate copies are deleted

# Create README for lab members
cargoship object put \
  --bucket chen-lab-shared-references \
  --key README.md \
  --body - <<EOF
# Chen Lab Shared References

This bucket contains common reference files used by multiple lab members.

## Available References

### Human Genomes
- \`genomes/hg38.fa\` - Human reference genome (GRCh38)
- \`genomes/hg19.fa\` - Human reference genome (GRCh37)
- \`annotations/gencode.v38.gtf\` - GENCODE gene annotations

### Mouse Genomes
- \`genomes/mm10.fa\` - Mouse reference genome (GRCm38)
- \`annotations/gencode.vM25.gtf\` - GENCODE mouse annotations

### Compound Libraries
- \`libraries/compound-library-v3.zip\` - Drug screening library (18 GB)

## Usage

\`\`\`bash
# Download reference genome
aws s3 cp s3://chen-lab-shared-references/genomes/hg38.fa .

# Or use directly in workflows (no download needed)
bwa mem s3://chen-lab-shared-references/genomes/hg38.fa sample.fastq
\`\`\`

## Adding New References

Contact Dr. Sarah Chen (sarah.chen@university.edu) to add new shared references.
Only Lab Manager can write to this bucket to ensure data integrity.
EOF

# Notify lab members
cat > /shared/chen-lab/announcements/shared-references.txt <<EOF
📢 New: Shared Reference Bucket

Hi team,

I've created a shared reference bucket to reduce duplicate uploads:
  s3://chen-lab-shared-references/

Available now:
  - Human genomes (hg38, hg19)
  - Mouse genome (mm10)
  - Gene annotations (GENCODE)
  - Compound screening library

Please use these shared references instead of uploading your own copies.

This will save the lab ~$188/year in duplicate storage costs!

See README: s3://chen-lab-shared-references/README.md

Questions? Slack #chen-lab-it or email me.

- Sarah
EOF
```

### ✅ Emergency Budget Override (Day 45)

NSF project hits budget limit mid-month, Sarah uses shared buffer.

```bash
# Robert tries to upload large imaging dataset
cd /data/robert/confocal-drug-response

cargoship upload \
  --source . \
  --project nsf-grant \
  --prefix robert-chen/drug-response-nov-2024/ \
  --tags "user=robert-chen,assay=drug-response" \
  --verbose

# Output:
# ⚠️  Budget Check Failed
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Project:        nsf-grant
# Budget:         $1,200 / month
# Current spend:  $1,187 (99%)
# Upload cost:    $42.30 (estimated)
# Total if uploaded: $1,229 (102% of budget)
#
# ❌ This upload would exceed NSF monthly budget by $29.
#
# Options:
#   1. Wait until next month (11 days)
#   2. Request budget override from Lab Manager
#   3. Use compressed upload (--compression-level 6) to reduce cost
#   4. Upload only critical files now, defer rest
#
# Contact Dr. Sarah Chen for budget override.

# Robert contacts Sarah via Slack
# Sarah reviews and approves override

# Sarah's approval command
cargoship budget override \
  --project nsf-grant \
  --amount 50 \
  --source shared-resources \
  --reason "Critical drug response imaging for paper deadline" \
  --approved-by sarah-chen \
  --notify robert-chen

# Output:
# ✅ Budget Override Approved
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Project:        nsf-grant
# Override amount: $50 (from shared-resources buffer)
# New limit:      $1,250 for November 2024
# Approved by:    Dr. Sarah Chen
# Reason:         Critical drug response imaging for paper deadline
# Valid until:    2024-11-30
#
# 💡 User robert-chen has been notified via email
# 💰 Shared resources buffer: $350 / $400 remaining

# Robert retries upload (now succeeds)
cargoship upload \
  --source . \
  --project nsf-grant \
  --prefix robert-chen/drug-response-nov-2024/ \
  --tags "user=robert-chen,assay=drug-response" \
  --verbose

# Output:
# ✅ Budget check passed (override approved)
# 🔄 Upload proceeding...
# ... [upload completes successfully]
```

**What Sarah thinks**: *"The budget override system worked perfectly. Robert got blocked automatically (no surprise overages), requested approval through proper channels, and I could approve it with full audit trail. The $50 came from our shared buffer, which is exactly what it's for. Everything is documented for grant accounting."*

## Lab Management Benefits

### 1. **Centralized Cost Tracking**
```
Before CargoShip:
  - Manual spreadsheet tracking (error-prone)
  - Monthly AWS bill review (hard to attribute costs)
  - No per-user visibility
  - Surprise overages at month-end

After CargoShip:
  - Automatic cost attribution by user, project, grant
  - Real-time budget monitoring with warnings
  - Per-upload cost estimates before upload
  - Monthly PDF reports for grant accounting
```

### 2. **Duplicate Data Elimination**
```
Without analysis:
  - 682 GB of duplicate reference files
  - $188/year wasted on redundant storage
  - No visibility into what's duplicated

With CargoShip analysis:
  - Identified 47 duplicate files automatically
  - Created shared reference bucket (one copy)
  - Saved $188/year ($1,880 over 10 years)
  - Lab-wide policy for reference data
```

### 3. **Budget Enforcement with Flexibility**
```
Old system:
  - No budget limits (only month-end review)
  - Surprise overages require PI approval
  - No emergency override mechanism
  - Students blame each other for overages

CargoShip system:
  - Pre-upload budget checks (prevent overages)
  - Automatic warnings at 75%, 90%, 100%
  - Lab Manager override for critical uploads
  - Full audit trail for grant reporting
```

### 4. **User Activity Visibility**
```
Before:
  - Who uploaded what? (check AWS logs manually)
  - When did they upload? (parse timestamps)
  - How much did it cost? (estimate from file sizes)
  - Network impact? (no visibility)

After:
  - Real-time upload monitoring (all users)
  - Per-user cost attribution (automatic)
  - Network utilization tracking (prevent congestion)
  - Historical reports (monthly, quarterly, annual)
```

## Real-World Lab Scenarios

### Scenario A: Monthly Operations (Typical Month)
```
Lab size:        15 researchers (3 postdocs + 12 students)
Projects:        5 active grants
Total uploads:   90 uploads (3.4 TB)
Total cost:      $3,923 / $5,000 budget (78%)
Network impact:  <2% utilization (10 Gbps link)
Time saved:      ~40 hours/month (vs manual tracking)
Cost savings:    $188/year (duplicate elimination)
```

### Scenario B: End-of-Grant Data Freeze
```
Scenario:        NIH R01 grant ending, finalize datasets
Timeline:        2 weeks to upload all lab data
Data volume:     15 TB (final papers, supplementary data)
Challenge:       Multiple users uploading simultaneously
Budget:          $1,200 remaining (tight deadline)

CargoShip solution:
  - Priority queue (paper data first, supplementary later)
  - Parallel uploads (10 users, no network congestion)
  - Budget tracking (stay under $1,200 limit)
  - Shared references (no duplicate uploads)
  - Result: Completed in 10 days, $1,187 total cost ✅
```

### Scenario C: New Grant Startup
```
Scenario:        New NSF grant started, onboard 3 new students
Challenge:       Students need training data, limited startup budget
Budget:          $600/month (startup funds, not grant budget)

CargoShip solution:
  - Separate "startup" project bucket
  - Training dataset uploads (<$100 for all 3 students)
  - Shared reference bucket (no redundant genome uploads)
  - Cost alerts (prevent accidental overages)
  - Result: Students trained, only $127 spent in month 1 ✅
```

## Lab-Wide Best Practices

### 1. **Project-Based Organization**
```bash
# ✅ Good: Use --project flag (automatic bucket + tags)
cargoship upload --source . --project nih-r01 --prefix your-name/experiment/

# ❌ Bad: Manual bucket selection (easy to mis-tag)
cargoship upload --source . --bucket chen-lab-nih-r01-us-west-2
```

### 2. **Consistent Naming Conventions**
```
Prefix structure: {user-name}/{project-type}/{date-or-description}/

Examples:
  ✅ james-kim/rnaseq/liver-study-oct-2024/
  ✅ robert-chen/confocal/neuron-imaging-2024-10-15/
  ✅ emily-wong/hts/drug-screen-q4-2024/

Avoid:
  ❌ data/experiment1/
  ❌ untitled-folder/
  ❌ james/stuff/
```

### 3. **Use Shared References**
```bash
# ✅ Good: Use shared reference bucket
bwa mem s3://chen-lab-shared-references/genomes/hg38.fa sample.fastq

# ❌ Bad: Upload your own copy of reference genome
cargoship upload --source hg38.fa --project nih-r01  # 32 GB waste!
```

### 4. **Regular Duplicate Audits**
```bash
# Run quarterly duplicate analysis
cargoship analyze duplicates --lab-wide --min-size 1GB

# Review and delete identified duplicates
cargoship deduplicate --dry-run    # Preview
cargoship deduplicate --execute    # Delete (after review)
```

### 5. **Monthly Budget Reviews**
```bash
# Generate report at month-end
cargoship cost report --month current --output-format pdf

# Share with PI and grant administrators
# Archive for grant accounting
```

## Troubleshooting Lab Issues

### Issue 1: User Budget Override Request

**Scenario**: Student needs to upload large dataset but project at budget limit.

**Solution**:
```bash
# 1. Student sees budget warning and contacts Lab Manager
# 2. Lab Manager reviews request
# 3. Approve override if justified
cargoship budget override \
  --project nsf-grant \
  --amount 100 \
  --source shared-resources \
  --reason "Final dissertation data upload" \
  --approved-by sarah-chen \
  --notify student-name

# 4. Student proceeds with upload
```

### Issue 2: Duplicate Data Accumulation

**Scenario**: Storage costs creeping up month-over-month.

**Diagnosis**:
```bash
# Run duplicate analysis
cargoship analyze duplicates --lab-wide

# Output shows: 47 duplicate files, 682 GB wasted, $188/year
```

**Solution**:
```bash
# 1. Create shared reference bucket
cargoship bucket create-shared --name chen-lab-shared-references

# 2. Move common files to shared bucket
cargoship move --source ... --destination s3://chen-lab-shared-references/...

# 3. Update lab wiki with shared bucket usage
# 4. Delete duplicate copies after verification
cargoship deduplicate --execute
```

### Issue 3: Simultaneous Large Uploads

**Scenario**: 5 users uploading large datasets simultaneously (network congestion concern).

**Monitoring**:
```bash
# Lab Manager checks real-time status
cargoship status --lab-wide

# Output shows:
#   User 1: 120 MB/s
#   User 2: 115 MB/s
#   User 3: 110 MB/s
#   User 4: 118 MB/s
#   User 5: 112 MB/s
#   Total: 575 MB/s (4.6% of 10 Gbps link)
#
# ✅ No congestion - all uploads proceed smoothly
```

**Result**: No action needed, network has plenty of capacity.

## Cost Optimization Summary

### What Sarah Achieved

**Before CargoShip** (Manual Tracking):
- Cost tracking: Manual spreadsheet (hours/month)
- Budget overages: Frequent (no pre-upload checks)
- Duplicate data: 682 GB ($188/year waste)
- User attribution: Difficult (manual AWS log parsing)
- Network visibility: None (users complain about slowness)
- Grant reports: Hours of manual work per month
- Lab coordination: Constant Slack messages asking "who's uploading?"

**After CargoShip** (Automated Lab Management):
- Cost tracking: Automatic per-user, per-project, per-upload
- Budget overages: Zero (pre-upload checks + warnings)
- Duplicate data: Eliminated via shared references ($188/year saved)
- User attribution: Real-time dashboard + monthly reports
- Network visibility: 1.8% utilization (no complaints)
- Grant reports: One-command PDF generation (5 minutes)
- Lab coordination: Self-service with automatic monitoring

**Key Benefits for Lab Managers**:
1. ✅ **40 hours/month saved** (vs manual tracking and AWS log parsing)
2. ✅ **$188/year saved** (duplicate data elimination)
3. ✅ **Zero budget overages** (pre-upload checks prevent surprises)
4. ✅ **Real-time visibility** (who's uploading what, right now)
5. ✅ **Automatic grant reports** (PDF ready for submission)
6. ✅ **Audit trail** (budget overrides, user activity, costs)
7. ✅ **Lab-wide coordination** (no network congestion, shared resources)

## Next Steps for Lab Managers

### Immediate (v0.5.0 - Available Today)
- ✅ Set up project-based buckets with budget limits
- ✅ Create shared reference bucket for common files
- ✅ Train lab members on CargoShip usage
- ✅ Run monthly duplicate analysis and cleanup
- ✅ Generate cost reports for grant accounting
- ✅ Monitor real-time uploads to prevent network congestion

### Coming Soon (v0.6.0+)
- 🔄 **Automatic duplicate prevention** (check before upload)
- 🔄 **Budget rollover** (unused budget carries to next month)
- 🔄 **Email notifications** (budget warnings, large uploads)
- 🔄 **Slack integration** (upload notifications, budget alerts)
- 🔄 **Multi-lab federation** (share references across labs)
- 🔄 **Enhanced analytics** (upload trends, cost predictions)

**What Sarah thinks**: *"CargoShip transformed how we manage lab data. The automatic cost tracking alone saves me 40 hours per month, and the budget enforcement prevents surprise overages. The duplicate analysis saved us $188/year - that's real money I can use for compute resources. The real-time monitoring means I can coordinate 15 researchers uploading simultaneously without anyone stepping on each other's toes. Grant reporting went from hours to minutes. This is exactly what a multi-project lab needs!"*
