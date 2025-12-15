# Session Affinity in Multi-Region Load Balancing

## Overview

Session affinity (also known as "sticky sessions") ensures that requests from the same session are consistently routed to the same AWS region. This is critical for multi-part uploads where all parts must be uploaded to the same region for consistency.

**Issue**: #131
**Implementation**: `pkg/multiregion/load_balancer.go`
**Status**: ✅ Fully Implemented

---

## Why Session Affinity Matters

### Multi-Part Upload Consistency

When uploading large files using S3's multipart upload API:
1. An upload is initiated in a specific region
2. All subsequent parts must be uploaded to the **same region**
3. The upload is completed in that region

Without session affinity, parts could be routed to different regions, causing upload failures.

### Example Scenario

```
Upload ID: abc-123-def
├─ InitiateMultipartUpload → us-west-2 ✅
├─ UploadPart 1 → us-west-2 ✅ (session affinity)
├─ UploadPart 2 → eu-west-1 ❌ (without affinity - FAILS!)
└─ CompleteMultipartUpload → us-west-2 ❌ (missing part 2)
```

With session affinity enabled:
```
Upload ID: abc-123-def (session key: "upload:abc-123-def")
├─ InitiateMultipartUpload → us-west-2 ✅
├─ UploadPart 1 → us-west-2 ✅ (routed by session key)
├─ UploadPart 2 → us-west-2 ✅ (routed by session key)
└─ CompleteMultipartUpload → us-west-2 ✅ (all parts in same region)
```

---

## How It Works

### Session Key Generation

Session keys are generated using a priority-based system:

1. **Explicit `session_id` in metadata** (highest priority)
   - Allows clients to explicitly control session affinity
   - Useful for multi-request workflows

2. **User ID from metadata** (`user_id`)
   - All requests from the same user go to the same region
   - Key format: `user:<user_id>`

3. **Client ID from metadata** (`client_id`)
   - All requests from the same client go to the same region
   - Key format: `client:<client_id>`

4. **Request ID** (backward compatibility)
   - Each unique request ID creates a separate session
   - Default behavior for most use cases

5. **Generated secure UUID** (fallback)
   - Cryptographically secure random UUID v4
   - Used when no identifiers are provided

### Session Key Priority Example

```go
// Priority 1: Explicit session ID
request := &UploadRequest{
    Metadata: map[string]string{
        "session_id": "shared-session-abc",
    },
}
// Session key: "shared-session-abc"

// Priority 2: User ID
request := &UploadRequest{
    Metadata: map[string]string{
        "user_id": "user-123",
    },
}
// Session key: "user:user-123"

// Priority 3: Client ID
request := &UploadRequest{
    Metadata: map[string]string{
        "client_id": "client-456",
    },
}
// Session key: "client:client-456"

// Priority 4: Request ID
request := &UploadRequest{
    ID: "req-789",
}
// Session key: "req-789"

// Priority 5: Generated UUID
request := &UploadRequest{
    // No ID or metadata
}
// Session key: "550e8400-e29b-41d4-a716-446655440000" (UUID v4)
```

---

## Configuration

### Enable Session Affinity

Set `StickySessions: true` in the load balancing configuration:

```go
config := &MultiRegionConfig{
    LoadBalancing: LoadBalancingConfig{
        Algorithm:      "least_connections",
        StickySessions: true,  // Enable session affinity
    },
}
```

### YAML Configuration

```yaml
multi_region:
  load_balancing:
    algorithm: least_connections
    sticky_sessions: true  # Enable session affinity

  session_affinity:
    ttl: 1h  # Session expiration time
```

### Session TTL

Sessions expire after a configurable TTL (default: 1 hour):
- Prevents memory growth from abandoned sessions
- Automatically cleans up expired sessions
- TTL resets on each request (sliding window)

---

## Usage Examples

### Example 1: Multi-Part Upload Workflow

```go
import "github.com/scttfrdmn/cargoship/pkg/multiregion"

// Create multi-region transporter with sticky sessions
config := &multiregion.MultiRegionConfig{
    Regions: []multiregion.RegionConfig{
        {Name: "us-west-2", Endpoint: "..."},
        {Name: "eu-west-1", Endpoint: "..."},
    },
    LoadBalancing: multiregion.LoadBalancingConfig{
        Algorithm:      "least_connections",
        StickySessions: true,  // IMPORTANT: Enable for multi-part uploads
    },
}

transporter, err := multiregion.NewMultiRegionTransporter(config)
if err != nil {
    return err
}

// All uploads with the same request ID will route to the same region
uploadID := "upload-abc-123"

request1 := &multiregion.UploadRequest{
    ID:   uploadID,
    Data: part1Reader,
}
// Routes to us-west-2 (selected by load balancer)

request2 := &multiregion.UploadRequest{
    ID:   uploadID,
    Data: part2Reader,
}
// Routes to us-west-2 (same as request1 - session affinity)

request3 := &multiregion.UploadRequest{
    ID:   uploadID,
    Data: part3Reader,
}
// Routes to us-west-2 (same as request1 - session affinity)
```

### Example 2: User-Based Session Affinity

Route all uploads from the same user to the same region:

```go
// User "alice" uploads multiple files
for _, file := range files {
    request := &multiregion.UploadRequest{
        ID:   generateRequestID(),
        Data: fileReader,
        Metadata: map[string]string{
            "user_id": "alice",  // All alice's uploads go to same region
        },
    }

    result, err := transporter.Upload(ctx, request)
    // All requests route to the same region (e.g., us-west-2)
}
```

### Example 3: Explicit Session Control

Explicitly control which requests share a session:

```go
sessionID := "batch-upload-2024-01"

// Upload batch of related files
for _, file := range batchFiles {
    request := &multiregion.UploadRequest{
        ID:   generateRequestID(),
        Data: fileReader,
        Metadata: map[string]string{
            "session_id": sessionID,  // Explicit session affinity
        },
    }

    result, err := transporter.Upload(ctx, request)
    // All requests in this batch route to the same region
}
```

### Example 4: Monitoring Session Affinity

Get statistics about active sessions:

```go
loadBalancer := transporter.GetLoadBalancer()

stats := loadBalancer.GetSessionAffinityStats()
fmt.Printf("Active sessions: %d\n", stats["total_sessions"])
fmt.Printf("Sessions by region:\n")
for region, count := range stats["sessions_by_region"].(map[string]int) {
    fmt.Printf("  %s: %d sessions\n", region, count)
}
```

---

## Session Lifecycle

### 1. Session Creation

When a request is routed with sticky sessions enabled:
1. Load balancer generates session key from request
2. Checks if session key exists in affinity map
3. If not found, routes using normal algorithm and creates session
4. Records: region, creation time, last used time, request count

### 2. Session Usage

On subsequent requests with the same session key:
1. Load balancer looks up session key
2. Checks if session has expired (TTL)
3. If valid, routes to stored region
4. Updates last used time and increments request count
5. If region is unhealthy, session is deleted and re-routed

### 3. Session Expiration

Sessions expire when:
- TTL exceeded (no activity for 1 hour by default)
- Targeted region becomes unhealthy
- Manually cleaned up by background goroutine

```go
// Automatic cleanup runs periodically
func (lb *DefaultLoadBalancer) cleanupExpiredSessions() {
    lb.mu.Lock()
    defer lb.mu.Unlock()

    now := time.Now()
    for sessionKey, affinity := range lb.sessionAffinityMap {
        if now.Sub(affinity.LastUsed) > sessionTTL {
            delete(lb.sessionAffinityMap, sessionKey)
        }
    }
}
```

---

## Security Considerations

### Cryptographically Secure Session IDs

When generating session keys without request identifiers, the implementation uses `crypto/rand` for unpredictable, secure UUIDs:

```go
func (lb *DefaultLoadBalancer) generateSecureSessionID() string {
    uuid := make([]byte, 16)
    _, err := cryptorand.Read(uuid)
    if err != nil {
        // Fallback to timestamp (should never happen)
        return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
    }

    // Set UUID v4 version and variant bits (RFC 4122)
    uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
    uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

    return fmt.Sprintf("%x-%x-%x-%x-%x",
        uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
```

### Session Hijacking Prevention

- Session keys are generated server-side
- No predictable patterns in generated UUIDs
- TTL prevents indefinite session persistence
- Sessions tied to specific regions (no cross-region reuse)

---

## Performance Impact

### Memory Usage

Each active session consumes approximately:
- 32 bytes for session key (string)
- 40 bytes for SessionAffinity struct
- **~72 bytes total per session**

With 1 million active sessions: ~72 MB memory

### Lookup Performance

- Session affinity map uses Go's native map (O(1) lookup)
- Mutex protection for concurrent access
- No performance degradation with many sessions

### Cleanup Overhead

- Background cleanup runs every 5 minutes
- O(n) scan of session map
- Minimal impact even with large session counts

---

## Testing

Comprehensive tests verify session affinity behavior:

### Test Coverage

```go
// Test session key generation priority
TestGenerateSessionKey_Priority()

// Test session affinity routing
TestDefaultLoadBalancer_Route_WithSessionAffinity()

// Test session expiration
TestCleanupExpiredSessions()

// Test metadata-based sessions
TestSessionAffinity_WithMetadata()

// Test concurrent session access
TestConcurrentRouting_WithSessionAffinity()

// Test session statistics
TestDefaultLoadBalancer_GetSessionAffinityStats()
```

### Running Tests

```bash
go test ./pkg/multiregion/... -v -run "SessionAffinity"
```

---

## Troubleshooting

### Issue: Parts uploaded to different regions

**Cause**: Sticky sessions not enabled

**Solution**: Enable in configuration
```go
config.LoadBalancing.StickySessions = true
```

### Issue: Sessions not persisting

**Cause**: Different request IDs for each part

**Solution**: Use explicit session ID in metadata
```go
request.Metadata["session_id"] = uploadID
```

### Issue: Memory growth from abandoned sessions

**Cause**: Sessions never expiring

**Solution**: Adjust TTL or ensure cleanup is running
```go
// Sessions auto-expire after TTL (default: 1h)
// Cleanup runs automatically in background
```

### Issue: Session always routes to unhealthy region

**Cause**: Stale session affinity

**Solution**: Sessions are automatically deleted when region becomes unhealthy. If issue persists, check health monitoring configuration.

---

## Best Practices

1. **Always enable sticky sessions for multi-part uploads**
   ```go
   config.LoadBalancing.StickySessions = true
   ```

2. **Use explicit session IDs for workflows**
   ```go
   request.Metadata["session_id"] = workflowID
   ```

3. **Use user IDs for user-scoped affinity**
   ```go
   request.Metadata["user_id"] = userID
   ```

4. **Monitor session statistics**
   ```go
   stats := loadBalancer.GetSessionAffinityStats()
   ```

5. **Set appropriate TTL for your workload**
   - Short TTL (15 min): Streaming uploads
   - Medium TTL (1 hour): Batch processing (default)
   - Long TTL (24 hours): Long-running workflows

---

## References

- **Issue #131**: Implement session affinity with proper session key generation
- **Issue #139**: Secure session key generation implementation
- **Code**: `pkg/multiregion/load_balancer.go`
- **Tests**: `pkg/multiregion/load_balancer_test.go`
- **Related**: Multi-region architecture (`docs/PHASE2_MULTI_REGION.md`)

---

## Changelog

- **v0.6.2**: Initial session affinity implementation with secure session key generation (Issue #139)
- **v0.6.3**: Comprehensive documentation and examples (Issue #131)
