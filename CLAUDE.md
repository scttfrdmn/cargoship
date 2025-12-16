# CargoShip Development with Claude Code

## Project Context
CargoShip is a high-performance S3 upload optimization tool for large-scale data transfers with advanced staging, compression, chunking, and multi-region capabilities.

**Current Version**: v0.6.0 (Released 2025-12-09)

## Response Style
- Concise by default. No explanations unless asked.
- No file creation unless explicitly requested.
- Fix bugs silently unless cause is non-obvious.

## Go Standards
- Go 1.23+ with modules
- `gofmt`, `goimports` on all code
- Pass `go vet`, `staticcheck`, `golangci-lint` before done
- Godoc comments on all exported identifiers
- No `panic` except unrecoverable init failures

## Code Style
- Idiomatic short names: `r` for reader, `ctx` for context, `err` for error
- Wrap errors with `fmt.Errorf("operation: %w", err)`
- Return early on errors; avoid deep nesting
- Prefer standard library over dependencies
- Group imports: stdlib, external, internal

## CLI Patterns
- Use `cobra` for CLI structure
- Flags over args when >1 input
- Exit codes: 0=success, 1=error, 2=usage error
- Stderr for errors/logs, stdout for output
- Support `--json` output where applicable

## AWS SDK
- Use `aws-sdk-go-v2`
- Load config with `config.LoadDefaultConfig(ctx)`
- Always pass context for cancellation
- Wrap SDK errors with operation context
- Use pagination helpers for list operations

### AWS Integration Testing (CargoShip-Specific)
Enable real AWS S3 integration tests with environment variables:
```bash
export CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1
export CARGOSHIP_TEST_BUCKET=cargoship-test-bucket
export AWS_REGION=us-west-2
export AWS_PROFILE=your-profile
go test -v -tags=integration ./pkg/aws/s3 -timeout=30m
```

Tests automatically create and clean up test buckets. Use custom bucket name via `CARGOSHIP_TEST_BUCKET`.

## Testing
- Minimum 60% coverage, target 80%+
- Table-driven tests as default
- Use `t.Helper()` in test helpers
- Mock AWS with interfaces, not SDK mocks
- Test error paths, not just happy path
- Use `testdata/` for fixtures
- Golden files for complex output verification

## Security
- Never log credentials or tokens
- Use `golang.org/x/crypto` for cryptographic operations
- Validate all external inputs
- Sanitize before logging user-provided data
- Budget alert credentials: Email SMTP passwords, Slack webhook URLs (opt-in, disabled by default)

## Git & GitHub
- Use `gh` CLI for all GitHub operations
- Conventional commits: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`
- Branch naming: `feat/`, `fix/`, `refactor/` prefixes
- PR per feature/fix; link to issue
- Close issues via commit message: `Fixes #123`, `Closes #123`

## Pre-commit Checks
- Run before every commit: `gofmt`, `go vet`, `staticcheck`
- Smoke tests: `go test -short ./...`
- Zero linting issues required

## Project Tracking
- Track all work via GitHub Issues: https://github.com/scttfrdmn/cargoship/issues
- Use GitHub Projects for planning/status
- Use Milestones for releases/versions
- Create labels as needed if no good existing match

## Do Not
- Create README, docs, or configs unless asked
- Add dependencies without justification
- Use `interface{}` or `any` without reason
- Ignore returned errors
- Use global state

## CargoShip Architecture (Key Components)

### Pipeline (pkg/pipeline/)
Streaming architecture: Scanner → Chunker → Archiver → S3 Uploader
- Zero local disk usage (io.Pipe streaming)
- Bounded memory: O(chunk_size × workers)
- Multi-prefix S3 sharding (8× request rate capacity)
- Adaptive shard count: Automatically optimizes S3 prefix count based on workload size, file count, and system resources (Issue #106)

### Budget & Cost Management (pkg/aws/cost/)
- Dual budget controls: Cost budgets (USD) + volume quotas (GB)
- Project-based tracking: Each manifest upload ID = project
- ML forecasting: 4 models (linear, exponential, moving_average, ensemble)
- Multi-channel alerts: Email (SMTP), Slack, CloudWatch, custom webhooks

### Chunking Engine (pkg/chunking/)
Intelligent content-aware chunking with compression estimation and archive padding.

### Multi-Region (pkg/multiregion/)
Advanced load balancing, health checking, and automatic failover for S3 uploads.

## CLI Commands Reference

### Core Operations
```bash
# Upload with automatic shard count (default behavior)
cargoship upload <path> s3://<bucket>/<prefix>

# Upload with manual shard count (override auto-detection)
cargoship upload <path> s3://<bucket>/<prefix> --shard-count 16

# Estimate costs
cargoship estimate <path> --storage-class GLACIER_IR
```

### Shard Count Optimization (v0.7.0, Issue #106)
Shard count is now automatically optimized by default based on:
- File count (1 shard per 10k files)
- Total compressed size (1 shard per 10 GB)
- Available CPU cores (1 shard per 2 cores)
- Available memory constraints
- Load balancing (minimum 6 chunks per shard)

**Range**: 4-32 shards (enforces minimum parallelism, caps maximum overhead)

```bash
# Auto mode (default, shard-count=0)
cargoship upload ./mydata s3://mybucket/backup

# Manual override
cargoship upload ./mydata s3://mybucket/backup --shard-count 20
```

### Budget Management (v0.6.0)
```bash
cargoship budget set --max-budget 1000 --max-volume-gb 500
cargoship budget status
cargoship cost projects
cargoship cost forecast --model ensemble
cargoship alerts configure email --smtp-host smtp.gmail.com
```

See full documentation in `docs/` for detailed usage.
