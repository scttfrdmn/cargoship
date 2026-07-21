# Archive & Manifest Format Spec

This is the authoritative, versioned specification for the CargoShip on-disk and
on-S3 storage format. It is written for **tooling authors** — anyone building a
reader, mounting layer, verifier, or migration tool that must consume CargoShip
archives without running CargoShip itself.

The format is **open, transparent, and portable** by design. Every archive can
be extracted with standard `tar` and `zstd`, and every upload is described by a
JSON manifest you can parse with any language. Nothing about reading your data
depends on CargoShip being installed.

::: tip This spec describes the wire format, not the CLI
For command behavior and flags, see the [Command reference](/reference/commands/upload).
For the conceptual model behind chunks and shards, see
[How it works](/intro/how-it-works). This page is the contract that governs the
bytes and JSON on S3.
:::

## The seven pages

| Page | Covers |
|------|--------|
| [Archive layout & shard keys](/reference/format/archive-layout) | The `tar`/`.tar.zst` container, the S3 key scheme, upload IDs, shard assignment, and per-object S3 metadata. |
| [Compression](/reference/format/compression) | The content-adaptive zstd level table, the plain-`.tar` fallback, and how a reader detects which applies. |
| [Manifest schema](/reference/format/manifest) | Every struct in the manifest JSON — `Manifest`, `FileEntry`, `ChunkEntry`, `ShardEntry`, and all optional metadata blocks — with exact field names, types, and JSON tags. |
| [Encryption](/reference/format/encryption) | The `EncryptionMetadata` block, S3 server-side data encryption, and the KMS-envelope-encrypted manifest wrapper. |
| [Split files](/reference/format/split-files) | The `{path}.part{N}` naming and `CARGOSHIP.*` PAX records used when a single large file spans multiple chunks. |
| [Reading archives (Go)](/reference/format/library-api) | Using the `github.com/scttfrdmn/cargoship/pkg/manifest` package to parse manifests, query files, and extract from chunks. |

## Format versioning

The format version is carried in the manifest's top-level `version` field. It is
independent of the CargoShip product version (the current product release is
v0.13.2; the current format version is **2.0**).

| Format version | Status | Notes |
|----------------|--------|-------|
| `2.0` | **Current** — written by all current releases | Adds optional encryption, deduplication, incremental-sync, and DVC/Git provenance blocks on top of the 1.0 core. |
| `1.0` | **Read-compatible** | The original stable format. Current CargoShip reads 1.0 manifests transparently; the core `Manifest`, `FileEntry`, `ChunkEntry`, and `ShardEntry` fields are unchanged. |

The version constants are defined in the source as:

```go
const (
	// ManifestVersion is the current manifest format version
	ManifestVersion = "2.0"

	// ManifestVersionV1 is the legacy v1.0 manifest format version (backward-compat read-only)
	ManifestVersionV1 = "1.0"
)
```

### What 2.0 adds over 1.0

Every field introduced after 1.0 is `omitempty` in the JSON — a 2.0 manifest
that uses none of the new features serializes identically to a 1.0 manifest
except for its `version` string. The additive blocks are:

- `encryption` — [`EncryptionMetadata`](/reference/format/encryption)
- `deduplication` — `ManifestDeduplication`
- `version_info`, `git_metadata`, `dvc_compatibility`, `dvc_pipeline` — dataset and pipeline provenance
- `previous_manifest_id`, `sync_type` — incremental sync chaining
- Per-file split fields (`offset`, `length`, `part_index`, `total_parts`) and DVC/dedup fields

### Compatibility rules for readers

::: info Reader contract
A conformant reader **must** follow these rules to remain forward-compatible.
:::

1. **Ignore unknown fields.** New optional fields may be added within a major
   version. A `2.0` reader must not fail on fields it does not recognize.
2. **Treat absent optional blocks as "feature not used."** `encryption`,
   `deduplication`, and the DVC/Git blocks are pointers that are omitted when
   unused. Their absence is normal, not an error.
3. **Branch on `version` only for major changes.** Within `2.x`, behavior is
   additive. A future `3.0` would signal a breaking change and warrant explicit
   handling.
4. **Do not assume compression from the manifest alone.** A chunk may be a plain
   `.tar` even when the manifest's top-level `compression_type` is `zstd`. See
   [Compression](/reference/format/compression) for the authoritative per-chunk
   signal (the key extension and the object's stored size).

## Stability guarantees

- The `tar` + `zstd` container and the `uploads/{upload-id}/shard-{n}/chunk-{m}`
  key scheme are stable and extractable with standard tooling.
- The `version` field governs format compatibility; new minor additions are
  backward compatible and clients ignore unknown fields.
- CargoShip **reads** all historical format versions; it **writes** the current
  version (`2.0`).

## Next

Start with the [Archive layout & shard keys](/reference/format/archive-layout) —
it defines the object names and container that every other page builds on.
