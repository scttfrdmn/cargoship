# Verifying integrity

`cargoship verify` confirms that an upload is complete and internally consistent
by validating its manifest and the checksums recorded in it. It downloads the
manifest (~30 KB), checks that shard counts, file counts, and size totals agree,
looks for missing or corrupted metadata, and validates checksum coverage where
present — all without downloading the archive data itself. Add `--deep` to go
further and re-download the stored objects to confirm the bytes still match
their recorded checksums. The full flag list lives in the
[command reference](/reference/commands/inspect).

```bash
cargoship verify S3_URL
```

## What verify checks

CargoShip records a checksum for every file and rolls per-shard and upload-level
totals into the manifest. `verify` re-derives those relationships and confirms
they still hold:

- the manifest structure parses and is the expected version,
- shard counts, file counts, and byte totals are internally consistent,
- no metadata entries are missing or malformed,
- checksum coverage is present and complete where recorded.

Because it reads only the manifest, a full verify of a multi-terabyte upload
finishes in seconds.

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3
```

```
📥 Downloading manifest: s3://my-bucket/archives/uploads/20260721-a1b2c3/manifest.json.gz
✅ Manifest downloaded successfully

🔍 Validating manifest integrity...
   Mode: Full validation (all checks)

✅ Validation: PASS

📊 Dataset Summary:
   Upload ID:        20260721-a1b2c3
   Total Files:      9,914 files
   Total Size:       48.2 GB
   Total Shards:     10
   Total Chunks:     186

✅ All 9,914 files verified successfully
```

When a check fails, `verify` prints the specific error with the field, expected
value, and actual value, then exits non-zero:

```
❌ Validation: FAIL

❌ Errors: 1
   • Shard count mismatch
     Field:    shard_count
     Expected: 10
     Actual:   9
```

## Three levels: quick, full, deep

`verify` runs at three depths, trading cost for assurance:

- **`--quick`** does a fast, metadata-only pass — it validates structure and
  consistency but skips the checksum checks. Use it as a cheap smoke test.
- **Full** (default) runs every consistency and checksum-coverage check against
  the manifest. It reads only the manifest, so a full verify of a multi-terabyte
  upload still finishes in seconds.
- **`--deep`** performs **data-level** verification: it re-downloads every stored
  object and recomputes its SHA-256, comparing against the checksums recorded at
  upload time. This is the only level that catches bit-rot or tampering in the
  stored data itself — a corrupted or missing object fails the check and exits
  non-zero. Because it transfers the archive data, it incurs GET requests and
  egress; run it periodically for long-term archives rather than on every check.

```bash
# Cheap metadata smoke test
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --quick

# Data-level integrity — re-download and re-hash the stored bytes
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --deep
```

Deep verify depends on the per-file checksums recorded at upload time (on by
default; disabled by `upload --no-file-checksums`). It also checks chunk-level
checksums regardless. For the full mechanics, see the
[Integrity model](/project/integrity).

Add `--verbose` for per-error and per-warning detail. Like `info` and `verify`'s
sibling commands, you can address the upload with an S3 URL or with
`--bucket`/`--prefix`/`--upload-id` flags, and `-r`/`--region` selects the region.

## Exit codes & CI

`verify` is designed to gate automation. It sets a clear exit code:

| Code | Meaning |
|------|---------|
| `0`  | All checks passed |
| `1`  | Verification failed (errors found) |
| `2`  | Bad invocation — unknown flag, missing argument |

A failed check is `1`, not `2`: the command ran correctly and the answer was
"this archive does not match". `2` means the command line itself was wrong, so
retrying it will not help. See [Exit codes](/reference/exit-codes) for the
CLI-wide contract.

That makes it a drop-in step in scripts, cron, and CI pipelines:

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --quick \
  || { echo "archive integrity check failed" >&2; exit 1; }
```

It's a good gate to run immediately after an upload, before a restore, and on a
schedule for compliance or audit requirements.

## Best practices

::: tip
- **Verify right after upload** to catch an incomplete transfer while the source
  data is still around.
- **Gate restores on `verify`** so you never thaw Glacier data for a broken
  archive.
- **Schedule a periodic `--quick` sweep** across critical uploads and alert on a
  non-zero exit.
- **Run `--deep` on a slower cadence** for long-term archives — it's the only
  level that detects bit-rot in the stored data, at the cost of GET requests and
  egress.
- **Use `--verbose`** when a check fails to see exactly which field diverged.
:::

## See also

- [Integrity model](/project/integrity) — what the byte-identity claim is and how
  `--deep` and verify-on-restore make it checkable.
- [Listing & inspecting uploads](/guides/inspecting).
- [Verify & restore it](/start/verify-and-restore) — the round-trip walkthrough.
- [Concepts: manifest](/intro/concepts#manifest).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
