# Uploading data

`cargoship upload` is the canonical way to archive a directory to S3. This guide
covers the options you'll actually reach for; the exhaustive flag list lives in
the [command reference](/reference/commands/upload).

```bash
cargoship upload SOURCE_DIR s3://BUCKET/PREFIX/
```

## Basic upload

```bash
cargoship upload ./my-data s3://my-bucket/archives/
```

CargoShip scans, chunks, shards, compresses, and uploads in parallel, then prints
an [upload ID](/intro/concepts#upload-id). See
[How it works](/intro/how-it-works) for the pipeline, or
[Your first upload](/start/first-upload) for a gentle walkthrough.

## Choosing a storage class

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --storage-class INTELLIGENT_TIERING
```

Common choices: `STANDARD` (default), `INTELLIGENT_TIERING` (S3 auto-tiers cold
data), `GLACIER` / `GLACIER_IR`, `DEEP_ARCHIVE` (cheapest storage, slowest and
priciest retrieval). To assign classes **per chunk by file age**, use tier-aware
storage:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --auto-tier --tier-strategy tier-aware
```

::: warning Cost implication
`--tier-strategy tier-aware` prompts for confirmation because it changes retrieval
characteristics. Add `--yes` to accept in automation, and `--tier-max GLACIER` to
cap how cold anything goes. See [Tier-aware storage](/guides/features/tiering).
:::

## Sharding & compression

The [shard count is adaptive](/guides/features/sharding) by default (4–32). Override it,
or pick a distribution strategy:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --shard-count 16 --shard-strategy size \
  --compression-level 9
```

- `--shard-strategy` — `hash` (balanced, default), `size`, `type`, `directory`.
- `--compression-level` — Zstandard 1–22 (higher = smaller + more CPU).
  CargoShip still skips re-compressing already-compressed content. See
  [Compression](/guides/features/compression).

## Deduplication

Skip storing identical files more than once:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --enable-dedup
```

Worthwhile for datasets with redundant files; adds a hashing pass.

## Encryption

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --kms-key-id alias/my-key --encrypt-manifest
```

Data chunks are written with SSE-KMS; `--encrypt-manifest` additionally
envelope-encrypts the manifest. See [Encryption](/guides/features/encryption).

## Integrity checksums

Every upload records a SHA-256 checksum at two levels: per stored chunk, and
**per file**. These are what let [`verify --deep`](/guides/verifying) confirm the
stored bytes still match, and what [restore](/guides/restoring) checks as it
writes files back. Per-file checksums are **on by default**.

```bash
# Faster uploads, but verify --deep can no longer confirm per-file integrity
cargoship upload ./my-data s3://my-bucket/archives/ --no-file-checksums
```

Only use `--no-file-checksums` when upload speed matters more than per-file
verifiability; chunk-level checksums are still recorded either way. See the
[Integrity model](/project/integrity) for what the checksums guarantee.

## Incremental sync

Upload only what's new or changed since a previous run:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --incremental --prev-manifest ./manifest.json.gz
```

For a dedicated sync workflow (including delete tracking), see
[Incremental sync](/guides/sync).

## Cost tracking

Tag an upload to a project so its spend rolls up in reports and budgets:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --project genomics-2026
```

Then see [Budgets & quotas](/guides/cost/budgets) and
[Cost management](/guides/cost/management).

## Best practices

::: tip
- **Estimate first** for large datasets: `cargoship estimate ./my-data --show-comparison`.
- **Set a region default** (`AWS_REGION`) or pass `--region` to match your bucket.
- **Leave shard count on auto** unless benchmarking — it's tuned to your workload.
- **Turn on `--enable-dedup`** only when you expect duplicate files; it costs a hash pass.
- **Keep per-file checksums on** (the default) so `verify --deep` and restore can
  confirm per-file integrity; only add `--no-file-checksums` when speed wins.
- **Tag `--project`** from day one so cost reporting is meaningful later.
- **Use `--quiet`** in scripts and cron; keep the progress UI for interactive runs.
:::

## See also

- [upload vs. create upload](/guides/upload-vs-create-upload) — which command to use.
- [Resuming interrupted uploads](/guides/resuming).
- Reference: [`upload`](/reference/commands/upload).
