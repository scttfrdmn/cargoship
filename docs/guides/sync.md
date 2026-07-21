# Incremental sync

`cargoship sync` keeps an S3 archive in step with a local directory by uploading
only the files that are new or have changed since the last run. The first sync
uploads everything (like `cargoship upload`); every sync after that reads the
previous manifest, compares it against the current filesystem, and transfers just
the differences — then writes a new manifest that references the prior one.

```bash
cargoship sync ./my-data s3://my-bucket/backups/
```

By default, change detection is fast: a file is re-uploaded when its size or
modification time differs from the manifest. Use `--checksum` for byte-accurate
detection (computes SHA256, slower), `--dry-run` to preview what would transfer,
and `--track-deletes` to record files that disappeared locally.

```bash
cargoship sync ./my-data s3://my-bucket/backups/ --dry-run
cargoship sync ./my-data s3://my-bucket/backups/ --checksum --track-deletes
```

::: warning Draft
This page is being expanded. For the complete, always-current flag list — including
`--force`, `--shard-count`, `--shard-strategy`, and `--storage-class` — see the
[`sync` command reference](/reference/commands/upload).
:::

## See also

- [Uploading data](/guides/uploading) — one-shot uploads and the `--incremental` flag.
- [Concepts: upload & manifest](/intro/concepts#upload) — what a sync produces.
- Reference: [Uploading & sync commands](/reference/commands/upload).
