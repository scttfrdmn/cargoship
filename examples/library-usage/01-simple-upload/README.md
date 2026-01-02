# Example 1: Reading a CargoShip Manifest

This example demonstrates how to read and parse a CargoShip manifest file.

## What This Example Shows

- Loading a manifest JSON file from disk or S3
- Parsing the manifest structure
- Accessing metadata (upload info, statistics, compression settings)
- Listing all files in the archive

## Use Case

**ObjectFS Integration**: ObjectFS needs to read CargoShip manifests to understand the archive structure and locate files within chunks.

## Running the Example

```bash
# With a local manifest file
go run main.go /path/to/manifest.json

# The example will display:
# - Manifest metadata (upload ID, timestamps, source path)
# - Statistics (file count, total size, compression ratio)
# - List of all files with their chunk locations
```

## Key Concepts

### Manifest Structure

A CargoShip manifest contains:
- **Metadata**: Upload ID, timestamps, source information
- **Files**: Array of FileEntry with path, size, chunk location
- **Chunks**: Array of ChunkEntry with S3 keys and sizes
- **Shards**: Array of ShardEntry for multi-prefix uploads

### File Lookup

Files are organized by chunks. To find a file:
1. Look up the file in the `Files` array
2. Get the `ChunkID` from the FileEntry
3. Find the corresponding chunk in the `Chunks` array
4. Use the chunk's `S3Key` to download it

## Next Steps

See Example 2 for efficient file lookup using the query API.
See Example 3 for extracting individual files from chunks.
