# Listing & inspecting uploads

CargoShip can tell you everything about an upload without downloading a single
chunk — it reads the lightweight manifest (~30 KB) instead. `cargoship list`
enumerates the files in an upload, `cargoship info` prints upload-level metadata
and statistics (file count, sizes, compression ratio, per-shard distribution,
storage location), and `cargoship balance` analyzes how evenly chunks are spread
across shards.

```bash
cargoship info s3://my-bucket/archives/uploads/20260721-a1b2c3
cargoship list --bucket my-bucket --upload-id 20260721-a1b2c3 --pattern "*.csv"
cargoship balance s3://my-bucket/archives/uploads/20260721-a1b2c3
```

Both `info` and `list` support `--json` / `--verbose` for scripting and detail;
`balance` reports imbalance and can plan a rebalance with `--dry-run`.

::: warning Draft
This page is being expanded. For the complete flag list on `list`, `info`, and
`balance`, see the [Inspection & retrieval command reference](/reference/commands/inspect).
:::

## See also

- [Verifying integrity](/guides/verifying) — check an upload against its checksums.
- [Downloading & extracting](/guides/downloading) — get files back out.
- [Concepts: manifest](/intro/concepts#manifest) — what these commands read.
- Reference: [Inspection & retrieval](/reference/commands/inspect).
