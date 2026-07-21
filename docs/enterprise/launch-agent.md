# Launch agents

Launch agents are lightweight, containerized CargoShip agents that run on your lab
infrastructure — NAS boxes, file servers, compute nodes — to automatically detect
and archive completed research datasets. They operate headlessly, connect securely
to a [controller](/enterprise/controller) over a TLS-encrypted WebSocket, and keep
working in the background without interrupting research workflows.

## Deploying on a NAS with Docker Compose

Download the deployment files and provide your configuration via a `.env` file:

```bash
curl -O https://raw.githubusercontent.com/scttfrdmn/cargoship/main/docker/launch/docker-compose.yml
curl -O https://raw.githubusercontent.com/scttfrdmn/cargoship/main/docker/launch/agent.yaml
```

```bash
# .env
CARGOSHIP_CONTROLLER_URL=wss://your-cargoship-instance.com
CARGOSHIP_AUTH_TOKEN=your-secure-auth-token
CARGOSHIP_DESTINATION=s3://research-archive
CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
DATA_PATH=/volume1/research-data
AWS_CREDENTIALS_PATH=/volume1/docker/cargoship/.aws
```

```bash
docker-compose up -d
docker-compose logs -f cargoship-agent
```

## Configuration

### Required environment variables

| Variable | Description |
|----------|-------------|
| `CARGOSHIP_CONTROLLER_URL` | WebSocket URL of your controller (e.g. `wss://cargoship.lab.edu`). |
| `CARGOSHIP_AUTH_TOKEN` | Authentication token for the secure connection. |
| `CARGOSHIP_DESTINATION` | S3 destination bucket/prefix (e.g. `s3://research-archive`). |

### Common optional variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CARGOSHIP_WATCH_PATHS` | Comma-separated paths to monitor | `/data` |
| `CARGOSHIP_STORAGE_CLASS` | Default storage class | `deep-archive` |
| `CARGOSHIP_MIN_AGE_DAYS` | Minimum file age before archival | `7` |
| `CARGOSHIP_PATTERNS` | File patterns to include | `*` |
| `CARGOSHIP_EXCLUDE_PATTERNS` | File patterns to exclude | `*.tmp,*.lock,.DS_Store` |
| `CARGOSHIP_CHECK_INTERVAL` | Scan interval (seconds) | `300` |
| `CARGOSHIP_LOG_LEVEL` | Log verbosity (debug, info, warn, error) | `info` |

### Config file

For richer setups, mount a config file to `/config/agent.yaml` with per-path watch
rules — each path can have its own include/exclude patterns, minimum age, and
storage class:

```yaml
name: "CargoShip NAS Agent"
tls_config:
  enabled: true
  insecure_skip_verify: false   # true only for self-signed certs

watch_paths:
  - path: "/data/genomics"
    include_patterns: ["*.fastq.gz", "*.bam", "*.vcf.gz"]
    exclude_patterns: ["*.tmp", "*.lock"]
    min_age: "168h"             # 7 days
    storage_class: "deep-archive"
    recursive: true

scan_interval: "300s"
archive:
  compression: "zstd"
  encryption: true
  max_concurrent: 2

health_check:
  enabled: true
  metrics_enabled: true
```

## Deployment targets

Agents run anywhere Docker does. Common patterns:

- **Synology / QNAP NAS** via Container Manager / Container Station — see
  [QNAP / NAS deployment](/enterprise/qnap) and [ghost-ship](/enterprise/ghost-ship).
- **Linux server** via a `systemd` unit that runs the container with
  `--restart unless-stopped`.

## Monitoring

```bash
docker logs -f cargoship-agent
docker exec cargoship-agent cargoship-launch -validate   # validate config
docker logs cargoship-agent 2>&1 | grep -i "controller\|websocket"  # connection
```

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [Controller](/enterprise/controller).
- [ghost-ship](/enterprise/ghost-ship) · [QNAP / NAS deployment](/enterprise/qnap).
- [Environment variables](/reference/environment-variables).
