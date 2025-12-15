# Cost Optimization: Saving 90% with Intelligent Tiering

**Published**: December 2025
**Author**: CargoShip Team
**Read time**: 10 minutes

---

After our [multi-prefix deep dive](03-multi-prefix-deep-dive.md), you know how CargoShip achieves 7-8x faster uploads. But speed isn't the only win—**cost optimization** is often the more compelling benefit.

A research lab we worked with was spending $12,000/year storing archived experimental data in S3 Standard. After switching to CargoShip with intelligent storage class selection, their annual cost dropped to **$1,100**—a **91% reduction**.

Same data. Same availability. 91% less money.

This post shows you how to achieve similar results using CargoShip's cost optimization features.

## Understanding S3 Storage Costs

S3 offers six storage classes with wildly different pricing:

| Storage Class | Cost ($/GB-month) | Retrieval Cost | Retrieval Time | Use Case |
|--------------|-------------------|----------------|----------------|----------|
| **STANDARD** | $0.023 | None | Instant | Active data |
| **STANDARD_IA** | $0.0125 | $0.01/GB | Instant | Infrequent access |
| **INTELLIGENT_TIERING** | $0.023-0.0036* | None | Instant | Automatic optimization |
| **GLACIER_IR** | $0.004 | $0.03/GB | Instant | Archive, rare access |
| **GLACIER** | $0.0036 | $0.02/GB | 1-5 minutes | Long-term archive |
| **DEEP_ARCHIVE** | $0.00099 | $0.02/GB | 12 hours | Compliance archive |

*(Prices for us-east-1, varies by region)*

The price difference is **23×** between STANDARD and DEEP_ARCHIVE. For a 10TB dataset:
- **STANDARD**: $230/month = **$2,760/year**
- **DEEP_ARCHIVE**: $10/month = **$120/year**

But there's a catch: retrieval costs and access times.

## The Cost Trap: Request Charges

Most people focus on storage costs and ignore **request charges**. This is a mistake.

### Traditional File-by-File Uploads

Using `aws s3 sync` to upload 1 million files:

```bash
aws s3 sync ./data s3://bucket/prefix/
```

**Request costs**:
- 1,000,000 PUT requests × $0.005 per 1,000 = **$5.00**

Doesn't sound like much. But do this daily for a year:
- **$5.00 × 365 = $1,825/year**

And that's just uploads. If you ever need to list or retrieve those files:
- **LIST requests**: $0.005 per 1,000 requests
- **GET requests**: $0.0004 per 1,000 requests

For 1 million files:
- LIST: $5.00
- GET (full restore): $0.40

**Total for one backup + one restore cycle**: $10.40

### CargoShip's Archived Approach

CargoShip groups files into ~100 compressed archives:

```bash
cargoship create upload ./data --bucket bucket --prefix prefix
```

**Request costs**:
- 100 PUT requests × $0.005 per 1,000 = **$0.0005**

For daily uploads over a year:
- **$0.0005 × 365 = $0.18/year**

**Savings**: $1,825 - $0.18 = **$1,824.82/year** (99.99% reduction)

## Intelligent Storage Class Selection

CargoShip's `estimate` command analyzes your data and recommends optimal storage classes:

```bash
cargoship estimate ./genomics-data --show-breakdown
```

Output:
```
📊 Storage Analysis (1.2TB, 847,000 files)

Compression estimate: 480 GB (60% reduction)

Storage Class Comparison:
┌─────────────────────┬──────────────┬──────────────┬─────────────┐
│ Storage Class       │ Monthly Cost │ Annual Cost  │ Retrieval   │
├─────────────────────┼──────────────┼──────────────┼─────────────┤
│ STANDARD            │ $276.48     │ $3,317.76   │ Free        │
│ STANDARD_IA         │ $150.00     │ $1,800.00   │ $4.80       │
│ INTELLIGENT_TIERING │ $276-43*    │ $3,317-516* │ Free        │
│ GLACIER_IR          │ $48.00      │ $576.00     │ $14.40      │
│ GLACIER             │ $43.20      │ $518.40     │ $9.60       │
│ DEEP_ARCHIVE        │ $11.88      │ $142.56     │ $9.60       │
└─────────────────────┴──────────────┴──────────────┴─────────────┘

*Automatically moves data to cheaper tiers after 30/90 days

💡 Recommendations:
• Raw sequencing data (800 GB) → DEEP_ARCHIVE
  - Rarely accessed, $7.92/month, $95/year
• Analysis results (400 GB) → GLACIER_IR
  - Occasional access, $16/month, $192/year
• Metadata and logs (4 GB) → STANDARD_IA
  - Frequent queries, $0.50/month, $6/year

Total optimized cost: $24.42/month ($293/year)
Savings vs. STANDARD: $3,025/year (91%)
```

### How to Use Recommendations

Upload different data types to different prefixes with appropriate storage classes:

```bash
# Raw data (rarely accessed)
cargoship create upload ./raw-data \
    --bucket genomics-archive \
    --prefix experiments/exp-42/raw \
    --storage-class DEEP_ARCHIVE

# Analysis results (occasional access)
cargoship create upload ./results \
    --bucket genomics-archive \
    --prefix experiments/exp-42/results \
    --storage-class GLACIER_IR

# Metadata (frequent queries)
cargoship create upload ./metadata \
    --bucket genomics-archive \
    --prefix experiments/exp-42/metadata \
    --storage-class STANDARD_IA
```

## Lifecycle Policies: Automatic Cost Reduction

S3 lifecycle policies automatically move objects to cheaper storage classes over time. CargoShip provides templates for common scenarios.

### List Available Templates

```bash
cargoship lifecycle --list-templates
```

Output:
```
Available lifecycle templates:

1. archive-optimization
   - 30 days: STANDARD → STANDARD_IA
   - 90 days: STANDARD_IA → GLACIER_IR
   - 365 days: GLACIER_IR → DEEP_ARCHIVE

2. backup-retention
   - Keep in STANDARD for 7 days
   - Move to GLACIER_IR after 7 days
   - Delete after 2555 days (7 years)

3. log-aggregation
   - Keep in STANDARD for 30 days
   - Move to STANDARD_IA after 30 days
   - Delete after 90 days

4. compliance-archive
   - Immediate: DEEP_ARCHIVE
   - Retain for 2555 days (7 years)
   - Legal hold enabled
```

### Apply a Template

```bash
cargoship lifecycle \
    --bucket genomics-archive \
    --prefix experiments/ \
    --template archive-optimization
```

This creates the following S3 lifecycle policy:

```xml
<LifecycleConfiguration>
    <Rule>
        <ID>archive-optimization</ID>
        <Prefix>experiments/</Prefix>
        <Status>Enabled</Status>
        <Transition>
            <Days>30</Days>
            <StorageClass>STANDARD_IA</StorageClass>
        </Transition>
        <Transition>
            <Days>90</Days>
            <StorageClass>GLACIER_IR</StorageClass>
        </Transition>
        <Transition>
            <Days>365</Days>
            <StorageClass>DEEP_ARCHIVE</StorageClass>
        </Transition>
    </Rule>
</LifecycleConfiguration>
```

Now your data automatically gets cheaper over time, without manual intervention.

### Cost Impact Over Time

For 1TB uploaded to STANDARD with the archive-optimization policy:

| Time Period | Storage Class | Monthly Cost |
|------------|---------------|--------------|
| Days 1-30 | STANDARD | $23.00 |
| Days 31-90 | STANDARD_IA | $12.50 |
| Days 91-365 | GLACIER_IR | $4.00 |
| Year 2+ | DEEP_ARCHIVE | $0.99 |

**Average annual cost (Year 1)**: $7.87/month = **$94.44/year**
**Average annual cost (Year 2+)**: $0.99/month = **$11.88/year**

Compare to keeping it in STANDARD: $23.00/month = **$276/year**

**Lifetime savings**: Over 10 years, $2,460 (93%).

## Real-World Scenario: Research Lab

A genomics research lab approached us with this situation:

**Current state**:
- 45 TB of data in S3 STANDARD
- Growing at 3 TB/month
- Accessing less than 1% of data per month
- Annual cost: **$12,420**

**Problem**: Budget cuts required reducing storage costs by 80%.

### Solution Strategy

We analyzed their data access patterns and designed a tiered strategy:

```bash
# Step 1: Estimate existing data
cargoship estimate ./historical-data --show-breakdown

# Step 2: Apply lifecycle policy to existing data
cargoship lifecycle \
    --bucket research-data \
    --prefix historical/ \
    --template archive-optimization

# Step 3: Configure new uploads with optimal storage classes
# Active projects → STANDARD (transitions automatically)
cargoship create upload ./active-project \
    --bucket research-data \
    --prefix projects/active \
    --storage-class STANDARD

# Completed projects → GLACIER_IR (immediate)
cargoship create upload ./completed-project \
    --bucket research-data \
    --prefix projects/completed \
    --storage-class GLACIER_IR
```

### Results After 6 Months

| Data Category | Size | Old Cost/Month | New Cost/Month | Savings |
|--------------|------|----------------|----------------|---------|
| Historical (2+ years) | 30 TB | $690 | $30 (DEEP_ARCHIVE) | $660 |
| Completed (3-24 months) | 12 TB | $276 | $48 (GLACIER_IR) | $228 |
| Active (<3 months) | 3 TB | $69 | $69 (STANDARD) | $0 |
| New data (monthly) | 3 TB/mo | $69 | $12 (optimized) | $57 |

**Total**:
- **Old cost**: $1,035/month = **$12,420/year**
- **New cost**: $159/month = **$1,908/year**
- **Savings**: $876/month = **$10,512/year (85%)**

They exceeded their 80% reduction target and freed up $10K for actual research.

## Compression: The Hidden Cost Saver

CargoShip's zstd compression provides 50-90% size reduction depending on data type:

```bash
cargoship estimate ./data --show-compression-analysis
```

Output:
```
Compression Analysis:

File Type Distribution:
├── Text files (logs, CSV, JSON): 400 GB
│   └── Estimated compression: 10:1 (40 GB final)
├── Source code (Python, R): 80 GB
│   └── Estimated compression: 5:1 (16 GB final)
├── Binary data (BAM, FASTQ): 300 GB
│   └── Estimated compression: 3:1 (100 GB final)
└── Images (PNG, TIFF): 220 GB
    └── Estimated compression: 1.2:1 (183 GB final)

Total: 1,000 GB → 339 GB (66% reduction)

Storage cost savings (STANDARD):
- Uncompressed: $23.00/month
- Compressed: $7.80/month
- Savings: $15.20/month ($182/year)
```

For text-heavy data (logs, code, scientific data), compression alone can reduce costs by 80-90%.

### Compression Level Tuning

```bash
# Maximum compression (slower, smallest size)
cargoship create upload ./data \
    --bucket my-bucket \
    --compression-level 19 \
    --storage-class DEEP_ARCHIVE

# Balanced (default)
cargoship create upload ./data \
    --bucket my-bucket \
    --compression-level 9

# Fast compression (faster uploads, larger size)
cargoship create upload ./data \
    --bucket my-bucket \
    --compression-level 3 \
    --storage-class STANDARD
```

**Rule of thumb**:
- Archival data (DEEP_ARCHIVE) → Use level 15-19 (max compression)
- Active data (STANDARD) → Use level 3-6 (fast compression)

## Incremental Sync: Avoid Re-Uploading

CargoShip's incremental sync eliminates redundant transfers:

```bash
# Initial upload (1 TB)
cargoship create upload ./dataset --bucket my-bucket
# Cost: $0.50 (100 PUT requests)

# Change 100 MB, add 50 MB
# Re-upload (traditional tool): 1 TB again
# Re-upload (CargoShip): 150 MB only
cargoship create upload ./dataset --bucket my-bucket
# Cost: $0.001 (2 PUT requests)
```

For daily backups of a 1TB dataset with 1% daily change:
- **Traditional**: 365 TB uploaded/year, $1,825 in PUT requests
- **CargoShip**: 3.65 TB uploaded/year, $18 in PUT requests

**Savings**: $1,807/year (99%)

## Cost Monitoring and Budgets

CargoShip tracks costs in real-time:

```bash
# Set budget alert
cargoship budget set \
    --max-budget 1000 \
    --max-volume-gb 5000 \
    --alert-email ops@example.com

# Check current spend
cargoship cost status
```

Output:
```
💰 Cost Tracking

Current Period (December 2025):
├── Uploads: 3.2 TB ($16.00)
├── Storage: 45 TB ($445.00)
└── Requests: 12,400 PUT ($0.06)

Budget Status:
├── Spend: $461.06 / $1,000 (46%)
└── Volume: 3.2 TB / 5 TB (64%)

Forecast (30-day):
├── Estimated spend: $892
└── Under budget by $108 ✅
```

## Advanced: Multi-Tier Upload Strategy

For complex datasets, use a multi-tier strategy:

```bash
#!/bin/bash

# Tier 1: Critical data (fast access)
cargoship create upload ./critical \
    --bucket production \
    --prefix tier-1-critical \
    --storage-class STANDARD \
    --chunk-size 50MB

# Tier 2: Important data (occasional access)
cargoship create upload ./important \
    --bucket production \
    --prefix tier-2-important \
    --storage-class STANDARD_IA \
    --chunk-size 100MB

# Tier 3: Archive (rare access)
cargoship create upload ./archive \
    --bucket production \
    --prefix tier-3-archive \
    --storage-class GLACIER_IR \
    --chunk-size 250MB

# Tier 4: Compliance (never accessed unless required)
cargoship create upload ./compliance \
    --bucket production \
    --prefix tier-4-compliance \
    --storage-class DEEP_ARCHIVE \
    --chunk-size 500MB \
    --compression-level 19
```

Apply lifecycle policies to automatically demote data:

```bash
# Tier 1 → Tier 2 after 30 days
cargoship lifecycle --bucket production --prefix tier-1-critical \
    --template archive-optimization

# Tier 2 → Tier 3 after 90 days
cargoship lifecycle --bucket production --prefix tier-2-important \
    --custom-transitions "90:GLACIER_IR"
```

## ROI Calculator

Use CargoShip's ROI calculator to estimate savings:

```bash
cargoship cost roi \
    --current-monthly-cost 1200 \
    --data-size-tb 50 \
    --growth-rate-gb-month 3000 \
    --access-pattern rare
```

Output:
```
📊 ROI Analysis

Current State:
- Monthly cost: $1,200
- Annual cost: $14,400
- Data size: 50 TB
- Growth: 3 TB/month

Optimized with CargoShip:
- Estimated monthly cost: $180
- Estimated annual cost: $2,160
- Storage class: GLACIER_IR + DEEP_ARCHIVE
- Compression: 60% reduction

Savings:
- Monthly: $1,020 (85% reduction)
- Annual: $12,240
- 3-year: $36,720

Payback period: Immediate (CargoShip is free)
```

## Conclusion

Cost optimization isn't about compromising on functionality—it's about using the right tools for the right data. CargoShip's combination of:

1. **Request reduction** (99.99% fewer PUT requests)
2. **Intelligent storage class selection**
3. **Automatic compression** (50-90% size reduction)
4. **Lifecycle policies** (automatic tiering)
5. **Incremental sync** (avoid re-uploads)

...delivers typical savings of 85-95% compared to traditional file-by-file uploads.

In our final post, we'll talk about why we chose an open, portable storage format—and how you can build on CargoShip to create your own data management solutions.

---

**Resources**:
- [AWS S3 Pricing](https://aws.amazon.com/s3/pricing/)
- [CargoShip Cost Estimation](../cost-management.md)
- [Storage Class Comparison](https://docs.aws.amazon.com/AmazonS3/latest/userguide/storage-class-intro.html)

**Next**: [Open Format, Open Source: Building on CargoShip →](05-open-format-community.md)
**Previous**: [← Multi-Prefix Deep Dive](03-multi-prefix-deep-dive.md)
