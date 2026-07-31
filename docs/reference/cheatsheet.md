# Cheat sheet

A one-page crib of the commands you'll reach for most. Every command links out to
a fuller guide from the [command reference](/reference/).

## Upload

```bash
# Basic upload
cargoship upload ./my-data s3://my-bucket/archives/

# Pick a storage class
cargoship upload ./my-data s3://my-bucket/archives/ --storage-class INTELLIGENT_TIERING

# Max compression, tag to a project for cost tracking
cargoship upload ./my-data s3://my-bucket/archives/ --compression-level 19 --project genomics-2026

# Quiet mode for scripts/cron
cargoship upload ./my-data s3://my-bucket/archives/ --quiet
```

## Estimate

```bash
cargoship estimate ./my-data
cargoship estimate ./my-data --storage-class GLACIER_IR --show-comparison
```

## Incremental sync

```bash
cargoship sync ./my-data s3://my-bucket/backups/
cargoship sync ./my-data s3://my-bucket/backups/ --dry-run
cargoship sync ./my-data s3://my-bucket/backups/ --checksum --track-deletes
```

## Inspect

```bash
cargoship info   s3://my-bucket/archives/uploads/20260721-a1b2c3
cargoship list   --bucket my-bucket --upload-id 20260721-a1b2c3 --pattern "*.csv"
cargoship balance s3://my-bucket/archives/uploads/20260721-a1b2c3
```

## Verify

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --quick
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --deep   # re-download & re-hash stored bytes
# exit 0 = passed, 1 = failed
```

## Download & restore

```bash
# Download from a readable storage class
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out --pattern "*.log"

# Restore specific files (handles Glacier; verifies each file as it writes)
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out --file data/train.csv

# Flatten restored files into ./out by basename (no directory tree)
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out --file data/train.csv --flatten

# Restore from Glacier, wait for thaw, cap the cost
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out \
  --dvc-stage train --tier standard --wait --max-restore-cost 5.00

# Browse interactively before restoring
cargoship browse s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out
cargoship shell  s3://my-bucket/archives/uploads/20260721-a1b2c3
```

## Resume

```bash
cargoship resume list
cargoship resume 20260721-a1b2c3
cargoship upload ./my-data s3://my-bucket/archives/ --force-restart
```

## Budget & cost

```bash
cargoship budget set --project genomics-2026 --max-budget 1000 --max-volume-gb 500
cargoship budget status
cargoship cost projects
cargoship cost forecast --model ensemble
cargoship analyze s3://my-bucket
```

## See also

- [Command reference overview](/reference/).
- [Uploading data](/guides/uploading).
- [Glossary](/reference/glossary).
