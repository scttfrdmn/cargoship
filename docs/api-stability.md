# CargoShip Public API Stability Guarantees

**Version**: v0.4.5+
**Last Updated**: 2025-10-15

## Overview

This document defines CargoShip's commitment to API stability for library consumers. CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/), providing clear guarantees about API changes across releases.

## Stability Levels

CargoShip APIs are classified into three stability levels:

1. **Stable** - Guaranteed backward compatibility within major version
2. **Beta** - May change in minor versions with deprecation notice
3. **Experimental** - May change in any version without notice

---

## Stable Public APIs

These APIs are guaranteed to remain backward compatible within the current major version (v0.x.x). Breaking changes will only occur in major version bumps (e.g., v0.x → v1.0).

### pkg/aws/s3 - S3 Transport

#### Transporter

**Package**: `github.com/scttfrdmn/cargoship/pkg/aws/s3`

```go
// Constructor - Stable signature
func NewTransporter(client *s3.Client, config config.S3Config) *Transporter

// Core methods - Stable signatures
func (t *Transporter) Upload(ctx context.Context, archive Archive) (*UploadResult, error)
func (t *Transporter) Exists(ctx context.Context, key string) (bool, error)
func (t *Transporter) GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error)
func (t *Transporter) GetConfig() config.S3Config
```

**Stability Guarantees**:
- ✅ Constructor signature will not change
- ✅ Method signatures will not change
- ✅ Return types will not change
- ✅ Behavior will remain consistent (bug fixes excepted)
- ✅ Thread-safety guarantees maintained

**Allowed Changes** (non-breaking):
- ✅ Adding new methods
- ✅ Performance improvements
- ✅ Bug fixes
- ✅ Internal implementation changes

**Not Allowed** (breaking):
- ❌ Removing methods
- ❌ Changing method signatures
- ❌ Changing return types
- ❌ Breaking documented behavior

#### Archive Type

```go
type Archive struct {
    Key             string                 // S3 object key - STABLE
    Reader          io.Reader              // Archive content - STABLE
    Size            int64                  // Archive size in bytes - STABLE
    StorageClass    config.StorageClass    // Target storage class - STABLE
    Metadata        map[string]string      // Custom metadata - STABLE
    OriginalSize    int64                  // Original uncompressed size - STABLE
    CompressionType string                 // Compression algorithm used - STABLE
    AccessPattern   string                 // Expected access pattern - STABLE
    RetentionDays   int                    // Expected retention period - STABLE
}
```

**Stability Guarantees**:
- ✅ Existing fields will not be removed
- ✅ Existing field types will not change
- ✅ Field semantics will remain consistent

**Allowed Changes** (non-breaking):
- ✅ Adding new fields (with zero-value defaults)
- ✅ Making fields optional

**Not Allowed** (breaking):
- ❌ Removing fields
- ❌ Changing field types
- ❌ Making optional fields required

#### UploadResult Type

```go
type UploadResult struct {
    Location     string             // S3 URL - STABLE
    Key          string             // S3 object key - STABLE
    ETag         string             // S3 ETag - STABLE
    UploadID     string             // Multipart upload ID - STABLE
    Duration     time.Duration      // Upload duration - STABLE
    Throughput   float64            // Upload throughput in MB/s - STABLE
    StorageClass types.StorageClass // Actual storage class used - STABLE
}
```

**Stability Guarantees**: Same as Archive type

---

### pkg/aws/config - Configuration

**Package**: `github.com/scttfrdmn/cargoship/pkg/aws/config`

#### S3Config

```go
type S3Config struct {
    Bucket                 string       // STABLE
    StorageClass           StorageClass // STABLE
    MultipartThreshold     int64        // STABLE
    MultipartChunkSize     int64        // STABLE
    Concurrency            int          // STABLE
    KMSKeyID               string       // STABLE
    UseTransferAcceleration bool        // STABLE
}
```

**Stability Guarantees**:
- ✅ All fields remain stable
- ✅ Zero values have sensible defaults
- ✅ Field semantics preserved

#### StorageClass Constants

```go
const (
    StorageClassStandard           StorageClass = "STANDARD"            // STABLE
    StorageClassStandardIA         StorageClass = "STANDARD_IA"         // STABLE
    StorageClassOneZoneIA          StorageClass = "ONEZONE_IA"          // STABLE
    StorageClassIntelligentTiering StorageClass = "INTELLIGENT_TIERING" // STABLE
    StorageClassGlacier            StorageClass = "GLACIER"             // STABLE
    StorageClassDeepArchive        StorageClass = "DEEP_ARCHIVE"        // STABLE
)
```

**Stability Guarantees**:
- ✅ Constants will not be removed
- ✅ Values will not change
- ✅ New constants may be added

#### Functions

```go
// Stable functions
func DefaultAWSConfig() *AWSConfig
func (c *AWSConfig) Validate() error
func LoadAWSConfig(ctx context.Context, profile, region string) (aws.Config, error)
func IsLocalStackConfig() bool
```

---

## Beta APIs

These APIs are production-ready but may change in minor versions. Deprecation notices will be provided at least 2 minor versions before removal.

### Cost Control Features

**Package**: `github.com/scttfrdmn/cargoship/pkg/aws/config`

```go
type CostControlConfig struct { /* ... */ }  // BETA
type PricingConfig struct { /* ... */ }      // BETA
```

**Stability**:
- ⚠️ May change in minor versions
- ⚠️ Deprecation notice: 2 minor versions
- ⚠️ Migration path provided

### Multi-Region Coordination

**Package**: `github.com/scttfrdmn/cargoship/pkg/multiregion`

**Stability**:
- ⚠️ APIs are stabilizing
- ⚠️ Major changes unlikely
- ⚠️ Monitor CHANGELOG for updates

---

## Experimental APIs

These features are under active development and may change without notice. Use at your own risk.

### Advanced Staging Optimization

**Package**: `github.com/scttfrdmn/cargoship/pkg/staging`

**Marked with**: `// Experimental: This API may change in future versions`

**Stability**:
- ⚠️ May change in any version
- ⚠️ No deprecation notice required
- ⚠️ Not recommended for production use

### Predictive Prefetching

**Package**: `github.com/scttfrdmn/cargoship/pkg/s3optimization`

**Marked with**: `// Experimental: This API may change in future versions`

---

## Deprecation Policy

When we need to make breaking changes to stable APIs:

### Process

1. **Announce** (Version N): API marked as deprecated with clear warning
   ```go
   // Deprecated: Use NewMethod instead. Will be removed in v0.x.0
   func OldMethod() {}
   ```

2. **Migrate** (Version N+1 to N+2): Both APIs available, warnings in docs
   - Migration guide published
   - Examples updated
   - Automated migration tools provided (if applicable)

3. **Remove** (Version N+3): Deprecated API removed in next major version

### Example Timeline

```
v0.5.0: Feature deprecated, marked with // Deprecated comment
v0.6.0: Feature still available, deprecation warnings in logs
v0.7.0: Feature still available, deprecation warnings remain
v1.0.0: Feature removed, new API only
```

### Guarantees

- ⏰ **Minimum 2 minor versions** before removal
- 📖 **Clear migration path** documented
- ✅ **Working examples** for new API
- 🔔 **Compile-time warnings** via // Deprecated comments

---

## Version Compatibility Matrix

### For Library Consumers (like ObjectFS)

| CargoShip Version | Recommended Usage | Notes |
|-------------------|-------------------|-------|
| v0.4.x | ✅ Production Ready | Current stable APIs |
| v0.5.x | ✅ Production Ready | Performance enhancements |
| v0.6.x | ✅ Production Ready | Planned features |
| v1.0.0 | 🔄 Migration Required | Major version, breaking changes |

### Dependency Pinning Recommendations

**Conservative** (Most Stable):
```go
require github.com/scttfrdmn/cargoship v0.4.5
```

**Recommended** (Security + Bug Fixes):
```go
require github.com/scttfrdmn/cargoship v0.4.x
```

**Flexible** (New Features):
```go
require github.com/scttfrdmn/cargoship v0.x
```

**Caution**: Do not use `latest` for production dependencies.

---

## API Change Examples

### ✅ Non-Breaking Changes (Allowed)

#### Adding New Methods
```go
// v0.4.5
type Transporter struct { /* ... */ }
func (t *Transporter) Upload(ctx, archive) error

// v0.5.0 - NEW METHOD ADDED
func (t *Transporter) UploadWithProgress(ctx, archive, progressChan) error
```

#### Adding Struct Fields
```go
// v0.4.5
type Archive struct {
    Key  string
    Size int64
}

// v0.5.0 - NEW FIELD ADDED
type Archive struct {
    Key         string
    Size        int64
    ContentType string // New field with zero-value default
}
```

#### Performance Improvements
```go
// v0.4.5: Uses standard algorithm
func (t *Transporter) Upload() { /* slow implementation */ }

// v0.5.0: Optimized implementation, same behavior
func (t *Transporter) Upload() { /* 2x faster, same result */ }
```

### ❌ Breaking Changes (Not Allowed in Minor Versions)

#### Removing Methods
```go
// v0.4.5
func (t *Transporter) Upload(ctx, archive) error

// v0.5.0 - BREAKING: Method removed
// THIS WOULD REQUIRE v1.0.0
```

#### Changing Signatures
```go
// v0.4.5
func NewTransporter(client *s3.Client, config S3Config) *Transporter

// v0.5.0 - BREAKING: Added required parameter
func NewTransporter(client *s3.Client, config S3Config, logger *Logger) *Transporter
// THIS WOULD REQUIRE v1.0.0
```

#### Changing Return Types
```go
// v0.4.5
func (t *Transporter) Upload(ctx, archive) error

// v0.5.0 - BREAKING: Changed return type
func (t *Transporter) Upload(ctx, archive) (*UploadResult, error)
// THIS WOULD REQUIRE v1.0.0 (unless error is backward compatible)
```

---

## Testing API Compatibility

CargoShip includes compatibility tests to ensure API stability:

```go
// pkg/aws/s3/compatibility_test.go

func TestPublicAPIStability(t *testing.T) {
    // Verify constructor signature hasn't changed
    var _ func(*s3.Client, config.S3Config) *Transporter = NewTransporter

    // Verify method signatures
    var tr *Transporter
    var _ func(context.Context, Archive) (*UploadResult, error) = tr.Upload

    // Verify types are compatible
    var _ Archive
    var _ UploadResult
    var _ config.S3Config
}
```

These tests fail if API contracts are broken, preventing accidental breaking changes.

---

## Migration Guides

When breaking changes are necessary, we provide comprehensive migration guides:

### Example: v0.x → v1.0 Migration Guide

```markdown
# Migrating from CargoShip v0.x to v1.0

## Breaking Changes

### 1. Upload Method Now Returns Result

**Before (v0.x)**:
\`\`\`go
err := transporter.Upload(ctx, archive)
\`\`\`

**After (v1.0)**:
\`\`\`go
result, err := transporter.Upload(ctx, archive)
if err != nil {
    // handle error
}
log.Printf("Uploaded to %s in %v", result.Location, result.Duration)
\`\`\`

**Migration**: Update all Upload calls to handle the result.

### 2. Config Validation Required

**Before (v0.x)**:
\`\`\`go
transporter := NewTransporter(client, config)
\`\`\`

**After (v1.0)**:
\`\`\`go
if err := config.Validate(); err != nil {
    log.Fatal(err)
}
transporter := NewTransporter(client, config)
\`\`\`

**Migration**: Add validation before creating transporter.
```

---

## Semantic Versioning Rules

CargoShip follows Semantic Versioning (MAJOR.MINOR.PATCH):

### MAJOR Version (v0 → v1, v1 → v2)

**Incremented When**:
- Breaking API changes
- Removing deprecated features
- Changing behavior in non-backward-compatible ways

**Example**: `v0.9.0 → v1.0.0`

### MINOR Version (v0.4 → v0.5)

**Incremented When**:
- Adding new features
- Adding new APIs
- Adding new struct fields (backward compatible)
- Deprecating features (not removing)

**Example**: `v0.4.5 → v0.5.0`

### PATCH Version (v0.4.4 → v0.4.5)

**Incremented When**:
- Bug fixes
- Security patches
- Documentation updates
- Performance improvements (behavior unchanged)

**Example**: `v0.4.4 → v0.4.5`

---

## Getting Help

### Checking API Stability

Before upgrading, check:

1. **CHANGELOG.md** - Lists all changes
2. **API Stability Badges** - In documentation
3. **Deprecation Warnings** - In code comments
4. **Migration Guides** - For major versions

### Reporting Issues

If you encounter unexpected API changes:

1. Check if the change is documented in CHANGELOG
2. File an issue: https://github.com/scttfrdmn/cargoship/issues
3. Include: version numbers, code example, expected vs actual behavior

---

## Examples for Library Consumers

### Minimal Integration (Like ObjectFS)

```go
package main

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
    cargos3 "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

func main() {
    ctx := context.Background()

    // Load AWS config
    cfg, _ := config.LoadDefaultConfig(ctx)
    client := s3.NewFromConfig(cfg)

    // Configure CargoShip
    cargoConfig := cargoconfig.S3Config{
        Bucket:             "my-bucket",
        StorageClass:       cargoconfig.StorageClassIntelligentTiering,
        MultipartThreshold: 10 * 1024 * 1024,
        Concurrency:        4,
    }

    // Create transporter
    transporter := cargos3.NewTransporter(client, cargoConfig)

    // Use transporter for optimized uploads
    archive := cargos3.Archive{
        Key:    "my-file.dat",
        Reader: /* ... */,
        Size:   1024 * 1024,
    }

    result, err := transporter.Upload(ctx, archive)
    // Handle result and error
}
```

### Version Pinning in go.mod

```go
module github.com/youruser/yourproject

go 1.21

require (
    // Pin to specific version for stability
    github.com/scttfrdmn/cargoship v0.4.5

    // Or pin to minor version for security updates
    // github.com/scttfrdmn/cargoship v0.4.x
)
```

---

## Summary

### For Library Consumers

✅ **Stable APIs** (pkg/aws/s3, pkg/aws/config):
- Safe for production use
- Backward compatible within major version
- Breaking changes only in major versions

⚠️ **Beta APIs** (cost control, multi-region):
- Production-ready but evolving
- May change with deprecation notice
- Monitor CHANGELOG

❌ **Experimental APIs** (staging, prefetching):
- Under active development
- May change without notice
- Use with caution

### Upgrade Safety

- **Patch versions** (v0.4.4 → v0.4.5): Always safe
- **Minor versions** (v0.4.x → v0.5.0): Safe for stable APIs
- **Major versions** (v0.x → v1.0): Requires migration

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| v0.4.5 | 2025-10-15 | Initial API stability guarantees document |

---

## Related Documentation

- [Versioning Guidelines](versioning.md)
- [Integration Patterns](integration-patterns.md)
- [CHANGELOG.md](../CHANGELOG.md)
- [Migration Guides](../docs/migrations/)

---

**Last Updated**: 2025-10-15
**Applies To**: CargoShip v0.4.5 and later

For questions or clarifications, please file an issue on GitHub.
