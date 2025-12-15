<div align="center">
  <img src="assets/images/logo.png" alt="CargoShip Logo" width="200" height="200">
  <h1>CargoShip</h1>
  <p><strong>Enterprise data archiving for AWS, built for speed and intelligence</strong></p>
</div>

[![Go Version](https://img.shields.io/github/go-mod/go-version/scttfrdmn/cargoship)](https://github.com/scttfrdmn/cargoship/blob/main/go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/scttfrdmn/cargoship.svg)](https://pkg.go.dev/github.com/scttfrdmn/cargoship)
[![License](https://img.shields.io/github/license/scttfrdmn/cargoship)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/scttfrdmn/cargoship?include_prereleases)](https://github.com/scttfrdmn/cargoship/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/scttfrdmn/cargoship)](https://goreportcard.com/report/github.com/scttfrdmn/cargoship)
[![codecov](https://codecov.io/gh/scttfrdmn/cargoship/branch/main/graph/badge.svg)](https://codecov.io/gh/scttfrdmn/cargoship)
[![Security](https://img.shields.io/badge/security-gosec-brightgreen)](https://github.com/securecodewarrior/gosec)
[![Build Status](https://github.com/scttfrdmn/cargoship/actions/workflows/test.yml/badge.svg)](https://github.com/scttfrdmn/cargoship/actions)
[![GitHub Issues](https://img.shields.io/github/issues/scttfrdmn/cargoship)](https://github.com/scttfrdmn/cargoship/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/scttfrdmn/cargoship)](https://github.com/scttfrdmn/cargoship/pulls)

CargoShip is a next-generation data archiving tool optimized for AWS infrastructure, featuring streaming pipeline architecture, multi-prefix parallel uploads (8× throughput), and intelligent cost optimization. Originally inspired by Duke University's SuitcaseCTL research, CargoShip has evolved into a modern, production-ready solution for enterprise-scale data archiving.

## 🚀 Enterprise Features with Modern Architecture

**High-performance streaming data uploads (v0.5.1+):**
- 🚀 **Streaming Pipeline** - Zero local disk usage, stream directly to S3
- ⚡ **Multi-Prefix Parallel Uploads** - 8x throughput improvement with S3 prefix sharding
- 📊 **Real-Time Progress Tracking** - Beautiful TUI with live upload metrics
- 🧠 **Intelligent Chunking** - Adaptive chunk sizing with compression-aware optimization
- 💰 **Cost Optimization** - Intelligent storage class selection and lifecycle policies
- 🎯 **Advanced S3 Features** - Multi-region support, predictive prefetching
- 📈 **Performance Monitoring** - Comprehensive metrics and analytics
- 💵 **Budget Tracking** - Project-based cost and volume quota management with burn rate forecasting
- 🛡️ **Security first** - KMS encryption and compliance-ready audit trails

## 🚀 Quick Start

### Installation

```bash
# Homebrew (macOS/Linux)
brew install scttfrdmn/tap/cargoship

# Scoop (Windows)
scoop bucket add scttfrdmn https://github.com/scttfrdmn/scoop-bucket
scoop install cargoship

# Go Install
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

See the [Installation Guide](docs/install.md) for more options.

### Basic Workflow

```bash
# 1. Estimate costs before uploading
cargoship estimate /data/project-2024 --storage-class deep-archive

# 2. Upload with streaming pipeline (NEW in v0.5.1+)
cargoship create upload /data/completed-analysis \
  --bucket my-bucket \
  --prefix project-2024 \
  --storage-class INTELLIGENT_TIERING \
  --shards 8 \
  --workers 4

# Real-time progress display:
# 🚢 Uploading: 1234 files | 5.67 GB | 89 chunks | 123.4 MB/s | 1m30s elapsed

# 3. List uploaded files using manifest (no download required)
cargoship list --bucket my-bucket --upload-id 20251206-123456-abcd1234

# 4. Manage S3 lifecycle policies
cargoship lifecycle --bucket my-bucket --template archive-optimization
```

## 💰 Intelligent Cost Optimization

CargoShip provides enterprise-grade cost optimization with proven algorithms:

```bash
$ cargoship estimate ./genomics-analysis --show-breakdown

📊 Archive Cost Estimate (1.2TB genomics data)
┌─────────────────┬──────────────┬──────────────┐
│ Storage Class   │ Monthly Cost │ Annual Cost  │
├─────────────────┼──────────────┼──────────────┤
│ Standard        │ $276.48     │ $3,317.76   │
│ Glacier         │ $61.44      │ $737.28     │
│ Deep Archive    │ $12.29      │ $147.48     │
└─────────────────┴──────────────┴──────────────┘

💡 Optimization Recommendations (v0.5.1+):
• Archive raw data → Deep Archive (90% savings)
• Analysis results → Glacier (75% savings)
• Enable lifecycle policies → Additional 15% savings
• Multi-prefix parallel uploads → 8x throughput improvement
• Streaming pipeline → Zero local disk usage
• Intelligent chunking → Optimal compression ratios

Total annual savings: $3,170/year with 8x upload performance

✅ Available Now:
• `cargoship create upload` - High-performance streaming uploads
• `cargoship lifecycle` - Automated lifecycle policy management
• `cargoship budget` - Project-based cost and volume quota tracking
• Real-time progress tracking with TUI
```

### Budget Tracking and Cost Control

CargoShip includes enterprise-grade budget management to prevent cost overruns:

```bash
# Set project budget with cost and volume limits
$ cargoship budget set genomics-2025 --cost 5000 --volume 2000

# Monitor budget status with burn rate forecasting
$ cargoship budget status genomics-2025

📊 Budget Status: genomics-2025
Cost Budget:
├─ Maximum:     $5,000.00
├─ Current:     $1,234.56 (24.7%)
├─ Remaining:   $3,765.44
├─ Daily Burn:  $41.23
└─ Projected:   $4,123.00 (within budget ✅)

Volume Quota:
├─ Maximum:     2,000 GB
├─ Current:     487 GB (24.4%)
├─ Remaining:   1,513 GB
└─ Projected:   1,948 GB (within quota ✅)
```

**Features**:
- **Dual Controls**: Cost budgets (USD) and volume quotas (GB)
- **Project-Based**: Track spending per project, grant, or department
- **Burn Rate Forecasting**: ML-powered end-of-period projections
- **Automatic Blocking**: Prevents uploads that exceed budgets
- **Multi-Channel Alerts**: Email, Slack, CloudWatch notifications

See the [Budget Management Guide](docs/BUDGET.md) for complete documentation.

```

## 🏗️ Modern Streaming Architecture

### High-Performance Pipeline (v0.5.1+)

CargoShip uses a modern streaming pipeline architecture for maximum performance:

**Pipeline Stages**:
- **Scanner**: Multi-threaded file discovery with parallel directory traversal
- **Chunker**: Intelligent chunking with compression-aware boundary detection
- **Archiver**: Streaming tar+zstd compression with zero disk writes
- **Uploader**: Multi-prefix parallel S3 uploads (8x capacity improvement)

**Performance Features**:
- **Zero Local Disk**: Streams directly from filesystem → compression → S3
- **Bounded Memory**: O(chunk_size × workers) prevents OOM conditions
- **Adaptive Chunking**: Smart file grouping based on size and compressibility
- **Multi-Prefix Sharding**: Parallel uploads across 8 S3 prefixes
- **Real-Time Progress**: Live metrics with throughput and ETA tracking

### Architecture Diagram

```
┌─────────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Source Data       │    │  CargoShip       │    │   AWS S3        │
│                     │    │  Pipeline        │    │                 │
│ • Local Files       │───▶│                  │───▶│ • Standard      │
│ • Network Shares    │    │ • Scanner        │    │ • IA            │
│ • Data Lakes        │    │ • Chunker        │    │ • Glacier       │
│ • Archive Systems   │    │ • Archiver       │    │ • Deep Archive  │
└─────────────────────┘    │ • Uploader (8x)  │    └─────────────────┘
                           └──────────────────┘
                                    │
                                    ▼
                           Real-Time Progress TUI
                           Files | GB | MB/s | ETA
```

### Performance Tuning

Optimize CargoShip for your workload:

```bash
# For many small files (<1MB):
cargoship create upload /data --bucket my-bucket \
  --chunk-size-mb 50 --workers 8

# For large files (>100MB):
cargoship create upload /data --bucket my-bucket \
  --chunk-size-mb 500 --workers 4

# Maximum throughput:
cargoship create upload /data --bucket my-bucket \
  --shards 16 --workers 8 --storage-class STANDARD
```

### Real-Time Progress Tracking

CargoShip provides beautiful real-time progress display during uploads (v0.5.1+):

```bash
$ cargoship create upload /data/genomics --bucket research-data --prefix 2024-study

🚢 Uploading: 1,234 files | 5.67 GB | 89 chunks | 123.4 MB/s | 1m30s elapsed

✅ Upload Complete!
   Files:       1,234 files
   Data Size:   5.67 GB (uncompressed)
   Chunks:      89 archives
   Shards:      8 S3 prefixes
   Throughput:  123.4 MB/s
   Duration:    1m32s
   Upload ID:   20251206-123456-abcd1234
```

**Features**:
- **Terminal Detection**: Automatically disables for non-TTY contexts (pipes, CI/CD)
- **Live Metrics**: Files, data size, chunks, throughput, elapsed time
- **Clean Display**: Single-line progress with ANSI escape codes
- **Zero Configuration**: Works out of the box, no flags required

### Performance Benchmarks

Real AWS S3 performance results (v0.5.1):

| Workload | Files | Size | Duration | Throughput | Memory |
|----------|-------|------|----------|------------|--------|
| **Small files** | 10,000 | 176 MB | 437ms | 403 MB/s | 3.4 GB |
| **Large files** | 100 | 56 GB | 311s | 185 MB/s | 4.9 GB |

**Key Performance Metrics**:
- **Compression**: zstd @ 527 MB/s (10.7× faster than gzip)
- **Memory Efficiency**: 6-8% of data size (excellent scaling)
- **Multi-Prefix**: 8× S3 request rate capacity
- **Streaming**: Zero local disk usage

**Benchmark Details**:
- Test environment: Real AWS S3 (not LocalStack simulation)
- Storage class: STANDARD
- Shards: 8 (default)
- Workers: 8 (Phase 4 optimization)
- Compression: zstd level 3

## 🎯 Use Cases

CargoShip excels at large-scale data archiving:

- **Research Data**: Genomics, imaging, sensor data archiving
- **Analytics Output**: ML training data, analysis results
- **Enterprise Backup**: Long-term retention with cost optimization
- **Compliance**: Audit trails and secure archival storage
- **Data Lakes**: Cost-effective cold storage tier migration

## 📖 Documentation

### Getting Started
- **[Installation Guide](docs/install.md)** - Get CargoShip running in your environment
- **[S3 Direct Upload Guide](docs/S3_DIRECT_UPLOAD.md)** - Complete guide to CargoShip's streaming pipeline
- **[CLI Reference](docs/CLI_REFERENCE.md)** - Comprehensive command-line reference
- **[Quick Start Wizard](docs/wizard.md)** - Interactive setup guide
- **[Complete Documentation](https://cargoship.app)** - Full documentation site

### Performance & Optimization
- **[Performance Benchmarks](docs/PERFORMANCE_BENCHMARKS.md)** - **7x faster than s5cmd**, 40x faster than rclone
- **[Optimization Guide](docs/OPTIMIZATION_GUIDE.md)** - Tuning for maximum performance
- **[Advanced Flow Control](docs/TASK_4_ADVANCED_FLOW_CONTROL.md)** - Network optimization algorithms
- **[Troubleshooting](docs/TROUBLESHOOTING.md)** - Common issues and solutions

### Migration & Integration
- **[Migration from rclone](docs/MIGRATION_FROM_RCLONE.md)** - Switch from rclone to CargoShip
- **[AWS Integration Guide](docs/AWS_INTEGRATION_REPORT.md)** - Complete AWS setup and configuration
- **[User Guide](docs/USER_GUIDE.md)** - Complete feature walkthrough

### Architecture & Features
- **[Storage Format Documentation](docs/STORAGE_FORMAT.md)** - Open S3 storage format for data portability
- **[Session Affinity](docs/SESSION_AFFINITY.md)** - Multi-region load balancing
- **[Manifest System](pkg/manifest/README.md)** - File indexing and fast query API
- **[Cost Management](docs/cost-management.md)** - Intelligent cost optimization features
- **[Budget Management](docs/BUDGET.md)** - Project-based cost and volume quota tracking

### Deployment & Operations
- **[Deployment Guide](docs/DEPLOYMENT_GUIDE.md)** - Production deployment strategies
- **[Launch Agent Setup](docs/launch-agent.md)** - Enterprise agent deployment
- **[Ghost Ship Deployment](docs/deployment/GHOST_SHIP_DEPLOYMENT_GUIDE.md)** - Distributed agent setup

## 🤝 Contributing

CargoShip welcomes contributions from developers and researchers! Originally inspired by Duke University's SuitcaseCTL research, now a fully independent project.

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Commit your changes: `git commit -m 'Add amazing feature'`
4. Push to the branch: `git push origin feature/amazing-feature`
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## 📄 License and Attribution

CargoShip is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

**Research Origins**: Originally inspired by Duke University's [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl) research project. CargoShip has evolved into an independent, production-ready solution with a modern streaming architecture and enterprise features.

## 🆘 Support

- **Documentation**: [cargoship.app](https://cargoship.app)
- **Issues**: [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)
- **Community**: [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)

---

**Ship your data with confidence. Ship it with CargoShip.** 🚢