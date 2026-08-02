---
prev:
  text: AWS setup & credentials
  link: /start/aws-setup
next:
  text: Verify & restore it
  link: /start/verify-and-restore
---

# Your first upload

The [Quick Start](/start/quickstart) got you a working upload. This page explains
what actually happened, so you can adjust it for real datasets.

## The command

```bash
cargoship upload ./my-data s3://my-bucket/archives/
```

Two positional arguments:

- **`./my-data`** — the source directory to archive (walked recursively).
- **`s3://my-bucket/archives/`** — the destination bucket and key prefix. You can
  also give `--bucket my-bucket` and let the prefix default.

## What CargoShip does

1. **Scans** `./my-data`, recording each file's path, size, and modification time.
2. **Chunks** the files into archive-sized groups and **shards** them across S3
   prefixes for parallel upload.
3. **Compresses** each chunk with Zstandard, choosing a level suited to the
   content (already-compressed data is stored without re-compression).
4. **Uploads** chunks concurrently and writes a **manifest** describing where
   everything went.
5. Prints an **upload ID** like `20260721-a1b2c3`. Save it — that's how you refer
   to this upload from now on.

Nothing is written to local disk in between; data streams straight to S3.

## Useful flags for a first real run

| Flag | Why you'd use it |
|------|------------------|
| `--region us-east-1` | Match your bucket's region (or set `AWS_REGION`). |
| `--storage-class INTELLIGENT_TIERING` | Let S3 tier cold data automatically. |
| `--compression-level 9` | Pin every chunk to one level, replacing [content-aware selection](/guides/features/compression). |
| `--shard-count 16` | Override the [adaptive shard count](/guides/features/sharding). |
| `--project my-study` | Tag spend for [budgets & cost reports](/guides/cost/budgets). |
| `--quiet` | Suppress the progress display (good for scripts/logs). |

The full flag list is in the [upload command reference](/reference/commands/upload).

## Preview cost first (optional)

```bash
cargoship estimate ./my-data --show-comparison
```

This shows projected storage/request cost and how CargoShip's chunking compares to
a naive per-file upload — without touching S3.

## Which upload command?

There are two upload paths. Use `cargoship upload` (this one) — it's the canonical
command with the richest feature set (tiering, dedup, encryption, incremental,
DVC). The streaming-pipeline variant `cargoship create upload` exists for specific
tuning needs; see [upload vs. create upload](/guides/upload-vs-create-upload) for
when to pick which.

## Next

- [Verify & restore it](/start/verify-and-restore) — prove the round trip.
- [Uploading data](/guides/uploading) — the full guide, including sharding,
  tiering, dedup, and incremental sync.
