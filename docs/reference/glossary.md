# Glossary

Quick alphabetical lookup of CargoShip terms. For the same terms explained in
context, see [Concepts & terminology](/intro/concepts).

**CargoHold** — CargoShip's sharding subsystem: the intelligent distribution of
chunks across shards. "CargoHold sharding" refers to this.

**Chunk** — The unit that becomes one S3 object. The chunker groups source files
into chunks (targeting a size range) and the archiver writes each as a `tar.zst`
(or `.tar`) object. A very large file may be split across multiple chunks (see
[split-file records](/reference/format/split-files)).

**Download** — Pulling files that are already in an immediately-readable storage
class ([`cargoship download`](/guides/downloading)). Contrast with *restore*.

**File entry** — One record in the manifest for one source file: its relative
path, size, modification time, the chunk and shard it landed in, its S3 key, and
optional hashes and metadata (AI-detected type, DVC provenance).

**Manifest** — The JSON index of an upload (`manifest.json` / `manifest.json.gz`).
It maps every file entry to its chunk, shard, and S3 key, and records totals,
compression, encryption, and optional Git/DVC metadata. Current format is v2.0
(v1.0 is read-compatible). It is the source of truth for all later operations —
see the [manifest schema](/reference/format/manifest).

**Project** — A cost-tracking label. Assigning `--project ID` on upload groups
spend so you can set [budgets and quotas](/guides/cost/budgets) and generate
per-project [cost reports](/guides/cost/management).

**Restore** — Getting files back out, additionally handling Glacier / Deep Archive
retrieval (a thaw step with retrieval cost) and selection by hash, path, Git
commit, or DVC stage ([`cargoship restore`](/guides/restoring)). Contrast with
*download*.

**Shard** — An S3 key prefix (`shard-0`, `shard-1`, …) that chunks are distributed
across to parallelize S3 request throughput. The number of shards — the **shard
count** — is chosen [adaptively](/guides/features/sharding) (4–32) or set with
`--shard-count`; distribution is controlled by `--shard-strategy` (round-robin,
hash, size, type, directory).

**Storage class / tier** — The S3 storage class an object is written with
(STANDARD, INTELLIGENT_TIERING, GLACIER, DEEP_ARCHIVE, …). CargoShip can assign
classes per chunk based on file age with
[tier-aware storage](/guides/features/tiering).

**Upload** — A single run of `cargoship upload` (or `sync`). One upload produces
one set of archive objects and one manifest, grouped under a unique upload ID.

**Upload ID** — A timestamped identifier (e.g. `20260721-a1b2c3`) that namespaces
everything an upload produces in S3, under `…/uploads/<upload-id>/`. You pass it to
inspection and restore commands.

## See also

- [Concepts & terminology](/intro/concepts) — the same terms in context.
- [How it works](/intro/how-it-works).
- [Cheat sheet](/reference/cheatsheet).
