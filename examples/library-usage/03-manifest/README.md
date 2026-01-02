# Example 3: Extracting Files from Chunks

This example demonstrates how to extract individual files from CargoShip chunks using standard Go libraries.

## What This Example Shows

- Decompressing zstd-compressed chunks
- Extracting a specific file from a tar archive
- Streaming file extraction (bounded memory)
- Manual extraction using archive/tar and klauspost/compress/zstd

## Use Case

**ObjectFS Integration**: When a user reads a file through ObjectFS (e.g., `cat /mnt/archive/data.txt`), ObjectFS needs to:
1. Find the file in the manifest (Example 2)
2. Download the chunk containing that file (Example 4)
3. Extract the specific file from the chunk (this example)
4. Return the file contents to the user

## Running the Example

```bash
# Extract a single file from a local chunk
go run main.go /path/to/chunk-001.tar.zst path/to/file.txt ./output/

# The file will be extracted to: ./output/file.txt (basename only)
```

## Key Concepts

### Chunk Format

CargoShip chunks are compressed tar archives:
```
chunk-001.tar.zst
├── file1.txt
├── file2.txt
└── dir/
    └── file3.txt
```

### Extraction Process

1. **Decompress**: zstd/gzip decompression
2. **Untar**: Extract specific file from tar
3. **Verify**: Optional checksum verification
4. **Cache**: ObjectFS would cache the decompressed chunk

### Performance

- **First access**: Download + decompress + extract (~seconds)
- **Cached access**: Extract from cached decompressed chunk (~milliseconds)
- **Chunk-level caching**: All files in chunk become instantly accessible

## Next Steps

See Example 4 for downloading chunks directly from S3.
See Example 5 for validating manifest integrity.
