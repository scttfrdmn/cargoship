# Migrating from rclone to CargoShip

**Version**: v0.6.2
**Last Updated**: December 2025

## Overview

This guide helps users transition from rclone to CargoShip for S3 uploads. While both tools transfer data to cloud storage, they use fundamentally different approaches. CargoShip's streaming pipeline architecture provides significant performance advantages for many use cases.

**TL;DR**: CargoShip is **5-10x faster** than rclone for typical backup and archival workloads.

---

## Table of Contents

1. [Why Migrate?](#why-migrate)
2. [Key Differences](#key-differences)
3. [Command Mapping](#command-mapping)
4. [Configuration Migration](#configuration-migration)
5. [Common Patterns](#common-patterns)
6. [Performance Comparison](#performance-comparison)
7. [Feature Comparison](#feature-comparison)
8. [Troubleshooting](#troubleshooting)

---

## Why Migrate?

### Performance

**Benchmark** (10,000 files, 20MB total):
- **rclone**: 45-60 seconds
- **CargoShip**: 1.2 seconds (**40-50x faster**)

### Cost Savings

**Example**: 1 million files upload
- **rclone**: 1,000,000 PUT requests = **$5.00**
- **CargoShip**: ~100 PUT requests = **$0.0005** (99.99% savings)

### Features

- **Incremental sync**: Only upload changed data
- **Deduplication**: Eliminate redundant transfers
- **Multi-prefix sharding**: Bypass S3 rate limits
- **Built-in compression**: Reduce storage costs
- **Observability**: Distributed tracing and metrics

---

## Key Differences

### Architecture

#### rclone: File-by-File Upload

```
┌──────────┐    ┌─────────┐
│   File   │ -> │   S3    │
│  Reader  │    │ Uploader│
└──────────┘    └─────────┘
     |               |
     v               v
 N files  ->    N uploads
```

- Each file = 1 S3 object
- N files = N PUT requests
- Individual file access

#### CargoShip: Streaming Pipeline

```
┌──────────┐    ┌─────────┐    ┌──────────┐    ┌─────────┐
│ Scanner  │ -> │ Chunker │ -> │ Archiver │ -> │ S3      │
│          │    │         │    │ (tar.zst)│    │ Uploader│
└──────────┘    └─────────┘    └──────────┘    └─────────┘
     |               |              |                |
     v               v              v                v
 N files  ->  M chunks  ->   M archives  ->   M uploads
```

- Files grouped into archives
- N files → M uploads (M << N)
- Compressed storage
- Manifest-based access

### Storage Format

#### rclone

```
s3://bucket/prefix/
├── file1.txt
├── file2.txt
├── file3.txt
└── ...
```

**Pros**:
- Individual file access
- Standard S3 objects
- Direct file serving

**Cons**:
- Many S3 requests
- Higher costs
- No deduplication

#### CargoShip

```
s3://bucket/prefix/uploads/<upload-id>/
├── shard-0/
│   ├── chunk-0.tar.zst
│   ├── chunk-1.tar.zst
│   └── ...
├── ...
└── manifest.json.gz
```

**Pros**:
- Fewer S3 requests
- Lower costs
- Built-in compression
- Deduplication
- Incremental sync

**Cons**:
- Requires extraction for individual files
- Not suitable for web serving

---

## Command Mapping

### Basic Upload

#### rclone

```bash
rclone copy /local/path remote:bucket/prefix
```

#### CargoShip

```bash
cargoship create upload /local/path \
  --bucket bucket \
  --prefix prefix \
  --region us-west-2
```

### Sync (Incremental)

#### rclone

```bash
rclone sync /local/path remote:bucket/prefix
```

#### CargoShip

```bash
# First upload
cargoship create upload /local/path --bucket bucket --prefix prefix

# Subsequent uploads (automatic incremental sync)
cargoship create upload /local/path --bucket bucket --prefix prefix
```

**Note**: CargoShip automatically detects changes and only uploads modified/new files.

### Copy with Progress

#### rclone

```bash
rclone copy /local/path remote:bucket/prefix --progress
```

#### CargoShip

```bash
cargoship create upload /local/path --bucket bucket --prefix prefix
# Progress shown by default
```

### List Files

#### rclone

```bash
rclone ls remote:bucket/prefix
```

#### CargoShip

```bash
# List uploads
cargoship list --bucket bucket --prefix prefix

# Download and view manifest
cargoship manifest show --bucket bucket --upload-id <id>
```

### Download/Restore

#### rclone

```bash
rclone copy remote:bucket/prefix /local/restore-path
```

#### CargoShip

```bash
cargoship download --bucket bucket --prefix prefix --output /local/restore-path
```

---

## Configuration Migration

### rclone Config

**rclone.conf**:
```ini
[myremote]
type = s3
provider = AWS
env_auth = true
region = us-west-2
```

### CargoShip Config

**~/.cargoship.yaml**:
```yaml
aws:
  region: us-west-2
  profile: default

upload:
  chunk_size_mb: 100
  workers: 8
  compression_level: 9

s3:
  storage_class: STANDARD
  multipart_threshold_mb: 16
```

### Environment Variables

#### rclone

```bash
export RCLONE_CONFIG=/path/to/rclone.conf
export RCLONE_S3_REGION=us-west-2
```

#### CargoShip

```bash
export AWS_REGION=us-west-2
export AWS_PROFILE=default
export CARGOSHIP_CONFIG=/path/to/.cargoship.yaml
```

---

## Common Patterns

### Pattern 1: Daily Backups

#### rclone

```bash
#!/bin/bash
DATE=$(date +%Y-%m-%d)
rclone sync /var/backups remote:backups/$DATE \
  --transfers 4 \
  --checkers 8 \
  --progress
```

#### CargoShip

```bash
#!/bin/bash
DATE=$(date +%Y-%m-%d)
cargoship create upload /var/backups \
  --bucket backups \
  --prefix $DATE \
  --storage-class GLACIER_IR \
  --quiet
```

**Improvements**:
- 10-50x faster
- Built-in compression (50-90% storage savings)
- Automatic deduplication
- Lower cost (fewer requests)

### Pattern 2: Large Dataset Upload

#### rclone

```bash
rclone copy ./imagenet remote:ml-datasets/imagenet \
  --transfers 16 \
  --multi-thread-streams 4 \
  --progress
```

#### CargoShip

```bash
cargoship create upload ./imagenet \
  --bucket ml-datasets \
  --prefix imagenet \
  --chunk-size 250MB \
  --workers 16
```

**Improvements**:
- Streaming pipeline (no disk usage)
- Better parallelism (multi-prefix sharding)
- Incremental sync for updates

### Pattern 3: Filtered Upload

#### rclone

```bash
rclone copy ./logs remote:log-archive/logs \
  --include "*.log" \
  --exclude "*.tmp"
```

#### CargoShip

```bash
# Use shell globbing
cargoship create upload ./logs/**/*.log \
  --bucket log-archive \
  --prefix logs

# Or pre-filter with find
find ./logs -name "*.log" -type f | \
  cargoship create upload --bucket log-archive --from-stdin
```

### Pattern 4: Bandwidth Limiting

#### rclone

```bash
rclone copy ./data remote:bucket/prefix \
  --bwlimit 10M
```

#### CargoShip

```bash
# Control via worker count
cargoship create upload ./data --bucket bucket --prefix prefix \
  --workers 2  # Limit parallelism

# Or use OS-level tools
trickle -s -u 10240 cargoship create upload ./data --bucket bucket
```

### Pattern 5: Dry Run

#### rclone

```bash
rclone copy ./data remote:bucket/prefix --dry-run
```

#### CargoShip

```bash
cargoship estimate ./data \
  --bucket bucket \
  --storage-class STANDARD
# Shows estimated size, cost, and duration
```

---

## Performance Comparison

### Benchmark Setup

- **Workload**: 10,000 files, 20MB total (2KB per file)
- **Network**: 1 Gbps
- **Region**: us-west-2
- **Platform**: macOS ARM64

### Results

| Tool | Duration | Throughput | Relative |
|------|----------|------------|----------|
| CargoShip | 1.17s | 171 Mbps | **1.0x (baseline)** |
| s5cmd | 8.17s | 19.6 Mbps | 7.0x slower |
| rclone | 45-60s | 3-4 Mbps | 38-51x slower |
| aws-cli | 63.47s | 2.5 Mbps | 54x slower |

### Why CargoShip is Faster

1. **Request Reduction**: 10 uploads vs 10,000
2. **Streaming Pipeline**: No disk I/O bottleneck
3. **Multi-Prefix Sharding**: Parallel uploads across prefixes
4. **Optimized Compression**: In-memory zstd compression
5. **HTTP/2**: Connection multiplexing and header compression

### When rclone May Be Better

- **Individual file serving**: Files accessed directly from S3 (e.g., static websites)
- **Cloud-to-cloud transfers**: rclone supports 40+ providers
- **Mount operations**: rclone mount for FUSE filesystem
- **Bidirectional sync**: rclone sync with conflict resolution

---

## Feature Comparison

| Feature | rclone | CargoShip |
|---------|--------|-----------|
| **Upload Speed** | Moderate | Very Fast (10-50x) |
| **Storage Cost** | Standard | Lower (compression) |
| **Transfer Cost** | N requests | N/1000 requests |
| **Incremental Sync** | Hash-based | Manifest-based |
| **Deduplication** | No | Yes (content-based) |
| **Compression** | No | Yes (zstd) |
| **Multi-Prefix** | No | Yes (8 prefixes) |
| **Individual File Access** | Yes | No (requires extraction) |
| **Cloud Providers** | 40+ | S3 only |
| **Mount/FUSE** | Yes | No |
| **Observability** | Basic | Advanced (traces, metrics) |
| **Memory Usage** | Low | Moderate |
| **Disk Usage** | Moderate | Zero (streaming) |

---

## Migration Checklist

### Before Migration

- [ ] **Identify use case**: Archival/backup (CargoShip) or file serving (rclone)
- [ ] **Test with sample data**: Run small test uploads
- [ ] **Review storage format**: Understand tar.zst archive structure
- [ ] **Check network bandwidth**: Ensure sufficient capacity
- [ ] **Verify AWS credentials**: Test with `aws sts get-caller-identity`

### During Migration

- [ ] **Run parallel**: Keep rclone running during initial testing
- [ ] **Compare performance**: Measure upload times
- [ ] **Verify data integrity**: Check uploaded archives
- [ ] **Test restore**: Ensure you can extract data
- [ ] **Monitor costs**: Compare request counts

### After Migration

- [ ] **Update scripts**: Replace rclone commands
- [ ] **Update documentation**: Document new workflow
- [ ] **Train team**: Share CargoShip best practices
- [ ] **Set up monitoring**: Enable tracing/metrics if needed

---

## Hybrid Approach

You can use both tools for different purposes:

### Use CargoShip For:
- ✅ Backups and archives
- ✅ Large dataset uploads
- ✅ Log aggregation
- ✅ Database backups
- ✅ ML model storage
- ✅ Research data archival

### Use rclone For:
- ✅ Static website hosting
- ✅ Individual file serving
- ✅ Cloud-to-cloud transfers (non-S3)
- ✅ FUSE filesystem mounts
- ✅ Bidirectional sync with conflict resolution

### Example Hybrid Script

```bash
#!/bin/bash

# Use CargoShip for bulk data
cargoship create upload ./datasets --bucket my-bucket --prefix datasets/v1

# Use rclone for web assets
rclone sync ./public remote:website-bucket/public \
  --transfers 8 \
  --delete-after
```

---

## Troubleshooting

### "Cannot access individual files"

**Problem**: Need to access specific files from CargoShip archives

**Solution**: Download and extract archives

```bash
# Download specific chunk
aws s3 cp s3://bucket/prefix/uploads/<id>/shard-0/chunk-0.tar.zst .

# Extract specific file
tar --use-compress-program=unzstd -xf chunk-0.tar.zst path/to/file.txt
```

Or use CargoShip's restore command:

```bash
cargoship download --bucket bucket --prefix prefix \
  --output /restore/path \
  --file path/to/specific/file.txt
```

### "Upload slower than rclone"

**Problem**: CargoShip not performing better

**Possible causes**:
1. Already compressed data (images, videos)
2. Very few large files
3. Under-configured workers
4. Network bottleneck

**Solutions**:

```bash
# For pre-compressed data
cargoship create upload ./media --compression-level 3

# For large files
cargoship create upload ./videos --chunk-size 500MB

# Increase workers
cargoship create upload ./data --workers 16
```

### "Want rclone's --dry-run"

**Problem**: Need to preview upload before executing

**Solution**: Use CargoShip's estimate command

```bash
cargoship estimate ./data --bucket bucket --storage-class STANDARD

# Output:
# Files: 10,000
# Total Size: 1.2 GB
# Compressed: 480 MB (60% reduction)
# Chunks: 5
# Estimated Cost: $0.00001 (requests) + $0.011/month (storage)
# Estimated Duration: 2-3 seconds
```

### "Missing rclone's bandwidth limit"

**Problem**: Need to limit upload bandwidth

**Solutions**:

```bash
# Option 1: Reduce workers
cargoship create upload ./data --workers 2

# Option 2: Use trickle (Linux/macOS)
trickle -s -u 10240 cargoship create upload ./data --bucket bucket

# Option 3: Use TC (Linux traffic control)
sudo tc qdisc add dev eth0 root tbf rate 10mbit burst 32kbit latency 400ms
```

---

## Additional Resources

- [S3 Direct Upload Guide](S3_DIRECT_UPLOAD.md)
- [Optimization Guide](OPTIMIZATION_GUIDE.md)
- [Performance Benchmarks](PERFORMANCE_BENCHMARKS.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [CLI Reference](CLI_REFERENCE.md)

---

## Getting Help

### CargoShip Support

- **Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Discussions**: https://github.com/scttfrdmn/cargoship/discussions
- **Documentation**: https://github.com/scttfrdmn/cargoship/tree/main/docs

### rclone Resources

- **Forum**: https://forum.rclone.org/
- **Documentation**: https://rclone.org/docs/

---

## Conclusion

CargoShip and rclone serve different use cases:

**Choose CargoShip** when you need:
- Maximum upload performance
- Cost optimization
- Archival and backup workflows
- Deduplication and incremental sync
- Built-in compression

**Choose rclone** when you need:
- Individual file access from S3
- Multi-cloud support (non-S3 providers)
- FUSE filesystem mounting
- Bidirectional sync with conflict resolution

For many archival and backup use cases, **migrating to CargoShip can provide 10-50x performance improvements** and **90%+ cost reductions**.
