# Example 2: Efficient File Lookup with ManifestQuery

This example demonstrates how to use the `ManifestQuery` API for O(1) file lookups in CargoShip manifests.

## What This Example Shows

- Creating a ManifestQuery from a manifest
- Finding files by exact path (O(1) hash map lookup)
- Finding files by pattern/glob
- Listing files in a specific directory
- Getting chunk information for a file

## Use Case

**ObjectFS Integration**: When ObjectFS receives a file read request (e.g., `cat /mnt/archive/path/to/file.txt`), it needs to quickly find which chunk contains that file. The ManifestQuery API provides O(1) lookups instead of O(n) linear scans.

## Running the Example

```bash
# Find a specific file
go run main.go /path/to/manifest.json path/to/file.txt

# Find files by glob pattern
go run main.go /path/to/manifest.json "*.go"

# List files in a directory
go run main.go /path/to/manifest.json src/
```

## Key Concepts

### ManifestQuery

The `ManifestQuery` wrapper provides:
- **O(1) file lookups** via internal hash map index
- **Pattern matching** for glob-style searches
- **Directory listings** for filesystem-like navigation

### Performance

- Linear scan (naive): O(n) for every lookup
- Hash map index (ManifestQuery): O(1) for exact matches
- For a 100,000 file archive, this is ~100,000x faster

## Next Steps

See Example 3 for extracting files from chunks once you've located them.
