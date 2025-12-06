# CargoShip Development with Claude Code

## Project Overview

CargoShip is a high-performance S3 file upload optimization tool featuring streaming pipeline architecture, multi-prefix parallel uploads (8× throughput), and zero-disk usage.

**Foundation**: Built on Duke University's SuitcaseCTL with enterprise AWS optimizations.

## Current Status (2025-12-05)

**Version**: v0.5.1+ (Branch: main)
**Latest Work**: Real-time TUI progress tracking for `cargoship create upload`

### Recently Completed
- ✅ **Issue #125**: BufferedPipeWriter race condition fixed (Commit a0ea650)
  - Root cause: Write() racing with CloseWithError() closing buffer channel
  - Fix: Added sync.WaitGroup to track active writers, wait before closing channel
  - Validated: Production test (100MB) completed with zero panics (80.77 MB/s)
  - Impact: Eliminates production panics during zstd compression uploads
- ✅ **Issue #118**: CLI progress tracking with terminal detection (Commit 9dd105e, 3173615)
- ✅ **Issue #120**: Legacy code removal - Removed Porter/rclone/suitcase (83 files, ~23,300 lines)
- ✅ **Issue #121**: MemoryManager goroutine leak fixed (Commit 7715f4d)
- ✅ **Issue #122**: Storage format documentation issue created
- ✅ **Issue #123**: Blog post series issue created (5 posts with full outlines)
- ✅ **Production Test 1**: Small dataset (50 files @ 10MB = 500MB)
  - Uploaded in 45.5s at 10.99 MB/s
  - Only 2 of 8 shards utilized (too few chunks created)
- ✅ **Production Test 2**: Large dataset (200 files @ 100MB = 20GB)
  - Uploaded in 28m3s at 11.88 MB/s
  - All 8 shards utilized ✅
  - 🚨 BufferedPipeWriter panic detected → Fixed in Issue #125
  - 🚨 Performance gap: 16× slower than 185 MB/s benchmark (Issue #126)

### Open Issues
- **Issue #126**: Performance investigation - 11.88 MB/s vs 185 MB/s benchmark (P1-High)
- **Issue #65**: MemoryManager goroutine leak (P2-Low) - Test-only, doesn't affect production
- **Issue #14-17**: Coordinator/test cleanup (P2-Medium) - Technical debt for v0.6.0

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

### v0.6.0 (Next) - Production Readiness
**Focus**: Documentation, community outreach, production validation

**High Priority**:
- ✅ Test `cargoship create upload` with real S3 (COMPLETE - see Recently Completed)
- Create `docs/STORAGE_FORMAT.md` (Issue #122) - Open format documentation
- Update README with new CLI command examples
- Fix MemoryManager goroutine leak (Issue #65)

**Medium Priority**:
- Blog post series (Issue #123) - 5 posts introducing CargoShip to community
- Resolve coordinator shutdown issues (Issue #14-17)

### v0.7.0 - Performance & Observability
- Go-native performance optimizations (zero-copy I/O, network tuning)
- Distributed tracing (OpenTelemetry/Jaeger)
- Circuit breaker patterns for production resilience
- Performance benchmarking suite

### v0.8.0+ - Enterprise Features
- Budget controls with grant period management
- io_uring support (Linux high-performance async I/O)
- HTTP/3 and QUIC protocol support
- Kubernetes operator for container deployments

## Key Architecture Components

### Pipeline (pkg/pipeline/)
**Streaming architecture**: Scanner → Chunker → Archiver → S3 Uploader

**Features**:
- **Zero disk usage**: Streams directly from filesystem → compression → S3
- **Bounded memory**: O(chunk_size × workers) prevents OOM
- **Adaptive chunking**: Smart file grouping based on size/compressibility
- **Multi-prefix sharding**: 8× S3 request rate capacity (Phase 3)
- **Real-time progress**: Live TUI with throughput tracking

**Key Files**:
- `pkg/pipeline/pipeline.go` - Core orchestrator
- `pkg/pipeline/scanner.go` - Multi-threaded file discovery
- `pkg/pipeline/archiver.go` - Streaming tar+zstd compression
- `pkg/pipeline/s3_uploader.go` - Parallel S3 uploads (8 workers default)

### ShardCoordinator vs Pipeline
**Pipeline**: Used by CLI (`cargoship create upload`)
- Single streaming pipeline with progress callback
- Simpler architecture for typical use cases
- Terminal detection with graceful fallback

**ShardCoordinator**: Used by benchmarks only
- Multiple ShardPipeline instances for maximum parallelism
- Per-shard compression and memory management
- Bubbletea TUI (ShardProgressRenderer) for advanced metrics

### Chunking Engine (pkg/chunking/)
**Intelligent file grouping**: Creates 200MB tar.zst chunks for S3 multipart optimization

**Strategies**:
- `groupSmall()`: Pack small files together
- `groupMixed()`: Balance small/medium/large files (includes file splitting in Phase 5)
- `groupLarge()`: Handle files >chunk_size

**Key Features**:
- Compression-aware boundary detection
- Adaptive target size calculation
- Archive padding for alignment

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
# Modern streaming upload (v0.5.1+)
cargoship create upload /data/genomics \
  --bucket my-research-bucket \
  --prefix 2024-analysis \
  --storage-class INTELLIGENT_TIERING \
  --shards 8 \
  --workers 4

# Real-time progress display:
# 🚢 Uploading: 1234 files | 5.67 GB | 89 chunks | 123.4 MB/s | 1m30s elapsed

# Cost estimation
cargoship estimate /data --storage-class DEEP_ARCHIVE --show-breakdown

# Lifecycle policy management
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

## Performance Benchmarks (v0.5.1)

**Real AWS S3 Results**:
- **Small files** (10k @ 176MB): 437ms (403 MB/s) - 23-41× faster than target
- **Large files** (100 @ 56GB): 311s (185 MB/s) - 30% over target
- **Memory usage**: 3.4-4.9 GB (6-8% of data size) - Excellent scaling
- **Compression**: zstd 527 MB/s (10.7× faster than gzip)

**Phase 5 Improvements** (Adaptive file splitting):
- Enables splitting large files across multiple chunks
- Target: <240s for 100 @ 56GB (23% improvement)
- Implementation complete (Commit b82c201), validation pending

## Known Technical Debt

Tracked in GitHub issues for transparent project management:

- **Issue #65** (P2-Low): MemoryManager goroutine leak in tests
- **Issue #14** (P2-Medium): Add Shutdown() calls to coordinator tests
- **Issue #15** (P1-Low): Fix flaky CloudWatch test
- **Issue #17** (P2-Medium): Fix TestFailoverScenarios_CrossRegionRetry timeout
- **Issue #68** (P1-Low): Fix flaky TestPipeline_ErrorHandling test

All critical bugs and test failures resolved in v0.5.0 Phase 1.

## Code Quality Standards

- **Linting**: 0 issues (golangci-lint)
- **Security**: 0 vulnerabilities (govulncheck)
- **Test Coverage**: All integration tests passing with real AWS S3
- **Go Style**: Follow official Go best practices and idiomatic patterns
- **Error Handling**: Comprehensive error handling with graceful degradation
- **Thread Safety**: Proper synchronization for concurrent operations

## Documentation Philosophy

- **CRITICAL**: Track work in GitHub issues, NOT verbose planning documents
- **Exception**: Only create docs for end-user documentation (guides, API docs, architecture diagrams)
- **Quality over quantity**: Prefer clear, actionable issues over long documents
- **Transparency**: Open development with public issue tracking

---

**Last Updated**: 2025-12-05
**Next Session**: Begin comprehensive README/documentation updates (Issue #124)
