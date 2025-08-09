# CargoShip Launch & Ghost Ship Architecture

> **🚢 From a coordinating CargoShip!** - Deploy autonomous archival agents to remote NAS systems that operate independently while remaining coordinated by a central controller.

## Overview

CargoShip's launch and ghost ship architecture implements **true distributed autonomous archival**. Ghost ships are deployed to remote NAS devices where they continuously monitor local files and archive them directly to cloud storage - no data round-trips, no manual intervention, just intelligent autonomous operation with central coordination.

## Quick Start

### 1. Local Development & Testing
```bash
# Start the complete development environment
cd docker/development
docker-compose up -d

# Run the test suite
./scripts/test-suite.sh --generate-data all

# Access monitoring dashboards
open http://localhost:3000  # Grafana (admin/admin123)
open http://localhost:8080  # Controller API
```

### 2. Deploy to Production NAS
```bash
# Deploy ghost ship to astrapi.local (QNAP NAS)
./scripts/launch-ghost-ship.sh \
  --target astrapi.local \
  --controller wss://your-controller.local:8080 \
  --config configs/astrapi/ghost-ship-production.yaml \
  launch

# Monitor the ghost ship
./scripts/launch-ghost-ship.sh --target astrapi.local status
./scripts/launch-ghost-ship.sh --target astrapi.local logs
```

## Architecture Highlights

### 🎯 True Launch Capability
- **Remote Deployment**: Deploy ghost ships to any NAS device via Docker
- **Autonomous Operation**: Continues working even when controller is offline  
- **Direct Archival**: Files archived directly from NAS to cloud (no round-trip)
- **Central Coordination**: Monitor and control from a coordinating CargoShip

### 👻 Ghost Ship Features
- **Rule-Based Intelligence**: Configurable archival rules for different file types
- **Storage Class Optimization**: Automatic selection of Standard, IA, Glacier, Deep Archive
- **High Performance**: 4.6x speed improvement with BBR/CUBIC optimization
- **Real-Time Monitoring**: Continuous status reporting and metrics collection

### 📡 Distributed Coordination
- **WebSocket Communication**: Real-time bidirectional communication
- **Job Assignment**: Central controller can assign specific archival jobs
- **Health Monitoring**: Continuous monitoring of all deployed ghost ships
- **Scalable Architecture**: Deploy to unlimited NAS systems

## Performance Results

From real-world testing on astrapi.local QNAP NAS:

```
🚀 Performance Achievements:
├── 4.6x faster than standard S3 uploads
├── 176.68 MB/s aggregate throughput (100 concurrent uploads)
├── 56.34 MB/s sustained rate for large files (3.1GB)
├── 95.28 MB/s download performance
└── 28.3% utilization of 5Gbps internet connection (highly efficient)

📊 System Utilization:
├── 10Gbps local network to astrapi.local
├── 5Gbps internet connection to AWS
├── <2GB memory usage per ghost ship
└── <80% CPU usage under high load
```

## Real-World Use Cases

### 1. Legal Office Compliance
```yaml
# Automatically archive legal documents
archival_rules:
  - name: "legal_document_compliance"
    path_pattern: "/volume1/Legal/Cases/**"
    file_pattern: "*.{pdf,doc,docx}"
    min_age: "24h"
    storage_class: "STANDARD_IA"
    encryption: true
    delete_after_archive: false
```

### 2. Photography Studio Workflow
```yaml
# Long-term photo preservation
archival_rules:
  - name: "photo_preservation"
    path_pattern: "/volume1/Photos/Completed/**"
    file_pattern: "*.{raw,cr2,nef,dng}"
    min_age: "7d"
    storage_class: "GLACIER" 
    compression: "none"
    delete_after_archive: false
```

### 3. IT Backup Lifecycle
```yaml
# Automated backup retention
archival_rules:
  - name: "backup_lifecycle"
    path_pattern: "/volume1/Backups/Monthly/**"
    file_pattern: "*.{zip,tar.gz}"
    min_age: "30d"
    storage_class: "DEEP_ARCHIVE"
    delete_after_archive: true  # Free up local space
```

## Key Files and Components

### Core Implementation
- **`pkg/launch/central_controller.go`** - Central coordination hub
- **`pkg/launch/ghost_ship.go`** - Autonomous archival agent
- **`pkg/launch/agent.go`** - Launch agent management
- **`pkg/launch/file_watcher.go`** - Filesystem monitoring
- **`pkg/launch/local_archiver.go`** - High-performance S3 uploads

### Configuration Examples
- **`examples/launch/ghost_ship_config.yaml`** - Complete ghost ship configuration
- **`configs/astrapi/ghost-ship-production.yaml`** - Production astrapi.local config
- **`examples/launch/central_controller_config.yaml`** - Controller configuration

### Deployment Tools
- **`scripts/launch-ghost-ship.sh`** - Ghost ship deployment script
- **`scripts/deploy-astrapi.sh`** - astrapi.local deployment automation
- **`scripts/test-suite.sh`** - Comprehensive testing framework

### Development Environment
- **`docker/development/docker-compose.yml`** - Complete local dev stack
- **`docker/development/testing-plan.md`** - Testing strategy
- **`docs/DEVELOPMENT_WORKFLOW.md`** - Development guide

## Testing Strategy

### Local Development Testing
```bash
# Component tests
./scripts/test-suite.sh local

# Integration tests  
./scripts/test-suite.sh integration

# Performance tests
./scripts/test-suite.sh performance
```

### Real-World Validation
```bash
# Deploy and test on actual QNAP NAS
./scripts/test-suite.sh astrapi

# Performance benchmarking
ssh admin@astrapi.local 'docker logs -f cargoship-ghost-ship'
```

### Monitoring & Debugging
```bash
# View system status
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/status

# Monitor ghost ship metrics
curl http://localhost:9091/metrics | grep cargoship_

# Check archival progress
aws s3 ls s3://your-bucket/astrapi/
```

## Configuration Options

### Ghost Ship Configuration
```yaml
# Identification
id: "production-ghost-ship"
name: "Production Ghost Ship"

# Performance tuning
max_concurrent_jobs: 8
worker_pool_size: 4
scan_interval: "5m"

# S3 optimization
optimization_config:
  enable_bbr: true
  enable_cubic: true
  max_connections: 25
  chunk_size_mb: 32

# Controller integration
controller_url: "wss://controller.local:8080"
reporting_enabled: true
report_interval: "1m"
```

### Central Controller Configuration  
```yaml
# Server settings
listen_address: "0.0.0.0"
port: 8080
tls_enabled: true

# Agent management
agent_timeout: "5m"
ping_interval: "30s"

# Authentication
auth_enabled: true
auth_tokens:
  - "${CARGOSHIP_PRODUCTION_TOKEN}"
```

## Security Features

- **🔒 TLS Encryption**: All communications encrypted with WebSocket Secure (WSS)
- **🎫 Token Authentication**: Secure API token-based authentication
- **🛡️ Certificate Validation**: Proper CA certificate chain validation
- **🔐 S3 Encryption**: Optional server-side encryption for archived files
- **📝 Audit Logging**: Complete operation audit trail
- **🚫 No Credential Exposure**: Secure credential handling and storage

## Monitoring & Alerting

### Key Metrics
- `cargoship_active_jobs` - Current active archival jobs
- `cargoship_bytes_archived_total` - Total bytes archived
- `cargoship_average_throughput_mbps` - Transfer performance
- `cargoship_ghost_ships_connected` - Connected agents
- `cargoship_failed_jobs_total` - Failed operations

### Dashboards
- **Grafana**: Real-time monitoring dashboards
- **Prometheus**: Metrics collection and alerting
- **Controller API**: RESTful status and management interface

## Deployment Architecture

```
┌─────────────────────┐     WebSocket/TLS     ┌──────────────────────┐
│  Central Controller │◄────────────────────►│    Ghost Ship        │
│  (Coordinator)      │   Real-time Comms    │   (astrapi.local)    │
└─────────────────────┘                      └──────────────────────┘
         │                                             │
         │ Monitors & Controls                         │ Direct Upload
         │ • Status Tracking                           │ (No Round-Trip)
         │ • Job Assignment                            │
         │ • Health Monitoring                         ▼
         │ • Performance Metrics                ┌─────────────┐
         │                                      │   AWS S3    │
    ┌────▼────┐                                 │   Storage   │
    │ Ghost   │─────── Autonomous ──────────────│             │
    │ Ship    │        Archival                 │ • Standard  │
    │ (NAS 2) │                                 │ • IA        │
    └─────────┘                                 │ • Glacier   │
                                                │ • Deep Arc  │
                                                └─────────────┘
```

## Benefits

### 🎯 **Operational Excellence**
- **Zero Data Round-Trip**: Files archived directly from NAS to cloud
- **Autonomous Operation**: Continues working even when disconnected
- **Intelligent Classification**: Rule-based file type and storage class selection
- **Cost Optimization**: Automatic storage class selection based on access patterns

### 📈 **Performance & Scale**
- **High Throughput**: 4.6x faster than standard uploads with network optimization
- **Concurrent Operations**: Handle 100+ simultaneous file operations
- **Unlimited Scale**: Deploy ghost ships to any number of NAS systems
- **Efficient Bandwidth**: Optimal utilization of high-speed connections

### 🔧 **Management & Control**
- **Central Visibility**: Monitor all distributed operations from one interface
- **Remote Management**: Deploy, configure, and control agents remotely
- **Real-Time Status**: Live monitoring of all archival operations
- **Flexible Configuration**: Dynamic rule updates and policy changes

This architecture delivers the true CargoShip vision: **"From a coordinating CargoShip!"** - enabling autonomous distributed archival while maintaining central coordination and control.