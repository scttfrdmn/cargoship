# Architecture

CargoShip is built around a streaming pipeline optimized for large-scale transfers
to S3. The design goals are zero local disk usage, bounded memory, and high
throughput through parallelism. For a gentler walkthrough see
[How it works](/intro/how-it-works); this page is the technical reference.

## Four-stage streaming pipeline

```
Scanner → Chunker → Archiver → S3 Uploader
```

Data flows through `io.Pipe` connections between stages, so nothing is written to
local disk and peak memory stays proportional to `chunk_size × workers`.

### Scanner (`pkg/pipeline/`)

Discovers files via `filepath.Walk` and emits file metadata downstream. Supports
filtering by pattern, size, age, and content type; optional AI file-type
detection via [Magika](/guides/features/magika) (`pkg/detection/`, batched 100
files at a time); and metadata collection for compression hints.

### Chunker (`pkg/chunking/`)

Partitions the file stream into [chunks](/intro/concepts#chunk) for parallel
upload — content-aware sizing based on compression estimation, archive-padding
awareness, and an [adaptive shard count](/guides/features/sharding) tuned to the
workload.

### Archiver

Compresses and packages chunks into `tar` archives. Compression algorithms:
Zstandard (default), lz4, gzip, or none. When Magika detects content type, the
archiver picks a level per type — high for code, moderate for documents, minimal
or none for already-compressed images, video, and archives. Stream-based; no
temp files.

### S3 uploader (`pkg/aws/s3/`)

Handles multipart uploads with multi-prefix sharding, an adaptive shard count
(4–32), parallel workers, automatic retry with exponential backoff, and manifest
generation.

## Key packages

| Package | Purpose |
|---------|---------|
| `pkg/pipeline/` | Pipeline orchestration and stage coordination |
| `pkg/chunking/` | Content-aware chunking and sizing |
| `pkg/aws/s3/` | S3 multipart upload, sharding, retry |
| `pkg/manifest/` | Upload manifests for retrieval |
| `pkg/multiregion/` | Multi-region load balancing and failover |
| `pkg/detection/` | Magika AI file-type detection |
| `pkg/aws/cost/` | Budget tracking and cost forecasting |
| `pkg/compression/` | Compression algorithm selection |

## Multi-prefix sharding

S3 partitions request throughput by key prefix, so CargoShip distributes chunks
across multiple shard prefixes (`shard-0`, `shard-1`, …) to raise the effective
request rate. The shard count is auto-tuned by file count, compressed data size,
available CPU, and a minimum-chunks-per-shard floor for load balancing. See
[Multi-prefix sharding](/guides/features/sharding).

## Memory model

Memory usage is bounded at `O(chunk_size × workers)`. Reducing chunk size or
worker count directly lowers peak memory — useful on constrained hosts.

## Manifest system (`pkg/manifest/`)

Every upload generates a [manifest](/intro/concepts#manifest) recording file paths
and S3 keys, chunk boundaries and checksums, compression metadata, and timestamps.
The manifest is what makes selective restore possible without downloading whole
archives. See the [manifest schema](/reference/format/manifest).

## See also

- [How it works](/intro/how-it-works).
- [Archive & manifest format spec](/reference/format/).
- [Multi-region](/guides/features/multi-region).
