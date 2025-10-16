# CargoShip Troubleshooting Guide

This guide provides comprehensive troubleshooting steps for common issues with CargoShip.

## Table of Contents

- [Getting Started](#getting-started)
- [Configuration Issues](#configuration-issues)
- [AWS Credentials and Permissions](#aws-credentials-and-permissions)
- [S3 Upload Issues](#s3-upload-issues)
- [Performance Problems](#performance-problems)
- [Debugging Tools](#debugging-tools)
- [Common Error Messages](#common-error-messages)
- [Getting Help](#getting-help)

## Getting Started

### Enable Debug Logging

For detailed troubleshooting information, enable verbose logging:

```bash
cargoship --verbose <command>
```

For even more detailed trace logging:

```bash
cargoship --verbose --trace <command>
```

### Check Your Configuration

Validate your configuration file:

```bash
# Basic validation
cargoship config --validate

# Detailed validation with AWS connectivity checks
cargoship config --validate-detailed

# View current configuration
cargoship config --show
```

## Configuration Issues

### Configuration File Not Found

**Symptom**: CargoShip can't find your configuration file

**Solution**:
1. Check configuration file locations (searched in order):
   - `~/.cargoship.yaml`
   - `~/.config/cargoship/.cargoship.yaml`
   - `./.cargoship.yaml`

2. Generate a new configuration file:
   ```bash
   cargoship setup
   ```

   Or manually generate an example:
   ```bash
   cargoship config --generate > ~/.cargoship.yaml
   ```

3. Specify configuration file explicitly:
   ```bash
   cargoship --file /path/to/config.yaml <command>
   ```

### Invalid Configuration Values

**Symptom**: Validation errors when running commands

**Solution**:
1. Run configuration validation:
   ```bash
   cargoship config --validate-detailed
   ```

2. Common validation issues:
   - **Invalid storage class**: Must be one of: `STANDARD`, `REDUCED_REDUNDANCY`, `STANDARD_IA`, `ONEZONE_IA`, `INTELLIGENT_TIERING`, `GLACIER`, `DEEP_ARCHIVE`, `GLACIER_IR`
   - **Invalid chunk size**: Must be at least 5MB for S3 multipart uploads
   - **Invalid concurrency**: Must be positive, recommended range: 4-16
   - **Invalid log level**: Must be one of: `debug`, `info`, `warn`, `error`
   - **Invalid compression**: Must be one of: `gzip`, `zstd`, `none`, `lz4`, `brotli`

3. Edit configuration:
   ```bash
   cargoship config --edit
   ```

### Configuration Precedence

Configuration is loaded with the following precedence (highest to lowest):

1. Command line flags
2. Environment variables (`CARGOSHIP_*`)
3. Configuration file
4. Built-in defaults

**Example**:
```bash
# Override region via environment variable
export CARGOSHIP_AWS_REGION=us-west-2
cargoship <command>

# Override via command line flag
cargoship --region us-west-2 <command>
```

## AWS Credentials and Permissions

### AWS Credentials Not Found

**Symptom**: Error messages about missing AWS credentials

**Solution**:
1. Verify AWS credentials are configured:
   ```bash
   aws sts get-caller-identity
   ```

2. Configure AWS credentials using one of these methods:

   **Method 1: AWS CLI**
   ```bash
   aws configure
   ```

   **Method 2: Environment Variables**
   ```bash
   export AWS_ACCESS_KEY_ID=your-key-id
   export AWS_SECRET_ACCESS_KEY=your-secret-key
   export AWS_REGION=us-east-1
   ```

   **Method 3: AWS Profile**
   ```bash
   # In ~/.aws/credentials
   [cargoship]
   aws_access_key_id = your-key-id
   aws_secret_access_key = your-secret-key

   # In ~/.cargoship.yaml
   aws:
     profile: cargoship
     region: us-east-1
   ```

3. Use the interactive setup wizard:
   ```bash
   cargoship setup
   ```

### Permission Denied Errors

**Symptom**: AWS API calls fail with permission errors

**Required IAM Permissions**:

For basic S3 uploads:
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
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name/*",
        "arn:aws:s3:::your-bucket-name"
      ]
    }
  ]
}
```

For multipart uploads (recommended):
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:AbortMultipartUpload",
        "s3:ListMultipartUploadParts",
        "s3:ListBucketMultipartUploads"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name/*",
        "arn:aws:s3:::your-bucket-name"
      ]
    }
  ]
}
```

For CloudWatch metrics:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cloudwatch:PutMetricData"
      ],
      "Resource": "*"
    }
  ]
}
```

### Bucket Access Denied

**Symptom**: Cannot access S3 bucket

**Solution**:
1. Verify bucket exists and you have access:
   ```bash
   aws s3 ls s3://your-bucket-name
   ```

2. Check bucket policy allows your IAM user/role

3. Verify bucket is in the same region:
   ```bash
   aws s3api get-bucket-location --bucket your-bucket-name
   ```

4. Test bucket access through CargoShip:
   ```bash
   cargoship config --validate-detailed
   ```

## S3 Upload Issues

### Upload Failures

**Symptom**: Uploads fail with various errors

**Solutions**:

1. **Check network connectivity**:
   ```bash
   # Test S3 endpoint connectivity
   curl -I https://s3.amazonaws.com

   # Or for specific region
   curl -I https://s3.us-west-2.amazonaws.com
   ```

2. **Verify file permissions**:
   ```bash
   ls -lh /path/to/file
   # Ensure file is readable
   ```

3. **Check disk space**:
   ```bash
   df -h
   # Ensure enough space for temporary files
   ```

4. **Enable verbose logging**:
   ```bash
   cargoship --verbose upload /path/to/file s3://bucket/key
   ```

### Slow Upload Performance

**Symptom**: Uploads are slower than expected

**Diagnostic Steps**:

1. **Check network bandwidth**:
   ```bash
   # Install speedtest-cli if needed
   speedtest-cli
   ```

2. **Monitor upload with verbose logging**:
   ```bash
   cargoship --verbose upload /path/to/large-file s3://bucket/key
   ```

3. **Collect performance profile**:
   ```bash
   cargoship profile collect --cpu --memory --duration 60
   ```

4. **Check runtime statistics**:
   ```bash
   cargoship profile stats
   ```

**Optimization Solutions**:

1. **Adjust concurrency** (for large files):
   ```yaml
   upload:
     max_concurrency: 16  # Increase for faster uploads
     chunk_size: "32MB"    # Larger chunks for big files
   ```

2. **Enable adaptive sizing** (for mixed file sizes):
   ```yaml
   upload:
     enable_adaptive_sizing: true
   ```

3. **Optimize chunk size** based on your use case:
   - Small files (<100MB): 8MB chunks, 4 concurrency
   - Medium files (100MB-1GB): 16MB chunks, 8 concurrency
   - Large files (>1GB): 32MB chunks, 16 concurrency

4. **Run the setup wizard** for automatic optimization:
   ```bash
   cargoship setup
   ```

### Multipart Upload Issues

**Symptom**: Multipart uploads fail or leave orphaned parts

**Solution**:

1. **List incomplete multipart uploads**:
   ```bash
   aws s3api list-multipart-uploads --bucket your-bucket-name
   ```

2. **Abort stuck multipart uploads**:
   ```bash
   aws s3api abort-multipart-upload \
     --bucket your-bucket-name \
     --key your-object-key \
     --upload-id upload-id
   ```

3. **Set lifecycle policy to clean up incomplete uploads**:
   ```bash
   aws s3api put-bucket-lifecycle-configuration \
     --bucket your-bucket-name \
     --lifecycle-configuration file://lifecycle.json
   ```

   `lifecycle.json`:
   ```json
   {
     "Rules": [
       {
         "Id": "DeleteIncompleteMultipartUpload",
         "Status": "Enabled",
         "AbortIncompleteMultipartUpload": {
           "DaysAfterInitiation": 7
         }
       }
     ]
   }
   ```

## Performance Problems

### High Memory Usage

**Symptom**: CargoShip uses excessive memory

**Diagnostic**:
```bash
# Check current memory usage
cargoship profile stats

# Collect memory profile
cargoship profile collect --memory
```

**Solutions**:

1. **Reduce concurrency**:
   ```yaml
   upload:
     max_concurrency: 4  # Lower value
   ```

2. **Reduce chunk size**:
   ```yaml
   upload:
     chunk_size: "8MB"  # Smaller chunks use less memory
   ```

3. **Set memory limit** (will slow down but prevent OOM):
   ```bash
   cargoship --memory-limit 1GB upload /path/to/file s3://bucket/key
   ```

4. **Enable garbage collection tuning**:
   ```bash
   export GOGC=50  # More aggressive GC
   cargoship upload /path/to/file s3://bucket/key
   ```

### High CPU Usage

**Symptom**: CargoShip uses excessive CPU

**Diagnostic**:
```bash
# Collect CPU profile
cargoship profile collect --cpu --duration 30
```

**Solutions**:

1. **Disable compression** (if not needed):
   ```yaml
   upload:
     compression_type: none
   ```

2. **Use faster compression**:
   ```yaml
   upload:
     compression_type: lz4  # Faster than gzip/zstd
   ```

3. **Reduce concurrency**:
   ```yaml
   upload:
     max_concurrency: 4
   ```

### Goroutine Leaks

**Symptom**: Growing number of goroutines over time

**Diagnostic**:
```bash
# Check goroutine count
cargoship profile stats

# Collect goroutine profile
cargoship profile collect --goroutine
```

**Analysis**:
```bash
# Analyze goroutine profile
go tool pprof goroutine-*.prof
```

In pprof console:
```
top 10        # Show top 10 goroutine sources
list main.    # Show detailed source code
```

## Debugging Tools

### Built-in Profiling

CargoShip provides comprehensive profiling capabilities:

```bash
# Collect CPU profile
cargoship profile collect --cpu --duration 30

# Collect memory profile
cargoship profile collect --memory

# Collect all profiles
cargoship profile collect --cpu --memory --goroutine --block --mutex --trace --duration 60

# List available profiles
cargoship profile list

# Show runtime statistics
cargoship profile stats
```

### Analyzing Profiles

**CPU Profile**:
```bash
go tool pprof cpu-*.prof

# In pprof console:
top 10        # Top 10 CPU consumers
list main.    # Show source code
web           # Generate visual graph (requires graphviz)
```

**Memory Profile**:
```bash
go tool pprof memory-*.prof

# In pprof console:
top 10              # Top 10 memory allocations
list main.          # Show source code
alloc_space         # Sort by allocated space
alloc_objects       # Sort by allocated objects
```

**Execution Trace**:
```bash
go tool trace trace-*.out
# Opens web browser with detailed execution trace
```

### Log Analysis

**Enable structured JSON logging**:
```yaml
logging:
  level: debug
  format: json
```

**Filter logs with jq**:
```bash
# Show only errors
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.level=="ERROR")'

# Show AWS API calls
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.msg | contains("AWS"))'

# Show upload progress
cargoship --verbose upload file s3://bucket/key 2>&1 | jq 'select(.msg | contains("upload"))'
```

### Network Debugging

**Enable AWS SDK logging**:
```bash
export AWS_SDK_LOAD_CONFIG=1
export AWS_SDK_LOG_LEVEL=debug
cargoship upload file s3://bucket/key
```

**Use tcpdump to capture S3 traffic**:
```bash
sudo tcpdump -i any -w s3-traffic.pcap 'host s3.amazonaws.com'
```

**Use Charles Proxy or mitmproxy**:
```bash
export HTTPS_PROXY=http://localhost:8888
cargoship upload file s3://bucket/key
```

## Common Error Messages

### "failed to load AWS config"

**Cause**: AWS credentials not configured or invalid

**Solution**:
1. Run `aws configure` to set up credentials
2. Verify credentials: `aws sts get-caller-identity`
3. Check `~/.aws/credentials` and `~/.aws/config`
4. Use `cargoship setup` wizard

### "failed to access bucket"

**Cause**: Bucket doesn't exist, wrong region, or permission denied

**Solution**:
1. Verify bucket exists: `aws s3 ls s3://bucket-name`
2. Check bucket region: `aws s3api get-bucket-location --bucket bucket-name`
3. Update region in config to match bucket
4. Verify IAM permissions
5. Run `cargoship config --validate-detailed`

### "chunk size below S3 minimum"

**Cause**: Chunk size configured below 5MB (S3 multipart minimum)

**Solution**:
```yaml
upload:
  chunk_size: "8MB"  # Or larger
```

### "invalid storage class"

**Cause**: Storage class not recognized by S3

**Solution**: Use one of the valid storage classes:
- `STANDARD`
- `REDUCED_REDUNDANCY`
- `STANDARD_IA`
- `ONEZONE_IA`
- `INTELLIGENT_TIERING`
- `GLACIER`
- `DEEP_ARCHIVE`
- `GLACIER_IR`

### "max concurrency must be positive"

**Cause**: Invalid concurrency configuration

**Solution**:
```yaml
upload:
  max_concurrency: 8  # Must be > 0, recommended: 4-16
```

### "memory limit value exceeds int64 range"

**Cause**: Memory limit too large

**Solution**:
```bash
# Use reasonable memory limit (in GB)
cargoship --memory-limit 4GB upload file s3://bucket/key
```

## Getting Help

### Collect Diagnostic Information

When reporting issues, collect this information:

1. **Version information**:
   ```bash
   cargoship --version
   ```

2. **Configuration** (redact sensitive data):
   ```bash
   cargoship config --show
   ```

3. **Verbose logs**:
   ```bash
   cargoship --verbose --trace <command> 2>&1 | tee cargoship-debug.log
   ```

4. **Runtime statistics**:
   ```bash
   cargoship profile stats
   ```

5. **System information**:
   ```bash
   go version
   uname -a
   aws --version
   ```

### Support Channels

- **GitHub Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Documentation**: https://docs.cargoship.io
- **Examples**: Check the `examples/` directory in the repository

### Reporting Bugs

When reporting bugs, include:

1. Steps to reproduce
2. Expected behavior
3. Actual behavior
4. Diagnostic information (see above)
5. Configuration file (redact sensitive data)
6. Log output with `--verbose --trace`
7. Environment details (OS, Go version, AWS region)

### Feature Requests

Submit feature requests via GitHub Issues with:

1. Clear description of the feature
2. Use case / problem it solves
3. Example usage (CLI commands, config, etc.)
4. Any relevant alternatives considered

## Best Practices

### Configuration Management

1. **Use version control** for configuration files:
   ```bash
   git add .cargoship.yaml
   git commit -m "Update CargoShip configuration"
   ```

2. **Use environment-specific configs**:
   ```bash
   cargoship --file ~/.cargoship-prod.yaml upload ...
   cargoship --file ~/.cargoship-dev.yaml upload ...
   ```

3. **Validate before deploying**:
   ```bash
   cargoship config --validate-detailed --file ~/.cargoship-prod.yaml
   ```

### Monitoring and Observability

1. **Enable CloudWatch metrics**:
   ```yaml
   metrics:
     enabled: true
     namespace: "CargoShip/Production"
   ```

2. **Set appropriate log level**:
   ```yaml
   logging:
     level: info  # Use 'debug' only when troubleshooting
   ```

3. **Collect regular profiles** in production:
   ```bash
   # Daily profile collection
   cargoship profile collect --memory --goroutine
   ```

### Performance Optimization

1. **Run setup wizard** for optimized defaults:
   ```bash
   cargoship setup
   ```

2. **Profile before optimizing**:
   ```bash
   cargoship profile collect --cpu --memory --duration 60
   ```

3. **Test configuration changes**:
   ```bash
   # Benchmark with current config
   time cargoship upload testfile s3://bucket/key

   # Adjust config and re-test
   cargoship setup
   time cargoship upload testfile s3://bucket/key
   ```

4. **Monitor resource usage**:
   ```bash
   cargoship profile stats
   ```

## Advanced Debugging

### Memory Leak Detection

1. **Collect multiple memory profiles**:
   ```bash
   cargoship profile collect --memory
   # Wait and run workload
   sleep 300
   cargoship profile collect --memory
   ```

2. **Compare profiles**:
   ```bash
   go tool pprof -base memory-1.prof memory-2.prof
   ```

### Deadlock Detection

1. **Enable blocking profile**:
   ```bash
   cargoship profile collect --block --mutex
   ```

2. **Analyze blocking operations**:
   ```bash
   go tool pprof block-*.prof
   ```

### Network Issues

1. **Test S3 endpoint connectivity**:
   ```bash
   # Replace with your region
   curl -v https://s3.us-west-2.amazonaws.com
   ```

2. **Check DNS resolution**:
   ```bash
   dig s3.us-west-2.amazonaws.com
   nslookup s3.us-west-2.amazonaws.com
   ```

3. **Verify SSL/TLS**:
   ```bash
   openssl s_client -connect s3.us-west-2.amazonaws.com:443
   ```

4. **Check proxy settings**:
   ```bash
   echo $HTTP_PROXY
   echo $HTTPS_PROXY
   echo $NO_PROXY
   ```

---

**Last Updated**: 2025-10-16
**Version**: v0.4.6
