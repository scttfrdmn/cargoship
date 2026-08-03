# ghost-ship

A ghost ship is an autonomous CargoShip agent deployed to a remote NAS (QNAP or
Synology) that continuously monitors local files and archives them directly to S3
— no data round-trip through a central host. It runs fully independently: no
central coordinator is involved, and archival continues on its own schedule.

## How it works

Ghost ships apply configurable **archival rules** to watched paths: match files by
pattern and minimum age, pick a storage class, optionally encrypt, and (optionally)
delete after a successful archive to free local space. Files go straight from the
NAS to S3.

```yaml
# ghost_ship.yaml (excerpt)
id: "production-ghost-ship"
name: "Production Ghost Ship"

s3_config:
  bucket: "cargoship-production"
  region: "us-west-2"          # must match the bucket's region
  concurrency: 20

watch_paths:
  - path: "/volume1/research-data"
    include_patterns: ["*.fastq.gz", "*.bam", "*.vcf.gz"]
    exclude_patterns: ["*.tmp", "*.lock"]
    min_age: "1h"
    storage_class: "STANDARD"
    recursive: true

max_concurrent_jobs: 4
scan_interval: "5m"
```

## Deployment

Ghost ships are distributed as platform-specific container images and run via
Container Station (QNAP) or Container Manager (Synology). A typical run mounts the
data directory read-only, the config, and AWS credentials read-only:

```bash
docker run -d \
  --name cargoship-ghost-container \
  --platform linux/amd64 \
  -e AWS_PROFILE=aws \
  -e AWS_DEFAULT_REGION=us-west-2 \
  -v /path/to/.aws:/home/cargoship/.aws:ro \
  -v /path/to/ghost_ship.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v /volume1/research-data:/volume1/research-data:ro \
  --restart unless-stopped \
  cargoship-ghost:latest
```

::: warning Region must match
Set `AWS_DEFAULT_REGION` to the bucket's region explicitly. The AWS SDK prioritizes
the environment variable over the config file, and a mismatch causes S3 301
`PermanentRedirect` errors. QNAP/Synology are typically x86_64 — build/run images
with `--platform linux/amd64` to avoid exec-format errors.
:::

## Security

- Runs as a non-root user; data and AWS credentials are mounted read-only.
- Outbound HTTPS to S3 only; a ghost ship opens no listening port.
- Optional server-side encryption for archived files.

## Monitoring

```bash
docker ps | grep cargoship-ghost
docker logs cargoship-ghost-container --tail 20
```

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [QNAP / NAS deployment](/enterprise/qnap) — step-by-step QNAP guide.
