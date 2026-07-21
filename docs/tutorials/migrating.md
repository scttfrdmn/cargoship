# Migrating from rclone / aws s3 cp

**You are:** already moving data to S3 with `rclone` or `aws s3 cp`, and you want
the CargoShip equivalents plus a clear sense of when CargoShip is the right tool
and when it isn't.

The short version: `rclone` and `aws s3 cp` upload **one object per file**.
CargoShip groups files into compressed, sharded archives and writes a manifest —
far fewer S3 requests, built-in compression, deduplication, and incremental sync.
That's a big win for **archival and backup**, and the wrong trade for **serving
individual files directly from S3**.

## The core difference

| | `rclone` / `aws s3 cp` | CargoShip |
|--|------------------------|-----------|
| Storage layout | one S3 object per file | compressed `tar.zst` chunks + manifest |
| S3 requests | one (or more) per file | a handful per upload |
| Compression | none | Zstandard, content-aware |
| Dedup / incremental | limited / hash-based | content dedup + manifest-based incremental |
| Individual file serving | yes, direct | no — restore first |

`rclone` leaves files individually addressable in S3 (good for static hosting).
CargoShip packs them, so you retrieve via `cargoship restore` rather than a plain
`GET`. Choose based on whether you need direct file access.

## Command mapping

### Basic upload

```bash
# rclone
rclone copy /local/path myremote:bucket/prefix

# aws cli
aws s3 cp /local/path s3://bucket/prefix/ --recursive

# CargoShip
cargoship upload /local/path s3://bucket/prefix/ --region us-west-2
```

### Incremental sync

```bash
# rclone
rclone sync /local/path myremote:bucket/prefix

# CargoShip — upload only new/changed files against a prior manifest
cargoship upload /local/path s3://bucket/prefix/ \
  --incremental --prev-manifest ./manifest.json.gz
```

For a dedicated sync workflow (including delete tracking) see
[Incremental sync](/guides/sync).

### Preview before uploading (dry-run)

```bash
# rclone
rclone copy /local/path myremote:bucket/prefix --dry-run

# CargoShip — size, compression, cost, without touching S3
cargoship estimate /local/path --show-comparison
```

`--show-comparison` contrasts a naive per-file upload against CargoShip's
chunking. See [Estimating costs](/guides/cost/estimate).

### List and inspect

```bash
# rclone
rclone ls myremote:bucket/prefix

# CargoShip — inspect an upload by its ID
cargoship info s3://bucket/prefix/uploads/<id>
cargoship list s3://bucket/prefix/uploads/<id> --pattern '*.log'
```

### Download / restore

```bash
# rclone
rclone copy myremote:bucket/prefix /local/restore-path

# CargoShip — whole upload, or a single file by path
cargoship restore s3://bucket/prefix/uploads/<id> /local/restore-path
cargoship restore s3://bucket/prefix/uploads/<id> /local/restore-path \
  --file path/to/one/file.txt
```

See [Restoring files](/guides/restoring).

## Config and environment

`rclone` keeps remotes in `rclone.conf`. CargoShip uses the standard AWS
credential chain — no remote definitions to port:

```bash
export AWS_REGION=us-west-2
export AWS_PROFILE=default
```

Named profiles are set via `AWS_PROFILE` (or `--region` for the region), not a
CargoShip-specific config. See [AWS setup](/start/aws-setup) and
[Config files & precedence](/guides/config/files).

## Common patterns

### Daily backup script

```bash
# rclone
DATE=$(date +%F)
rclone sync /var/backups myremote:backups/$DATE --transfers 4 --progress

# CargoShip
DATE=$(date +%F)
cargoship upload /var/backups s3://backups/$DATE/ \
  --storage-class GLACIER_IR \
  --quiet
```

`--quiet` suppresses the progress UI for cron/logs; `--storage-class GLACIER_IR`
sends rarely-touched backups to a cheaper class. Fewer requests and built-in
compression are where the cost reduction comes from.

### Large dataset upload

```bash
# rclone
rclone copy ./dataset myremote:ml-datasets/v1 --transfers 16 --multi-thread-streams 4

# CargoShip — sharding is adaptive; override only to benchmark
cargoship upload ./dataset s3://ml-datasets/v1/ --shard-count 16
```

Leave the [shard count](/guides/features/sharding) on auto (it's tuned to your
workload); the flag is there for benchmarking.

### Limiting bandwidth

CargoShip has no built-in `--bwlimit`. Constrain it at the OS level:

```bash
# rclone
rclone copy ./data myremote:bucket/prefix --bwlimit 10M

# CargoShip via trickle (Linux/macOS)
trickle -s -u 10240 cargoship upload ./data s3://bucket/prefix/
```

## When to keep rclone

CargoShip doesn't replace `rclone` for every job. Keep `rclone` (or `aws s3 cp`)
when you need:

- **Direct file serving** — static websites, objects fetched individually by
  other systems. CargoShip packs files into archives, so they aren't directly
  addressable.
- **Non-S3 clouds** — `rclone` speaks 40+ providers; CargoShip targets S3.
- **FUSE mounts** or **bidirectional sync with conflict resolution** — rclone
  features CargoShip doesn't have.

A hybrid setup is common: CargoShip for bulk archives and backups, `rclone` for
web assets.

## Accessing one file without a full restore

The most common post-migration question — "how do I get a single file back out?"
— has a first-class answer:

```bash
cargoship restore s3://bucket/prefix/uploads/<id> ./out --file path/to/file.txt
```

No need to download whole archives or hand-extract `tar.zst` chunks.

## Migration checklist

- [ ] Confirm the use case is archival/backup, not direct file serving.
- [ ] Test on a small directory; compare `cargoship estimate` output to your bill.
- [ ] Verify the round trip: `cargoship verify` then a `cargoship restore`.
- [ ] Replace `rclone`/`aws s3 cp` calls in scripts with `cargoship upload`.
- [ ] Add `--project` tags if you want [cost reporting](/guides/cost/management).

## Next steps

- [Uploading data](/guides/uploading) · [Incremental sync](/guides/sync) · [Restoring files](/guides/restoring).
- [Estimating costs](/guides/cost/estimate) · [Sharding](/guides/features/sharding).
- [`upload` reference](/reference/commands/upload).
