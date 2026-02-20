# Changelog

All notable changes to CargoShip will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Enhanced Data Retrieval (v0.11.0 milestone, Issues #200–#201)
- S3 Glacier/Deep Archive pre-flight check and restore orchestration (Issue #200)
  - `GlacierRestorer` with `CheckAndRestore` — HeadObject checks, RestoreObject requests, Expedited/Standard/Bulk tiers
  - `WaitForRestore` — polls until all chunks accessible, with progress callback
  - `EstimateRetrievalCost` — approximate USD fees by storage class and tier
  - New `ChunkKeysForPaths`, `ChunkKeysForDVCStage`, `ChunkKeysForCommit`, `AllChunkKeys` helpers on `SelectiveExtractor`
- Quota-aware restore with `--max-restore-cost` flag (Issue #201)
- `cargoship restore` new flags: `--tier`, `--wait`, `--dry-run`, `--max-restore-cost`, `--restore-days`
- `cargoship browse` new flags: `--tier`, `--wait`, `--max-restore-cost`, `--restore-days`
- Roadmap version numbering corrected: v0.9.0 Enhanced Data Retrieval → v0.11.0

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

## [0.7.0] - 2026-01-15

### Added
- **Adaptive Shard Count** — Automatic S3 prefix optimization based on workload (Issue #106)
  - Auto-tunes 4–32 shards from file count, data size, and available CPU/memory
  - Manual override via `--shard-count` flag
  - Minimum 6 chunks per shard for load balancing
- **Zero-Copy I/O** — Linux splice-based data movement for near-zero CPU overhead (Issue #153)
  - Phase 1: zero-copy read paths
  - Phase 2: BufferedPipe chunk pooling
  - Phase 3: upload buffer pooling
  - Phase 4: splice integration for Linux
- **HTTP/2 and TCP Network Tuning** — 3x throughput improvement on high-latency links (Issue #154)
- **Geographic Region Selection** — Automatic S3 region selection based on client location (Issue #138)
- **Content-Aware Compression** — Magika AI file type detection for optimal compression levels (Issue #105)
  - Code files: level 9; Documents: level 6; Images/video/archives: none
- **File Deduplication** — Cross-upload deduplication via content hashing (Issue #108)
  - Scanner integration, dedup index, manifest export, CLI flag
- **Shard Balance Analysis** — `cargoship balance` command for shard rebalancing (Issue #109)
  - Analysis, planning, chunk download/extraction, and execution pipeline
- **Resume Failed Uploads** — Automatic recovery from interrupted uploads (Issue #157)
- **Upload Failure Cleanup** — Automatic cleanup of partial S3 multipart uploads (Issue #158)
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
- Goroutine leaks in staging package via `Shutdown()` methods (Issue #142)
- AWS SDK HTTP connection leaks (Issue #65)
- Cost package test flakiness and period filtering
- PrefixRouter deadlock on context cancellation
- BBR packet tracking timing tolerance (Issue #152)
- Monitoring interval configurability for flaky test (Issue #151)

### Issues Closed
- #16, #64, #65, #88, #89, #90, #91, #92, #93, #95, #96, #98, #99, #101, #104
- #105, #106, #108, #109, #111, #114, #138, #139, #141, #142, #148, #151–#155, #157–#159

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