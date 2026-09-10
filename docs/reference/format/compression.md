# Compression

CargoShip compresses chunks with **Zstandard (zstd)**, choosing the level per
chunk from the content types of the files inside it. Chunks dominated by
already-compressed data skip zstd entirely and ship as a plain `.tar`. This page
defines the level table, the per-chunk decision, and how a reader determines
what actually happened.

## Content-adaptive levels

CargoShip classifies each file into a content type from its extension (and, when
available, an AI-detected type — see below), then maps that type to a zstd level.
The internal `compression.Level` values are:

```go
const (
	LevelFastest Level = 1
	LevelFast    Level = 3
	LevelDefault Level = 5
	LevelBetter  Level = 7
	LevelBest    Level = 9
)
```

`LevelBest` (9) maps to zstd's `SpeedBestCompression`. The default content-aware
mapping:

| Content type | Level | Constant | Rationale |
|--------------|-------|----------|-----------|
| Code (`.go`, `.py`, `.js`, `.ts`, `.java`, `.c`, `.rs`, …) | 9 | `LevelBest` | Highest text redundancy — best ratio. |
| Documents (`.pdf`, `.docx`, `.xlsx`, `.pptx`, `.rtf`, …) | 6 | — | Good compression on text-heavy formats. |
| Text (`.txt`, `.md`, `.log`, `.csv`, `.ini`, `.conf`, …) | 6 | — | Good compression, high redundancy. |
| Binary (`.exe`, `.dll`, `.so`, `.bin`, `.o`, `.class`, …) | 3 | `LevelFast` | Moderate benefit; keep it cheap. |
| Images (`.jpg`, `.png`, `.gif`, `.webp`, …) | 1 | `LevelFastest` | Already compressed. |
| Video (`.mp4`, `.mkv`, `.mov`, `.webm`, …) | 1 (algorithm `none`) | `LevelFastest` | Already compressed; compression skipped. |
| Audio (`.mp3`, `.flac`, `.aac`, `.ogg`, …) | 1 (algorithm `none`) | `LevelFastest` | Already compressed; compression skipped. |
| Archives (`.zip`, `.gz`, `.7z`, `.tar`, `.rar`, …) | 1 (algorithm `none`) | `LevelFastest` | Already compressed; recompression skipped. |
| Unknown / default | 5 | `LevelDefault` | Safe middle ground. |

::: info Levels 6 and 5 have no named constant
Documents and text use level `6`, and the default is `5` (`LevelDefault`). The
named constants (`LevelFastest`/`Fast`/`Default`/`Better`/`Best` = 1/3/5/7/9)
cover the common points; intermediate levels are used directly as integers.
:::

### AI-assisted classification (Magika)

When CargoShip's optional Magika file-type detection is enabled, the detected
type is stored in `FileEntry.Metadata["magika_type"]` and takes **priority** over
the extension when picking a content type. This catches misnamed files and files
with no extension (e.g. source code in a `.bin` file). If no Magika type is
present, classification falls back to the extension. Either way the result is one
of the content types in the table above.

## The per-chunk plain-`.tar` decision

Compression is decided per chunk, not per file. While archiving a chunk,
CargoShip counts compressible vs. already-compressed files:

- If **more than half** the files are compressible, the whole chunk is written as
  `tar → zstd` at the level chosen from the chunk's content mix, and the object
  is named `.tar.zst`.
- Otherwise zstd is skipped: the tar stream is written raw and the object is
  named `.tar`.

A file counts as "already compressed" when its estimated compression benefit is
low — images, video, audio, and archives fall below the threshold, so a chunk
made mostly of those ships uncompressed.

## What the manifest and metadata report

The manifest's top-level compression fields describe the **upload's configured**
compression, not necessarily every chunk:

- `compression_type` — e.g. `"zstd"`
- `compression_level` — the level integer
- `compression_ratio` — achieved ratio across the upload

::: warning Do not decide decode strategy from the top-level fields or S3 headers
Both the manifest's top-level `compression_type` and the `cargoship-compression`
S3 object header report `zstd` even for a chunk that was written as a plain
`.tar`. To decode a specific chunk correctly, use — in order of reliability:

1. **The chunk key's extension**: `.tar.zst` = zstd-wrapped, `.tar` = raw tar.
2. **Magic bytes**: a zstd frame starts with `0x28 0xB5 0x2F 0xFD`.
3. **Try/fallback**: attempt zstd-decode; if the stream is not a zstd frame,
   read it as raw tar.

A per-chunk `CompressionType` in the manifest, when present, is the most specific
field-level signal, but the key extension is always definitive.
:::

## Random-access frame index (format 2.1)

By default a compressed chunk is a single zstd frame, so restoring one file means
downloading and decoding the whole chunk. With `--frame-size N` (default
`16MiB`; `0` disables it), the archiver cuts the zstd stream into multiple
independently-decodable frames at file boundaries — a new frame once `N`
uncompressed bytes have accumulated — and records a **frame index** in the
manifest. A file never spans a frame, so a reader can fetch and decode just the
one frame that contains it.

The index lives in three additive fields (all `omitempty`):

- `format_features` — contains `"frames"` when any chunk carries a frame index.
- `chunks[].frames[]` — per frame: `compressed_offset`, `compressed_size` (its
  byte range within the chunk object, for a ranged `GET`), `uncompressed_offset`,
  and `uncompressed_size` (its span in the tar stream).
- `files[].archive_offset` — the byte offset of the file's data within the
  uncompressed tar stream (right after its tar header).

Each frame also carries `checksum` — the SHA-256 (hex) of its **compressed**
bytes, i.e. exactly what a ranged `GET` of `[compressed_offset, compressed_size)`
returns.

To read a single file with the index: find the frame whose
`[uncompressed_offset, uncompressed_offset+uncompressed_size)` contains the
file's `archive_offset`; ranged-`GET` `[compressed_offset, compressed_size)`;
**verify the fetched bytes against the frame's `checksum` before decoding**;
zstd-decode that one frame; slice at `archive_offset − uncompressed_offset` for
the file's `size` (or `length` for a split part). Frames tile the object
contiguously, each begins with the zstd magic `0x28 0xB5 0x2F 0xFD`, and each
`checksum` matches its bytes — all three of which `cargoship verify --deep`
checks. Readers without frame support decode the whole (concatenated-frame)
stream exactly as before.

## Content-integrity contract (for readers and mount layers)

A reader — including a random-access mount layer such as
[lith](https://github.com/scttfrdmn/lith) — should verify bytes against the
manifest's strong hashes rather than a weak witness like an S3 ETag (a multipart
ETag is not a content hash). The manifest records SHA-256 (hex; the algorithm is
`checksum_algorithm`) at three granularities, all optional:

| Granularity | Field | Attests | Use for |
|---|---|---|---|
| Whole file | `files[].checksum` | the file's full content | verifying a reassembled file |
| Whole object | `chunks[].checksum` | the entire chunk object | `verify --deep`, full-object fetches |
| Per frame | `chunks[].frames[].checksum` | one frame's compressed bytes | **incremental verification of a ranged fetch** |

Per-frame checksums are the granularity a streaming/mount reader wants: fetch a
frame's byte range, verify it against `frames[].checksum`, then decode — so a
changed or hostile endpoint is caught at the range level without downloading the
whole object.

**Fallbacks.** These fields are additive and may be absent:
- A chunk without `frames` (a single-frame chunk, or any pre-2.1 archive) has no
  per-frame hashes; verify at whole-file or whole-object granularity instead.
- An archive written before per-file checksums existed may lack `files[].checksum`
  (and `checksum_algorithm`); `verify --deep` reports such data as *unverifiable*
  rather than assuming it. A reader must decide its own policy for that case.

## Manifest compression (separate concern)

The manifest object itself is compressed with **gzip**, not zstd, when stored as
`manifest.json.gz`. This is independent of chunk compression. Decompress the
manifest with `gzip`/`zcat`. See
[Archive layout](/reference/format/archive-layout#the-manifest-object).

## Reader guidance

- Branch on the chunk key extension to pick a decoder; keep a raw-tar fallback.
- Do not assume a uniform level across chunks — each chunk was compressed at the
  level appropriate to its own content.
- Treat `compression_ratio` as an upload-wide statistic, not a per-chunk value;
  per-chunk sizes are in each `ChunkEntry` (`uncompressed_size` /
  `compressed_size`).
