# Listing & inspecting uploads

CargoShip can tell you everything about an upload without downloading a single
chunk — it reads the lightweight manifest (~30 KB) instead. Three commands cover
inspection: `cargoship info` prints upload-level metadata and statistics,
`cargoship list` enumerates the files inside an upload, and `cargoship balance`
analyzes how evenly chunks are spread across shards. Full flag lists live in the
[command reference](/reference/commands/inspect).

```bash
cargoship info   s3://my-bucket/archives/uploads/20260721-a1b2c3
cargoship list   --bucket my-bucket --upload-id 20260721-a1b2c3
cargoship balance s3://my-bucket/archives/uploads/20260721-a1b2c3
```

## info — upload metadata & statistics

`cargoship info` downloads the manifest and prints identification, storage
location, dataset statistics, compression settings, and shard distribution:

```bash
cargoship info s3://my-bucket/archives/uploads/20260721-a1b2c3
```

```
📦 Upload Information
   Upload ID:        20260721-a1b2c3
   Created:          2026-07-21 09:14:03 (6h ago)
   Source:           /data/genomics
   Manifest Version: 2

📊 Dataset Statistics
   Total Files:      9,914 files
   Total Size:       48.2 GB (uncompressed)
   Compressed Size:  17.9 GB (37.1% of original)
   Space Saved:      30.3 GB (62.9% reduction)

🎯 Shard Distribution
   Total Shards:     10
   Total Chunks:     186 (18.6 chunks/shard avg)
```

You can address the upload with an S3 URL (shown above) or with flags for
backward compatibility:

```bash
cargoship info --bucket my-bucket --prefix archives --upload-id 20260721-a1b2c3
```

- `--verbose` adds per-shard statistics (chunk counts, per-shard sizes,
  compression) — handy before a `--shard-ids` download.
- `--json` emits the manifest as JSON for scripting.
- `-r`/`--region` selects the region (default `us-west-2`).

## list — enumerate files

`cargoship list` shows the files in an upload. It requires `--bucket`/`-b` and
`--upload-id`/`-u`, with optional `--prefix`/`-p` for the S3 prefix:

```bash
# Everything in the upload
cargoship list --bucket my-bucket --upload-id 20260721-a1b2c3

# Only files matching a glob
cargoship list --bucket my-bucket --upload-id 20260721-a1b2c3 --pattern "*.csv"

# Full detail (chunk & shard placement)
cargoship list --bucket my-bucket --upload-id 20260721-a1b2c3 --verbose
```

`--pattern` filters by glob; `--verbose` adds chunk and shard placement for each
file, which pairs well with selective [downloads](/guides/downloading).

## balance — shard distribution

Adaptive sharding usually spreads data evenly, but skewed file-size
distributions can leave one shard much larger than the rest. `cargoship balance`
reports the imbalance ratio (largest shard ÷ average). An upload is flagged
imbalanced when that ratio exceeds the `--threshold` (default `2.0`).

```bash
# Read-only analysis
cargoship balance s3://my-bucket/archives/uploads/20260721-a1b2c3
```

```
✓ Manifest loaded: 9,914 files across 10 shards

📊 Initial Balance:
   Imbalance ratio: 1.34x (largest/avg)
   💡 Shards are well balanced. No rebalancing needed.
```

To plan a redistribution without touching data, use `--dry-run`; to actually
rewrite chunk placement, use `--execute`:

```bash
# Show the rebalancing plan only
cargoship balance s3://my-bucket/.../20260721-a1b2c3 --dry-run

# Use a looser threshold (3x instead of 2x)
cargoship balance s3://my-bucket/.../20260721-a1b2c3 --threshold 3.0

# Apply the rebalance (modifies data)
cargoship balance s3://my-bucket/.../20260721-a1b2c3 --execute
```

::: warning --execute modifies your archive
`--execute` moves files between shards and rewrites the manifest. Run with
`--dry-run` first to review the plan (files moved, bytes, chunks affected).
`balance` also supports `--format json` for scripting.
:::

## Best practices

::: tip
- **Start with `info`** to confirm an upload completed and to see its size and
  compression before planning a download or restore.
- **Use `--json`** (info) or `--format json` (balance) to feed dashboards and CI.
- **`list --verbose`** reveals which shard holds a file, so you can pull just
  that shard with `download --shard-ids`.
- **Leave sharding on auto** — only reach for `balance --execute` when `info`
  shows a genuinely lopsided distribution.
:::

## See also

- [Verifying integrity](/guides/verifying) — check an upload against its checksums.
- [Downloading & extracting](/guides/downloading) — get files back out.
- [Concepts: manifest](/intro/concepts#manifest) — what these commands read.
- Reference: [Inspection & retrieval](/reference/commands/inspect).
