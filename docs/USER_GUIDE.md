# CargoShip User Guide

## Table of Contents

1. [Introduction](#introduction)
2. [Installation](#installation)
3. [Quick Start](#quick-start)
4. [Configuration](#configuration)
5. [Basic Usage](#basic-usage)
6. [Advanced Features](#advanced-features)
7. [Multi-Region Setup](#multi-region-setup)
8. [Performance Optimization](#performance-optimization)
9. [Security Best Practices](#security-best-practices)
10. [Troubleshooting](#troubleshooting)
11. [CLI Reference](#cli-reference)

## Introduction

CargoShip is a high-performance AWS data archiving tool designed for enterprise-scale data management. It provides:

- **Intelligent Compression**: Automatic algorithm selection for optimal performance
- **Multi-Region Support**: Distribute and replicate data across AWS regions
- **GPG Encryption**: End-to-end encryption for sensitive data
- **Smart Failover**: Automatic failover with minimal downtime
- **Performance Optimization**: Adaptive network and compression tuning

## Installation

### Prerequisites

- **Go 1.21+** (for building from source)
- **AWS CLI** configured with appropriate credentials
- **GPG** (for encryption features)
- **Git** (for cloning the repository)

### From Binary (Recommended)

```bash
# Download the latest release
curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64 -o cargoship
chmod +x cargoship
sudo mv cargoship /usr/local/bin/
```

### From Source

```bash
git clone https://github.com/scttfrdmn/cargoship.git
cd cargoship
go build -o cargoship ./cmd/cargoship
sudo mv cargoship /usr/local/bin/
```

### Verify Installation

```bash
cargoship version
cargoship --help
```

## Quick Start

### 1. Configure AWS Credentials

```bash
# Using AWS CLI
aws configure

# Or set environment variables
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_DEFAULT_REGION=us-east-1
```

### 2. Initialize Configuration

```bash
# Create default configuration
cargoship config init

# Or use the interactive wizard
cargoship wizard
```

### 3. Create Your First Archive

```bash
# Archive a directory to S3
cargoship create /path/to/data --bucket my-archive-bucket

# Archive with compression
cargoship create /path/to/data --bucket my-archive-bucket --compression gzip

# Archive with encryption
cargoship create /path/to/data --bucket my-archive-bucket --encrypt
```

## Configuration

### Configuration File

CargoShip uses a YAML configuration file located at `~/.cargoship.yaml`:

```yaml
# Basic AWS Configuration
aws:
  region: us-east-1
  bucket: my-default-bucket
  storage_class: STANDARD_IA

# Compression Settings
compression:
  algorithm: zstd
  level: 3
  enable_auto_selection: true

# Encryption Settings
encryption:
  enabled: true
  gpg_key_id: your-gpg-key-id
  
# Multi-Region Configuration
multi_region:
  enabled: false
  primary_region: us-east-1
  regions:
    - name: us-east-1
      priority: 1
      weight: 50
    - name: us-west-2
      priority: 2
      weight: 30

# Performance Settings
performance:
  max_concurrent_uploads: 8
  chunk_size_mb: 64
  enable_adaptive_tuning: true

# Logging
logging:
  level: info
  file: ~/.cargoship.log
```

### Environment Variables

Key environment variables:

```bash
# AWS Configuration
export AWS_PROFILE=production
export AWS_DEFAULT_REGION=us-east-1

# CargoShip Configuration
export CARGOSHIP_CONFIG=/path/to/config.yaml
export CARGOSHIP_LOG_LEVEL=debug
export CARGOSHIP_BUCKET=my-backup-bucket

# GPG Configuration
export GPG_KEY_ID=your-key-id
export GNUPGHOME=/path/to/gpg/home
```

## Basic Usage

### Creating Archives

#### Simple Directory Archive

```bash
cargoship create /home/user/documents \
  --bucket my-archive-bucket \
  --prefix backups/documents
```

#### With Compression Options

```bash
# Automatic compression selection
cargoship create /data --bucket my-bucket --compression auto

# Specific compression algorithm
cargoship create /data --bucket my-bucket --compression zstd --level 5

# Compare compression algorithms
cargoship benchmark --size 100MB --data-type mixed
```

#### With Encryption

```bash
# Encrypt with default GPG key
cargoship create /sensitive-data --bucket my-bucket --encrypt

# Encrypt with specific key
cargoship create /sensitive-data --bucket my-bucket --encrypt --gpg-key user@example.com

# Create and use new GPG key
cargoship create-keys --name "Archive Key" --email archive@company.com
cargoship create /data --bucket my-bucket --encrypt --gpg-key archive@company.com
```

### Listing and Finding Archives

```bash
# List all archives
cargoship find --bucket my-bucket

# Search by prefix
cargoship find --bucket my-bucket --prefix backups/

# Search by date range
cargoship find --bucket my-bucket --after 2024-01-01 --before 2024-12-31

# Search by size
cargoship find --bucket my-bucket --min-size 1GB --max-size 10GB
```

### Archive Analysis

```bash
# Analyze storage costs
cargoship analyze cost --bucket my-bucket

# Analyze compression efficiency
cargoship analyze compression --bucket my-bucket

# Storage lifecycle analysis
cargoship analyze lifecycle --bucket my-bucket
```

## Advanced Features

### Multi-File Archives (Suitcases)

```bash
# Create suitcase with size limit
cargoship create-suitcase /large-dataset \
  --bucket my-bucket \
  --max-size 5GB \
  --format targz

# Create with file count limit
cargoship create-suitcase /many-files \
  --bucket my-bucket \
  --max-files 10000
```

### Metadata and Inventories

```bash
# Include detailed metadata
cargoship create /data \
  --bucket my-bucket \
  --include-metadata \
  --metadata-format json

# Generate inventory
cargoship create /data \
  --bucket my-bucket \
  --generate-inventory \
  --inventory-format csv
```

### Custom Filters and Patterns

```bash
# Exclude patterns
cargoship create /project \
  --bucket my-bucket \
  --exclude "*.tmp" \
  --exclude "node_modules" \
  --exclude ".git"

# Include only specific patterns
cargoship create /data \
  --bucket my-bucket \
  --include "*.pdf" \
  --include "*.doc*"
```

## Multi-Region Setup

### Basic Multi-Region Configuration

```yaml
multi_region:
  enabled: true
  primary_region: us-east-1
  regions:
    - name: us-east-1
      priority: 1
      weight: 50
      capacity:
        max_concurrent_uploads: 10
        max_bandwidth_mbps: 1000
      health_check:
        enabled: true
        interval: 30s
        timeout: 5s
        failure_threshold: 3
    - name: us-west-2
      priority: 2
      weight: 30
      capacity:
        max_concurrent_uploads: 8
        max_bandwidth_mbps: 800
      health_check:
        enabled: true
        interval: 30s
        timeout: 5s
        failure_threshold: 3
    - name: eu-west-1
      priority: 3
      weight: 20
      capacity:
        max_concurrent_uploads: 6
        max_bandwidth_mbps: 600

  # Load balancing strategy
  load_balancing:
    strategy: weighted_round_robin
    sticky_sessions: false

  # Failover configuration
  failover:
    auto_failover: true
    strategy: graceful
    detection_interval: 15s
    failover_timeout: 30s
    retry_attempts: 2

  # Monitoring
  monitoring:
    enabled: true
    metrics_interval: 60s
```

### Multi-Region Commands

```bash
# Upload with multi-region support
cargoship create /data \
  --bucket my-bucket \
  --multi-region \
  --redundancy 2

# Check region status
cargoship status regions

# Manual failover
cargoship failover --from us-east-1 --to us-west-2

# Performance test across regions
cargoship performance \
  --test-bucket my-test-bucket \
  --regions us-east-1,us-west-2,eu-west-1 \
  --file-sizes 1MB,100MB,1GB
```

## Performance Optimization

### Compression Benchmarking

```bash
# Test compression algorithms
cargoship benchmark --size 1GB --data-type mixed --format table

# Compare performance with real data
cargoship benchmark --file /path/to/sample-data.tar

# JSON output for analysis
cargoship benchmark --size 500MB --format json > compression-results.json
```

### Network Performance Testing

```bash
# Comprehensive performance test
cargoship performance \
  --test-bucket my-benchmark-bucket \
  --file-sizes 1MB,10MB,100MB,1GB \
  --iterations 5 \
  --regions us-east-1,us-west-2 \
  --multi-region \
  --failover-tests

# Focus on specific regions
cargoship performance \
  --test-bucket my-bucket \
  --regions us-east-1 \
  --file-sizes 100MB,1GB \
  --iterations 10
```

### Adaptive Tuning

```yaml
# Enable adaptive performance tuning
performance:
  enable_adaptive_tuning: true
  network_monitoring:
    enabled: true
    interval: 5s
    adaptation_threshold: 0.1
  
  staging:
    enabled: true
    max_memory_mb: 512
    chunk_ahead_count: 3
    
  bandwidth_management:
    max_bandwidth_mbps: 1000
    adaptive_throttling: true
    burst_allowance: 1.5
```

## Security Best Practices

### GPG Key Management

```bash
# Generate dedicated archive key
cargoship create-keys \
  --name "Production Archive Key" \
  --email archives@company.com \
  --key-size 4096 \
  --expires 2y

# List available keys
cargoship create-keys --list

# Export public key for sharing
gpg --export --armor archives@company.com > archive-public-key.asc

# Backup private key securely
gpg --export-secret-keys --armor archives@company.com > archive-private-key.asc
```

### AWS IAM Permissions

Minimum required permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:GetObjectVersion"
            ],
            "Resource": "arn:aws:s3:::your-archive-bucket/*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketLocation",
                "s3:GetBucketVersioning"
            ],
            "Resource": "arn:aws:s3:::your-archive-bucket"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetBucketLocation"
            ],
            "Resource": "*"
        }
    ]
}
```

For multi-region setups:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:*"
            ],
            "Resource": [
                "arn:aws:s3:::your-archive-bucket-*",
                "arn:aws:s3:::your-archive-bucket-*/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:ListAllMyBuckets",
                "s3:GetBucketLocation"
            ],
            "Resource": "*"
        }
    ]
}
```

### Encryption Best Practices

1. **Always encrypt sensitive data**:
   ```bash
   cargoship create /sensitive-data --bucket my-bucket --encrypt
   ```

2. **Use separate keys for different data classifications**:
   ```bash
   # Financial data
   cargoship create /finance --bucket my-bucket --encrypt --gpg-key finance@company.com
   
   # HR data
   cargoship create /hr --bucket my-bucket --encrypt --gpg-key hr@company.com
   ```

3. **Regularly rotate encryption keys**:
   ```bash
   # Create new key
   cargoship create-keys --name "Archive Key 2025" --email archives-2025@company.com
   
   # Update configuration to use new key
   # Keep old key available for decryption
   ```

4. **Secure key storage**:
   - Store private keys in hardware security modules (HSMs)
   - Use key management services (AWS KMS, HashiCorp Vault)
   - Implement proper backup and recovery procedures

## Troubleshooting

### Common Issues

#### Connection Problems

```bash
# Test AWS connectivity
aws s3 ls s3://your-bucket

# Check CargoShip configuration
cargoship config validate

# Test with verbose logging
cargoship create /data --bucket my-bucket --log-level debug
```

#### Performance Issues

```bash
# Run performance diagnostics
cargoship performance --test-bucket my-bucket --file-sizes 10MB

# Check network connectivity
cargoship analyze network --region us-east-1

# Monitor resource usage
cargoship analyze system
```

#### Multi-Region Issues

```bash
# Check region health
cargoship status regions

# Test failover manually
cargoship failover --test --from us-east-1 --to us-west-2

# View detailed logs
cargoship --log-level debug status regions
```

### Log Analysis

CargoShip logs include structured information for troubleshooting:

```bash
# View recent logs
tail -f ~/.cargoship.log

# Search for errors
grep ERROR ~/.cargoship.log

# Filter by operation
grep "upload" ~/.cargoship.log | grep ERROR
```

### Getting Help

```bash
# Built-in help
cargoship help
cargoship help create
cargoship help multi-region

# Version and build information
cargoship version --verbose

# Configuration validation
cargoship config validate --verbose
```

## CLI Reference

### Global Flags

- `--config`: Path to configuration file
- `--log-level`: Logging level (debug, info, warn, error)
- `--log-file`: Path to log file
- `--profile`: AWS profile to use
- `--region`: AWS region override

### Commands

#### create
Create archives from files or directories.

```bash
cargoship create [source] [flags]
```

**Key Flags:**
- `--bucket`: S3 bucket name (required)
- `--prefix`: Object prefix/path
- `--compression`: Compression algorithm (auto, gzip, zstd, lz4)
- `--level`: Compression level
- `--encrypt`: Enable GPG encryption
- `--gpg-key`: GPG key ID or email
- `--exclude`: Exclude patterns
- `--include`: Include patterns
- `--multi-region`: Enable multi-region upload
- `--redundancy`: Number of redundant copies

#### find
Search and list archives.

```bash
cargoship find [flags]
```

**Key Flags:**
- `--bucket`: S3 bucket to search
- `--prefix`: Object prefix filter
- `--after`: Find objects after date
- `--before`: Find objects before date
- `--min-size`: Minimum object size
- `--max-size`: Maximum object size

#### analyze
Analyze storage costs, compression, and performance.

```bash
cargoship analyze [cost|compression|lifecycle|network|system] [flags]
```

#### benchmark
Test compression algorithm performance.

```bash
cargoship benchmark [flags]
```

**Key Flags:**
- `--size`: Test data size
- `--data-type`: Data type simulation
- `--file`: Use real file for testing
- `--format`: Output format (table, json)

#### performance
Run comprehensive AWS performance tests.

```bash
cargoship performance [flags]
```

**Key Flags:**
- `--test-bucket`: S3 bucket for testing (required)
- `--file-sizes`: File sizes to test
- `--iterations`: Number of test iterations
- `--regions`: AWS regions to test
- `--multi-region`: Include multi-region tests
- `--failover-tests`: Include failover tests

#### config
Manage configuration.

```bash
cargoship config [init|validate|show] [flags]
```

#### wizard
Interactive setup wizard.

```bash
cargoship wizard
```

#### create-keys
GPG key management.

```bash
cargoship create-keys [flags]
```

#### status
Show system and region status.

```bash
cargoship status [regions|system] [flags]
```

### Exit Codes

- `0`: Success
- `1`: General error
- `2`: Configuration error
- `3`: Authentication error
- `4`: Network error
- `5`: Storage error
- `6`: Encryption error

### Environment Variables Reference

| Variable | Description | Default |
|----------|-------------|---------|
| `CARGOSHIP_CONFIG` | Configuration file path | `~/.cargoship.yaml` |
| `CARGOSHIP_LOG_LEVEL` | Log level | `info` |
| `CARGOSHIP_LOG_FILE` | Log file path | `~/.cargoship.log` |
| `CARGOSHIP_BUCKET` | Default S3 bucket | - |
| `AWS_PROFILE` | AWS profile | `default` |
| `AWS_DEFAULT_REGION` | AWS region | `us-east-1` |
| `GPG_KEY_ID` | Default GPG key | - |
| `GNUPGHOME` | GPG home directory | `~/.gnupg` |

---

For more information, visit the [CargoShip GitHub repository](https://github.com/scttfrdmn/cargoship) or check the latest documentation at the project wiki.