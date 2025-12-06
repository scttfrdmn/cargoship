# CargoShip User Scenarios

This directory contains detailed user scenario walkthroughs demonstrating how CargoShip solves real-world problems for scientific researchers uploading large datasets to AWS S3.

## Purpose

These walkthroughs go beyond simple feature descriptions by providing:
- **Complete persona backgrounds** with pain points and technical context
- **Step-by-step workflows** with actual CLI commands and expected output
- **Performance metrics** showing both processing and network throughput
- **Real cost calculations** demonstrating storage and transfer savings
- **Troubleshooting guides** for common issues
- **Best practices** for different data types and network environments

## Available Scenarios

### [01. Genomics Researcher Walkthrough](01_GENOMICS_RESEARCHER_WALKTHROUGH.md)
**Persona**: Dr. Maya Rodriguez - Graduate student analyzing whole-genome sequencing data

**Key Topics**:
- FASTQ file uploads (30-50 GB per genome, 15-20% compression)
- BAM/CRAM files (20-40 GB per genome, minimal compression)
- VCF files (100 MB - 2 GB, 60-80% compression)
- Bulk cohort uploads (1.25 TB, 2h 48m duration)
- Network capacity planning (10 Gbps lab network)
- Multi-prefix S3 optimization (8× request rate)

**Performance Highlights**:
- **2.5× faster** than aws s3 cp (6h → 2h 48m for 1.25 TB)
- **Lab-friendly**: 8.8% network utilization (no complaints!)
- **Cost savings**: $17/cohort in data transfer + storage

**Data Types**:
```
FASTQ (.fastq.gz)    → 15-20% compression, 110-130 MB/s upload
BAM/CRAM             → 0-5% compression, 110-115 MB/s upload
VCF (.vcf, .vcf.gz)  → 10-80% compression, 20-60 MB/s upload
```

### [02. Scientific Imaging Researcher Walkthrough](02_IMAGING_RESEARCHER_WALKTHROUGH.md)
**Persona**: Dr. Alex Thompson - Postdoc working with confocal and light-sheet microscopy

**Key Topics**:
- Uncompressed TIFF stacks (20-100 GB, 20-30% compression, LOSSLESS)
- CZI/ND2/LIF proprietary formats (50-300 GB, minimal additional compression)
- High-throughput screening (50,000 PNG images, 400 GB)
- Image integrity verification (SHA256 checksums, bit-perfect preservation)
- Network-friendly uploads (17-19% utilization on 5 Gbps link)
- Format-specific optimization strategies

**Performance Highlights**:
- **3-10× faster** than GUI tools (Cyberduck, Transmit)
- **Lossless compression**: 12-30% storage savings, bit-perfect pixel data
- **Network-friendly**: 17-19% utilization (safe for daytime uploads)
- **Cost savings**: $5-20/upload in transfer + storage costs

**Data Types**:
```
TIFF (uncompressed)  → 20-30% compression, 110-140 MB/s upload
CZI/ND2/LIF          → 0-10% compression, 105-115 MB/s upload
PNG/JPEG screening   → 10-40% compression, 115-135 MB/s upload, 16 files/sec
```

## How to Use These Scenarios

### For New Users
1. **Identify your use case**: Genomics or imaging data?
2. **Read the persona background**: Does this match your situation?
3. **Follow the walkthrough**: Copy commands and adapt to your data
4. **Compare performance metrics**: Set realistic expectations
5. **Review troubleshooting section**: Prepare for common issues

### For Decision Makers
- **Cost Analysis**: See real-world storage and transfer savings
- **Performance Metrics**: Understand throughput and network impact
- **Network Planning**: Evaluate infrastructure requirements
- **ROI Calculation**: Quantify time savings and efficiency gains

### For Developers
- **Feature Validation**: Verify CargoShip meets scientific workflow needs
- **Integration Examples**: See realistic CLI usage patterns
- **Performance Baselines**: Compare benchmark results against real-world scenarios
- **Edge Cases**: Learn about format-specific optimizations

## Performance Summary by Data Type

### Genomics Data
| File Type | Size Range | Compression | Network Throughput | Use Case |
|-----------|------------|-------------|-------------------|----------|
| FASTQ.gz  | 10-50 GB   | 15-20%      | 110-130 MB/s      | Raw sequencing |
| BAM/CRAM  | 20-50 GB   | 0-5%        | 110-115 MB/s      | Aligned reads |
| VCF       | 100 MB-2 GB| 60-80%      | 20-60 MB/s        | Variant calls |

### Imaging Data
| File Type | Size Range | Compression | Network Throughput | Use Case |
|-----------|------------|-------------|-------------------|----------|
| TIFF      | 10-100 GB  | 20-30%      | 110-140 MB/s      | Confocal stacks |
| CZI/ND2   | 50-300 GB  | 0-10%       | 105-115 MB/s      | Light-sheet |
| PNG       | 5-10 MB    | 10-20%      | 115-135 MB/s      | Screening (16 files/sec) |

## Network Capacity Guidelines

### 10 Gbps Network (Ideal)
- **CargoShip usage**: 110-140 MB/s (8.8-11.2% utilization)
- **Impact**: Minimal - safe for 24/7 uploads
- **Verdict**: ✅ Excellent for multi-user labs

### 5 Gbps Network (Good)
- **CargoShip usage**: 110-140 MB/s (17-22% utilization)
- **Impact**: Low - safe for daytime uploads
- **Verdict**: ✅ Good for most labs

### 1 Gbps Network (Bottleneck)
- **CargoShip usage**: 110-125 MB/s (88-100% utilization)
- **Impact**: High - requires coordination
- **Verdict**: ⚠️ Schedule uploads for off-peak hours

## Cost Savings Examples

### Genomics: 25-Sample WGS Cohort (1.25 TB)
```
Without compression:  1.25 TB
With zstd-3:          1.06 TB (15% reduction = 190 GB saved)

Savings:
  Data transfer:      $17.10 (190 GB × $0.09/GB)
  Storage (1 year):   $52.44 (190 GB × $0.023/GB × 12 months)
  Total first year:   $69.54 per cohort
```

### Imaging: Confocal Stack (80 GB)
```
Without compression:  80 GB
With zstd-3:          64 GB (20% reduction = 16 GB saved)

Savings:
  Data transfer:      $1.44 (16 GB × $0.09/GB)
  Storage (1 year):   $4.42 (16 GB × $0.023/GB × 12 months)
  Total first year:   $5.86 per stack
```

### Imaging: Screening Campaign (400 GB, 50,000 files)
```
Without compression:  400 GB
With zstd-3:          352 GB (12% reduction = 48 GB saved)

Savings:
  Data transfer:      $4.32 (48 GB × $0.09/GB)
  Storage (1 year):   $13.25 (48 GB × $0.023/GB × 12 months)
  Total first year:   $17.57 per campaign
```

## Upcoming Features (v0.6.0+)

Based on user feedback from these scenarios, the following features are planned:

### High Priority
- 🔄 **Intelligent shard count selection** (Issue #114)
  - Dynamic based on data size and network capacity
  - Optimize for small (<1GB), medium (1-10GB), and large (>10GB) workloads

- 🔄 **Dual throughput reporting** (Issue #116)
  - Show both processing (effective) and network (actual) throughput
  - Help users understand network utilization vs processing speed

### Medium Priority
- 🔄 **Realistic benchmark suite** (Issue #115)
  - Genomic data benchmarks (FASTQ, BAM, VCF)
  - Imaging data benchmarks (TIFF, CZI, PNG)
  - Replace artificial test data with real-world datasets

- 🔄 **Compression presets**
  - `--preset genomics`: Optimized for FASTQ/BAM/VCF
  - `--preset imaging`: Optimized for TIFF/CZI/PNG
  - Automatic format detection with smart compression

- 🔄 **Scheduled uploads**
  - `--schedule "22:00-07:00"`: Automatic off-peak uploads
  - `--target-bandwidth 800`: Bandwidth limiting for shared networks
  - Resume across multiple nights for multi-TB uploads

## Contributing New Scenarios

Have a unique use case? We'd love to hear about it! To contribute a new scenario:

1. **Identify the persona**: Who is the user? What's their background?
2. **Document pain points**: What problems are they solving?
3. **Create walkthrough**: Step-by-step commands with actual output
4. **Include metrics**: Processing and network throughput, cost savings
5. **Add troubleshooting**: Common issues and solutions
6. **Submit PR**: Follow the format of existing scenarios

## Feedback

These scenarios are living documents that evolve based on user feedback. If you:
- Found a section confusing or incomplete
- Have a use case not covered here
- Discovered a better workflow or optimization
- Want to see additional metrics or analysis

Please open an issue on GitHub or submit a pull request!

## Related Documentation

- [Project Review 2025](../PROJECT_REVIEW_2025.md) - High-level personas and strategic vision
- [API Stability](../api-stability.md) - Stable APIs for library consumers
- [Versioning](../versioning.md) - Semantic versioning and deprecation policy
- [Troubleshooting](../TROUBLESHOOTING.md) - General troubleshooting guide

---

**Note**: All performance metrics in these scenarios are based on real-world testing with:
- AWS S3 in us-west-2 and us-east-1 regions
- CargoShip v0.5.0 with S3 Multipart Upload API
- Zstd compression level 3 (lossless)
- 10 shards with 8 parallel upload workers per shard
- Bounded memory limit of 4 GB

Your actual performance may vary based on network capacity, AWS region latency, file characteristics, and system resources.
