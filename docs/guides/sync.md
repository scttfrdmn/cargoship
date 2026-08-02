# Incremental sync

`cargoship sync` keeps an S3 archive in step with a local directory by uploading
only the files that are new or have changed since the last run. The first sync
uploads everything (like [`cargoship upload`](/guides/uploading)); every sync
after that reads the previous manifest, compares it against the current
filesystem, and transfers just the differences — then writes a new manifest that
references the prior one. The exhaustive flag list lives in the
[command reference](/reference/commands/upload).

```bash
cargoship sync SOURCE_DIR s3://BUCKET/PREFIX/
```

## How change detection works

By default, sync runs in **fast mode**: a file is re-uploaded when either

- its **size** differs from the manifest, or
- its **modification time** is newer than the manifest.

This is quick because it never re-reads file contents. For byte-accurate
detection — worth it when timestamps are unreliable (restored files, `rsync`
copies, CI checkouts) — add `--checksum` to compare SHA256 hashes instead.

```bash
# Fast mode (default): size + mtime
cargoship sync ./my-data s3://my-bucket/backups/

# Byte-accurate: computes SHA256 (slower, no false negatives)
cargoship sync ./my-data s3://my-bucket/backups/ --checksum
```

## Previewing a sync

Run with `--dry-run` to see exactly what would transfer without uploading
anything. This is the safe way to sanity-check a large sync before committing to
the bandwidth and cost:

```bash
cargoship sync ./my-data s3://my-bucket/backups/ --dry-run
```

```
Comparing ./my-data against previous manifest...
  new:      42 files   (1.2 GB)
  changed:   7 files   (318 MB)
  unchanged: 9,914 files (skipped)
Would upload 49 files (1.5 GB) across 10 shards. No changes made (--dry-run).
```

## Tracking deletions

By default, sync is additive: files removed locally stay in the archive so old
manifests remain fully restorable. Add `--track-deletes` to record local
deletions in the new manifest, keeping it a faithful mirror of the source tree:

```bash
cargoship sync ./my-data s3://my-bucket/backups/ --checksum --track-deletes
```

::: info Deletions are recorded, not purged
`--track-deletes` marks files as deleted in the *new* manifest so restores from
it won't include them. It does not delete chunk data from S3 — earlier manifests
still reference those chunks. Use an [S3 lifecycle policy](/guides/cost/lifecycle)
to expire old data on a schedule.
:::

## Forcing a full re-sync

If a manifest is missing or you want a clean baseline, `--force` ignores the
previous manifest and uploads everything, just like a first run:

```bash
cargoship sync ./my-data s3://my-bucket/backups/ --force
```

## Sharding, compression & storage class

Sync accepts the same tuning knobs as `upload`:

```bash
cargoship sync ./my-data s3://my-bucket/backups/ \
  --shard-count 16 --shard-strategy size \
  --compression-level 9 \
  --storage-class GLACIER_IR
```

- `--shard-count` — number of parallel shards, 1–100 (default 10). Unlike
  `upload`, `sync` does not auto-select the count.
- `--shard-strategy` — `round-robin` (default), `hash`, `size`, `type`,
  `directory`. See [Sharding](/guides/features/sharding).
- `--compression-level` — Zstandard 1–22. Passing it **overrides** content-aware
  per-chunk selection and pins every chunk to that level; omit it to keep
  automatic selection. See [Compression](/guides/features/compression).
- `--storage-class` — `STANDARD` (default), `GLACIER_IR`, `DEEP_ARCHIVE`, etc.
  See [Lifecycle & storage classes](/guides/cost/lifecycle).
- `-q`/`--quiet` — minimal output for cron and scripts.
- `-r`/`--region` — AWS region (default `us-west-2`); match your bucket.

## sync vs. upload --incremental

Both do incremental transfers. Reach for `sync` when you want a repeatable
directory-mirroring workflow that manages the manifest chain for you. Reach for
[`upload --incremental`](/guides/uploading#incremental-sync) when you already
hold a specific previous manifest and want to layer one incremental pass on top
of it.

## Best practices

::: tip
- **Always `--dry-run` first** on a new sync target — confirm the new/changed
  counts look sane before spending bandwidth.
- **Use `--checksum`** when mtimes can't be trusted (restored trees, CI, `rsync`);
  stick with the default fast mode for ordinary day-to-day backups.
- **Add `--track-deletes`** only when you want the archive to mirror the source;
  leave it off when you want an append-only history.
- **Pin `--region`** (or set `AWS_REGION`) so scheduled syncs never guess wrong.
- **Run quiet in cron**: add `-q` and check the exit code.
:::

## See also

- [Uploading data](/guides/uploading) — one-shot uploads and the `--incremental` flag.
- [Resuming interrupted uploads](/guides/resuming) — recover a sync that was cut off.
- [Concepts: upload & manifest](/intro/concepts#manifest) — what a sync produces.
- Reference: [Uploading & sync commands](/reference/commands/upload).
