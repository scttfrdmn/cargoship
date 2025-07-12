# CargoShip

> **Intelligent research data archival for AWS S3**

[![Go Reference](https://pkg.go.dev/badge/github.com/scttfrdmn/cargoship.svg)](https://pkg.go.dev/github.com/scttfrdmn/cargoship)
[![Go Report Card](https://goreportcard.com/badge/github.com/scttfrdmn/cargoship)](https://goreportcard.com/report/github.com/scttfrdmn/cargoship)
[![Test Coverage](https://img.shields.io/badge/coverage-85%25-brightgreen.svg)](https://github.com/scttfrdmn/cargoship)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

CargoShip helps researchers move their data to AWS S3 intelligently and cost-effectively. Built on the foundation of Duke University's [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl), CargoShip adds smart archival workflows, cost optimization, and simple deployment for research environments.

## 🔬 For Researchers, By Researchers

**Move your research data to S3 the smart way:**
- 📊 **Automatic cost optimization** - Save up to 90% on storage costs
- 🚀 **Launch agents** - Deploy on lab NAS boxes and research servers  
- 🧠 **Smart data detection** - Automatically identify completed datasets
- 💰 **Budget-friendly** - Perfect for research grants and academic budgets
- 🔍 **Simple setup** - No complex enterprise infrastructure required

![CargoShip Research Demo](docs/vhs/demo.gif)

## 🚀 Quick Start for Researchers

### Installation

```bash
# Install CargoShip
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest

# Or download pre-built binary
curl -sSL https://get.cargoship.dev/install.sh | sh
```

### Basic Research Workflow

```bash
# 1. Survey your research data and estimate costs
cargoship survey /data/research-project-2024
cargoship estimate /data/completed-analysis --storage-class deep-archive

# 2. Archive completed research datasets
cargoship ship /data/completed-analysis \
  --destination s3://my-research-bucket/project-2024 \
  --storage-class deep-archive \
  --max-cost-per-month 50

# 3. Deploy launch agent on your lab NAS
docker run -d --name cargoship-agent \
  -v /mnt/research-data:/data:ro \
  -v ~/.aws:/root/.aws:ro \
  scttfrdmn/cargoship:launch \
  --watch /data --auto-archive
```

## 💰 Research Budget Optimization

CargoShip helps stretch your research budget with intelligent cost optimization:

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

💡 Research Recommendations:
• Archive raw sequencing data → Deep Archive (90% savings)
• Keep analysis results → Glacier (75% savings)  
• Enable lifecycle policies → Additional 15% savings

Total research budget savings: $3,170/year
```

## 🧪 Research Lab Integration

### Launch Agents for Lab Infrastructure

Deploy CargoShip agents on your existing lab infrastructure:

```yaml
# docker-compose.yml for lab deployment
version: '3.8'
services:
  cargoship-agent:
    image: scttfrdmn/cargoship:launch
    volumes:
      - /mnt/lab-nas:/data:ro
      - ./config:/config
    environment:
      - CARGOSHIP_WATCH_PATHS=/data/completed,/data/analysis-output
      - CARGOSHIP_DESTINATION=s3://lab-research-archive
      - CARGOSHIP_STORAGE_CLASS=deep-archive
      - CARGOSHIP_MAX_MONTHLY_COST=200
```

### Smart Dataset Detection

CargoShip automatically detects completed research datasets:

```bash
# Configure intelligent archival rules
cargoship config set rules.auto-archive true
cargoship config set rules.detect-patterns "*.bam,*.fastq.gz,analysis_complete.txt"
cargoship config set rules.min-age-days 7
cargoship config set rules.storage-class deep-archive
```

## 🏗️ Architecture for Research Labs

```
┌─────────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Research Data     │    │  CargoShip       │    │   AWS S3        │
│                     │    │  Launch Agent    │    │                 │
│ • Lab NAS           │───▶│                  │───▶│ • Deep Archive  │
│ • Analysis Servers  │    │ • Smart Detection│    │ • Intelligent   │
│ • Sequencing Output │    │ • Cost Optimizer │    │   Tiering       │
│ • Microscopy Data   │    │ • Budget Controls│    │ • Lifecycle     │
└─────────────────────┘    └──────────────────┘    └─────────────────┘
```

## 📖 Documentation

- **[Installation Guide](docs/install.md)** - Get CargoShip running in your lab
- **[Research Workflows](docs/research-guide.md)** - Common patterns for research data
- **[Launch Agent Setup](docs/launch-agent.md)** - Deploy agents on NAS and servers
- **[Cost Optimization](docs/cost-optimization.md)** - Maximize your research budget
- **[AWS Setup for Researchers](docs/aws-research-setup.md)** - Simple AWS configuration

## 🤝 Contributing

CargoShip welcomes contributions from the research community! Built on Duke University's SuitcaseCTL foundation.

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/research-feature`
3. Commit your changes: `git commit -m 'Add feature for research workflow'`
4. Push to the branch: `git push origin feature/research-feature`
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## 📄 License and Attribution

CargoShip is licensed under the MIT License. See [LICENSE](LICENSE) for details.

**Built on SuitcaseCTL**: CargoShip extends [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl) by Duke University. We gratefully acknowledge their innovative foundation for research data management.

## 🆘 Support

- **Documentation**: [cargoship.dev](https://cargoship.dev)
- **Issues**: [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)
- **Research Community**: [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)

---

**Move your research data intelligently. Ship it with CargoShip.** 🚢