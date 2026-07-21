# Split files

A single file that is too large to fit in one chunk is split into **parts**,
each stored in a (potentially different) chunk. CargoShip records the split both
in the tar container — via a naming convention and `CARGOSHIP.*` PAX records —
and in the manifest, so a reader can reassemble the original file exactly.

## When a file is split

A file is split when it exceeds the chunk boundary. Each part carries a byte
range of the original file. A file that is **not** split has `TotalParts` of `0`
or `1` and `PartIndex` `0`; readers should treat both `0` and `1` as "not split."

## tar entry naming

Each part is stored as a distinct tar entry whose name is the original path with
a `.part{N}` suffix, where `N` is the zero-based part index:

```
data/bigfile.bin.part0
data/bigfile.bin.part1
data/bigfile.bin.part2
```

A non-split file keeps its plain path (`data/bigfile.bin`) with no suffix.

## `CARGOSHIP.*` PAX records

Alongside the `.part{N}` name, each split part's tar header carries PAX extended
records that make the split self-describing even without the manifest:

| PAX record | Value | Meaning |
|------------|-------|---------|
| `CARGOSHIP.part_index` | integer | Zero-based index of this part. |
| `CARGOSHIP.total_parts` | integer | Total number of parts for the original file. |
| `CARGOSHIP.offset` | integer | Byte offset of this part within the original file. |
| `CARGOSHIP.original_path` | string | The original (un-suffixed) file path. |

These are only written when `TotalParts > 1`. As produced by the archiver:

```go
if file.TotalParts > 1 {
	header.Name = fmt.Sprintf("%s.part%d", file.Path, file.PartIndex)

	if header.PAXRecords == nil {
		header.PAXRecords = make(map[string]string)
	}
	header.PAXRecords["CARGOSHIP.part_index"]   = fmt.Sprintf("%d", file.PartIndex)
	header.PAXRecords["CARGOSHIP.total_parts"]  = fmt.Sprintf("%d", file.TotalParts)
	header.PAXRecords["CARGOSHIP.offset"]       = fmt.Sprintf("%d", file.Offset)
	header.PAXRecords["CARGOSHIP.original_path"] = file.Path
} else {
	header.Name = file.Path
}
```

::: info PAX records are standard tar
`CARGOSHIP.*` records use the standard PAX extended-header mechanism, so generic
tar tooling preserves and can display them. The `.part{N}` name is what a plain
`tar -x` writes to disk; the PAX records tell an aware reader how to reassemble.
:::

## Manifest fields for split files

Each part is a separate `FileEntry` in the manifest, sharing the same `Path` but
carrying its own byte range and part index (from
[`FileEntry`](/reference/format/manifest#fileentry)):

```go
// File splitting information (Phase 5)
Offset     int64 `json:"offset,omitempty"`      // Start offset for partial files (0 = full file)
Length     int64 `json:"length,omitempty"`      // Length of this part (0 = full file)
PartIndex  int   `json:"part_index,omitempty"`  // Part index for split files (0 = not split)
TotalParts int   `json:"total_parts,omitempty"` // Total parts if split (0 or 1 = not split)
```

- `Path` is the **original** path (no `.part{N}` suffix) — parts of one file
  share it.
- `Offset` is the part's start offset in the original file; `Length` is the
  part's byte length.
- Each part's `ChunkID` / `ShardID` / `S3Key` locate the chunk that holds it —
  different parts may live in different chunks and shards.

Example: two manifest entries for one split file:

```json
[
  {
    "path": "data/bigfile.bin",
    "size": 8589934592,
    "chunk_id": 4,
    "shard_id": 4,
    "s3_key": "uploads/20260721-abc/shard-4/chunk-4.tar.zst",
    "offset": 0,
    "length": 4294967296,
    "part_index": 0,
    "total_parts": 2
  },
  {
    "path": "data/bigfile.bin",
    "size": 8589934592,
    "chunk_id": 5,
    "shard_id": 5,
    "s3_key": "uploads/20260721-abc/shard-5/chunk-5.tar.zst",
    "offset": 4294967296,
    "length": 4294967296,
    "part_index": 1,
    "total_parts": 2
  }
]
```

::: tip `size` is the whole-file size on every part
Each part entry reports the original file's total `size`. Use `length` for the
size of the individual part, and `offset` for where it belongs.
:::

## Reassembly algorithm

To rebuild a split file, a reader should:

1. **Collect all parts.** Group manifest `FileEntry` records by `path` where
   `total_parts > 1` (or scan tar entries whose name matches `{path}.part{N}` /
   whose `CARGOSHIP.original_path` equals the target).
2. **Order by `part_index`** (equivalently, by `offset`).
3. **Fetch and extract each part** from its chunk (each part is one tar entry).
4. **Write parts in offset order** into the output file — write part `i`'s bytes
   at its `offset`. Because offsets are contiguous and start at 0, appending in
   `part_index` order also reconstructs the file exactly.
5. **Verify** the reassembled file. When present, check against the whole-file
   integrity signals (`Checksum` = SHA256); note `ContentHash` is MD5 for DVC
   only and is not an integrity guarantee.

A part-count check (`parts collected == total_parts`) guards against missing
chunks before you commit the reassembled output.

## Standard-tooling caveat

Extracting a split-file archive with a plain `tar -x` produces the individual
`bigfile.bin.part0`, `bigfile.bin.part1`, … files on disk — not the reassembled
original. To rebuild it manually, concatenate the parts in index order:

```bash
cat data/bigfile.bin.part0 data/bigfile.bin.part1 > data/bigfile.bin
```

An aware reader uses the `CARGOSHIP.*` PAX records (or the manifest) to do this
automatically. See [Reading archives](/reference/format/library-api).
