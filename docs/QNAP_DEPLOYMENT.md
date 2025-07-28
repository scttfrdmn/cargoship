# QNAP CargoShip Ghost Ship Deployment Guide

This guide provides step-by-step instructions for deploying CargoShip Ghost Ship on QNAP NAS systems with Container Station.

## Prerequisites

1. QNAP NAS with Container Station installed
2. SSH access to the QNAP system
3. AWS credentials configured on the NAS
4. Docker engine on development machine for building images

## System Information

- **QNAP Host**: astrapi.local (192.168.1.92)
- **SSH User**: scttfrdmn (local development machine username)
- **Docker Path**: `/share/ZFS530_DATA/.qpkg/container-station/bin/docker`
- **Architecture**: x86_64 (linux/amd64)
- **Data Directory**: `/volume1/genomics_training`
- **Config Location**: `~/cargoship/astrapi-config.yaml`

## Architecture Notes

⚠️ **Critical**: QNAP systems typically run x86_64 architecture. Always build images with `--platform linux/amd64` to avoid exec format errors.

## Deployment Steps

### 1. Build Architecture-Specific Image

```bash
# Build for x86_64 (QNAP architecture)
docker buildx build --platform linux/amd64 -f docker/Dockerfile.astrapi -t cargoship:astrapi-amd64 .

# Save image to file for transfer
docker save cargoship:astrapi-amd64 > /tmp/cargoship-astrapi-amd64.tar
```

### 2. Transfer Files to QNAP

```bash
# Create deployment directory
ssh scttfrdmn@astrapi.local "mkdir -p ~/cargoship"

# Transfer docker image
scp /tmp/cargoship-astrapi-amd64.tar scttfrdmn@astrapi.local:~/cargoship/

# Transfer configuration
scp docker/astrapi-config.yaml scttfrdmn@astrapi.local:~/cargoship/
```

### 3. Load Docker Image on QNAP

```bash
ssh scttfrdmn@astrapi.local "cd ~/cargoship && /share/ZFS530_DATA/.qpkg/container-station/bin/docker load < cargoship-astrapi-amd64.tar"
```

### 4. Deploy Ghost Ship Container

```bash
ssh scttfrdmn@astrapi.local "cd ~/cargoship && /share/ZFS530_DATA/.qpkg/container-station/bin/docker run -d \
  --name cargoship-astrapi-ghost-ship \
  --network host \
  -v /volume1/genomics_training:/volume1/genomics_training:ro \
  -v ~/cargoship/astrapi-config.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v ~/.aws:/root/.aws:ro \
  --restart unless-stopped \
  cargoship:astrapi-amd64"
```

### 5. Verify Deployment

```bash
# Check container status
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker ps | grep cargoship"

# Check logs
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker logs cargoship-astrapi-ghost-ship"
```

## Configuration Details

### Ghost Ship Configuration (`astrapi-config.yaml`)

```yaml
id: "astrapi-ghost-ship"
name: "Astrapi QNAP Ghost Ship"
description: "Production autonomous archival for astrapi.local QNAP system"

# S3 Configuration
s3_config:
  bucket: "cargoship-qnap-production"
  region: "us-east-1"  # Important: Correct region to avoid 301 redirects
  concurrency: 10
  multipart_threshold: 104857600  # 100MB
  multipart_chunk_size: 16777216  # 16MB
  timeout: 600

# Performance optimization (4.6x improvement)
optimization_config:
  enable_bbr: true
  enable_cubic: true
  predictive_mode: true
  max_connections: 20
  buffer_size: 8388608  # 8MB buffer
  chunk_size_mb: 16
  network_optimization: true

# Watch genomics training directory
watch_paths:
  - path: "/volume1/genomics_training"
    include_patterns: ["*.fasta", "*.fastq", "*.vcf", "*.sam", "*.bam", "*.bed", "*.gff", "*.gtf"]
    exclude_patterns: ["*.tmp", "*.lock", "*~"]
    min_age: "1h"
    storage_class: "STANDARD"
    recursive: true

# Performance settings
max_concurrent_jobs: 3
worker_pool_size: 2
scan_interval: "5m"
```

## Troubleshooting

### Common Issues

1. **Exec format error**: Wrong architecture - rebuild with `--platform linux/amd64`
2. **Permission denied**: Check SSH key authentication and file permissions
3. **Docker not found**: Use full path `/share/ZFS530_DATA/.qpkg/container-station/bin/docker`
4. **Volume mount failures**: Ensure directories exist and are accessible
5. **S3 301 redirects**: Verify correct AWS region in configuration

### Monitoring Commands

```bash
# Container status
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker ps"

# Real-time logs
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker logs -f cargoship-astrapi-ghost-ship"

# Container stats
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker stats cargoship-astrapi-ghost-ship"

# Check genomics files
ssh scttfrdmn@astrapi.local "find /volume1/genomics_training -name '*.fasta' -o -name '*.fastq' -o -name '*.vcf' | wc -l"
```

### Maintenance Commands

```bash
# Stop ghost ship
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker stop cargoship-astrapi-ghost-ship"

# Start ghost ship
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker start cargoship-astrapi-ghost-ship"

# Restart ghost ship
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker restart cargoship-astrapi-ghost-ship"

# Remove and redeploy
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker stop cargoship-astrapi-ghost-ship && /share/ZFS530_DATA/.qpkg/container-station/bin/docker rm cargoship-astrapi-ghost-ship"
```

## Performance Notes

- **4.6x Performance Boost**: BBR/CUBIC congestion control with predictive mode
- **529 Genomics Files**: Expected file count in `/volume1/genomics_training`
- **Autonomous Operation**: Files automatically archived after 1 hour
- **Optimized Settings**: 3 concurrent jobs, 2 worker threads, 5-minute scan interval
- **Network Optimization**: 20 max connections, 8MB buffers for high throughput

## Security Considerations

- Container runs as non-root user (cargoship:1000)
- Read-only access to genomics data directory
- AWS credentials mounted read-only
- Network host mode for optimal performance
- Automatic restart unless explicitly stopped