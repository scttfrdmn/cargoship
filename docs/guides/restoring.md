# Restoring files

`cargoship restore` gets targeted files back out of an archive without
downloading the whole dataset, and — unlike [`download`](/guides/downloading) —
it handles Glacier / Deep Archive retrieval. You select what to restore by
content hash, exact path, Git commit, or DVC pipeline stage, and CargoShip
downloads only the chunks that contain those files.

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out \
  --file data/train.csv --file models/model.pkl
```

Selection modes (pick one, or combine `--file` with others):

- `--hash` — restore a single file by its MD5 content hash.
- `--file` — one or more exact file paths (repeatable).
- `--git-commit` — all files from a specific Git commit SHA.
- `--dvc-stage` — all files produced by a DVC pipeline stage.

## Glacier & Deep Archive {#glacier}

When the chunks you want live in Glacier or Deep Archive, they must be thawed
before download. `cargoship restore` requests that retrieval for you:

- `--tier` — retrieval speed: `expedited` (1–5 min), `standard` (3–5 h, default),
  `bulk` (5–12 h). Faster tiers cost more.
- `--wait` — block until the thaw finishes, then download.
- `--restore-days` — how many days to keep the restored copy available (default 7).
- `--max-restore-cost` — abort if the estimated retrieval cost exceeds this USD limit.
- `--dry-run` — show the size and estimated cost without restoring.

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./out \
  --dvc-stage train --tier standard --wait --max-restore-cost 5.00
```

::: info Restore jobs
Without `--wait`, a Glacier restore saves a job to `~/.cargoship/restore-jobs/`
and prints a job ID. Track it and finish the download later with
`cargoship restore jobs list|check|download|clean`.
:::

::: warning Draft
This page is being expanded. For the complete flag list on `restore` and
`restore jobs`, see the [Inspection & retrieval command reference](/reference/commands/inspect).
:::

## Interactive alternatives

- [`cargoship browse`](/guides/browsing) — a terminal UI to pick files and restore them.
- [`cargoship shell`](/guides/browsing) — navigate the archive (`ls`/`cd`/`cat`/`get`) and extract individual files.

## See also

- [Downloading & extracting](/guides/downloading) — for readable storage classes.
- [Concepts: restore vs. download](/intro/concepts#restore-vs-download).
- [Tier-aware storage](/guides/features/tiering) — how files end up in Glacier.
- Reference: [Inspection & retrieval](/reference/commands/inspect).
