# Scenario 1: Genomics Researcher with Large-Scale Sequencing Data

## Persona: Dr. Maya Rodriguez

**Background**:
- Graduate student in genomics (5th year PhD)
- Analyzes whole-genome sequencing data (WGS) for population studies
- Typical workload: 20-50 human genomes per month (~30-50 GB per genome)
- Primary concern: **Uploading large FASTQ files efficiently without saturating network**
- Technical level: Comfortable with command line, knows NGS tools (BWA, GATK, samtools)
- Works on HPC cluster with 10 Gbps network link to AWS Direct Connect

**Pain Points**:
- FASTQ uploads via `aws s3 cp` are slow (150-200 MB/s) and monopolize network bandwidth
- Lab mates complain when large uploads interfere with interactive work
- Has tried rclone, but still seeing network saturation during uploads
- Needs to know both processing throughput AND actual network usage
- Current solution: Runs uploads late at night to avoid conflicts (inconvenient!)

---

## Version Legend
- ✅ **v0.5.0 (Current)**: Features available today
- 🔄 **v0.6.0+ (Planned)**: Features in development (see linked GitHub issues)

## Current State (v0.5.0): What Works Today

### ✅ Initial Setup (Day 0) - 5-Minute Configuration

Maya already has AWS credentials configured for her lab's S3 buckets.

```bash
# Install CargoShip
brew install scttfrdmn/tap/cargoship

# Verify AWS credentials
aws sts get-caller-identity
# Output:
# {
#   "UserId": "AIDAI...",
#   "Account": "123456789012",
#   "Arn": "arn:aws:iam::123456789012:user/maya.rodriguez"
# }

# Verify CargoShip installation
cargoship --version
# Output: cargoship version 0.5.0
```

**What Maya thinks**: *"Okay, this is just like other CLI tools I use. Simple installation."*

### ✅ Understanding Her Data Profile (Day 0)

Maya's typical genomic datasets:
- **FASTQ files** (raw sequencing reads):
  - Whole-genome: 30-50 GB per genome (paired-end)
  - RNA-seq: 5-15 GB per sample
  - Compression: 15-20% reduction with zstd
  - File count: 2 files per sample (R1, R2)
- **BAM/CRAM files** (aligned reads):
  - 20-40 GB per genome
  - Already compressed (no further reduction)
  - Single file per sample
- **VCF files** (variants):
  - 100 MB - 2 GB per genome
  - Compression: 60-80% reduction with zstd

**Network Environment**:
- HPC cluster: 10 Gbps link = 1,250 MB/s capacity
- Shared with 15 other lab members
- "Fair share" policy: ~70-80 MB/s per user during business hours
- Full bandwidth available 10 PM - 7 AM

### ✅ First Upload: Single Genome (Day 1)

Maya wants to upload a single WGS dataset (2 × 25 GB FASTQ files = 50 GB total).

```bash
# Navigate to data directory
cd /cluster/data/WGS/sample001

# Check files
ls -lh
# Output:
# -rw-r--r-- 1 maya lab  25G Oct 15 14:30 sample001_R1.fastq.gz
# -rw-r--r-- 1 maya lab  25G Oct 15 14:30 sample001_R2.fastq.gz

# Upload to S3 with CargoShip
cargoship upload \
  --source . \
  --bucket genomics-lab-wgs-us-west-2 \
  --prefix cohort-2024/sample001/ \
  --compression zstd \
  --compression-level 3 \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Source:       /cluster/data/WGS/sample001
#   Bucket:       genomics-lab-wgs-us-west-2
#   Prefix:       cohort-2024/sample001/
#   Compression:  zstd (level 3)
#   Shards:       10 (auto-selected based on data size)
#
# 📊 Scanning files...
# ✅ Found 2 files (50.0 GB uncompressed)
#
# ⚙️  Pipeline Configuration:
#   Chunk size:       200 MB (adaptive)
#   S3 upload workers: 8 (parallel)
#   Memory limit:     4 GB (bounded)
#
# 🔄 Processing pipeline started...
#
# [Progress bar showing real-time stats]
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files uploaded:           2
#   Data processed:           50.0 GB (uncompressed)
#   Data uploaded:            42.5 GB (compressed, zstd-3)
#   Compression ratio:        15% reduction
#   Duration:                 6m 45s (405 seconds)
#
#   📊 Throughput:
#     Processing (effective):  126.5 MB/s (uncompressed data rate)
#     Network (actual):        107.5 MB/s (compressed data uploaded)
#     Network utilization:     8.6% of 10 Gbps link (1,250 MB/s)
#
#   💾 Memory Usage:
#     Peak:                    3.8 GB
#     Average:                 3.2 GB
#
#   ☁️  S3 Statistics:
#     Shards:                  10
#     Total API calls:         1,347 (multipart)
#     Failed parts (retried):  0
#
# 🔗 S3 Location:
#   s3://genomics-lab-wgs-us-west-2/cohort-2024/sample001/
```

**What Maya thinks**: *"Wow! 107 MB/s actual network usage - that's way better than my old 150-200 MB/s, and it finished in under 7 minutes! I can see BOTH the processing throughput (126 MB/s uncompressed) AND the actual network usage (107 MB/s). Only using 8.6% of the network link - my lab mates won't even notice!"*

**Technical Details**:
- **Processing throughput** (126.5 MB/s): Rate at which CargoShip reads and compresses the FASTQ data
- **Network throughput** (107.5 MB/s): Rate at which compressed data is uploaded to S3
- **Compression benefit**: 15% reduction (50 GB → 42.5 GB) saves 7.5 GB of network transfer
- **Network impact**: Only 8.6% of 10 Gbps link (friendly to lab mates!)
- **Memory efficiency**: 3.8 GB peak for 50 GB dataset (7.6% ratio)

### ✅ Bulk Upload: Full Cohort (Day 2)

Maya needs to upload an entire cohort of 25 genomes (1.25 TB total).

```bash
# Navigate to cohort directory
cd /cluster/data/WGS/cohort-2024

# Directory structure:
# cohort-2024/
# ├── sample001/
# │   ├── sample001_R1.fastq.gz (25 GB)
# │   └── sample001_R2.fastq.gz (25 GB)
# ├── sample002/
# │   ├── sample002_R1.fastq.gz (25 GB)
# │   └── sample002_R2.fastq.gz (25 GB)
# ... (25 samples total)

# Count files and size
find . -name "*.fastq.gz" | wc -l
# Output: 50 files (2 per sample × 25 samples)

du -sh .
# Output: 1.25 TB

# Upload entire cohort
cargoship upload \
  --source . \
  --bucket genomics-lab-wgs-us-west-2 \
  --prefix cohort-2024/ \
  --compression zstd \
  --compression-level 3 \
  --parallel-prefix \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Source:       /cluster/data/WGS/cohort-2024
#   Bucket:       genomics-lab-wgs-us-west-2
#   Prefix:       cohort-2024/
#   Compression:  zstd (level 3)
#   Shards:       10 (auto-selected)
#   Mode:         Multi-prefix parallel upload (8× S3 request rate)
#
# 📊 Scanning files...
# ✅ Found 50 files in 25 directories (1.25 TB uncompressed)
#
# ⚙️  Pipeline Configuration:
#   Chunk size:       200 MB (adaptive)
#   S3 upload workers: 8 (parallel per shard)
#   Memory limit:     4 GB (bounded)
#   File splitting:   Enabled (large files split across chunks)
#
# 🔄 Processing pipeline started...
#
# [Progress bar with real-time throughput]
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files uploaded:           50 (25 samples × 2 files)
#   Data processed:           1.25 TB (uncompressed)
#   Data uploaded:            1.06 TB (compressed, zstd-3)
#   Compression ratio:        15% reduction
#   Duration:                 2h 48m 32s (10,112 seconds)
#
#   📊 Throughput:
#     Processing (effective):  130.2 MB/s (uncompressed data rate)
#     Network (actual):        110.4 MB/s (compressed data uploaded)
#     Network utilization:     8.8% of 10 Gbps link (1,250 MB/s)
#
#   💾 Memory Usage:
#     Peak:                    4.1 GB
#     Average:                 3.6 GB
#
#   ☁️  S3 Statistics:
#     Shards:                  10 (multi-prefix: sample001/ through sample025/)
#     Total API calls:         33,680 (multipart)
#     Failed parts (retried):  3 (auto-recovered)
#     S3 request rate:         8× baseline (multi-prefix optimization)
#
# 🔗 S3 Location:
#   s3://genomics-lab-wgs-us-west-2/cohort-2024/
#
# 💡 Tip: Use --show-compression-stats to see per-file compression details
```

**What Maya thinks**: *"2 hours 48 minutes for 1.25 TB! That's amazing! And I can do this during the day because it's only using 8.8% of the network. My old method would have taken 6+ hours and required a late-night session. The multi-prefix optimization is automatically distributing uploads across S3 paths for 8× request rate - no S3 throttling!"*

**Technical Analysis**:
- **Actual upload time**: 2h 48m (vs 6+ hours with aws s3 cp)
- **Network-friendly**: 110 MB/s sustained rate (only 8.8% of 10 Gbps)
- **Compression savings**: 190 GB saved (1.25 TB → 1.06 TB)
- **Cost savings**: 190 GB less data transfer = $17.10 saved (at $0.09/GB egress)
- **Memory efficiency**: 4.1 GB peak for 1.25 TB dataset (0.3% ratio)
- **S3 optimization**: Multi-prefix sharding avoids S3 rate limits (8× request rate)

### ✅ Monitoring During Upload (Real-Time Visibility)

While the upload runs, Maya can monitor progress:

```bash
# In another terminal session
cargoship status

# Output (updates every 5 seconds):
# 🚀 CargoShip Upload Status
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📊 Progress: 37.5% (18/50 files, 468.8 GB / 1.25 TB)
# ⏱️  Elapsed: 1h 02m 18s | ETA: 1h 44m 22s
#
# 📈 Current Throughput:
#   Processing: 128.5 MB/s (uncompressed)
#   Network:    109.2 MB/s (compressed)
#   Network:    8.7% of 10 Gbps link 🟢
#
# 💾 Memory: 3.9 GB / 4.0 GB limit (97%)
#
# 🔧 Active Shards: 10 / 10
#   ├─ shard-0: sample003_R1.fastq.gz (uploading part 48/127)
#   ├─ shard-1: sample007_R2.fastq.gz (uploading part 62/127)
#   ├─ shard-2: sample011_R1.fastq.gz (uploading part 35/127)
#   ├─ shard-3: sample014_R2.fastq.gz (uploading part 89/127)
#   ├─ shard-4: sample018_R1.fastq.gz (uploading part 12/127)
#   ├─ shard-5: sample019_R2.fastq.gz (uploading part 101/127)
#   ├─ shard-6: sample021_R1.fastq.gz (uploading part 56/127)
#   ├─ shard-7: sample024_R2.fastq.gz (uploading part 73/127)
#   ├─ shard-8: sample002_R1.fastq.gz (completed ✅)
#   └─ shard-9: sample006_R2.fastq.gz (uploading part 44/127)
#
# ⚠️  Errors: 0 (auto-retry enabled)
#
# Press Ctrl+C to view detailed stats (upload continues in background)
```

**What Maya observes**:
- Real-time throughput showing BOTH processing and network rates
- Network utilization stays under 10% (lab-friendly!)
- All 10 shards actively uploading different files in parallel
- Each large FASTQ file split into ~127 parts (200 MB chunks)
- Memory usage bounded at 4 GB (predictable and safe)
- ETA gives realistic estimate based on current throughput

### ✅ Compression Statistics (Detailed Analysis)

Maya wants to understand compression performance for different file types:

```bash
# After upload completes, view compression details
cargoship stats --show-compression

# Output:
# 📊 Compression Performance Analysis
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# Algorithm: zstd (level 3)
#
# By File Type:
#
# FASTQ Files (.fastq.gz):
#   Files:              50
#   Uncompressed:       1.25 TB
#   Compressed:         1.06 TB
#   Reduction:          15.2% (190 GB saved)
#   Avg speed:          527 MB/s (zstd compression)
#   Network impact:     Saved 30 minutes of transfer time
#
# 💡 Analysis:
#   FASTQ files are already gzip-compressed (.gz extension)
#   zstd-3 achieves 15% additional compression (good!)
#   Trade-off: Slight CPU overhead for 15% network savings
#   Recommendation: ✅ Keep zstd-3 for FASTQ uploads
#
# 💰 Cost Savings:
#   Data transfer saved:     190 GB
#   S3 egress cost savings:  $17.10 (at $0.09/GB)
#   S3 storage savings:      $4.37/month (at $0.023/GB-month)
#   Total first-year value:  $69.54
```

**What Maya thinks**: *"Even though my FASTQ files are already gzip-compressed, zstd-3 is squeezing out another 15%! That's real money saved on data transfer and storage. The compression speed (527 MB/s) is way faster than my disk can read, so no bottleneck there."*

### ✅ Comparing with BAM Files (Day 3)

Maya uploads aligned BAM files (already highly compressed):

```bash
cd /cluster/data/aligned/cohort-2024

# BAM files are already compressed (no .gz extension needed)
ls -lh sample001.bam
# Output: -rw-r--r-- 1 maya lab  32G Oct 16 09:15 sample001.bam

cargoship upload \
  --source . \
  --bucket genomics-lab-wgs-us-west-2 \
  --prefix aligned/cohort-2024/ \
  --compression zstd \
  --compression-level 3 \
  --verbose

# Output:
# ... [similar output as before]
#
# 📈 Performance Metrics:
#   Files uploaded:           25 (BAM files)
#   Data processed:           800 GB (uncompressed size)
#   Data uploaded:            795 GB (compressed, zstd-3)
#   Compression ratio:        0.6% reduction
#   Duration:                 2h 01m 15s
#
#   📊 Throughput:
#     Processing (effective):  110.2 MB/s
#     Network (actual):        109.5 MB/s
#     Network utilization:     8.8% of 10 Gbps link
#
# 💡 Analysis:
#   BAM files are already highly compressed (BGZF format)
#   zstd-3 only achieves 0.6% additional compression
#   Recommendation: Consider --no-compression for BAM files to save CPU
```

**What Maya learns**: *"Okay, BAM files don't compress further. Next time I'll use --no-compression for BAM files to save CPU cycles. But it's good that CargoShip is smart enough to still upload efficiently even when compression doesn't help much."*

### ✅ Network-Aware Upload Scheduling (Advanced)

Maya wants to schedule large uploads for off-peak hours:

```bash
# 🔄 Planned for v0.6.0 (not yet implemented)
#
# cargoship upload \
#   --source /cluster/data/WGS/cohort-2025 \
#   --bucket genomics-lab-wgs-us-west-2 \
#   --prefix cohort-2025/ \
#   --compression zstd \
#   --schedule "22:00-07:00" \
#   --target-bandwidth 800 \
#   --verbose
#
# Output:
# 🕐 Upload scheduled for 22:00 (in 6h 15m)
# 📊 Estimated duration: 1h 23m (at 800 MB/s target)
# 💡 Upload will automatically pause at 07:00 and resume at 22:00
```

**Future capability**: CargoShip will support scheduled uploads with bandwidth targets for optimal network sharing.

## Data Type Performance Characteristics

### FASTQ Files (Raw Sequencing Reads)
```
📊 Typical Profile:
  File size:           10-50 GB per file (paired-end)
  Format:              .fastq.gz (gzip-compressed text)
  Compression (zstd-3): 15-20% additional reduction
  Upload throughput:   110-130 MB/s (network, after compression)
  Processing:          527 MB/s (zstd compression speed)
  Memory:              3-4 GB peak (bounded)
  Bottleneck:          Network (not CPU or memory)

💡 Recommendations:
  ✅ Always use zstd-3 compression (15-20% savings)
  ✅ Multi-prefix mode for bulk uploads (8× S3 request rate)
  ✅ Safe to run during business hours (8-9% network utilization)
  ⚠️  5 Gbps networks may become bottleneck (625 MB/s capacity)
```

### BAM/CRAM Files (Aligned Reads)
```
📊 Typical Profile:
  File size:           20-50 GB per file
  Format:              .bam (BGZF-compressed binary)
  Compression (zstd-3): 0-5% additional reduction (minimal)
  Upload throughput:   110-115 MB/s (network, minimal compression gain)
  Processing:          600+ MB/s (fast-path for pre-compressed data)
  Memory:              3-4 GB peak (bounded)
  Bottleneck:          Network (compression provides little benefit)

💡 Recommendations:
  ⚠️  Consider --no-compression to save CPU (minimal gain)
  ✅ Still use multi-prefix mode for bulk uploads
  ✅ Safe to run during business hours (8-9% network utilization)
  💡 CRAM files (reference-based compression) show 0% additional reduction
```

### VCF Files (Variant Calls)
```
📊 Typical Profile:
  File size:           100 MB - 2 GB per file
  Format:              .vcf or .vcf.gz (text or gzip-compressed)
  Compression (zstd-3): 60-80% reduction for .vcf, 10-15% for .vcf.gz
  Upload throughput:   20-60 MB/s (network, varies with compression ratio)
  Processing:          400-527 MB/s (zstd compression speed)
  Memory:              1-2 GB peak (smaller files)
  Bottleneck:          Compression CPU (for uncompressed .vcf)

💡 Recommendations:
  ✅ Always use zstd-3 compression (huge savings for .vcf)
  ✅ Consider higher compression levels (zstd-9) for archival
  ✅ Network-friendly (low bandwidth usage)
  💡 .vcf.gz files still benefit from 10-15% additional compression
```

## Real-World Scenarios

### Scenario A: Daily Upload (Single Sample)
```
Data:      1 × WGS sample (2 × 25 GB FASTQ.gz) = 50 GB
Duration:  6m 45s
Network:   107 MB/s (8.6% of 10 Gbps link)
Cost:      $0.45 transfer + $1.15/month storage = $1.60 first year
Impact:    Lab-friendly (can run during business hours)
```

### Scenario B: Weekly Cohort Upload (25 Samples)
```
Data:      25 × WGS samples (50 × 25 GB FASTQ.gz) = 1.25 TB
Duration:  2h 48m
Network:   110 MB/s (8.8% of 10 Gbps link)
Cost:      $11.25 transfer + $28.75/month storage = $40.00 first year
Impact:    Lab-friendly (can run during business hours)
Savings:   $17.10 data transfer (15% compression)
```

### Scenario C: Monthly Archive (100 Samples)
```
Data:      100 × WGS samples (200 × 25 GB FASTQ.gz) = 5 TB
Duration:  11h 14m
Network:   110 MB/s (8.8% of 10 Gbps link)
Cost:      $45.00 transfer + $115/month storage = $160 first year
Impact:    Requires coordination (multi-day upload window)
Savings:   $68.40 data transfer (15% compression)
```

## Network Capacity Planning

### 10 Gbps Network (Maya's Current Setup)
```
Capacity:         1,250 MB/s (theoretical maximum)
CargoShip usage:  110 MB/s (8.8% utilization)
Available:        1,140 MB/s for other lab members
Verdict:          ✅ Excellent - no impact on lab operations
```

### 5 Gbps Network (Bottleneck Scenario)
```
Capacity:         625 MB/s (theoretical maximum)
CargoShip usage:  110 MB/s (17.6% utilization)
Available:        515 MB/s for other lab members
Verdict:          ⚠️  Good - minor impact during large uploads
Recommendation:   Schedule large uploads for off-peak hours
```

### 1 Gbps Network (Severe Bottleneck)
```
Capacity:         125 MB/s (theoretical maximum)
CargoShip usage:  110 MB/s (88% utilization)
Available:        15 MB/s for other lab members
Verdict:          ❌ Poor - significant impact on lab operations
Recommendation:   Use AWS Direct Connect or schedule uploads late night
```

## Troubleshooting Common Issues

### Issue 1: Slower Than Expected Upload

**Symptom**:
```bash
cargoship upload ... --verbose
# Output shows:
#   Network (actual): 45 MB/s (expected ~110 MB/s)
```

**Diagnosis**:
```bash
# Check network utilization
cargoship status
# Output shows:
#   Network: 45 MB/s (36% of 1 Gbps link)

# Conclusion: Network is the bottleneck (1 Gbps = 125 MB/s max)
```

**Solutions**:
1. **Upgrade network link**: Contact IT about AWS Direct Connect (10 Gbps)
2. **Schedule uploads**: Use off-peak hours when network is less congested
3. **Increase compression**: Higher zstd levels reduce network usage (but slower)

### Issue 2: High Memory Usage Warning

**Symptom**:
```bash
cargoship upload ... --verbose
# Output shows:
#   💾 Memory: 7.2 GB / 4.0 GB limit (180%) ⚠️
```

**Diagnosis**:
```bash
# Check system memory
free -h
# Output shows:
#   Available: 64 GB (plenty of RAM)

# Conclusion: CargoShip memory limit is too conservative
```

**Solutions**:
```bash
# Increase memory limit
cargoship upload ... --memory-limit 8G

# Or disable limit (use available system memory)
cargoship upload ... --memory-limit unlimited
```

### Issue 3: S3 Rate Limiting Errors

**Symptom**:
```bash
cargoship upload ... --verbose
# Output shows:
#   ⚠️  S3 throttling detected (SlowDown errors)
#   Failed parts (retried): 247
```

**Diagnosis**:
```bash
# Check S3 prefix structure
aws s3 ls s3://bucket/prefix/ | head -20
# Output shows:
#   All files in single prefix: cohort-2024/

# Conclusion: S3 rate limiting (single prefix = 3,500 req/sec limit)
```

**Solutions**:
```bash
# Enable multi-prefix mode (automatic distribution)
cargoship upload ... --parallel-prefix

# This distributes uploads across multiple S3 prefixes:
#   cohort-2024/sample001/
#   cohort-2024/sample002/
#   ... (8× request rate capacity)
```

## Best Practices

### 1. **Always Use Multi-Prefix Mode for Bulk Uploads**
```bash
# ✅ Good: Distribute across prefixes
cargoship upload ... --parallel-prefix

# ❌ Bad: Single prefix (risk of S3 throttling)
cargoship upload ... --prefix all-samples/
```

### 2. **Choose Compression Based on File Type**
```bash
# FASTQ files (15-20% gain)
cargoship upload ... --compression zstd --compression-level 3

# BAM/CRAM files (minimal gain, skip compression)
cargoship upload ... --no-compression

# VCF files (60-80% gain for .vcf, 10-15% for .vcf.gz)
cargoship upload ... --compression zstd --compression-level 3
```

### 3. **Monitor Network Utilization**
```bash
# Check real-time throughput
cargoship status

# Look for network utilization indicator:
#   Network: 110 MB/s (8.8% of 10 Gbps link) 🟢 Good!
#   Network: 110 MB/s (88% of 1 Gbps link)  ⚠️  High!
```

### 4. **Use Bounded Memory Limits**
```bash
# Set conservative limit for shared HPC systems
cargoship upload ... --memory-limit 4G

# Use higher limit for dedicated upload nodes
cargoship upload ... --memory-limit 8G
```

### 5. **Schedule Large Uploads for Off-Peak Hours**
```bash
# 🔄 Planned for v0.6.0
# cargoship upload ... --schedule "22:00-07:00"
```

## Performance Summary

### What Maya Achieved

**Before CargoShip** (aws s3 cp with rclone):
- Upload speed: 150-200 MB/s (uncompressed)
- Network saturation: 12-16% of 10 Gbps link
- Lab impact: Complaints from lab mates during uploads
- Scheduling: Late-night uploads required (inconvenient)
- Transparency: No visibility into actual network usage
- Cohort upload (1.25 TB): 6+ hours

**After CargoShip** (v0.5.0):
- Upload speed: 110 MB/s (compressed network traffic)
- Network utilization: 8.8% of 10 Gbps link (lab-friendly!)
- Lab impact: Zero complaints (below threshold)
- Scheduling: Daytime uploads possible
- Transparency: Real-time processing + network throughput metrics
- Cohort upload (1.25 TB): 2h 48m (2.5× faster)
- Cost savings: $17.10/cohort in data transfer (15% compression)

**Key Benefits**:
1. ✅ **2.5× faster uploads** (6h → 2h 48m for 1.25 TB cohort)
2. ✅ **Lab-friendly** (8.8% network utilization vs 12-16%)
3. ✅ **Cost savings** ($17/cohort in transfer, $4.37/month in storage)
4. ✅ **Transparency** (see both processing and network throughput)
5. ✅ **Predictable memory** (4 GB bounded, safe for shared HPC)
6. ✅ **Automatic optimization** (multi-prefix sharding, file splitting)
7. ✅ **Zero S3 throttling** (8× request rate with multi-prefix mode)

## Next Steps for Maya

### Immediate (v0.5.0 - Available Today)
- ✅ Use CargoShip for all FASTQ uploads (15-20% compression savings)
- ✅ Enable multi-prefix mode for bulk cohort uploads (8× S3 request rate)
- ✅ Monitor network utilization to stay lab-friendly
- ✅ Share CargoShip with lab mates (encourage adoption)

### Coming Soon (v0.6.0+)
- 🔄 **Scheduled uploads** (--schedule flag) for automated off-peak transfers
- 🔄 **Intelligent shard count** (dynamic based on workload size)
- 🔄 **Compression presets** (--preset genomics for optimal FASTQ settings)
- 🔄 **Progress notifications** (Slack/email alerts when upload completes)

**What Maya thinks**: *"CargoShip is a game-changer for genomics data uploads. I can finally upload large cohorts during the day without annoying my lab mates, and the dual throughput reporting helps me understand exactly what's happening with my network. The 2.5× speedup and cost savings are just icing on the cake!"*
