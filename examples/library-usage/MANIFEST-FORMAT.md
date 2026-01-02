# CargoShip Manifest Format Specification

**Version:** 1.0
**Status:** Stable
**Last Updated:** 2026-01-02

## Overview

CargoShip manifests are JSON files that contain complete metadata about uploaded archives. They enable:
- **File lookup**: Find which chunk contains any file (O(1) with index)
- **Chunk discovery**: Locate chunks in S3
- **Metadata access**: File sizes, timestamps, compression info
- **Integrity verification**: Checksums and validation

## Use Cases

- **ObjectFS**: Mount CargoShip archives as POSIX filesystems
- **Selective extraction**: Extract specific files without downloading entire archive
- **Inventory management**: Track archived data without scanning S3
- **Cost analysis**: Understand storage costs and compression efficiency

## Manifest Structure

```json
{
  "version": "1.0",
  "upload_id": "20260102-abc123def",
  "created_at": "2026-01-02T12:00:00Z",
  "completed_at": "2026-01-02T12:15:30Z",
  "source_path": "/data/archive",
  "hostname": "backup-server-01",

  "bucket": "my-bucket",
  "prefix": "uploads/20260102-abc123def",
  "region": "us-east-1",

  "total_files": 10000,
  "total_bytes": 107374182400,
  "total_chunks": 100,
  "shard_count": 8,

  "compression_type": "zstd",
  "compression_level": 3,
  "compression_ratio": 0.35,

  "encryption": {
    "enabled": true,
    "data_kms_key_id": "arn:aws:kms:us-east-1:123456789012:key/abc-def",
    "manifest_encrypted": false
  },

  "files": [...],
  "chunks": [...],
  "shards": [...]
}
```

## Top-Level Fields

### Metadata

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | string | Yes | Manifest format version (currently "1.0") |
| `upload_id` | string | Yes | Unique upload session ID (timestamp-random) |
| `created_at` | timestamp | Yes | When upload started (ISO 8601) |
| `completed_at` | timestamp | Yes | When upload finished (ISO 8601) |
| `source_path` | string | Yes | Original local path that was uploaded |
| `hostname` | string | Yes | Machine that performed the upload |
| `previous_manifest_id` | string | No | For incremental syncs (Issue #148) |
| `sync_type` | string | No | "full" or "incremental" |

### S3 Location

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucket` | string | Yes | S3 bucket name |
| `prefix` | string | Yes | S3 prefix (key prefix) for all chunks |
| `region` | string | Yes | AWS region |

### Statistics

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `total_files` | integer | Yes | Total number of files in archive |
| `total_bytes` | integer | Yes | Total uncompressed size (bytes) |
| `total_chunks` | integer | Yes | Total number of chunks (tar.zst files) |
| `shard_count` | integer | Yes | Number of S3 prefix shards (1-32) |

### Compression

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `compression_type` | string | Yes | Compression algorithm ("zstd", "gzip", "none") |
| `compression_level` | integer | Yes | Compression level used (zstd: 1-22) |
| `compression_ratio` | float | Yes | Actual compression ratio achieved (0.0-1.0) |

### Encryption

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `encryption` | object | No | Encryption configuration (Issue #163) |
| `encryption.enabled` | boolean | No | Whether encryption is active |
| `encryption.data_kms_key_id` | string | No | KMS key ID/ARN for data chunks |
| `encryption.manifest_encrypted` | boolean | No | Whether manifest itself is encrypted |
| `encryption.manifest_kms_key_id` | string | No | KMS key ID for manifest encryption |
| `encryption.algorithm` | string | No | Encryption algorithm (e.g., "AES-256-GCM") |
| `encryption.encrypted_dek` | string | No | Base64-encoded encrypted data encryption key |

## FileEntry Structure

```json
{
  "path": "src/main.go",
  "size": 1024,
  "mod_time": "2026-01-01T10:00:00Z",
  "chunk_id": 0,
  "shard_id": 0,
  "s3_key": "uploads/20260102-abc123def/shard-0/chunk-000.tar.zst",
  "checksum": "sha256:abc123...",
  "metadata": {
    "magika_type": "golang"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Relative path from source root |
| `size` | integer | Yes | File size in bytes |
| `mod_time` | timestamp | Yes | Last modification time (ISO 8601) |
| `chunk_id` | integer | Yes | Which chunk contains this file |
| `shard_id` | integer | Yes | Which shard the chunk is in |
| `s3_key` | string | Yes | Full S3 key for the chunk |
| `offset` | integer | No | Start offset for partial files (file splitting) |
| `length` | integer | No | Length of this part (file splitting) |
| `part_index` | integer | No | Part index for split files |
| `total_parts` | integer | No | Total parts if split |
| `checksum` | string | No | SHA256 checksum (optional) |
| `metadata` | object | No | Additional metadata (e.g., Magika type) |

## ChunkEntry Structure

```json
{
  "id": 0,
  "shard_id": 0,
  "s3_key": "uploads/20260102-abc123def/shard-0/chunk-000.tar.zst",
  "file_count": 100,
  "file_paths": ["src/main.go", "src/util.go", ...],
  "uncompressed_size": 10485760,
  "compressed_size": 3670016,
  "created_at": "2026-01-02T12:05:00Z",
  "uploaded_at": "2026-01-02T12:05:30Z",
  "checksum": "sha256:def456..."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | integer | Yes | Chunk ID (0-indexed) |
| `shard_id` | integer | Yes | Which shard this chunk belongs to |
| `s3_key` | string | Yes | Full S3 key for download |
| `file_count` | integer | Yes | Number of files in this chunk |
| `file_paths` | array | Yes | Array of file paths in this chunk (for quick lookup) |
| `uncompressed_size` | integer | Yes | Total uncompressed size (bytes) |
| `compressed_size` | integer | Yes | Actual compressed size in S3 (bytes) |
| `created_at` | timestamp | Yes | When chunk was created |
| `uploaded_at` | timestamp | Yes | When upload to S3 completed |
| `checksum` | string | No | SHA256 of compressed archive |

## ShardEntry Structure

```json
{
  "id": 0,
  "prefix": "uploads/20260102-abc123def/shard-0",
  "chunk_count": 13,
  "file_count": 1250,
  "uncompressed_size": 13421772800,
  "compressed_size": 4697620480,
  "chunk_keys": [
    "uploads/20260102-abc123def/shard-0/chunk-000.tar.zst",
    "uploads/20260102-abc123def/shard-0/chunk-001.tar.zst",
    ...
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | integer | Yes | Shard ID (0-indexed) |
| `prefix` | string | Yes | S3 prefix path for this shard |
| `chunk_count` | integer | Yes | Number of chunks in this shard |
| `file_count` | integer | Yes | Total files across all chunks in shard |
| `uncompressed_size` | integer | Yes | Total uncompressed size (bytes) |
| `compressed_size` | integer | Yes | Total compressed size (bytes) |
| `chunk_keys` | array | Yes | All chunk S3 keys in this shard |

## File Lookup Algorithm

To find a file in a CargoShip archive:

```python
def find_file(manifest, file_path):
    # 1. Find the file entry (O(1) with hash map index)
    file_entry = manifest.file_index[file_path]
    if not file_entry:
        return None

    # 2. Get the chunk containing this file
    chunk = manifest.chunks[file_entry.chunk_id]

    # 3. Download chunk from S3
    s3_key = chunk.s3_key
    local_chunk = download_from_s3(manifest.bucket, s3_key)

    # 4. Decompress chunk
    decompressed = decompress(local_chunk, manifest.compression_type)

    # 5. Extract file from tar
    extracted_file = extract_from_tar(decompressed, file_entry.path)

    return extracted_file
```

## Validation Checks

A valid manifest must satisfy:

1. **Version**: Valid version string ("1.0")
2. **Required fields**: All required top-level fields present
3. **Statistics consistency**:
   - `total_files` == len(files)
   - `total_chunks` == len(chunks)
   - `shard_count` == len(shards)
4. **Reference integrity**:
   - All `file.chunk_id` references exist in `chunks`
   - All `chunk.shard_id` references exist in `shards`
5. **Size calculations**:
   - `total_bytes` == sum(file.size for file in files)
   - `compression_ratio` in range [0.0, 1.0]
6. **Uniqueness**:
   - No duplicate file paths
   - No duplicate chunk IDs
   - No duplicate shard IDs

## ObjectFS Integration Guide

### Mounting a CargoShip Archive

```python
class CargoShipFilesystem:
    def __init__(self, manifest_url):
        # 1. Download and parse manifest
        self.manifest = self.download_manifest(manifest_url)

        # 2. Validate manifest
        self.validate(self.manifest)

        # 3. Build file index for O(1) lookups
        self.file_index = {f.path: f for f in self.manifest.files}

        # 4. Initialize chunk cache
        self.chunk_cache = LRUCache(max_size_gb=10)

    def read_file(self, path):
        # O(1) file lookup
        file_entry = self.file_index.get(path)
        if not file_entry:
            raise FileNotFoundError(path)

        # Get or download chunk
        chunk = self.get_cached_chunk(file_entry.chunk_id)

        # Extract file from chunk
        return self.extract_file(chunk, file_entry.path)

    def get_cached_chunk(self, chunk_id):
        # Check cache first
        if chunk_id in self.chunk_cache:
            return self.chunk_cache[chunk_id]

        # Cache miss: download and decompress
        chunk_entry = self.manifest.chunks[chunk_id]
        compressed = self.download_from_s3(chunk_entry.s3_key)
        decompressed = self.decompress(compressed)

        # Cache for future access
        self.chunk_cache[chunk_id] = decompressed
        return decompressed
```

### Performance Characteristics

| Operation | Without Cache | With Cache | Notes |
|-----------|---------------|------------|-------|
| File lookup | O(1) | O(1) | Hash map index |
| First file read | ~2-5s | ~2-5s | Download + decompress chunk |
| Subsequent reads (same chunk) | ~2-5s | <10ms | Cached chunk |
| Directory listing | O(n) | O(n) | Scan file index |
| Stat operations | O(1) | O(1) | Metadata in manifest |

### Caching Strategy

Recommended cache structure:
```
/var/cache/objectfs/<upload-id>/
├── manifest.json (always cached)
├── chunks/
│   ├── chunk-000.tar.zst (compressed, optional)
│   ├── chunk-000.tar (decompressed, cached)
│   ├── chunk-001.tar (decompressed, cached)
│   └── ...
```

**Cache eviction**: LRU based on chunk access time
**Cache size**: Configurable (default: 10GB decompressed)

## Version History

### Version 1.0 (Current)
- Initial stable format
- File splitting support (Phase 5)
- Encryption metadata (Issue #163)
- Incremental sync support (Issue #148)
- Magika file type detection metadata (Issue #30)

## See Also

- [Example 1: Reading Manifests](01-simple-upload/)
- [Example 2: File Lookup](02-compression/)
- [Example 3: File Extraction](03-manifest/)
- [Example 4: S3 Download](04-multiregion/)
- [Example 5: Validation](05-progress/)
- [CargoShip Documentation](https://github.com/scttfrdmn/cargoship)
