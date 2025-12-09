# CargoShip Development with Claude Code

## Project Overview

CargoShip is a high-performance S3 archiving tool featuring streaming pipeline architecture, multi-prefix parallel uploads (8× throughput), and zero-disk usage.

**Foundation**: Built on Duke University's SuitcaseCTL research with enterprise AWS optimizations.

## Current Status (2025-12-09)

**Version**: v0.5.1+ (Branch: main)

### Recently Completed - Phase 3-5 Pipeline (2025-11-26 to 2025-12-09)

**✅ Phase 3 - Streaming Pipeline Architecture (Issue #63)**
- Multi-prefix S3 sharding (8× request rate capacity)
- Zero local disk usage with io.Pipe streaming
- Bounded memory: O(chunk_size × workers)
- Real-time progress tracking with terminal detection
- **Benchmarks**: Small files 437ms (403 MB/s), Large files 311s (185 MB/s)

**✅ Phase 4 - Parallel S3 Upload Workers (Issue #64)**
- Increased default workers from 4 to 8 (matches shard count)
- **Impact**: Small files improved 10-90%, Large files unchanged (chunking bottleneck)

**✅ Phase 5 - Adaptive File Splitting (Issue #69)**
- Implementation complete with configuration bug fixes
- Per-stage instrumentation added for performance analysis
- **Finding**: File splitting is memory safety feature (not performance optimization)
- **Decision**: Disabled by default for backward compatibility

**✅ CLI Integration Complete (Issue #118)**
- `cargoship create upload` command fully functional
- TUI progress: `🚢 Uploading: N files | X GB | Y chunks | Z MB/s | elapsed`
- Terminal detection with graceful fallback to quiet mode
- JSON output mode for automation

### Active Issues
- **Issue #68**: Flaky test - TestPipeline_ErrorHandling (P2-Low)
- **Issue #65**: Goroutine leak cleanup with CPU profiling (P2-Low)
- **Issues #14, #17**: Coordinator test cleanup (P2-Medium)
- **Issue #126**: Performance investigation - 11.88 MB/s vs 185 MB/s benchmark (P1-High)

## Issue Tracking - **USE GITHUB ISSUES, NOT DOCUMENTS**

```bash
# Create issue with labels
gh issue create --title "Title" --body "Description" --label "type: bug,priority: high"

# List issues by milestone
gh issue list --label "v0.6.0" --limit 50

# View issue
gh issue view 123
```

**Available Labels**:
- **Type**: `type: bug`, `type: feature`, `type: enhancement`, `type: test`, `type: refactor`, `type: documentation`
- **Area**: `area: s3`, `area: cli`, `area: pipeline`, `area: testing`, `area: performance`, `area: docs`
- **Priority**: `priority: critical`, `priority: high`, `priority: medium`, `priority: low`
- **Effort**: `effort: small` (<4h), `effort: medium` (1-2d), `effort: large` (>2d)

## Development Roadmap

### v0.6.0 (Next) - Production Hardening
**Focus**: Performance investigation, documentation, test reliability

**High Priority**:
- Investigate performance gap (Issue #126) - 11.88 MB/s vs 185 MB/s
- Fix flaky tests (Issues #135, #68)
- Blog post series (Issue #123) - Community outreach

**Medium Priority**:
- TODO audit items (Issues #136-141)
- Coordinator test cleanup (Issues #14-17)

### v0.7.0+ - Enterprise Features
- Performance optimizations (zero-copy I/O, network tuning)
- Distributed tracing and observability
- Budget controls and lifecycle management
- Kubernetes operator

## Key Architecture Components

### Pipeline (pkg/pipeline/)
**Streaming architecture**: Scanner → Chunker → Archiver → S3 Uploader

**Features**:
- Zero disk usage - streams directly to S3
- Bounded memory - O(chunk_size × workers)
- Multi-prefix sharding - 8× S3 request rate capacity
- Real-time progress - terminal detection with graceful fallback

**Key Components**:
- `pipeline.go` - Core orchestrator
- `scanner.go` - Multi-threaded file discovery
- `archiver.go` - Streaming tar+zstd compression
- `s3_uploader.go` - Parallel uploads (8 workers)

### Manifest System (pkg/manifest/)
**Fast file queries**: List and filter uploaded files without downloading archives

**Features**:
- Glob pattern matching (`*.log`, `data/*.csv`)
- Compression statistics and shard distribution
- S3 key: `[prefix/]uploads/{upload-id}/manifest.json.gz`

**Documentation**: See `pkg/manifest/README.md` and `docs/STORAGE_FORMAT.md`

## Common Commands

### Build & Test
```bash
# Build
go build ./cmd/cargoship

# Test (skip integration)
go test ./... -short

# Test with real AWS S3
export AWS_PROFILE=aws AWS_REGION=us-west-2
export CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1
export CARGOSHIP_TEST_BUCKET=cargoship-pipeline-test
go test -v ./pkg/pipeline -timeout=30m

# Lint (target: 0 issues)
golangci-lint run ./... --timeout=120s

# Security scan
govulncheck ./...
```

### CLI Usage
```bash
# Upload with streaming pipeline
cargoship create upload /data/project \
  --bucket my-bucket \
  --prefix archive-2024 \
  --storage-class INTELLIGENT_TIERING \
  --workers 8

# List uploaded files (no download)
cargoship list --bucket my-bucket --upload-id 20251206-123456-abcd1234 --pattern "*.log"

# Cost estimation
cargoship estimate /data --storage-class DEEP_ARCHIVE

# Lifecycle policies
cargoship lifecycle --bucket my-bucket --template archive-optimization
```

### Git Workflow
```bash
# Create feature branch
git checkout -b feature/new-feature

# Commit with conventional commits
git commit -m "feat: Add new feature description"
git commit -m "fix: Resolve bug description"
git commit -m "docs: Update documentation"

# Push to remote
git push origin feature/new-feature

# Create PR
gh pr create --title "Title" --body "Description"
```

## Performance Benchmarks

**Real AWS S3 (v0.5.1)**:
- Small files (10k @ 176MB): 437ms (403 MB/s)
- Large files (100 @ 56GB): 311s (185 MB/s)
- Memory: 3.4-4.9 GB (6-8% of data size)
- Compression: zstd 527 MB/s

**Known Issue**: Production uploads showing 11.88 MB/s (16× slower than benchmark)
- See Issue #126 for investigation

## Code Quality

- Zero linting issues (golangci-lint)
- Zero security vulnerabilities (govulncheck)
- Comprehensive error handling with graceful degradation
- Thread-safe concurrent operations

## Documentation Philosophy

**Use GitHub issues for work tracking, NOT verbose planning documents.**

Only create documentation for:
- End-user guides (TROUBLESHOOTING.md, USER_GUIDE.md)
- API documentation (STORAGE_FORMAT.md, API_STABILITY.md)
- Architecture diagrams (when explicitly requested)

---

**Last Updated**: 2025-12-09
