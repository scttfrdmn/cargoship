# CargoShip S3 Storage Format

**Version:** 1.0
**Last Updated:** 2025-12-06
**Status:** Stable

## Overview

CargoShip stores data on AWS S3 using an **open, transparent, and portable format** designed for:
- **Data Portability**: Extract your data without CargoShip
- **Third-Party Integration**: Build tools that read CargoShip archives
- **Compliance**: Auditable data format for organizational requirements
- **Transparency**: Open-source project with open data formats

This document describes the storage format for **CargoShip v0.5.0+** (Pipeline architecture). Legacy formats (Porter/rclone/suitcase) have been removed.

## Quick Reference

**Archive Format:** TAR + Zstandard (tar.zst)
**S3 Key Pattern:** `uploads/{upload-id}/shard-{N}/chunk-{M}.tar.zst`
**Manifest Format:** JSON + gzip (manifest.json.gz)
**Compression:** Zstandard (default level 3, ~2.5:1 ratio)
**Shard Count:** 8 (default, distributes load across S3 prefixes)

## Architecture

### Upload Structure

```
s3://bucket/[prefix]/
├── uploads/
│   ├── {upload-id}/
│   │   ├── shard-0/
│   │   │   ├── chunk-0.tar.zst
│   │   │   ├── chunk-8.tar.zst
│   │   │   └── ...
│   │   ├── shard-1/
│   │   │   ├── chunk-1.tar.zst
│   │   │   ├── chunk-9.tar.zst
│   │   │   └── ...
│   │   ├── shard-2/
│   │   ├── shard-3/
│   │   ├── shard-4/
│   │   ├── shard-5/
│   │   ├── shard-6/
│   │   ├── shard-7/
│   │   └── manifest.json.gz  ← Lightweight file index (~30KB for 10k files)
```

### Upload ID Format

Format: `{timestamp}-{random}`
Example: `20251206-123456-abcd1234`

- **Timestamp**: YYYYMMDDHHmmss format (local time)
- **Random**: 8-character hex string for uniqueness
- **Purpose**: Unique identifier for each upload session

### Shard Distribution

CargoShip uses **8 shards by default** to distribute load across S3 prefixes:

- **Shard Assignment**: `shardID = chunkID % shardCount` (consistent hashing)
- **S3 Performance**: Each shard is an independent S3 prefix, enabling parallel uploads
- **Request Rate**: 8 shards × 3,500 req/s = 28,000 req/s aggregate throughput
- **Rationale**: Balances parallelism with manageable shard count

Chunks are round-robin distributed across shards:
```
Chunk 0 → Shard 0
Chunk 1 → Shard 1
Chunk 2 → Shard 2
...
Chunk 7 → Shard 7
Chunk 8 → Shard 0  (wraps around)
Chunk 9 → Shard 1
```

## Archive Format

### File Structure

Each chunk is a **TAR archive** compressed with **Zstandard**:

```
chunk-N.tar.zst
├── [Zstandard compressed stream]
│   └── [TAR archive]
│       ├── file1.txt          (preserved path)
│       ├── dir/file2.log
│       ├── data/report.csv
│       └── ...
```

### Compression

**Algorithm:** Zstandard (zstd)
**Level:** 3 (default, SpeedDefault)
**Concurrency:** Auto (uses all available CPUs)
**Typical Ratio:** 2.5:1 (varies by content type)

**Why Zstandard?**
- **Fast**: 500+ MB/s compression throughput
- **Efficient**: Better compression ratio than gzip
- **Streaming**: Supports online compression/decompression
- **Industry Standard**: Wide tool support (zstd CLI, libraries)

### TAR Format

**Standard:** POSIX ustar format (Go's `archive/tar`)
**Preserved Attributes:**
- File path (relative to source directory)
- File size (exact bytes)
- Modification time (nanosecond precision)
- File mode/permissions (Unix-style, e.g., 0644)

**Not Preserved:**
- Ownership (UID/GID) - intentionally omitted for portability
- Extended attributes (xattrs)
- Access Control Lists (ACLs)
- Sparse file information

## S3 Key Naming Convention

### Pattern

```
[prefix/]uploads/{upload-id}/shard-{shard-id}/chunk-{chunk-id}.tar.zst
```

### Components

| Component | Format | Example | Description |
|-----------|--------|---------|-------------|
| `prefix` | Optional | `production/` | User-specified S3 key prefix |
| `uploads` | Fixed | `uploads/` | Fixed prefix for all CargoShip uploads |
| `upload-id` | `YYYYMMDD-HHmmss-{random}` | `20251206-123456-abcd1234` | Unique upload session identifier |
| `shard-id` | `0-7` | `shard-3` | Shard number (0-indexed) |
| `chunk-id` | `0-N` | `chunk-42` | Chunk number (globally sequential) |
| Extension | `.tar.zst` | `.tar.zst` | TAR + Zstandard compression |

### Examples

**Without prefix:**
```
s3://my-bucket/uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst
s3://my-bucket/uploads/20251206-123456-abcd1234/shard-1/chunk-1.tar.zst
```

**With prefix:**
```
s3://my-bucket/production/uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst
s3://my-bucket/staging/uploads/20251206-123456-abcd1234/shard-1/chunk-1.tar.zst
```

## S3 Object Metadata

CargoShip stores metadata as **S3 object metadata** (HTTP headers):

| Metadata Key | Example Value | Description |
|--------------|---------------|-------------|
| `cargoship-chunk-id` | `42` | Chunk ID number |
| `cargoship-file-count` | `150` | Number of files in this chunk |
| `cargoship-chunk-size` | `524288000` | Uncompressed size (bytes) |
| `cargoship-compression` | `zstd` | Compression algorithm |
| `cargoship-archive` | `tar` | Archive format |

**Access Metadata:**
```bash
aws s3api head-object \
  --bucket my-bucket \
  --key uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst \
  --query 'Metadata'
```

## Manifest Format

CargoShip generates a **manifest.json.gz** file containing complete metadata for the upload.

### Location

```
s3://bucket/[prefix/]uploads/{upload-id}/manifest.json.gz
```

### Format

**Structure:** JSON (pretty-printed)
**Compression:** gzip (for efficient storage/transfer)
**Size:** ~30KB compressed for 10,000 files

### Schema

```json
{
  "version": "1.0",
  "upload_id": "20251206-123456-abcd1234",
  "created_at": "2025-12-06T12:34:56Z",
  "completed_at": "2025-12-06T12:40:00Z",

  "source_path": "/data/uploads",
  "hostname": "upload-server-01",

  "bucket": "my-bucket",
  "prefix": "production",
  "region": "us-west-2",

  "total_files": 10000,
  "total_bytes": 5000000000,
  "total_chunks": 100,
  "shard_count": 8,

  "compression_type": "zstd",
  "compression_level": 3,
  "compression_ratio": 0.42,

  "files": [
    {
      "path": "data/file1.txt",
      "size": 1024,
      "mod_time": "2025-12-06T10:00:00Z",
      "chunk_id": 0,
      "shard_id": 0,
      "s3_key": "uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst"
    }
  ],

  "chunks": [
    {
      "id": 0,
      "shard_id": 0,
      "s3_key": "uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst",
      "file_count": 150,
      "file_paths": ["data/file1.txt", "..."],
      "uncompressed_size": 500000000,
      "compressed_size": 210000000,
      "created_at": "2025-12-06T12:35:00Z",
      "uploaded_at": "2025-12-06T12:36:00Z",
      "checksum": "sha256:..."
    }
  ],

  "shards": [
    {
      "id": 0,
      "prefix": "uploads/20251206-123456-abcd1234/shard-0",
      "chunk_count": 13,
      "file_count": 1250,
      "uncompressed_size": 625000000,
      "compressed_size": 262500000,
      "chunk_keys": [
        "uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst",
        "uploads/20251206-123456-abcd1234/shard-0/chunk-8.tar.zst"
      ]
    }
  ]
}
```

### Manifest API

CargoShip provides a Go API for querying manifests:

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"

// Download from S3
m, err := manifest.DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)

// Query files
query := manifest.NewManifestQuery(m)
allFiles := query.ListFiles("")           // All files
txtFiles := query.ListFiles("*.txt")      // Glob pattern
shard0Files := query.FilesInShard(0)      // By shard
```

See: [`pkg/manifest/README.md`](../pkg/manifest/README.md)

## Manual Data Extraction

### Prerequisites

- **aws CLI**: AWS Command Line Interface
- **zstd**: Zstandard compression tool
- **tar**: TAR archive tool (pre-installed on most systems)

**Install Tools:**
```bash
# macOS
brew install zstd awscli

# Ubuntu/Debian
apt-get install zstd awscli

# RHEL/CentOS/Fedora
dnf install zstd awscli
```

### Step-by-Step Extraction

#### 1. List Available Uploads

```bash
# List all uploads in a bucket
aws s3 ls s3://my-bucket/uploads/

# Output:
#   PRE 20251201-100000-aaaa1111/
#   PRE 20251206-123456-abcd1234/
```

#### 2. Download Manifest (Optional but Recommended)

```bash
# Download manifest to see what's in the upload
aws s3 cp s3://my-bucket/uploads/20251206-123456-abcd1234/manifest.json.gz .

# Decompress and view
zstd -d manifest.json.gz
less manifest.json

# Or use jq for pretty output
zstd -dc manifest.json.gz | jq .
```

#### 3. List Chunks

```bash
# List all chunks in upload
aws s3 ls s3://my-bucket/uploads/20251206-123456-abcd1234/ --recursive

# Output:
#   uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst
#   uploads/20251206-123456-abcd1234/shard-0/chunk-8.tar.zst
#   uploads/20251206-123456-abcd1234/shard-1/chunk-1.tar.zst
#   ...
```

#### 4. Download Specific Chunk

```bash
# Download a single chunk
aws s3 cp \
  s3://my-bucket/uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst \
  .

# Or download all chunks
aws s3 sync \
  s3://my-bucket/uploads/20251206-123456-abcd1234/ \
  ./local-download/
```

#### 5. Decompress Archive

```bash
# Decompress zstd
zstd -d chunk-0.tar.zst

# Output: chunk-0.tar
```

#### 6. Extract Files

```bash
# Extract TAR archive
tar -xf chunk-0.tar

# Extract to specific directory
mkdir -p extracted/
tar -xf chunk-0.tar -C extracted/

# List contents without extracting
tar -tzf chunk-0.tar
```

#### 7. Verify File Integrity

```bash
# Check file count
tar -tzf chunk-0.tar | wc -l

# Compare with manifest
zstd -dc manifest.json.gz | jq '.chunks[] | select(.id == 0) | .file_count'

# Verify specific file
tar -xOf chunk-0.tar data/file1.txt | sha256sum
```

### Bulk Extraction Script

**Extract Entire Upload:**

```bash
#!/bin/bash
# extract-cargoship-upload.sh

BUCKET="my-bucket"
UPLOAD_ID="20251206-123456-abcd1234"
OUTPUT_DIR="./extracted"

# Download all chunks
aws s3 sync \
  s3://${BUCKET}/uploads/${UPLOAD_ID}/ \
  ./download/ \
  --exclude "manifest.json.gz"

# Extract all chunks
mkdir -p ${OUTPUT_DIR}
for chunk in download/shard-*/chunk-*.tar.zst; do
  echo "Extracting: $chunk"
  zstd -d $chunk
  tar -xf ${chunk%.zst} -C ${OUTPUT_DIR}
  rm ${chunk%.zst}  # Clean up intermediate .tar
done

echo "Extraction complete: ${OUTPUT_DIR}"
```

**Usage:**
```bash
chmod +x extract-cargoship-upload.sh
./extract-cargoship-upload.sh
```

### Selective Extraction

**Extract Specific Files Using Manifest:**

```bash
#!/bin/bash
# extract-specific-files.sh

BUCKET="my-bucket"
UPLOAD_ID="20251206-123456-abcd1234"
PATTERN="*.log"  # Glob pattern to match

# Download manifest
aws s3 cp s3://${BUCKET}/uploads/${UPLOAD_ID}/manifest.json.gz .
zstd -d manifest.json.gz

# Find files matching pattern
FILES=$(jq -r --arg pattern "$PATTERN" \
  '.files[] | select(.path | test($pattern)) | .s3_key' \
  manifest.json | sort -u)

# Download and extract only relevant chunks
for s3_key in $FILES; do
  chunk=$(basename $s3_key)
  aws s3 cp s3://${BUCKET}/$s3_key .
  zstd -d $chunk
  tar -xf ${chunk%.zst}
  rm $chunk ${chunk%.zst}
done
```

## S3 Storage Classes

CargoShip supports all AWS S3 storage classes:

| Storage Class | Use Case | Retrieval Time | Cost |
|---------------|----------|----------------|------|
| `STANDARD` | Frequently accessed data | Immediate | Highest |
| `STANDARD_IA` | Infrequently accessed | Immediate | Medium |
| `INTELLIGENT_TIERING` | Unknown access patterns | Immediate | Auto-optimized |
| `GLACIER_IR` | Archive, instant retrieval | Immediate | Low |
| `GLACIER` | Long-term archive | Minutes-hours | Very low |
| `DEEP_ARCHIVE` | Long-term cold storage | Hours | Lowest |

**Specify at Upload:**
```bash
cargoship create upload /data \
  --bucket my-bucket \
  --storage-class STANDARD_IA
```

**Note:** Manifest is always stored in `STANDARD` class for fast access.

## Format Guarantees

### Backward Compatibility

CargoShip provides **strong backward compatibility guarantees**:

1. **Archive Format**: TAR + Zstandard will remain supported indefinitely
2. **S3 Key Structure**: Uploads/<upload-id>/shard-N/chunk-M.tar.zst pattern is stable
3. **Manifest Schema**: Version field ensures forward compatibility
4. **Extraction**: Standard tools (aws CLI, zstd, tar) will always work

### Format Versioning

**Current Version:** 1.0

**Version Policy:**
- **Major version** changes require migration (e.g., 1.x → 2.x)
- **Minor version** additions are backward compatible (e.g., 1.0 → 1.1)
- **Manifest version field** indicates format version

**Future Compatibility:**
- CargoShip will **read** all historical format versions
- **Write** operations use the latest stable format
- **Migration tools** provided for major version changes

### Breaking Change Process

If a breaking format change is required:

1. **Advance Notice**: Announced 6+ months before change
2. **Migration Period**: Both old and new formats supported
3. **Migration Tools**: Automated tools to convert old → new format
4. **Documentation**: Clear migration guide with examples
5. **Opt-In**: New format is opt-in until proven stable

### Non-Breaking Changes

The following are **NOT** considered breaking changes:
- Adding new fields to manifest (clients ignore unknown fields)
- Adding new metadata to S3 objects
- Introducing new shard strategies (with opt-in flag)
- Performance optimizations (chunk sizing, compression tuning)

## Integration Examples

### Python: Read Manifest

```python
import boto3
import gzip
import json

def read_manifest(bucket, upload_id, prefix=""):
    s3 = boto3.client('s3')

    # Download manifest
    manifest_key = f"{prefix}/uploads/{upload_id}/manifest.json.gz" if prefix else f"uploads/{upload_id}/manifest.json.gz"
    obj = s3.get_object(Bucket=bucket, Key=manifest_key)

    # Decompress and parse
    with gzip.open(obj['Body'], 'rt') as f:
        manifest = json.load(f)

    return manifest

# Usage
manifest = read_manifest("my-bucket", "20251206-123456-abcd1234")
print(f"Total files: {manifest['total_files']}")
print(f"Total size: {manifest['total_bytes']} bytes")

# Find specific files
log_files = [f for f in manifest['files'] if f['path'].endswith('.log')]
print(f"Log files: {len(log_files)}")
```

### Node.js: Extract Specific Chunk

```javascript
const { S3Client, GetObjectCommand } = require("@aws-sdk/client-s3");
const { pipeline } = require('stream/promises');
const zlib = require('zlib');
const tar = require('tar-stream');
const fs = require('fs');

async function extractChunk(bucket, chunkKey, outputDir) {
  const s3 = new S3Client({});

  // Download from S3
  const { Body } = await s3.send(new GetObjectCommand({
    Bucket: bucket,
    Key: chunkKey
  }));

  // Decompress zstd → tar → extract
  const extract = tar.extract();

  extract.on('entry', (header, stream, next) => {
    const output = fs.createWriteStream(`${outputDir}/${header.name}`);
    stream.pipe(output);
    stream.on('end', next);
  });

  // Note: Node.js doesn't have native zstd, would need fzstd package
  // This is simplified example
  await pipeline(Body, extract);
}

// Usage
await extractChunk(
  "my-bucket",
  "uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst",
  "./output"
);
```

### Go: Use CargoShip's Native API

```go
package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func main() {
	ctx := context.Background()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(err)
	}

	s3Client := s3.NewFromConfig(cfg)

	// Download manifest
	m, err := manifest.DownloadFromS3(
		ctx,
		s3Client,
		"my-bucket",
		"",  // prefix (empty if none)
		"20251206-123456-abcd1234",
	)
	if err != nil {
		panic(err)
	}

	// Query files
	query := manifest.NewManifestQuery(m)

	// Find all .log files
	logFiles := query.ListFiles("*.log")
	fmt.Printf("Found %d log files\n", len(logFiles))

	for _, file := range logFiles {
		fmt.Printf("%s - %d bytes - chunk %d - shard %d\n",
			file.Path, file.Size, file.ChunkID, file.ShardID)
	}
}
```

## Performance Characteristics

### Compression

- **Throughput**: 500-600 MB/s (zstd level 3)
- **Ratio**: 2-3:1 typical (varies by content)
- **CPU Usage**: Scales with available cores
- **Memory**: ~10-20 MB per worker thread

### Upload

- **Throughput**: 20-30 MB/s per shard (8 shards = 160-240 MB/s aggregate)
- **Latency**: <1s per chunk (typical 200MB chunks)
- **Request Rate**: 3,500 req/s per shard (28,000 req/s aggregate)
- **Memory**: Bounded by chunk size × workers (typically 2-4 GB)

### Download & Extraction

- **Download**: 50-90 MB/s (varies by S3 region and network)
- **Decompression**: 800-1000 MB/s (zstd -d)
- **Extraction**: Limited by disk I/O (~200 MB/s SSD)

## Troubleshooting

### Corrupt or Incomplete Archives

**Symptom:** `zstd: error decoding` or `tar: unexpected EOF`

**Cause:** Incomplete S3 upload or network interruption

**Solution:**
```bash
# 1. Check S3 object metadata
aws s3api head-object \
  --bucket my-bucket \
  --key uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst

# 2. Verify size matches manifest
zstd -dc manifest.json.gz | jq '.chunks[] | select(.id == 0) | .compressed_size'

# 3. Re-download if size mismatch
aws s3 cp --force-glacier-transfer \
  s3://my-bucket/uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst \
  ./chunk-0.tar.zst
```

### Missing Files

**Symptom:** Expected files not found after extraction

**Cause:** Files distributed across multiple chunks

**Solution:**
```bash
# 1. Check manifest for file location
zstd -dc manifest.json.gz | jq '.files[] | select(.path == "missing-file.txt")'

# Output shows: chunk_id: 5, shard_id: 5

# 2. Download correct chunk
aws s3 cp s3://my-bucket/uploads/20251206-123456-abcd1234/shard-5/chunk-5.tar.zst .

# 3. Extract
zstd -d chunk-5.tar.zst && tar -xf chunk-5.tar
```

### Glacier Retrieval

**Symptom:** `403 Forbidden` when accessing GLACIER/DEEP_ARCHIVE objects

**Cause:** Objects in cold storage need restoration

**Solution:**
```bash
# 1. Initiate restore request (for GLACIER)
aws s3api restore-object \
  --bucket my-bucket \
  --key uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst \
  --restore-request Days=7,GlacierJobParameters={Tier=Standard}

# 2. Check restore status
aws s3api head-object \
  --bucket my-bucket \
  --key uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst \
  --query 'Restore'

# 3. Download after restoration complete (3-5 hours for GLACIER)
aws s3 cp s3://my-bucket/uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst .
```

## Related Documentation

- **Manifest API**: [`pkg/manifest/README.md`](../pkg/manifest/README.md)
- **CLI Reference**: [`README.md`](../README.md)
- **Architecture Overview**: `ARCHITECTURE.md` (coming soon)
- **Developer Guide**: [`DEVELOPER_TOOLS.md`](DEVELOPER_TOOLS.md)

## Feedback

Have questions or suggestions about the storage format?

- **GitHub Issues**: https://github.com/scttfrdmn/cargoship/issues
- **Discussions**: https://github.com/scttfrdmn/cargoship/discussions

## License

CargoShip is open-source under the MIT License. The storage format is open and unencumbered.
