# Restoring files

`cargoship restore` gets targeted files back out of an archive without
downloading the whole dataset, and — unlike [`download`](/guides/downloading) —
it handles Glacier / Deep Archive retrieval. You select what to restore by
content hash, exact path, Git commit, or DVC pipeline stage, and CargoShip
downloads only the chunks that contain those files. The full flag list lives in
the [command reference](/reference/commands/inspect).

```bash
cargoship restore S3_URL OUTPUT_DIR [selection flags]
```

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out \
  --file data/train.csv --file models/model.pkl
```

## Selecting what to restore

Pick one selection mode, or combine `--file` with the others:

- `--hash` — restore a single file by its MD5 content hash.
- `--file` — one or more exact file paths (repeatable).
- `--git-commit` — all files associated with a specific Git commit SHA.
- `--dvc-stage` — all files produced by a named DVC pipeline stage.

```bash
# By MD5 content hash
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out \
  --hash d8e8fca2dc0f896fd7cb4cb0031ba249

# Everything a DVC stage produced
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out \
  --dvc-stage preprocess

# Everything tied to a Git commit
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out \
  --git-commit 4fa59ee
```

The `--git-commit` and `--dvc-stage` modes read the provenance metadata CargoShip
records at upload time, so you can reconstruct exactly the inputs or outputs of a
pipeline run. See [DVC integration](/reference/commands/dvc) for how stages are
tracked.

## Integrity: verified as it restores

Restore verifies content **as it writes**. Each file's SHA-256 is recomputed
from the bytes coming out of the archive and compared against the checksum
recorded at upload time; on a mismatch CargoShip fails that file rather than
writing corrupt data to disk. This is on by default and covers both direct and
chunked storage.

- `--no-verify` skips restore-time verification (faster, but a corrupted stored
  object would be written out silently). Leave it on unless you have a specific
  reason not to.

For the guarantee this backs and how to audit it yourself, see the
[Integrity model](/project/integrity).

## Output layout

By default, restored files are written under `OUTPUT_DIR` with their paths
reconstructed **relative to the upload root** — so a file uploaded from
`project/data/train.csv` lands at `OUTPUT_DIR/data/train.csv`. Paths are always
sanitized and confined to `OUTPUT_DIR` (absolute paths and `..` components can't
escape it).

- `--flatten` writes each restored file by **basename** directly into
  `OUTPUT_DIR`, ignoring its directory structure — convenient for pulling a
  handful of specific files without recreating deep trees. Basenames must be
  unique across your selection to avoid collisions.

```bash
# Recreate the directory structure (default)
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out --file data/train.csv

# Drop the file straight into ./out as train.csv
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out --file data/train.csv --flatten
```

## Glacier & Deep Archive {#glacier}

When the chunks you want live in Glacier Flexible Retrieval or Deep Archive, they
must be **thawed** before download. `cargoship restore` requests that retrieval
for you and reports the estimated cost:

- `--tier` — retrieval speed: `expedited` (1–5 min), `standard` (3–5 h,
  default), `bulk` (5–12 h). Faster tiers cost more.
- `--wait` — block until the thaw finishes, then download automatically.
- `--restore-days` — how many days to keep the restored copy available (default 7).
- `--max-restore-cost` — abort if the estimated retrieval cost exceeds this USD limit.
- `--dry-run` — show the size and estimated cost without restoring.

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out \
  --dvc-stage train --tier standard --wait --max-restore-cost 5.00
```

```
Resolving --dvc-stage train against manifest...
  matched 12 files in 4 chunks (312 MB, Deep Archive)
Estimated retrieval cost: $0.62  (tier: standard, under --max-restore-cost 5.00)
Requesting Glacier restore for 4 chunks...
Waiting for restore to complete (tier: standard, ~3-5h)...
```

Always check the price first with `--dry-run` on Deep Archive data — bulk is the
cheapest tier, expedited the most expensive:

```bash
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out \
  --dvc-stage train --dry-run
```

::: warning Retrieval costs real money
Glacier and Deep Archive charge per-GB retrieval fees on top of storage, and
`expedited` is billed at a premium. Use `--dry-run` to see the estimate and
`--max-restore-cost` as a hard ceiling so a large accidental restore can't run up
the bill.
:::

## Deferred restore jobs

Without `--wait`, a Glacier restore doesn't block for hours. Instead it requests
the thaw, saves a **job** under `$XDG_DATA_HOME/cargoship/restore-jobs/` (falling
back to `~/.cargoship/restore-jobs/`), and prints a job ID. You come back later
to finish the download:

```bash
# Kick off the thaw and walk away
cargoship restore s3://my-bucket/.../20260721-a1b2c3 ./out --dvc-stage train

# List saved jobs and their status
cargoship restore jobs list

# Poll S3; mark jobs 'ready' once all chunks are accessible
cargoship restore jobs check

# Download a job once it's ready
cargoship restore jobs download JOB_ID

# Tidy up completed/failed jobs (default: older than 24h)
cargoship restore jobs clean --older-than 72h
```

`restore jobs check` accepts an optional job ID to poll just one job;
`restore jobs download` takes `--cache-gb` to size the LRU chunk cache. See the
[command reference](/reference/commands/inspect) for the complete `restore jobs`
flags.

## Best practices

::: tip
- **`--dry-run` any Glacier/Deep Archive restore** before committing — confirm
  size and cost first.
- **Cap spend with `--max-restore-cost`** in scripts so a bad selector can't
  trigger a huge bill.
- **Prefer `bulk` for large, non-urgent restores** and reserve `expedited` for
  genuine emergencies.
- **Use deferred jobs** (omit `--wait`) for long `standard`/`bulk` thaws instead
  of holding a terminal open for hours.
- **Restore by `--dvc-stage` or `--git-commit`** to rebuild an exact experiment
  input set rather than hunting for individual paths.
:::

## Interactive alternatives

- [`cargoship browse`](/guides/browsing) — a terminal UI to pick files and restore them.
- [`cargoship shell`](/guides/browsing) — navigate the archive (`ls`/`cd`/`cat`/`get`) and extract individual files.

## See also

- [Integrity model](/project/integrity) — verify-on-restore and the byte-identity claim.
- [Downloading & extracting](/guides/downloading) — for readable storage classes.
- [Concepts: restore vs. download](/intro/concepts#restore-vs-download).
- [Tier-aware storage](/guides/features/tiering) — how files end up in Glacier.
- Reference: [Inspection & retrieval](/reference/commands/inspect).
