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
