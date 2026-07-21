# Downloading & extracting

`cargoship download` pulls files out of an upload and extracts them locally. It
first fetches the lightweight manifest (~30 KB), works out which chunks hold the
files you asked for, and downloads only those chunks — so retrieving a handful of
files from a large archive never requires downloading the whole thing. The full
flag list lives in the [command reference](/reference/commands/inspect).

```bash
cargoship download S3_URL OUTPUT_DIR
```

## Downloading a whole upload

```bash
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored
```

CargoShip reads the manifest, downloads every chunk, decompresses, and
reconstructs the original directory tree under `./restored`.

## Selective extraction

Most retrievals only need part of an archive. Narrow the extraction three ways —
CargoShip maps your selection to the chunks that actually contain those files and
skips the rest.

```bash
# By glob pattern
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./logs \
  --pattern "*.log"

# By exact paths (comma-separated)
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./reports \
  --files "data/report.csv,data/summary.csv"

# By shard ID
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored \
  --shard-ids 0,2,4
```

- `--pattern` — a single glob (e.g. `"*.log"`, `"data/*.csv"`).
- `--files` — comma-separated list of exact paths within the archive.
- `--shard-ids` — comma-separated shard IDs; useful when you already know a
  shard's contents from [`cargoship info --verbose`](/guides/inspecting).

## Preview with --dry-run

Always safe to run first: `--dry-run` resolves your selection against the
manifest and reports what would download without transferring anything.

```bash
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./reports \
  --pattern "*.csv" --dry-run
```

```
Loaded manifest: 9,914 files across 10 shards
Pattern "*.csv" matched 128 files in 3 chunks (2 shards)
Would download 3 chunks (412 MB) → ./reports. No data transferred (--dry-run).
```

Add `--verbose` to list each file as it is extracted, and `-r`/`--region` to
match your bucket's region (default `us-west-2`).

## When to use restore instead

`download` works on files that are already in a **readable** storage class
(`STANDARD`, `STANDARD_IA`, `INTELLIGENT_TIERING`, `GLACIER_IR`). If the chunks
live in **Glacier Flexible Retrieval or Deep Archive**, they must be thawed
first — `download` cannot do that. It also selects only by pattern, path, or
shard. Reach for [`cargoship restore`](/guides/restoring) when you need to:

- retrieve from Glacier / Deep Archive (see [Glacier & Deep Archive](/guides/restoring#glacier)), or
- select files by content hash, Git commit, or DVC pipeline stage.

See [restore vs. download](/intro/concepts#restore-vs-download) for the full
distinction.

## Best practices

::: tip
- **Filter, don't fetch everything** — `--pattern` or `--files` on a large
  archive saves both time and egress cost.
- **`--dry-run` before big pulls** to confirm the match count and size.
- **Match the region** with `-r` (or `AWS_REGION`) to avoid cross-region latency.
- **Reconstruct into a fresh directory** so a partial extraction can't shadow
  existing files.
:::

## See also

- [Restoring files (incl. Glacier)](/guides/restoring).
- [Browsing archives (TUI/shell)](/guides/browsing) — pick files interactively.
- [Listing & inspecting uploads](/guides/inspecting) — see what's inside first.
- [Concepts: restore vs. download](/intro/concepts#restore-vs-download).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
