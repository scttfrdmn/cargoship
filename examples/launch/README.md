# CargoShip Ghost Ship Architecture

This example demonstrates a **ghost ship**: an autonomous CargoShip agent deployed
to a remote NAS that archives local files straight to S3, with no data round-trip
through your workstation.

> **Central coordination was removed in v0.20.0.** Earlier versions also shipped a
> central controller (`cargoship controller`), a `webui` dashboard, and a
> `cargoship-launch` agent binary. That subsystem was unfinished scaffolding, and a
> security audit found an authentication bypass in it, so it was removed rather
> than hardened — see
> [issue #340](https://github.com/scttfrdmn/cargoship/issues/340). Ghost ships are
> unaffected: they were built to archive autonomously and never required a
> controller to function.

## Architecture Overview

```
    ┌─────────────┐                            ┌─────────────┐
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

Each ghost ship runs independently. There is no hub, and **nothing listens for
inbound connections at all** — `ghost-ship` binds no port.

## Components

### Ghost Ship (`pkg/launch/ghost_ship.go`)
- **Purpose**: Autonomous archival agent running on a remote NAS
- **Features**:
  - Rule-based file discovery and archival
  - S3 optimization integration (BBR/CUBIC)
  - Periodic directory scanning and pattern matching
  - Configurable storage classes and retention

A ghost ship discovers files by **polling** each watch path on `scan_interval`,
not by subscribing to filesystem events. The separate "launch agent"
(`pkg/launch/agent.go`) documented here previously was a parallel, unreachable
implementation of the same job and was removed in
[#347](https://github.com/scttfrdmn/cargoship/issues/347); `ghost-ship` never
used it.

## Usage Example

### Deploy a Ghost Ship to the astrapi NAS

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
  cargoship:astrapi-latest \
  ghost-ship --config /etc/cargoship/ghost_ship_config.yaml
```

### Monitor Operations

A ghost ship reports status by **logging** it on `report_interval`; it serves no
metrics or health endpoint. An earlier version of this page showed a
`curl .../metrics` command for a port that has never been bound — see
[#348](https://github.com/scttfrdmn/cargoship/issues/348), which tracks the same
claim still present in the Docker development environment.

```bash
docker logs cargoship-ghost-ship
```

Uploads land in your bucket like any other CargoShip run, so the ordinary
commands work against them:

```bash
cargoship list s3://your-bucket/prefix
cargoship verify s3://your-bucket/prefix/<upload-id> --deep
```

## Configuration

### Ghost Ship Configuration (`ghost_ship_config.yaml`)
- **Watch Paths**: Directories to monitor for archival
- **Archival Rules**: Conditions and actions for automatic archival
- **S3 Configuration**: Bucket, region, optimization settings
- **Performance Settings**: Concurrency, scan intervals

## Key Features

### 1. Autonomous Ghost Ship Archival
- **Rule-Based Processing**: Configurable archival rules
- **File Discovery**: Automatic scanning and pattern matching
- **S3 Optimization**: BBR/CUBIC congestion control, predictive bandwidth
- **Storage Classes**: Intelligent storage class selection (Standard, IA, Glacier, Deep Archive)

### 2. Performance Optimization
- **Concurrent Operations**: Multiple simultaneous archival jobs
- **Network Adaptation**: Real-time network condition adaptation
- **Resource Management**: Configurable worker pools and concurrency limits

## Real-World Benefits

1. **No Data Round-Trip**: Files archived directly from NAS to cloud storage
2. **No Coordination Dependency**: Each agent works on its own, with nothing to keep running alongside it
3. **Scalable Architecture**: Add ghost ships to any number of NAS systems
4. **Intelligent Archival**: Rule-based policies for different file types
5. **Cost Optimization**: Automatic storage class selection based on access patterns
6. **Performance**: Utilizes high-bandwidth connections efficiently (10Gbps local, 5Gbps internet)
