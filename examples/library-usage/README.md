# CargoShip Library Usage Examples

This directory contains examples demonstrating how to use CargoShip as a Go library for reading, parsing, and extracting data from CargoShip archives.

## Overview

CargoShip creates compressed, chunked archives in S3 with JSON manifests describing the archive structure. These examples show how external tools (like ObjectFS) can integrate with CargoShip archives.

## Use Case: ObjectFS Integration

[ObjectFS](https://github.com/scttfrdmn/objectfs) provides POSIX filesystem semantics over S3 with aggressive caching. These examples demonstrate the CargoShip APIs that ObjectFS (or similar tools) can use to:

- Parse CargoShip manifests
- Perform O(1) file lookups
- Extract specific files without downloading entire archives
- Download chunks on-demand from S3
- Validate archive integrity

## Examples

### [01-simple-upload](01-simple-upload/)
**Reading CargoShip Manifests**

Demonstrates how to parse a CargoShip manifest JSON file and access metadata:
- Upload metadata (ID, timestamps, source path)
- S3 location (bucket, prefix, region)
- Statistics (file count, total size, chunks, shards)
- Compression settings
- Encryption configuration

```bash
cd 01-simple-upload
go run main.go /path/to/manifest.json
```

### [02-compression](02-compression/)
**Efficient File Lookup with ManifestQuery**

Shows how to use the `ManifestQuery` API for O(1) file lookups via hash map indexing:
- `FindFile(path)` - exact file path lookup
- `FindByPattern(glob)` - pattern matching (e.g., `*.go`)
- Directory listings
- Chunk location discovery

```bash
cd 02-compression
go run main.go /path/to/manifest.json src/main.go
go run main.go /path/to/manifest.json "*.txt"
```

**Performance**: O(1) file lookup, critical for POSIX stat operations.

### [03-manifest](03-manifest/)
**Extracting Files from Chunks**

Demonstrates extracting specific files from compressed chunks:
- Download chunk from S3
- Decompress (zstd/gzip)
- Extract files from tar archive
- Handle file splitting (large files across chunks)

```bash
cd 03-manifest
go run main.go /path/to/chunk.tar.zst /output/dir file1.txt file2.go
```

**Performance**: Only downloads required chunks, not entire archive.

### [04-multiregion](04-multiregion/)
**Downloading from S3**

Shows how to download manifests and chunks from S3 using AWS SDK v2:
- Load AWS credentials
- Download manifest JSON
- Download specific chunks
- Error handling and retries

```bash
cd 04-multiregion
go run main.go s3://bucket/prefix/manifest.json chunk-000
```

**Integration**: Combine with Example 3 for on-demand file extraction.

### [05-progress](05-progress/)
**Manifest Validation**

Validates manifest integrity before use:
- Version compatibility
- Required fields present
- Statistics consistency (file count, size calculations)
- Reference integrity (file → chunk → shard)
- Duplicate detection
- Compression ratio validation

```bash
cd 05-progress
go run main.go /path/to/manifest.json
```

**Best Practice**: Always validate manifests before mounting/reading.

## Manifest Format

See [MANIFEST-FORMAT.md](MANIFEST-FORMAT.md) for the complete manifest specification including:
- Field descriptions and types
- File/chunk/shard entry structures
- File lookup algorithms
- Validation requirements
- ObjectFS integration patterns

## Getting Started

### Prerequisites

```bash
go 1.23+
```

### Running Examples

Each example is a standalone Go module with its own `go.mod`. Run from within each example directory:

```bash
cd 01-simple-upload
go mod tidy
go run main.go <args>
```

### Using CargoShip as a Library

Add CargoShip to your `go.mod`:

```go
require github.com/scttfrdmn/cargoship v0.6.0
```

Import the manifest package:

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"
```

## Key APIs

### Manifest Parsing

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"

var m manifest.Manifest
err := json.Unmarshal(data, &m)
```

### File Lookup

```go
query := manifest.NewManifestQuery(&m)
fileEntry := query.FindFile("path/to/file.txt")  // O(1)
matches := query.FindByPattern("*.go")           // Pattern matching
```

### File Extraction

```go
extractedFiles, err := manifest.ExtractFiles(
    chunkPath,
    outputDir,
    []string{"file1.txt", "file2.go"},
)
```

### Validation

```go
errors := validateManifest(&m)
if len(errors) > 0 {
    // Handle validation errors
}
```

## ObjectFS Integration Pattern

Recommended workflow for mounting CargoShip archives as POSIX filesystems:

1. **Download manifest** (Example 4: S3 download)
2. **Validate manifest** (Example 5: Validation)
3. **Build file index** (Example 2: ManifestQuery for O(1) lookups)
4. **Implement file operations**:
   - `stat()`: Read from manifest metadata (no S3 calls)
   - `read()`: Download chunk (Example 4), extract file (Example 3)
   - `readdir()`: Scan manifest file index (Example 2)
5. **Cache chunks**: LRU cache for decompressed chunks (reduce S3 calls)

See MANIFEST-FORMAT.md "ObjectFS Integration Guide" section for detailed architecture.

## Performance Characteristics

| Operation | Time Complexity | Notes |
|-----------|----------------|-------|
| File lookup | O(1) | Hash map index in ManifestQuery |
| First file read | ~2-5s | Download + decompress chunk |
| Cached read (same chunk) | <10ms | Decompressed chunk in cache |
| Directory listing | O(n files) | Scan manifest file index |
| Stat operations | O(1) | Metadata in manifest |

## Architecture Overview

```
CargoShip Archive Structure:
├── manifest.json                    (Examples 1, 2, 5)
├── shard-0/
│   ├── chunk-000.tar.zst           (Examples 3, 4)
│   ├── chunk-001.tar.zst
│   └── ...
├── shard-1/
│   └── ...
└── shard-N/
    └── ...

ObjectFS Usage:
1. Download manifest.json → Parse (Example 1)
2. Validate manifest → Check integrity (Example 5)
3. Build file index → O(1) lookups (Example 2)
4. On file read:
   a. Lookup file in index → Find chunk ID
   b. Download chunk from S3 → Example 4
   c. Extract file from chunk → Example 3
   d. Cache chunk → Reuse for next file in same chunk
```

## Error Handling

All examples demonstrate:
- Checking for required command-line arguments
- Validating file paths and S3 URLs
- Handling AWS SDK errors
- Graceful error messages

## See Also

- [CargoShip Main Documentation](../../README.md)
- [CargoShip CLI Usage](../../docs/)
- [ObjectFS Project](https://github.com/scttfrdmn/objectfs)
- [Issue #28: ObjectFS integration examples](https://github.com/scttfrdmn/cargoship/issues/28)

## Contributing

To add new examples:
1. Create new directory: `examples/library-usage/06-your-example/`
2. Add `main.go`, `go.mod`, `README.md`
3. Update this README with example description
4. Ensure example compiles and runs correctly

## License

Same as CargoShip main project.
