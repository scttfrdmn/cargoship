# Example 4: Downloading Chunks from S3

This example demonstrates how to download CargoShip chunks from S3.

## What This Example Shows

- Downloading manifest from S3
- Downloading specific chunks from S3
- Using AWS SDK v2 for S3 operations
- Efficient chunk caching strategies

## Use Case

**ObjectFS Integration**: ObjectFS needs to download chunks on-demand when files are accessed. This example shows the pattern for:
1. Download manifest once on mount
2. Download chunks as needed
3. Cache chunks locally
4. Reuse cached chunks for subsequent file access

## Running the Example

```bash
# Download manifest and a specific chunk
go run main.go s3://bucket/prefix/manifest.json chunk-001.tar.zst ./cache/

# Downloads:
# - s3://bucket/prefix/manifest.json → ./cache/manifest.json
# - s3://bucket/prefix/shard-0/chunk-001.tar.zst → ./cache/chunk-001.tar.zst
```

## Key Concepts

### On-Demand Download

ObjectFS should download chunks lazily:
- Don't download all chunks on mount
- Download only when files are accessed
- Cache downloaded chunks for reuse

### Cache Strategy

Recommended caching approach:
```
/var/cache/objectfs/
├── manifests/
│   └── <upload-id>/manifest.json
└── chunks/
    ├── <upload-id>/
    │   ├── chunk-001.tar.zst (compressed)
    │   └── chunk-001.tar (decompressed, cached)
```

### Performance

- **Cold start** (no cache): Download + decompress
- **Warm cache**: Read from local disk (100-1000x faster)
- **Chunk granularity**: Download 100 files at once, not one-by-one

## Next Steps

See Example 5 for manifest validation and integrity checking.
