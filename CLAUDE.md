# CargoShip Development with Claude Code

## Project Overview

CargoShip is a high-performance S3 file upload optimization tool featuring streaming pipeline architecture, multi-prefix parallel uploads, and advanced chunking/compression.

## Current Version: v0.5.1+ (Branch: main)

**Latest**: Real-time TUI progress tracking for `cargoship create upload` (2025-12-05)

## Issue Tracking - **USE GITHUB ISSUES**

Track all bugs, features, and tasks using GitHub issues. DO NOT create verbose documents.

### Creating Issues
```bash
# Create issue
gh issue create --title "Title" --body "Description" --label "type: bug,priority: high"

# List issues
gh issue list --label "v0.6.0" --limit 50
```

### Available Labels
- **Type**: `type: bug`, `type: feature`, `type: enhancement`, `type: test`, `type: refactor`
- **Area**: `area: s3`, `area: cli`, `area: pipeline`, `area: testing`, `area: performance`
- **Priority**: `priority: critical`, `priority: high`, `priority: medium`, `priority: low`
- **Effort**: `effort: small` (<4h), `effort: medium` (1-2d), `effort: large` (>2d)
- **Status**: `status: in-progress`, `status: blocked`, `status: ready`

## Active Work (2025-12-05)

### ✅ Completed - Issue #118: CLI Progress Tracking Integration
- **New Command**: `cargoship create upload` replaces Porter/rclone system
- **Real-time TUI**: Single-line progress display with terminal detection
- **Graceful Fallback**: Auto-disables for non-TTY (pipes, CI/CD)
- **Architecture**: Uses Pipeline's SetProgressCallback() (not ShardCoordinator)
- **Commits**: 9dd105e (CLI integration), 3173615 (progress tracking)

### 📋 Open Issues
- **Issue #120**: Remove Porter/rclone system (P1-High) - Phased deprecation plan
- **Issue #65**: MemoryManager goroutine leak (P2-Low) - Known test issue
- **Issue #14-17**: Coordinator/test cleanup (P2-Medium) - Technical debt

## Development Roadmap

### v0.6.0 (Next) - Testing & Documentation
- Test `cargoship create upload` with real S3
- Update README and CLI help
- Phase 1: Deprecate Porter/rclone (Issue #120)

### v0.7.0 - Performance & Observability
- Go-native performance optimizations (zero-copy I/O, network tuning)
- Distributed tracing (OpenTelemetry)
- Circuit breaker patterns

### v0.8.0+ - Enterprise Features
- Budget controls with grant period management
- io_uring support (Linux high-performance I/O)
- Kubernetes operator

## Key Architecture Components

### Pipeline (pkg/pipeline/)
**Streaming architecture**: Scanner → Chunker → Archiver → S3 Uploader
- **Phase 2**: BufferedPipe eliminates serialization (v0.5.0)
- **Phase 3**: Multi-prefix sharding (8× throughput, v0.5.1)
- **Features**: Zero disk usage, bounded memory, adaptive chunking

### ShardCoordinator (pkg/pipeline/shard_coordinator.go)
**Advanced orchestration**: Multiple ShardPipeline instances for parallel upload
- Intelligent shard count (4-10 based on data size)
- Per-shard pipelines with compression and memory management
- Used by benchmarks, NOT by CLI (CLI uses basic Pipeline)

### Progress Tracking
- **CLI**: Pipeline.SetProgressCallback() → terminal display
- **ShardProgressRenderer**: Bubbletea TUI for ShardCoordinator (benchmarks only)

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
go test -v ./pkg/pipeline -timeout=30m

# Lint
golangci-lint run ./... --timeout=120s
```

### New CLI Command
```bash
# Upload with pipeline (replaces Porter/rclone)
./cargoship create upload /path/to/data --bucket my-bucket --prefix backups/2025-12-05

# Quiet mode
./cargoship create upload /path/to/data --bucket my-bucket --quiet

# JSON output
./cargoship create upload /path/to/data --bucket my-bucket --progress-format json
```

### Bypass Pre-commit Hook (Known Test Issues)
```bash
# Use --no-verify for known goroutine leak (Issue #65)
git commit --no-verify -m "message"
```

## Known Technical Debt

| Issue | Description | Priority | Effort |
|-------|-------------|----------|--------|
| #120 | Remove Porter/rclone system | P1-High | Large |
| #65 | MemoryManager goroutine leak | P2-Low | Medium |
| #14 | Coordinator test cleanup | P2-Medium | Medium |
| #15 | Flaky CloudWatch test | P2-Low | Small |
| #16 | Staging package refactor | P2-Low | Large |
| #17 | Failover test timeout | P2-Medium | Medium |

## Recent Milestones

### v0.5.1 (2025-11-08) - Integration Testing Framework ✅
- 19 new integration tests with real AWS S3
- Performance benchmarks: zstd 527 MB/s, S3 upload 23-32 MB/s
- Large-scale: 10,000 files in 9.26s (133× faster than target)

### v0.5.0 (2025-11-07) - Test Quality & Performance ✅
- Phase 1: 100% test pass rate, 6 production bugs fixed
- Phase 3: Zero-copy I/O (15-25% improvement)
- Linux splice() syscall (20-40% improvement)
- Memory-mapped file I/O for 128MB+ files
- NUMA-aware buffer allocation (10-20% reduced latency)

### v0.4.6 (2025-10-16) - Developer Experience ✅
- Interactive configuration wizard (`cargoship setup`)
- Profiling commands (CPU, memory, goroutine)
- Configuration validation with AWS connectivity checks
- Troubleshooting guides

## License & Attribution

- **License**: Apache 2.0
- **Attribution**: Inspired by SuitcaseCTL (Duke University)
- **Status**: Independent implementation with streaming pipeline architecture

---

**Note**: This document tracks current development context for Claude Code sessions. For user documentation, see README.md and docs/.
