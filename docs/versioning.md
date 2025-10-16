# CargoShip Versioning Guidelines

**Version**: v0.4.5+
**Last Updated**: 2025-10-15

## Overview

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/) (SemVer) to provide clear expectations about compatibility and changes between versions.

**Format**: `MAJOR.MINOR.PATCH` (e.g., v0.4.5)

---

## Semantic Versioning Rules

### MAJOR Version (Breaking Changes)

**Format**: `vX.0.0` (e.g., v1.0.0)

**Incremented When**:
- Breaking changes to public APIs
- Removing deprecated features
- Changing behavior in non-backward-compatible ways
- Removing or renaming public functions/methods
- Changing function signatures
- Removing struct fields
- Changing error behavior that breaks documented contracts

**Examples**:
```go
// v0.9.0 → v1.0.0 (MAJOR bump required)

// Before (v0.9.0)
func Upload(ctx context.Context, archive Archive) error

// After (v1.0.0) - BREAKING: Changed return type
func Upload(ctx context.Context, archive Archive) (*UploadResult, error)
```

**Consumer Impact**: 🔴 **HIGH** - Code changes required

**Migration**: Requires updating consumer code, migration guide provided

---

### MINOR Version (New Features)

**Format**: `vX.Y.0` (e.g., v0.5.0)

**Incremented When**:
- Adding new features
- Adding new public functions/methods
- Adding new struct fields (with sensible zero-value defaults)
- Deprecating features (marking for future removal)
- Adding new packages
- Enhancing functionality in backward-compatible ways

**Examples**:
```go
// v0.4.5 → v0.5.0 (MINOR bump)

// Before (v0.4.5)
type Archive struct {
    Key  string
    Size int64
}

// After (v0.5.0) - ADDED: New optional field
type Archive struct {
    Key         string
    Size        int64
    ContentType string // New field, zero value works
}

// New method added
func (t *Transporter) UploadWithProgress(
    ctx context.Context,
    archive Archive,
    progress chan<- ProgressUpdate,
) (*UploadResult, error)
```

**Consumer Impact**: 🟡 **MEDIUM** - Optional upgrades to use new features

**Migration**: No breaking changes, new features available immediately

---

### PATCH Version (Bug Fixes)

**Format**: `vX.Y.Z` (e.g., v0.4.5)

**Incremented When**:
- Bug fixes
- Security patches
- Performance improvements (behavior unchanged)
- Documentation updates
- Internal refactoring (no API changes)
- Test improvements
- Dependency updates (non-breaking)

**Examples**:
```go
// v0.4.4 → v0.4.5 (PATCH bump)

// Before (v0.4.4) - Bug: timeout errors not handled correctly
func (t *Transporter) Upload() error {
    // Bug: didn't handle "context deadline exceeded"
}

// After (v0.4.5) - Fixed: handles both timeout error formats
func (t *Transporter) Upload() error {
    // Fix: now handles both "timed out" and "context deadline exceeded"
}
```

**Consumer Impact**: 🟢 **LOW** - Safe to upgrade immediately

**Migration**: Drop-in replacement, no code changes needed

---

## Pre-1.0 Versioning (Current: v0.x.x)

CargoShip is currently in **v0.x.x** (pre-1.0), which has special SemVer rules:

### v0.x.x Stability

**v0.MINOR.PATCH** follows slightly different rules:

- **v0.MINOR**: Can include breaking changes (with clear documentation)
- **v0.PATCH**: Bug fixes only, fully backward compatible

**Example**:
```
v0.4.5 → v0.5.0: May include minor breaking changes (documented)
v0.4.4 → v0.4.5: Always backward compatible
```

### Current Approach

Despite being v0.x.x, CargoShip treats **Stable APIs** as production-ready:
- ✅ **Stable APIs** (pkg/aws/s3, pkg/aws/config): Treated as if v1.0+
- ⚠️ **Beta APIs**: May change in minor versions
- ❌ **Experimental APIs**: May change in any version

**Commitment**: We avoid breaking stable APIs even in v0.MINOR bumps.

---

## Version Lifecycle

### Active Support

| Version Type | Support Duration | Updates |
|--------------|------------------|---------|
| **Latest** | Indefinite | All updates |
| **Previous MINOR** | 3 months | Security patches only |
| **Older versions** | None | Upgrade recommended |

**Example** (as of v0.5.0 release):
- v0.5.x: Full support
- v0.4.x: Security patches for 3 months
- v0.3.x: No support, upgrade required

### End of Life (EOL)

When a version reaches EOL:
1. Announced 30 days in advance
2. Security patches stop
3. Issues closed with "upgrade required"
4. Documentation marked as archived

---

## What Constitutes a Breaking Change?

### Breaking Changes ❌ (Require MAJOR bump)

1. **Removing Public APIs**
   ```go
   // v0.x.0
   func OldMethod() error // Exists

   // v1.0.0
   // OldMethod removed - BREAKING
   ```

2. **Changing Function Signatures**
   ```go
   // v0.x.0
   func NewTransporter(client *s3.Client, config S3Config) *Transporter

   // v1.0.0
   func NewTransporter(client *s3.Client, config S3Config, logger *Logger) *Transporter
   // BREAKING: Added required parameter
   ```

3. **Changing Return Types**
   ```go
   // v0.x.0
   func Upload() error

   // v1.0.0
   func Upload() (*Result, error) // BREAKING: Changed return type
   ```

4. **Removing Struct Fields**
   ```go
   // v0.x.0
   type Config struct {
       Bucket string
       Region string
   }

   // v1.0.0
   type Config struct {
       Bucket string
       // Region removed - BREAKING
   }
   ```

5. **Changing Behavior**
   ```go
   // v0.x.0: Returns error if file doesn't exist
   func Download() error

   // v1.0.0: Returns nil if file doesn't exist (different behavior)
   func Download() error // BREAKING: Changed documented behavior
   ```

### Non-Breaking Changes ✅ (MINOR or PATCH)

1. **Adding New Methods**
   ```go
   // v0.4.0
   func Upload() error

   // v0.5.0
   func UploadAsync() error // NEW METHOD - OK
   ```

2. **Adding Struct Fields**
   ```go
   // v0.4.0
   type Config struct {
       Bucket string
   }

   // v0.5.0
   type Config struct {
       Bucket      string
       Concurrency int // NEW FIELD with zero-value default - OK
   }
   ```

3. **Performance Improvements**
   ```go
   // v0.4.4
   func Upload() error { /* slow */ }

   // v0.4.5
   func Upload() error { /* 2x faster, same behavior */ } // OK
   ```

4. **Bug Fixes**
   ```go
   // v0.4.4: Bug - doesn't retry on network errors
   func Upload() error

   // v0.4.5: Fixed - now retries properly
   func Upload() error // OK - fixing incorrect behavior
   ```

5. **Documentation Updates**
   ```go
   // v0.4.4
   // Upload uploads a file
   func Upload() error

   // v0.4.5
   // Upload uploads a file to S3 with automatic retry
   // Returns error if upload fails after 3 retries
   func Upload() error // OK - better documentation
   ```

---

## Deprecation Process

### Marking as Deprecated

**Step 1**: Add deprecation comment
```go
// Deprecated: Use NewMethod instead. This will be removed in v1.0.0.
// See: https://docs.cargoship.dev/migration/old-method
func OldMethod() error {
    // Still functional
}
```

**Step 2**: Update documentation
```markdown
## Deprecated Methods

### OldMethod (Deprecated in v0.5.0)
**Replacement**: NewMethod
**Removal**: v1.0.0
**Migration**: [See guide](migration/old-method.md)
```

**Step 3**: Provide migration path
```go
// NewMethod is the replacement for deprecated OldMethod
func NewMethod() error {
    // New implementation
}

// OldMethod calls NewMethod internally (backward compatibility)
// Deprecated: Use NewMethod instead.
func OldMethod() error {
    return NewMethod() // Delegate to new method
}
```

### Deprecation Timeline

```
Version N   : Feature deprecated, warning added
Version N+1 : Still available, documented as deprecated
Version N+2 : Still available, deprecation warnings in logs
Version N+3 : Removed (usually in next MAJOR version)
```

**Minimum Timeline**: 2 MINOR versions or 6 months, whichever is longer

**Example**:
```
v0.5.0 (Oct 2025): OldMethod deprecated
v0.6.0 (Dec 2025): Still available
v0.7.0 (Feb 2026): Still available
v1.0.0 (Apr 2026): Removed
```

---

## Version Numbering Guidelines

### Pre-releases

For testing and development:

**Alpha**: `v0.5.0-alpha.1`
- Early development
- APIs may change significantly
- Not for production use

**Beta**: `v0.5.0-beta.1`
- Feature complete
- APIs stabilizing
- Ready for testing, not production

**Release Candidate**: `v0.5.0-rc.1`
- Production-ready
- No planned changes
- Final testing before release

### Build Metadata

Optional metadata: `v0.5.0+build.123`
- Build number or commit SHA
- Doesn't affect version precedence

---

## Version Comparison

### Precedence Rules

```
v0.4.4 < v0.4.5 < v0.5.0-alpha.1 < v0.5.0-beta.1 < v0.5.0-rc.1 < v0.5.0 < v0.5.1 < v0.6.0 < v1.0.0
```

### Version Constraints (go.mod)

**Exact Version**:
```go
require github.com/scttfrdmn/cargoship v0.4.5
```

**Minimum Version**:
```go
require github.com/scttfrdmn/cargoship v0.4.5
// Will use v0.4.5 or higher (controlled by go.sum)
```

**Version Ranges** (via go.mod replace or third-party tools):
```
>= v0.4.0, < v0.5.0  // v0.4.x only
>= v0.4.5            // v0.4.5 or higher
```

---

## Release Checklist

### For Maintainers

#### PATCH Release (v0.4.4 → v0.4.5)

- [ ] All bug fixes tested
- [ ] No API changes
- [ ] No new features
- [ ] Update CHANGELOG.md
- [ ] Run full test suite
- [ ] Tag release: `git tag -a v0.4.5 -m "Release v0.4.5"`
- [ ] Push tag: `git push origin v0.4.5`

#### MINOR Release (v0.4.5 → v0.5.0)

- [ ] New features documented
- [ ] API additions documented
- [ ] Deprecations marked
- [ ] Migration guide (if needed)
- [ ] Update CHANGELOG.md
- [ ] Update API stability docs
- [ ] Run full test suite + benchmarks
- [ ] Tag release: `git tag -a v0.5.0 -m "Release v0.5.0"`
- [ ] Push tag: `git push origin v0.5.0`
- [ ] Announce on GitHub Releases

#### MAJOR Release (v0.9.0 → v1.0.0)

- [ ] All breaking changes documented
- [ ] Comprehensive migration guide
- [ ] Deprecation warnings resolved
- [ ] API compatibility tests updated
- [ ] Update all documentation
- [ ] Update examples
- [ ] Announcement blog post
- [ ] Tag release: `git tag -a v1.0.0 -m "Release v1.0.0"`
- [ ] Push tag: `git push origin v1.0.0`
- [ ] Announce widely

---

## Version Decision Tree

```
┌─ Does it break existing APIs?
│  ├─ YES → MAJOR version bump (v1.0.0)
│  └─ NO  → Continue
│
├─ Does it add new features/APIs?
│  ├─ YES → MINOR version bump (v0.5.0)
│  └─ NO  → Continue
│
├─ Does it fix bugs or improve performance?
│  ├─ YES → PATCH version bump (v0.4.5)
│  └─ NO  → Documentation/internal only (no version change)
```

---

## Best Practices for Consumers

### Stay Current

**Recommended**: Update to latest PATCH version regularly
```bash
go get github.com/scttfrdmn/cargoship@v0.4.x
go mod tidy
```

### Test Before Upgrading MINOR

**Process**:
1. Read CHANGELOG for v0.5.0
2. Check for new features you can use
3. Test in development environment
4. Update production after validation

### Plan for MAJOR Upgrades

**Process**:
1. Review migration guide
2. Create feature branch
3. Update code following migration guide
4. Run full test suite
5. Deploy to staging
6. Monitor for issues
7. Deploy to production

### Version Pinning Strategy

**Conservative** (Maximum Stability):
```go
require github.com/scttfrdmn/cargoship v0.4.5
// Exact version, no automatic updates
```

**Balanced** (Recommended):
```go
require github.com/scttfrdmn/cargoship v0.4.5
// go.sum will lock to specific version
// Update manually: go get -u github.com/scttfrdmn/cargoship
```

**Aggressive** (Latest Features):
```go
require github.com/scttfrdmn/cargoship v0.4.5
// Then: go get -u github.com/scttfrdmn/cargoship@latest
// Only for non-production or if you trust the project
```

---

## FAQ

### Q: When will v1.0.0 be released?

**A**: When APIs are stable and production-proven. Current estimate: Q2 2026.

### Q: Can I use v0.x.x in production?

**A**: Yes! Stable APIs (pkg/aws/s3, pkg/aws/config) are production-ready despite v0.x versioning.

### Q: What if a PATCH breaks my code?

**A**: File an issue immediately. PATCH versions should never break code. We'll either:
1. Revert the change
2. Issue a new PATCH to fix
3. Acknowledge it as a bug in previous version

### Q: How long are old versions supported?

**A**: Previous MINOR version gets security patches for 3 months after new MINOR release.

### Q: Should I pin to exact versions?

**A**: Recommended approach:
- Development: Use `v0.4.x` for latest patches
- Production: Pin exact version, test updates before deploying

### Q: What about go.mod indirect dependencies?

**A**: We maintain stable dependencies. Breaking changes in our dependencies are absorbed and don't affect our API guarantees.

---

## Examples from CargoShip History

### PATCH: v0.4.3 → v0.4.4

**Changes**:
- Fixed multiregion failover test assertion
- Added AWS CRT evaluation documentation

**Why PATCH**: No API changes, bug fix + docs

### MINOR: v0.3.x → v0.4.0

**Changes**:
- Added multi-region coordinator
- Added 8 load balancing strategies
- New packages: pkg/multiregion

**Why MINOR**: New features, backward compatible

### MAJOR: v0.x → v1.0 (Future)

**Planned Changes**:
- Upload() returns (*Result, error) instead of error
- Required config validation
- Remove deprecated v0.x features

**Why MAJOR**: Breaking changes to public APIs

---

## Related Documentation

- [API Stability Guarantees](api-stability.md)
- [Integration Patterns](integration-patterns.md)
- [CHANGELOG.md](../CHANGELOG.md)
- [Migration Guides](../docs/migrations/)

---

## Contributing

When contributing to CargoShip:

1. **Check** if your change is breaking
2. **Label** PRs correctly:
   - `breaking`: Requires MAJOR bump
   - `feature`: Requires MINOR bump
   - `bugfix`: Requires PATCH bump
3. **Update** CHANGELOG.md
4. **Add** tests ensuring backward compatibility

---

**Last Updated**: 2025-10-15
**Applies To**: CargoShip v0.4.5 and later

For questions about versioning, please file an issue on GitHub.
