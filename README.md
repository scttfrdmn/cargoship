# CargoShip

> **Enterprise data archiving for AWS, built for speed and intelligence**

[![Go Reference](https://pkg.go.dev/badge/github.com/scttfrdmn/cargoship.svg)](https://pkg.go.dev/github.com/scttfrdmn/cargoship)
[![Go Report Card](https://goreportcard.com/badge/github.com/scttfrdmn/cargoship)](https://goreportcard.com/report/github.com/scttfrdmn/cargoship)
[![Test Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)](https://github.com/scttfrdmn/cargoship)
[![Security Scan](https://github.com/scttfrdmn/cargoship/actions/workflows/security.yml/badge.svg)](https://github.com/scttfrdmn/cargoship/actions/workflows/security.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/scttfrdmn/cargoship/badge)](https://securityscorecards.dev/viewer/?uri=github.com/scttfrdmn/cargoship)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fscttfrdmn%2Fcargoship.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fcargoship?ref=badge_shield)
[![Vulnerabilities](https://img.shields.io/snyk/vulnerabilities/github/scttfrdmn/cargoship.svg)](https://snyk.io/test/github/scttfrdmn/cargoship)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

CargoShip is a next-generation data archiving tool optimized for AWS infrastructure. Built on the foundation of Duke University's [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl), CargoShip adds native AWS integration, intelligent cost optimization, and enterprise-grade observability with advanced network optimization algorithms.

## 🚀 Enterprise Features with Research Flexibility

**Advanced data archiving for any environment:**
- 📊 **Intelligent cost optimization** - Save up to 90% with advanced algorithms
- ⚡ **Network optimization** - BBR and CUBIC flow control for maximum throughput  
- 🧠 **Smart compression** - ZSTD with adaptive chunking and staging
- 💰 **Cost intelligence** - Real-time analysis across all storage classes
- 🔍 **Enterprise observability** - Comprehensive metrics and monitoring
- 🛡️ **Security first** - KMS encryption and compliance-ready audit trails

![CargoShip Research Demo](docs/vhs/demo.gif)

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

# 2. Archive with enterprise-grade optimization
cargoship ship /data/completed-analysis \
  --destination s3://my-bucket/project-2024 \
  --storage-class intelligent-tiering \
  --enable-bbr-optimization \
  --max-cost-per-month 200

# 3. Deploy with advanced monitoring
docker run -d --name cargoship-agent \
  -v /mnt/data:/data:ro \
  -v ~/.aws:/root/.aws:ro \
  scttfrdmn/cargoship:latest \
  --watch /data --enable-advanced-flow-control
```

## 💰 Intelligent Cost Optimization

CargoShip provides enterprise-grade cost optimization with advanced algorithms:

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
• Analysis results → Glacier with BBR optimization (75% savings)  
• Enable lifecycle policies → Additional 15% savings
• Advanced flow control → 3x faster uploads

Total annual savings: $3,170/year with 200% performance gain
```

## 🏗️ Enterprise Architecture

### Advanced Network Optimization (v0.4.0)

CargoShip v0.4.0 introduces state-of-the-art flow control algorithms:

- **BBR Congestion Control**: Google's BBR algorithm for optimal bandwidth utilization
- **CUBIC TCP Algorithm**: Advanced congestion window management  
- **RTT Estimation**: Sophisticated round-trip time analysis with Kalman filtering
- **Loss Detection**: Multi-method packet loss detection and recovery
- **Bandwidth-Delay Product**: Dynamic buffer optimization for maximum throughput

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

- **[Installation Guide](docs/install.md)** - Get CargoShip running in your environment
- **[Advanced Flow Control](docs/TASK_4_ADVANCED_FLOW_CONTROL.md)** - v0.4.0 network optimization
- **[User Guide](docs/USER_GUIDE.md)** - Complete feature walkthrough
- **[Launch Agent Setup](docs/launch-agent.md)** - Enterprise deployment patterns
- **[AWS Integration](docs/AWS_INTEGRATION_REPORT.md)** - Complete AWS setup guide

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