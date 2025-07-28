# CargoShip Launch & Ghost Ship Architecture

This example demonstrates the complete CargoShip "launch" and "ghost ship" architecture for autonomous distributed archival operations.

## Architecture Overview

```
┌─────────────────────┐    WebSocket/TLS    ┌──────────────────────┐
│   Local CargoShip   │◄──────────────────►│  Central Controller  │
│    Controller       │     Coordination    │     (Hub)           │
└─────────────────────┘                     └──────────────────────┘
           │                                           │
           │ Monitors & Controls                       │ Coordinates
           │                                           │
    ┌──────▼──────┐                            ┌──────▼──────┐
    │ Ghost Ship  │                            │ Ghost Ship  │
    │ (astrapi)   │                            │ (other NAS) │
    └─────────────┘                            └─────────────┘
          │                                            │
          │ Autonomous Archival                        │ Autonomous Archival
          │                                            │
    ┌─────▼─────┐                                ┌─────▼─────┐
    │ Local     │────────── S3 ─────────────────│ Local     │
    │ Files     │        Archive                │ Files     │
    └───────────┘                               └───────────┘
```

## Components

### 1. Central Controller (`central_controller.go`)
- **Purpose**: Coordinates multiple distributed ghost ships
- **Features**: 
  - WebSocket-based real-time communication
  - Agent registration and health monitoring
  - Job assignment and status tracking
  - RESTful API for management
  - TLS security and authentication

### 2. Ghost Ship (`ghost_ship.go`)
- **Purpose**: Autonomous archival agents running on remote NAS systems
- **Features**:
  - Rule-based file discovery and archival
  - S3 optimization integration (BBR/CUBIC)
  - Autonomous operation with controller reporting
  - File watching and pattern matching
  - Configurable storage classes and retention

### 3. Launch Agent (`agent.go`)
- **Purpose**: Manages ghost ship deployment and communication
- **Features**:
  - Controller connection management
  - Health monitoring and reporting
  - Job processing coordination
  - Secure WebSocket communication

## Usage Example

### 1. Start Central Controller

```bash
# Create controller configuration
cp examples/launch/central_controller_config.yaml /etc/cargoship/controller.yaml

# Set authentication tokens
export CARGOSHIP_CONTROLLER_TOKEN="your-secure-token"
export CARGOSHIP_ADMIN_TOKEN="your-admin-token"

# Start central controller
cargoship controller start --config /etc/cargoship/controller.yaml
```

### 2. Deploy Ghost Ship to astrapi NAS

```bash 
# Use the astrapi deployment script
./scripts/deploy-astrapi.sh deploy

# Or manually deploy ghost ship
docker run -d \
  --name cargoship-ghost-ship \
  --network host \
  -v /volume1/Public:/data/public:ro \
  -v /volume1/homes/.aws:/root/.aws:ro \
  -v /etc/cargoship:/etc/cargoship:ro \
  -e CARGOSHIP_AUTH_TOKEN="${CARGOSHIP_AUTH_TOKEN}" \
  cargoship:astrapi-latest \
  ghost-ship --config /etc/cargoship/ghost_ship_config.yaml
```

### 3. Monitor Operations

```bash
# Check controller status
curl -H "Authorization: Bearer ${CARGOSHIP_CONTROLLER_TOKEN}" \
  https://cargoship-controller.local:8080/api/v1/status

# List connected ghost ships
curl -H "Authorization: Bearer ${CARGOSHIP_CONTROLLER_TOKEN}" \
  https://cargoship-controller.local:8080/api/v1/ghostships

# View ghost ship jobs
curl -H "Authorization: Bearer ${CARGOSHIP_CONTROLLER_TOKEN}" \
  https://cargoship-controller.local:8080/api/v1/ghostships/astrapi-ghost-01/jobs
```

## Configuration

### Ghost Ship Configuration (`ghost_ship_config.yaml`)
- **Watch Paths**: Directories to monitor for archival
- **Archival Rules**: Conditions and actions for automatic archival
- **S3 Configuration**: Bucket, region, optimization settings
- **Controller Integration**: Connection to central controller
- **Performance Settings**: Concurrency, scan intervals

### Central Controller Configuration (`central_controller_config.yaml`)
- **Server Settings**: Listen address, port, TLS configuration
- **Authentication**: Token-based authentication
- **Agent Management**: Timeouts, ping intervals
- **Monitoring**: Health checks, metrics

## Key Features

### 1. True Launch Capability
- **Remote Deployment**: Deploy ghost ships to remote NAS systems
- **Autonomous Operation**: Ghost ships operate independently 
- **Central Coordination**: Controller monitors and manages distributed agents
- **Direct Archival**: Files archived directly from NAS to S3 (no round-trip)

### 2. Autonomous Ghost Ship Archival
- **Rule-Based Processing**: Configurable archival rules
- **File Discovery**: Automatic scanning and pattern matching
- **S3 Optimization**: BBR/CUBIC congestion control, predictive bandwidth
- **Storage Classes**: Intelligent storage class selection (Standard, IA, Glacier, Deep Archive)

### 3. Distributed Coordination
- **WebSocket Communication**: Real-time bidirectional communication
- **Health Monitoring**: Continuous agent health and status tracking
- **Job Management**: Central job assignment and progress tracking
- **Security**: TLS encryption and token authentication

### 4. Performance Optimization
- **High Bandwidth Utilization**: 4.6x performance improvements over standard
- **Concurrent Operations**: Multiple simultaneous archival jobs
- **Network Adaptation**: Real-time network condition adaptation
- **Resource Management**: Configurable worker pools and concurrency limits

## Real-World Benefits

1. **No Data Round-Trip**: Files archived directly from NAS to cloud storage
2. **Autonomous Operation**: Continues working even if controller is offline
3. **Scalable Architecture**: Add ghost ships to any number of NAS systems
4. **Intelligent Archival**: Rule-based policies for different file types
5. **Cost Optimization**: Automatic storage class selection based on access patterns
6. **Performance**: Utilizes high-bandwidth connections efficiently (10Gbps local, 5Gbps internet)

This architecture enables truly distributed, autonomous archival operations while maintaining central visibility and control - the essence of the CargoShip "launch" and "ghost ship" concept.