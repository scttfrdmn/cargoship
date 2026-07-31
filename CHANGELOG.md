# Changelog

All notable changes to CargoShip will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.16.0] - 2026-07-31

The **Trust & Verifiability** release (#270). CargoShip's core promise is that
what you restore is byte-identical to what you uploaded. This release makes that
claim continuously and independently checkable — a data-level verify, an open
and drift-checked format, an adversarial round-trip run against real S3 every
release, and a published dated verification report — rather than an assurance
you have to take on faith. Along the way this work caught and fixed a data-loss
bug and a path-traversal bug (see Fixed).

### Added
- **Data-level `verify --deep` (#271).** `verify` previously only checked that a
  manifest was internally coherent; it never re-read stored bytes. `--deep` now
  re-downloads each chunk and recomputes its SHA-256 against the manifest, and
  extracts and hashes every file, so checksum verification is a first-class
  PASS/FAIL. Every upload records SHA-256 at two levels: per chunk (the exact
  stored `.tar.zst` bytes) and per file. Per-file checksums are on by default;
  opt out with `upload --no-file-checksums`.
- **Verify-on-restore (#283).** Restore now verifies each file's SHA-256 as it
  writes and refuses to emit corrupt bytes — it fails the file instead of
  silently returning whatever S3 handed back. On by default; `restore
  --no-verify` opts out. Covers both direct and chunked storage paths.
- **Open, machine-checkable archive format (#274).** A JSON Schema for the
  manifest (`pkg/manifest/schema.json`, embedded), a struct↔schema drift guard,
  a dependency-free draft-07 validator that checks the *real* uploaded manifest,
  version-compat fixtures, and an independent-reader test that parses archives
  using only the standard library + zstd — proving the format is not locked to
  CargoShip's own code.
- **Whole-pipeline round-trip property test (#281).** A deliberately hostile
  corpus (empty/large files, incompressible and highly-compressible content,
  deep nesting, unicode / spaces / dotfile names) is uploaded through the real
  pipeline and restored through the real restore path, then compared
  byte-for-byte by SHA-256 — across both direct and chunked storage.
- **Real-AWS integration lane (#279, #290, #291).** The credential-gated
  integration suite now runs against real S3 in a dedicated `cargoship-dev`
  account via GitHub OIDC (no long-lived keys), on a weekly canary, on every
  release tag, and on demand. This is the standing, continuous evidence behind
  the integrity claim.
- **Published per-release verification report (#299).** Each release attaches a
  dated report (`vX.Y.Z-YYYY-MM-DD.md`) to its GitHub Release, stating how many
  files and bytes round-tripped byte-identical across both storage paths and
  which integration suites passed on real S3. It is generated from the exact
  release-gating test run, so the published numbers cannot drift from what
  actually passed. See the new [Integrity model](https://cargoship.app/project/integrity)
  and [Verification reports](https://cargoship.app/project/verification-reports)
  pages.
- **Dataset-relative restore layout + `--flatten` (#287).** Restores now
  reconstruct paths relative to the upload root by default; `--flatten` writes
  basenames into the destination for targeted restores.
- **Staging performance trend & anomaly analysis (#140).** Trend and anomaly
  detection over the upload-outcome corpus introduced in v0.15.0.

### Changed
- Restore writes a consistent, escape-safe layout across direct and chunked
  modes (#282).

### Fixed
- **Data-loss in compressed chunks (#275).** The archiver closed its pipe in the
  wrong order, so the final zstd frame was not flushed and the last file in
  every compressed chunk was silently truncated. The chunk-level checksum did
  not catch it; the new per-file verify did. Fixed the close order and added a
  regression test.
- **Path traversal on restore (#282).** A manifest entry with an absolute path
  or `..` components could escape the restore destination directory. Both write
  paths now sanitize entry paths and verify containment.
- **Chunked restore of full-URL S3 keys (#281, #273).** Chunked restore passed a
  stored full-URL key to `GetObject` verbatim and failed every file; manifest
  S3 keys are now kept portable (prefix-relative), with a defensive normalizer
  for older manifests.
- CI: `go.sum` tidied in the library-usage example modules for govulncheck (#285).

## [0.15.0] - 2026-07-23

### Added
- **Per-upload outcome history — the measurement corpus (#261).** An opt-in,
  durable, append-only record of each completed upload joins its inputs
  (dataset size, file count, file-type mix, chunk/shard counts, compression
  algorithm/level, storage class, region) to its outcomes (actual compression
  ratio, throughput, duration, error count, cost). Off by default; enable with
  `CARGOSHIP_UPLOAD_HISTORY` or `cost_control.upload_history_location`. Metadata
  only — no file content, names, or paths. Inspect it with `cargoship cost
  history` (table or `--json`). This is the substrate later optimization work
  learns from.
- **Persistent staging compression-ratio history (#262).** The staging
  compression predictor's online learner now survives restarts (opt-in via
  `CARGOSHIP_COMPRESSION_HISTORY`), so predictions converge across runs instead
  of starting cold each process. Decay-window pruning and the per-content-type
  cap are honored on load.

### Changed
- **Real exponential & moving-average cost forecasts (#263).** `cargoship cost
  forecast` previously advertised four models but only linear was implemented —
  the other two silently fell back to linear, which also made the "ensemble"
  a linear forecast in disguise. Exponential now uses Holt's double exponential
  smoothing (level + trend); moving average uses an exponentially-weighted
  7-day window; the ensemble genuinely blends the three distinct components.

### Fixed
- CI: regenerated the CLI reference for the #246 budget flags (`--global`,
  `--store`) and made the fuzz lane deterministic (execution-count budget
  instead of a wall-clock deadline that could spuriously time out) (#266).

## [0.14.0] - 2026-07-23

### Added
- **Durable, shareable budgets (#246).** Project spend/volume is now recorded on
  every upload and persisted, so `cargoship budget status` reflects real spend
  across restarts. Budgets can be stored in S3 (`budget --store s3://bucket/prefix`)
  with optimistic-concurrency (ETag/If-Match) writes so a laptop, CI, and
  teammates converge without lost updates. Adds an org/team-wide budget ceiling
  (`budget set --global`) enforced across all projects on top of per-project caps.
- Live AWS Price List API integration for S3 storage & request pricing, replacing the hardcoded-only fallback path (#235)
- First-class VitePress documentation site at cargoship.app, replacing the mkdocs tree (#216)
- Versioned docs: `latest` (root) + `dev` (/dev) trees with a version switcher (#231)
- Benchmark methodology page plus reproducible provenance recorded by the benchmark runner (#230)
- Tutorial: millions of small files & the S3 request-cost problem (#236)

### Fixed
- **S3 request pricing is now storage-class-aware everywhere (#237, #252).**
  Consolidated three drifting fallback price tables into one canonical source
  (`pkg/aws/pricingfallback`), fixing a second live instance of the 10× PUT-price
  bug and correcting archival PUT costs (Glacier/Deep Archive) in both the
  fallback and live Price List API paths.
- Corrected the S3 PUT request fallback price: $0.0005 → $0.005 per 1,000 requests (10× error) (#233)
- Restore & verify now work for direct-upload archives (the documented default path was broken) (#228, #229)
- Project budgets now persist across CLI invocations (#241, superseded by #246)
- `dvc` list_files no longer calls a broken `cargoship list` contract (#219)
- Recorded first-packet send time in the BBR prober (#232)

### Testing & CI
- Integration suite now runs in CI against the in-process Substrate emulator (credential-free); repaired rotted integration tests (#240)
- End-to-end coverage for `estimate`/`cost`/`list`/`download`/`sync`/`dvc`, which surfaced and fixed two bugs (#242)
- Comprehensive test-harness build-out (#238): shared fixture builder + golden files (#251), fuzz targets for the manifest & chunking formats (#249), a scheduled real-AWS integration lane (#248), and a monotonic per-package coverage ratchet enforced in CI (#250)
- Pilot mutation testing (gremlins) on the cost/manifest/chunking packages to surface weak assertions (#256)
- Corrected the large-file memory assertion in `TestIntegration_LargeFiles`: the reported multi-GB usage was an emulator measurement artifact, not a product regression — real-S3 memory is a flat ~12 MB for a 5 GB file (#239, #245)
- Fixed data races in the pipeline progress counter (#234) and the `TestWaitForRestore` mock client (#220)
- CI speedup: dropped the redundant Test job and run integration with `-short` (#243)

## [0.13.2] - 2026-07-07

### CI/CD & Security
- Repaired CI, which had failed on every job since February: integration tests imported the Substrate module root instead of the `github.com/scttfrdmn/substrate/emulator` package, breaking `go mod tidy` and leaving `go.sum` incomplete. Imports now target the emulator package; the local-path `replace` directive was dropped and `substrate` pinned to `v0.71.0`.
- Added `.github/workflows/security.yml`: govulncheck (pinned v1.3.0), gitleaks, Trivy (filesystem + config), and Semgrep, all SHA-pinned. Removed the duplicate govulncheck-only job from `test.yml`.
- SHA-pinned every action across all active workflows; routed `${{ inputs }}` through `env` in `performance.yml` to close a shell-injection vector.
- Added Dependabot cooldown and enabled Dependabot alerts/automated security fixes.
- Added `.gitleaks.toml` and `.semgrepignore` to allowlist documentation/test/example false positives.

### Fixed
- Populate manifest `S3Key`/`ShardID`/`ChunkID` in pipeline direct-upload mode (#205)
- Guard against a nil-pointer panic in `upload.go` when `pipe.Run` returns a nil result on the error path

### Security Hardening
- Bumped `go-git/v5` v5.16.5 → v5.19.1 (resolves 5 CVEs including an RCE, plus GO-2026-5496)
- Bumped `aws-sdk-go-v2/service/s3` v1.82.0 → v1.97.3 (resolves GO-2026-5764 in the eventstream protocol)
- Bumped transitive Python dependencies (cryptography, orjson, urllib3, dulwich) in `dvc-cargoship` via uv `constraint-dependencies`; dropped EOL Python 3.9
- WebSocket upgraders in `pkg/controller` and `pkg/launch` now enforce same-origin checks (CWE-352); dashboard derives `ws`/`wss` from the page scheme
- `Dockerfile.controller` runs as a non-root user; compose services set `no-new-privileges`

## [0.13.1] - 2026-03-18

### Test Infrastructure
- Migrated all remaining integration test packages from LocalStack to in-process Substrate emulator (`github.com/scttfrdmn/substrate`): `cmd/cargoship/cmd`, `pkg/pipeline`, `pkg/manifest`, `pkg/s3optimization`, `tests/integration/dvc`
- All integration tests now run without Docker or external AWS services by default; real-AWS path retained via `CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1`
- Removed LocalStack endpoint hard-codes (`localhost:4566`) from all test files
- Fixed nil-pointer dereferences in manifest integration tests (`aws.ToString` for optional response fields)

## [0.13.0] - 2026-02-19

### DVC Pipeline Auto-Discovery
- `BuildFileStageIndex` in `pkg/manifest`: parses `dvc.yaml` and returns a map of output path → stage name; directory outputs stored with trailing "/" for prefix matching
- `AnnotateFilesWithDVCStages` in `pkg/manifest`: walks `[]FileEntry` and sets `DVCMetadata.Stage` for every file that matches a stage output; graceful no-op when `dvc.yaml` is absent
- `cargoship upload --dvc-auto`: auto-discovers DVC stages from `dvc.yaml` and annotates each `FileEntry` with its stage name; re-uploads manifest to S3 so stage-aware commands work correctly
- `cargoship dvc stages <S3_URL>`: prints stage → file-count table from manifest DVCMetadata
- `cargoship dvc status <LOCAL_PATH> <S3_URL>`: compares local files against manifest by content hash; reports `unchanged`, `modified`, or `missing` per file
- `cargoship dvc export <S3_URL> [OUTPUT_DIR]`: downloads manifest and generates `.dvc` sidecar files via existing `GenerateDVCFiles`

## [0.12.0] - 2026-02-19

### Archive Filesystem Shell (Issue #203)
- `cargoship shell s3://bucket/prefix` — interactive filesystem shell for CargoShip archives
  - Navigate archive structure: `ls`, `cd`, `pwd` — no extraction required
  - Inspect files: `stat` (size, hash, chunk, DVC stage, git commit), `cat`, `head`
  - Search: `find <glob>` searches full path and basename
  - DVC-aware: `stage list` shows all pipeline stages; `stage <name>` lists stage files
  - On-demand extraction: `get <file> [dest]` restores a single file to local disk
  - Falls back to generic CargoShip REPL when called with no arguments
- New `pkg/archivefs` package: virtual filesystem tree built from manifest paths

### Documentation (Issue #204)
- CLI_REFERENCE.md: added Data Retrieval Commands section (restore, restore jobs, browse);
  expanded cargoship shell entry to cover archive filesystem mode; version bumped to v0.11.0
- USER_GUIDE.md: added Retrieving Archived Data section covering shell browsing, selective
  restore, TUI browse, Glacier async workflow, and restore job management
- mkdocs.yml: added Data Retrieval nav entry

## [0.11.0] - 2026-02-19

### Enhanced Data Retrieval (Issues #200–#202)
- S3 Glacier/Deep Archive pre-flight check and restore orchestration (Issue #200)
  - `GlacierRestorer` with `CheckAndRestore` — HeadObject checks, RestoreObject requests, Expedited/Standard/Bulk tiers
  - `WaitForRestore` — polls until all chunks accessible, with progress callback
  - `EstimateRetrievalCost` — approximate USD fees by storage class and tier
  - New `ChunkKeysForPaths`, `ChunkKeysForDVCStage`, `ChunkKeysForCommit`, `AllChunkKeys` helpers on `SelectiveExtractor`
- Quota-aware restore with `--max-restore-cost` flag (Issue #201)
- `cargoship restore` new flags: `--tier`, `--wait`, `--dry-run`, `--max-restore-cost`, `--restore-days`
- `cargoship browse` new flags: `--tier`, `--wait`, `--max-restore-cost`, `--restore-days`
- Restore job management for async Glacier restores (Issue #202)
  - `pkg/restore` — Job persistence layer (`~/.cargoship/restore-jobs/`, XDG_DATA_HOME aware)
  - `cargoship restore jobs list` — tabular view of all queued/in-progress/completed jobs
  - `cargoship restore jobs check [job-id]` — poll S3 and mark jobs ready when chunks accessible
  - `cargoship restore jobs download <job-id>` — download files once Glacier restore completes
  - `cargoship restore jobs clean [--older-than 24h]` — remove old completed/failed jobs
  - When Glacier restore is pending without `--wait`, job is auto-saved with instructions

## [0.10.0] - 2026-02-19

### DVC Integration (Issues #171–#192)
- Core DVC remote interface and Python package (`dvc-cargoship`) (Issue #181)
- DVC `.dvc` file generation for tracked datasets (Issue #180)
- MD5 content hashing with persistent hash cache (Issue #177)
- Git metadata extraction for manifest v2.0 (Issue #178)
- IncrementalScanner with MD5-based change detection (Issue #179)
- Performance helpers for DVC remote — batching and parallel restore (Issue #182)
- Budget integration for DVC operations (Issue #183)
- PyPI publication and integration tests for `dvc-cargoship` (Issue #184)
- DVC pipeline metadata extraction (Issue #185)
- DVC stage and git commit cost tracking (Issue #186)
- Federal compliance report generation for NSF/NIH grants (Issue #187)
- Hash-based manifest query API with DVC-aware lookups (Issue #188)
- Selective chunk extraction and batch restore with LRU cache (Issue #189)
- Interactive TUI file browser for selective restore (Issue #190)
- End-to-end DVC integration test suite (Issue #191)
- DVC performance benchmark suite (Issue #192)

### Security
- Remove `InsecureSkipVerify` from `TLSConfig` struct; restrict to `CARGOSHIP_TLS_INSECURE` env var (Issue #195)
- Add `--` separator in Magika CLI invocation to prevent flag injection (Issue #196)
- Mark `SMTPPassword`, `SlackWebhookURL`, `WebhookURL` with `json:"-"` to prevent credential leakage (Issue #197)
- Validate WebSocket `Origin` header against server `Host` (Issue #198)
- Validate symlink targets in tar extraction stay within output directory (Issue #198)
- Profile output directories use mode `0700` (Issue #199)
- Add `Timeout` to `http.Client` in geo locator (Issue #199)
- Filter job environment variables through security denylist in launch server (Issue #199)

## [0.9.1] - 2026-01-02

### Fixed
- Cost package test flakiness and inconsistent period filtering

## [0.9.0] - 2026-01-02

### Added
- **File Deduplication** — Cross-upload deduplication via content hashing, complete implementation (Issue #108)
  - Pipeline integration, dedup index, manifest export, CLI `--enable-dedup` flag
- **Shard Balance Analysis** — `cargoship balance` command, complete implementation (Issue #109)
  - Analysis, planning, chunk download/extraction, and full execution pipeline
- **Tier-Aware Chunking** — Groups files by storage tier for optimal cost savings (Issue #164)
  - `--tier-strategy tier-aware` flag; cost warning prompts with `--yes` bypass
- **Tier Cost Limits** — `--tier-max` flag prevents automatic selection of restrictive tiers (Issue #168)
- **Archive Tier TCO Modeling** — Total cost of ownership analysis for Glacier/Deep Archive (Issue #169)
- **Cost Benchmarking & Transparency** — ASCII cost comparison charts, chunking cost breakdown (Issue #165)
- **Direct Upload Mode** — Fast path bypassing archiving/compression for small datasets; 3.7× improvement (Issue #166)
  - `--direct-upload`, `--force-direct-upload`, `--direct-upload-threshold-mb` flags
- **S3 Cost Analyzer** — `cargoship analyze` command for existing bucket cost analysis (Issue #170)
  - S3-compatible storage provider support
- **Interactive TUI** — Pause/resume controls and live worker counts (Issue #112)

### Fixed
- Race conditions across multiregion, pipeline, staging, and adaptive controller packages
- Deadlock in `RealTimeLoadBalancer` and congestion control
- CloudWatch publisher timer race condition (Issue #15)
- AWS Open Data bucket configurations for benchmark suite (Issue #166)
- GitHub Actions: updated to Go 1.24, artifact actions v4

## [0.7.3] - 2025-12-17

### Added
- **AWS KMS Encryption** — SSE-KMS for data chunks + envelope encryption for manifests (Issue #163)
  - `--kms-key-id` and `--encrypt-manifest` flags; decrypt support in download/list/info
- **Magika AI File Type Detection** — Optional AI-powered compression type selection (Issue #30)
  - Lazy detection with extension pre-filter; ~1000 files/sec throughput
- **Distributed Tracing** — OpenTelemetry tracing across all pipeline stages (Issue #155)
  - stdout, Jaeger, and OTLP exporters; `--tracing`, `--tracing-exporter`, `--tracing-endpoint` flags
- **Prometheus Metrics** — `--prometheus-addr` flag; per-upload counters and throughput gauges (Issue #155)
- **Adaptive Shard Count** — Auto-tunes 4–32 shards from file count, data size, CPU/memory (Issue #106)
- **Resume Interrupted Uploads** — Local state persistence + auto-detection + `cargoship resume` command (Issue #119)
- **Automatic Storage Tier Selection** — `--auto-tier` selects STANDARD/GLACIER/DEEP_ARCHIVE by file age (Issue #32)
- **Zero-Copy I/O** — Linux splice: BufferedPipe pooling, upload buffer pooling (Issue #153)
- **HTTP/2 and TCP Network Tuning** — 3× throughput improvement on high-latency links (Issue #154)
- **Content-Aware Compression** — Multi-level encoder pools; code/text/binary/media profiles (Issue #105)
- **Adaptive Worker Scaling** — Worker count scales with workload size (Issue #58)
- **Performance Optimizations** — mmap LRU cache, lock-free manifest updates, parallel scanner batching, HTTP connection pooling (Issue #34)
- **CloudWatch Metrics** — CargoHold pipeline operation metrics (Issue #111)
- **`cargoship migrate`** — Archive conversion command (Issue #100)
- **CargoHold config file** — Config file support for CargoHold options (Issue #101)
- **GoReleaser** — Automated multi-platform release builds (Issue #160)
- **Incremental Sync** — Only upload changed files using content hashing (Issue #148)
- **Manifest Enhancements** — Fast in-memory indexing, validation, compression (Issues #88, #91, #92)
- **CargoHold** — Archive format with selective extraction, manifest query API (Issues #89, #90, #93)
- **`upload` command** — Primary upload command with CargoHold sharding support (Issue #95)
- **`download` command** — S3 URL support and auto-compression detection (Issue #96)
- **`verify` command** — Dataset integrity verification (Issue #99)

### Changed
- Staging package refactored: removed Simple* stubs, merged compression types (Issue #16)
- Default S3 upload workers increased from 4 to 8 (Issue #64)
- TLS certificate loading for controller (Issue #141)
- Session key generation for load balancer affinity (Issue #139)

### Fixed
- Critical context propagation bug in pipeline stages (Issue #155)
- BBR packet tracking timing tolerance (Issue #152)
- Monitoring interval configurability for flaky test (Issue #151)
- Goroutine leaks in staging package via `Shutdown()` methods (Issue #142)
- AWS SDK HTTP connection leaks (Issue #65)
- PrefixRouter deadlock on context cancellation
- Platform build issues: mmap, NUMA, SYS_GETCPU cross-compilation
- CloudWatch publisher race condition (Issue #15)

### Issues Closed
- #15, #16, #30, #32, #34, #58, #64, #65, #88–#96, #98–#101, #104–#106, #108–#109
- #111, #114, #119, #138–#142, #148, #151–#155, #157–#160, #163

## [0.6.2] - 2025-12-15

### Added
- Advanced S3 transporters integrated into Pipeline CLI: staging, adaptive, optimized, and basic modes
  - `--transporter` flag: `basic`, `staging`, `adaptive`, `optimized`, `none`
  - `--optimization`, `--congestion-control`, `--disable-staging` flags

### Fixed
- OptimizedTransporter Content-Length bug — switched to S3 manager uploader (Issue #162)

## [0.6.1] - 2025-12-14

### Added
- **Performance Profiling Infrastructure** — Phases 1–4: continuous profiling, regression detection, CI/CD integration (Issue #33)
- **S3 URL support for `info` command** (Issue #98)
- **Upload Failure Cleanup** — Automatic cleanup of partial S3 multipart uploads (Issue #158)
- **Resume Failed Uploads** — Initial resume infrastructure (Issue #157)
- **Manifest Enhancements** — Thread-safe builder, validation, compression, query API (Issues #88–#93)
- **`upload` command** — CargoHold sharding, `--shard-count`, `--shard-strategy` (Issue #95)
- **`download` command** — S3 URL + auto-compression detection (Issue #96)
- **`verify` command** — Dataset integrity verification (Issue #99)
- **`migrate` command** — Archive conversion (Issue #100)
- **CargoHold config** — Config file support (Issue #101)
- **CloudWatch metrics** — Pipeline operation metrics (Issue #111)
- Budget alert notification system (Issue #133)
- HTTP/2 and TCP network tuning (Issue #154)
- Zero-copy I/O optimizations (Issue #153)

### Fixed
- Goroutine leaks in staging package via `Shutdown()` methods (Issue #142)
- AWS SDK HTTP connection leaks (Issue #65)
- BBR packet tracking timing tolerance (Issue #152)
- Monitoring interval configurability (Issue #151)
- Platform build issues: mmap, NUMA cross-compilation

## [0.6.0] - 2025-12-09

### Added
- **Budget & Cost Management System** - Comprehensive enterprise cost tracking and forecasting
  - Dual budget controls: Cost budgets (USD) AND volume quotas (GB) enforced independently
  - Grant period management: 1-3 year budget periods with rollover support
  - Threshold alerts: Warning at 80%, critical at 100%
  - Budget enforcement: Operations blocked if limits would be exceeded
- **Project-Based Cost Tracking** - Each manifest upload ID becomes a project for granular analysis
  - Time period filtering (day/week/month/year/custom date ranges)
  - Multi-dimensional breakdowns (region, storage class, project)
  - Cost summaries with total costs, savings, file counts, data volumes
- **ML-Powered Forecasting** - Budget forecasting and burn rate analysis
  - 4 forecasting models: linear, exponential, moving_average, ensemble
  - Confidence intervals: 90%, 95%, 99% prediction bounds
  - Burn rate analysis: Historical trends, acceleration, volatility tracking
  - Budget exhaustion predictions with exact dates and probability estimates
- **Multi-Channel Alert Notifications** - Production-ready alert system
  - Email (SMTP): TLS 1.2+ encrypted, multiple recipients, Gmail/Office 365/AWS SES support
  - Slack webhooks: Rich message formatting with color-coded attachments
  - Custom webhooks: JSON payload with complete alert metadata
  - CloudWatch integration: Native AWS metrics and alarms
  - 6 alert types: cost_threshold, volume_threshold, cost_over_budget, volume_over_quota, budget_projection, volume_projection
  - 3 severity levels: info, warning, critical
- **Comprehensive CLI Commands** - 20+ subcommands across 3 command groups
  - Budget management: `budget status`, `budget set`, `budget list`, `budget remove`
  - Cost tracking: `cost summary`, `cost projects`, `cost project`, `cost forecast`, `cost burnrate`, `cost exhaustion`
  - Alert configuration: `alerts configure`, `alerts test`, `alerts enable/disable`
- **Comprehensive Documentation** - 2,970+ lines of user and developer docs
  - User guide: `docs/BUDGET_USER_GUIDE.md` (870 lines)
  - API reference: `docs/BUDGET_API.md` (1,100+ lines)
  - Alert setup: `docs/ALERTS_CONFIGURATION.md` (1,000+ lines)

### Changed
- Enhanced cost estimation system for S3 operations with regional pricing support

### Fixed
- Timezone issue in week period calculation (Issue #150)

### Removed
- **Rclone Integration** - CargoShip transitioned to S3-native architecture
  - `--cloud-destination` CLI flag removed (use direct S3 commands instead)
  - `cargoship rclone` command removed (use [rclone](https://rclone.org/) directly for non-S3 providers)
  - Cloud transporter plugin removed (S3-only focus)
  - Rclone configuration sections removed from config files
  - **Migration**: Use `cargoship create upload` with `--bucket` and `--prefix` flags for S3 uploads
  - **Note**: Non-S3 cloud providers are no longer supported; users needing GCS, Azure Blob, etc. should use rclone as a standalone tool

### Technical Details
- **Production Code**: ~140KB across 6 core files
- **Test Code**: ~102KB with 67 tests passing
- **Test Coverage**: pkg/aws/cost 72.5%
- **Quality**: Zero linting issues, zero security vulnerabilities

### Issues Closed
- #3: Define Grant and Project types
- #4: Implement cost estimator for S3 operations
- #5: Implement budget CLI commands
- #6: Implement budget alert system
- #39: Remove rclone integration code (Phase 2)
- #40: Update dependencies (Phase 3)
- #41: Remove configuration and documentation (Phase 4)
- #136: Implement alert notification system (duplicate of #6)
- #147: Budget & Cost Management System (Phase 1-6)
- #148: Incremental sync with manifest-based delta detection
- #149: Project-based cost tracking
- #150: Timezone issue in week period calculation

## [0.5.1] - 2025-11-08

### Added
- **Integration Testing Framework** - Comprehensive testing with real AWS S3 validation
  - 19 new integration tests validating end-to-end workflows
  - Real AWS S3 validation (not just LocalStack simulation)
  - Automatic bucket lifecycle management with proper cleanup
- **Performance Benchmark Suite** - 5 comprehensive benchmarks
  - Compression speed testing (gzip, zstd, bzip2 throughput)
  - S3 throughput validation (upload/download with 10MB-100MB files)
  - Memory efficiency testing (100MB-1GB files)
  - Deduplication overhead analysis
  - End-to-end workflow benchmarking (50 files)
- **Failure Scenario Tests** - 7 production reliability tests
  - S3 bucket not found error handling
  - Corrupted archive detection
  - Invalid permissions handling
  - Network timeout and retry logic
  - Partial upload cleanup validation
  - Concurrent upload race condition testing (10 concurrent)
  - Disk space monitoring and handling
- **Large-Scale Scenario Tests** - 5 comprehensive edge case tests
  - Large directory tree: 10,000 files in 11.38s (11,383 files/sec)
  - Deep nesting: 25 directory levels, 330-char paths
  - Long paths: 484-494 character path validation
  - Special characters: Unicode, emoji, punctuation (7 files)
  - Mixed file sizes: 184 files (1KB-50MB), 855 MB/s compression

### Performance Metrics
- **Compression**: zstd 527.30 MB/s (10.7x faster than gzip 49.14 MB/s)
- **S3 Upload**: 10MB → 32.20 MB/s, 100MB → 23.07 MB/s
- **S3 Download**: 10MB → 64.15 MB/s, 100MB → 89.68 MB/s
- **Memory Efficiency**: 4.21 MB peak for 100MB file (4.21% ratio)
- **Large-Scale**: 10,000 files in 9.26s (133x faster than 20min target)

### Impact Summary
- 19 new integration tests with real AWS S3
- All 7 failure scenarios validated for production readiness
- Comprehensive benchmarks proving 10x+ improvements
- Successfully handles 10,000 files, 25-level nesting, 494-char paths
- Zero linting issues, all tests passing with real AWS validation

## [0.4.1] - 2025-07-27

### Changed
- **Documentation Accuracy**: Corrected ML claims to accurately reflect proven algorithm implementations
- **Performance Transparency**: Updated benchmarks to emphasize BBR congestion control and CUBIC algorithms
- **Test Coverage**: Standardized 95% test coverage reporting across all files
- **Enterprise Messaging**: Aligned all documentation for consistent professional positioning
- **GitHub Pages**: Enhanced site highlighting production-proven network algorithms

### Added
- **Algorithm Transparency**: Clear descriptions of BBR (Google), CUBIC (Linux kernel), and signal processing methods
- **Technical Honesty**: Accurate representation of deterministic algorithms vs future ML capabilities
- **Realistic Roadmap**: Updated roadmap with honest ML implementation timeline (v0.6.0 - September 2026)

### Fixed
- **ML Overclaims**: Removed misleading references to "AI-driven" optimization where deterministic algorithms are used
- **Documentation Inconsistency**: Unified messaging across README, docs, and GitHub Pages
- **Link References**: Corrected documentation cross-references and outdated URLs

### Transparency Note
This release prioritizes honest representation of CargoShip's capabilities. The 4.6x performance improvements are achieved through Google's production-tested BBR algorithm and Linux kernel's CUBIC implementation, not machine learning. Future ML capabilities are planned for v0.6.0 with proper data collection and model training infrastructure.

## [0.4.0] - 2025-07-27

### Added
- **BBR Congestion Control**: Complete implementation of Google's BBR algorithm with bandwidth probing and state machine management
- **CUBIC TCP Algorithm**: Advanced congestion control with cubic function-based window growth and Hystart support
- **RTT Estimation System**: Sophisticated round-trip time analysis with multiple algorithms (Exponential, Kalman, Jacobson-Karels, Adaptive, Ensemble)
- **Loss Detection & Recovery**: Multi-method packet loss detection (timeout, duplicate ACK, SACK, ECN) with comprehensive recovery strategies
- **Bandwidth-Delay Product**: Dynamic BDP calculation with optimization algorithms and adaptive buffer sizing
- **Advanced Network Adaptation**: Real-time parameter optimization with ML integration and predictive algorithms
- **Comprehensive Test Suite**: 95+ test functions across all flow control components with 100% pass rate

### Changed
- **Upload Performance**: Improved from 3x to 4.6x faster uploads with advanced network optimization
- **Memory Efficiency**: Optimized data structures with bounded collections and automatic cleanup
- **Network Intelligence**: Enhanced network condition monitoring and adaptive parameter adjustment
- **Enterprise Features**: Strengthened enterprise-grade observability and monitoring capabilities

### Technical Details
- **Lines of Code**: 8,386+ lines of production-ready network optimization algorithms
- **Components**: 5 major algorithmic components (BBR, CUBIC, RTT, Loss Detection, BDP)
- **Files Created**: 10 new files (5 implementation + 5 comprehensive test files)
- **Static Analysis**: Zero violations with clean compilation across all components
- **Thread Safety**: Full concurrent access patterns with proper locking mechanisms

### Performance Improvements
- **BBR Algorithm**: Optimal bandwidth utilization with sophisticated probing
- **CUBIC Control**: Enhanced congestion window management with TCP-friendly fallback
- **RTT Analysis**: Multi-algorithm estimation with confidence scoring and accuracy tracking
- **Loss Recovery**: Fast, timeout, and congestion-based recovery with adaptive thresholds
- **BDP Optimization**: Dynamic buffer sizing with network condition awareness

## [0.3.2] - 2025-07-13

### Added
- **Multi-Region Stability**: Complete region selection strategy testing with advanced failover scenarios
- **Performance Benchmarking**: Comprehensive throughput, latency, and scalability testing framework
- **Real-World Simulation**: Network partition, data center outage, and load spike testing

### Changed
- **Region Selection**: Enhanced algorithms for round-robin, weighted, latency-based, geographic, and priority-based selection
- **Failover Logic**: Improved cross-region retry scenarios and timeout handling
- **Test Coverage**: Expanded multiregion package testing with realistic failure patterns

## [0.3.1] - 2025-06-28

### Added
- **JWT Authentication**: Complete JWT-based authentication with RSA and HMAC signing support
- **Role-Based Access Control**: Agent, admin, and readonly roles with comprehensive permission management
- **TUI/GUI Interface**: Full Terminal and Graphical User Interface supporting all CargoShip operations
- **Security Framework**: Integrated gosec vulnerability scanning and security best practices
- **LocalStack Integration**: Complete AWS testing framework with LocalStack S3 simulation

### Fixed
- **Resource Management**: Resolved goroutine leaks and improved resource cleanup
- **Memory Usage**: Optimized memory allocation patterns and reduced resource consumption

### Security
- **Vulnerability Scanning**: Integrated gosec for continuous security analysis
- **Access Control**: Implemented comprehensive role-based permission system
- **Secure Authentication**: JWT tokens with configurable signing algorithms

## [0.3.0] - 2024-07-11

### Added
- Multi-region pipeline distribution with intelligent failover
- Advanced failover optimization with circuit breakers and predictive monitoring
- Comprehensive performance benchmarking command
- Production-grade security scanning pipeline
- Complete code signing infrastructure with GPG key management
- Extensive user documentation and security guides

### Changed
- Improved multiregion package coverage from 80% to 85.9%
- Enhanced GPG package test coverage to 88.8%
- Optimized rclone package performance and reliability

### Fixed
- Multiregion coordinator initialization and background services
- Test failures in coordinator validation and health checks
- Memory leaks in connection pooling and health monitoring

## [0.2.0] - 2024-07-10

### Added
- Predictive chunk staging with content analysis
- Network adaptation for optimal transfer performance
- Enhanced staging system with memory-efficient buffering
- Comprehensive test suite with 85%+ coverage
- Advanced compression algorithms (zstd, lz4)
- Multi-threaded upload optimization

### Changed
- Improved staging package coverage from 71.1% to 81.8%
- Enhanced compression package reliability and performance
- Optimized memory usage in chunk staging operations

### Fixed
- Buffer overflow issues in staging operations
- Race conditions in concurrent upload scenarios
- Memory leaks in compression and staging pipelines

## [0.1.0] - 2024-07-09

### Added
- Core AWS S3 integration with native SDK support
- Intelligent cost optimization and storage class selection
- Basic multi-region support and coordination
- Comprehensive AWS configuration and credential management
- Cost estimation and budget tracking
- CloudWatch metrics integration
- Basic CLI interface with survey, estimate, and ship commands

### Changed
- Migrated from rclone to native AWS SDK for improved performance
- Implemented intelligent storage class selection algorithms
- Added comprehensive error handling and retry logic

### Fixed
- S3 multipart upload reliability issues
- AWS credential handling and region selection
- Cost calculation accuracy for different storage classes

---

## Version Schema

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** version when you make incompatible API changes
- **MINOR** version when you add functionality in a backwards compatible manner  
- **PATCH** version when you make backwards compatible bug fixes

### Pre-1.0.0 Development

During pre-1.0.0 development:
- **0.MINOR.PATCH** where MINOR may include breaking changes
- **0.x.0** releases may contain significant new features
- **0.x.y** releases contain bug fixes and small improvements

### Release Types

- **Alpha** (`0.x.0-alpha.1`): Early development, unstable API
- **Beta** (`0.x.0-beta.1`): Feature complete, API stabilizing
- **Release Candidate** (`0.x.0-rc.1`): Production ready candidate
- **Stable** (`0.x.0`): Production ready release