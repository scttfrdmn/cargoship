# CargoShip User Guide for Researchers

A comprehensive guide for using CargoShip to archive research data to AWS S3 intelligently and cost-effectively.

## Table of Contents

- [Getting Started](#getting-started)
- [Research Workflows](#research-workflows)
- [Cost Optimization](#cost-optimization)
- [Launch Agents](#launch-agents)
- [AWS Setup for Researchers](#aws-setup-for-researchers)
- [Common Use Cases](#common-use-cases)
- [Troubleshooting](#troubleshooting)

## Getting Started

### Installation

Choose the installation method that works best for your research environment:

```bash
# Option 1: Go install (requires Go 1.21+)
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest

# Option 2: Download binary (Linux/macOS)
curl -sSL https://get.cargoship.dev/install.sh | sh

# Option 3: Docker (for containerized environments)
docker pull scttfrdmn/cargoship:latest
```

### Basic Configuration

Create a simple configuration for your research environment:

```bash
# Initialize CargoShip configuration
cargoship config init

# Set your research AWS profile
cargoship config set aws.profile research
cargoship config set aws.region us-east-1

# Configure your research bucket
cargoship config set storage.default_bucket my-research-archive
cargoship config set storage.storage_class deep-archive

# Set budget controls
cargoship config set cost_control.max_monthly_budget 500.00
cargoship config set cost_control.alert_threshold 0.8
```

## Research Workflows

### 1. Survey and Estimate

Before archiving, understand your data and costs:

```bash
# Survey your research data
cargoship survey /data/genomics-project-2024

# Get cost estimates for different scenarios
cargoship estimate /data/completed-analysis \
  --storage-class standard \
  --storage-class glacier \
  --storage-class deep-archive \
  --show-recommendations
```

**Example Output:**
```
📊 Research Data Survey: /data/genomics-project-2024
Total Size: 2.3TB
File Count: 12,847 files
Largest Files: raw_sequences/ (1.8TB), analysis_output/ (500GB)

💰 Storage Cost Estimates (Monthly):
┌─────────────────┬──────────────┬──────────────┐
│ Storage Class   │ Monthly Cost │ Best For     │
├─────────────────┼──────────────┼──────────────┤
│ Standard        │ $529.50     │ Active data  │
│ Glacier         │ $105.90     │ Archive      │
│ Deep Archive    │ $52.95      │ Long-term    │
└─────────────────┴──────────────┴──────────────┘

🧬 Research Data Recommendations:
• Archive raw sequences → Deep Archive (save $476/month)
• Keep final results → Standard (for collaboration)
• Set lifecycle policy → Additional 15% savings
```

### 2. Archive Completed Research

Archive your completed research datasets:

```bash
# Archive completed analysis
cargoship ship /data/completed-analysis \
  --destination s3://research-archive/project-2024/analysis \
  --storage-class deep-archive \
  --description "RNA-seq analysis, experiment batch 3" \
  --tags "project=rna-seq,batch=3,status=complete"

# Archive with metadata preservation
cargoship ship /data/microscopy-images \
  --destination s3://research-archive/imaging/batch-15 \
  --storage-class glacier \
  --preserve-metadata \
  --include-checksums
```

### 3. Intelligent Archive Rules

Set up automatic archival based on your research patterns:

```bash
# Configure intelligent archival
cargoship config set rules.auto_archive true
cargoship config set rules.completed_markers "analysis_complete.txt,FINISHED.flag"
cargoship config set rules.min_age_days 7
cargoship config set rules.file_patterns "*.bam,*.fastq.gz,*.tiff,*.czi"
```

## Cost Optimization

### Research Budget Management

CargoShip helps maximize your research budget:

```bash
# Set monthly budget limits
cargoship budget set 500.00 --alert-at 80%

# Get budget recommendations
cargoship budget analyze /data/to-archive

# Track current spending
cargoship budget status --this-month
```

### Storage Class Selection

Choose the right storage class for your research data:

| Storage Class | Best For | Cost | Retrieval |
|---------------|----------|------|-----------|
| **Standard** | Active analysis data | High | Immediate |
| **Standard-IA** | Recently completed work | Medium | Immediate |
| **Glacier** | Archived datasets | Low | 3-5 hours |
| **Deep Archive** | Long-term preservation | Lowest | 12+ hours |

**Research Recommendations:**
- **Raw sequencing data** → Deep Archive (rarely accessed)
- **Processed datasets** → Glacier (occasional access)
- **Final results/papers** → Standard-IA (collaboration ready)
- **Working data** → Standard (active analysis)

### Lifecycle Policies

Set up automatic transitions to save costs:

```bash
# Create research-optimized lifecycle policy
cargoship lifecycle create research-policy \
  --transition-to-ia 30 \
  --transition-to-glacier 90 \
  --transition-to-deep-archive 365 \
  --apply-to-bucket research-archive
```

## Launch Agents

Deploy CargoShip agents on your lab infrastructure for automatic archival.

### NAS Box Deployment

Deploy on your lab NAS or file server:

```yaml
# docker-compose.yml
version: '3.8'
services:
  cargoship-agent:
    image: scttfrdmn/cargoship:launch
    container_name: lab-archive-agent
    restart: unless-stopped
    volumes:
      - /mnt/lab-nas:/data:ro
      - ./config:/config:ro
      - ~/.aws:/root/.aws:ro
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
      - CARGOSHIP_DESTINATION=s3://lab-research-archive
      - CARGOSHIP_STORAGE_CLASS=deep-archive
      - CARGOSHIP_MAX_MONTHLY_COST=200
      - CARGOSHIP_MIN_AGE_DAYS=7
    networks:
      - lab-network

networks:
  lab-network:
    driver: bridge
```

Deploy the agent:

```bash
# Start the lab archive agent
docker-compose up -d

# Check agent status
docker-compose logs -f cargoship-agent

# Monitor archival activity
cargoship agent status --agent lab-archive-agent
```

### Research Server Deployment

For deployment on research computing servers:

```bash
# Install as systemd service
sudo cargoship agent install \
  --watch-paths /scratch/completed,/data/analysis-output \
  --destination s3://research-archive \
  --storage-class glacier \
  --max-cost 300

# Start the service
sudo systemctl start cargoship-agent
sudo systemctl enable cargoship-agent

# Monitor logs
sudo journalctl -u cargoship-agent -f
```

### Agent Configuration

Configure intelligent detection patterns:

```yaml
# config/agent.yaml
watch:
  paths:
    - /data/completed
    - /analysis/output
    - /sequencing/finished
  
  patterns:
    include:
      - "*.bam"
      - "*.fastq.gz"
      - "*.tiff"
      - "analysis_complete.txt"
    exclude:
      - "*.tmp"
      - "*.lock"
      - ".DS_Store"

rules:
  min_age_days: 7
  auto_archive: true
  completed_markers:
    - "ANALYSIS_COMPLETE"
    - "processing_finished.flag"
  
archive:
  destination: s3://research-archive/{project}/{date}
  storage_class: deep-archive
  max_monthly_cost: 200
  
notifications:
  email: lab-admin@university.edu
  slack_webhook: https://hooks.slack.com/services/YOUR/WEBHOOK
```

## AWS Setup for Researchers

### Simple AWS Configuration

Set up AWS for research use:

```bash
# Configure AWS credentials
aws configure --profile research
# AWS Access Key ID: YOUR_ACCESS_KEY
# AWS Secret Access Key: YOUR_SECRET_KEY  
# Default region: us-east-1
# Default output format: json

# Create research bucket
aws s3 mb s3://my-research-archive --profile research

# Set bucket lifecycle policy
cargoship lifecycle apply research-policy \
  --bucket my-research-archive
```

### Research IAM Policy

Use this minimal IAM policy for research access:

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
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-research-archive",
        "arn:aws:s3:::my-research-archive/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetBucketLocation",
        "s3:ListAllMyBuckets"
      ],
      "Resource": "*"
    }
  ]
}
```

## Common Use Cases

### Genomics Research

Archive sequencing data efficiently:

```bash
# Archive raw sequencing data
cargoship ship /data/raw-sequences \
  --destination s3://genomics-archive/project-001/raw \
  --storage-class deep-archive \
  --compression zstd \
  --tags "project=cancer-genomics,grant=NIH-001"

# Archive analysis results  
cargoship ship /data/variant-calls \
  --destination s3://genomics-archive/project-001/analysis \
  --storage-class glacier \
  --preserve-metadata \
  --include-checksums
```

### Microscopy Data

Handle large imaging datasets:

```bash
# Archive microscopy images
cargoship ship /data/confocal-images \
  --destination s3://imaging-archive/experiment-456 \
  --storage-class glacier \
  --max-archive-size 10GB \
  --compression lz4 \
  --parallel 4
```

### Long-term Data Preservation

Set up institutional archival:

```bash
# Create preservation-grade archive
cargoship ship /data/thesis-dataset \
  --destination s3://university-preservation/student-123 \
  --storage-class deep-archive \
  --encrypt-kms arn:aws:kms:us-east-1:account:key/preservation-key \
  --preserve-metadata \
  --integrity-check sha256 \
  --redundancy 3
```

## Troubleshooting

### Common Issues

**Agent not detecting files:**
```bash
# Check agent logs
docker logs cargoship-agent

# Verify watch patterns
cargoship agent test-patterns /data/test-file.bam

# Test file age detection
cargoship agent test-age /data/analysis-output
```

**High costs:**
```bash
# Analyze current spending
cargoship costs analyze --detailed

# Check storage class distribution
cargoship costs breakdown --by-storage-class

# Get optimization recommendations
cargoship costs optimize --dry-run
```

**Transfer failures:**
```bash
# Check AWS credentials
cargoship config test aws

# Verify S3 permissions
cargoship test s3://my-research-bucket

# Check network connectivity
cargoship network test --region us-east-1
```

### Getting Help

- **Documentation**: [https://cargoship.dev](https://cargoship.dev)
- **GitHub Issues**: [Report bugs and request features](https://github.com/scttfrdmn/cargoship/issues)
- **Research Community**: [Join discussions](https://github.com/scttfrdmn/cargoship/discussions)
- **Slack Channel**: `#cargoship` on Research Computing Slack

---

**Need help with your research data archival? We're here to help!** 🚢