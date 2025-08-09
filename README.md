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

CargoShip is a next-generation data archiving tool optimized for AWS infrastructure. Built on the foundation of Duke University's [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl), CargoShip adds native AWS integration, intelligent cost optimization, and enterprise-grade observability with advanced network optimization algorithms.

## 🚀 Enterprise Features with Research Flexibility

**Advanced data archiving for any environment:**
- 📊 **Intelligent cost optimization** - Save up to 90% with proven algorithms
- ⚡ **Advanced network algorithms** - BBR and CUBIC congestion control for maximum throughput  
- 🧠 **Smart compression** - ZSTD with adaptive chunking and staging
- 💰 **Advanced budget controls** - Cost AND volume limits with grant period management
- 🔍 **Enterprise observability** - Comprehensive metrics and monitoring
- 🛡️ **Security first** - KMS encryption and compliance-ready audit trails

## 🚀 Quick Start

### Installation

```bash
# Install CargoShip
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest

# Or download pre-built binary
curl -sSL https://get.cargoship.dev/install.sh | sh
```

### Basic Workflow

```bash
# 1. Survey your data and estimate costs with advanced algorithms
cargoship survey /data/project-2024
cargoship estimate /data/completed-analysis --storage-class deep-archive

# 2. Archive with advanced network optimization and budget controls
cargoship ship /data/completed-analysis \
  --destination s3://my-bucket/project-2024 \
  --storage-class intelligent-tiering \
  --enable-bbr-congestion-control \
  --max-cost-per-month 200 \
  --max-volume 500GB

# 3. Deploy with advanced monitoring
docker run -d --name cargoship-agent \
  -v /mnt/data:/data:ro \
  -v ~/.aws:/root/.aws:ro \
  scttfrdmn/cargoship:latest \
  --watch /data --enable-advanced-flow-control
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

💡 Optimization Recommendations:
• Archive raw data → Deep Archive (90% savings)
• Analysis results → Glacier with BBR congestion control (75% savings)  
• Enable lifecycle policies → Additional 15% savings
• Advanced flow control algorithms → 4.6x faster uploads

Total annual savings: $3,170/year with 360% performance gain

💡 Coming in v0.5.0 (Dec 2025):
• Volume-based budget controls (--max-volume 100GB)
• Grant period management (1-3 year budgets with rollover)
• Real-time burn rate monitoring with optimization suggestions
```

## 🏗️ Enterprise Architecture

### Advanced Network Optimization (v0.4.0)

CargoShip v0.4.0 introduces production-proven network algorithms:

- **BBR Congestion Control**: Google's production-tested algorithm for optimal bandwidth utilization
- **CUBIC TCP Algorithm**: Linux kernel's proven congestion window management  
- **RTT Estimation**: Signal processing with Kalman filtering and statistical methods
- **Loss Detection**: Multi-method packet loss detection with deterministic recovery
- **Bandwidth-Delay Product**: Dynamic buffer sizing with network-aware optimization

### Deployment Architecture

Deploy CargoShip with enterprise-grade features:

```yaml
# docker-compose.yml for enterprise deployment
version: '3.8'
services:
  cargoship-enterprise:
    image: scttfrdmn/cargoship:v0.4.0
    volumes:
      - /mnt/enterprise-storage:/data:ro
      - ./config:/config
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
      - CARGOSHIP_DESTINATION=s3://enterprise-archive
      - CARGOSHIP_STORAGE_CLASS=intelligent-tiering
      - CARGOSHIP_ENABLE_BBR=true
      - CARGOSHIP_ENABLE_CUBIC=true
      - CARGOSHIP_ADVANCED_MONITORING=true
      - CARGOSHIP_MAX_MONTHLY_COST=5000
```

### Intelligent Data Detection

CargoShip automatically detects datasets ready for archival:

```bash
# Configure advanced archival rules with flow control
cargoship config set rules.auto-archive true
cargoship config set rules.detect-patterns "*.bam,*.fastq.gz,analysis_complete.txt"
cargoship config set rules.min-age-days 7
cargoship config set rules.storage-class intelligent-tiering
cargoship config set flow-control.algorithm bbr
cargoship config set monitoring.enable-advanced-metrics true
```

## 🏗️ Enterprise Architecture

```
┌─────────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Enterprise Data   │    │  CargoShip       │    │   AWS S3        │
│                     │    │  v0.4.0          │    │                 │
│ • Data Lakes        │───▶│                  │───▶│ • All Storage   │
│ • Analytics Output  │    │ • BBR/CUBIC      │    │   Classes       │
│ • ML Training Data  │    │ • RTT Estimation │    │ • Intelligent   │
│ • Archive Systems   │    │ • Loss Recovery  │    │   Tiering       │
└─────────────────────┘    │ • BDP Optimization│    │ • Cost Optimize │
                           └──────────────────┘    └─────────────────┘
```

## 📖 Documentation

### Getting Started
- **[Installation Guide](docs/install.md)** - Get CargoShip running in your environment
- **[User Guide](docs/USER_GUIDE.md)** - Complete feature walkthrough
- **[Quick Start Wizard](docs/wizard.md)** - Interactive setup guide
- **[Complete Documentation](https://cargoship.app)** - Full documentation site

### Key Features
- **[Advanced Flow Control](docs/TASK_4_ADVANCED_FLOW_CONTROL.md)** - v0.4.0 network optimization algorithms
- **[AWS Integration Guide](docs/AWS_INTEGRATION_REPORT.md)** - Complete AWS setup and configuration
- **[Cost Management](docs/cost-management.md)** - Intelligent cost optimization features
- **[Architecture Overview](docs/ARCHITECTURE.md)** - System design and components

### Deployment & Operations
- **[Deployment Guide](docs/DEPLOYMENT_GUIDE.md)** - Production deployment strategies
- **[Launch Agent Setup](docs/launch-agent.md)** - Enterprise agent deployment
- **[Ghost Ship Deployment](docs/deployment/GHOST_SHIP_DEPLOYMENT_GUIDE.md)** - Distributed agent setup

## 🤝 Contributing

CargoShip welcomes contributions from developers and researchers! Built on Duke University's SuitcaseCTL foundation with enterprise enhancements.

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Commit your changes: `git commit -m 'Add amazing feature'`
4. Push to the branch: `git push origin feature/amazing-feature`
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## 📄 License and Attribution

CargoShip is licensed under the MIT License. See [LICENSE](LICENSE) for details.

**Built on SuitcaseCTL**: CargoShip extends [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl) by Duke University. We gratefully acknowledge their innovative foundation for research data management.

## 🆘 Support

- **Documentation**: [cargoship.app](https://cargoship.app)
- **Issues**: [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)
- **Community**: [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)

---

**Ship your data with confidence. Ship it with CargoShip.** 🚢