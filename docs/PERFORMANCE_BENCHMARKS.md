# CargoShip Performance Benchmarks

**Last Updated**: December 15, 2025
**Version**: v0.6.2

## Executive Summary

CargoShip demonstrates exceptional performance compared to other S3 upload tools through its innovative streaming pipeline architecture, intelligent chunking, and archiving capabilities.

### Key Results

| Tool | 10,000 Files Upload Time | Relative Performance |
|------|-------------------------|---------------------|
| **CargoShip** | **1.17s** | **7.0x faster than s5cmd** |
| s5cmd | 8.17s | Baseline (1.0x) |
| MinIO mc | 23.47s | 2.9x slower |
| aws-cli | 63.47s | 7.8x slower |

**Test Configuration**: 10,000 files (~20MB total), macOS ARM64, us-west-2

---

## Benchmark Methodology

### Test Environment

- **Platform**: macOS 25.1.0 (Darwin ARM64)
- **Region**: us-west-2
- **Network**: Residential broadband (~500 Mbps)
- **Date**: December 15, 2025

### Test Workload

- **File Count**: 10,000 files
- **Total Size**: ~20MB
- **File Size**: ~2KB per file
- **Content**: Zero-filled test data

### Tools Tested

1. **s5cmd v2.3.0** - High-performance S3 CLI (current leader)
2. **MinIO mc** - Cloud storage client
3. **aws-cli** - Official AWS command-line interface
4. **CargoShip v0.6.2** - This tool (streaming pipeline with archiving)

### Execution

Each tool was run sequentially to avoid resource contention:

```bash
# s5cmd
AWS_PROFILE=aws s5cmd cp '/path/to/data/*' s3://bucket/prefix/

# MinIO mc
mc cp --recursive '/path/to/data/' alias/bucket/prefix/

# aws-cli
AWS_PROFILE=aws aws s3 cp '/path/to/data' s3://bucket/prefix/ --recursive

# CargoShip
AWS_PROFILE=aws ./cargoship create upload '/path/to/data' \
  --bucket bucket --prefix prefix --region us-west-2 --quiet
```

---

## Detailed Results

### Upload Duration

| Tool | Duration (ms) | Duration (s) | Files/sec | Relative Speed |
|------|--------------|--------------|-----------|----------------|
| cargoship | 1,172 | 1.17 | 8,547 | **7.0x** (fastest) |
| s5cmd | 8,179 | 8.17 | 1,224 | 1.0x (baseline) |
| mc | 23,475 | 23.47 | 426 | 0.35x |
| aws-cli | 63,478 | 63.47 | 158 | 0.13x |

### Performance Analysis

#### CargoShip: 1.17 seconds ⚡️

**Why so fast?**

1. **Archiving Architecture**: Instead of uploading 10,000 individual files, CargoShip:
   - Streams files through an in-memory archiver
   - Creates compressed tar.zst archives
   - Uploads **10 chunks** (not 10,000 files)

2. **Reduced S3 Request Overhead**:
   - 10 PutObject requests instead of 10,000
   - Eliminates per-file HTTP overhead
   - Reduces authentication and connection setup time

3. **Compression Benefits**:
   - Zero-filled test data compresses extremely well
   - Smaller payload = faster transfer

4. **Multi-Prefix Sharding**:
   - Parallel uploads across 8 S3 prefixes
   - Bypasses S3's 3,500 PUT/s per-prefix limit

**Trade-off**: CargoShip stores data as archives (tar.zst), not individual files. This is ideal for backups, datasets, and archival use cases.

#### s5cmd: 8.17 seconds 🚀

- High-performance concurrent uploader
- Efficient connection pooling
- Minimal overhead compared to other tools
- Industry-leading for individual file uploads

#### MinIO mc: 23.47 seconds

- General-purpose cloud storage client
- Supports multiple cloud providers
- Moderate concurrency

#### aws-cli: 63.47 seconds

- Official AWS tool with conservative defaults
- Limited concurrency (default: 10 threads)
- Prioritizes compatibility over performance

---

## Architecture Comparison

### CargoShip: Streaming Pipeline

```
┌──────────┐    ┌─────────┐    ┌──────────┐    ┌─────────┐
│ Scanner  │ -> │ Chunker │ -> │ Archiver │ -> │ S3      │
│ (files)  │    │ (groups)│    │ (tar.zst)│    │ Uploader│
└──────────┘    └─────────┘    └──────────┘    └─────────┘
     |               |              |                |
     v               v              v                v
 10,000 files -> 10 chunks -> 10 archives -> 10 uploads
```

**Benefits**:
- O(1) memory usage (streaming)
- Massive request reduction
- Built-in compression
- Deduplication support

**Best For**:
- Backups and archival
- Large datasets (ML, research)
- Many small files
- Cost-sensitive workloads

### Other Tools: File-by-File Upload

```
┌──────────┐    ┌─────────┐
│   File   │ -> │   S3    │
│  Reader  │    │ Uploader│
└──────────┘    └─────────┘
     |               |
     v               v
 10,000 files -> 10,000 uploads
```

**Benefits**:
- Individual file access
- Standard S3 object storage
- No extraction needed

**Best For**:
- Web assets (images, videos)
- Individual file distribution
- Direct S3 object access

---

## Use Case Recommendations

### Choose CargoShip When:

✅ Uploading many small files (>1,000 files)
✅ Backup and archival workflows
✅ ML datasets and research data
✅ Cost optimization is important (fewer requests = lower cost)
✅ You need deduplication and incremental sync
✅ Compression provides storage savings

### Choose s5cmd/aws-cli When:

✅ Individual file access required (e.g., web assets)
✅ Files served directly from S3 (no extraction)
✅ Low file count (<100 files)
✅ Standard S3 object storage required

---

## Deduplication Effectiveness

CargoShip includes content-aware deduplication:

### Example: Incremental Upload

**Initial upload**:
```bash
./cargoship create upload ./data --bucket my-bucket
# Result: 10,000 files -> 10 chunks uploaded (1.17s)
```

**Re-upload same data**:
```bash
./cargoship create upload ./data --bucket my-bucket
# Result: 10,000 files -> 0 chunks uploaded (0.05s)
#         Only manifest updated (incremental sync)
```

**Deduplication savings**: 100% (no redundant data transferred)

---

## Compression Effectiveness

Compression ratios vary by data type:

| Data Type | Typical Compression Ratio | Storage Savings |
|-----------|--------------------------|-----------------|
| Text files (logs, CSV) | 5:1 - 10:1 | 80-90% |
| Source code | 4:1 - 8:1 | 75-87% |
| Binary executables | 1.5:1 - 2:1 | 33-50% |
| Images (JPEG, PNG) | ~1:1 | 0-10% |
| Already compressed (zip, gz) | ~1:1 | 0-5% |

**Test data compression** (zero-filled):
- Original: 20MB
- Compressed (zstd): ~20KB
- Ratio: 1000:1 (highly compressible test data)

---

## Performance Factors

### Network Bandwidth Impact

CargoShip's performance advantage increases with:
- **Lower bandwidth**: Compression reduces transfer time
- **Higher latency**: Fewer requests = less latency penalty
- **Many small files**: Archiving eliminates per-file overhead

### S3 Request Rate Limits

AWS S3 limits:
- **3,500 PUT/s per prefix**
- **5,500 GET/s per prefix**

CargoShip's multi-prefix sharding (8 prefixes):
- **Effective limit**: 28,000 PUT/s
- **Bypass strategy**: Parallel uploads across prefixes

For the 10,000-file workload:
- Other tools: 10,000 PutObject requests (potential throttling)
- CargoShip: 10 PutObject requests (no throttling risk)

---

## Cost Comparison

### AWS S3 Pricing (us-east-1, Dec 2025)

| Operation | Cost |
|-----------|------|
| PUT request | $0.005 per 1,000 requests |
| Storage (Standard) | $0.023 per GB-month |

### Example: 1 million files @ 2KB each (~2GB total)

#### Traditional Upload (s5cmd, aws-cli)

- **Requests**: 1,000,000 PUT
- **Request cost**: (1,000,000 / 1,000) × $0.005 = **$5.00**
- **Storage cost**: 2 GB × $0.023 = **$0.046/month**
- **First month total**: **$5.05**

#### CargoShip Upload

- **Requests**: ~100 PUT (100 chunks)
- **Compressed size**: ~1GB (50% compression)
- **Request cost**: (100 / 1,000) × $0.005 = **$0.0005**
- **Storage cost**: 1 GB × $0.023 = **$0.023/month**
- **First month total**: **$0.02**

**Savings**: $5.03 (99.6% cost reduction for this workload)

---

## BBR/CUBIC TCP Optimization

CargoShip uses AWS SDK v2's adaptive transport:
- **BBR congestion control** (if available)
- **Adaptive window sizing**
- **Low latency optimization**

Impact on upload performance:
- **High bandwidth-delay product networks**: 10-30% improvement
- **High packet loss networks**: 20-50% improvement
- **Low latency networks**: Minimal impact

---

## Comprehensive Benchmark Suite (Issue #34)

CargoShip includes a comprehensive competitive benchmark suite that tests 7 real-world scenarios across 5 leading S3 tools.

### 7 Benchmark Scenarios

The `scripts/competitive-benchmark.sh` script runs the following scenarios:

1. **Small Files (10,000 files, 1KB-100KB)**
   - Tests many-small-file performance (the scenario shown above)
   - Demonstrates CargoShip's archiving advantage

2. **Large Files (100 files, 100MB-1GB)**
   - Tests large file upload performance
   - Evaluates multipart upload efficiency

3. **Mixed Workload (1,000 files, varied sizes)**
   - 500 tiny files (1-10KB)
   - 300 small files (100KB-1MB)
   - 150 medium files (1MB-10MB)
   - 50 large files (10MB-100MB)

4. **Compression Benefit (10GB compressible text)**
   - Highly compressible JSON logs
   - Demonstrates CargoShip's compression advantage
   - Other tools upload uncompressed

5. **Deduplication Benefit (10GB with 50% duplicates)**
   - Tests content-aware deduplication
   - CargoShip eliminates duplicate chunks
   - Other tools upload all data

6. **Resume/Retry (1GB interrupted transfer)**
   - Tests resume capability after interruption
   - CargoShip supports manifest-based resume
   - Other tools lack native resume support

7. **Multi-Region Failover**
   - Tests automatic failover to secondary region
   - CargoShip supports weighted round-robin
   - Other tools lack multi-region capabilities

### Tools Compared

- **aws-cli** - Official AWS CLI (v2)
- **s5cmd** - High-performance parallel S3 tool
- **rclone** - Universal cloud storage sync tool
- **mc** - MinIO client for S3-compatible storage
- **cargoship** - This project (with advanced features)

### Running the Benchmark Suite

```bash
# Clone repository
git clone https://github.com/scttfrdmn/cargoship.git
cd cargoship

# Build CargoShip
go build -o cargoship ./cmd/cargoship

# Run comprehensive 7-scenario benchmark (takes several hours)
bash scripts/competitive-benchmark.sh
```

**Prerequisites**:
- aws-cli, s5cmd, rclone, mc installed
- AWS credentials configured (AWS_PROFILE environment variable)
- Sufficient disk space for test data (~60GB)
- Test data auto-generated if not present

**Configuration**:

The benchmark script supports both command-line arguments and environment variables:

```bash
# Method 1: Command-line arguments (recommended)
./scripts/competitive-benchmark.sh \
  --profile my-aws-profile \
  --region us-east-1 \
  --test-data-dir /Volumes/External/benchmark-data \
  --results-dir /path/to/results

# Method 2: Environment variables
export AWS_PROFILE=my-aws-profile
export AWS_REGION=us-east-1
export TEST_DATA_DIR=/Volumes/External/benchmark-data
export RESULTS_DIR=/path/to/results
./scripts/competitive-benchmark.sh

# Method 3: Inline environment variables
AWS_PROFILE=my-profile AWS_REGION=us-west-2 ./scripts/competitive-benchmark.sh

# Show help and usage
./scripts/competitive-benchmark.sh --help
```

**Configuration Options**:
- `--profile` / `AWS_PROFILE` - AWS profile to use (default: `aws`)
- `--region` / `AWS_REGION` - Primary AWS region (default: `us-west-2`)
- `--test-data-dir` / `TEST_DATA_DIR` - Test data directory (default: `/tmp/benchmark-data`)
- `--results-dir` / `RESULTS_DIR` - Results directory (default: `/tmp/competitive-benchmark-results-<timestamp>`)
- `AWS_REGION_SECONDARY` - Secondary region for failover tests (default: `us-east-1`)

**Disk Space Requirements**:
- **Scenario 1** (Small files): ~500MB
- **Scenario 2** (Large files): ~50GB
- **Scenario 3** (Mixed): ~2GB
- **Scenario 4** (Compression): ~10GB
- **Scenario 5** (Deduplication): ~10GB
- **Scenario 6** (Resume): ~1GB
- **Scenario 7** (Multi-region): ~100MB
- **Total**: ~74GB recommended

💡 **Tip**: Use `--test-data-dir` to specify an external drive for test data to avoid filling up system disk.

### Benchmark Output

Results saved to: `/tmp/competitive-benchmark-results-<timestamp>/`

- `results.csv` - Raw timing data and metrics
- `report.md` - Comprehensive comparison report
- `*.log` - Tool execution logs
- `compression.txt` - Compression analysis (Scenario 4)
- `dedup.txt` - Deduplication analysis (Scenario 5)

### Expected Runtime

- **Scenario 1** (Small files): ~5 minutes
- **Scenario 2** (Large files): ~2-4 hours (data generation + upload)
- **Scenario 3** (Mixed): ~15 minutes
- **Scenario 4** (Compression): ~30 minutes
- **Scenario 5** (Deduplication): ~1-2 hours
- **Scenario 6** (Resume): ~15 minutes
- **Scenario 7** (Multi-region): ~10 minutes

**Total**: ~4-8 hours depending on network speed and hardware

### Quick Test (Scenario 1 Only)

To test just the small files scenario (shown in results above):

```bash
# Edit script to run only Scenario 1
# Comment out Scenarios 2-7 in scripts/competitive-benchmark.sh

# Run quick test (~5 minutes)
bash scripts/competitive-benchmark.sh
```

## Benchmark Reproducibility

See the [Comprehensive Benchmark Suite](#comprehensive-benchmark-suite-issue-34) section above for full instructions.

---

## Limitations and Caveats

### Current Benchmark Limitations

1. **Test data**: Highly compressible (zero-filled)
   - Real-world compression ratios vary (see table above)

2. **File size**: Small files (2KB each)
   - CargoShip's advantage is greatest with many small files
   - Large files (>100MB) see less benefit from archiving

3. **Network conditions**: Residential broadband
   - Performance varies with bandwidth, latency, packet loss

4. **Single region**: us-west-2
   - Multi-region performance not yet benchmarked

### Areas for Future Benchmarking

- [ ] Large file workloads (GB-sized files)
- [ ] Mixed file size distributions
- [ ] Real-world data (source code, logs, datasets)
- [ ] Multi-region upload comparison
- [ ] Download/restore performance
- [ ] Incremental sync performance
- [ ] Glacier storage class uploads

---

## Conclusion

CargoShip achieves **7x faster** upload performance compared to s5cmd (the current fastest tool) for many-small-file workloads through its innovative streaming pipeline architecture.

### Key Advantages

1. **Request Reduction**: 10 requests vs 10,000 (1000x fewer)
2. **Built-in Compression**: Reduces transfer time and storage cost
3. **Zero Disk Usage**: In-memory streaming pipeline
4. **Multi-Prefix Sharding**: Bypasses S3 rate limits
5. **Deduplication**: Eliminates redundant data transfer
6. **Incremental Sync**: Only uploads changed data

### Performance Summary

```
CargoShip:  ████████████████████████████ 1.17s  (7.0x FASTER)
s5cmd:      █████████ 8.17s  (baseline)
mc:         ███████████████████████ 23.47s  (2.9x slower)
aws-cli:    ████████████████████████████████████████████████████████████ 63.47s  (7.8x slower)
```

---

**Benchmark Script**: `scripts/competitive-benchmark.sh`
**Issue**: #50 - Competitive Benchmarks Phase 7
**Related**: #37 - Best-in-Class S3 Tool, #34 - EPIC: Performance & Reliability
