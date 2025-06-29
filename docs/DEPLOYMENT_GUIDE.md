# CargoShip Production Deployment Guide

## Overview

CargoShip is production-ready for enterprise data archiving with AWS integration. This guide covers deployment, configuration, monitoring, and operational best practices.

## Prerequisites

### System Requirements
- **OS**: Linux, macOS, or Windows
- **Memory**: Minimum 512MB RAM, Recommended 2GB+ for large datasets
- **Network**: Stable internet connection for AWS uploads
- **Storage**: Sufficient local storage for temporary compression (typically 1.5x source data size)

### AWS Requirements
- **AWS Account** with appropriate permissions
- **AWS CLI** configured with credentials
- **S3 Buckets** for data storage (will be created if needed)
- **CloudWatch** permissions for metrics (optional but recommended)

## Installation

### Download Binary
```bash
# Linux AMD64
curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64 -o cargoship
chmod +x cargoship

# macOS
curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-amd64 -o cargoship
chmod +x cargoship

# Install to PATH
sudo mv cargoship /usr/local/bin/
```

### Build from Source
```bash
git clone https://github.com/scttfrdmn/cargoship.git
cd cargoship
go build -o cargoship ./cmd/cargoship
```

## AWS Configuration

### IAM Permissions

Create an IAM policy with minimum required permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "CargoShipS3Access",
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:PutObjectAcl",
                "s3:GetObject",
                "s3:DeleteObject",
                "s3:ListBucket",
                "s3:GetBucketLifecycleConfiguration",
                "s3:PutBucketLifecycleConfiguration",
                "s3:DeleteBucketLifecycle"
            ],
            "Resource": [
                "arn:aws:s3:::your-bucket-name",
                "arn:aws:s3:::your-bucket-name/*"
            ]
        },
        {
            "Sid": "CargoShipPricing",
            "Effect": "Allow",
            "Action": [
                "pricing:GetProducts"
            ],
            "Resource": "*"
        },
        {
            "Sid": "CargoShipCloudWatch",
            "Effect": "Allow",
            "Action": [
                "cloudwatch:PutMetricData"
            ],
            "Resource": "*"
        }
    ]
}
```

### AWS Credentials Setup

Choose one of the following methods:

#### Method 1: AWS CLI Profile
```bash
aws configure --profile cargoship
# Enter your AWS Access Key ID, Secret, and region
```

#### Method 2: Environment Variables
```bash
export AWS_ACCESS_KEY_ID=your_key_id
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_DEFAULT_REGION=us-west-2
```

#### Method 3: IAM Role (EC2/ECS)
For EC2 instances or ECS containers, attach the IAM role with the policy above.

## Configuration

### Environment Variables

```bash
# AWS Configuration
export AWS_PROFILE=cargoship
export AWS_DEFAULT_REGION=us-west-2

# CargoShip Configuration
export CARGOSHIP_DEFAULT_BUCKET=my-archive-bucket
export CARGOSHIP_LOG_LEVEL=info
export CARGOSHIP_METRICS_ENABLED=true
export CARGOSHIP_METRICS_NAMESPACE=CargoShip/Production
```

### Configuration File (Optional)

Create `~/.cargoship.yaml`:

```yaml
aws:
  region: us-west-2
  profile: cargoship
  
storage:
  default_bucket: my-archive-bucket
  default_storage_class: INTELLIGENT_TIERING
  kms_key_id: arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012
  
upload:
  max_concurrency: 8
  chunk_size: 16MB
  enable_adaptive_sizing: true
  
metrics:
  enabled: true
  namespace: CargoShip/Production
  flush_interval: 30s
  
logging:
  level: info
  structured: true
```

## Basic Usage

### Cost Estimation
```bash
# Estimate costs with real-time pricing
cargoship estimate ./data --real-time-pricing

# Compare storage classes
cargoship estimate ./data --show-recommendations

# Optimize for specific network conditions
cargoship estimate ./data --bandwidth 100 --show-upload-optimization
```

### Lifecycle Policy Management
```bash
# List available policy templates
cargoship lifecycle --list-templates

# Apply aggressive cost optimization
cargoship lifecycle --bucket my-bucket --template archive-optimization

# Apply with cost estimation
cargoship lifecycle --bucket my-bucket --template intelligent-tiering --estimate-size 500
```

### Data Archiving
```bash
# Archive with optimal settings
cargoship create suitcase ./data --destination s3://my-bucket/archives/

# Archive with custom storage class
cargoship create suitcase ./data --storage-class DEEP_ARCHIVE

# Archive with encryption
cargoship create suitcase ./data --encryption --kms-key-id arn:aws:kms:...
```

## Monitoring and Observability

### CloudWatch Metrics

CargoShip publishes comprehensive metrics to CloudWatch:

#### Upload Performance Metrics
- `UploadDuration` - Time taken for uploads
- `UploadThroughput` - Upload speed in MB/s
- `UploadSize` - Size of uploaded data
- `UploadSuccess` - Success/failure indicator
- `ChunkCount` - Number of multipart chunks
- `Concurrency` - Concurrent upload streams

#### Cost Optimization Metrics
- `EstimatedMonthlyCost` - Projected monthly storage cost
- `EstimatedAnnualCost` - Projected annual storage cost
- `ActualMonthlyCost` - Actual measured costs
- `PotentialSavingsPercent` - Cost savings potential
- `DataSizeGB` - Amount of data archived

#### Network Performance Metrics
- `NetworkBandwidth` - Measured bandwidth in MB/s
- `NetworkLatency` - Network latency in milliseconds
- `OptimalChunkSize` - Recommended chunk size
- `OptimalConcurrency` - Recommended concurrency
- `PacketLoss` - Network packet loss percentage

#### Operational Metrics
- `ActiveUploads` - Currently active uploads
- `QueuedUploads` - Uploads waiting to start
- `CompletedUploads` - Successfully completed uploads
- `FailedUploads` - Failed upload count
- `MemoryUsageMB` - Memory consumption
- `CPUUsagePercent` - CPU utilization

#### Lifecycle Policy Metrics
- `LifecyclePoliciesActive` - Number of active policies
- `LifecycleSavingsPercent` - Cost savings from policies
- `ObjectsTransitioned` - Objects moved between storage classes

### Testing Metrics Integration
```bash
# Test CloudWatch metrics publishing
cargoship metrics --test --namespace CargoShip/Test

# Verify metrics in CloudWatch
aws cloudwatch list-metrics --namespace "CargoShip/Production"
```

### CloudWatch Dashboards

Create a CloudWatch dashboard with key metrics:

```json
{
    "widgets": [
        {
            "type": "metric",
            "properties": {
                "metrics": [
                    ["CargoShip/Production", "UploadThroughput"],
                    [".", "NetworkBandwidth"]
                ],
                "period": 300,
                "stat": "Average",
                "region": "us-west-2",
                "title": "Upload Performance"
            }
        },
        {
            "type": "metric",
            "properties": {
                "metrics": [
                    ["CargoShip/Production", "EstimatedMonthlyCost"],
                    [".", "ActualMonthlyCost"]
                ],
                "period": 3600,
                "stat": "Average",
                "region": "us-west-2",
                "title": "Cost Optimization"
            }
        }
    ]
}
```

### Alerting

Set up CloudWatch alarms for critical metrics:

```bash
# Alert on upload failures
aws cloudwatch put-metric-alarm \
  --alarm-name "CargoShip-Upload-Failures" \
  --alarm-description "Alert when upload failure rate exceeds threshold" \
  --metric-name FailedUploads \
  --namespace CargoShip/Production \
  --statistic Sum \
  --period 300 \
  --threshold 5 \
  --comparison-operator GreaterThanThreshold

# Alert on high costs
aws cloudwatch put-metric-alarm \
  --alarm-name "CargoShip-High-Costs" \
  --alarm-description "Alert when monthly costs exceed budget" \
  --metric-name EstimatedMonthlyCost \
  --namespace CargoShip/Production \
  --statistic Average \
  --period 3600 \
  --threshold 1000 \
  --comparison-operator GreaterThanThreshold
```

## Production Best Practices

### Performance Optimization

1. **Network Optimization**
   ```bash
   # Let CargoShip detect optimal settings
   cargoship estimate ./data --show-upload-optimization
   
   # Use recommended chunk size and concurrency
   cargoship upload --chunk-size 24MB --concurrency 8
   ```

2. **Memory Management**
   ```bash
   # Set memory limits for large datasets
   cargoship upload --memory-limit 2GB
   ```

3. **Parallel Processing**
   ```bash
   # Use multiple prefixes for very large datasets
   cargoship upload --max-prefixes 8 --prefix-pattern hash
   ```

### Cost Optimization

1. **Lifecycle Policies**
   ```bash
   # Apply aggressive optimization for archives
   cargoship lifecycle --template archive-optimization
   
   # Use intelligent tiering for unknown patterns
   cargoship lifecycle --template intelligent-tiering
   ```

2. **Storage Class Selection**
   ```bash
   # Use cost estimation to choose optimal class
   cargoship estimate --show-recommendations
   
   # Archive directly to Deep Archive for long-term storage
   cargoship upload --storage-class DEEP_ARCHIVE
   ```

### Security

1. **Encryption**
   ```bash
   # Use KMS encryption
   cargoship upload --encryption --kms-key-id arn:aws:kms:...
   
   # Use SSE-S3 encryption
   cargoship upload --encryption
   ```

2. **Access Control**
   - Use least-privilege IAM policies
   - Rotate AWS credentials regularly
   - Use IAM roles for EC2/ECS deployments

### Backup and Recovery

1. **Configuration Backup**
   ```bash
   # Export lifecycle policies
   cargoship lifecycle --export backup.json
   
   # Backup configuration
   cp ~/.cargoship.yaml /backup/
   ```

2. **Monitoring Backup Operations**
   - Set up CloudWatch alarms for backup failures
   - Monitor storage costs and usage trends
   - Regular validation of archived data integrity

## Troubleshooting

### Common Issues

#### Upload Failures
```bash
# Check AWS credentials
aws sts get-caller-identity

# Verify bucket permissions
aws s3 ls s3://your-bucket/

# Test with verbose logging
cargoship upload --verbose
```

#### High Costs
```bash
# Analyze cost breakdown
cargoship estimate --show-recommendations

# Check lifecycle policies
cargoship lifecycle --bucket your-bucket

# Review storage class distribution
aws s3api list-objects-v2 --bucket your-bucket --query 'Contents[*].StorageClass'
```

#### Performance Issues
```bash
# Check network conditions
cargoship estimate --show-upload-optimization --bandwidth auto

# Verify optimal settings
cargoship metrics --test

# Monitor system resources
top -p $(pgrep cargoship)
```

### Logging

Enable detailed logging for troubleshooting:

```bash
# Debug level logging
cargoship --verbose upload

# Structured JSON logs
cargoship --log-format json upload

# Log to file
cargoship upload 2>&1 | tee cargoship.log
```

### Support

- **GitHub Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Documentation**: https://cargoship.app
- **AWS Support**: For AWS-specific issues

## Scaling

### Enterprise Deployment

For enterprise-scale deployments:

1. **Container Deployment**
   ```dockerfile
   FROM alpine:latest
   RUN apk add --no-cache ca-certificates
   COPY cargoship /usr/local/bin/
   ENTRYPOINT ["cargoship"]
   ```

2. **Kubernetes Deployment**
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: cargoship
   spec:
     replicas: 3
     selector:
       matchLabels:
         app: cargoship
     template:
       metadata:
         labels:
           app: cargoship
       spec:
         containers:
         - name: cargoship
           image: cargoship:latest
           env:
           - name: AWS_REGION
             value: us-west-2
           - name: CARGOSHIP_METRICS_ENABLED
             value: "true"
   ```

3. **Auto Scaling**
   - Use CloudWatch metrics to trigger scaling
   - Monitor queue depth and upload throughput
   - Scale based on data volume and performance targets

### Multi-Region Deployment

For global deployments:

```bash
# Deploy to multiple regions
cargoship upload --region us-west-2 --bucket us-west-2-archives
cargoship upload --region eu-west-1 --bucket eu-west-1-archives

# Use region-specific lifecycle policies
cargoship lifecycle --region us-west-2 --template archive-optimization
cargoship lifecycle --region eu-west-1 --template archive-optimization
```

## Maintenance

### Regular Tasks

1. **Update CargoShip**
   ```bash
   # Check for updates
   cargoship version --check
   
   # Download latest version
   curl -L https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64 -o cargoship-new
   ```

2. **Monitor Costs**
   ```bash
   # Weekly cost reports
   cargoship estimate --format json | jq '.cost_estimate.total_monthly_cost'
   
   # Lifecycle policy effectiveness
   cargoship lifecycle --bucket your-bucket
   ```

3. **Performance Review**
   ```bash
   # Monthly performance analysis
   cargoship metrics --test
   
   # Network optimization review
   cargoship estimate --show-upload-optimization
   ```

---

**Last Updated**: 2025-06-28
**Version**: Phase 4 Production Ready
**Status**: ✅ VALIDATED AGAINST LIVE AWS