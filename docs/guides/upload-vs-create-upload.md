# `upload` vs. `create upload`

CargoShip has two commands that upload a directory to S3. This page settles which
to use.

## Short answer

**Use [`cargoship upload`](/reference/commands/upload).** It is the canonical,
fully-featured upload command. Reach for `cargoship create upload` only if you
specifically need its lower-level streaming-pipeline tuning knobs.

## The two commands

| | `cargoship upload` | `cargoship create upload` |
|---|---|---|
| Role | Canonical, feature-rich | Streaming-pipeline variant |
| Sharding | Adaptive shard count (4–32) + strategies | Fixed `--shards` (default 8) |
| Tier-aware storage | Yes (`--auto-tier`, `--tier-*`) | No |
| Deduplication | Yes (`--enable-dedup`) | No |
| KMS encryption | Yes (`--kms-key-id`, `--encrypt-manifest`) | No |
| Incremental sync | Yes (`--incremental`, `--prev-manifest`) | No |
| DVC / Git metadata | Yes (`--dvc-auto`, `--git-metadata`, …) | No |
| Explicit pipeline tuning | Via transporter flags | Yes (`--workers`, `--chunk-size-mb`, HTTP/2 knobs) |
| Multiple source dirs | One source | Accepts multiple source dirs |

## When to use `create upload`

`cargoship create upload` exposes the streaming pipeline's worker counts, chunk
size, and HTTP/2 connection tuning directly. Consider it when:

- You're **benchmarking or tuning** throughput and want to set per-stage worker
  counts and chunk size explicitly.
- You need to **archive several source directories** in one invocation.
- You want the pipeline's built-in resume flags (`--resume`, `--upload-id`) and
  failure-cleanup controls without the higher-level features.

For everything else — tiering, dedup, encryption, incremental sync, DVC
integration, and the adaptive shard count — use `cargoship upload`.

## Same on-disk result

Both commands produce the **same archive and manifest format** (tar.zst chunks +
JSON manifest, [documented here](/reference/format/)). An upload made with one can
be inspected, verified, and restored the same way regardless of which command
created it.

## See also

- [Uploading data](/guides/uploading) — the full `upload` guide.
- [Multi-prefix sharding](/guides/features/sharding) — how the shard count is chosen.
- Reference: [`upload`](/reference/commands/upload).
