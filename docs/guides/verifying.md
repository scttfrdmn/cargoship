# Verifying integrity

`cargoship verify` confirms that an upload is complete and internally consistent
by validating its manifest and the checksums recorded in it. It downloads the
manifest, checks that shard counts, file counts, and size totals agree, looks for
missing or corrupted metadata, and validates checksum coverage where present — all
without downloading the archive data itself.

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3
```

Use `--quick` for a fast, metadata-only pass (structure and consistency, skipping
deeper checks), and `--verbose` to see per-error detail. `verify` is a good gate
before a restore, and useful for compliance or audit checks.

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --quick
```

**Exit codes:** `0` — all checks passed; `1` — verification failed (errors found).
This makes `verify` easy to wire into scripts and CI.

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Inspection & retrieval command reference](/reference/commands/inspect).
:::

## See also

- [Listing & inspecting uploads](/guides/inspecting).
- [Verify & restore it](/start/verify-and-restore) — the round-trip walkthrough.
- [Concepts: manifest](/intro/concepts#manifest).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
