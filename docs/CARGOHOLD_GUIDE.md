# CargoHold User Guide

**Version:** 1.0
**Last Updated:** 2025-12-12
**Applies to:** CargoShip v0.6.0+

## Table of Contents

1. [Introduction](#introduction)
2. [Architecture](#architecture)
3. [Getting Started](#getting-started)
4. [CLI Reference](#cli-reference)
5. [Performance](#performance)
6. [Best Practices](#best-practices)
7. [Troubleshooting](#troubleshooting)
8. [Migration](#migration)

---

## Introduction

### What is CargoHold?

**CargoHold** is CargoShip's high-performance sharded archive system for storing large datasets on AWS S3. Unlike traditional archive tools that create monolithic files, CargoHold distributes your data across multiple parallel shards, enabling:

- **8x faster uploads** through parallel S3 prefix sharding
- **10x faster selective extraction** by downloading only needed chunks
- **Zero local disk usage** with direct streaming to S3
- **Efficient compression** using Zstandard (2.5:1 typical ratio)
- **Manifest-driven operations** with O(1) file lookups

### Key Features

| Feature | Benefit |
|---------|---------|
| **Parallel Sharding** | 8 S3 prefixes × 3,500 req/s = 28,000 req/s aggregate throughput |
| **Streaming Architecture** | Zero local disk usage, bounded memory (O(chunk_size × workers)) |
| **Smart Chunking** | Adaptive 200MB chunks balance upload size and parallelism |
| **Fast Compression** | Zstandard at 500+ MB/s compression throughput |
| **Selective Extraction** | Download only the files you need (10x faster than full restore) |
| **Manifest Index** | O(1) file lookups, <10MB memory for 1M files |

### When to Use CargoHold

**Ideal For:**
- Large datasets (GB to TB scale)
- Many small files (1,000+ files)
- Frequent selective extractions
- High-throughput uploads
- Long-term S3 archival

**Not Ideal For:**
- Single large files (use multipart upload directly)
- Frequent random access to individual files (consider uncompressed storage)
- Real-time streaming workloads

---

## Architecture

### Storage Structure

CargoHold organizes data using a **sharded archive** architecture:

```
s3://bucket/[prefix]/
└── uploads/
    └── {upload-id}/                    ← Unique upload session
        ├── shard-0/                    ← S3 prefix shard (parallel uploads)
        │   ├── chunk-0.tar.zst         ← Compressed TAR archive (~200MB)
        │   ├── chunk-8.tar.zst
        │   └── ...
        ├── shard-1/
        │   ├── chunk-1.tar.zst
        │   └── ...
        ├── shard-2/
        ├── ... (shards 3-7)
        └── manifest.json.gz            ← Lightweight file index (~30KB for 10k files)
```

### Components

#### 1. Shards

**Shards** are S3 prefix partitions that enable parallel uploads:

- **Default Count:** 8 shards (configurable with `--shards` flag)
- **Distribution:** Round-robin assignment (chunk N → shard N mod 8)
- **Performance:** Each shard is an independent S3 prefix with 3,500 req/s limit
- **Aggregate Throughput:** 8 shards × 3,500 req/s = 28,000 req/s

#### 2. Chunks

**Chunks** are compressed TAR archives containing batches of files:

- **Format:** TAR + Zstandard (`.tar.zst`)
- **Target Size:** 200MB uncompressed (adaptive based on file sizes)
- **Compression:** Zstandard level 3 (fast, ~2.5:1 ratio)
- **Contents:** Multiple files per chunk, preserving directory structure

#### 3. Manifest

The **manifest** is a JSON index of all files and their locations:

- **Format:** JSON + gzip compression (manifest.json.gz)
- **Size:** ~30KB compressed for 10,000 files
- **Purpose:** Enable fast file lookups and selective extraction
- **Index:** O(1) hash map lookup by file path

### Upload ID Format

Each upload session has a unique identifier:

```
Format: YYYYMMDD-HHmmss-{random}
Example: 20251212-143022-a7f3b9c2
```

- **Timestamp:** Local time when upload started
- **Random:** 8-character hex string for uniqueness
- **Use:** Identifier for all operations (download, list, info)

### Archive Format Details

#### TAR Structure

```
chunk-0.tar.zst
└── [Zstandard compressed stream]
    └── [TAR archive - POSIX ustar format]
        ├── data/file1.txt          (relative paths preserved)
        ├── logs/app.log
        ├── reports/summary.csv
        └── ...
```

**Preserved Attributes:**
- File path (relative to source directory)
- File size (exact bytes)
- Modification time (nanosecond precision)
- File mode/permissions (Unix-style)

**Not Preserved:**
- Ownership (UID/GID)
- Extended attributes (xattrs)
- Access Control Lists (ACLs)
- Sparse file information

#### Compression

**Algorithm:** Zstandard (zstd)

| Aspect | Details |
|--------|---------|
| Level | 3 (SpeedDefault) - balances speed and compression |
| Throughput | 500+ MB/s compression (multi-threaded) |
| Ratio | 2.5:1 typical (varies by content type) |
| Compatibility | Wide tool support (zstd CLI, libraries) |

**Compression by Content Type:**

| Content Type | Typical Ratio | Notes |
|--------------|---------------|-------|
| Text/Logs | 3-5:1 | Highly compressible |
| Source Code | 3-4:1 | Good compression |
| JSON/CSV | 2.5-3:1 | Structured data |
| Images (JPEG) | 1.1:1 | Already compressed |
| Binary/Encrypted | 1.0:1 | Not compressible |

---

## Getting Started

### Prerequisites

1. **AWS Credentials:** Configured via `aws configure` or environment variables
2. **S3 Bucket:** Target bucket with write permissions
3. **CargoShip:** Installed and in PATH (`cargoship --version`)

### Quick Start: Upload Your First Dataset

#### 1. Upload a Directory

```bash
cargoship create upload /path/to/data --bucket my-bucket
```

This command:
- Scans the directory
- Creates compressed chunks (~200MB each)
- Uploads to 8 parallel S3 shards
- Generates a manifest file
- Shows real-time progress in TUI

**Output:**
```
🚢 CargoShip Upload Progress
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Upload ID: 20251212-143022-a7f3b9c2

  Scanner    ████████████████████████████ 100% | 10,000 files
  Archiver   ████████████████████████████ 100% | 50 chunks
  Uploader   ████████████████████████████ 100% | 5.2 GB

  Throughput: 245 MB/s
  Duration: 21s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Upload complete!
   Manifest: s3://my-bucket/uploads/20251212-143022-a7f3b9c2/manifest.json.gz
```

#### 2. List Files in Upload

```bash
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2
```

**Output:**
```
📦 Files in upload 20251212-143022-a7f3b9c2

  data/file1.txt                  1.2 MB
  data/file2.txt                  2.5 MB
  logs/app.log                    512 KB
  reports/summary.csv             128 KB
  ...

  Total: 10,000 files | 5.2 GB uncompressed | 2.1 GB compressed (2.5:1)
```

#### 3. Get Upload Information

```bash
cargoship info s3://my-bucket/uploads/20251212-143022-a7f3b9c2
```

**Output:**
```
📊 Upload Information

  Upload ID:        20251212-143022-a7f3b9c2
  Created:          2025-12-12 14:30:22
  Completed:        2025-12-12 14:30:43
  Duration:         21 seconds

  Source:           /path/to/data
  Hostname:         upload-server-01

  Files:            10,000 files
  Total Size:       5.2 GB (uncompressed)
  Compressed:       2.1 GB (2.5:1 ratio)

  Chunks:           50 chunks (avg 200 MB each)
  Shards:           8 shards

  Compression:      zstd level 3
  Storage:          s3://my-bucket/uploads/20251212-143022-a7f3b9c2
  Region:           us-west-2
```

#### 4. Download Files

**Full Download:**
```bash
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./restored
```

**Selective Download (10x faster):**
```bash
# Download only log files
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./logs \
  --pattern "*.log"

# Download specific files
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./reports \
  --files "reports/summary.csv,reports/details.csv"

# Download specific shards
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./restored \
  --shard-ids 0,2,4
```

---

## CLI Reference

### Upload Command

**Usage:**
```bash
cargoship create upload SOURCE_DIR... [flags]
```

**Key Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--bucket` | (required) | S3 bucket name |
| `--prefix` | "" | S3 key prefix (optional) |
| `--region` | us-west-2 | AWS region |
| `--shards` | 8 | Number of S3 prefix shards |
| `--chunk-size-mb` | 200 | Target chunk size (0 = adaptive) |
| `--workers` | 4 | Workers per stage (scanner, archiver, uploader) |
| `--storage-class` | STANDARD | S3 storage class |
| `--progress-format` | tui | Output format: tui, json, text |
| `--quiet` | false | Disable progress display |
| `--resume` | false | Resume incomplete upload |
| `--http2` | true | Enable HTTP/2 |

**Examples:**

```bash
# Basic upload
cargoship create upload /data --bucket my-bucket

# Custom prefix and storage class
cargoship create upload /data \
  --bucket my-bucket \
  --prefix backups/2025-12 \
  --storage-class GLACIER_IR

# High-performance upload (more shards and workers)
cargoship create upload /data \
  --bucket my-bucket \
  --shards 16 \
  --workers 8

# Resume interrupted upload
cargoship create upload /data \
  --bucket my-bucket \
  --resume \
  --upload-id 20251212-143022-a7f3b9c2

# JSON output for scripting
cargoship create upload /data \
  --bucket my-bucket \
  --progress-format json
```

### Download Command

**Usage:**
```bash
cargoship download S3_URL OUTPUT_DIR [flags]
```

**Key Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--pattern` | "" | Glob pattern (e.g., "*.log") |
| `--files` | [] | Comma-separated file paths |
| `--shard-ids` | [] | Specific shard IDs (0-7) |
| `--workers` | 4 | Parallel download workers |
| `--dry-run` | false | Show what would be downloaded |
| `--verbose` | false | List each file as extracted |

**Examples:**

```bash
# Full download
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./restored

# Pattern matching
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./logs \
  --pattern "*.log"

cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./data \
  --pattern "data/*.csv"

# Specific files
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./reports \
  --files "report.csv,summary.txt,data/results.json"

# Specific shards only
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./restored \
  --shard-ids 0,1,2

# Dry run to preview
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./test \
  --pattern "*.csv" \
  --dry-run
```

### List Command

**Usage:**
```bash
cargoship list --bucket BUCKET --upload-id UPLOAD_ID [flags]
```

**Key Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-b, --bucket` | (required) | S3 bucket name |
| `-u, --upload-id` | (required) | Upload ID to query |
| `-p, --prefix` | "" | S3 prefix |
| `-r, --region` | us-west-2 | AWS region |
| `--pattern` | "" | Filter by glob pattern |
| `--verbose` | false | Show full file details |

**Examples:**

```bash
# List all files
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2

# Filter by pattern
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2 \
  --pattern "*.log"

# Verbose output
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2 \
  --verbose
```

### Info Command

**Usage:**
```bash
cargoship info S3_URL [flags]
cargoship info --bucket BUCKET --upload-id UPLOAD_ID [flags]
```

**Key Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | Output as JSON |
| `--verbose` | false | Show per-shard statistics |

**Examples:**

```bash
# Using S3 URL
cargoship info s3://my-bucket/uploads/20251212-143022-a7f3b9c2

# Using flags
cargoship info \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2

# JSON output for scripting
cargoship info \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  --json

# Detailed per-shard stats
cargoship info \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  --verbose
```

---

## Performance

### Upload Performance

#### Throughput

**Typical Performance:**

| Dataset Type | File Count | Size | Throughput | Duration |
|--------------|-----------|------|------------|----------|
| Small Files | 10,000 | 5 GB | 200-250 MB/s | ~20s |
| Medium Files | 1,000 | 50 GB | 300-400 MB/s | ~2min |
| Large Files | 100 | 500 GB | 400-500 MB/s | ~17min |
| Mixed Workload | 10,000 | 100 GB | 250-350 MB/s | ~5min |

**Factors Affecting Throughput:**
- **Network bandwidth** (primary bottleneck above 500 MB/s)
- **CPU cores** (compression scales with cores)
- **File count** (overhead for many small files)
- **Content type** (compression ratio varies)
- **Shard count** (more shards = more parallelism)

#### Comparison vs Traditional Tools

Based on benchmarks from Issue #110:

| Tool | Throughput | Notes |
|------|-----------|-------|
| **CargoHold** | **245 MB/s** | 8 parallel shards, streaming |
| tar + zstd + aws s3 cp | 120 MB/s | Single-threaded upload |
| aws s3 sync | 80-100 MB/s | Many small API calls |
| rclone | 150-180 MB/s | Better than aws-cli |
| s5cmd | 200-220 MB/s | Fastest alternative |

**CargoHold Advantages:**
- **2-3x faster** than traditional tar+upload approach
- **8x throughput** improvement from multi-prefix sharding
- **Zero local disk** usage (streaming architecture)
- **Automatic compression** with optimal settings

### Download Performance

#### Selective Extraction

**Performance Comparison:**

| Operation | Files Requested | Chunks Downloaded | Time | Speedup |
|-----------|----------------|-------------------|------|---------|
| Full Download | 10,000 (100%) | 50 (100%) | 30 min | 1x |
| Selective (10%) | 1,000 (10%) | 12 (24%) | 7 min | **4.3x faster** |
| Selective (1%) | 100 (1%) | 3 (6%) | 2 min | **15x faster** |
| Single File | 1 (0.01%) | 1 (2%) | 30s | **60x faster** |

**Key Insight:** Selective extraction is dramatically faster because:
1. Manifest download is instant (~30KB)
2. Only chunks containing requested files are downloaded
3. Decompression is only done for needed chunks

#### Full Restore Performance

| Dataset | Size | Chunks | Download Time | Extract Time | Total |
|---------|------|--------|---------------|--------------|-------|
| 10k files | 5 GB | 25 | 20s | 15s | 35s |
| 100k files | 50 GB | 250 | 3min | 2min | 5min |
| 1M files | 500 GB | 2,500 | 25min | 15min | 40min |

### Memory Usage

**Bounded Memory Model:**

```
Memory Usage = chunk_size × workers × stages
             = 200 MB × 4 workers × 3 stages
             = 2.4 GB (typical)
```

**Memory by Operation:**

| Operation | Typical Memory | Peak Memory | Notes |
|-----------|---------------|-------------|-------|
| Upload | 2-3 GB | 4 GB | Bounded by chunk size |
| Download | 1-2 GB | 3 GB | Streaming decompression |
| List | <100 MB | <200 MB | Only manifest in memory |
| Info | <100 MB | <200 MB | Only manifest in memory |

**Manifest Index:**
- **1M files:** <10 MB memory (O(1) hash map)
- **10M files:** <100 MB memory
- **Build time:** <100ms for 1M files

### S3 Request Rates

**Upload:**
```
Requests = chunk_count × (1 PutObject + metadata checks)
         ≈ chunk_count

Example: 50 chunks = 50 PutObject requests
```

**Download:**
```
Requests = 1 (manifest) + chunks_with_requested_files × GetObject

Full restore: 1 + 50 = 51 requests
Selective (10%): 1 + 12 = 13 requests (75% reduction)
```

**Cost Implications:**
- **Fewer requests** = lower costs
- **Larger chunks** = fewer requests but higher transfer costs
- **200MB default** balances requests vs transfer

---

## Best Practices

### Shard Count Selection

**Default: 8 shards (recommended for most workloads)**

| Shard Count | Best For | Throughput | Complexity |
|-------------|----------|------------|------------|
| 1 | Testing, small datasets | Limited | Low |
| 4 | Small to medium datasets | Good | Low |
| **8** | **Most production workloads** | **Excellent** | **Medium** |
| 16 | Very large datasets, high bandwidth | Maximum | High |
| 32+ | Extreme scale (rare) | Extreme | Very High |

**Guidelines:**
- **Start with 8 shards** unless you have a specific reason to change
- **Increase to 16** if you have >1 Gbps bandwidth and large datasets
- **Decrease to 4** for smaller datasets (<10 GB) to reduce complexity
- **Never use 1** in production (defeats parallel architecture)

**S3 Request Rate Math:**
```
Total throughput = shards × 3,500 req/s per prefix
8 shards = 28,000 req/s (sufficient for most workloads)
16 shards = 56,000 req/s (for extreme scale)
```

### Chunk Size Optimization

**Default: 200 MB (adaptive, recommended)**

| Chunk Size | Best For | Pros | Cons |
|-----------|----------|------|------|
| 50-100 MB | Many small files | Faster selective extraction | More S3 requests |
| **200 MB** | **General purpose** | **Balanced** | **Recommended** |
| 500 MB+ | Few large files | Fewer S3 requests | Slower selective extraction |

**Guidelines:**
- **Use 200 MB default** for most workloads
- **Decrease to 100 MB** if you need very fast selective extraction
- **Increase to 500 MB** if you have few large files and want to minimize requests
- **Use adaptive (0)** to let CargoShip optimize automatically

### Compression Strategy

**Zstandard Level 3 (default, recommended)**

| Level | Speed | Ratio | Use Case |
|-------|-------|-------|----------|
| 1 (fastest) | 600+ MB/s | 2.0:1 | Maximum speed |
| **3 (default)** | **500+ MB/s** | **2.5:1** | **Balanced** |
| 5-10 (medium) | 200-400 MB/s | 2.8-3.2:1 | Better compression |
| 15+ (slow) | <100 MB/s | 3.5-4.0:1 | Archive storage |

**Content-Specific Recommendations:**

| Content Type | Compression Benefit | Recommendation |
|--------------|--------------------|--------------------|
| Text, logs, source code | High (3-5:1) | Use default level 3 |
| Structured data (JSON, CSV) | Medium (2.5-3:1) | Use default level 3 |
| Images (JPEG, PNG) | Very low (1.1:1) | Consider skipping compression |
| Video, audio | None (1.0:1) | Skip compression |
| Pre-compressed files | None (1.0:1) | Skip compression |

### Storage Class Selection

| Storage Class | Use Case | Retrieval | Cost/GB/Month |
|--------------|----------|-----------|---------------|
| STANDARD | Frequent access | Instant | $0.023 |
| INTELLIGENT_TIERING | Unknown access pattern | Instant | $0.023-$0.0125 |
| GLACIER_IR | Archive, occasional retrieval | Instant | $0.004 |
| GLACIER_FR | Long-term archive | 1-5 min | $0.0036 |
| DEEP_ARCHIVE | Compliance, rarely accessed | 12 hours | $0.00099 |

**Recommendations:**
```bash
# Frequent access (default)
cargoship create upload /data --bucket my-bucket --storage-class STANDARD

# Unknown access pattern (auto-optimize)
cargoship create upload /data --bucket my-bucket --storage-class INTELLIGENT_TIERING

# Archive with occasional access
cargoship create upload /data --bucket my-bucket --storage-class GLACIER_IR

# Long-term cold archive
cargoship create upload /data --bucket my-bucket --storage-class GLACIER_FR
```

### Worker Configuration

**Default: 4 workers per stage (recommended)**

```
Pipeline Stages:
  Scanner  (4 workers) → Archiver (4 workers) → Uploader (4 workers)
```

| Workers | Use Case | CPU Usage | Memory |
|---------|----------|-----------|--------|
| 2 | Low-resource systems | Low | 1-2 GB |
| **4** | **Most systems** | **Medium** | **2-3 GB** |
| 8 | High-performance systems | High | 4-6 GB |
| 16+ | Extreme scale (overkill) | Very High | 8-12 GB |

**Guidelines:**
- **Match CPU cores** (4 workers = 4-8 cores ideal)
- **Don't over-provision** (16 workers on 4 cores won't help)
- **Monitor memory** (more workers = more memory)

### Network Tuning

**HTTP/2 Settings (default: enabled)**

```bash
# Default settings (recommended)
cargoship create upload /data --bucket my-bucket

# Aggressive tuning for high-bandwidth networks
cargoship create upload /data \
  --bucket my-bucket \
  --http2-max-streams 500 \
  --max-idle-conns 200

# Conservative for unstable networks
cargoship create upload /data \
  --bucket my-bucket \
  --network-profile conservative
```

**Network Profiles:**

| Profile | Max Streams | Idle Conns | Use Case |
|---------|-------------|------------|----------|
| conservative | 100 | 50 | Unstable networks |
| **default** | **250** | **100** | **Most networks** |
| aggressive | 500 | 200 | High-bandwidth, stable |

### Monitoring & Observability

**Enable Verbose Logging:**
```bash
cargoship create upload /data --bucket my-bucket --verbose --trace
```

**JSON Output for Monitoring:**
```bash
cargoship create upload /data \
  --bucket my-bucket \
  --progress-format json | jq '.throughput_mbps'
```

**CloudWatch Metrics (if enabled):**
- Upload throughput (MB/s)
- Files processed
- Compression ratio
- Error rates

### Cost Optimization

**Reduce Costs by:**
1. **Use GLACIER_IR** for archives (82% savings vs STANDARD)
2. **Enable compression** (2.5:1 ratio = 60% storage savings)
3. **Selective extraction** (download only what you need)
4. **Lifecycle policies** (auto-transition to cheaper storage)

**Cost Example:**

| Storage Type | Size | Cost/Month | Annual Cost |
|--------------|------|------------|-------------|
| Uncompressed STANDARD | 100 GB | $2.30 | $27.60 |
| Compressed STANDARD | 40 GB | $0.92 | $11.04 (60% savings) |
| Compressed GLACIER_IR | 40 GB | $0.16 | $1.92 (93% savings) |

---

## Troubleshooting

### Common Issues

#### Upload Fails with "NoCredentialsError"

**Symptom:**
```
Error: failed to create AWS session: NoCredentialsError
```

**Solution:**
```bash
# Configure AWS credentials
aws configure

# Or set environment variables
export AWS_ACCESS_KEY_ID=your_key
export AWS_SECRET_ACCESS_KEY=your_secret
export AWS_REGION=us-west-2

# Or use AWS profile
export AWS_PROFILE=your-profile
```

#### Upload Stalls or Hangs

**Symptom:** Progress stops, no error message

**Possible Causes:**
1. **Network issues** - check connectivity
2. **S3 throttling** - reduce workers or shards
3. **Memory pressure** - reduce chunk size or workers

**Solutions:**
```bash
# Reduce concurrency
cargoship create upload /data \
  --bucket my-bucket \
  --workers 2 \
  --shards 4

# Enable verbose logging
cargoship create upload /data \
  --bucket my-bucket \
  --verbose \
  --trace

# Use conservative network profile
cargoship create upload /data \
  --bucket my-bucket \
  --network-profile conservative
```

#### Download Fails with "NoSuchKey"

**Symptom:**
```
Error: failed to download manifest: NoSuchKey
```

**Possible Causes:**
1. **Wrong upload ID**
2. **Wrong bucket or prefix**
3. **Manifest not uploaded** (incomplete upload)

**Solutions:**
```bash
# Verify upload ID exists
aws s3 ls s3://my-bucket/uploads/

# Check manifest exists
aws s3 ls s3://my-bucket/uploads/20251212-143022-a7f3b9c2/manifest.json.gz

# Use correct upload ID from upload output
cargoship info s3://my-bucket/uploads/20251212-143022-a7f3b9c2
```

#### Out of Memory (OOM) Errors

**Symptom:**
```
Error: signal: killed (OOM)
```

**Solutions:**
```bash
# Reduce chunk size
cargoship create upload /data \
  --bucket my-bucket \
  --chunk-size-mb 100

# Reduce workers
cargoship create upload /data \
  --bucket my-bucket \
  --workers 2

# Use memory limit (slower but safer)
cargoship create upload /data \
  --bucket my-bucket \
  --memory-limit 2GB
```

#### Slow Upload Performance

**Symptom:** Throughput < 100 MB/s

**Diagnostics:**
```bash
# Enable profiling
cargoship create upload /data \
  --bucket my-bucket \
  --profile \
  --verbose
```

**Common Causes:**
1. **Network bandwidth** - upgrade connection
2. **CPU bottleneck** - compression is CPU-intensive
3. **Too few shards** - increase to 16
4. **Too few workers** - increase to 8

**Solutions:**
```bash
# Increase parallelism
cargoship create upload /data \
  --bucket my-bucket \
  --shards 16 \
  --workers 8

# Use faster compression
cargoship create upload /data \
  --bucket my-bucket \
  --compression-level 1
```

#### Selective Extraction Not Working

**Symptom:** Downloads entire archive instead of selected files

**Possible Causes:**
1. **Pattern doesn't match** - check glob syntax
2. **Case sensitivity** - patterns are case-sensitive
3. **Wrong path** - use paths relative to upload root

**Solutions:**
```bash
# Test pattern first with list
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2 \
  --pattern "*.log"

# Use dry-run to preview
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./test \
  --pattern "*.log" \
  --dry-run

# Check exact file paths
cargoship list \
  --bucket my-bucket \
  --upload-id 20251212-143022-a7f3b9c2 \
  --verbose
```

### Performance Troubleshooting

#### Diagnosing Bottlenecks

**1. Check Pipeline Balance:**
```bash
# Enable verbose output to see stage throughput
cargoship create upload /data --bucket my-bucket --verbose
```

Look for:
- **Scanner slower than archiver?** → Many small files, increase scanner workers
- **Archiver slower than uploader?** → CPU bottleneck, reduce compression level
- **Uploader slower than archiver?** → Network bottleneck, increase shards

**2. Profile CPU Usage:**
```bash
# Enable profiling
cargoship create upload /data --bucket my-bucket --profile

# Analyze profile
go tool pprof /tmp/cargoship-cpu-*.prof
```

**3. Monitor Network:**
```bash
# Watch network throughput during upload
watch -n 1 'ifstat -i eth0'

# Check S3 request rates
aws cloudwatch get-metric-statistics \
  --namespace AWS/S3 \
  --metric-name AllRequests \
  --dimensions Name=BucketName,Value=my-bucket \
  --start-time $(date -u -d '5 minutes ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 60 \
  --statistics Sum
```

### Getting Help

**1. Enable Debug Output:**
```bash
cargoship create upload /data \
  --bucket my-bucket \
  --verbose \
  --trace > upload.log 2>&1
```

**2. Check GitHub Issues:**
- Search existing issues: https://github.com/scttfrdmn/cargoship/issues
- Report new issue with logs and `cargoship --version`

**3. Community Support:**
- GitHub Discussions (coming soon)
- Include manifest (redact sensitive paths): `cargoship info ... --json`

---

## Migration

### From Traditional TAR Archives

#### Before (Traditional Approach)

```bash
# Create archive
tar -czf data.tar.gz /path/to/data

# Upload to S3
aws s3 cp data.tar.gz s3://my-bucket/archives/

# Download
aws s3 cp s3://my-bucket/archives/data.tar.gz ./

# Extract
tar -xzf data.tar.gz
```

**Limitations:**
- Single large file (no parallelism)
- Must download entire archive
- Slow compression (single-threaded gzip)
- No metadata/manifest

#### After (CargoHold)

```bash
# Upload (parallel, streaming)
cargoship create upload /path/to/data --bucket my-bucket

# List files (instant, no download)
cargoship list --bucket my-bucket --upload-id 20251212-143022-a7f3b9c2

# Selective download (10x faster)
cargoship download \
  s3://my-bucket/uploads/20251212-143022-a7f3b9c2 \
  ./restored \
  --pattern "*.log"
```

**Benefits:**
- **8x faster upload** (parallel sharding)
- **10x faster selective extraction**
- **Zero local disk** usage
- **Manifest-driven** operations

### Migration Strategies

#### Strategy 1: Parallel Operations (Recommended)

Keep existing TAR archives, start using CargoHold for new data:

```bash
# Existing archives remain untouched
s3://my-bucket/archives/*.tar.gz

# New uploads use CargoHold
s3://my-bucket/uploads/20251212-*/
```

**Pros:**
- No disruption
- Gradual migration
- Keep old data accessible

**Cons:**
- Mixed storage formats
- Higher total storage

#### Strategy 2: Gradual Reupload

Re-upload old archives using CargoHold:

```bash
# Download old archive
aws s3 cp s3://my-bucket/archives/old-data.tar.gz ./
tar -xzf old-data.tar.gz

# Reupload with CargoHold
cargoship create upload ./old-data --bucket my-bucket --prefix migrated/

# Verify, then delete old archive
cargoship verify s3://my-bucket/migrated/uploads/20251212-*/
aws s3 rm s3://my-bucket/archives/old-data.tar.gz
```

**Pros:**
- Unified format
- Leverage CargoHold benefits
- Cleaner storage

**Cons:**
- Time-consuming for large archives
- Temporary storage increase

#### Strategy 3: Keep Archive, Add Manifest

If you must keep TAR format, consider creating manifests:

```bash
# Extract to temporary location
tar -xzf data.tar.gz -C /tmp/extracted

# Upload with CargoHold to create manifest
cargoship create upload /tmp/extracted --bucket my-bucket

# Keep old TAR + new manifest
```

**Use case:** Compliance requires specific archive format

### From aws-cli / s5cmd / rclone

#### aws s3 sync → CargoHold

**Before:**
```bash
aws s3 sync /data s3://my-bucket/data/
```

**After:**
```bash
cargoship create upload /data --bucket my-bucket
```

**Key Differences:**

| Aspect | aws s3 sync | CargoHold |
|--------|------------|-----------|
| Format | Individual files | Compressed chunks |
| Speed | 80-100 MB/s | 200-250 MB/s (2-3x faster) |
| Storage | Full size | 40% (2.5:1 compression) |
| Retrieval | File-by-file | Selective or full |
| Metadata | S3 object metadata | Manifest file |

**When to use aws s3 sync:**
- Need frequent access to individual files
- Don't want compression
- Existing workflows depend on individual S3 objects

**When to use CargoHold:**
- Archival/backup use case
- Want faster uploads
- Need compression
- Want selective extraction

### Cost Comparison

**Example: 100 GB dataset, stored for 1 year**

| Method | Storage | Transfer Cost | Total Cost | vs CargoHold |
|--------|---------|---------------|------------|-------------|
| Uncompressed STANDARD | $276 | $0 | $276 | Baseline |
| TAR + gzip STANDARD | $165 | $0 | $165 | 40% savings |
| TAR + gzip GLACIER_IR | $28.80 | $0 | $28.80 | 90% savings |
| **CargoHold STANDARD** | **$110** | **$0** | **$110** | **60% savings** |
| **CargoHold GLACIER_IR** | **$19.20** | **$0** | **$19.20** | **93% savings** |

**Notes:**
- CargoHold uses Zstandard (better compression than gzip)
- Storage class choice has bigger impact than format
- Transfer costs omitted (same for all methods)

---

## Appendix

### Manifest Schema Reference

See complete schema in `/docs/STORAGE_FORMAT.md`

### Performance Tuning Cheat Sheet

```bash
# Maximum speed (high-bandwidth network)
cargoship create upload /data \
  --bucket my-bucket \
  --shards 16 \
  --workers 8 \
  --http2-max-streams 500 \
  --network-profile aggressive

# Maximum compression (cold archive)
cargoship create upload /data \
  --bucket my-bucket \
  --compression-level 10 \
  --storage-class DEEP_ARCHIVE

# Low memory (constrained systems)
cargoship create upload /data \
  --bucket my-bucket \
  --chunk-size-mb 100 \
  --workers 2 \
  --memory-limit 1GB

# Balanced (recommended default)
cargoship create upload /data \
  --bucket my-bucket
```

### Glossary

| Term | Definition |
|------|------------|
| **Shard** | S3 prefix partition for parallel uploads |
| **Chunk** | TAR+zstd archive containing batches of files |
| **Manifest** | JSON index of all files and their locations |
| **Upload ID** | Unique identifier for an upload session |
| **Selective Extraction** | Downloading only specific files from archive |
| **Streaming Pipeline** | Zero-disk architecture with direct S3 upload |

### Related Documentation

- **Storage Format:** `/docs/STORAGE_FORMAT.md`
- **Architecture:** `/docs/ARCHITECTURE.md`
- **User Guide:** `/docs/USER_GUIDE.md`
- **Troubleshooting:** `/docs/TROUBLESHOOTING.md`

### Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-12 | Initial comprehensive guide |

---

**Questions or feedback?** Open an issue: https://github.com/scttfrdmn/cargoship/issues
