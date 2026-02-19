# CargoShip Architecture

CargoShip uses a streaming pipeline architecture optimized for large-scale data transfers to AWS S3. The design prioritizes zero local disk usage, bounded memory consumption, and high throughput through parallel operations.

## Four-Stage Streaming Pipeline

```
Scanner → Chunker → Archiver → S3 Uploader
```

Data flows through `io.Pipe` connections between stages, ensuring zero disk I/O and bounded memory usage proportional to `chunk_size × workers`.

### Stage 1: Scanner (`pkg/pipeline/`)

Discovers files via `filepath.Walk` and emits file metadata downstream. Supports:

- File filtering by pattern, size, age, and content type
- Optional AI-powered file type detection via Magika (`pkg/detection/`)
- Metadata collection for compression hints
- Batch processing for efficiency (100 files/batch with Magika)

### Stage 2: Chunker (`pkg/chunking/`)

Intelligently partitions the file stream into chunks for parallel upload:

- Content-aware sizing based on compression estimation
- Archive padding awareness to minimize wasted space
- Configurable chunk sizes (default: 5GB per chunk)
- Adaptive shard count based on workload characteristics

### Stage 3: Archiver

Compresses and packages chunks into tar archives:

- Compression algorithms: zstd (default), lz4, gzip, none
- Content-aware compression levels via Magika type detection:
  - Code files: level 9 (best compression)
  - Documents: level 6
  - Images/video/archives: level 1 or none
- Stream-based: no temporary files written to disk

### Stage 4: S3 Uploader (`pkg/aws/s3/`)

Handles multi-part uploads to AWS S3:

- Multi-prefix sharding for 8× request rate capacity
- Adaptive shard count: 4–32 shards auto-tuned to workload
- Parallel uploads with configurable worker count
- Automatic retry with exponential backoff
- Manifest generation for retrieval and inventory

## Key Packages

| Package | Purpose |
|---------|---------|
| `pkg/pipeline/` | Pipeline orchestration, stage coordination |
| `pkg/chunking/` | Content-aware chunking and sizing |
| `pkg/aws/s3/` | S3 multi-part upload, sharding, retry |
| `pkg/manifest/` | Upload manifests for retrieval |
| `pkg/multiregion/` | Multi-region load balancing and failover |
| `pkg/detection/` | Magika AI file type detection integration |
| `pkg/aws/cost/` | Budget tracking and ML cost forecasting |
| `pkg/compression/` | Compression algorithm selection |

## Multi-Prefix S3 Sharding

S3 partitions requests by key prefix. CargoShip distributes uploads across multiple prefixes to multiply effective request rate:

```
s3://bucket/prefix-0/<chunk>
s3://bucket/prefix-1/<chunk>
...
s3://bucket/prefix-N/<chunk>
```

The shard count is automatically tuned (Issue #106) based on:
- File count (1 shard per 10k files)
- Compressed data size (1 shard per 10 GB)
- Available CPU cores (1 shard per 2 cores)
- Minimum 6 chunks per shard for load balancing

## Multi-Region Load Balancing (`pkg/multiregion/`)

For cross-region uploads, CargoShip supports:

- Health checking across configured regions
- Weighted load balancing
- Automatic failover on region errors
- Latency-based routing

## Memory Model

Memory usage is bounded: `O(chunk_size × workers)`. For the default configuration (5 GB chunks, 4 workers), peak memory is approximately 20 GB. Reducing `--chunk-size` or `--max-concurrency` directly reduces memory usage.

## Manifest System (`pkg/manifest/`)

Every upload generates a manifest recording:

- File paths and S3 keys
- Chunk boundaries and checksums
- Compression metadata
- Upload timestamps and region

Manifests enable selective file restoration without downloading entire archives.

## CLI Entry Point (`cmd/cargoship/`)

The CLI uses [cobra](https://github.com/spf13/cobra) for command structure. Core commands:

```bash
cargoship upload <path> s3://<bucket>/<prefix>   # Upload data
cargoship estimate <path>                         # Estimate costs
cargoship survey <path>                           # Analyze data
cargoship budget set                              # Configure budgets
cargoship manifest list                           # List manifests
```

See [CLI Reference](CLI_REFERENCE.md) for complete command documentation.
