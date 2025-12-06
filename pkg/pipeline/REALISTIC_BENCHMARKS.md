# Realistic Scientific Data Benchmarks

This document describes the realistic benchmark suite for CargoShip, designed to accurately measure performance with genomic and scientific imaging workloads.

## Problem

The original benchmarks used artificial test data (`byte(i % 256)`) that compressed extremely well (~90%+ compression), giving misleading throughput numbers. Real-world scientific data compresses much less, leading to dramatically different network utilization patterns.

**Original benchmark**: 1,112 MB/s throughput with 10 shards
- Test data: Repetitive patterns
- Compression: ~90% (10MB → ~1MB)
- Network usage: ~111 MB/s (17% of theoretical max)

**Real-world scientific data** (genomics, imaging):
- Compression: ~5-20% (often pre-compressed formats)
- Network usage: ~890-1,056 MB/s (142-169% of 5 Gbps link capacity)
- **Result**: Bottlenecks on 5 Gbps links, requires 10 Gbps for full throughput

## Benchmark Suite

### Synthetic Data Generators

The benchmarks use synthetic data generators that match real-world entropy characteristics:

#### 1. Genomic Data

**FASTQ (Raw Sequencing Reads)**
- 4-letter DNA alphabet (A, C, G, T) + quality scores
- Expected compression: 15-20%
- Use case: Whole genome sequencing, RNA-seq

**BAM (Aligned Sequences)**
- Pre-compressed binary format
- Expected compression: 0-10% additional
- Use case: Aligned genomic reads

**VCF (Variant Calls)**
- Text-based tabular format with high redundancy
- Expected compression: 60-80%
- Use case: Genetic variants, SNPs

#### 2. Scientific Imaging Data

**TIFF (Microscopy Stacks)**
- Uncompressed or LZW compressed pixel data
- Moderate entropy with local correlation
- Expected compression: 10-30%
- Use case: Confocal, light-sheet microscopy

**DICOM (Medical Imaging)**
- Often pre-compressed (JPEG2000)
- Expected compression: 0-10% additional
- Use case: CT, MRI scans

#### 3. Pre-Compressed Archives

**tar.gz, ZIP**
- Already compressed data
- Expected compression: 0-5%
- Use case: Existing archives, backups

### Available Benchmarks

| Benchmark | Data Type | Size | Files | Shards | Expected Network Usage (5 Gbps) |
|-----------|-----------|------|-------|--------|--------------------------------|
| `BenchmarkRealistic_FASTQ_1GB` | FASTQ | 1GB | 50 × 20MB | 8 | ~142-151% ⚠️ |
| `BenchmarkRealistic_FASTQ_10GB` | FASTQ | 10GB | 100 × 100MB | 10 | ~142-151% ⚠️ |
| `BenchmarkRealistic_BAM_5GB` | BAM | 5GB | 10 × 500MB | 8 | ~160-178% 🔴 |
| `BenchmarkRealistic_VCF_500MB` | VCF | 500MB | 50 × 10MB | 8 | ~48-64% ✓ |
| `BenchmarkRealistic_TIFF_2GB` | TIFF | 2GB | 20 × 100MB | 8 | ~124-160% 🔴 |
| `BenchmarkRealistic_TIFF_20GB` | TIFF | 20GB | 40 × 500MB | 10 | ~124-160% 🔴 |
| `BenchmarkRealistic_DICOM_5GB` | DICOM | 5GB | 100 × 50MB | 8 | ~160-178% 🔴 |
| `BenchmarkRealistic_PreCompressed_10GB` | Pre-compressed | 10GB | 20 × 500MB | 10 | ~169-178% 🔴 |
| `BenchmarkRealistic_MixedWorkload_10GB` | Mixed | 10GB | 100 files | 10 | ~140-150% ⚠️ |

**Legend**:
- ✓ OK: Within 5 Gbps capacity (<80% utilization)
- ⚠️ CAUTION: High utilization (80-100%) or slight oversubscription
- 🔴 WARNING: Exceeds 5 Gbps capacity (>100%), requires 10 Gbps link

## Running Benchmarks

### Prerequisites

1. **AWS credentials configured**:
   ```bash
   export AWS_PROFILE=your-profile
   export AWS_REGION=us-west-2
   ```

2. **S3 test bucket**:
   ```bash
   export CARGOSHIP_TEST_BUCKET=your-test-bucket
   ```

3. **Enable S3 integration tests**:
   ```bash
   export CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1
   ```

### Running Individual Benchmarks

```bash
# Run FASTQ 1GB benchmark
go test -v -tags=benchmark -run='^$' -bench=BenchmarkRealistic_FASTQ_1GB \
  -benchtime=1x -timeout=30m ./pkg/pipeline

# Run TIFF 2GB benchmark
go test -v -tags=benchmark -run='^$' -bench=BenchmarkRealistic_TIFF_2GB \
  -benchtime=1x -timeout=30m ./pkg/pipeline

# Run mixed workload benchmark
go test -v -tags=benchmark -run='^$' -bench=BenchmarkRealistic_MixedWorkload_10GB \
  -benchtime=1x -timeout=60m ./pkg/pipeline
```

### Running All Realistic Benchmarks

```bash
# Run all realistic benchmarks (WARNING: Takes 2-4 hours, uses significant AWS resources)
go test -v -tags=benchmark -run='^$' -bench=BenchmarkRealistic \
  -benchtime=1x -timeout=4h ./pkg/pipeline
```

### Interpreting Results

Each benchmark reports:

```
=== FASTQ Benchmark Results ===
Configuration:
  Files: 50 @ 20.00 MB each = 1.00 GB total
  Shards: 8

Compression:
  Input size: 1.00 GB
  Estimated uploaded size: 0.83 GB (17.5% compression)

Performance:
  Upload duration: 12.5s
  Processing throughput: 82.0 MB/s (uncompressed)
  Estimated network throughput: 68.0 MB/s (compressed)

Network Capacity Analysis:
  5 Gbps link (625 MB/s): 145.0% saturation
  ⚠️  WARNING: Exceeds 5 Gbps capacity - requires 10 Gbps link
  10 Gbps link (1250 MB/s): 72.5% saturation
```

**Key metrics**:
- **Processing throughput**: Speed of reading/compressing data (uncompressed MB/s)
- **Network throughput**: Speed of uploading to S3 (compressed MB/s)
- **5 Gbps saturation**: Network utilization on 5 Gbps link
  - <80%: Comfortable headroom
  - 80-100%: High utilization, may experience congestion
  - >100%: Bottleneck, requires 10 Gbps link
- **10 Gbps saturation**: Network utilization on 10 Gbps link

## Network Capacity Planning

### Recommended Network Links by Workload

| Workload Size | Data Type | Recommended Link | Rationale |
|---------------|-----------|------------------|-----------|
| <1 GB | Any | 1 Gbps | Low volume, any modern link sufficient |
| 1-10 GB | FASTQ, VCF | 5 Gbps | Good compression, fits within 5 Gbps |
| 1-10 GB | BAM, TIFF, DICOM | 10 Gbps | Minimal compression, needs headroom |
| >10 GB | Any | 10 Gbps | Large transfers benefit from maximum throughput |

### Real-World Use Cases

**Genomics Core Facility**:
- 10 NovaSeq runs/week @ 500 GB/run (raw FASTQ)
- Need: <2 hour upload time per run
- Calculation: 500 GB / 2 hr = 70 MB/s average
- **Recommendation**: 1 Gbps link sufficient (with 8-10 parallel uploads)

**Microscopy Core**:
- 50 imaging sessions/week @ 100 GB/session (TIFF stacks)
- Need: <30 min upload time per session
- Calculation: 100 GB / 30 min = 56 MB/s average
- **Recommendation**: 1 Gbps link sufficient (upload during acquisition)

**Clinical Imaging Center**:
- 200 patients/day @ 1 GB/patient (DICOM)
- Need: <1 min upload time per patient
- Calculation: 200 GB / 200 min = 17 MB/s average
- **Recommendation**: Any modern link (100 Mbps+)

## Cost Estimation

AWS S3 costs (us-west-2, January 2025):
- **PUT requests**: $0.005 per 1,000 requests
- **Storage (Standard)**: $0.023 per GB/month
- **Data transfer OUT**: $0.09 per GB (first 10 TB/month)

### Example: 10GB Upload

Assuming 10GB upload with 64MB chunks:
- Chunks: ~160 chunks
- PUT requests: 160 × $0.005/1000 = $0.0008
- Storage (1 month): 10 × $0.023 = $0.23
- **Total monthly cost**: ~$0.23 (negligible API cost)

### Large-Scale Example: 1TB/month Upload

Assuming 1TB upload with 64MB chunks:
- Chunks: ~16,000 chunks
- PUT requests: 16,000 × $0.005/1000 = $0.08
- Storage (1 month): 1,000 × $0.023 = $23.00
- **Total monthly cost**: ~$23.08 (API cost <0.5%)

**Key insight**: S3 API costs are negligible (<1%) for realistic scientific workloads. Storage dominates costs.

## Troubleshooting

### Benchmark Hangs or Times Out

**Symptom**: Benchmark doesn't complete within timeout
**Cause**: Network congestion or S3 throttling
**Solution**:
1. Check network utilization: `nload` or `iftop`
2. Reduce shard count or file size
3. Increase timeout: `-timeout=60m` → `-timeout=120m`

### "Skip" Message

**Symptom**: "Skipping realistic benchmark: requires CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1"
**Solution**: Set environment variable:
```bash
export CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1
```

### AWS Credentials Errors

**Symptom**: "Failed to load AWS config"
**Solution**: Configure AWS credentials:
```bash
aws configure --profile your-profile
export AWS_PROFILE=your-profile
```

### S3 Bucket Access Denied

**Symptom**: "Access Denied" or "Bucket does not exist"
**Solution**:
1. Create bucket: `aws s3 mb s3://your-test-bucket`
2. Verify permissions: `aws s3 ls s3://your-test-bucket`
3. Set correct bucket: `export CARGOSHIP_TEST_BUCKET=your-test-bucket`

## Development Notes

### Adding New Data Generators

To add a new scientific data type:

1. Create generator function:
```go
func generateNewDataType(size int64) []byte {
    data := make([]byte, size)
    // Generate data with realistic entropy characteristics
    return data
}
```

2. Add benchmark:
```go
func BenchmarkRealistic_NewType_5GB(b *testing.B) {
    benchmarkRealisticWorkload(b, "NewType", generateNewDataType,
        500*1024*1024, 10, 8)
}
```

3. Update expected compression ratio in `benchmarkRealisticWorkload()`:
```go
case "NewType":
    expectedCompressionRatio = 0.30 // 30% compression
```

### Testing Data Generators

Run the compression characteristics test:
```bash
go test -tags=benchmark ./pkg/pipeline -run TestCompressionCharacteristics -v
```

This validates that generators produce data with expected entropy.

## References

- [AWS S3 Pricing](https://aws.amazon.com/s3/pricing/)
- [AWS S3 Performance Guidelines](https://docs.aws.amazon.com/AmazonS3/latest/userguide/optimizing-performance.html)
- [FASTQ Format Specification](https://en.wikipedia.org/wiki/FASTQ_format)
- [BAM/SAM Format](https://samtools.github.io/hts-specs/SAMv1.pdf)
- [DICOM Standard](https://www.dicomstandard.org/)
