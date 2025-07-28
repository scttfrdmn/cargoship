# CargoShip Launch & Ghost Ship Architecture

## Overview

CargoShip's launch and ghost ship architecture enables autonomous distributed archival operations across multiple NAS devices. The system consists of a central controller that coordinates multiple autonomous "ghost ships" - agents that run on remote NAS systems and perform intelligent file archival directly to cloud storage without data round-trips.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            CargoShip Launch Architecture                    │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────┐     WebSocket/TLS      ┌──────────────────────────────┐
│   Central Controller│◄─────────────────────►│       Ghost Ship             │
│   (Coordinator)     │      Bidirectional     │    (astrapi.local)           │
│                     │      Communication     │                              │
│  ┌─────────────────┐│                       │ ┌──────────────────────────┐ │
│  │  WebSocket API  ││                       │ │   Autonomous Archival    │ │
│  │  REST API       ││                       │ │   File Watcher           │ │
│  │  Health Monitor ││                       │ │   Rule Engine            │ │
│  │  Job Dispatcher ││                       │ │   S3 Optimization       │ │
│  └─────────────────┘│                       │ └──────────────────────────┘ │
└─────────────────────┘                       └──────────────────────────────┘
         │                                                    │
         │ Monitors & Controls                                │ Direct Upload
         │ • Agent Status                                     │ (No Round-Trip)
         │ • Job Assignment                                   │
         │ • Health Tracking                                  │
         │ • Performance Metrics                              │
         │                                                    ▼
    ┌────▼────┐                                         ┌─────────────┐
    │ Ghost   │                                         │   AWS S3    │
    │ Ship    │──────── Autonomous Archival ───────────│   Storage   │
    │ (NAS 2) │                                         │             │
    └─────────┘                                         │ • Standard  │
         │                                              │ • IA        │
         │ Local Files                                  │ • Glacier   │
         ▼                                              │ • Deep Arc  │ 
   ┌─────────────┐                                      └─────────────┘
   │ NAS Storage │
   │ /volume1/   │
   └─────────────┘
```

## Core Components

### 1. Central Controller (`pkg/launch/central_controller.go`)

**Purpose**: Coordinates and monitors distributed ghost ships

**Key Features**:
- **WebSocket Server**: Real-time bidirectional communication with agents
- **REST API**: Management interface for monitoring and control
- **Agent Registry**: Tracks connected ghost ships and their capabilities
- **Job Dispatcher**: Assigns archival jobs to appropriate agents
- **Health Monitor**: Continuous health checking and status aggregation
- **Authentication**: Token-based security with TLS support

**API Endpoints**:
```
GET  /api/v1/status              # Global system status
GET  /api/v1/ghostships          # List connected ghost ships
GET  /api/v1/ghostships/{id}     # Get specific ghost ship details
GET  /api/v1/ghostships/{id}/jobs # Get ghost ship jobs
POST /api/v1/ghostships/{id}/launch # Launch ghost ship operations
POST /api/v1/ghostships/{id}/stop   # Stop ghost ship operations
```

**WebSocket Endpoints**:
```
/api/v1/agents/connect           # Agent connection endpoint
/api/v1/ghostships/connect       # Ghost ship connection endpoint
```

### 2. Ghost Ship (`pkg/launch/ghost_ship.go`)

**Purpose**: Autonomous archival agent running on remote NAS systems

**Key Features**:
- **Autonomous Operation**: Continues working even when disconnected from controller
- **Rule-Based Archival**: Configurable rules for different file types and conditions
- **File Watcher**: Real-time filesystem monitoring with pattern matching
- **S3 Optimization**: BBR/CUBIC congestion control for high-performance uploads
- **Storage Class Intelligence**: Automatic storage class selection based on rules
- **Controller Integration**: Reports status and receives job assignments

**Archival Rules Engine**:
```yaml
archival_rules:
  - name: "document_archival"
    path_pattern: "/volume1/Public/Documents/**"
    file_pattern: "*.{pdf,doc,docx}"
    min_age: "1h"
    storage_class: "STANDARD_IA"
    encryption: true
    priority: 2
```

### 3. Launch Agent (`pkg/launch/agent.go`)

**Purpose**: Manages ghost ship deployment and communication

**Key Features**:
- **Controller Connection**: Secure WebSocket communication
- **Health Monitoring**: Continuous status reporting
- **Job Processing**: Handles assigned archival tasks
- **Configuration Management**: Dynamic configuration updates

### 4. Supporting Components

#### File Watcher (`pkg/launch/file_watcher.go`)
- **Filesystem Monitoring**: Uses fsnotify for real-time file events
- **Pattern Matching**: Include/exclude patterns with glob support
- **Recursive Scanning**: Directory tree monitoring
- **Event Filtering**: Intelligent filtering of system files and temporaries

#### Local Archiver (`pkg/launch/local_archiver.go`)
- **High-Performance Uploads**: Concurrent S3 operations
- **Progress Tracking**: Real-time upload progress and statistics
- **Error Handling**: Retry logic with exponential backoff
- **Optimization Integration**: BBR/CUBIC transport protocols

#### Controller Connection (`pkg/launch/controller.go`)
- **WebSocket Management**: Connection lifecycle and reconnection
- **Message Protocol**: Structured communication with controller
- **Authentication**: Secure token-based authentication
- **Heartbeat System**: Connection health monitoring

## Message Protocol

### Agent-to-Controller Messages
```json
{
  "type": "register",
  "id": "msg-123",
  "timestamp": "2025-01-27T10:00:00Z",
  "agent_id": "astrapi-ghost-01",
  "data": {
    "name": "Astrapi Ghost Ship",
    "capabilities": ["file_watching", "s3_upload"],
    "watch_paths": ["/volume1/Public"]
  }
}
```

### Controller-to-Agent Messages
```json
{
  "type": "job_assign",
  "id": "msg-456", 
  "timestamp": "2025-01-27T10:01:00Z",
  "agent_id": "astrapi-ghost-01",
  "data": {
    "job_id": "job-789",
    "path": "/volume1/Public/Documents/*.pdf",
    "destination": "documents/",
    "storage_class": "STANDARD_IA"
  }
}
```

## Deployment Architecture

### Local Development Environment
```
docker-compose up -d
├── cargoship-controller (port 8080)
├── ghost-ship-1 (port 9091) 
├── ghost-ship-2 (port 9092)
├── localstack (S3 simulation)
├── prometheus (metrics)
└── grafana (dashboards)
```

### Production Deployment (astrapi.local)
```
Central Controller (Local/Cloud)
     │
     │ WebSocket/TLS
     ▼
Docker Container on astrapi.local
├── Ghost Ship Agent
├── File Watcher
├── S3 Transporter
└── Metrics Exporter
     │
     │ Direct Upload
     ▼
   AWS S3 Storage
```

## Configuration Management

### Ghost Ship Configuration
```yaml
# Ghost ship identification
id: "astrapi-ghost-01"
name: "Astrapi Ghost Ship"

# S3 Configuration
s3_config:
  bucket: "cargoship-astrapi-archive"
  region: "us-west-2"
  concurrency: 15

# Optimization
optimization_config:
  enable_bbr: true
  enable_cubic: true
  max_connections: 25

# Watch paths
watch_paths:
  - path: "/volume1/Public/Documents"
    include_patterns: ["*.pdf", "*.doc*"]
    storage_class: "STANDARD_IA"

# Controller integration
controller_url: "wss://controller.local:8080"
auth_token: "${CARGOSHIP_AUTH_TOKEN}"
```

### Central Controller Configuration
```yaml
# Server settings
listen_address: "0.0.0.0"
port: 8080
tls_enabled: true

# Authentication
auth_enabled: true
auth_tokens:
  - "${CARGOSHIP_CONTROLLER_TOKEN}"

# Agent management
agent_timeout: "5m"
ping_interval: "30s"
```

## Performance Characteristics

### Network Optimization
- **BBR Congestion Control**: Bandwidth-delay product optimization
- **CUBIC Algorithm**: High-bandwidth, low-latency performance
- **Predictive Bandwidth**: Dynamic bandwidth allocation
- **Connection Pooling**: Efficient connection reuse

### Measured Performance
- **Baseline Performance**: Standard S3 uploads
- **Optimized Performance**: 4.6x improvement with BBR/CUBIC
- **Aggregate Throughput**: 176.68 MB/s with 100 concurrent uploads
- **Network Utilization**: 28.3% of 5Gbps connection (highly efficient)
- **Large File Performance**: 56.34 MB/s sustained on 3.1GB uploads

### Scaling Characteristics
- **Concurrent Operations**: 100+ simultaneous file uploads
- **Multiple Ghost Ships**: Unlimited distributed agents
- **Memory Usage**: <2GB per ghost ship under normal load
- **CPU Usage**: <80% under high concurrency scenarios

## Security Architecture

### Authentication & Authorization
- **Token-Based Auth**: Secure API tokens for all communications
- **TLS Encryption**: End-to-end encrypted WebSocket connections
- **Certificate Validation**: Proper CA certificate chain validation
- **Role-Based Access**: Different token types for different access levels

### Network Security
- **WebSocket Security**: WSS (WebSocket Secure) protocol
- **API Security**: HTTPS-only REST API endpoints
- **Network Isolation**: Container-based network isolation
- **Firewall Integration**: Configurable port and protocol restrictions

### Data Security
- **S3 Encryption**: Optional server-side encryption for archived files
- **Credential Management**: Secure AWS credential handling
- **Log Security**: No sensitive data in logs or metrics
- **Audit Trail**: Complete archival operation logging

## Monitoring & Observability

### Metrics Collection
- **Prometheus Integration**: Comprehensive metrics export
- **Custom Metrics**: CargoShip-specific performance indicators
- **System Metrics**: CPU, memory, network, and disk utilization
- **Business Metrics**: Files archived, bytes transferred, success rates

### Key Metrics
```
cargoship_active_jobs                    # Current active archival jobs
cargoship_completed_jobs_total           # Total completed jobs
cargoship_failed_jobs_total              # Total failed jobs  
cargoship_bytes_archived_total           # Total bytes archived
cargoship_average_throughput_mbps        # Average transfer rate
cargoship_ghost_ships_connected          # Connected ghost ships
cargoship_controller_uptime_seconds      # Controller uptime
```

### Logging
- **Structured Logging**: JSON-formatted logs with structured fields
- **Log Levels**: Configurable verbosity (debug, info, warn, error)
- **Distributed Tracing**: Request correlation across components
- **Log Aggregation**: Centralized log collection and analysis

### Health Monitoring
- **Health Endpoints**: HTTP health check endpoints for all services
- **Heartbeat System**: Regular ping/pong between controller and agents
- **Automatic Recovery**: Self-healing connections and operations
- **Alert Integration**: Integration with monitoring systems

## Fault Tolerance

### Connection Resilience  
- **Automatic Reconnection**: Exponential backoff reconnection logic
- **Connection Pooling**: Multiple connection fallbacks
- **Offline Operation**: Ghost ships continue autonomous operation
- **State Synchronization**: Automatic state sync on reconnection

### Error Handling
- **Retry Logic**: Configurable retry attempts with backoff
- **Circuit Breakers**: Automatic failure isolation
- **Graceful Degradation**: Reduced functionality under failure conditions
- **Error Propagation**: Proper error reporting and handling

### Data Integrity
- **Checksum Validation**: SHA-256 verification of uploaded files
- **Atomic Operations**: All-or-nothing archival operations
- **Transaction Logging**: Complete operation audit trail
- **Recovery Procedures**: Automatic recovery from partial failures

## Use Case Scenarios

### 1. Autonomous Document Archival
- **Scenario**: Legal office with compliance requirements
- **Configuration**: Documents older than 24 hours → STANDARD_IA storage
- **Benefits**: Automatic compliance, cost optimization, no manual intervention

### 2. Media Asset Management
- **Scenario**: Photography studio with large RAW files
- **Configuration**: RAW files older than 7 days → GLACIER storage
- **Benefits**: Long-term preservation, significant cost savings, automated workflow

### 3. Backup Lifecycle Management
- **Scenario**: IT department with backup retention policies
- **Configuration**: Backups older than 30 days → DEEP_ARCHIVE, local deletion
- **Benefits**: Policy enforcement, storage cost optimization, automated cleanup

### 4. Multi-Location Coordination
- **Scenario**: Company with multiple office locations
- **Configuration**: Central controller coordinating ghost ships at each location
- **Benefits**: Centralized monitoring, consistent policies, unified reporting

## Deployment Scenarios

### Development Environment
```bash
# Start local development stack
cd docker/development
docker-compose up -d

# Run comprehensive tests
./scripts/test-suite.sh --generate-data all
```

### Production Deployment
```bash
# Deploy to astrapi.local QNAP NAS
./scripts/launch-ghost-ship.sh \
  --target astrapi.local \
  --config configs/astrapi/ghost-ship-production.yaml \
  launch

# Monitor operations
./scripts/launch-ghost-ship.sh --target astrapi.local status
```

### Scaling Deployment
```bash
# Deploy to multiple NAS devices
for nas in nas1.local nas2.local nas3.local; do
  ./scripts/launch-ghost-ship.sh \
    --target "$nas" \
    --id "ghost-${nas%%.*}" \
    launch
done
```

## Future Enhancements

### Planned Features
- **Web Dashboard**: Browser-based management interface
- **Advanced Scheduling**: Cron-based archival scheduling
- **Content-Aware Rules**: File content analysis for intelligent classification
- **Multi-Cloud Support**: Azure Blob Storage and Google Cloud Storage
- **Bandwidth Throttling**: Configurable bandwidth limits and QoS
- **Compression Optimization**: Intelligent compression based on file type

### API Extensions
- **GraphQL API**: More flexible query capabilities
- **Batch Operations**: Bulk job management and status queries
- **Webhook Integration**: Event-driven notifications and integrations
- **Plugin System**: Extensible rule engine and transport plugins

This architecture enables truly distributed, autonomous archival operations while maintaining central visibility and control - embodying the CargoShip "launch" and "ghost ship" concept for modern cloud storage workflows.