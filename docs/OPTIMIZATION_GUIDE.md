# CargoShip Optimization Guide

**Version**: v0.6.2
**Last Updated**: December 2025

## Overview

This guide provides advanced optimization techniques to maximize CargoShip's performance for your specific workloads. Following these recommendations can improve upload speed by 2-10x depending on your environment.

---

## Table of Contents

1. [Quick Wins](#quick-wins)
2. [Chunk Size Optimization](#chunk-size-optimization)
3. [Worker Concurrency Tuning](#worker-concurrency-tuning)
4. [Compression Strategy](#compression-strategy)
5. [Network Optimization](#network-optimization)
6. [Memory Management](#memory-management)
7. [S3 Configuration](#s3-configuration)
8. [Workload-Specific Optimizations](#workload-specific-optimizations)
9. [Monitoring and Profiling](#monitoring-and-profiling)
10. [Cost Optimization](#cost-optimization)

---

## Quick Wins

### 1. Use Default Settings First

CargoShip's defaults are optimized for most workloads:

```bash
# Good starting point
cargoship create upload ./data --bucket my-bucket
```

Defaults:
- Chunk size: 100MB
- Workers: 4 per prefix (32 total)
- Compression: Level 9 (balanced)
- Multi-prefix sharding: Enabled (8 prefixes)

### 2. Enable Incremental Sync

**Impact**: 10-100x faster for repeat uploads

```bash
# Always use the same bucket/prefix combination
cargoship create upload ./dataset --bucket my-bucket --prefix dataset-v1
```

Benefits:
- Only uploads changed files
- Content-based deduplication
- Manifest-driven delta detection

### 3. Choose Appropriate Storage Class

**Impact**: 50-90% cost reduction

```bash
# Infrequent access
cargoship create upload ./archives --storage-class GLACIER_IR

# Long-term archive
cargoship create upload ./archives --storage-class DEEP_ARCHIVE
```

See [Cost Optimization](#cost-optimization) for details.

---

## Chunk Size Optimization

### Understanding Chunk Size

Chunk size determines:
- **Archive file size**: Larger chunks = fewer S3 objects
- **Memory usage**: Larger chunks = more memory per worker
- **Upload parallelism**: Smaller chunks = more parallel uploads
- **Compression efficiency**: Larger chunks = better compression ratios

### Optimal Chunk Sizes by Workload

#### Many Small Files (<1MB each)

**Recommendation**: 50-100MB chunks

```bash
cargoship create upload ./logs --chunk-size 50MB
```

**Rationale**:
- Groups many files per archive
- Reduces S3 request count dramatically
- Example: 10,000 files × 10KB = 100MB → 2 chunks vs 10,000 PUT requests

**Performance Impact**:
- 1000x fewer S3 requests
- 50-100x faster uploads
- 99% cost reduction

#### Mixed File Sizes (1-100MB)

**Recommendation**: 100-250MB chunks

```bash
cargoship create upload ./dataset --chunk-size 150MB
```

**Rationale**:
- Balanced between parallelism and efficiency
- Good compression within chunks
- Reasonable memory usage

#### Large Files (>100MB each)

**Recommendation**: 250-500MB chunks

```bash
cargoship create upload ./videos --chunk-size 500MB
```

**Rationale**:
- Fewer but larger transfers
- Better compression for similar content
- Reduces overhead

### Dynamic Chunk Sizing

CargoShip includes intelligent chunking based on content patterns. Enable with:

```bash
cargoship create upload ./data --bucket my-bucket --smart-chunking
```

Benefits:
- Adapts to file size distribution
- Optimizes chunk boundaries
- Improves compression ratios

### Chunk Size Formula

Calculate optimal chunk size:

```
optimal_chunk_size = min(
    total_data_size / (workers * 4),
    500MB
)
```

Example:
- Total data: 10GB
- Workers: 8
- Formula: 10GB / (8 × 4) = 312MB
- Use: 250MB or 300MB chunks

---

## Worker Concurrency Tuning

### Understanding Workers

Workers control parallel upload concurrency:
- Each worker uploads one chunk at a time
- Multiple workers = multiple simultaneous uploads
- Default: 4 workers per prefix × 8 prefixes = 32 workers

### Calculate Optimal Worker Count

#### Method 1: Network Bandwidth

```
optimal_workers = (available_bandwidth_mbps × 0.8) / per_worker_throughput_mbps
```

Example:
- Available: 1000 Mbps (125 MB/s)
- Target utilization: 80% = 800 Mbps (100 MB/s)
- Per-worker throughput: ~10 MB/s
- Optimal workers: 100 / 10 = **10 workers**

```bash
cargoship create upload ./data --workers 10
```

#### Method 2: CPU Cores

```
optimal_workers = cpu_cores × 2
```

Example:
- 8 CPU cores
- Optimal workers: 8 × 2 = **16 workers**

```bash
cargoship create upload ./data --workers 16
```

### Worker Guidelines by Network Type

| Network Type | Bandwidth | Recommended Workers |
|--------------|-----------|---------------------|
| Residential DSL | 10-50 Mbps | 2-4 |
| Cable/Fiber | 100-500 Mbps | 4-8 |
| Business | 500-1000 Mbps | 8-16 |
| Datacenter | 1-10 Gbps | 16-32 |
| High-speed | 10+ Gbps | 32-64 |

### Too Many vs Too Few Workers

#### Too Few Workers (Under-utilization)

**Symptoms**:
- Low network utilization (<50%)
- Slow uploads despite fast network
- `htop` shows low CPU usage

**Solution**: Increase workers

```bash
# Before: 2 workers, 30% network usage
cargoship create upload ./data --workers 2

# After: 8 workers, 80% network usage
cargoship create upload ./data --workers 8
```

#### Too Many Workers (Over-saturation)

**Symptoms**:
- High memory usage
- Increased error rates
- CPU contention
- No speed improvement

**Solution**: Decrease workers

```bash
# Before: 64 workers, high errors
cargoship create upload ./data --workers 64

# After: 16 workers, stable
cargoship create upload ./data --workers 16
```

---

## Compression Strategy

### Compression Level Selection

Zstandard compression levels 1-19:

| Level | Speed | Ratio | CPU Usage | Best For |
|-------|-------|-------|-----------|----------|
| 1-3 | Very Fast | ~2x | Low | Already compressed, network-bound |
| 6-9 | Fast | ~4x | Medium | Balanced (default: 9) |
| 12-15 | Medium | ~5x | High | Text, logs, code |
| 16-19 | Slow | ~5.5x | Very High | Archival, CPU available |

### Workload-Specific Compression

#### Already Compressed Data

```bash
# Images (JPEG, PNG), videos (MP4, MKV), archives (zip, gz)
cargoship create upload ./media --compression-level 3
```

**Rationale**: Minimal additional compression, wasted CPU

#### Text Data

```bash
# Logs, code, CSV, JSON, XML
cargoship create upload ./logs --compression-level 12
```

**Rationale**: Excellent compression ratios (5-10x), worth the CPU cost

#### Mixed Data

```bash
# Datasets with various file types
cargoship create upload ./dataset --compression-level 9
```

**Rationale**: Balanced approach (default)

### Compression Performance

**Benchmark** (1GB test file):

```
Level 3:  Compress: 1.2s, Ratio: 2.1x, Upload: 6.5s, Total: 7.7s
Level 9:  Compress: 3.8s, Ratio: 4.3x, Upload: 3.2s, Total: 7.0s ✓ Best
Level 15: Compress: 12.1s, Ratio: 5.1x, Upload: 2.7s, Total: 14.8s
```

**Conclusion**: Level 9 provides best total time for most workloads.

### Disable Compression

For pre-compressed data:

```bash
cargoship create upload ./archives --compression-level 0
```

**When to use**:
- Data already compressed (.gz, .zip, .7z)
- Images and videos
- Network is the bottleneck

---

## Network Optimization

### BBR Congestion Control

CargoShip uses BBR (Bottleneck Bandwidth and RTT) congestion control when available.

**Check if BBR is enabled**:

```bash
# Linux
sysctl net.ipv4.tcp_congestion_control

# Expected: bbr
```

**Enable BBR** (Linux):

```bash
echo "net.core.default_qdisc=fq" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_congestion_control=bbr" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

**Performance impact**:
- 10-30% improvement on high-latency networks
- 20-50% improvement on lossy networks

### TCP Window Scaling

Increase TCP buffer sizes for high bandwidth-delay product networks:

```bash
# Linux
sudo sysctl -w net.core.rmem_max=134217728
sudo sysctl -w net.core.wmem_max=134217728
sudo sysctl -w net.ipv4.tcp_rmem='4096 87380 67108864'
sudo sysctl -w net.ipv4.tcp_wmem='4096 65536 67108864'
```

**When to use**:
- High bandwidth (>1 Gbps)
- High latency (>50ms RTT)
- Long-distance transfers

### Connection Pooling

CargoShip maintains persistent HTTP/2 connections to S3.

**Tune connection pool**:

```go
// In code (advanced)
transporter, err := s3.NewOptimizedTransporter(&s3.Config{
    MaxConnections: 100,  // Default: 50
    IdleTimeout:    30 * time.Second,
})
```

### Regional Endpoints

Use region-specific endpoints for lowest latency:

```bash
# Good: region-specific
cargoship create upload ./data --bucket my-bucket --region us-west-2

# Bad: generic endpoint (redirects)
# AWS_REGION=us-west-2 aws s3 cp ... (may use wrong endpoint)
```

---

## Memory Management

### Memory Usage Formula

```
memory_usage = chunk_size × (workers + pipeline_stages)
```

Example:
- Chunk size: 100MB
- Workers: 8
- Pipeline stages: 3 (scanner, archiver, uploader)
- Memory: 100MB × (8 + 3) = **1.1GB**

### Memory-Constrained Environments

#### Low Memory (<2GB)

```bash
cargoship create upload ./data --bucket my-bucket \
  --chunk-size 50MB \
  --workers 2
```

Memory: 50MB × (2 + 3) = 250MB

#### Moderate Memory (2-8GB)

```bash
cargoship create upload ./data --bucket my-bucket \
  --chunk-size 100MB \
  --workers 8
```

Memory: 100MB × (8 + 3) = 1.1GB

#### High Memory (>8GB)

```bash
cargoship create upload ./data --bucket my-bucket \
  --chunk-size 250MB \
  --workers 16
```

Memory: 250MB × (16 + 3) = 4.75GB

### Monitor Memory Usage

```bash
# During upload
watch -n 1 'ps aux | grep cargoship'

# Or use htop
htop -p $(pgrep cargoship)
```

---

## S3 Configuration

### Multi-Prefix Sharding

CargoShip distributes uploads across 8 S3 prefixes by default.

**S3 Rate Limits**:
- 3,500 PUT/s per prefix
- 5,500 GET/s per prefix

**CargoShip effective limits**:
- 28,000 PUT/s (8 prefixes × 3,500)
- 44,000 GET/s (8 prefixes × 5,500)

**Tune prefix count** (advanced):

```go
// In code
config := &pipeline.Config{
    ShardCount: 16,  // 16 prefixes instead of 8
}
```

### Transfer Acceleration

Enable S3 Transfer Acceleration for global uploads:

```bash
# Enable on bucket first
aws s3api put-bucket-accelerate-configuration \
  --bucket my-bucket \
  --accelerate-configuration Status=Enabled

# Use with CargoShip
cargoship create upload ./data --bucket my-bucket --accelerate
```

**When to use**:
- Uploads from distant regions (>1000 miles)
- International transfers
- High latency (>100ms)

**Cost**: +$0.04/GB (on top of standard transfer)

**Performance**: 50-500% improvement for long-distance transfers

### Multipart Upload Tuning

CargoShip uses multipart uploads for chunks >5MB.

**Part size** (automatic based on chunk size):
- Chunk 100MB: Part size ~16MB
- Chunk 500MB: Part size ~64MB

**Tune for very large chunks**:

```bash
# 1GB chunks with 100MB parts
cargoship create upload ./data --chunk-size 1GB --multipart-size 100MB
```

---

## Workload-Specific Optimizations

### 1. Many Small Files (Logs, Code, Configuration)

**Characteristics**:
- 10,000+ files
- <1MB per file
- Text content

**Optimal Settings**:

```bash
cargoship create upload ./logs --bucket my-bucket \
  --chunk-size 50MB \
  --compression-level 12 \
  --workers 8 \
  --storage-class STANDARD_IA
```

**Expected Performance**:
- 10,000 files (20MB total)
- Upload time: 1-2 seconds
- 1000x faster than individual file uploads

### 2. ML Datasets (Mixed File Sizes)

**Characteristics**:
- 1,000-100,000 files
- 1-100MB per file
- Images, annotations, metadata

**Optimal Settings**:

```bash
cargoship create upload ./imagenet --bucket ml-datasets \
  --chunk-size 150MB \
  --compression-level 6 \
  --workers 16 \
  --storage-class STANDARD
```

**Expected Performance**:
- 100GB dataset
- Upload time: 5-10 minutes (1 Gbps network)
- Throughput: 200-400 Mbps

### 3. Video Files (Large Individual Files)

**Characteristics**:
- 10-1,000 files
- >100MB per file
- Already compressed (H.264, H.265)

**Optimal Settings**:

```bash
cargoship create upload ./videos --bucket video-archive \
  --chunk-size 500MB \
  --compression-level 3 \
  --workers 4 \
  --storage-class GLACIER_IR
```

**Expected Performance**:
- 1TB video library
- Upload time: 2-4 hours (1 Gbps network)
- Throughput: 600-800 Mbps

### 4. Database Backups (Large Single Files)

**Characteristics**:
- 1-10 files
- >1GB per file
- Compressible (SQL dumps)

**Optimal Settings**:

```bash
cargoship create upload ./db-backups --bucket backups \
  --chunk-size 1GB \
  --compression-level 12 \
  --workers 2 \
  --storage-class GLACIER_IR
```

**Expected Performance**:
- 100GB database backup
- Compressed: ~20GB (5:1 ratio)
- Upload time: 5-10 minutes
- Throughput: 400-800 Mbps

---

## Monitoring and Profiling

### Real-Time Monitoring

#### Basic Progress

```bash
cargoship create upload ./data --bucket my-bucket
# ⣾ Uploading: 342/1000 files (34%) | 1.2 GB / 3.5 GB | 15.3 MB/s | ETA: 2m 34s
```

#### JSON Output for Monitoring

```bash
cargoship create upload ./data --bucket my-bucket --json | tee upload-metrics.json
```

### Distributed Tracing

Enable OpenTelemetry tracing:

```bash
# Start Jaeger
docker run -d --name jaeger -p 16686:16686 -p 14268:14268 jaegertracing/all-in-one:latest

# Upload with tracing
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter jaeger \
  --tracing-endpoint http://localhost:14268/api/traces

# View traces
open http://localhost:16686
```

**What you'll see**:
- Request flow through pipeline stages
- Per-chunk upload times
- Retry attempts and errors
- Bottleneck identification

### Prometheus Metrics

Expose Prometheus metrics:

```bash
cargoship create upload ./data --bucket my-bucket \
  --prometheus-addr :9090

# In another terminal
curl http://localhost:9090/metrics
```

**Key metrics**:
- `cargoship_upload_duration_seconds` - Upload latency histogram
- `cargoship_upload_throughput_bytes_per_sec` - Real-time throughput
- `cargoship_retry_attempts_total` - Retry counts
- `cargoship_active_uploads` - Concurrent uploads

### CPU Profiling

Profile CPU usage:

```bash
# Enable pprof
cargoship create upload ./data --bucket my-bucket --pprof :6060

# Capture CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile
```

### Memory Profiling

Profile memory allocation:

```bash
# Enable pprof
cargoship create upload ./data --bucket my-bucket --pprof :6060

# Capture heap profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

---

## Cost Optimization

### Storage Class Selection

**Decision Tree**:

```
Access frequency?
├─ Daily/weekly → STANDARD ($0.023/GB-month)
├─ Monthly → STANDARD_IA ($0.0125/GB-month)
├─ Quarterly → GLACIER_IR ($0.004/GB-month)
├─ Yearly → GLACIER ($0.0036/GB-month)
└─ Multi-year → DEEP_ARCHIVE ($0.00099/GB-month)
```

### Request Cost Optimization

**Traditional tools** (10,000 files):
- Requests: 10,000 PUT
- Cost: 10,000 / 1,000 × $0.005 = **$0.05**

**CargoShip** (10,000 files):
- Requests: ~10 PUT (10 chunks)
- Cost: 10 / 1,000 × $0.005 = **$0.00005**

**Savings**: 99.9% ($0.05 → $0.00005)

### Compression Savings

**Example**: 1TB dataset

**Without compression**:
- Storage: 1TB × $0.023 = **$23.55/month**

**With compression** (4:1 ratio):
- Storage: 0.25TB × $0.023 = **$5.89/month**

**Savings**: $17.66/month (75%)

### Lifecycle Policies

Automate transitions to cheaper storage:

```bash
# Create lifecycle policy
aws s3api put-bucket-lifecycle-configuration \
  --bucket my-bucket \
  --lifecycle-configuration file://lifecycle.json
```

`lifecycle.json`:
```json
{
  "Rules": [{
    "Id": "archive-old-uploads",
    "Status": "Enabled",
    "Transitions": [
      {
        "Days": 30,
        "StorageClass": "STANDARD_IA"
      },
      {
        "Days": 90,
        "StorageClass": "GLACIER_IR"
      },
      {
        "Days": 365,
        "StorageClass": "DEEP_ARCHIVE"
      }
    ]
  }]
}
```

**Annual savings** (1TB dataset):
- Months 0-1: STANDARD ($23.55)
- Months 1-3: STANDARD_IA ($12.81)
- Months 3-12: GLACIER_IR ($4.10)
- Year 2+: DEEP_ARCHIVE ($1.01)

**Year 1 cost**: $83.58 (vs $282.60 with STANDARD)
**Savings**: $199.02 (70%)

---

## Performance Checklist

Before running large uploads:

- [ ] **Network**: Verify available bandwidth (`speedtest-cli`)
- [ ] **Workers**: Calculate optimal count (bandwidth / 10 MB/s)
- [ ] **Chunk Size**: Choose based on file sizes (50-500MB)
- [ ] **Compression**: Select level (3 for media, 9-12 for text)
- [ ] **Storage Class**: Choose appropriate tier
- [ ] **Memory**: Ensure sufficient RAM (chunk_size × workers)
- [ ] **Region**: Use closest S3 region
- [ ] **Incremental**: Use same bucket/prefix for repeats
- [ ] **Monitoring**: Enable metrics/tracing if needed

---

## Advanced Tuning

### Custom Chunking Algorithm

Implement custom chunking logic:

```go
type CustomChunker struct {
    // Your logic here
}

func (c *CustomChunker) ChunkFiles(files []File) []Chunk {
    // Custom grouping logic
    // E.g., group by file type, size ranges, etc.
}

// Use custom chunker
p := pipeline.New(&pipeline.Config{
    Chunker: &CustomChunker{},
})
```

### Custom Compression

Use custom compression:

```go
// Implement Compressor interface
type CustomCompressor struct{}

func (c *CustomCompressor) Compress(data []byte) ([]byte, error) {
    // Your compression logic (e.g., brotli, lz4)
}

// Use custom compressor
p := pipeline.New(&pipeline.Config{
    Compressor: &CustomCompressor{},
})
```

---

## Benchmarking Your Setup

### Run Benchmark

```bash
# Create test data
mkdir -p /tmp/test-data
for i in {1..10000}; do
  dd if=/dev/zero of=/tmp/test-data/file-$i.dat bs=2048 count=1
done

# Benchmark upload
time cargoship create upload /tmp/test-data --bucket test-bucket

# Measure throughput
# Expected: 1-3 seconds for 20MB (10,000 files)
```

### Compare Configurations

```bash
# Configuration A: Default
time cargoship create upload ./test-data --bucket test-bucket

# Configuration B: High concurrency
time cargoship create upload ./test-data --bucket test-bucket --workers 16

# Configuration C: Large chunks
time cargoship create upload ./test-data --bucket test-bucket --chunk-size 200MB
```

---

## References

- [S3 Direct Upload Guide](S3_DIRECT_UPLOAD.md)
- [Performance Benchmarks](PERFORMANCE_BENCHMARKS.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [CLI Reference](CLI_REFERENCE.md)

---

## Support

For optimization assistance:
- **Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Discussions**: https://github.com/scttfrdmn/cargoship/discussions
