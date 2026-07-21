# QNAP / NAS deployment

This guide walks through deploying a CargoShip [ghost ship](/enterprise/ghost-ship)
on a QNAP NAS running Container Station. The same pattern applies to Synology
Container Manager with paths adjusted for its volume layout.

## Prerequisites

- QNAP NAS with Container Station installed and SSH access enabled.
- AWS credentials configured on the NAS (`~/.aws/credentials`, `~/.aws/config`).
- A target S3 bucket that already exists.
- Docker on a build machine for producing the image.

::: warning Architecture
QNAP systems are typically x86_64. Always build images with
`--platform linux/amd64` or you'll get exec-format errors. The Container Station
Docker binary lives at
`/share/ZFS530_DATA/.qpkg/container-station/bin/docker`.
:::

## Steps

### 1. Build and save the image

```bash
docker buildx build --platform linux/amd64 -f docker/Dockerfile.astrapi -t cargoship:nas-amd64 .
docker save cargoship:nas-amd64 > /tmp/cargoship-nas-amd64.tar
```

### 2. Transfer files to the NAS

```bash
ssh user@nas.local "mkdir -p ~/cargoship"
scp /tmp/cargoship-nas-amd64.tar user@nas.local:~/cargoship/
scp docker/nas-config.yaml user@nas.local:~/cargoship/
```

### 3. Load the image on the NAS

```bash
ssh user@nas.local "cd ~/cargoship && /share/ZFS530_DATA/.qpkg/container-station/bin/docker load < cargoship-nas-amd64.tar"
```

### 4. Deploy the container

```bash
ssh user@nas.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker run -d \
  --name cargoship-ghost \
  --network host \
  -e AWS_PROFILE=aws \
  -e AWS_DEFAULT_REGION=us-west-2 \
  -v /volume1/research-data:/volume1/research-data:ro \
  -v ~/cargoship/nas-config.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v ~/.aws:/root/.aws:ro \
  --restart unless-stopped \
  cargoship:nas-amd64"
```

### 5. Verify

```bash
ssh user@nas.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker ps | grep cargoship"
ssh user@nas.local "/share/ZFS530_DATA/.qpkg/container-station/bin/docker logs -f cargoship-ghost"
```

## Troubleshooting

| Symptom | Cause & fix |
|---------|-------------|
| `exec format error` | Wrong architecture — rebuild with `--platform linux/amd64`. |
| S3 301 `PermanentRedirect` | Region mismatch — set `AWS_DEFAULT_REGION` to the bucket's region. |
| `docker: command not found` | Use the full Container Station path shown above. |
| Permission denied on `.aws` | Fix credential file perms: `chmod 600 ~/.aws/credentials`. |
| Volume mount failures | Ensure the host directories exist and are accessible. |

## Security notes

- Container runs as a non-root user; data and AWS credentials are mounted read-only.
- Host network mode is used for throughput; restart is automatic unless stopped.

## See also

- [ghost-ship](/enterprise/ghost-ship).
- [Launch agents](/enterprise/launch-agent).
- [Deployment guide](/enterprise/deployment).
