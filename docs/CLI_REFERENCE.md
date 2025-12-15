# CargoShip CLI Reference

**Version**: v0.6.2
**Last Updated**: December 2025

Complete command-line reference for CargoShip.

---

## Table of Contents

1. [Global Options](#global-options)
2. [Upload Commands](#upload-commands)
3. [Download Commands](#download-commands)
4. [Management Commands](#management-commands)
5. [Cost Commands](#cost-commands)
6. [Utility Commands](#utility-commands)
7. [Configuration](#configuration)
8. [Environment Variables](#environment-variables)

---

## Global Options

Available for all commands:

```
--context string        Override execution context (local, agent, controller, repl)
--memory-limit string   Set memory limit (e.g., "2GB", "512MB")
--pprof                 Enable profiling endpoint at localhost:6060
--pprof-addr string     Profiling endpoint address (default "localhost:6060")
--profile               Enable performance profiling (generates profile files)
-t, --trace             Enable trace-level logging
-v, --verbose           Enable verbose output
--version               Show version information
-h, --help              Show help information
```

**Examples**:

```bash
# Enable verbose logging
cargoship create upload ./data --bucket my-bucket --verbose

# Enable profiling
cargoship create upload ./data --bucket my-bucket --pprof

# Show version
cargoship --version
```

---

## Upload Commands

### `cargoship create upload`

Upload directories to S3 using the streaming pipeline.

**Synopsis**:
```bash
cargoship create upload SOURCE_DIR... [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--region` | string | us-west-2 | AWS region |
| `--prefix` | string | | S3 key prefix |
| `--storage-class` | string | STANDARD | S3 storage class |
| `--chunk-size-mb` | int | 200 | Target chunk size in MB (0 = adaptive) |
| `--workers` | int | 4 | Workers per pipeline stage |
| `--shards` | int | 8 | Number of S3 prefix shards |
| `--quiet` | bool | false | Disable progress display |
| `--progress-format` | string | tui | Progress format: tui, json, text |
| `--resume` | bool | false | Resume incomplete upload |
| `--upload-id` | string | | Upload ID to resume (auto-detect if empty) |
| `--skip-existing` | bool | false | Skip chunks that already exist (HeadObject check) |
| `--cleanup-on-failure` | bool | true | Auto-delete partial uploads on error |
| `--no-cleanup` | bool | false | Disable automatic cleanup (for debugging) |
| `--http2` | bool | true | Enable HTTP/2 |
| `--http2-max-streams` | int | 250 | Max concurrent HTTP/2 streams |
| `--max-idle-conns` | int | 100 | Max idle connections per host |
| `--idle-conn-timeout` | duration | 5m | Idle connection timeout |
| `--network-profile` | string | default | Network profile: default, aggressive, conservative |

**Storage Classes**:
- `STANDARD` - Standard storage
- `STANDARD_IA` - Infrequent Access
- `INTELLIGENT_TIERING` - Automatic tier transitions
- `ONEZONE_IA` - Single AZ Infrequent Access
- `GLACIER_IR` - Glacier Instant Retrieval
- `GLACIER` - Glacier Flexible Retrieval
- `DEEP_ARCHIVE` - Glacier Deep Archive

**Examples**:

```bash
# Basic upload
cargoship create upload ./data --bucket my-bucket

# Upload with prefix and storage class
cargoship create upload ./backups \
  --bucket my-bucket \
  --prefix backups/2025-12-15 \
  --storage-class GLACIER_IR

# High-performance upload (large network)
cargoship create upload ./dataset \
  --bucket ml-datasets \
  --workers 16 \
  --chunk-size-mb 500 \
  --http2-max-streams 500

# Memory-constrained upload
cargoship create upload ./data \
  --bucket my-bucket \
  --workers 2 \
  --chunk-size-mb 50 \
  --memory-limit 1GB

# Resume interrupted upload
cargoship create upload ./data \
  --bucket my-bucket \
  --resume

# JSON output for automation
cargoship create upload ./data \
  --bucket my-bucket \
  --progress-format json
```

### `cargoship upload`

Alias for `cargoship create upload`.

### `cargoship sync`

Incrementally sync directory to S3 (only upload new/changed files).

**Synopsis**:
```bash
cargoship sync SOURCE_DIR [flags]
```

**Flags**:

Same as `create upload`, plus:
- `--delete` - Delete remote files not in source
- `--dry-run` - Show what would be synced without uploading

**Examples**:

```bash
# Incremental sync
cargoship sync ./dataset --bucket my-bucket --prefix dataset-v1

# Sync with deletions
cargoship sync ./data --bucket my-bucket --delete

# Dry run
cargoship sync ./data --bucket my-bucket --dry-run
```

---

## Download Commands

### `cargoship download`

Download and extract files from a CargoShip upload.

**Synopsis**:
```bash
cargoship download [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--upload-id` | string | | Specific upload ID to download |
| `--output` | string | *required* | Output directory |
| `--file` | string | | Download specific file only |
| `--workers` | int | 4 | Parallel download workers |
| `--decompress` | bool | true | Decompress archives |

**Examples**:

```bash
# Download entire upload
cargoship download \
  --bucket my-bucket \
  --prefix backups/2025-12-15 \
  --output ./restored-data

# Download specific file
cargoship download \
  --bucket my-bucket \
  --upload-id 20251215-abc123 \
  --output ./restored \
  --file path/to/specific/file.txt

# Download without decompression (keep archives)
cargoship download \
  --bucket my-bucket \
  --prefix dataset \
  --output ./archives \
  --decompress=false
```

---

## Management Commands

### `cargoship list`

List files from a CargoShip upload using the manifest.

**Synopsis**:
```bash
cargoship list [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--upload-id` | string | | Specific upload ID |
| `--long` | bool | false | Long format (sizes, dates) |
| `--format` | string | text | Output format: text, json, csv |

**Examples**:

```bash
# List files in upload
cargoship list --bucket my-bucket --prefix backups/2025-12-15

# Long format
cargoship list --bucket my-bucket --upload-id 20251215-abc123 --long

# JSON output
cargoship list --bucket my-bucket --prefix dataset --format json
```

### `cargoship info`

Display metadata and statistics for a CargoShip upload.

**Synopsis**:
```bash
cargoship info [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--upload-id` | string | | Specific upload ID |
| `--format` | string | text | Output format: text, json |

**Examples**:

```bash
# Show upload info
cargoship info --bucket my-bucket --upload-id 20251215-abc123

# JSON output
cargoship info --bucket my-bucket --prefix dataset --format json
```

**Output**:
```
Upload ID: 20251215-abc123
Created: 2025-12-15 10:30:45 PST
Files: 10,000
Total Size: 1.2 GB
Compressed Size: 480 MB (60% reduction)
Chunks: 12
Shards: 8
Storage Class: GLACIER_IR
Region: us-west-2
```

### `cargoship verify`

Verify dataset integrity using manifest checksums.

**Synopsis**:
```bash
cargoship verify [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--upload-id` | string | | Specific upload ID |
| `--deep` | bool | false | Download and verify chunk contents |

**Examples**:

```bash
# Quick verify (manifest only)
cargoship verify --bucket my-bucket --upload-id 20251215-abc123

# Deep verify (download and check chunks)
cargoship verify --bucket my-bucket --upload-id 20251215-abc123 --deep
```

### `cargoship delete`

Delete a CargoShip upload from S3.

**Synopsis**:
```bash
cargoship delete [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--upload-id` | string | | Specific upload ID |
| `--force` | bool | false | Skip confirmation prompt |

**Examples**:

```bash
# Delete upload with confirmation
cargoship delete --bucket my-bucket --upload-id 20251215-abc123

# Force delete without confirmation
cargoship delete --bucket my-bucket --prefix old-backups --force
```

**Warning**: This permanently deletes data. Use with caution.

### `cargoship scuttle`

🚨 **NUCLEAR OPTION**: Delete ALL CargoShip data from a bucket/prefix.

**Synopsis**:
```bash
cargoship scuttle [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--prefix` | string | | S3 key prefix |
| `--confirm` | string | *required* | Type bucket name to confirm |

**Examples**:

```bash
# Delete all data (requires confirmation)
cargoship scuttle --bucket my-bucket --confirm my-bucket
```

**Warning**: This is irreversible. All uploads, manifests, and data will be permanently deleted.

---

## Cost Commands

### `cargoship estimate`

Estimate AWS costs for archiving data.

**Synopsis**:
```bash
cargoship estimate SOURCE_DIR [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--storage-class` | string | STANDARD | S3 storage class |
| `--region` | string | us-west-2 | AWS region |
| `--format` | string | text | Output format: text, json |
| `--real-time-pricing` | bool | false | Fetch current AWS pricing |

**Examples**:

```bash
# Estimate storage costs
cargoship estimate ./dataset --storage-class GLACIER_IR

# Real-time pricing
cargoship estimate ./data --real-time-pricing

# JSON output
cargoship estimate ./data --format json
```

**Output**:
```
Dataset Analysis:
  Files: 10,000
  Total Size: 1.2 GB
  Estimated Compressed: 480 MB (60% reduction)

Cost Estimate (GLACIER_IR, us-west-2):
  Upload (one-time): $0.00002 (10 PUT requests)
  Storage: $0.00192/month

  First Year Total: $0.02
  Annual (ongoing): $0.02/year
```

### `cargoship cost`

Cost management and budget tracking.

**Synopsis**:
```bash
cargoship cost <subcommand> [flags]
```

**Subcommands**:
- `projects` - List projects with costs
- `forecast` - Forecast future costs
- `report` - Generate cost report

**Examples**:

```bash
# List project costs
cargoship cost projects

# Forecast costs
cargoship cost forecast --model ensemble

# Generate monthly report
cargoship cost report --month 2025-12
```

### `cargoship budget`

Manage project budgets and volume quotas.

**Synopsis**:
```bash
cargoship budget <subcommand> [flags]
```

**Subcommands**:
- `set` - Set budget limits
- `status` - Show budget status
- `clear` - Clear budget limits

**Examples**:

```bash
# Set budget limits
cargoship budget set --max-budget 1000 --max-volume-gb 500

# Check budget status
cargoship budget status

# Clear limits
cargoship budget clear
```

See [BUDGET_USER_GUIDE.md](BUDGET_USER_GUIDE.md) for details.

### `cargoship alerts`

Manage budget alert notifications.

**Synopsis**:
```bash
cargoship alerts <subcommand> [flags]
```

**Subcommands**:
- `configure` - Configure alert channels (email, Slack)
- `test` - Test alert delivery
- `list` - List configured alerts

**Examples**:

```bash
# Configure email alerts
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --from noreply@example.com \
  --to admin@example.com

# Test alerts
cargoship alerts test

# List alerts
cargoship alerts list
```

---

## Utility Commands

### `cargoship config`

Manage CargoShip configuration.

**Synopsis**:
```bash
cargoship config <subcommand> [flags]
```

**Subcommands**:
- `show` - Display current configuration
- `set` - Set configuration value
- `get` - Get configuration value
- `reset` - Reset to defaults

**Examples**:

```bash
# Show config
cargoship config show

# Set value
cargoship config set aws.region us-east-1

# Get value
cargoship config get upload.workers

# Reset to defaults
cargoship config reset
```

### `cargoship setup`

Interactive setup wizard for CargoShip configuration.

**Synopsis**:
```bash
cargoship setup [flags]
```

**Examples**:

```bash
# Run setup wizard
cargoship setup
```

Guides you through:
1. AWS credentials configuration
2. Default settings (region, storage class)
3. Performance tuning (workers, chunk size)
4. Budget and alert setup

### `cargoship shell`

Start an interactive CargoShip shell.

**Synopsis**:
```bash
cargoship shell [flags]
```

**Examples**:

```bash
# Start interactive shell
cargoship shell

# Shell commands:
> upload ./data --bucket my-bucket
> list --bucket my-bucket
> info --upload-id 20251215-abc123
> exit
```

### `cargoship dashboard`

Launch comprehensive CargoShip TUI dashboard.

**Synopsis**:
```bash
cargoship dashboard [flags]
```

**Examples**:

```bash
# Launch dashboard
cargoship dashboard
```

Features:
- Real-time upload progress
- System metrics (CPU, memory, network)
- Recent uploads list
- Budget status
- Cost analytics

### `cargoship profile`

Performance profiling and diagnostics tools.

**Synopsis**:
```bash
cargoship profile <subcommand> [flags]
```

**Subcommands**:
- `cpu` - CPU profiling
- `mem` - Memory profiling
- `trace` - Execution trace

**Examples**:

```bash
# CPU profile during upload
cargoship profile cpu -- create upload ./data --bucket my-bucket

# Memory profile
cargoship profile mem -- create upload ./data --bucket my-bucket

# Execution trace
cargoship profile trace -- create upload ./data --bucket my-bucket
```

### `cargoship benchmark`

Benchmark compression algorithms.

**Synopsis**:
```bash
cargoship benchmark [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | | File to benchmark (generates test data if empty) |
| `--size` | string | 100MB | Test data size |
| `--algorithms` | string | all | Algorithms: zstd, gzip, brotli, lz4, or all |

**Examples**:

```bash
# Benchmark all algorithms
cargoship benchmark --size 1GB

# Benchmark specific algorithm
cargoship benchmark --algorithms zstd --size 500MB

# Benchmark real file
cargoship benchmark --file ./dataset.tar
```

### `cargoship migrate`

Convert traditional archives to CargoHold sharded format.

**Synopsis**:
```bash
cargoship migrate [flags]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bucket` | string | *required* | S3 bucket name |
| `--input-prefix` | string | *required* | Source prefix (traditional format) |
| `--output-prefix` | string | *required* | Destination prefix (CargoHold format) |
| `--dry-run` | bool | false | Preview migration without executing |

**Examples**:

```bash
# Migrate archives
cargoship migrate \
  --bucket my-bucket \
  --input-prefix legacy-archives \
  --output-prefix new-format

# Dry run
cargoship migrate \
  --bucket my-bucket \
  --input-prefix legacy-archives \
  --output-prefix new-format \
  --dry-run
```

### `cargoship lifecycle`

Manage S3 lifecycle policies for cost optimization.

**Synopsis**:
```bash
cargoship lifecycle <subcommand> [flags]
```

**Subcommands**:
- `list-templates` - List available policy templates
- `apply` - Apply lifecycle policy
- `show` - Show current policy
- `remove` - Remove lifecycle policy

**Examples**:

```bash
# List templates
cargoship lifecycle list-templates

# Apply policy
cargoship lifecycle apply \
  --bucket my-bucket \
  --template archive-optimize

# Show current policy
cargoship lifecycle show --bucket my-bucket

# Remove policy
cargoship lifecycle remove --bucket my-bucket
```

### `cargoship completion`

Generate shell completion scripts.

**Synopsis**:
```bash
cargoship completion <shell> [flags]
```

**Supported Shells**:
- `bash`
- `zsh`
- `fish`
- `powershell`

**Examples**:

```bash
# Bash
cargoship completion bash > /etc/bash_completion.d/cargoship

# Zsh
cargoship completion zsh > "${fpath[1]}/_cargoship"

# Fish
cargoship completion fish > ~/.config/fish/completions/cargoship.fish
```

---

## Configuration

### Configuration File

**Location**: `~/.cargoship.yaml`

**Example**:

```yaml
aws:
  region: us-west-2
  profile: default

upload:
  chunk_size_mb: 100
  workers: 8
  compression_level: 9
  default_storage_class: STANDARD

s3:
  multipart_threshold_mb: 16
  http2_enabled: true
  max_idle_conns: 100

budget:
  max_budget_usd: 1000
  max_volume_gb: 500
  alerts:
    email_enabled: true
    slack_enabled: false

observability:
  tracing_enabled: false
  metrics_enabled: false
  pprof_enabled: false
```

---

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `AWS_REGION` | Default AWS region | `us-west-2` |
| `AWS_PROFILE` | AWS credentials profile | `default` |
| `AWS_ACCESS_KEY_ID` | AWS access key | `AKIAIOSFODNN7EXAMPLE` |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` |
| `CARGOSHIP_CONFIG` | Config file path | `~/.cargoship.yaml` |
| `CARGOSHIP_BUCKET` | Default S3 bucket | `my-bucket` |
| `CARGOSHIP_REGION` | Default region | `us-west-2` |
| `CARGOSHIP_STORAGE_CLASS` | Default storage class | `GLACIER_IR` |
| `CARGOSHIP_WORKERS` | Default worker count | `8` |
| `CARGOSHIP_CHUNK_SIZE_MB` | Default chunk size | `200` |

**Examples**:

```bash
# Use environment variables
export AWS_REGION=us-east-1
export CARGOSHIP_BUCKET=my-bucket
cargoship create upload ./data
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Configuration error |
| 3 | Authentication error |
| 4 | Network error |
| 5 | Storage error (S3) |
| 6 | Insufficient permissions |
| 7 | Resource not found |
| 8 | Budget exceeded |

---

## Additional Resources

- [S3 Direct Upload Guide](S3_DIRECT_UPLOAD.md)
- [Optimization Guide](OPTIMIZATION_GUIDE.md)
- [Performance Benchmarks](PERFORMANCE_BENCHMARKS.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Migration Guide](MIGRATION_FROM_RCLONE.md)

---

## Support

- **Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Discussions**: https://github.com/scttfrdmn/cargoship/discussions
- **Documentation**: https://github.com/scttfrdmn/cargoship/tree/main/docs
