# Concepts & terminology

CargoShip uses a small, consistent vocabulary. These terms appear throughout the
docs and in command output — this page defines each once. See also the
[Glossary](/reference/glossary) for a quick alphabetical lookup.

## Upload

A single run of `cargoship upload` (or `sync`). One upload produces one set of
archive objects and one manifest, all grouped under a unique **upload ID**.

## Upload ID

A timestamped identifier (e.g. `20260721-a1b2c3`) that namespaces everything an
upload produces in S3, under `…/uploads/<upload-id>/`. You pass it to
inspection and restore commands (`--upload-id`, or as part of an `s3://…/uploads/<id>`
URL).

## File entry

One record in the manifest for one source file: its relative path, size,
modification time, the chunk and shard it landed in, its S3 key, and optional
hashes and metadata (including AI-detected type and DVC provenance).

## Chunk

The unit that becomes **one S3 object**. The chunker groups source files into
chunks (targeting a size range), then the archiver writes each chunk as a
`tar.zst` (or `.tar`) object. A very large file may be split across multiple
chunks (see [split-file records](/reference/format/split-files)).

## Shard

An S3 key **prefix** (`shard-0`, `shard-1`, …) that chunks are distributed across
to parallelize S3 request throughput. The number of shards — the **shard count** —
is chosen [adaptively](/guides/features/sharding) or set with `--shard-count`.
Distribution is controlled by `--shard-strategy` (hash, size, type, directory).

## CargoHold

The name for CargoShip's sharding subsystem — the intelligent distribution of
chunks across shards. When you see "CargoHold sharding," it refers to this.

## Manifest

The JSON index of an upload (`manifest.json` / `manifest.json.gz`). It maps every
file entry to its chunk, shard, and S3 key, and records totals, compression,
encryption, and optional Git/DVC metadata. Current format is **v2.0**
(v1.0 is read-compatible). It is the source of truth for all later operations —
see the [Manifest schema](/reference/format/manifest).

## Storage class & tier

The S3 storage class an object is written with (STANDARD, INTELLIGENT_TIERING,
GLACIER, DEEP_ARCHIVE, …). CargoShip can assign classes per chunk based on file
age with [tier-aware storage](/guides/features/tiering).

## Restore vs. download

- **Download** ([`cargoship download`](/guides/downloading)) pulls files that are
  in an immediately-readable storage class.
- **Restore** ([`cargoship restore`](/guides/restoring)) additionally handles
  Glacier/Deep Archive retrieval (which requires a thaw step and has retrieval
  cost), and can select files by hash, path, Git commit, or DVC stage.

## Project

A cost-tracking label. Assigning `--project <id>` on upload groups spend so you
can set [budgets and quotas](/guides/cost/budgets) and generate per-project
[cost reports](/guides/cost/management).

## Next

- [How it works](/intro/how-it-works) — see these pieces in motion.
- [Quick Start](/start/quickstart) — put them to use.
