# Resuming interrupted uploads

Large uploads can be interrupted — a dropped connection, a laptop lid, a Ctrl-C.
CargoShip checkpoints progress to `~/.cargoship/state/` as it goes, so re-running
the same `cargoship upload` command automatically picks up where it left off
instead of starting over. The `cargoship resume` command manages those saved
states. The full flag list lives in the [command reference](/reference/commands/upload).

## How it works

While an upload runs, CargoShip writes a small state file (typically 10–50 KB)
recording which chunks have been uploaded, the upload configuration, and file
hashes. Each state file lives in `~/.cargoship/state/` and is keyed by upload ID.

If the upload stops for any reason, just run the **same** `cargoship upload`
command again — CargoShip detects the matching state, skips the chunks already in
S3, and continues:

```bash
cargoship upload ./my-data s3://my-bucket/archives/
# ...interrupted at 60%...
cargoship upload ./my-data s3://my-bucket/archives/   # resumes from ~60%
```

## Managing saved states

```bash
# Show resumable uploads and their progress
cargoship resume list

# Resume a specific interrupted upload by ID
cargoship resume 20260721-143052-a3b4c5d6

# Clean up state files older than 24h
cargoship resume clean --older-than 24h
```

`cargoship resume list` reads `~/.cargoship/state/` and prints each upload's ID,
status, source and destination, progress in files and bytes, and how long ago it
started:

```
UPLOAD ID                    STATUS       PROGRESS         SOURCE → DEST
20260721-143052-a3b4c5d6     interrupted  6.2/10.3 GB (60%) ./my-data → s3://my-bucket/archives/
20260720-091500-9f8e7d6c     completed    48.2/48.2 GB      ./genomics → s3://my-bucket/archives/
```

`cargoship resume <upload-id>` resumes one specific upload without re-typing the
original source and destination — the state file already holds them.

## Cleaning up

State files are tiny, but they accumulate. `cargoship resume clean` removes them:

```bash
# Default: remove states older than 24h
cargoship resume clean

# Remove states older than a week
cargoship resume clean --older-than 168h

# Remove only fully completed uploads
cargoship resume clean --completed
```

- `--older-than` — age threshold as a Go duration (e.g. `24h`, `72h`, `168h`).
- `--completed` — clean only uploads that finished successfully.

## Starting fresh

To deliberately ignore a saved state and re-upload everything, run the upload
with `--force-restart`. This bypasses resume detection and discards the previous
checkpoint:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --force-restart
```

Use it when the source data changed substantially, or when you want a clean
baseline rather than continuing a stale, partial upload.

## Best practices

::: tip
- **Re-run the identical command** to resume — same source, same destination, and
  CargoShip does the rest automatically.
- **Check `resume list`** after a crash to confirm progress before re-running.
- **Reach for `--force-restart`** only when you truly want to discard prior
  progress; normal re-runs already resume.
- **Schedule `resume clean --completed`** so old state files don't pile up.
:::

## See also

- [Uploading data](/guides/uploading).
- [Incremental sync](/guides/sync) — only transfer new or changed files.
- Reference: [Uploading & sync commands](/reference/commands/upload).
