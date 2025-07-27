# CargoShip Launch Agents

Deploy headless CargoShip agents on your lab infrastructure for automatic, intelligent research data archival.

## Overview

Launch agents are lightweight, containerized agents that run on your lab's NAS boxes, file servers, and compute nodes to automatically detect and archive completed research datasets. They operate headlessly and connect securely to your main CargoShip instance.

**Key Benefits:**
- 🔍 **Smart Detection** - Uses multiple algorithms to identify research data types (genomics, imaging, computational)
- 🏗️ **Headless Operation** - No local UI, managed remotely via secure WebSocket connection
- 📦 **Containerized** - Easy deployment on NAS devices like QNAP and Synology
- 🛡️ **Secure** - TLS-encrypted communication with authentication tokens
- ⚡ **Background Processing** - No interruption to research workflows

## Quick Start

### NAS Box Deployment

Deploy on your lab NAS using the provided Docker Compose configuration:

```bash
# Download the deployment files
curl -O https://raw.githubusercontent.com/scttfrdmn/cargoship/main/docker/launch/docker-compose.yml
curl -O https://raw.githubusercontent.com/scttfrdmn/cargoship/main/docker/launch/agent.yaml

# Create .env file with your configuration
cat > .env << 'EOF'
CARGOSHIP_CONTROLLER_URL=wss://your-cargoship-instance.com
CARGOSHIP_AUTH_TOKEN=your-secure-auth-token
CARGOSHIP_DESTINATION=s3://research-archive
CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
DATA_PATH=/volume1/research-data
AWS_CREDENTIALS_PATH=/volume1/docker/cargoship/.aws
EOF

# Deploy the agent
docker-compose up -d

# Check status
docker-compose logs -f cargoship-agent
```

## Configuration

### Required Environment Variables

The agent requires these essential variables to connect and operate:

| Variable | Description | Example |
|----------|-------------|---------|
| `CARGOSHIP_CONTROLLER_URL` | WebSocket URL of your CargoShip controller | `wss://cargoship.lab.edu` |
| `CARGOSHIP_AUTH_TOKEN` | Authentication token for secure connection | `your-secure-token` |
| `CARGOSHIP_DESTINATION` | S3 destination bucket/prefix | `s3://research-archive` |

### Optional Environment Variables

Customize agent behavior with these optional variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `CARGOSHIP_WATCH_PATHS` | Comma-separated paths to monitor | `/data` |
| `CARGOSHIP_STORAGE_CLASS` | Default storage class | `deep-archive` |
| `CARGOSHIP_MIN_AGE_DAYS` | Minimum file age before archival | `7` |
| `CARGOSHIP_PATTERNS` | File patterns to include | `*` |
| `CARGOSHIP_EXCLUDE_PATTERNS` | File patterns to exclude | `*.tmp,*.lock,.DS_Store` |
| `CARGOSHIP_CHECK_INTERVAL` | How often to scan (seconds) | `300` |
| `CARGOSHIP_AGENT_NAME` | Friendly name for the agent | `CargoShip Agent` |
| `CARGOSHIP_AGENT_DESCRIPTION` | Description of the agent | `Automated research data archival` |
| `CARGOSHIP_LOG_LEVEL` | Log verbosity (debug, info, warn, error) | `info` |

### Configuration File

For advanced configuration, mount a config file to `/config/agent.yaml`:

```yaml
# config/agent.yaml
# Agent identification
id: ""  # Will be auto-generated if empty
name: "CargoShip NAS Agent"
description: "Headless agent for research data archival"

# Controller connection (REQUIRED)
controller_url: ""  # Set via CARGOSHIP_CONTROLLER_URL
auth_token: ""      # Set via CARGOSHIP_AUTH_TOKEN

# TLS configuration for secure communication
tls_config:
  enabled: true
  insecure_skip_verify: false  # Set to true for self-signed certificates

# File watching configuration
watch_paths:
  - path: "/data/genomics"
    include_patterns:
      - "*.fastq.gz"
      - "*.bam"
      - "*.vcf.gz"
    exclude_patterns:
      - "*.tmp"
      - "*.lock"
    min_age: "168h"        # 7 days
    storage_class: "deep-archive"
    recursive: true
  
  - path: "/data/imaging"
    include_patterns:
      - "*.tiff"
      - "*.czi"
      - "*.lsm"
    min_age: "336h"        # 14 days
    storage_class: "glacier"
    recursive: true

# Scan interval (how often to check for new files)
scan_interval: "300s"  # 5 minutes

# Archive configuration
archive:
  destination: ""  # Set via CARGOSHIP_DESTINATION
  storage_class: "deep-archive"
  compression: "zstd"
  encryption: true
  max_concurrent: 2
  retry_attempts: 3
  retry_delay: "30s"

# Health monitoring
health_check:
  enabled: true
  check_interval: "30s"
  report_interval: "300s"  # 5 minutes
  metrics_enabled: true

# Logging level
log_level: "info"  # debug, info, warn, error
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
# View agent logs
docker logs cargoship-agent

# Check if agent is running
docker ps | grep cargoship-agent

# Validate configuration
docker exec cargoship-agent cargoship-launch -validate

# View agent version
docker exec cargoship-agent cargoship-launch -version
```

### Monitoring Communication

Monitor the secure connection to your CargoShip controller:

```bash
# Check WebSocket connection logs
docker logs cargoship-agent 2>&1 | grep -i "controller\|websocket\|connection"

# Monitor agent registration status
docker logs cargoship-agent 2>&1 | grep -i "register\|heartbeat"

# View file detection activity
docker logs cargoship-agent 2>&1 | grep -i "scan\|candidate\|archive"
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

- **Documentation**: [https://cargoship.app](https://cargoship.app)
- **GitHub Issues**: [Report agent-specific issues](https://github.com/scttfrdmn/cargoship/issues)
- **Research Community**: [Join agent discussions](https://github.com/scttfrdmn/cargoship/discussions)

---

**Deploy intelligent archival agents in your research infrastructure today!** 🚢