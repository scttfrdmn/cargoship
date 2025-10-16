# CargoShip Developer Tools Guide (v0.4.6)

This guide covers the new developer experience features introduced in CargoShip v0.4.6, including the interactive setup wizard, configuration validation, performance profiling, and debugging tools.

## Table of Contents

- [Quick Start](#quick-start)
- [Interactive Setup Wizard](#interactive-setup-wizard)
- [Configuration Management](#configuration-management)
- [Performance Profiling](#performance-profiling)
- [Debugging and Logging](#debugging-and-logging)
- [Best Practices](#best-practices)

## Quick Start

### First-Time Setup

If you're new to CargoShip, the interactive setup wizard is the fastest way to get started:

```bash
cargoship setup
```

This wizard will guide you through:
1. AWS configuration (region, profile, credentials verification)
2. S3 storage setup (bucket, storage class, encryption)
3. Upload optimization (file size profiles, compression)
4. Optional features (CloudWatch metrics, logging)
5. Configuration testing and validation

### Verify Your Setup

After setup, validate your configuration:

```bash
# Basic validation
cargoship config --validate

# Detailed validation with AWS connectivity checks
cargoship config --validate-detailed
```

## Interactive Setup Wizard

The setup wizard (`cargoship setup`) provides an intelligent, step-by-step configuration process.

### Features

- **Smart Defaults**: Automatically configures optimal settings based on your use case
- **Built-in Validation**: Tests AWS credentials and S3 bucket access in real-time
- **File Size Profiles**: Optimizes settings for your typical file sizes
- **Beautiful UI**: Clear prompts with visual feedback (✅, ⚠️, ❌)

### Usage

```bash
# Run interactive setup
cargoship setup

# Save to custom location
cargoship setup --output /path/to/config.yaml

# Non-interactive mode (uses defaults)
cargoship setup --non-interactive
```

### Step-by-Step Walkthrough

#### Step 1: AWS Configuration

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 1: AWS Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

AWS Region [us-east-1]: us-west-2
AWS Profile (leave empty for default credentials):

Verifying AWS credentials...
✅ AWS credentials verified
```

The wizard:
- Prompts for AWS region (defaults to us-east-1)
- Optionally uses an AWS profile from `~/.aws/credentials`
- Verifies credentials using STS GetCallerIdentity
- Shows your AWS account ID, user ID, and ARN in verbose mode

#### Step 2: S3 Storage Configuration

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 2: S3 Storage Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Default S3 Bucket (optional): my-cargoship-bucket

Verifying bucket access...
✅ Bucket access verified

S3 Storage Classes:
  1. INTELLIGENT_TIERING (recommended - automatic cost optimization)
  2. STANDARD (frequent access)
  3. STANDARD_IA (infrequent access)
  4. GLACIER (long-term archive)
  5. DEEP_ARCHIVE (lowest cost, rare access)
Choose storage class [1]: 1

Enable server-side encryption? [Y/n]: y
```

The wizard:
- Optionally configures a default S3 bucket
- Tests bucket access using S3 HeadBucket API
- Presents storage class options with recommendations
- Configures server-side encryption (SSE)

#### Step 3: Upload Optimization

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 3: Upload Optimization
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

What type of files will you primarily upload?
  1. Large files (>1GB) - videos, archives, datasets
  2. Medium files (100MB-1GB) - documents, images
  3. Small files (<100MB) - logs, configs, code
  4. Mixed sizes
Choose file size profile [4]: 1

✅ Configured for large files (high concurrency, large chunks)

Compression options:
  1. zstd (recommended - fast and efficient)
  2. gzip (compatible, slower)
  3. none (no compression)
Choose compression [1]: 1
```

The wizard automatically configures:

| File Size Profile | Max Concurrency | Chunk Size | Adaptive Sizing |
|------------------|-----------------|------------|-----------------|
| Large (>1GB)     | 16              | 32MB       | No              |
| Medium (100MB-1GB) | 8            | 16MB       | No              |
| Small (<100MB)   | 4               | 8MB        | No              |
| Mixed            | 8               | 16MB       | Yes             |

#### Step 4: Optional Features

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 4: Optional Features
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Enable CloudWatch metrics? [Y/n]: y
Metrics namespace [CargoShip/Production]:

Logging levels:
  1. debug (detailed logs)
  2. info (standard logs)
  3. warn (warnings only)
  4. error (errors only)
Choose log level [2]: 2
```

Configures:
- CloudWatch metrics with custom namespace
- Log level (debug, info, warn, error)

#### Step 5: Testing Configuration

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 5: Testing Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Testing AWS credentials... ✅
Testing bucket access... ✅

✅ Configuration test passed!

╔══════════════════════════════════════════════════════════════╗
║              Setup Complete! 🎉                              ║
╚══════════════════════════════════════════════════════════════╝

Configuration saved to: /Users/user/.cargoship.yaml

Next steps:
  1. Test your setup:
     cargoship upload <file> s3://your-bucket/path
  2. View your configuration:
     cargoship config --show
  3. Edit configuration:
     cargoship config --edit
```

The wizard:
- Tests AWS credentials via STS
- Tests S3 bucket access
- Saves configuration to `~/.cargoship.yaml` or custom path
- Provides clear next steps

### Debug Mode

Enable verbose logging during setup to see detailed information:

```bash
cargoship --verbose setup
```

This shows:
- AWS account details (account ID, ARN, user ID)
- S3 bucket region and access details
- Configuration parameter selections
- Validation results

## Configuration Management

CargoShip v0.4.6 includes enhanced configuration validation and management tools.

### Validation

#### Basic Validation

Validates configuration structure and values:

```bash
cargoship config --validate
```

Output example:
```
╔══════════════════════════════════════════════════════════════╗
║             Configuration Validation                         ║
╚══════════════════════════════════════════════════════════════╝

AWS Configuration:
  ✅ Region: us-west-2
  ℹ️  Profile: default

Storage Configuration:
  ✅ Default Bucket: my-bucket
  ✅ Storage Class: INTELLIGENT_TIERING
  ✅ SSE Encryption: true

Upload Configuration:
  ✅ Max Concurrency: 8
  ✅ Chunk Size: 16MB
  ✅ Compression: zstd
  ✅ Adaptive Sizing: true

Metrics Configuration:
  ✅ Enabled: true
  ✅ Namespace: CargoShip/Production

Logging Configuration:
  ✅ Level: info

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Validation Summary:
  Errors: 0
  Warnings: 0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Configuration is valid!
```

#### Detailed Validation

Includes AWS connectivity and S3 bucket access checks:

```bash
cargoship config --validate-detailed
```

Additional checks:
- AWS credentials verification (STS GetCallerIdentity)
- S3 bucket accessibility (HeadBucket)
- Network connectivity to AWS endpoints

### Validation Rules

The validator checks:

**AWS Configuration:**
- Region must be set
- Profile (if specified) must be valid
- Credentials must be accessible (detailed mode)

**Storage Configuration:**
- Bucket must exist and be accessible (detailed mode)
- Storage class must be valid S3 storage class
- Valid classes: STANDARD, REDUCED_REDUNDANCY, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, GLACIER, DEEP_ARCHIVE, GLACIER_IR

**Upload Configuration:**
- Max concurrency must be positive
- Warning if concurrency > 100 (may cause resource issues)
- Chunk size must be valid format (e.g., "16MB", "32MB")
- Chunk size must be ≥ 5MB for S3 multipart uploads
- Compression type must be valid: gzip, zstd, none, lz4, brotli

**Metrics Configuration:**
- Warning if enabled but namespace not set

**Logging Configuration:**
- Log level must be: debug, info, warn, error

### Viewing Configuration

#### YAML Format

```bash
cargoship config --show
```

#### JSON Format

```bash
cargoship config --show --format json
```

#### Specific File

```bash
cargoship config --show --file /path/to/config.yaml
```

### Editing Configuration

#### Interactive Editor

Opens configuration in your default editor:

```bash
cargoship config --edit
```

Uses `$EDITOR` or `$VISUAL` environment variable, or falls back to: nano, vim, vi, emacs

#### Edit Specific File

```bash
cargoship config --edit --file /path/to/config.yaml
```

After editing, the configuration is automatically validated.

### Generating Example Configuration

```bash
# Print example to stdout
cargoship config --generate

# Save to file
cargoship config --generate > ~/.cargoship.yaml
```

## Performance Profiling

CargoShip v0.4.6 includes comprehensive performance profiling tools.

### Overview

The `cargoship profile` command provides three subcommands:
- `collect`: Collect various performance profiles
- `list`: List available profile files
- `stats`: Show current runtime statistics

### Collecting Profiles

#### CPU Profile

Captures CPU usage for a specified duration:

```bash
# 30-second CPU profile (default)
cargoship profile collect --cpu

# Custom duration
cargoship profile collect --cpu --duration 60

# Custom output directory
cargoship profile collect --cpu --output-dir ./profiles
```

#### Memory Profile

Captures heap memory allocations:

```bash
cargoship profile collect --memory
```

#### Goroutine Profile

Captures goroutine stack traces:

```bash
cargoship profile collect --goroutine
```

#### Block Profile

Captures blocking operations (mutex, channel):

```bash
cargoship profile collect --block
```

#### Mutex Profile

Captures mutex contention:

```bash
cargoship profile collect --mutex
```

#### Execution Trace

Captures detailed execution trace for visualization:

```bash
cargoship profile collect --trace --duration 30
```

#### Allocation Profile

Captures all memory allocations:

```bash
cargoship profile collect --allocs
```

#### Collect Multiple Profiles

```bash
# Collect CPU, memory, and goroutine profiles
cargoship profile collect --cpu --memory --goroutine --duration 60

# Collect all profiles
cargoship profile collect --cpu --memory --goroutine --block --mutex --trace --allocs --duration 60
```

### Analyzing Profiles

#### CPU Profile Analysis

```bash
# Analyze with pprof
go tool pprof cpu-20251016-153045.prof
```

In pprof console:
```
(pprof) top 10              # Show top 10 CPU consumers
(pprof) list main.upload    # Show source code for function
(pprof) web                 # Generate visual graph (requires graphviz)
(pprof) pdf                 # Generate PDF report
(pprof) help                # Show all commands
```

#### Memory Profile Analysis

```bash
go tool pprof memory-20251016-153045.prof
```

In pprof console:
```
(pprof) top 10              # Top 10 memory allocations
(pprof) alloc_space         # Sort by allocated space
(pprof) alloc_objects       # Sort by allocated objects
(pprof) list main.          # Show source code
```

#### Compare Profiles

Compare two profiles to find memory leaks or regressions:

```bash
# Collect baseline
cargoship profile collect --memory
# ... run workload ...
# Collect after workload
cargoship profile collect --memory

# Compare
go tool pprof -base memory-1.prof memory-2.prof
```

#### Execution Trace Visualization

```bash
go tool trace trace-20251016-153045.out
```

Opens browser with interactive trace viewer showing:
- Goroutine execution timeline
- Network/syscall blocking
- GC events
- Goroutine analysis

### Runtime Statistics

Show current runtime statistics:

```bash
cargoship profile stats
```

Output example:
```
╔══════════════════════════════════════════════════════════════╗
║              Runtime Statistics                              ║
╚══════════════════════════════════════════════════════════════╝

Memory Statistics:
  • Allocated:     45.32 MB
  • Total Alloc:   234.56 MB
  • Sys:           123.45 MB
  • Heap Alloc:    45.32 MB
  • Heap Sys:      98.76 MB
  • Heap Idle:     53.44 MB
  • Heap In Use:   45.32 MB
  • Heap Released: 48.00 MB
  • Heap Objects:  123456

Garbage Collection:
  • Num GC:        23
  • Pause Total:   45.6ms
  • Last Pause:    1.2ms
  • GC CPU %:      0.45%

Goroutines:
  • Active:        12
  • Threads:       8

System:
  • NumCPU:        8
  • GOMAXPROCS:    8
  • Go Version:    go1.21.0
  • OS/Arch:       darwin/amd64
```

### Listing Profiles

```bash
# List profiles in default temp directory
cargoship profile list

# List profiles in custom directory
cargoship profile list --dir ./profiles
```

Output example:
```
╔══════════════════════════════════════════════════════════════╗
║                 Available Profile Files                      ║
╚══════════════════════════════════════════════════════════════╝

Directory: /tmp/cargoship-profile-123456

  • cpu-20251016-153045.prof (1.23 MB, modified: 2025-10-16 15:30:50)
  • memory-20251016-153045.prof (512.00 KB, modified: 2025-10-16 15:30:51)
  • goroutine-20251016-153045.prof (24.56 KB, modified: 2025-10-16 15:30:52)
  • trace-20251016-153045.out (5.67 MB, modified: 2025-10-16 15:31:15)
```

### Performance Profiling Workflow

1. **Establish Baseline**:
   ```bash
   cargoship profile stats
   cargoship profile collect --memory
   ```

2. **Run Your Workload**:
   ```bash
   cargoship upload large-file.dat s3://bucket/key
   ```

3. **Collect Post-Workload Profiles**:
   ```bash
   cargoship profile stats
   cargoship profile collect --memory --cpu
   ```

4. **Analyze Differences**:
   ```bash
   go tool pprof -base memory-before.prof memory-after.prof
   ```

5. **Investigate Issues**:
   ```bash
   # If high CPU:
   go tool pprof cpu-*.prof

   # If memory leaks:
   go tool pprof -base memory-1.prof memory-2.prof

   # If goroutine leaks:
   cargoship profile collect --goroutine
   go tool pprof goroutine-*.prof
   ```

## Debugging and Logging

### Verbose Logging

Enable detailed logging with the `--verbose` flag:

```bash
cargoship --verbose <command>
```

Example output:
```
cargoship 🚢  INFO  Starting CargoShip setup wizard
cargoship 🚢  DEBUG Loaded default configuration region=us-east-1 storage-class=INTELLIGENT_TIERING
cargoship 🚢  DEBUG Verifying AWS credentials via STS GetCallerIdentity region=us-west-2
cargoship 🚢  INFO  AWS identity verified account=123456789012 arn=arn:aws:iam::123456789012:user/myuser
```

### Trace Logging

Enable trace-level logging for even more detail:

```bash
cargoship --verbose --trace <command>
```

This shows:
- Function call traces
- Detailed AWS API calls
- Internal state transitions

### Structured Logging

CargoShip uses structured logging (slog) with key-value pairs for easy parsing:

```json
{
  "time": "2025-10-16T15:30:45Z",
  "level": "INFO",
  "msg": "AWS identity verified",
  "account": "123456789012",
  "user-id": "AIDAI23XXXXX",
  "arn": "arn:aws:iam::123456789012:user/myuser"
}
```

### Log Filtering

Use `jq` to filter structured logs:

```bash
# Show only errors
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.level=="ERROR")'

# Show AWS-related logs
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.msg | contains("AWS"))'

# Show upload progress
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.msg | contains("upload"))'
```

### Debug Flags

#### Memory Limit

Prevent out-of-memory errors:

```bash
cargoship --memory-limit 2GB upload large-file s3://bucket/key
```

#### CPU Profiling

Legacy profiling flag (creates CPU profile automatically):

```bash
cargoship --profile upload large-file s3://bucket/key
```

Profile saved to temp directory, location printed at end of execution.

## Best Practices

### Configuration Management

1. **Use Version Control**:
   ```bash
   git add .cargoship.yaml
   git commit -m "Add CargoShip configuration"
   ```

2. **Environment-Specific Configs**:
   ```bash
   # Development
   cargoship --file ~/.cargoship-dev.yaml <command>

   # Production
   cargoship --file ~/.cargoship-prod.yaml <command>
   ```

3. **Validate Before Deploying**:
   ```bash
   cargoship config --validate-detailed --file ~/.cargoship-prod.yaml
   ```

### Performance Optimization

1. **Start with Setup Wizard**:
   ```bash
   cargoship setup
   ```

2. **Profile Before Optimizing**:
   ```bash
   cargoship profile collect --cpu --memory
   ```

3. **Benchmark Changes**:
   ```bash
   # Before
   time cargoship upload testfile s3://bucket/key

   # Adjust config
   vi ~/.cargoship.yaml

   # After
   time cargoship upload testfile s3://bucket/key
   ```

4. **Monitor Resource Usage**:
   ```bash
   cargoship profile stats
   ```

### Debugging Workflow

1. **Enable Verbose Logging**:
   ```bash
   cargoship --verbose --trace <command> 2>&1 | tee debug.log
   ```

2. **Validate Configuration**:
   ```bash
   cargoship config --validate-detailed
   ```

3. **Check AWS Connectivity**:
   ```bash
   aws sts get-caller-identity
   aws s3 ls s3://your-bucket
   ```

4. **Collect Diagnostic Profiles**:
   ```bash
   cargoship profile collect --cpu --memory --goroutine
   ```

5. **Analyze Profiles**:
   ```bash
   go tool pprof cpu-*.prof
   go tool pprof memory-*.prof
   ```

### Production Deployment

1. **Run Setup Wizard in Non-Interactive Mode**:
   ```bash
   # Set via environment or config file
   cargoship setup --non-interactive
   ```

2. **Enable CloudWatch Metrics**:
   ```yaml
   metrics:
     enabled: true
     namespace: "CargoShip/Production"
   ```

3. **Set Appropriate Log Level**:
   ```yaml
   logging:
     level: info  # Not debug in production
   ```

4. **Regular Health Checks**:
   ```bash
   cargoship config --validate-detailed
   cargoship profile stats
   ```

5. **Automated Profiling**:
   ```bash
   # Daily profile collection
   0 2 * * * /usr/local/bin/cargoship profile collect --memory --goroutine
   ```

---

**Last Updated**: 2025-10-16
**Version**: v0.4.6
