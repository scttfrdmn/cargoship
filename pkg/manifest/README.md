# CargoShip Manifest System

The CargoShip manifest system provides comprehensive metadata tracking for uploaded datasets, enabling fast file listing and querying without downloading actual data.

## Overview

A manifest is automatically generated during each `cargoship create upload` operation and contains:
- Complete file inventory with paths, sizes, and locations
- Chunk (archive) metadata with compression statistics
- Shard distribution information
- Upload metadata (timestamp, source, destination)

## Key Features

- **Fast File Queries**: List and filter files without downloading archives
- **Glob Pattern Matching**: Find files using wildcards (`*.txt`, `data/*.csv`)
- **Shard-Based Filtering**: Query files by shard ID
- **Compression Statistics**: Track compression ratios and sizes
- **Version Tracking**: Manifest format versioning for backward compatibility

## Manifest Structure

```json
{
  "version": "1.0",
  "upload_id": "1732123456-abc123",
  "created_at": "2025-11-26T10:00:00Z",
  "completed_at": "2025-11-26T10:05:30Z",

  "source_path": "/data/uploads",
  "hostname": "upload-server-01",

  "bucket": "my-bucket",
  "prefix": "production",
  "region": "us-west-2",

  "total_files": 1000,
  "total_bytes": 500000000,
  "total_chunks": 50,
  "shard_count": 8,

  "compression_type": "zstd",
  "compression_level": 3,
  "compression_ratio": 0.42,

  "files": [...],
  "chunks": [...],
  "shards": [...]
}
```

## Storage Location

Manifests are stored in S3 at:
```
s3://bucket/prefix/uploads/{upload-id}/manifest.json.gz
```

Compressed with gzip for efficient storage and transfer.

## Usage Examples

### Building a Manifest (Pipeline Integration)

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"

// Create builder
builder, err := manifest.NewBuilder(
    uploadID,    // "1732123456-abc123"
    sourcePath,  // "/data/uploads"
    bucket,      // "my-bucket"
    prefix,      // "production"
    region,      // "us-west-2"
)

// Configure shards
builder.SetShardCount(8)

// Add files as they're processed
builder.AddFile(manifest.FileEntry{
    Path:    "data/file1.txt",
    Size:    1024,
    ModTime: time.Now(),
    ChunkID: 0,
    ShardID: 0,
    S3Key:   "production/uploads/.../chunk-0.tar.zst",
})

// Add chunks as they're uploaded
builder.AddChunk(manifest.ChunkEntry{
    ID:               0,
    ShardID:          0,
    S3Key:            "production/uploads/.../chunk-0.tar.zst",
    FileCount:        25,
    UncompressedSize: 256000,
    CompressedSize:   107520,
    CreatedAt:        time.Now(),
    UploadedAt:       time.Now(),
})

// Update shard statistics
builder.UpdateShardStats(
    shardID,         // 0
    chunkKey,        // S3 key
    fileCount,       // 25
    uncompressedSize, // 256000
    compressedSize,  // 107520
)

// Finalize and upload
manifest := builder.Finalize()
err = manifest.UploadToS3(ctx, s3Client, true) // true = compress
```

### Querying a Manifest

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"

// Download from S3
manifest, err := manifest.DownloadFromS3(
    ctx,
    s3Client,
    "my-bucket",
    "production",
    "1732123456-abc123", // upload ID
)

// Create query interface
query := manifest.NewManifestQuery(manifest)

// Find specific file
file := query.FindFile("data/report.csv")
if file != nil {
    fmt.Printf("Found: %s (size: %d, shard: %d, chunk: %d)\n",
        file.Path, file.Size, file.ShardID, file.ChunkID)
}

// List all files
allFiles := query.ListFiles("")
fmt.Printf("Total files: %d\n", len(allFiles))

// Filter by glob pattern
txtFiles := query.ListFiles("*.txt")
csvFiles := query.ListFiles("data/*.csv")
logFiles := query.ListFiles("logs/2025-11-*/*.log")

// Query by shard
shard0Files := query.FilesInShard(0)

// Query by chunk
chunk5Files := query.FilesInChunk(5)

// Get summary
summary := query.GetSummary()
fmt.Printf("Upload: %s\n", summary.UploadID)
fmt.Printf("Files: %d (%d bytes)\n", summary.TotalFiles, summary.TotalBytes)
fmt.Printf("Chunks: %d across %d shards\n", summary.TotalChunks, summary.ShardCount)
fmt.Printf("Compression: %.2f%%\n", summary.CompressionRatio*100)
```

### Glob Pattern Syntax

The `ListFiles()` function uses `filepath.Match` syntax:

| Pattern | Description | Example |
|---------|-------------|---------|
| `*` | Matches any sequence (not `/`) | `*.txt` matches `file.txt` |
| `?` | Matches single character | `file?.txt` matches `file1.txt` |
| `[abc]` | Matches any char in set | `file[123].txt` matches `file1.txt` |
| `dir/*.csv` | Match in directory | `data/*.csv` matches `data/report.csv` |

**Note**: Patterns match against both full path and basename:
- `*.txt` matches `file.txt` AND `dir/file.txt`
- `data/*.csv` only matches full path `data/report.csv`

### Serialization

```go
// To JSON (pretty-printed)
jsonData, err := manifest.ToJSON()

// To compressed JSON (gzip)
gzData, err := manifest.ToJSONCompressed()

// From JSON
manifest, err := manifest.FromJSON(jsonData)

// From compressed JSON
manifest, err := manifest.FromJSONCompressed(gzData)
```

### URL Parsing

```go
// Parse S3 URL
bucket, prefix, err := manifest.ParseS3URL("s3://my-bucket/production/data")
// bucket = "my-bucket"
// prefix = "production/data"

bucket, prefix, err := manifest.ParseS3URL("s3://my-bucket")
// bucket = "my-bucket"
// prefix = ""
```

## File Entry Fields

```go
type FileEntry struct {
    // File information
    Path    string    // Relative path from source root
    Size    int64     // File size in bytes
    ModTime time.Time // Last modification time

    // Location in archive
    ChunkID int    // Which chunk contains this file
    ShardID int    // Which shard the chunk is in
    S3Key   string // Full S3 key for the chunk

    // Optional: File splitting (Phase 5)
    Offset     int64 // Start offset for partial files
    Length     int64 // Length of this part
    PartIndex  int   // Part index for split files
    TotalParts int   // Total parts if split

    // Optional metadata
    Checksum string            // SHA256 checksum
    Metadata map[string]string // Additional metadata
}
```

## Chunk Entry Fields

```go
type ChunkEntry struct {
    // Identification
    ID      int    // Chunk ID
    ShardID int    // Shard this chunk belongs to
    S3Key   string // Full S3 key

    // Contents
    FileCount        int      // Number of files
    FilePaths        []string // File paths (for quick lookup)
    UncompressedSize int64    // Total uncompressed size
    CompressedSize   int64    // Actual compressed size in S3

    // Timestamps
    CreatedAt  time.Time // When chunk was created
    UploadedAt time.Time // When upload completed

    // Checksums
    Checksum string // SHA256 of compressed archive
}
```

## Shard Entry Fields

```go
type ShardEntry struct {
    // Identification
    ID     int    // Shard ID (0-7 for default 8 shards)
    Prefix string // S3 prefix path

    // Statistics
    ChunkCount       int   // Number of chunks
    FileCount        int64 // Total files
    UncompressedSize int64 // Total uncompressed size
    CompressedSize   int64 // Total compressed size

    // S3 keys
    ChunkKeys []string // All chunk S3 keys in this shard
}
```

## Integration with Pipeline

The manifest is automatically built during pipeline execution:

1. **Scanner Stage**: Files discovered
2. **Chunking Stage**: Files assigned to chunks
3. **Archiver Stage**: Files added to manifest with metadata
4. **S3 Uploader Stage**: Chunks uploaded, shard stats updated
5. **Pipeline Complete**: Manifest finalized and uploaded to S3

Enable manifest generation in pipeline config:
```go
config := &PipelineConfig{
    EnableManifest: true,
    // ... other config ...
}
```

The manifest is uploaded to:
```
s3://{bucket}/{prefix}/uploads/{upload-id}/manifest.json.gz
```

## Testing

### Unit Tests

```bash
# Run manifest package tests
go test ./pkg/manifest -v

# 10 tests covering:
# - Query interface creation
# - Exact file lookup
# - Glob pattern matching
# - Shard-based filtering
# - Chunk-based filtering
# - Summary generation
# - Invalid pattern handling
```

### Integration Tests

```bash
# Run with real AWS S3 (requires AWS credentials)
export AWS_PROFILE=aws
export AWS_REGION=us-west-2
export CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1
export CARGOSHIP_TEST_BUCKET=cargoship-pipeline-test

go test -v -tags=integration ./pkg/pipeline -run TestManifestIntegration

# Tests cover:
# - Manifest generation during upload
# - Manifest upload to S3
# - Manifest download from S3
# - Manifest query API
# - Shard and chunk statistics
```

## CLI Usage (Coming in v0.6.0)

```bash
# List all files in dataset
cargoship list s3://bucket/prefix/dataset

# Filter by glob pattern
cargoship list s3://bucket/prefix/dataset --pattern "*.txt"

# Show verbose information
cargoship list s3://bucket/prefix/dataset --verbose

# Filter by shard
cargoship list s3://bucket/prefix/dataset --shard-id 0
```

See [Issue #97](https://github.com/scttfrdmn/cargoship/issues/97) for CLI implementation status.

## Performance Characteristics

- **Manifest Size**: ~100KB compressed for 10,000 files
- **Query Speed**: Instant (no S3 downloads required)
- **Upload Overhead**: <1% of total upload time
- **Download Time**: <100ms for typical manifests

## Version Compatibility

Current manifest version: **1.0**

The manifest format follows semantic versioning:
- **Major version** changes require migration
- **Minor version** additions are backward compatible
- **Patch version** fixes are transparent

## Future Enhancements

- [ ] **Recursive glob patterns** (e.g., `**/*.txt`) for deep directory matching
- [ ] **Checksum validation** for data integrity verification
- [ ] **Metadata indexing** for custom file attributes
- [ ] **Compression statistics** per file type
- [ ] **Restore planning** using manifest for selective downloads

## Related Issues

- **Issue #97**: CLI list command with manifest
- **Issue #127**: Glob pattern matching implementation
- **Issue #16**: Manifest query API design

## References

- Source: `pkg/manifest/`
- Tests: `pkg/manifest/types_test.go`
- Integration: `pkg/pipeline/manifest_integration_test.go`
- Pipeline: `pkg/pipeline/pipeline.go` (manifest building)
