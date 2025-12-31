# CargoShip Benchmarking Guide - Issue #166

**Last Updated**: December 30, 2025

This guide describes CargoShip's enhanced benchmarking capabilities with realistic datasets, AWS Open Data Registry integration, and multi-source storage testing.

---

## Table of Contents

1. [Overview](#overview)
2. [Benchmark Types](#benchmark-types)
3. [Realistic Test Data](#realistic-test-data)
4. [AWS Open Data Registry](#aws-open-data-registry)
5. [Storage Source Testing](#storage-source-testing)
6. [Running Benchmarks](#running-benchmarks)
7. [Interpreting Results](#interpreting-results)

---

## Overview

CargoShip provides three tiers of benchmarking data:

1. **Synthetic Data** (default) - Fast generation, consistent results
2. **Realistic Domain-Specific Data** (Issue #166) - Simulates real-world file distributions
3. **AWS Open Data Registry** (Issue #166) - Production datasets from public sources

---

## Benchmark Types

### Competitive Benchmark (`competitive-benchmark.sh`)

Compares CargoShip against leading S3 tools across 7 scenarios:
- **aws-cli** - Official AWS CLI (v2)
- **s5cmd** - High-performance parallel S3 tool
- **rclone** - Universal cloud storage sync tool
- **mc** - MinIO client for S3-compatible storage
- **cargoship** - This project (with advanced features)

**Test Scenarios**:
1. Small files (1KB-100KB, 10,000 files)
2. Large files (100MB-1GB, 100 files)
3. Mixed workload (varied sizes, 1,000 files)
4. Compression benefit (10GB compressible data)
5. Deduplication benefit (10GB with 50% duplicates)
6. Resume/retry (interrupted 1GB transfer)
7. Multi-region failover (primary region failure)

---

## Realistic Test Data

### Overview

Generate domain-specific file distributions that mirror real-world workloads.

### Supported Domains

#### 1. Software Engineering
**Distribution**: 45% code, 25% build artifacts, 15% media, 10% docs, 5% configs

**File Types**:
- **Code**: Python (.py), Go (.go), JavaScript (.js), Rust (.rs), Java (.java), C++ (.cpp)
  - Size: 5-55KB per file
  - Characteristics: Highly compressible text
- **Build Artifacts**: JAR (10-60MB), Python wheels (5-25MB), shared libraries (1-11MB)
  - Characteristics: Already compressed, binary data
- **Media**: PNG (50-250KB), SVG (10-60KB), JPEG (100-600KB)
  - Characteristics: Pre-compressed images
- **Documentation**: Markdown, reStructuredText, plain text (5-110KB)
  - Characteristics: Compressible text
- **Configuration**: YAML, JSON, TOML, XML (1-50KB)
  - Characteristics: Highly compressible structured data

**Use Cases**: Repository backups, CI/CD artifacts, development environments

#### 2. Media Production
**Distribution**: 70% video, 15% audio, 10% graphics, 5% metadata

**File Types**:
- **Video**: MP4 (100-600MB), MOV (200-1200MB), AVI (150-950MB)
  - Characteristics: Already compressed, large files
- **Audio**: MP3 (3-13MB), WAV (10-60MB), FLAC (10-40MB)
  - Characteristics: Compressed audio codecs
- **Graphics**: PSD (20-120MB), AI (10-60MB), TIFF (50-250MB), PNG (5-25MB)
  - Characteristics: High-resolution images, some uncompressed
- **Metadata**: JSON sidecar files (5-55KB)
  - Characteristics: Small structured data

**Use Cases**: Video editing, post-production, asset management

#### 3. Database Backup
**Distribution**: 60% dumps, 30% WAL logs, 10% configs

**File Types**:
- **Database Dumps**: SQL (50-550MB), binary dumps (100-1100MB), compressed SQL (20-220MB)
  - Characteristics: Highly compressible if uncompressed, structured data
- **WAL Logs**: Binary transaction logs (1-17MB, typically 16MB)
  - Characteristics: Incremental, time-series
- **Configuration**: PostgreSQL/MySQL configs
  - Characteristics: Small text files

**Use Cases**: Database backups, point-in-time recovery, disaster recovery

#### 4. Scientific Computing
**Distribution**: 40% data files, 35% images, 15% results, 10% docs

**File Types**:
- **Data Files**: HDF5 (50-550MB), NetCDF (30-330MB), CSV (10-110MB), Parquet (20-220MB)
  - Characteristics: Binary scientific formats, some compressed
- **Images**: TIFF (20-120MB), FITS (50-250MB), PNG (10-60MB)
  - Characteristics: High-resolution scientific imagery
- **Results**: CSV, JSON, text files (2-55MB)
  - Characteristics: Numerical data, compressible
- **Documentation**: Markdown papers and reports
  - Characteristics: Small text files

**Use Cases**: Research data archiving, genomics, climate modeling, astronomy

### Generation

```bash
# Generate all domains, small size (10GB)
./scripts/generate-realistic-test-data.sh

# Generate specific domain, medium size (100GB)
./scripts/generate-realistic-test-data.sh --domain software --size medium

# Generate large dataset (500GB) to external drive
./scripts/generate-realistic-test-data.sh \
  --domain all \
  --size large \
  --output-dir /Volumes/External/test-data
```

**Size Tiers**:
- **Small**: 10GB, 10,000 files (~2.5GB per domain)
- **Medium**: 100GB, 100,000 files (~25GB per domain)
- **Large**: 500GB, 1,000,000 files (~125GB per domain)

---

## AWS Open Data Registry

### Overview

Download real-world production datasets from AWS Open Data Registry for authentic benchmarking.

**Benefits**:
- Free egress within AWS (same region)
- No AWS credentials required (public buckets)
- Real file formats and sizes
- Reproducible across environments

### Supported Datasets

#### 1. Landsat 8 - Satellite Imagery
**Bucket**: `s3://landsat-pds` (no-sign-request)
**Format**: GeoTIFF
**Size**: 1PB total (sample: 1-100GB)
**Use Case**: Geospatial analysis, remote sensing
**Reference**: https://registry.opendata.aws/landsat-8/

**Characteristics**:
- Large TIFF files (hundreds of MB per scene)
- Minimal compression opportunity
- High I/O throughput requirements

#### 2. 1000 Genomes - Genomics Data
**Bucket**: `s3://1000genomes` (no-sign-request)
**Format**: BAM, VCF, VCF.GZ
**Size**: 200TB total (sample: 1-100GB)
**Use Case**: Genomic analysis, bioinformatics
**Reference**: https://registry.opendata.aws/1000-genomes/

**Characteristics**:
- Mixed compressed (VCF.GZ) and binary (BAM) files
- Large file sizes (GB per sample)
- Sequential access patterns

#### 3. NOAA NEXRAD - Weather Radar
**Bucket**: `s3://noaa-nexrad-level2` (no-sign-request)
**Format**: Binary radar data
**Size**: 300TB total (sample: 1-100GB)
**Use Case**: Weather modeling, climate research
**Reference**: https://registry.opendata.aws/noaa-nexrad/

**Characteristics**:
- Time-series binary data
- Continuous data stream (updates every 5-10 min)
- Moderate file sizes (10-50MB)

#### 4. NASA NEX - Climate Models
**Bucket**: `s3://nasanex` (no-sign-request)
**Format**: NetCDF
**Size**: 500TB total (sample: 1-100GB)
**Use Case**: Climate modeling, Earth science
**Reference**: https://registry.opendata.aws/nasanex/

**Characteristics**:
- Multidimensional scientific arrays
- NetCDF compressed format
- Large files (100s MB to GB)

#### 5. Common Crawl - Web Archives
**Bucket**: `s3://commoncrawl` (no-sign-request)
**Format**: WARC (Web ARChive)
**Size**: 400TB total (sample: 1-100GB)
**Use Case**: NLP, web analysis, search indexing
**Reference**: https://registry.opendata.aws/commoncrawl/

**Characteristics**:
- Pre-compressed WARC.GZ files
- Large files (100-500MB each)
- Text-heavy content

#### 6. Allen Brain Atlas - Neuroscience
**Bucket**: `s3://allen-brain-observatory` (no-sign-request)
**Format**: NWB (Neurodata Without Borders), HDF5
**Size**: 10TB total (sample: 1-100GB)
**Use Case**: Neuroscience research, brain imaging
**Reference**: https://registry.opendata.aws/allen-brain-observatory/

**Characteristics**:
- HDF5-based scientific format
- Large datasets (GB per experiment)
- Complex hierarchical structure

#### 7. SpaceNet - Satellite Imagery
**Bucket**: `s3://spacenet-dataset` (no-sign-request)
**Format**: GeoTIFF, GeoJSON
**Size**: 1TB total (sample: 1-100GB)
**Use Case**: Computer vision, urban planning
**Reference**: https://registry.opendata.aws/spacenet/

**Characteristics**:
- High-resolution imagery
- Mixed vector (GeoJSON) and raster (GeoTIFF)
- Tarballs for bulk download

### Download

```bash
# Download single dataset, small sample (1GB)
./scripts/download-aws-open-data.sh --dataset landsat

# Download multiple datasets, medium sample (10GB)
./scripts/download-aws-open-data.sh \
  --dataset landsat,genomes,noaa \
  --sample-size medium

# Download large sample (100GB) to external drive
./scripts/download-aws-open-data.sh \
  --dataset nasa \
  --sample-size large \
  --output-dir /Volumes/External/aws-data
```

**Sample Sizes**:
- **Small**: ~1GB, 100 files
- **Medium**: ~10GB, 1,000 files
- **Large**: ~100GB, 10,000 files

---

## Storage Source Testing

### Overview

Test performance characteristics of different storage types to simulate real-world deployment scenarios.

### Supported Storage Sources

#### 1. NVMe SSD (default)
**Read Speed**: 3,500 MB/s
**Characteristics**:
- Highest performance
- Typical for modern development workstations
- Best case scenario

**Use Cases**: Development, high-performance computing, real-time processing

#### 2. SATA SSD
**Read Speed**: 550 MB/s
**Characteristics**:
- Common in enterprise servers
- Good balance of performance and cost
- Typical case scenario

**Use Cases**: Production servers, enterprise storage, virtualized environments

#### 3. HDD (Hard Disk Drive)
**Read Speed**: 150 MB/s
**Characteristics**:
- Legacy storage, budget-conscious deployments
- Sequential read performance
- Worst case scenario for solid-state

**Use Cases**: Archival storage, budget deployments, legacy infrastructure

#### 4. NAS (Network-Attached Storage)
**Read Speed**: 125 MB/s (1Gbps network)
**Characteristics**:
- Network-limited performance
- Adds latency and network overhead
- Common in enterprise file shares

**Use Cases**: Shared storage, network file systems, distributed teams

### Configuration

Storage source is specified via `--storage-source` flag. The benchmark framework records this metadata in results for comparison.

```bash
# Test with NVMe (default, best case)
./scripts/competitive-benchmark.sh --storage-source nvme

# Test with SATA SSD (typical case)
./scripts/competitive-benchmark.sh --storage-source sata

# Test with HDD (worst case)
./scripts/competitive-benchmark.sh --storage-source hdd

# Test with NAS (network-limited)
./scripts/competitive-benchmark.sh --storage-source nas
```

**Note**: Storage source metadata is recorded in benchmark results. For simulated performance testing with artificial rate limiting, see advanced configuration.

---

## Running Benchmarks

### Quick Start (Synthetic Data)

```bash
# Default: synthetic data, NVMe storage
./scripts/competitive-benchmark.sh
```

### Realistic Domain-Specific Data

```bash
# Step 1: Generate realistic test data
./scripts/generate-realistic-test-data.sh \
  --domain all \
  --size small \
  --output-dir /tmp/realistic-data

# Step 2: Run benchmark with realistic data
./scripts/competitive-benchmark.sh \
  --use-realistic-data \
  --test-data-dir /tmp/realistic-data \
  --storage-source sata
```

### AWS Open Data Registry

```bash
# Step 1: Download AWS Open Data samples
./scripts/download-aws-open-data.sh \
  --dataset landsat,genomes \
  --sample-size medium \
  --output-dir /tmp/aws-data

# Step 2: Run benchmark with AWS data
./scripts/competitive-benchmark.sh \
  --use-aws-open-data \
  --test-data-dir /tmp/aws-data \
  --storage-source nvme
```

### Comprehensive Testing Matrix

Test across multiple dimensions for complete performance profiling:

```bash
#!/bin/bash
# Comprehensive benchmark matrix - Issue #166

# Datasets: synthetic, realistic, AWS Open Data
# Storage: NVMe, SATA, HDD, NAS

# 1. Synthetic + NVMe (baseline)
./scripts/competitive-benchmark.sh \
  --storage-source nvme \
  --results-dir ./results/synthetic-nvme

# 2. Realistic + SATA (typical production)
./scripts/generate-realistic-test-data.sh --domain all --size medium
./scripts/competitive-benchmark.sh \
  --use-realistic-data \
  --storage-source sata \
  --results-dir ./results/realistic-sata

# 3. AWS Open Data + HDD (worst case)
./scripts/download-aws-open-data.sh --dataset landsat,genomes --sample-size large
./scripts/competitive-benchmark.sh \
  --use-aws-open-data \
  --storage-source hdd \
  --results-dir ./results/aws-hdd

# Compare results
echo "Benchmark matrix complete. Results in ./results/"
```

---

## Interpreting Results

### Output Files

Each benchmark run produces:
- **results.csv** - Raw timing data and metrics
- **report.md** - Detailed comparison report with analysis
- **compression.txt** - Compression ratio and savings (if applicable)
- **dedup.txt** - Deduplication ratio and savings (if applicable)
- **{scenario}-{tool}.log** - Execution logs per tool per scenario

### Results CSV Format

```csv
scenario,tool,duration_ms,throughput_mbps,file_count,total_size_mb,put_requests,estimated_cost_usd
scenario1,cargoship,45230,425.3,10000,524,10000,0.050
scenario1,s5cmd,52100,390.1,10000,524,10000,0.050
...
```

### Key Metrics

**Duration (ms)**: Total execution time
**Throughput (Mbps)**: Effective network throughput (MB/s * 8)
**File Count**: Number of files uploaded
**Total Size (MB)**: Total data size
**PUT Requests**: S3 PUT request count (cost factor)
**Estimated Cost**: Approximate AWS cost (PUT requests + 1 hour storage)

### Comparing Across Configurations

When comparing benchmarks with different data types or storage sources:

1. **Synthetic vs. Realistic**: Realistic data shows more variance due to diverse file sizes and types
2. **Storage Source Impact**: Slower storage (HDD, NAS) increases overall time but may not affect S3 throughput if network is slower
3. **AWS Open Data**: Production datasets may have different compression/dedup characteristics than synthetic

### Example Analysis

```
Scenario 1: Small Files (10,000 files, 524MB)

| Tool       | Synthetic/NVMe | Realistic/SATA | AWS Data/HDD | Relative |
|------------|----------------|----------------|--------------|----------|
| cargoship  | 45.2s          | 52.1s          | 68.3s        | 1.00x    |
| s5cmd      | 52.1s          | 58.9s          | 75.2s        | 1.13x    |
| aws-cli    | 98.3s          | 105.2s         | 122.1s       | 2.17x    |

Key Findings:
- CargoShip fastest across all configurations
- Storage source adds 15-50% overhead depending on tool
- Realistic data +15% time vs. synthetic (more file type diversity)
- AWS Open Data +50% time vs. synthetic (larger individual files)
```

---

## Best Practices

### Data Generation

1. **Match your workload**: Choose domain that matches your use case
2. **Size appropriately**: Start with small (10GB), scale to medium (100GB) for production-like tests
3. **External storage**: Use external drives for large datasets to avoid filling system disk

### AWS Open Data

1. **Same region**: Download in same AWS region as benchmark to avoid egress charges
2. **Incremental**: Start with small samples, increase size gradually
3. **Cleanup**: Large datasets can consume significant disk space - clean up after benchmarking

### Storage Source Testing

1. **Baseline first**: Always run NVMe baseline for comparison
2. **Network vs. disk**: NAS tests show network impact, HDD tests show disk I/O impact
3. **Consistent environment**: Run all storage source tests on same hardware for fair comparison

### Reproducibility

1. **Document configuration**: Record AWS region, storage source, data type in results
2. **Version tools**: Note versions of cargoship, s5cmd, aws-cli, etc.
3. **System specs**: Include CPU, RAM, network speed in reports
4. **Multiple runs**: Run 3+ iterations and average results for statistical validity

---

## Advanced Configuration

### Custom Realistic Data

Create your own domain-specific distributions by modifying `generate-realistic-test-data.sh`:

```bash
# Example: 80% video, 20% metadata (custom media workflow)
generate_custom_domain() {
    local domain_dir="$OUTPUT_DIR/custom-media"
    local domain_size=$1
    local domain_files=$2

    mkdir -p "$domain_dir/video"
    mkdir -p "$domain_dir/metadata"

    # 80% video files
    video_files=$((domain_files * 80 / 100))
    # ... generate video files

    # 20% metadata files
    metadata_files=$((domain_files * 20 / 100))
    # ... generate metadata files
}
```

### Rate-Limited Storage Simulation

For true storage source simulation (not just metadata), use `pv` (pipe viewer) to rate-limit reads:

```bash
# Install pv
brew install pv  # macOS
apt-get install pv  # Ubuntu

# Simulate HDD (150 MB/s) during upload
find ./test-data -type f -print0 | \
  xargs -0 -P4 -I{} bash -c "pv -L 150M {} | cargoship upload - s3://bucket/prefix"
```

### Multi-Region Testing

Test multi-region failover with realistic data:

```yaml
# multiregion-config.yaml
multiregion:
  enabled: true
  regions:
    - name: us-west-2
      weight: 70
    - name: us-east-1
      weight: 30
  load_balancing_algorithm: weighted_round_robin
```

```bash
./scripts/competitive-benchmark.sh \
  --use-realistic-data \
  --test-data-dir /tmp/realistic-data \
  --config multiregion-config.yaml
```

---

## Troubleshooting

### Out of Disk Space

**Problem**: Test data generation fails with "No space left on device"

**Solution**:
```bash
# Use external drive
./scripts/generate-realistic-test-data.sh \
  --output-dir /Volumes/External/test-data

# Or use smaller size
./scripts/generate-realistic-test-data.sh --size small
```

### AWS CLI Credentials

**Problem**: `aws s3 ls` fails with "Unable to locate credentials"

**Solution**: AWS Open Data uses public buckets with `--no-sign-request` flag (no credentials needed). If you see this error, ensure you're using the `download-aws-open-data.sh` script which handles this automatically.

### Slow Data Generation

**Problem**: Realistic data generation takes too long

**Solution**:
```bash
# Generate on fast storage (NVMe)
./scripts/generate-realistic-test-data.sh \
  --domain software \
  --size small \
  --output-dir /tmp/fast-data

# Then move to slower storage for testing if needed
mv /tmp/fast-data /Volumes/HDD/test-data
```

### Benchmark Timeout

**Problem**: Benchmark times out or hangs

**Solution**:
- Check S3 bucket permissions
- Verify network connectivity to AWS
- Reduce dataset size
- Check system resources (RAM, CPU)

---

## Future Enhancements

Planned features for future releases:

- **Automated nightly benchmarks** (v0.7.0) - Continuous performance monitoring
- **Public benchmark dashboard** (v0.8.0) - Community results at benchmarks.cargoship.dev
- **Cost benchmarking** (Issue #165) - Total cost of ownership comparisons
- **Domain-specific scenarios** (v0.8.0) - Medical imaging, ML datasets, financial data

---

## References

- **Issue #166**: Realistic Benchmark Data & Infrastructure
- **Issue #34**: Best-in-Class S3 Tool
- **Issue #165**: Cost Benchmarking & Transparency
- **AWS Open Data Registry**: https://registry.opendata.aws
- **CargoShip GitHub**: https://github.com/scttfrdmn/cargoship

---

**Last Updated**: December 30, 2025
**CargoShip Version**: v0.6.0+
