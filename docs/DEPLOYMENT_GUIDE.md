# Production Deployment Guide

**CargoShip v0.6.2**
**Last Updated**: December 2025

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Initial Configuration](#initial-configuration)
4. [Performance Tuning](#performance-tuning)
5. [Monitoring & Observability](#monitoring--observability)
6. [Security Best Practices](#security-best-practices)
7. [Production Checklist](#production-checklist)
8. [Troubleshooting](#troubleshooting)
9. [Common Deployment Patterns](#common-deployment-patterns)
10. [Scaling Guidelines](#scaling-guidelines)

---

## Prerequisites

### AWS Account Requirements

**IAM Permissions** (minimum required):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectAcl",
        "s3:GetObject",
        "s3:GetObjectAcl",
        "s3:ListBucket",
        "s3:DeleteObject",
        "s3:GetBucketLocation"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name",
        "arn:aws:s3:::your-bucket-name/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:ListAllMyBuckets"
      ],
      "Resource": "*"
    }
  ]
}
```

**Additional permissions** (for advanced features):

```json
{
  "Effect": "Allow",
  "Action": [
    "kms:Decrypt",
    "kms:Encrypt",
    "kms:GenerateDataKey",
    "cloudwatch:PutMetricData",
    "cloudwatch:GetMetricData",
    "s3:PutLifecycleConfiguration",
    "s3:GetLifecycleConfiguration"
  ],
  "Resource": "*"
}
```

### System Requirements

#### Minimum Configuration
- **OS**: Linux (kernel 4.4+), macOS 10.15+, Windows 10+
- **CPU**: 2 cores
- **RAM**: 4 GB
- **Disk**: 1 GB free space (for binary and temp files)
- **Network**: 10 Mbps uplink

#### Recommended Configuration
- **OS**: Linux (kernel 5.4+) with io_uring support
- **CPU**: 8+ cores
- **RAM**: 16 GB
- **Disk**: 10 GB free space (SSD/NVMe preferred)
- **Network**: 1 Gbps uplink

#### High-Performance Configuration
- **OS**: Linux (kernel 5.10+) with BBR congestion control
- **CPU**: 16+ cores (multi-socket for NUMA optimization)
- **RAM**: 32 GB+
- **Disk**: 50 GB NVMe SSD
- **Network**: 10 Gbps uplink

### Network Requirements

- **Bandwidth**: Adequate upload bandwidth to S3 (1 Gbps recommended)
- **Latency**: <100ms to AWS region (use `ping s3.{region}.amazonaws.com`)
- **Firewall**: Allow HTTPS (443) to S3 endpoints
- **DNS**: Reliable DNS resolution for S3 endpoints

### Software Dependencies

CargoShip has minimal dependencies (statically linked), but for optimal performance:

- **Linux**: Install `zstd` tools for manual archive inspection
- **macOS**: Install `gdate` (GNU date) for precise timing in scripts
- **All platforms**: AWS CLI v2 for complementary operations

---

## Installation

### Option 1: Package Manager (Recommended)

#### Homebrew (macOS/Linux)
```bash
brew install scttfrdmn/tap/cargoship
```

#### Scoop (Windows)
```bash
scoop bucket add scttfrdmn https://github.com/scttfrdmn/scoop-bucket
scoop install cargoship
```

### Option 2: Go Install
```bash
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

### Option 3: Binary Download
```bash
# Linux (x86_64)
curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64 -o cargoship
chmod +x cargoship
sudo mv cargoship /usr/local/bin/

# macOS (ARM64)
curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-arm64 -o cargoship
chmod +x cargoship
sudo mv cargoship /usr/local/bin/
```

### Verify Installation
```bash
cargoship --version
# Output: cargoship version 0.6.2

cargoship --help
```

---

## Initial Configuration

### AWS Credentials

CargoShip uses AWS SDK credential chain (same as AWS CLI):

#### Option 1: Environment Variables (CI/CD)
```bash
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

#### Option 2: AWS Profile (Local Development)
```bash
export AWS_PROFILE=production
export AWS_REGION=us-west-2
```

#### Option 3: IAM Instance Profile (EC2)
No configuration needed—CargoShip automatically uses instance metadata.

### Configuration File

Create `~/.cargoship.yaml` for persistent settings:

```yaml
# Global defaults
aws:
  region: us-west-2
  profile: default

# Upload configuration
upload:
  chunk_size_mb: 200
  workers: 8
  shards: 8
  compression_level: 9
  storage_class: STANDARD

# S3 configuration
s3:
  multipart_threshold_mb: 16
  part_size_mb: 5
  max_retries: 3

# Budget management
budget:
  enabled: true
  max_budget_usd: 10000
  max_volume_gb: 50000
  alert_threshold_percent: 80

# Observability (optional)
observability:
  tracing_enabled: false
  prometheus_addr: ""
  cloudwatch_enabled: false
```

### Test Configuration

Verify your setup works:

```bash
# Create small test file
echo "Hello CargoShip" > test.txt

# Upload to S3
cargoship create upload test.txt \
  --bucket YOUR_BUCKET \
  --prefix test \
  --region us-west-2

# Check upload succeeded
aws s3 ls s3://YOUR_BUCKET/test/uploads/
```

If successful, you'll see output like:
```
🚢 Uploading: 1 files | 15 B | 1 chunks | 0.5 MB/s | 0.1s elapsed
✅ Upload Complete!
   Files:       1 files
   Data Size:   15 B
   Upload ID:   20251215-abc123
```

---

## Performance Tuning

### Workload-Based Configuration

#### Small Files (<1 MB) - Many Files

**Characteristics**: 10,000+ files, each <1 MB
**Bottleneck**: S3 request overhead, chunking efficiency

**Optimal Configuration**:
```bash
cargoship create upload ./logs \
  --bucket my-bucket \
  --chunk-size 50MB \
  --workers 8 \
  --shards 8 \
  --compression-level 9
```

**Expected Performance**:
- Throughput: 150-200 MB/s
- Files/second: 8,000-10,000
- Memory usage: 2-4 GB

#### Medium Files (1-100 MB) - Mixed Workload

**Characteristics**: Mixed file sizes, typical dataset
**Bottleneck**: Network bandwidth, compression

**Optimal Configuration**:
```bash
cargoship create upload ./dataset \
  --bucket my-bucket \
  --chunk-size 200MB \
  --workers 8 \
  --shards 8 \
  --compression-level 9
```

**Expected Performance**:
- Throughput: 100-150 MB/s
- Memory usage: 4-8 GB

#### Large Files (>100 MB) - Bulk Data

**Characteristics**: Few large files (videos, archives, databases)
**Bottleneck**: Compression, disk I/O

**Optimal Configuration**:
```bash
cargoship create upload ./videos \
  --bucket my-bucket \
  --chunk-size 500MB \
  --workers 4 \
  --shards 8 \
  --compression-level 3  # Fast compression for already-compressed data
```

**Expected Performance**:
- Throughput: 150-200 MB/s
- Memory usage: 4-6 GB

### Network-Based Tuning

#### Residential Broadband (50-100 Mbps)
```bash
cargoship create upload ./data \
  --bucket my-bucket \
  --workers 2 \
  --shards 4
```

#### Business Connection (1 Gbps)
```bash
cargoship create upload ./data \
  --bucket my-bucket \
  --workers 8 \
  --shards 8
```

#### Datacenter (10+ Gbps)
```bash
cargoship create upload ./data \
  --bucket my-bucket \
  --workers 16 \
  --shards 16
```

### Compression Level Selection

| Level | Speed | Ratio | CPU Usage | Use Case |
|-------|-------|-------|-----------|----------|
| 1-3 | Very Fast | 2-3x | Low | Already compressed data, fast uploads |
| 6-9 | Balanced | 4-5x | Medium | General purpose (default: 9) |
| 12-15 | Slow | 5-6x | High | Text/logs, archival data |
| 16-19 | Very Slow | 6-7x | Very High | Maximum compression for long-term storage |

**Examples**:
```bash
# Fast compression for media files
cargoship create upload ./videos --compression-level 3

# Balanced compression (default)
cargoship create upload ./data --compression-level 9

# Maximum compression for text data
cargoship create upload ./logs --compression-level 15
```

### Storage Class Optimization

Select storage class based on access patterns:

```bash
# Active data (frequent access)
cargoship create upload ./active \
  --storage-class STANDARD

# Infrequent access (monthly)
cargoship create upload ./archive \
  --storage-class STANDARD_IA

# Long-term archive (rare access)
cargoship create upload ./compliance \
  --storage-class GLACIER_IR

# Compliance archive (never accessed unless required)
cargoship create upload ./legal-hold \
  --storage-class DEEP_ARCHIVE
```

**Cost Comparison** (us-east-1, per GB-month):
- STANDARD: $0.023
- STANDARD_IA: $0.0125 (+ $0.01/GB retrieval)
- GLACIER_IR: $0.004 (+ $0.03/GB retrieval)
- GLACIER: $0.0036 (+ retrieval fees + time)
- DEEP_ARCHIVE: $0.00099 (+ retrieval fees + 12h delay)

### Multi-Region Configuration

For high availability and disaster recovery:

```yaml
# ~/.cargoship.yaml
multi_region:
  enabled: true
  regions:
    - name: us-west-2
      weight: 50
      health_check_interval: 30s
    - name: us-east-1
      weight: 30
      health_check_interval: 30s
    - name: eu-west-1
      weight: 20
      health_check_interval: 30s

  load_balancing:
    algorithm: least_connections
    sticky_sessions: true

  health_check:
    enabled: true
    timeout: 5s
    failure_threshold: 3
```

See [SESSION_AFFINITY.md](SESSION_AFFINITY.md) for detailed multi-region configuration.

---

## Monitoring & Observability

### Real-Time Progress

CargoShip automatically displays real-time progress in terminal (TTY) mode:

```bash
cargoship create upload ./data --bucket my-bucket

# Output:
# 🚢 Uploading: 1,234 files | 5.67 GB | 89 chunks | 123.4 MB/s | 1m30s elapsed
```

**Features**:
- Live file count, data size, chunk count
- Current throughput (MB/s)
- Elapsed time
- Automatically disabled in non-TTY contexts (pipes, CI/CD)

### Prometheus Metrics

Enable Prometheus metrics endpoint:

```bash
cargoship create upload ./data --bucket my-bucket --prometheus-addr :9090
```

**Available Metrics**:
- `cargoship_upload_bytes_total` - Total bytes uploaded
- `cargoship_upload_files_total` - Total files processed
- `cargoship_upload_chunks_total` - Total chunks created
- `cargoship_upload_duration_seconds` - Upload duration
- `cargoship_upload_throughput_mbps` - Current throughput
- `cargoship_shard_uploads_total{shard="N"}` - Uploads per shard
- `cargoship_shard_bytes_total{shard="N"}` - Bytes per shard
- `cargoship_shard_errors_total{shard="N"}` - Errors per shard
- `cargoship_retry_attempts_total` - Total retry attempts

**Query Examples**:
```promql
# Average upload throughput (last 5m)
rate(cargoship_upload_bytes_total[5m]) / 1024 / 1024

# Shard balance (uploads per shard)
sum by (shard) (rate(cargoship_shard_uploads_total[5m]))

# Error rate
rate(cargoship_shard_errors_total[5m]) / rate(cargoship_shard_uploads_total[5m])
```

### Distributed Tracing

Enable OpenTelemetry tracing for deep visibility:

```bash
# Jaeger
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter jaeger \
  --tracing-endpoint http://localhost:14268/api/traces

# OTLP (OpenTelemetry Protocol)
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter otlp \
  --tracing-endpoint http://localhost:4318

# Stdout (debugging)
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter stdout
```

**Trace Hierarchy**:
```
upload-request (root span)
├── scanner-stage
├── chunker-stage
├── archiver-stage
└── uploader-stage
    ├── shard-0
    │   ├── chunk-0-upload
    │   │   ├── s3-put-object
    │   │   └── retry-1 (if needed)
    │   └── chunk-8-upload
    └── shard-7
        └── chunk-7-upload
```

**Tracing Setup** (Jaeger example):
```bash
# Run Jaeger
docker run -d \
  --name jaeger \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest

# Upload with tracing
cargoship create upload ./data --bucket my-bucket \
  --tracing \
  --tracing-exporter jaeger \
  --tracing-endpoint http://localhost:14268/api/traces

# View traces at http://localhost:16686
```

### Budget Monitoring

Track costs in real-time:

```bash
# Set budget limits
cargoship budget set \
  --max-budget 1000 \
  --max-volume-gb 5000 \
  --alert-threshold 80

# Check current status
cargoship budget status

# Output:
# 💰 Budget Status
# Current Period (December 2025):
# ├── Spend: $461.06 / $1,000 (46%)
# └── Volume: 3.2 TB / 5 TB (64%)
#
# Forecast (30-day):
# ├── Estimated spend: $892
# └── Under budget by $108 ✅
```

### CloudWatch Integration

Enable CloudWatch metrics:

```yaml
# ~/.cargoship.yaml
observability:
  cloudwatch_enabled: true
  cloudwatch_namespace: CargoShip/Production
  cloudwatch_region: us-west-2
```

**Custom Metrics Published**:
- `UploadThroughput` (MB/s)
- `UploadDuration` (seconds)
- `FilesProcessed` (count)
- `BytesUploaded` (bytes)
- `ErrorRate` (percent)
- `ShardBalance` (distribution metric)

**CloudWatch Alarms** (recommended):
```bash
# High error rate
aws cloudwatch put-metric-alarm \
  --alarm-name cargoship-high-error-rate \
  --metric-name ErrorRate \
  --namespace CargoShip/Production \
  --statistic Average \
  --period 300 \
  --threshold 5 \
  --comparison-operator GreaterThanThreshold

# Low throughput
aws cloudwatch put-metric-alarm \
  --alarm-name cargoship-low-throughput \
  --metric-name UploadThroughput \
  --namespace CargoShip/Production \
  --statistic Average \
  --period 300 \
  --threshold 50 \
  --comparison-operator LessThanThreshold
```

---

## Security Best Practices

### 1. Credential Management

**DO**:
- Use IAM instance profiles on EC2
- Use IAM roles for Lambda/ECS
- Rotate access keys every 90 days
- Use temporary credentials (STS) when possible

**DON'T**:
- Hardcode credentials in scripts
- Commit credentials to version control
- Share credentials across environments
- Use root account credentials

### 2. S3 Bucket Security

**Enable Encryption** (server-side):
```bash
# SSE-S3 (default, automatic)
cargoship create upload ./data --bucket my-bucket

# SSE-KMS (customer-managed keys)
cargoship create upload ./data --bucket my-bucket \
  --kms-key-id arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012
```

**Bucket Policy** (restrict access):
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": "arn:aws:s3:::my-bucket/*",
      "Condition": {
        "Bool": {
          "aws:SecureTransport": "false"
        }
      }
    }
  ]
}
```

### 3. Network Security

**VPC Endpoints** (recommended for EC2):
```bash
# Create S3 VPC endpoint
aws ec2 create-vpc-endpoint \
  --vpc-id vpc-12345678 \
  --service-name com.amazonaws.us-west-2.s3 \
  --route-table-ids rtb-12345678
```

**Benefits**:
- No internet gateway required
- Lower latency
- No data transfer charges
- Enhanced security (traffic stays in AWS network)

### 4. Audit Logging

Enable S3 access logging:

```bash
# Create logging bucket
aws s3 mb s3://my-bucket-logs

# Enable logging
aws s3api put-bucket-logging \
  --bucket my-bucket \
  --bucket-logging-status file://logging.json
```

**logging.json**:
```json
{
  "LoggingEnabled": {
    "TargetBucket": "my-bucket-logs",
    "TargetPrefix": "cargoship-logs/"
  }
}
```

### 5. Data Integrity

CargoShip automatically validates data integrity using SHA-256 checksums. For additional validation:

```bash
# Enable content-MD5 validation
cargoship create upload ./data --bucket my-bucket --validate-checksums

# Download and verify manifest
aws s3 cp s3://my-bucket/prefix/uploads/20251215-abc123/manifest.json.gz .
gunzip manifest.json.gz
jq '.chunks[].checksum' manifest.json
```

---

## Production Checklist

### Pre-Deployment
- [ ] AWS credentials configured and tested (`aws sts get-caller-identity`)
- [ ] S3 bucket created with appropriate region
- [ ] IAM permissions validated (minimum: s3:PutObject, s3:GetObject, s3:ListBucket)
- [ ] Network connectivity verified (ping S3 endpoint, check latency)
- [ ] CargoShip installed and version confirmed (`cargoship --version`)
- [ ] Configuration file created (`~/.cargoship.yaml`)
- [ ] Test upload successful (`cargoship create upload test.txt ...`)

### Security
- [ ] IAM instance profile configured (EC2) or credentials properly managed
- [ ] S3 bucket encryption enabled (SSE-S3 or SSE-KMS)
- [ ] Bucket policy restricts access appropriately
- [ ] VPC endpoint configured (if on EC2)
- [ ] Access logging enabled
- [ ] Credentials rotation schedule documented

### Performance
- [ ] Workload characteristics analyzed (file sizes, count, patterns)
- [ ] Optimal configuration determined (chunk size, workers, shards)
- [ ] Network bandwidth verified (`speedtest-cli`)
- [ ] System resources adequate (CPU, RAM, disk)
- [ ] Compression level appropriate for data type
- [ ] Storage class selected based on access patterns

### Monitoring
- [ ] Prometheus metrics enabled (if using Prometheus)
- [ ] CloudWatch metrics enabled (if using CloudWatch)
- [ ] Budget limits configured (`cargoship budget set`)
- [ ] Alerts configured (error rate, throughput, budget)
- [ ] Log aggregation configured (if needed)
- [ ] Distributed tracing configured (if needed)

### Operational Readiness
- [ ] Backup and restore procedures documented
- [ ] Disaster recovery plan in place
- [ ] Incident response runbook created
- [ ] On-call rotation configured
- [ ] Escalation paths documented
- [ ] Capacity planning completed

### Documentation
- [ ] Architecture diagram created
- [ ] Configuration documented
- [ ] Troubleshooting guide available
- [ ] Team training completed
- [ ] Contact information updated

---

## Troubleshooting

### Issue: Slow Upload Speed

**Symptoms**: Throughput significantly below network capacity

**Diagnosis**:
```bash
# Check network bandwidth
speedtest-cli

# Check current throughput
cargoship create upload ./data --bucket my-bucket
# Look at real-time MB/s metric

# Check system resources
top
iostat -x 1
```

**Solutions**:
1. **Increase workers** if CPU/network underutilized:
   ```bash
   cargoship create upload ./data --bucket my-bucket --workers 16
   ```

2. **Increase shards** if hitting S3 rate limits:
   ```bash
   cargoship create upload ./data --bucket my-bucket --shards 16
   ```

3. **Reduce compression level** if CPU-bound:
   ```bash
   cargoship create upload ./data --bucket my-bucket --compression-level 3
   ```

4. **Check for throttling** in CloudWatch metrics or S3 access logs

### Issue: High Memory Usage

**Symptoms**: Out of memory errors, system swapping

**Diagnosis**:
```bash
# Monitor memory during upload
watch -n 1 free -h

# Check memory profile
cargoship create upload ./data --bucket my-bucket --profile-mem
```

**Solutions**:
1. **Reduce chunk size**:
   ```bash
   cargoship create upload ./data --bucket my-bucket --chunk-size 100MB
   ```

2. **Reduce workers**:
   ```bash
   cargoship create upload ./data --bucket my-bucket --workers 4
   ```

3. **Reduce shards** (fewer concurrent uploads):
   ```bash
   cargoship create upload ./data --bucket my-bucket --shards 4
   ```

**Expected Memory Usage**:
- Formula: `chunk_size × workers × 2`
- Example: 200MB chunks × 8 workers × 2 = ~3.2 GB

### Issue: Upload Failures / Errors

**Symptoms**: Upload fails with errors, high retry rate

**Common Errors**:

#### AccessDenied (403)
**Cause**: Insufficient IAM permissions
**Solution**:
```bash
# Check current identity
aws sts get-caller-identity

# Verify bucket permissions
aws s3 ls s3://my-bucket/

# Add required permissions to IAM policy
```

#### SlowDown (503)
**Cause**: S3 rate limit exceeded
**Solution**:
```bash
# Reduce concurrency
cargoship create upload ./data --bucket my-bucket --workers 4 --shards 8

# Add retry delay
cargoship create upload ./data --bucket my-bucket --retry-delay 2s
```

#### RequestTimeout
**Cause**: Network issues, large chunks, slow uploads
**Solution**:
```bash
# Reduce chunk size
cargoship create upload ./data --bucket my-bucket --chunk-size 100MB

# Increase timeout
cargoship create upload ./data --bucket my-bucket --timeout 300s

# Check network connectivity
ping s3.us-west-2.amazonaws.com
```

### Issue: Budget Exceeded

**Symptoms**: Budget alert triggered, uploads blocked

**Diagnosis**:
```bash
# Check current budget status
cargoship budget status

# View project costs
cargoship cost projects

# Forecast next month
cargoship cost forecast
```

**Solutions**:
1. **Increase budget limit**:
   ```bash
   cargoship budget set --max-budget 2000
   ```

2. **Optimize storage class**:
   ```bash
   cargoship estimate ./data --show-breakdown
   # Use recommended storage classes
   ```

3. **Enable lifecycle policies**:
   ```bash
   cargoship lifecycle --bucket my-bucket --template archive-optimization
   ```

### Issue: Manifest Not Found

**Symptoms**: Cannot list or restore uploads

**Diagnosis**:
```bash
# List manifests
aws s3 ls s3://my-bucket/prefix/uploads/ --recursive | grep manifest

# Check upload ID
cargoship list --bucket my-bucket --prefix prefix
```

**Solutions**:
1. **Verify upload completed** successfully
2. **Check S3 path** is correct
3. **Verify IAM permissions** for s3:GetObject on manifest
4. **Manual manifest download**:
   ```bash
   aws s3 cp s3://my-bucket/prefix/uploads/20251215-abc123/manifest.json.gz .
   ```

### Enable Debug Logging

For detailed troubleshooting:

```bash
# Enable debug mode
export CARGOSHIP_LOG_LEVEL=debug
cargoship create upload ./data --bucket my-bucket

# Enable trace logging
export CARGOSHIP_LOG_LEVEL=trace
cargoship create upload ./data --bucket my-bucket
```

---

## Common Deployment Patterns

### Pattern 1: Daily Backups

```bash
#!/bin/bash
# daily-backup.sh

DATE=$(date +%Y-%m-%d)
SOURCE="/var/backups"
BUCKET="company-backups"
PREFIX="daily/$DATE"

# Upload with incremental sync (only changed files)
cargoship create upload "$SOURCE" \
  --bucket "$BUCKET" \
  --prefix "$PREFIX" \
  --storage-class GLACIER_IR \
  --compression-level 12 \
  --quiet

# Apply lifecycle policy (transition to DEEP_ARCHIVE after 90 days)
cargoship lifecycle \
  --bucket "$BUCKET" \
  --prefix daily/ \
  --template backup-retention

# Check budget
cargoship budget status
```

### Pattern 2: Large Dataset Migration

```bash
#!/bin/bash
# migrate-dataset.sh

# Estimate costs first
cargoship estimate /data/large-dataset \
  --storage-class GLACIER_IR \
  --show-breakdown

# Upload with optimal configuration
cargoship create upload /data/large-dataset \
  --bucket research-archive \
  --prefix datasets/2025/project-alpha \
  --storage-class GLACIER_IR \
  --chunk-size 500MB \
  --workers 16 \
  --shards 8 \
  --compression-level 9

# Verify upload
cargoship list --bucket research-archive --prefix datasets/2025/project-alpha
```

### Pattern 3: Multi-Tier Archive

```bash
#!/bin/bash
# multi-tier-archive.sh

# Tier 1: Critical data (STANDARD, fast access)
cargoship create upload ./critical \
  --bucket production \
  --prefix tier-1-critical \
  --storage-class STANDARD \
  --chunk-size 50MB

# Tier 2: Important data (STANDARD_IA, occasional access)
cargoship create upload ./important \
  --bucket production \
  --prefix tier-2-important \
  --storage-class STANDARD_IA \
  --chunk-size 100MB

# Tier 3: Archive data (GLACIER_IR, rare access)
cargoship create upload ./archive \
  --bucket production \
  --prefix tier-3-archive \
  --storage-class GLACIER_IR \
  --chunk-size 250MB \
  --compression-level 12

# Tier 4: Compliance data (DEEP_ARCHIVE, never accessed)
cargoship create upload ./compliance \
  --bucket production \
  --prefix tier-4-compliance \
  --storage-class DEEP_ARCHIVE \
  --chunk-size 500MB \
  --compression-level 19

# Apply lifecycle policies
cargoship lifecycle --bucket production --prefix tier-1-critical \
  --custom-transitions "30:STANDARD_IA,90:GLACIER_IR"

cargoship lifecycle --bucket production --prefix tier-2-important \
  --custom-transitions "90:GLACIER_IR,365:DEEP_ARCHIVE"
```

### Pattern 4: Continuous Log Aggregation

```bash
#!/bin/bash
# log-aggregation.sh

# Run hourly via cron
HOUR=$(date +%Y-%m-%d-%H)
LOGS_DIR="/var/log/app"
BUCKET="log-archive"

# Upload logs with high compression (text data)
cargoship create upload "$LOGS_DIR" \
  --bucket "$BUCKET" \
  --prefix "logs/$HOUR" \
  --storage-class STANDARD_IA \
  --compression-level 15 \
  --chunk-size 50MB \
  --workers 4 \
  --quiet

# Apply log retention policy (delete after 90 days)
cargoship lifecycle --bucket "$BUCKET" --prefix logs/ \
  --template log-aggregation
```

### Pattern 5: Disaster Recovery Setup

```bash
#!/bin/bash
# dr-setup.sh

# Multi-region upload for disaster recovery
cargoship create upload /data/critical \
  --bucket dr-primary \
  --region us-west-2 \
  --storage-class STANDARD \
  --chunk-size 200MB \
  --workers 8

# Replicate to DR region (using S3 replication)
aws s3api put-bucket-replication \
  --bucket dr-primary \
  --replication-configuration file://replication.json

# Or manual cross-region upload
cargoship create upload /data/critical \
  --bucket dr-secondary \
  --region us-east-1 \
  --storage-class STANDARD \
  --chunk-size 200MB \
  --workers 8
```

---

## Scaling Guidelines

### Vertical Scaling (Single Instance)

As workload grows, increase instance resources:

| Workload | CPU | RAM | Network | Configuration |
|----------|-----|-----|---------|---------------|
| **Small** (<100 GB/day) | 2 cores | 4 GB | 100 Mbps | workers=2, shards=4 |
| **Medium** (100 GB - 1 TB/day) | 8 cores | 16 GB | 1 Gbps | workers=8, shards=8 |
| **Large** (1-10 TB/day) | 16 cores | 32 GB | 10 Gbps | workers=16, shards=16 |
| **Enterprise** (>10 TB/day) | 32+ cores | 64+ GB | 25+ Gbps | workers=32, shards=16 |

### Horizontal Scaling (Multiple Instances)

For extremely large workloads, distribute across multiple instances:

```bash
# Instance 1: Upload directory A
cargoship create upload /data/dir-a --bucket my-bucket --prefix dir-a

# Instance 2: Upload directory B
cargoship create upload /data/dir-b --bucket my-bucket --prefix dir-b

# Instance 3: Upload directory C
cargoship create upload /data/dir-c --bucket my-bucket --prefix dir-c
```

**Benefits**:
- Linear scaling of throughput
- Independent failure domains
- Parallel processing of multiple datasets

**Considerations**:
- Coordinate upload IDs to avoid conflicts
- Aggregate budget tracking across instances
- Centralized monitoring required

### Auto-Scaling (Future)

Planned for v0.7.0+:
- Dynamic worker adjustment based on available bandwidth
- Automatic shard count optimization
- Adaptive chunk size based on file distribution
- Self-tuning compression levels

---

## Performance Expectations

Based on v0.6.2 testing with real AWS S3:

### Benchmark Results

| Workload | Files | Size | Duration | Throughput | Compression | Memory |
|----------|-------|------|----------|------------|-------------|--------|
| **Small files** | 10,000 | 200 MB | 1.2s | 167 MB/s | N/A | 156 MB |
| **Medium files** | 1,000 | 5 GB | 38s | 135 MB/s | 3.2:1 | 2.1 GB |
| **Large files** | 100 | 56 GB | 311s | 185 MB/s | 1.8:1 | 4.9 GB |

### Compression Performance

| Algorithm | Level | Speed | Ratio | Use Case |
|-----------|-------|-------|-------|----------|
| **zstd** | 3 | 527 MB/s | 2-3x | Already compressed, fast uploads |
| **zstd** | 9 | 150 MB/s | 4-5x | General purpose (default) |
| **zstd** | 15 | 45 MB/s | 5-6x | Text data, archival |
| **zstd** | 19 | 18 MB/s | 6-7x | Maximum compression, compliance |

### Competitive Comparison

| Tool | Duration | Throughput | Relative Performance |
|------|----------|------------|---------------------|
| **CargoShip** | 1.17s | 171 MB/s | **7x faster** than s5cmd |
| **s5cmd** | 8.17s | 19.6 MB/s | Baseline (1.0x) |
| **rclone** | 45-60s | 3-4 MB/s | 38-51x slower |
| **aws-cli** | 63.47s | 2.5 MB/s | 54x slower |

*(10,000 files, 200MB total, macOS ARM64)*

See [PERFORMANCE_BENCHMARKS.md](PERFORMANCE_BENCHMARKS.md) for detailed methodology and results.

---

## Support and Resources

- **Documentation**: [cargoship.app](https://cargoship.app)
- **GitHub**: [github.com/scttfrdmn/cargoship](https://github.com/scttfrdmn/cargoship)
- **Issues**: [github.com/scttfrdmn/cargoship/issues](https://github.com/scttfrdmn/cargoship/issues)
- **Discussions**: [github.com/scttfrdmn/cargoship/discussions](https://github.com/scttfrdmn/cargoship/discussions)

### Additional Guides

- [S3 Direct Upload Guide](S3_DIRECT_UPLOAD.md) - Complete feature documentation
- [Optimization Guide](OPTIMIZATION_GUIDE.md) - Performance tuning deep dive
- [Migration from rclone](MIGRATION_FROM_RCLONE.md) - Switch from rclone
- [Troubleshooting](TROUBLESHOOTING.md) - Common issues and solutions
- [CLI Reference](CLI_REFERENCE.md) - Complete command reference

---

**Ship your data with confidence. Ship it with CargoShip.** 🚢
