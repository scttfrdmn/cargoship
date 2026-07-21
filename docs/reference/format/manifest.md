# Manifest schema

The manifest is a single JSON document describing an entire upload: every file,
which chunk and shard holds it, the chunks themselves, per-shard rollups, and
optional metadata blocks for encryption, deduplication, and DVC/Git provenance.
It is the source of truth for every read operation.

This page documents the schema by its Go source structs, exactly as serialized.
Field names, types, and JSON tags below match
`pkg/manifest/types.go` and are the authoritative reference for a manifest
reader in any language.

## Serialization

- **Encoding:** JSON, pretty-printed (`json.MarshalIndent` with two-space
  indent).
- **Compression:** optionally gzip — `manifest.json` (plain) or
  `manifest.json.gz` (gzip). Decompress `.gz` with gzip, **not** zstd.
- **Encryption:** optionally KMS-envelope-encrypted into a
  `manifest.encrypted.json[.gz]` wrapper — see
  [Encryption](/reference/format/encryption).
- **Version:** the top-level `version` is `"2.0"` for current uploads; `"1.0"`
  is read-compatible. See [format versioning](/reference/format/#format-versioning).

::: info Timestamps
All time fields serialize as RFC 3339 / ISO 8601 strings (Go `time.Time`), e.g.
`2026-07-21T12:34:56Z`.
:::

## `Manifest` (top level)

```go
type Manifest struct {
	// Version of the manifest format (for future compatibility)
	Version string `json:"version"`

	// Upload metadata
	UploadID    string    `json:"upload_id"`    // Unique upload session ID (timestamp-random)
	CreatedAt   time.Time `json:"created_at"`   // When upload started
	CompletedAt time.Time `json:"completed_at"` // When upload finished

	// Source information
	SourcePath string `json:"source_path"` // Original local path that was uploaded
	Hostname   string `json:"hostname"`    // Machine that performed the upload

	// Sync information (Issue #148)
	PreviousManifestID string `json:"previous_manifest_id,omitempty"` // For version chain in incremental syncs
	SyncType           string `json:"sync_type,omitempty"`            // "full" or "incremental" (empty = full for backwards compat)

	// S3 location
	Bucket string `json:"bucket"` // S3 bucket name
	Prefix string `json:"prefix"` // S3 prefix (key prefix)
	Region string `json:"region"` // AWS region

	// Statistics
	TotalFiles  int64 `json:"total_files"`  // Total number of files
	TotalBytes  int64 `json:"total_bytes"`  // Total uncompressed size
	TotalChunks int   `json:"total_chunks"` // Total number of chunks (archives)
	ShardCount  int   `json:"shard_count"`  // Number of S3 prefix shards

	// Compression
	CompressionType  string  `json:"compression_type"`  // "zstd", "gzip", etc.
	CompressionLevel int     `json:"compression_level"` // Compression level used
	CompressionRatio float64 `json:"compression_ratio"` // Actual compression ratio achieved

	// Encryption (Issue #163)
	Encryption *EncryptionMetadata `json:"encryption,omitempty"`

	// Deduplication (Issue #108)
	Deduplication *ManifestDeduplication `json:"deduplication,omitempty"`

	// DVC integration (Issue #172) — all omitempty for v1.0 backward compatibility
	VersionInfo      *VersionInfo      `json:"version_info,omitempty"`
	GitMetadata      *GitMetadata      `json:"git_metadata,omitempty"`
	DVCCompatibility *DVCCompatibility `json:"dvc_compatibility,omitempty"`
	DVCPipeline      *DVCPipeline      `json:"dvc_pipeline,omitempty"`

	// Collections
	Files  []FileEntry  `json:"files"`
	Chunks []ChunkEntry `json:"chunks"`
	Shards []ShardEntry `json:"shards"`
}
```

::: tip `compression_type` / `compression_level` are upload-wide
They describe the configured compression, not necessarily each chunk. A chunk of
already-compressed data can ship as a plain `.tar` even when this says `zstd`.
See [Compression](/reference/format/compression).
:::

## `FileEntry`

One entry per file (or per part, for [split files](/reference/format/split-files)).

```go
type FileEntry struct {
	// File information
	Path    string    `json:"path"`     // Relative path from source root
	Size    int64     `json:"size"`     // File size in bytes
	ModTime time.Time `json:"mod_time"` // Last modification time

	// Location in archive
	ChunkID int    `json:"chunk_id"` // Which chunk contains this file
	ShardID int    `json:"shard_id"` // Which shard the chunk is in
	S3Key   string `json:"s3_key"`   // Full S3 key for the chunk

	// File splitting information (Phase 5)
	Offset     int64 `json:"offset,omitempty"`      // Start offset for partial files (0 = full file)
	Length     int64 `json:"length,omitempty"`      // Length of this part (0 = full file)
	PartIndex  int   `json:"part_index,omitempty"`  // Part index for split files (0 = not split)
	TotalParts int   `json:"total_parts,omitempty"` // Total parts if split (0 or 1 = not split)

	// Optional metadata
	Checksum    string            `json:"checksum,omitempty"`     // SHA256 checksum (optional)
	ContentHash string            `json:"content_hash,omitempty"` // MD5 hex digest for DVC compatibility (Issue #172)
	Metadata    map[string]string `json:"metadata,omitempty"`     // Additional metadata

	// DVC provenance (Issue #172)
	DVCMetadata *DVCMetadata `json:"dvc_metadata,omitempty"`

	// Deduplication (Issue #108)
	IsDuplicate       bool   `json:"is_duplicate,omitempty"`
	DuplicateOfHash   string `json:"duplicate_of_hash,omitempty"`
	OriginalChunkID   int    `json:"original_chunk_id,omitempty"`
	OriginalShardID   int    `json:"original_shard_id,omitempty"`
	OriginalS3Key     string `json:"original_s3_key,omitempty"`
	DeduplicationRefs int32  `json:"deduplication_refs,omitempty"`
}
```

::: warning Two different hashes — know which is which
- `Checksum` is a **SHA256** and is the integrity signal for the file.
- `ContentHash` is an **MD5** hex digest, present only for DVC compatibility.
  It is **not** an integrity guarantee — do not use it for verification.
:::

### The `Metadata` map

Several attributes are carried as string keys in the `metadata` map rather than
dedicated struct fields. Notably:

| Key | Meaning |
|-----|---------|
| `magika_type` | AI-detected content type (Magika label, e.g. `python`). |
| `magika_mime` | AI-detected MIME type. |
| `magika_score` | Magika confidence score. |
| `content_hash` | MD5 content hash (also surfaced in the `ContentHash` field). |
| `atime` | Access time, when captured. |

Readers should treat this map as open-ended and ignore keys they do not use.

### Deduplication fields on a file

When deduplication is active and a file is a byte-for-byte duplicate of another,
`IsDuplicate` is `true` and the `Original*` fields point at the stored copy
(`OriginalChunkID` / `OriginalShardID` / `OriginalS3Key`) so a reader can resolve
the duplicate to its real bytes without a second copy existing in S3.

## `ChunkEntry`

One entry per chunk (one `.tar.zst` or `.tar` object).

```go
type ChunkEntry struct {
	// Chunk identification
	ID      int    `json:"id"`       // Chunk ID
	ShardID int    `json:"shard_id"` // Which shard this chunk belongs to
	S3Key   string `json:"s3_key"`   // Full S3 key

	// Contents
	FileCount        int      `json:"file_count"`        // Number of files in this chunk
	FilePaths        []string `json:"file_paths"`        // Paths of files in this chunk (for quick lookup)
	UncompressedSize int64    `json:"uncompressed_size"` // Total uncompressed size
	CompressedSize   int64    `json:"compressed_size"`   // Actual compressed size in S3

	// Timestamps
	CreatedAt  time.Time `json:"created_at"`  // When chunk was created
	UploadedAt time.Time `json:"uploaded_at"` // When upload to S3 completed

	// Checksums
	Checksum string `json:"checksum,omitempty"` // SHA256 of compressed archive
}
```

`Checksum` here is the **SHA256 of the compressed archive object** — verify a
downloaded chunk against it before extracting.

## `ShardEntry`

One entry per shard (S3 prefix), with rollup statistics.

```go
type ShardEntry struct {
	// Shard identification
	ID     int    `json:"id"`     // Shard ID (0-based)
	Prefix string `json:"prefix"` // S3 prefix path (e.g. "uploads/20260721-abc123/shard-0")

	// Statistics
	ChunkCount       int   `json:"chunk_count"`
	FileCount        int64 `json:"file_count"`
	UncompressedSize int64 `json:"uncompressed_size"`
	CompressedSize   int64 `json:"compressed_size"`

	// S3 keys
	ChunkKeys []string `json:"chunk_keys"` // All chunk S3 keys in this shard
}
```

## Provenance blocks (v2.0, optional)

These pointer fields are `omitempty` and appear only when the corresponding
feature was used. A reader must tolerate their absence.

### `VersionInfo`

```go
type VersionInfo struct {
	DataVersion  string `json:"data_version,omitempty"`  // e.g. "v1.2.0"
	ExperimentID string `json:"experiment_id,omitempty"` // DVC experiment ID
	Tag          string `json:"tag,omitempty"`           // free-form grouping label
}
```

### `GitMetadata`

```go
type GitMetadata struct {
	Commit string `json:"commit,omitempty"` // HEAD commit SHA (full 40-char hex)
	Branch string `json:"branch,omitempty"` // current branch (empty in detached HEAD)
	Tag    string `json:"tag,omitempty"`    // nearest annotated tag (git describe --tags)
	Remote string `json:"remote,omitempty"` // fetch URL of "origin"
	Dirty  bool   `json:"dirty,omitempty"`  // true if working tree had uncommitted changes
}
```

### `DVCCompatibility`

```go
type DVCCompatibility struct {
	Enabled           bool   `json:"enabled"`
	DVCVersion        string `json:"dvc_version,omitempty"`         // e.g. "3.51.2"
	CacheDir          string `json:"cache_dir,omitempty"`           // resolved DVC cache dir
	DVCFilesGenerated bool   `json:"dvc_files_generated,omitempty"` // .dvc sidecars written
}
```

### `DVCMetadata` (per-file)

Attached to a `FileEntry` when the file is a tracked output of a DVC stage.

```go
type DVCMetadata struct {
	Stage        string `json:"stage,omitempty"`         // DVC stage that produced this file
	Pipeline     string `json:"pipeline,omitempty"`      // pipeline name from dvc.yaml
	ExperimentID string `json:"experiment_id,omitempty"` // DVC experiment run
}
```

### `DVCPipeline` and its parts

Pipeline provenance for one named stage, extracted from `dvc.yaml` / `dvc.lock`.

```go
type DVCPipeline struct {
	StageName    string         `json:"stage_name"`
	PipelineFile string         `json:"pipeline_file,omitempty"` // path to dvc.yaml (repo-relative)
	Command      string         `json:"command,omitempty"`       // stage command from dvc.yaml
	Deps         []DVCDep       `json:"deps,omitempty"`
	Outputs      []DVCOut       `json:"outputs,omitempty"`
	Params       map[string]any `json:"params,omitempty"`     // resolved params from dvc.lock
	ExecutedAt   time.Time      `json:"executed_at,omitempty"` // mtime of dvc.lock (proxy for last run)
	LockHash     string         `json:"lock_hash,omitempty"`   // MD5 of the full dvc.lock contents
}

type DVCDep struct {
	Path string `json:"path"`           // repo-relative dependency path
	MD5  string `json:"md5,omitempty"`  // hash from dvc.lock (empty if absent)
	Size int64  `json:"size,omitempty"` // size from dvc.lock (0 if absent)
}

type DVCOut struct {
	Path string `json:"path"`           // repo-relative output path
	MD5  string `json:"md5,omitempty"`
	Size int64  `json:"size,omitempty"`
}
```

## Deduplication block

The upload-level dedup summary, present when deduplication is enabled.

```go
type ManifestDeduplication struct {
	Enabled          bool                     `json:"enabled"`
	UniqueFiles      int64                    `json:"unique_files"`
	DuplicateFiles   int64                    `json:"duplicate_files"`
	BytesSaved       int64                    `json:"bytes_saved"`
	BytesProcessed   int64                    `json:"bytes_processed"`
	DeduplicationPct float64                  `json:"deduplication_pct"` // percent of bytes saved
	FileReferences   map[string]FileReference `json:"file_references"`   // content hash -> stored location
}

type FileReference struct {
	Hash     string `json:"hash"`      // SHA-256 hash
	ShardID  int    `json:"shard_id"`
	ChunkID  int    `json:"chunk_id"`
	S3Key    string `json:"s3_key"`
	Size     int64  `json:"size"`
	RefCount int32  `json:"ref_count"` // number of references to this file
}
```

## Encryption block

The `encryption` field is an `EncryptionMetadata` describing data (S3 SSE) and
manifest (envelope) encryption. It is documented in full on the
[Encryption](/reference/format/encryption) page.

## Example manifest

A minimal, v2.0 manifest with one file and one chunk:

```json
{
  "version": "2.0",
  "upload_id": "20260721-123456-abcd1234",
  "created_at": "2026-07-21T12:34:56Z",
  "completed_at": "2026-07-21T12:40:00Z",
  "source_path": "/data/uploads",
  "hostname": "upload-server-01",
  "bucket": "my-bucket",
  "prefix": "production",
  "region": "us-west-2",
  "total_files": 1,
  "total_bytes": 1024,
  "total_chunks": 1,
  "shard_count": 8,
  "compression_type": "zstd",
  "compression_level": 9,
  "compression_ratio": 0.42,
  "files": [
    {
      "path": "src/main.go",
      "size": 1024,
      "mod_time": "2026-07-21T10:00:00Z",
      "chunk_id": 0,
      "shard_id": 0,
      "s3_key": "production/uploads/20260721-123456-abcd1234/shard-0/chunk-0.tar.zst",
      "checksum": "sha256:...",
      "content_hash": "9e107d9d372bb6826bd81d3542a419d6",
      "metadata": { "magika_type": "go" }
    }
  ],
  "chunks": [
    {
      "id": 0,
      "shard_id": 0,
      "s3_key": "production/uploads/20260721-123456-abcd1234/shard-0/chunk-0.tar.zst",
      "file_count": 1,
      "file_paths": ["src/main.go"],
      "uncompressed_size": 1024,
      "compressed_size": 430,
      "created_at": "2026-07-21T12:35:00Z",
      "uploaded_at": "2026-07-21T12:36:00Z",
      "checksum": "sha256:..."
    }
  ],
  "shards": [
    {
      "id": 0,
      "prefix": "production/uploads/20260721-123456-abcd1234/shard-0",
      "chunk_count": 1,
      "file_count": 1,
      "uncompressed_size": 1024,
      "compressed_size": 430,
      "chunk_keys": [
        "production/uploads/20260721-123456-abcd1234/shard-0/chunk-0.tar.zst"
      ]
    }
  ]
}
```

## Consistency invariants

A well-formed manifest satisfies:

- `total_files == len(files)` (counting parts of split files as entries),
  `total_chunks == len(chunks)`, `shard_count == len(shards)`.
- Every `FileEntry.chunk_id` refers to a chunk in `chunks`; every
  `chunk.shard_id` refers to a shard in `shards`.
- Each `FileEntry.s3_key` equals the `s3_key` of its chunk, and each shard's
  `chunk_keys` lists exactly the chunks with that `shard_id`.
- `compression_ratio` is in `[0.0, 1.0]`.

To consume all this from Go, see [Reading archives](/reference/format/library-api).
