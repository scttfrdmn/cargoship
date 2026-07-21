# Downloading & extracting

`cargoship download` pulls files out of an upload and extracts them locally. It
first fetches the manifest, works out which chunks hold the files you asked for,
and downloads only those chunks — so retrieving a handful of files from a large
archive does not require downloading the whole thing.

```bash
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored
```

Narrow the extraction with `--pattern` (glob), `--files` (exact comma-separated
paths), or `--shard-ids` (specific shards). Add `--dry-run` to see what would be
downloaded first.

```bash
cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./logs \
  --pattern "*.log"

cargoship download s3://my-bucket/archives/uploads/20260721-a1b2c3 ./reports \
  --files "data/report.csv,data/summary.csv"
```

`download` works on files already in a readable storage class. For Glacier /
Deep Archive retrieval (which needs a thaw step) or selecting files by hash, Git
commit, or DVC stage, use [`cargoship restore`](/guides/restoring).

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Inspection & retrieval command reference](/reference/commands/inspect).
:::

## See also

- [Restoring files (incl. Glacier)](/guides/restoring).
- [Browsing archives (TUI/shell)](/guides/browsing) — pick files interactively.
- [Concepts: restore vs. download](/intro/concepts#restore-vs-download).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
