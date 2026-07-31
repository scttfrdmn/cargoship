---
prev:
  text: Your first upload
  link: /start/first-upload
next:
  text: Clean up
  link: /start/cleanup
---

# Verify & restore it

An archive you can't get back isn't a backup. This step proves the round trip:
inspect the upload, verify its integrity, and pull a file back.

Throughout, replace the URL with your bucket, prefix, and the upload ID that
`cargoship upload` printed.

## Inspect the manifest

See what's in the upload without downloading anything:

```bash
cargoship info s3://my-bucket/archives/uploads/20260721-a1b2c3
```

This reads the manifest and prints totals (files, bytes, chunks, shards),
compression ratio, and encryption status. Add `--verbose` for per-shard detail or
`--json` for scripting. To list individual files:

```bash
cargoship list s3://my-bucket/archives/uploads/20260721-a1b2c3 --pattern '*.fastq'
```

## Verify integrity

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3
```

`verify` checks the archive against the checksums recorded in the manifest and
exits non-zero if anything is missing or corrupt — so it's safe to use in scripts
and CI. Use `--quick` for a fast metadata-only check, or `--deep` to re-download
the stored objects and re-hash them for data-level integrity.

## Restore a file

Pull a single file back to a local directory:

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored \
  --file path/inside/dataset.txt
```

You can also restore by pattern, by Git commit, or by DVC stage — see
[Restoring files](/guides/restoring). If that file returns byte-for-byte, the
full pipeline (compress → shard → upload → manifest → restore) is proven working.

::: tip Glacier & Deep Archive
If you uploaded to a cold storage class, files must be **thawed** before download.
`restore` handles this and can estimate retrieval cost first
(`--max-restore-cost`, `--tier`, `--wait`). See
[Restoring files](/guides/restoring#glacier).
:::

## Browse interactively (optional)

To explore an archive like a filesystem without extracting it:

```bash
cargoship shell s3://my-bucket/archives/uploads/20260721-a1b2c3
```

Then `ls`, `cd`, `cat`, `stat`, `find`, and `get` inside the archive. See
[Browsing archives](/guides/browsing).

## Next

- [Clean up](/start/cleanup) — remove the test upload safely.
- [Verifying integrity](/guides/verifying) and [Restoring files](/guides/restoring)
  for the full commands.
