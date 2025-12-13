# CargoHold Performance Benchmarks

Comprehensive performance benchmarking suite for CargoHold vs competitors.

## Overview

This benchmark suite compares CargoHold's parallel sharded upload system against:
- **s5cmd**: High-performance S3 CLI tool
- **MinIO mc**: MinIO client for S3-compatible storage
- **tar.zst**: Traditional tar with zstd compression + aws s3 cp

## Benchmark Scenarios

| Scenario | Files | Total Size | Description |
|----------|-------|------------|-------------|
| Small    | 10k   | ~1 GB      | Many small files (100KB each) |
| Medium   | 100k  | ~10 GB     | Typical dataset |
| Large    | 1M    | ~100 GB    | Large-scale archival |
| XLarge   | 10M   | ~1 TB      | Enterprise scale |

## Quick Start

```bash
# Install dependencies
./setup.sh

# Run all benchmarks
./run_benchmarks.sh

# Run specific scenario
./run_benchmarks.sh --scenario medium --tool cargohold

# Compare all tools
./compare.sh
```

## Requirements

- AWS credentials configured
- S3 bucket for testing
- Tools installed: cargoship, s5cmd, mc (MinIO client)
- Sufficient disk space for test data generation

## Test Data

Generated test data simulates real-world workloads:
- Mixed file sizes (1KB - 10MB)
- Various content types (text, binary, images, code)
- Realistic entropy characteristics

## Metrics Collected

- **Upload Time**: Total time to upload all files
- **Download Time**: Time to retrieve all files
- **Throughput**: MB/s during transfer
- **Memory Usage**: Peak RSS during operation
- **CPU Usage**: Average CPU utilization
- **S3 Request Count**: Total API requests made
- **Error Rate**: Failed operations percentage

## Shard Strategy Testing

CargoHold benchmarks test all shard strategies:
- **Hash**: Content-based hashing (default)
- **Size**: File size distribution
- **Type**: Content type grouping
- **Adaptive**: Dynamic strategy selection

## Output

Results are saved in:
- `results/`: Raw benchmark data (JSON)
- `reports/`: HTML reports with charts
- `regression/`: Historical comparison data

## CI Integration

Automated regression detection runs on:
- Pull requests (compare vs main)
- Nightly builds (track trends)
- Release candidates (validation)

## Performance Targets

| Tool | 10k files | 100k files | 1M files | 10M files |
|------|-----------|------------|----------|-----------|
| s5cmd | 3.6s | 36s | 6min | 60min |
| mc | 25.5s | 255s | 42min | 420min |
| tar.zst | 60s | 10min | 20min | 3hr |
| **CargoHold** | **<3s** | **<30s** | **<2min** | **<10min** |
