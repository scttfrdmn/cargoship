# CargoShip Launch Agents

Deploy CargoShip agents on your lab infrastructure for automatic, intelligent research data archival.

## Overview

Launch agents are containerized CargoShip instances that run on your lab's NAS boxes, file servers, and compute nodes to automatically detect and archive completed research datasets.

**Key Benefits:**
- 🔍 **Automatic Detection** - Identifies completed research datasets
- 💰 **Cost Optimization** - Intelligent storage class selection
- ⚡ **Background Operation** - No interruption to research workflows
- 🛡️ **Reliable** - Handles network issues and retries automatically

## Quick Start

### NAS Box Deployment

Deploy on your lab NAS using Docker Compose:

```yaml
# docker-compose.yml
version: '3.8'
services:
  cargoship-agent:
    image: scttfrdmn/cargoship:launch
    container_name: research-archive-agent
    restart: unless-stopped
    volumes:
      - /mnt/research-data:/data:ro
      - ./config:/config:ro
      - ~/.aws:/root/.aws:ro
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
      - CARGOSHIP_DESTINATION=s3://research-archive
      - CARGOSHIP_STORAGE_CLASS=deep-archive
      - CARGOSHIP_MAX_MONTHLY_COST=300
      - CARGOSHIP_MIN_AGE_DAYS=7
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

Start the agent:

```bash
# Deploy the agent
docker-compose up -d

# Check status
docker-compose logs -f cargoship-agent

# Monitor archival activity
docker exec cargoship-agent cargoship agent status
```

## Configuration

### Environment Variables

Configure the agent using environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `CARGOSHIP_WATCH_PATHS` | Comma-separated paths to monitor | `/data` |
| `CARGOSHIP_DESTINATION` | S3 destination bucket/prefix | Required |
| `CARGOSHIP_STORAGE_CLASS` | Default storage class | `deep-archive` |
| `CARGOSHIP_MAX_MONTHLY_COST` | Budget limit in USD | `1000` |
| `CARGOSHIP_MIN_AGE_DAYS` | Minimum file age before archival | `7` |
| `CARGOSHIP_PATTERNS` | File patterns to include | `*` |
| `CARGOSHIP_EXCLUDE_PATTERNS` | File patterns to exclude | `*.tmp,*.lock` |
| `CARGOSHIP_CHECK_INTERVAL` | How often to scan (seconds) | `3600` |

### Configuration File

For advanced configuration, mount a config file:

```yaml
# config/agent.yaml
agent:
  name: "lab-nas-agent"
  check_interval: 3600  # 1 hour
  max_concurrent_uploads: 3

watch:
  paths:
    - path: "/data/genomics/completed"
      storage_class: "deep-archive"
      max_cost: 100
    - path: "/data/imaging/analysis-output"
      storage_class: "glacier"
      max_cost: 200
  
  patterns:
    include:
      - "*.bam"
      - "*.fastq.gz"
      - "*.tiff"
      - "*.czi"
      - "analysis_complete.txt"
    exclude:
      - "*.tmp"
      - "*.lock"
      - ".DS_Store"
      - "*.partial"

rules:
  min_age_days: 7
  auto_archive: true
  completed_markers:
    - "ANALYSIS_COMPLETE"
    - "processing_finished.flag"
    - "job_done.txt"
  
  size_limits:
    max_file_size: "50GB"
    max_archive_size: "500GB"

archive:
  destination: "s3://research-archive/{project}/{year}/{month}"
  storage_class: "deep-archive"
  compression: "zstd"
  encrypt: true
  
  lifecycle:
    transition_to_glacier: 90    # days
    transition_to_deep_archive: 365

budget:
  max_monthly_cost: 300.00
  alert_threshold: 0.8
  alert_email: "lab-admin@university.edu"

notifications:
  slack:
    webhook: "https://hooks.slack.com/services/YOUR/WEBHOOK"
    channel: "#lab-archive"
  email:
    smtp_server: "smtp.university.edu"
    from: "cargoship@lab.university.edu"
    to: ["lab-admin@university.edu"]
```

## Research Patterns

### Genomics Labs

Configure for sequencing and analysis workflows:

```yaml
# Genomics-optimized configuration
watch:
  paths:
    - path: "/data/raw-sequences"
      storage_class: "deep-archive"
      patterns: ["*.fastq.gz", "*.fq.gz"]
      min_age_days: 30
    - path: "/data/alignments"
      storage_class: "glacier"
      patterns: ["*.bam", "*.sam"]
      min_age_days: 14
    - path: "/data/variants"
      storage_class: "standard-ia"
      patterns: ["*.vcf", "*.vcf.gz"]
      min_age_days: 7

rules:
  completed_markers:
    - "analysis_complete.txt"
    - "pipeline_finished.flag"
    - "QC_passed.txt"
```

### Imaging Labs

Configure for microscopy and image analysis:

```yaml
# Imaging-optimized configuration
watch:
  paths:
    - path: "/data/raw-images"
      storage_class: "glacier"
      patterns: ["*.tiff", "*.tif", "*.czi", "*.lsm"]
      min_age_days: 14
    - path: "/data/processed-images"
      storage_class: "standard-ia"
      patterns: ["*.png", "*.jpg", "*.pdf"]
      min_age_days: 7

rules:
  completed_markers:
    - "experiment_complete.txt"
    - "analysis_finished.flag"
  
  size_limits:
    max_file_size: "10GB"    # Large microscopy files
    max_archive_size: "1TB"
```

### Computational Research

Configure for compute-intensive workflows:

```yaml
# Compute-optimized configuration
watch:
  paths:
    - path: "/scratch/completed"
      storage_class: "deep-archive"
      patterns: ["*.out", "*.log", "*.dat"]
      min_age_days: 3
    - path: "/results/published"
      storage_class: "standard-ia"
      patterns: ["*.csv", "*.xlsx", "*.pdf"]
      min_age_days: 1

rules:
  completed_markers:
    - "job_complete.txt"
    - "SLURM_JOB_FINISHED"
```

## Deployment Scenarios

### Synology NAS

Deploy on Synology DiskStation:

```bash
# SSH into your Synology NAS
ssh admin@your-nas.local

# Create directory structure
sudo mkdir -p /volume1/docker/cargoship/{config,logs}

# Create docker-compose.yml
cat > /volume1/docker/cargoship/docker-compose.yml << 'EOF'
version: '3.8'
services:
  cargoship-agent:
    image: scttfrdmn/cargoship:launch
    container_name: synology-cargoship-agent
    restart: unless-stopped
    volumes:
      - /volume1/research-data:/data:ro
      - /volume1/docker/cargoship/config:/config:ro
      - /volume1/docker/cargoship/.aws:/root/.aws:ro
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/completed
      - CARGOSHIP_DESTINATION=s3://lab-archive
      - CARGOSHIP_STORAGE_CLASS=deep-archive
      - CARGOSHIP_MAX_MONTHLY_COST=200
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
EOF

# Start the agent
cd /volume1/docker/cargoship
docker-compose up -d
```

### QNAP NAS

Deploy on QNAP using Container Station:

1. Open Container Station
2. Create a new application using Docker Compose
3. Use the following configuration:

```yaml
version: '3.8'
services:
  cargoship-agent:
    image: scttfrdmn/cargoship:launch
    container_name: qnap-cargoship-agent
    restart: unless-stopped
    volumes:
      - /share/Research:/data:ro
      - /share/Container/cargoship/config:/config:ro
      - /share/Container/cargoship/.aws:/root/.aws:ro
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/analysis-complete,/data/experiments/finished
      - CARGOSHIP_DESTINATION=s3://qnap-research-archive
      - CARGOSHIP_STORAGE_CLASS=glacier
```

### Linux Server

Deploy on a Linux research server:

```bash
# Install Docker if not present
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Create service directory
sudo mkdir -p /opt/cargoship/{config,logs}

# Create systemd service
sudo tee /etc/systemd/system/cargoship-agent.service << 'EOF'
[Unit]
Description=CargoShip Research Data Archive Agent
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/bin/docker run --rm \
  --name cargoship-agent \
  -v /data/research:/data:ro \
  -v /opt/cargoship/config:/config:ro \
  -v /home/researcher/.aws:/root/.aws:ro \
  -e CARGOSHIP_WATCH_PATHS=/data/completed \
  -e CARGOSHIP_DESTINATION=s3://server-research-archive \
  -e CARGOSHIP_STORAGE_CLASS=deep-archive \
  scttfrdmn/cargoship:launch
ExecStop=/usr/bin/docker stop cargoship-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Enable and start the service
sudo systemctl daemon-reload
sudo systemctl enable cargoship-agent
sudo systemctl start cargoship-agent

# Check status
sudo systemctl status cargoship-agent
sudo journalctl -u cargoship-agent -f
```

## Monitoring and Management

### Agent Status

Check agent health and activity:

```bash
# View agent status
docker exec cargoship-agent cargoship agent status

# Check recent archival activity
docker exec cargoship-agent cargoship agent activity --last 24h

# View current configuration
docker exec cargoship-agent cargoship agent config show

# Test file detection patterns
docker exec cargoship-agent cargoship agent test-patterns /data/test-file.bam
```

### Cost Monitoring

Track archival costs:

```bash
# Check current month costs
docker exec cargoship-agent cargoship costs status --this-month

# Get cost breakdown by storage class
docker exec cargoship-agent cargoship costs breakdown --by-storage-class

# View budget status
docker exec cargoship-agent cargoship budget status
```

### Logs and Debugging

Monitor agent activity:

```bash
# View live logs
docker logs -f cargoship-agent

# Check for errors
docker logs cargoship-agent 2>&1 | grep ERROR

# Debug file detection
docker exec cargoship-agent cargoship agent debug-scan /data/problematic-directory
```

## Troubleshooting

### Common Issues

**Agent not detecting files:**

```bash
# Check file permissions
docker exec cargoship-agent ls -la /data/

# Test pattern matching
docker exec cargoship-agent cargoship agent test-patterns /data/test-file.fastq.gz

# Verify minimum age requirements
docker exec cargoship-agent cargoship agent test-age /data/old-file.bam
```

**High costs or unexpected charges:**

```bash
# Analyze current spending
docker exec cargoship-agent cargoship costs analyze --detailed

# Check for large files
docker exec cargoship-agent cargoship costs top-files --limit 10

# Verify storage class selection
docker exec cargoship-agent cargoship agent config show | grep storage_class
```

**AWS credential issues:**

```bash
# Test AWS access
docker exec cargoship-agent cargoship config test aws

# Check S3 bucket permissions
docker exec cargoship-agent cargoship test s3://your-research-bucket

# Verify AWS configuration
docker exec cargoship-agent aws sts get-caller-identity
```

### Support and Community

- **Documentation**: [https://cargoship.dev/docs/launch-agent](https://cargoship.dev/docs/launch-agent)
- **GitHub Issues**: [Report agent-specific issues](https://github.com/scttfrdmn/cargoship/issues)
- **Research Community**: [Join agent discussions](https://github.com/scttfrdmn/cargoship/discussions)

---

**Deploy intelligent archival agents in your research infrastructure today!** 🚢