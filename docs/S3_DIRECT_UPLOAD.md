# S3 Direct Upload Guide

**CargoShip v0.6.2**

## Overview

CargoShip provides high-performance S3 uploads through its streaming pipeline architecture. Unlike traditional tools that upload files individually, CargoShip streams data through an in-memory pipeline that archives, compresses, and uploads data efficiently.

### Key Features

- **Streaming Pipeline**: Zero disk usage, O(1) memory consumption
- **Automatic Archiving**: Groups files into tar.zst archives for efficient transfer
- **Multi-Prefix Sharding**: Bypasses S3's 3,500 PUT/s per-prefix limit
- **Intelligent Chunking**: Content-aware chunking for optimal compression
- **Deduplication**: Eliminates redundant data transfer
- **Incremental Sync**: Only uploads changed data
- **Multi-Region Support**: Automatic failover and load balancing

---

## Quick Start

### Basic Upload

```bash
cargoship create upload /path/to/data \
  --bucket my-bucket \
  --region us-west-2
```

This command:
1. Scans files in `/path/to/data`
2. Groups them into chunks (default: 100MB each)
3. Creates compressed tar.zst archives
4. Uploads to S3 using 8 parallel workers
5. Stores manifest for incremental sync

### Upload with Prefix

```bash
cargoship create upload ./dataset \
  --bucket research-data \
  --prefix experiments/2025/jan \
  --region us-east-1
```

Result: Files stored at `s3://research-data/experiments/2025/jan/`

---

## Upload Architecture

### Pipeline Flow

```
┌──────────┐    ┌─────────┐    ┌──────────┐    ┌─────────┐
│ Scanner  │ -> │ Chunker │ -> │ Archiver │ -> │ S3      │
│          │    │         │    │ (tar.zst)│    │ Uploader│
└──────────┘    └─────────┘    └──────────┘    └─────────┘
```

1. **Scanner**: Walks directory tree, identifies files
2. **Chunker**: Groups files into optimal chunks based on:
   - Target chunk size (default: 100MB)
   - File boundaries (doesn't split files)
   - Content patterns (smart chunking)
3. **Archiver**: Creates tar.zst archives in-memory
4. **Uploader**: Uploads to S3 with:
   - Multi-prefix sharding (8 prefixes)
   - Parallel workers (default: 4 per prefix)
   - Automatic retries with exponential backoff

### Storage Format

Files are stored as:
```
s3://bucket/prefix/uploads/<upload-id>/
├── shard-0/
│   ├── chunk-0.tar.zst
│   ├── chunk-1.tar.zst
│   └── ...
├── shard-1/
│   └── ...
├── ...
└── manifest.json.gz (metadata)
```

**Manifest** contains:
- File list with checksums
- Chunk mappings
- Upload metadata
- Deduplication index

---

## Configuration Options

### Chunk Size

Control archive size:

```bash
# Small chunks (50MB) - faster uploads, more files
cargoship create upload ./data --bucket my-bucket --chunk-size 50MB

# Large chunks (500MB) - fewer files, better compression
cargoship create upload ./data --bucket my-bucket --chunk-size 500MB
```

**Guidelines**:
- Small files (<1MB): Use 50-100MB chunks
- Medium files (1-100MB): Use 100-250MB chunks
- Large files (>100MB): Use 250-500MB chunks
- Very large files (>1GB): Consider increasing to 1GB

### Worker Concurrency

Adjust parallel upload workers:

```bash
# Low concurrency (2 workers) - limited bandwidth
cargoship create upload ./data --bucket my-bucket --workers 2

# High concurrency (16 workers) - high bandwidth
cargoship create upload ./data --bucket my-bucket --workers 16
```

**Default**: 4 workers per prefix × 8 prefixes = 32 parallel uploads

**Guidelines**:
- Residential broadband (50-100 Mbps): 2-4 workers
- Business/datacenter (1+ Gbps): 8-16 workers
- High-speed (10+ Gbps): 16-32 workers

### Storage Class

Choose S3 storage class:

```bash
# Standard (frequent access)
cargoship create upload ./data --bucket my-bucket --storage-class STANDARD

# Infrequent Access (cheaper storage, retrieval fees)
cargoship create upload ./data --bucket my-bucket --storage-class STANDARD_IA

# Glacier Instant Retrieval (archive, instant access)
cargoship create upload ./data --bucket my-bucket --storage-class GLACIER_IR

# Glacier Flexible Retrieval (archive, 1-5 min retrieval)
cargoship create upload ./data --bucket my-bucket --storage-class GLACIER

# Deep Archive (long-term, 12h retrieval)
cargoship create upload ./data --bucket my-bucket --storage-class DEEP_ARCHIVE
```

**Cost Comparison** (us-east-1, per GB-month):
- STANDARD: $0.023
- STANDARD_IA: $0.0125 (+ $0.01/GB retrieval)
- GLACIER_IR: $0.004 (+ $0.03/GB retrieval)
- GLACIER: $0.0036 (+ retrieval time/cost)
- DEEP_ARCHIVE: $0.00099 (+ 12h retrieval)

---

## Advanced Features

### Multi-Prefix Sharding

CargoShip automatically distributes uploads across 8 S3 prefixes to bypass the 3,500 PUT/s per-prefix rate limit.

**Effective limit**: 28,000 PUT/s (8 prefixes × 3,500)

No configuration needed - enabled by default.

### Deduplication

Content-based deduplication eliminates redundant data:

```bash
# Initial upload
cargoship create upload ./dataset --bucket my-bucket
# Result: 10GB uploaded

# Re-upload same data (incremental sync)
cargoship create upload ./dataset --bucket my-bucket
# Result: 0GB uploaded (only manifest updated)

# Upload with some changes
# Modified: 100MB, Added: 50MB, Deleted: 20MB
cargoship create upload ./dataset --bucket my-bucket
# Result: 150MB uploaded (only changed/new data)
```

**How it works**:
1. SHA-256 checksums for each file
2. Manifest tracks all uploaded content
3. Only uploads files with changed checksums
4. Chunk-level deduplication within archives

### Encryption

#### Server-Side Encryption (SSE-S3)

Enabled by default:

```bash
cargoship create upload ./data --bucket my-bucket
# All objects encrypted with SSE-S3
```

#### Customer-Managed Keys (SSE-KMS)

Use AWS KMS keys:

```bash
cargoship create upload ./data --bucket my-bucket \
  --kms-key-id arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
```

### Compression Levels

Zstandard compression with adjustable levels:

```bash
# Fast compression (level 3) - faster, larger files
cargoship create upload ./data --bucket my-bucket --compression-level 3

# Balanced (level 9) - default
cargoship create upload ./data --bucket my-bucket --compression-level 9

# Maximum compression (level 19) - slower, smallest files
cargoship create upload ./data --bucket my-bucket --compression-level 19
```

**Trade-offs**:
- Level 1-3: Fast, ~2x compression, 100+ MB/s
- Level 9-12: Balanced, ~4x compression, 30-50 MB/s (default: 9)
- Level 15-19: Maximum, ~5x compression, 10-20 MB/s

### Progress Monitoring

#### Real-Time Progress

```bash
cargoship create upload ./data --bucket my-bucket
# Shows real-time progress:
# ⣾ Uploading: 342/1000 files (34%) | 1.2 GB / 3.5 GB | 15.3 MB/s | ETA: 2m 34s
```

#### Quiet Mode

Minimal output:

```bash
cargoship create upload ./data --bucket my-bucket --quiet
# ✅ Upload complete: 1000 files, 3.5 GB
```

#### JSON Output

Machine-readable output:

```bash
cargoship create upload ./data --bucket my-bucket --json
{
  "status": "completed",
  "files": 1000,
  "bytes": 3758096384,
  "chunks": 35,
  "duration_seconds": 42.3,
  "throughput_mbps": 710.2,
  "manifest_url": "s3://my-bucket/uploads/20251215-abc123/manifest.json.gz"
}
```

---

## Use Cases

### Backup and Archival

```bash
# Daily backups with date-based prefixes
DATE=$(date +%Y-%m-%d)
cargoship create upload /var/backups \
  --bucket company-backups \
  --prefix daily/$DATE \
  --storage-class GLACIER_IR \
  --compression-level 15
```

### ML Dataset Distribution

```bash
# Upload training data
cargoship create upload ./imagenet-train \
  --bucket ml-datasets \
  --prefix imagenet/train \
  --chunk-size 500MB \
  --workers 16
```

### Log Aggregation

```bash
# Upload application logs
cargoship create upload /var/log/myapp \
  --bucket log-archive \
  --prefix logs/$(hostname)/$(date +%Y/%m/%d) \
  --storage-class STANDARD_IA \
  --compression-level 12  # High compression for text
```

### Research Data Archival

```bash
# Upload research datasets
cargoship create upload ./experiment-results \
  --bucket research-archive \
  --prefix projects/neuromorphic/exp-42 \
  --storage-class DEEP_ARCHIVE \
  --metadata "project=neuromorphic,experiment=42,researcher=alice"
```

---

## Multi-Region Uploads

### Basic Multi-Region

```bash
cargoship create upload ./data \
  --bucket my-bucket \
  --regions us-west-2,eu-west-1,ap-southeast-1
```

Features:
- Automatic region selection based on health and load
- Failover to healthy regions
- Session affinity for multi-part uploads

### Advanced Multi-Region Configuration

Create `~/.cargoship.yaml`:

```yaml
multi_region:
  enabled: true
  regions:
    - name: us-west-2
      weight: 50
      health_check_interval: 30s
    - name: eu-west-1
      weight: 30
      health_check_interval: 30s
    - name: ap-southeast-1
      weight: 20
      health_check_interval: 30s

  load_balancing:
    algorithm: least_connections
    sticky_sessions: true  # Required for multi-part uploads

  health_check:
    enabled: true
    timeout: 5s
    failure_threshold: 3
```

See [SESSION_AFFINITY.md](SESSION_AFFINITY.md) for detailed multi-region configuration.

---

## Performance Tips

### 1. Choose Appropriate Chunk Size

```bash
# Many small files (logs, code)
cargoship create upload ./logs --chunk-size 50MB

# Mixed file sizes (datasets)
cargoship create upload ./data --chunk-size 100MB

# Large files (videos, archives)
cargoship create upload ./videos --chunk-size 500MB
```

### 2. Optimize Worker Count

```bash
# Network bandwidth: 1 Gbps = ~125 MB/s
# Target: 80% utilization = 100 MB/s
# Per-worker throughput: ~10 MB/s
# Optimal workers: 100 / 10 = 10 workers

cargoship create upload ./data --workers 10
```

### 3. Use Appropriate Compression Level

```bash
# Already compressed data (images, videos, archives)
cargoship create upload ./media --compression-level 3

# Text data (logs, code, CSV)
cargoship create upload ./logs --compression-level 12
```

### 4. Leverage Incremental Sync

```bash
# Initial upload
cargoship create upload ./dataset --bucket my-bucket

# Later: only upload changes (much faster)
cargoship create upload ./dataset --bucket my-bucket
# Only changed/new files uploaded
```

### 5. Monitor with Observability

```bash
# Enable distributed tracing
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter jaeger \
  --tracing-endpoint http://localhost:14268/api/traces

# Enable Prometheus metrics
cargoship create upload ./data --bucket my-bucket \
  --prometheus-addr :9090
```

See [Distributed Tracing](../examples/tracing-demo.sh) for details.

---

## Troubleshooting

### Slow Uploads

**Problem**: Upload speed slower than expected

**Solutions**:
1. Increase worker count: `--workers 16`
2. Check network bandwidth: `speedtest-cli`
3. Use larger chunks: `--chunk-size 250MB`
4. Reduce compression: `--compression-level 3`

### Out of Memory

**Problem**: CargoShip crashes with OOM error

**Solutions**:
1. Reduce chunk size: `--chunk-size 50MB`
2. Reduce worker count: `--workers 2`
3. Check available memory: `free -h`

### Throttling Errors

**Problem**: S3 returns 503 SlowDown errors

**Solutions**:
1. Multi-prefix sharding enabled by default (check)
2. Reduce concurrency: `--workers 4`
3. Use exponential backoff (automatic)
4. Contact AWS to increase limits

### Failed Chunks

**Problem**: Some chunks fail to upload

**Solutions**:
1. Check AWS credentials: `aws sts get-caller-identity`
2. Verify bucket permissions: `aws s3 ls s3://bucket`
3. Check network connectivity
4. Retry automatically enabled (3 attempts)

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for more details.

---

## API Usage (Go Library)

### Basic Upload

```go
package main

import (
    "context"
    "github.com/scttfrdmn/cargoship/pkg/pipeline"
    "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

func main() {
    ctx := context.Background()

    // Create S3 transporter
    transporter, err := s3.NewOptimizedTransporter(&s3.Config{
        Bucket:       "my-bucket",
        Region:       "us-west-2",
        StorageClass: "STANDARD",
    })
    if err != nil {
        panic(err)
    }

    // Create pipeline
    p := pipeline.New(&pipeline.Config{
        SourcePath:      "/path/to/data",
        ChunkSizeBytes:  100 * 1024 * 1024, // 100MB
        Workers:         4,
        CompressionLevel: 9,
    })

    // Run upload
    result, err := p.Run(ctx, transporter)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Uploaded %d files (%d bytes) in %v\n",
        result.FilesProcessed,
        result.BytesProcessed,
        result.Duration)
}
```

### Advanced: Custom Progress Tracking

```go
// Implement ProgressReporter interface
type MyProgressReporter struct{}

func (r *MyProgressReporter) OnFileScanned(path string, size int64) {
    fmt.Printf("Scanned: %s (%d bytes)\n", path, size)
}

func (r *MyProgressReporter) OnChunkComplete(chunkID int, bytes int64) {
    fmt.Printf("Chunk %d complete: %d bytes\n", chunkID, bytes)
}

func (r *MyProgressReporter) OnUploadComplete(totalFiles int, totalBytes int64) {
    fmt.Printf("Upload complete: %d files, %d bytes\n", totalFiles, totalBytes)
}

// Use custom reporter
p := pipeline.New(&pipeline.Config{
    SourcePath:       "/path/to/data",
    ProgressReporter: &MyProgressReporter{},
})
```

---

## Best Practices

### 1. Use Appropriate Storage Classes

- **STANDARD**: Active data, frequent access
- **STANDARD_IA**: Infrequent access (>30 days between accesses)
- **GLACIER_IR**: Long-term archive, instant access
- **GLACIER**: Long-term archive, 1-5 minute access
- **DEEP_ARCHIVE**: Long-term archive, 12-hour access

### 2. Enable Incremental Sync

Always use the same bucket/prefix for repeat uploads to leverage deduplication:

```bash
# First upload
cargoship create upload ./data --bucket my-bucket --prefix dataset

# Future uploads (incremental)
cargoship create upload ./data --bucket my-bucket --prefix dataset
```

### 3. Monitor Costs

Use CargoShip's built-in cost estimation:

```bash
cargoship estimate ./data --storage-class GLACIER_IR
# Estimated cost: $42.50/month
```

### 4. Tag Uploads

Add metadata for organization:

```bash
cargoship create upload ./data --bucket my-bucket \
  --metadata "team=research,project=neuroscience,experiment=42"
```

### 5. Test First

Test uploads with small datasets:

```bash
# Test with 1GB sample
cargoship create upload ./sample-1gb --bucket test-bucket --dry-run
```

---

## References

- [Performance Benchmarks](PERFORMANCE_BENCHMARKS.md)
- [Optimization Guide](OPTIMIZATION_GUIDE.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [CLI Reference](CLI_REFERENCE.md)
- [Session Affinity](SESSION_AFFINITY.md)
- [Migration Guide](MIGRATION_FROM_RCLONE.md)

---

## Support

- **Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Discussions**: https://github.com/scttfrdmn/cargoship/discussions
- **Documentation**: https://github.com/scttfrdmn/cargoship/tree/main/docs
