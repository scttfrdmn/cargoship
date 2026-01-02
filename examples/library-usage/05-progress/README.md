# Example 5: Manifest Validation

This example demonstrates how to validate CargoShip manifests for integrity and consistency.

## What This Example Shows

- Using the manifest validation API
- Checking for corrupted or incomplete manifests
- Verifying file/chunk/shard consistency
- Detecting missing chunks or files

## Use Case

**ObjectFS Integration**: Before mounting a CargoShip archive, ObjectFS should validate the manifest to ensure:
- All chunks are present
- File entries reference valid chunks
- No data corruption
- Version compatibility

## Running the Example

```bash
# Validate a manifest
go run main.go /path/to/manifest.json

# Output shows:
# - Validation results
# - Any errors or warnings
# - Manifest statistics
```

## Key Concepts

### Validation Checks

The validation API performs:
1. **Structure validation**: JSON schema compliance
2. **Reference validation**: File → Chunk → Shard references
3. **Size validation**: Compressed/uncompressed sizes make sense
4. **Version validation**: Manifest format version compatibility

### Error Handling

Validation errors indicate:
- **Critical errors**: Manifest is unusable (corrupted, missing chunks)
- **Warnings**: Manifest is usable but has issues (missing checksums)

### Best Practices

ObjectFS should:
1. Validate manifest on mount
2. Cache validation results
3. Refuse to mount if critical errors found
4. Warn user about non-critical issues

## Manifest Format

See MANIFEST-FORMAT.md for complete manifest specification.
