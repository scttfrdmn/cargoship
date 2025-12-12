# Timestamp Tracking and Auditing

**Version:** 1.0
**Last Updated:** 2025-12-12
**Applies To:** CargoShip v0.5.0+

## Overview

CargoShip provides comprehensive timestamp tracking at multiple levels (upload, chunk, and file) for auditing, cost analysis, and compliance requirements. All timestamps are stored in the manifest and are available without additional S3 API calls.

## Quick Reference

| Level | Timestamp Field | Description | Precision | Location |
|-------|----------------|-------------|-----------|----------|
| **Upload** | `created_at` | Upload start time | Millisecond | Manifest root |
| **Upload** | `completed_at` | Upload completion time | Millisecond | Manifest root |
| **Chunk** | `created_at` | Chunk creation time | Millisecond | Manifest chunks[] |
| **Chunk** | `uploaded_at` | S3 upload completion | Millisecond | Manifest chunks[] |
| **File** | `mod_time` | Original file mtime | Millisecond | Manifest files[] |

## Manifest Timestamp Fields

### Upload-Level Timestamps

Located at manifest root level:

```json
{
  "upload_id": "20251206-123456-abcd1234",
  "created_at": "2025-12-06T14:30:00.123Z",
  "completed_at": "2025-12-06T14:35:42.456Z",
  ...
}
```

**`created_at`**
- **When:** Set at upload session start (before first chunk)
- **Purpose:** Track upload initiation time
- **Use Cases:**
  - Calculate upload duration
  - Audit when data was uploaded
  - Cost allocation by time period

**`completed_at`**
- **When:** Set when final chunk uploaded and manifest saved
- **Purpose:** Track upload completion time
- **Use Cases:**
  - Calculate total upload time
  - Determine SLA compliance
  - Cost reporting by completion date

**Upload Duration Calculation:**
```go
duration := manifest.CompletedAt.Sub(manifest.CreatedAt)
// Example: 5m42.333s
```

### Chunk-Level Timestamps

Located in `chunks[]` array:

```json
{
  "chunks": [
    {
      "id": 0,
      "s3_key": "uploads/20251206-123456-abcd1234/shard-0/chunk-0.tar.zst",
      "created_at": "2025-12-06T14:30:01.234Z",
      "uploaded_at": "2025-12-06T14:30:15.678Z",
      "compressed_size": 67108864,
      ...
    }
  ]
}
```

**`created_at`**
- **When:** Set when chunk TAR archive creation begins
- **Purpose:** Track chunk processing start
- **Use Cases:**
  - Debug chunk creation performance
  - Identify slow chunks
  - Parallel processing analysis

**`uploaded_at`**
- **When:** Set after successful S3 upload completion
- **Purpose:** Track when chunk appeared in S3
- **Use Cases:**
  - Resume capability (skip uploaded chunks)
  - Cost allocation by actual upload time
  - S3 availability verification

**Chunk Processing Time:**
```go
processingTime := chunk.UploadedAt.Sub(chunk.CreatedAt)
// Example: 14.444s for a 64MB chunk
```

### File-Level Timestamps

Located in `files[]` array:

```json
{
  "files": [
    {
      "path": "data/experiment-2025-12-06.csv",
      "size": 1048576,
      "mod_time": "2025-12-06T10:15:30.000Z",
      "chunk_id": 0,
      ...
    }
  ]
}
```

**`mod_time`**
- **When:** Preserved from original file's modification time
- **Purpose:** Track original file timestamp
- **Use Cases:**
  - Preserve file metadata
  - Restore files with original timestamps
  - Data provenance tracking
  - Compliance requirements

## Timezone Handling

All timestamps in the manifest use **UTC timezone** (RFC 3339 format):

```
2025-12-06T14:30:00.123Z
                      ↑
                      UTC indicator
```

**Benefits:**
- No timezone ambiguity
- Consistent across regions
- Standard format for APIs

**Display for Users:**
```go
// Convert to local time for display
localTime := manifest.CreatedAt.Local()
fmt.Println(localTime.Format("2006-01-02 15:04:05 MST"))
// Output: 2025-12-06 06:30:00 PST
```

## Querying Timestamps

### Using cargoship CLI

```bash
# View upload timestamps
cargoship manifest view s3://bucket/prefix/uploads/{upload-id}/manifest.json.gz

# Filter by upload date
cargoship cost projects --after 2025-12-01 --before 2025-12-31
```

### Programmatic Access

**Go:**
```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"

// Load manifest
m, err := manifest.LoadFromS3(ctx, s3Client, bucket, manifestKey)

// Access timestamps
fmt.Printf("Upload started: %s\n", m.CreatedAt)
fmt.Printf("Upload completed: %s\n", m.CompletedAt)
fmt.Printf("Duration: %s\n", m.CompletedAt.Sub(m.CreatedAt))

// Per-chunk timing
for _, chunk := range m.Chunks {
    chunkTime := chunk.UploadedAt.Sub(chunk.CreatedAt)
    fmt.Printf("Chunk %d took %s to upload\n", chunk.ID, chunkTime)
}
```

**Python:**
```python
import json
import gzip
from datetime import datetime

# Load and decompress manifest
with gzip.open('manifest.json.gz', 'rt') as f:
    manifest = json.load(f)

# Parse timestamps
created = datetime.fromisoformat(manifest['created_at'].replace('Z', '+00:00'))
completed = datetime.fromisoformat(manifest['completed_at'].replace('Z', '+00:00'))

duration = completed - created
print(f"Upload duration: {duration}")
```

## Cost Tracking with Timestamps

### Current Behavior

CargoShip cost tracking uses `time.Now()` when **recording** costs:

```go
// Cost is timestamped when calculated, not when uploaded
record := CostRecord{
    Timestamp: time.Now(),  // ← Cost recording time
    ProjectID: uploadID,     // ← Links to manifest
    ...
}
```

**Implications:**
- Costs may be recorded hours/days after upload
- Useful for "when did we calculate this cost"
- Less useful for "when was this uploaded"

### Using Upload Timestamps for Costs

For accurate upload-time cost reporting, use `manifest.CompletedAt`:

```go
// Load manifest to get actual upload completion time
manifest, _ := loadManifest(uploadID)

record := CostRecord{
    Timestamp: manifest.CompletedAt,  // ← Actual upload time
    ProjectID: uploadID,
    ...
}
```

**Benefits:**
- Costs aligned with actual upload date
- Accurate monthly/quarterly cost reporting
- Better correlation with billing cycles

## Auditing and Compliance

### Audit Requirements

CargoShip timestamps support common audit requirements:

✅ **When was data uploaded?**
→ `manifest.created_at` and `manifest.completed_at`

✅ **What files were uploaded at what time?**
→ `files[].mod_time` preserves original timestamps

✅ **When did each chunk reach S3?**
→ `chunks[].uploaded_at` for per-chunk tracking

✅ **How long did the upload take?**
→ `completed_at - created_at` for total duration

### Validation

**Timestamp Consistency Checks:**

```go
// Validate timestamps are in logical order
if !manifest.CreatedAt.Before(manifest.CompletedAt) {
    return fmt.Errorf("invalid timestamps: created_at must be before completed_at")
}

// Validate chunk timestamps
for _, chunk := range manifest.Chunks {
    if !chunk.CreatedAt.Before(chunk.UploadedAt) {
        return fmt.Errorf("invalid chunk %d: created_at must be before uploaded_at", chunk.ID)
    }

    // Chunk should be within upload window
    if chunk.UploadedAt.Before(manifest.CreatedAt) ||
       chunk.UploadedAt.After(manifest.CompletedAt) {
        log.Warnf("chunk %d uploaded outside upload window", chunk.ID)
    }
}
```

### Data Retention

Manifests with timestamps are stored indefinitely in S3:

```
s3://bucket/prefix/uploads/{upload-id}/manifest.json.gz
```

**Retention Considerations:**
- Manifests are small (~30KB for 10K files)
- Enable S3 versioning for compliance
- Use S3 Object Lock for immutability
- Consider lifecycle policies for old uploads

## S3 Native Timestamps

### LastModified Field

S3 also provides `LastModified` timestamp for each object:

```go
// Query S3 metadata (requires API call)
result, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
    Bucket: aws.String(bucket),
    Key:    aws.String(key),
})
lastModified := *result.LastModified
```

### Comparison: Manifest vs S3

| Feature | Manifest Timestamps | S3 LastModified |
|---------|-------------------|-----------------|
| **API Calls** | 0 (in manifest) | 1 per object (HeadObject) |
| **Cost** | Free | $0.0004/1K requests |
| **Precision** | Millisecond | Second-level |
| **Granularity** | Upload + Chunk | Object only |
| **Details** | Start, end, duration | Single timestamp |
| **Availability** | Requires manifest | Always available |

### When to Use S3 LastModified

Use S3 `LastModified` only as a **fallback** if:

❌ Manifest is corrupted or missing
❌ Need S3-verified timestamp for compliance
❌ Cross-verifying manifest data

**Primary recommendation:** Use manifest timestamps (faster, more detailed, no cost).

## Resume and Recovery

Timestamps enable intelligent resume:

```go
// Check if chunk already uploaded (Issue #157)
for _, chunk := range manifest.Chunks {
    if !chunk.UploadedAt.IsZero() {
        // Chunk already uploaded, skip
        fmt.Printf("⏭️  Skipping chunk %d (uploaded at %s)\n",
            chunk.ID, chunk.UploadedAt)
        continue
    }

    // Upload chunk
    uploadChunk(chunk)
}
```

## Performance Analysis

Use timestamps to identify bottlenecks:

```go
// Find slowest chunks
type chunkTiming struct {
    ID       int
    Duration time.Duration
}

var timings []chunkTiming
for _, chunk := range manifest.Chunks {
    duration := chunk.UploadedAt.Sub(chunk.CreatedAt)
    timings = append(timings, chunkTiming{chunk.ID, duration})
}

// Sort by duration (slowest first)
sort.Slice(timings, func(i, j int) bool {
    return timings[i].Duration > timings[j].Duration
})

// Report top 5 slowest
for i := 0; i < 5 && i < len(timings); i++ {
    fmt.Printf("Chunk %d: %s\n", timings[i].ID, timings[i].Duration)
}
```

## Best Practices

### For Users

✅ **Use manifest timestamps for auditing** (fast, comprehensive)
✅ **Preserve file timestamps** (`mod_time` maintains provenance)
✅ **Monitor upload duration** (identify performance issues)
✅ **Store manifests long-term** (compliance and auditing)

❌ **Don't query S3 LastModified** unless manifest unavailable
❌ **Don't assume local timezone** (all timestamps are UTC)

### For Developers

✅ **Use `manifest.CompletedAt`** for cost timestamp accuracy
✅ **Validate timestamp consistency** (created < completed)
✅ **Log timestamps at key stages** (debugging)
✅ **Handle timezone conversions** when displaying to users

❌ **Don't use `time.Now()`** for upload-related timestamps
❌ **Don't assume timestamp order** without validation

## Examples

### Cost Report by Upload Date

```bash
# Get costs for uploads completed in December 2025
cargoship cost projects --after 2025-12-01T00:00:00Z --before 2025-12-31T23:59:59Z
```

### Upload Performance Analysis

```bash
# List uploads with duration
cargoship manifest list s3://bucket/prefix/ --show-duration

# Output:
# Upload ID                         Started              Completed            Duration
# 20251206-123456-abcd1234         2025-12-06 14:30:00  2025-12-06 14:35:42  5m42s
# 20251207-234501-ef567890         2025-12-07 09:15:20  2025-12-07 09:47:13  31m53s
```

### Programmatic Audit Trail

```go
// Generate audit report
func generateAuditReport(manifests []*manifest.Manifest) {
    for _, m := range manifests {
        fmt.Printf("Upload: %s\n", m.UploadID)
        fmt.Printf("  Started:   %s\n", m.CreatedAt.Format(time.RFC3339))
        fmt.Printf("  Completed: %s\n", m.CompletedAt.Format(time.RFC3339))
        fmt.Printf("  Duration:  %s\n", m.CompletedAt.Sub(m.CreatedAt))
        fmt.Printf("  Files:     %d\n", m.TotalFiles)
        fmt.Printf("  Size:      %.2f GB\n", float64(m.TotalBytes)/(1024*1024*1024))
        fmt.Printf("  Chunks:    %d\n", m.TotalChunks)

        // Per-chunk timing summary
        var totalChunkTime time.Duration
        for _, chunk := range m.Chunks {
            totalChunkTime += chunk.UploadedAt.Sub(chunk.CreatedAt)
        }
        avgChunkTime := totalChunkTime / time.Duration(len(m.Chunks))
        fmt.Printf("  Avg chunk: %s\n", avgChunkTime)
        fmt.Println()
    }
}
```

## Related Documentation

- **[Storage Format](STORAGE_FORMAT.md)** - Overall storage structure
- **[Cost Management](cost-management.md)** - Cost tracking and budgets
- **[User Guide](USER_GUIDE.md)** - General usage documentation
- **[Budget User Guide](BUDGET_USER_GUIDE.md)** - Budget configuration

## Technical Details

### Timestamp Format

All timestamps use **RFC 3339** format:

```
2025-12-06T14:30:00.123456789Z
│          │  │  │  └─ Nanoseconds (up to 9 digits)
│          │  │  └──── Seconds
│          │  └─────── Minutes
│          └────────── Hours (24-hour)
└───────────────────── Date (YYYY-MM-DD)
```

### JSON Schema

```json
{
  "created_at": {
    "type": "string",
    "format": "date-time",
    "description": "RFC 3339 timestamp in UTC"
  },
  "completed_at": {
    "type": "string",
    "format": "date-time",
    "description": "RFC 3339 timestamp in UTC"
  }
}
```

### Go Types

```go
type Manifest struct {
    CreatedAt   time.Time `json:"created_at"`   // Go time.Time
    CompletedAt time.Time `json:"completed_at"` // Go time.Time
}

// Serialization
data, _ := json.Marshal(manifest)
// {"created_at":"2025-12-06T14:30:00.123Z",...}

// Deserialization
var m Manifest
json.Unmarshal(data, &m)
// m.CreatedAt is time.Time in UTC
```

## Troubleshooting

### Issue: Timestamps in Wrong Timezone

**Problem:** Timestamps appear in local time instead of UTC

**Solution:** Convert to local time for display:
```go
localTime := manifest.CreatedAt.Local()
```

### Issue: Missing UploadedAt on Chunks

**Problem:** Some chunks have zero `uploaded_at`

**Cause:** Upload was interrupted or partial

**Solution:** Use resume capability to complete upload:
```bash
cargoship create upload ./data --bucket my-bucket --resume
```

### Issue: CreatedAt After CompletedAt

**Problem:** Timestamps are reversed

**Cause:** System clock changed during upload or manifest corruption

**Solution:** Validate and reject invalid manifests:
```go
if manifest.CompletedAt.Before(manifest.CreatedAt) {
    return fmt.Errorf("invalid manifest: timestamps reversed")
}
```

## Support

For questions or issues related to timestamp tracking:

- **GitHub Issues:** https://github.com/scttfrdmn/cargoship/issues
- **Documentation:** https://github.com/scttfrdmn/cargoship/tree/main/docs

---

**Last Updated:** 2025-12-12
**Related Issue:** #146
