# CargoShip Ghost Ship Deployment Guide

## 🎯 Overview

This guide documents the deployment of CargoShip ghost ships on QNAP and Synology NAS systems after resolving the critical interface mismatch issue in `pkg/launch/ghost_ship.go`.

## 🔧 Prerequisites

### System Requirements
- **QNAP**: Container Station installed, Docker support enabled
- **Synology**: Container Manager installed, Docker support enabled  
- **AWS Credentials**: Properly configured `~/.aws/credentials` and `~/.aws/config` files
- **S3 Buckets**: Target buckets created and accessible

### Required Images
- `cargoship-ghost:qnap-fixed` - Contains interface fix for QNAP deployment
- `cargoship-ghost:synology-fixed` - Contains interface fix for Synology deployment

## 📋 QNAP Deployment (Container Station)

### 1. Image Preparation
```bash
# Build and save image
docker save cargoship-ghost:qnap-fixed -o /tmp/cargoship-ghost-qnap-fixed.tar

# Transfer to QNAP
scp /tmp/cargoship-ghost-qnap-fixed.tar scttfrdmn@astrapi.local:/share/homes/scttfrdmn/

# Load on QNAP
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker load -i /share/homes/scttfrdmn/cargoship-ghost-qnap-fixed.tar"
```

### 2. Configuration Setup
```bash
# Ensure config exists at proper location
/share/docker/cargoship-ghost/ghost_ship.yaml

# Key optimizations for i5/16GB hardware:
s3_config:
  concurrency: 50                 # Increased from 15
  multipart_threshold: 10485760   # 10MB (reduced from 100MB)
  multipart_chunk_size: 10485760  # 10MB chunks

# Performance settings:
max_concurrent_jobs: 4
worker_pool_size: 8
scan_interval: "30s"
```

### 3. Container Deployment
```bash
/share/ZFS530_DATA/.qpkg/container-station/bin/docker run -d \
  --name cargoship-ghost-container \
  --platform linux/amd64 \
  -u 1000:100 \
  -e AWS_PROFILE=aws \
  --memory=4g \
  --cpus=4 \
  --shm-size=1g \
  -v /share/homes/scttfrdmn/.aws:/home/cargoship/.aws:ro \
  -v /share/docker/cargoship-ghost/ghost_ship.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v /share/ZFS530_DATA:/share/ZFS530_DATA \
  -v /share/Public:/share/Public \
  --restart unless-stopped \
  cargoship-ghost:qnap-fixed
```

### 4. Verification
```bash
# Check container status
/share/ZFS530_DATA/.qpkg/container-station/bin/docker ps

# Monitor logs
/share/ZFS530_DATA/.qpkg/container-station/bin/docker logs cargoship-ghost-container --tail 20

# Verify S3 uploads
AWS_PROFILE=aws aws s3 ls s3://cargoship-astrapi-production --recursive --human-readable
```

## 📋 Synology Deployment (Container Manager)

### 1. Image Preparation  
```bash
# Transfer image directly
docker save cargoship-ghost:synology-fixed | ssh scttfrdmn@chubchub.local "/usr/local/bin/docker load"
```

### 2. Configuration Setup
```bash
# Config location
/volume1/docker/cargoship/ghost_ship.yaml

# Update paths for Synology volume structure
watch_paths:
  - path: "/volume1/homes/scttfrdmn"
    
archival_rules:
  - path_pattern: "/volume1/homes/scttfrdmn/**"
```

### 3. Container Deployment
```bash
/usr/local/bin/docker run -d \
  --name cargoship-ghost-container \
  --platform linux/amd64 \
  -u 1027:100 \
  -e AWS_PROFILE=aws \
  --memory=2g \
  -v /volume1/homes/scttfrdmn/.aws:/home/cargoship/.aws:ro \
  -v /volume1/docker/cargoship/ghost_ship.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v /volume1/homes:/volume1/homes \
  --restart unless-stopped \
  cargoship-ghost:synology-fixed
```

### 4. Verification
```bash
# Check container status  
/usr/local/bin/docker ps

# Monitor logs
/usr/local/bin/docker logs cargoship-ghost-container --tail 20
```

## 🔍 Troubleshooting

### Common Issues

#### 1. Interface Mismatch Error
**Symptom**: Ghost ship starts but no archival activity
**Solution**: Ensure using fixed images with `regularTransporterWrapper`

#### 2. AWS Profile Not Found
**Symptom**: "failed to get shared config profile, aws"
**Solution**: Verify AWS credentials mount and `AWS_PROFILE=aws` environment variable

#### 3. Permission Denied on AWS Files
**Symptom**: Container can't read `.aws/credentials`
**Solution**: Fix file permissions: `chmod 600 ~/.aws/credentials`

#### 4. S3 301 Redirect Errors (Synology)
**Symptom**: "PermanentRedirect" errors in logs
**Status**: Known issue, files are being detected and queued correctly
**Priority**: Low (interface functionality confirmed working)

### Performance Optimization

#### QNAP (i5/16GB RAM)
- **Memory**: Consider increasing from 4GB to 6-8GB for better performance
- **Concurrency**: Current 50 concurrent uploads optimized for hardware
- **Multipart**: 10MB threshold works well for mixed file sizes

#### Synology (Atom/32GB RAM)  
- **Memory**: 2GB allocation appropriate for Atom processor
- **Concurrency**: Default 15 concurrent uploads suitable
- **CPU Limits**: Avoid CPU constraints due to kernel limitations

## 📊 Monitoring

### Health Checks
```bash
# QNAP
ssh scttfrdmn@astrapi.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker logs cargoship-ghost-container | grep -E '(completed|error)' | tail -10"

# Synology  
ssh scttfrdmn@chubchub.local "/usr/local/bin/docker logs cargoship-ghost-container | grep -E '(queued|error)' | tail -10"
```

### Performance Metrics
- **QNAP**: ~0.5-0.6 Mbps throughput typical
- **Synology**: File detection and queuing active (S3 uploads pending redirect fix)

## 🚀 Future Improvements

### High Priority
- [ ] Resolve S3 301 redirect issue on Synology
- [ ] Implement centralized monitoring dashboard

### Medium Priority  
- [ ] Standardize deployment scripts for both platforms
- [ ] Add automated health checks
- [ ] Optimize QNAP memory allocation

### Low Priority
- [ ] Create web-based configuration management
- [ ] Add real-time performance monitoring
- [ ] Implement automated backup verification

## 📝 Notes

- **Interface Fix**: The core issue was resolved by implementing `regularTransporterWrapper` in `pkg/launch/ghost_ship.go`
- **AWS SDK**: Both systems now properly use `config.WithSharedConfigProfile()` for profile loading
- **Container Security**: All deployments use proper user mapping and read-only credential mounts
- **Resource Management**: Containers configured with appropriate limits for each platform's hardware capabilities