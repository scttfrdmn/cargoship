# Archive layout & shard keys

A completed upload is a set of **chunk** objects plus one **manifest** object,
all living under a single **upload ID** prefix in an S3 bucket. This page
defines the container format for a chunk, the S3 key scheme, how chunks are
assigned to shards, and the per-object metadata CargoShip writes.

## The chunk container

Each chunk is a POSIX `tar` archive, optionally wrapped in a single Zstandard
stream:

```
chunk-{m}.tar.zst
└── [zstd stream]
    └── [tar archive]
        ├── data/file1.txt      (path relative to source root)
        ├── data/file2.log
        └── ...
```

- **Archive format:** POSIX `ustar` (Go's `archive/tar`).
- **Compression:** a single Zstandard frame wrapping the whole tar stream —
  **or none**, in which case the object is a plain `.tar`. See below.
- Chunks are produced by streaming: files flow `tar → zstd → S3` through
  in-memory pipes, so nothing is staged to local disk.

### tar attributes

Preserved on each entry:

- File path, relative to the source root (the tar entry `Name`).
- Exact byte size.
- Modification time.
- File mode / Unix permissions.

Not preserved (intentionally omitted for portability):

- Ownership (UID/GID), extended attributes (xattrs), ACLs, and sparse-file
  information.

### Plain `.tar` vs `.tar.zst`

CargoShip skips zstd for chunks whose contents are already compressed. During
archiving it counts how many files in the chunk are compressible; **if more than
half are already-compressed formats** (images, video, audio, archives — see
[Compression](/reference/format/compression)), it writes the tar directly with
no zstd frame and names the object `.tar`.

::: warning The key extension is the authoritative compression signal
Do not infer compression from the manifest's top-level `compression_type` field,
and do not trust the `cargoship-compression` S3 metadata header — both always
report `zstd`/`tar` even for plain-`.tar` chunks (see the caveat below). The
reliable indicators, in order, are:

1. **The object key's extension** — `.tar.zst` (zstd frame) vs `.tar` (raw tar).
2. **Magic bytes** — a zstd frame begins with `0x28 0xB5 0x2F 0xFD`.

A robust reader should try zstd-decode and fall back to reading the stream as raw
tar (or branch on the extension).
:::

## S3 key scheme

Every object for an upload lives under one prefix keyed by the upload ID:

```
s3://{bucket}/{prefix}/uploads/{upload-id}/
├── manifest.json.gz                         # the index (see below)
├── shard-0/
│   ├── chunk-0.tar.zst
│   └── chunk-8.tar.zst
├── shard-1/
│   ├── chunk-1.tar.zst
│   └── chunk-9.tar.zst
└── … shards 2..N
```

The chunk key is:

```
{prefix}/uploads/{upload-id}/shard-{n}/chunk-{m}.tar.zst
```

| Component | Format | Example | Notes |
|-----------|--------|---------|-------|
| `{prefix}` | optional, user-set | `production` | S3 key prefix; omitted (no leading segment) when empty. |
| `uploads` | fixed literal | `uploads` | Constant segment for all CargoShip uploads. |
| `{upload-id}` | `YYYYMMDD-HHmmss-{random}` | `20260721-123456-abcd1234` | Unique per upload session; timestamp plus hex suffix. |
| `shard-{n}` | `shard-0` … `shard-{N-1}` | `shard-3` | Shard (S3 prefix) index, 0-based. |
| `chunk-{m}` | `chunk-0` … `chunk-M` | `chunk-42` | Globally sequential chunk ID. |
| extension | `.tar.zst` or `.tar` | `.tar.zst` | zstd-wrapped, or plain tar for already-compressed chunks. |

::: info The manifest records the exact key
You never need to reconstruct a key by formatting rules: every `FileEntry`,
`ChunkEntry`, and `ShardEntry` in the [manifest](/reference/format/manifest)
carries the full `s3_key` (and shards carry `chunk_keys`). Use those. The scheme
here is for understanding and for tools that scan a bucket without a manifest.
:::

## Shard assignment

A chunk's shard is a deterministic function of its ID:

```go
shardID := job.ID % shardCount
```

Chunks round-robin across shards. With the default 8 shards, chunk 0 → shard 0,
chunk 1 → shard 1, …, chunk 8 → shard 0, and so on. Each shard is an independent
S3 key prefix, which is what lets uploads and restores parallelize S3 request
throughput rather than serializing through one prefix.

The shard count is chosen adaptively (4–32) from the workload's file count, size,
and the machine's resources, or set manually with `--shard-count`. The value
used for an upload is recorded in the manifest's `shard_count` field, and every
shard is enumerated in the `shards` array. See
[Why sharding matters](/intro/how-it-works#why-sharding-matters).

## Per-object S3 metadata

CargoShip attaches user-metadata headers to each chunk object:

| Metadata key | Example | Meaning |
|--------------|---------|---------|
| `cargoship-chunk-id` | `42` | The chunk's `ID`. |
| `cargoship-file-count` | `150` | Number of files in the chunk. |
| `cargoship-chunk-size` | `524288000` | Uncompressed total size of the chunk's files, in bytes. |
| `cargoship-compression` | `zstd` | Compression label — **see caveat**. |
| `cargoship-archive` | `tar` | Archive format label. |

Read them with:

```bash
aws s3api head-object \
  --bucket my-bucket \
  --key uploads/20260721-123456-abcd1234/shard-0/chunk-0.tar.zst \
  --query 'Metadata'
```

::: warning Metadata compression header caveat
The `cargoship-compression` header is written as the literal string `zstd` (and
`cargoship-archive` as `tar`) on **every** chunk, including plain-`.tar` chunks
that were not zstd-compressed. Do not rely on this header to decide how to
decode a chunk. The authoritative signals are the **key extension** (`.tar` vs
`.tar.zst`) and the per-chunk `CompressionType` in the manifest. See
[Compression](/reference/format/compression).
:::

## The manifest object

The upload's index sits alongside the shards under the upload-ID prefix. Its
name depends on compression and encryption:

| Object name | Content |
|-------------|---------|
| `manifest.json` | Plain JSON. |
| `manifest.json.gz` | gzip-compressed JSON (the common case). |
| `manifest.encrypted.json` | KMS-envelope-encrypted wrapper (plain). |
| `manifest.encrypted.json.gz` | Encrypted wrapper, gzip-compressed. |

::: warning `manifest.json.gz` is **gzip**, not zstd
The manifest uses gzip (`compress/gzip`), unlike the chunks which use zstd.
Decompress it with `gzip -d` / `zcat`, not `zstd -d`.
:::

See the [Manifest schema](/reference/format/manifest) for the JSON structure and
[Encryption](/reference/format/encryption) for the encrypted variants.

## Manual extraction with standard tools

Because the format is open, you can recover data with `aws`, `zstd`, and `tar`
alone:

```bash
# 1. Fetch and read the manifest (gzip, not zstd)
aws s3 cp s3://my-bucket/uploads/20260721-123456-abcd1234/manifest.json.gz .
gzip -dc manifest.json.gz | jq .

# 2. Download a chunk
aws s3 cp \
  s3://my-bucket/uploads/20260721-123456-abcd1234/shard-0/chunk-0.tar.zst .

# 3. Decompress and extract (.tar.zst)
zstd -d chunk-0.tar.zst
tar -xf chunk-0.tar -C ./extracted/

# For a plain .tar chunk, skip the zstd step:
#   tar -xf chunk-0.tar -C ./extracted/
```

For split large files reassembled across chunks, see
[Split files](/reference/format/split-files).
