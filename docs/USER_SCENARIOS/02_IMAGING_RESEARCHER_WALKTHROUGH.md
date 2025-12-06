# Scenario 2: Scientific Imaging Researcher with Microscopy Data

## Persona: Dr. Alex Thompson

**Background**:
- Postdoctoral researcher in cell biology / neuroscience
- Works with confocal and light-sheet microscopy generating large 3D/4D image stacks
- Typical workload: 10-30 imaging sessions per month (~50-200 GB per session)
- Primary concern: **Preserving image quality while managing storage costs**
- Technical level: Strong in imaging software (FIJI/ImageJ), basic command line
- Works on lab workstation with 5 Gbps network link

**Pain Points**:
- TIFF stacks and proprietary formats (CZI, ND2) are massive (50-500 GB per experiment)
- Lab storage server fills up quickly (5 TB capacity for entire lab)
- Previous uploads via GUI tools (Cyberduck, Transmit) are painfully slow
- Worried about compressing images (might lose scientific data?)
- Current solution: Burns data to external hard drives, ships to collaborators (takes weeks!)

---

## Version Legend
- ✅ **v0.5.0 (Current)**: Features available today
- 🔄 **v0.6.0+ (Planned)**: Features in development (see linked GitHub issues)

## Current State (v0.5.0): What Works Today

### ✅ Initial Setup (Day 0) - Understanding Image Data

Alex's typical imaging datasets:
- **Multi-channel TIFF stacks** (confocal microscopy):
  - Resolution: 2048 × 2048 × 100-500 Z-planes × 4 channels
  - File size: 20-100 GB per time-lapse series
  - Format: Uncompressed TIFF or OME-TIFF
  - Compression potential: 10-30% with zstd (lossless!)
- **Light-sheet microscopy** (proprietary formats):
  - CZI files (Zeiss): 50-300 GB per sample
  - ND2 files (Nikon): 30-200 GB per experiment
  - Already internally compressed (minimal additional compression)
- **Single-plane images** (screening):
  - PNG/TIFF: 5-10 MB per image
  - Typical: 10,000-50,000 images per screening campaign (100-500 GB)
  - Compression: 20-40% with zstd (PNG already compressed, TIFF uncompressed)

**Network Environment**:
- Lab workstation: 5 Gbps link = 625 MB/s capacity
- Shared with 8 lab members (microscope users)
- "Peak hours" 9 AM - 6 PM (50-100 MB/s available per user)
- Full bandwidth available evenings and weekends

### ✅ First Upload: Single Confocal Stack (Day 1)

Alex just finished a confocal imaging session (4-channel Z-stack time-lapse: 80 GB).

```bash
# Install CargoShip
brew install scttfrdmn/tap/cargoship

# Configure AWS credentials (lab account)
aws configure --profile cellbio-lab
# AWS Access Key ID: [provided by lab IT]
# AWS Secret Access Key: [provided by lab IT]
# Default region: us-east-1
# Default output format: json

# Navigate to experiment directory
cd /Volumes/MicroscopyData/confocal/experiment-2024-10-15

# Check directory structure
tree -L 2
# Output:
# .
# ├── metadata.xml (Zeiss metadata)
# ├── stack_ch1_gfp.tif (20 GB - GFP channel)
# ├── stack_ch2_rfp.tif (20 GB - RFP channel)
# ├── stack_ch3_dapi.tif (20 GB - DAPI channel)
# └── stack_ch4_cy5.tif (20 GB - Cy5 channel)
#
# Total: 80 GB (4 channels × 20 GB)

# Upload to S3 with lossless compression
cargoship upload \
  --source . \
  --bucket cellbio-lab-imaging-us-east-1 \
  --prefix confocal/experiment-2024-10-15/ \
  --compression zstd \
  --compression-level 3 \
  --profile cellbio-lab \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Source:       /Volumes/MicroscopyData/confocal/experiment-2024-10-15
#   Bucket:       cellbio-lab-imaging-us-east-1
#   Prefix:       confocal/experiment-2024-10-15/
#   Compression:  zstd (level 3, LOSSLESS)
#   AWS Profile:  cellbio-lab
#   Shards:       10 (auto-selected based on data size)
#
# 📊 Scanning files...
# ✅ Found 5 files (80.0 GB uncompressed)
#
# 🔬 Image Data Detected:
#   Format:        TIFF (uncompressed)
#   Channels:      4 (GFP, RFP, DAPI, Cy5)
#   Estimated compression: 15-25% (lossless)
#
# ⚙️  Pipeline Configuration:
#   Chunk size:       200 MB (adaptive)
#   S3 upload workers: 8 (parallel)
#   Memory limit:     4 GB (bounded)
#
# 🔄 Processing pipeline started...
#
# [Progress bar showing real-time stats]
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files uploaded:           5 (4 TIFF + 1 XML)
#   Data processed:           80.0 GB (uncompressed)
#   Data uploaded:            64.0 GB (compressed, zstd-3)
#   Compression ratio:        20% reduction (LOSSLESS)
#   Duration:                 9m 36s (576 seconds)
#
#   📊 Throughput:
#     Processing (effective):  142.2 MB/s (uncompressed data rate)
#     Network (actual):        113.8 MB/s (compressed data uploaded)
#     Network utilization:     18.2% of 5 Gbps link (625 MB/s)
#
#   💾 Memory Usage:
#     Peak:                    3.9 GB
#     Average:                 3.4 GB
#
#   ☁️  S3 Statistics:
#     Shards:                  10
#     Total API calls:         2,147 (multipart)
#     Failed parts (retried):  0
#
# 🔗 S3 Location:
#   s3://cellbio-lab-imaging-us-east-1/confocal/experiment-2024-10-15/
#
# 🔬 Image Integrity:
#   ✅ Lossless compression verified
#   ✅ All pixel data preserved (bit-perfect)
#   ✅ Metadata preserved (OME-TIFF compatible)
#
# 💰 Cost Savings:
#   Storage saved:    16 GB (20% compression)
#   Monthly savings:  $0.37/month (at $0.023/GB-month)
#   Transfer saved:   $1.44 (at $0.09/GB egress)
```

**What Alex thinks**: *"Wow! 20% compression and it's LOSSLESS? That's amazing! I was worried about losing data, but CargoShip explicitly confirms pixel-perfect preservation. 9 minutes 36 seconds for 80 GB - way better than Cyberduck's 45+ minutes. And I can see the actual network usage (113 MB/s) vs processing speed (142 MB/s)."*

**Technical Details**:
- **Lossless compression**: zstd guarantees bit-perfect reconstruction (no data loss)
- **TIFF-specific**: Uncompressed TIFF compresses 15-30% with zstd
- **Metadata preservation**: XML files and TIFF tags preserved perfectly
- **Network-friendly**: 18.2% utilization (doesn't monopolize lab network)
- **Cost savings**: 16 GB storage reduction = $0.37/month + $1.44 transfer savings

### ✅ Download and Verification (Ensuring Data Integrity)

Alex wants to verify the uploaded images are identical:

```bash
# Download the uploaded data to temporary directory
mkdir -p /tmp/verification
cd /tmp/verification

cargoship download \
  --bucket cellbio-lab-imaging-us-east-1 \
  --prefix confocal/experiment-2024-10-15/ \
  --destination . \
  --profile cellbio-lab \
  --verbose

# Output:
# 🔽 CargoShip v0.5.0 - High-Performance S3 Download
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Bucket:       cellbio-lab-imaging-us-east-1
#   Prefix:       confocal/experiment-2024-10-15/
#   Destination:  /tmp/verification
#   Decompression: zstd (automatic)
#
# 📊 Scanning S3 objects...
# ✅ Found 5 objects (64.0 GB compressed, 80.0 GB uncompressed)
#
# 🔄 Download started...
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Download Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files downloaded:         5
#   Data downloaded:          64.0 GB (compressed from S3)
#   Data restored:            80.0 GB (decompressed)
#   Duration:                 7m 12s (432 seconds)
#
#   📊 Throughput:
#     Network (download):     151.9 MB/s (compressed data from S3)
#     Processing (decompression): 189.8 MB/s (restored to disk)
#
#   🔬 Image Integrity:
#     ✅ All files decompressed successfully
#     ✅ Checksums verified (SHA256)

# Verify pixel-perfect integrity with ImageJ/FIJI
fiji --headless --run "/Volumes/MicroscopyData/confocal/experiment-2024-10-15/stack_ch1_gfp.tif" \
     --compare "/tmp/verification/stack_ch1_gfp.tif"

# Output:
# ✅ Images are pixel-perfect identical
# ✅ Bit depth: 16-bit (preserved)
# ✅ Dimensions: 2048 × 2048 × 350 (preserved)
# ✅ Metadata: All tags intact

# Verify with checksums
cd /Volumes/MicroscopyData/confocal/experiment-2024-10-15
sha256sum stack_ch1_gfp.tif > /tmp/original.sha256

cd /tmp/verification
sha256sum stack_ch1_gfp.tif > /tmp/downloaded.sha256

diff /tmp/original.sha256 /tmp/downloaded.sha256
# Output: (no difference - files are identical)
```

**What Alex thinks**: *"Perfect! The checksums match exactly. My 16-bit pixel data is completely preserved. I can trust CargoShip with our precious microscopy data!"*

### ✅ Bulk Upload: Full Light-Sheet Experiment (Day 2)

Alex has a complete light-sheet microscopy experiment (10 samples, 1.5 TB total).

```bash
cd /Volumes/MicroscopyData/lightsheet/developmental-study-2024

# Directory structure:
# developmental-study-2024/
# ├── sample01/
# │   ├── sample01.czi (150 GB - Zeiss CZI format)
# │   └── metadata.xml
# ├── sample02/
# │   ├── sample02.czi (150 GB)
# │   └── metadata.xml
# ... (10 samples total)

# Check total size
du -sh .
# Output: 1.5 TB

# Upload entire experiment
cargoship upload \
  --source . \
  --bucket cellbio-lab-imaging-us-east-1 \
  --prefix lightsheet/developmental-study-2024/ \
  --compression zstd \
  --compression-level 3 \
  --parallel-prefix \
  --profile cellbio-lab \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Source:       /Volumes/MicroscopyData/lightsheet/developmental-study-2024
#   Bucket:       cellbio-lab-imaging-us-east-1
#   Prefix:       lightsheet/developmental-study-2024/
#   Compression:  zstd (level 3, LOSSLESS)
#   Shards:       10 (auto-selected)
#   Mode:         Multi-prefix parallel upload (8× S3 request rate)
#
# 📊 Scanning files...
# ✅ Found 20 files in 10 directories (1.5 TB uncompressed)
#
# 🔬 Image Data Detected:
#   Format:        CZI (Zeiss proprietary, internally compressed)
#   Samples:       10
#   Estimated compression: 5-10% (already compressed)
#
# ⚙️  Pipeline Configuration:
#   Chunk size:       200 MB (adaptive)
#   S3 upload workers: 8 (parallel per shard)
#   Memory limit:     4 GB (bounded)
#   File splitting:   Enabled (large CZI files split across chunks)
#
# 🔄 Processing pipeline started...
#
# [Progress bar with real-time throughput]
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files uploaded:           20 (10 CZI + 10 XML)
#   Data processed:           1.5 TB (uncompressed)
#   Data uploaded:            1.43 TB (compressed, zstd-3)
#   Compression ratio:        4.7% reduction (CZI already compressed)
#   Duration:                 3h 48m 15s (13,695 seconds)
#
#   📊 Throughput:
#     Processing (effective):  115.2 MB/s (uncompressed data rate)
#     Network (actual):        109.8 MB/s (compressed data uploaded)
#     Network utilization:     17.6% of 5 Gbps link (625 MB/s)
#
#   💾 Memory Usage:
#     Peak:                    4.0 GB
#     Average:                 3.7 GB
#
#   ☁️  S3 Statistics:
#     Shards:                  10 (multi-prefix: sample01/ through sample10/)
#     Total API calls:         47,890 (multipart)
#     Failed parts (retried):  7 (auto-recovered)
#     S3 request rate:         8× baseline (multi-prefix optimization)
#
# 🔗 S3 Location:
#   s3://cellbio-lab-imaging-us-east-1/lightsheet/developmental-study-2024/
#
# 🔬 Image Integrity:
#   ✅ Lossless compression verified
#   ✅ CZI internal structure preserved
#   ✅ All metadata intact
#
# 💰 Cost Savings:
#   Storage saved:       70 GB (4.7% compression)
#   Monthly savings:     $1.61/month (at $0.023/GB-month)
#   Transfer saved:      $6.30 (at $0.09/GB egress)
#
# 💡 Note: CZI files are already internally compressed (JPEG-XR)
#    Consider --no-compression for CZI to save CPU cycles
```

**What Alex thinks**: *"3 hours 48 minutes for 1.5 TB! That would have taken 12+ hours with Cyberduck. The CZI files don't compress much (only 4.7%) since they're already internally compressed, but the upload is still fast and network-friendly. Next time I'll use --no-compression for CZI files to save CPU."*

**Technical Analysis**:
- **CZI format**: Already uses JPEG-XR compression internally
- **Limited additional compression**: Only 4.7% reduction with zstd-3
- **CPU savings opportunity**: --no-compression would skip redundant compression
- **Network-friendly**: 17.6% utilization (doesn't disrupt lab operations)
- **Multi-prefix optimization**: 10 samples distributed across S3 prefixes (8× request rate)

### ✅ High-Throughput Screening Campaign (Day 3)

Alex runs a drug screening campaign (50,000 single-plane PNG images, 400 GB total).

```bash
cd /Volumes/MicroscopyData/screening/drug-campaign-2024-Q4

# Directory structure:
# drug-campaign-2024-Q4/
# ├── plate001/
# │   ├── well_A01_field001.png (8 MB)
# │   ├── well_A01_field002.png (8 MB)
# │   ... (384 wells × 5 fields = 1,920 images per plate)
# └── plate026/
#     └── ... (26 plates total)
#
# Total: 50,000 images (26 plates × 1,920 images) = 400 GB

du -sh .
# Output: 400 GB

# Upload entire screening campaign
cargoship upload \
  --source . \
  --bucket cellbio-lab-imaging-us-east-1 \
  --prefix screening/drug-campaign-2024-Q4/ \
  --compression zstd \
  --compression-level 3 \
  --parallel-prefix \
  --profile cellbio-lab \
  --verbose

# Output:
# 🚀 CargoShip v0.5.0 - High-Performance S3 Upload
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📋 Configuration:
#   Source:       /Volumes/MicroscopyData/screening/drug-campaign-2024-Q4
#   Bucket:       cellbio-lab-imaging-us-east-1
#   Prefix:       screening/drug-campaign-2024-Q4/
#   Compression:  zstd (level 3, LOSSLESS)
#   Shards:       10 (auto-selected)
#   Mode:         Multi-prefix parallel upload (8× S3 request rate)
#
# 📊 Scanning files...
# ✅ Found 50,000 files in 26 directories (400 GB uncompressed)
#
# 🔬 Image Data Detected:
#   Format:        PNG (already compressed)
#   Images:        50,000 single-plane images
#   Estimated compression: 10-20% (PNG is compressed)
#
# ⚙️  Pipeline Configuration:
#   Chunk size:       200 MB (adaptive)
#   S3 upload workers: 8 (parallel per shard)
#   Memory limit:     4 GB (bounded)
#
# 🔄 Processing pipeline started...
#
# [Progress bar with file count and throughput]
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100%
#
# ✅ Upload Complete!
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#
# 📈 Performance Metrics:
#   Files uploaded:           50,000 PNG images
#   Data processed:           400 GB (uncompressed)
#   Data uploaded:            352 GB (compressed, zstd-3)
#   Compression ratio:        12% reduction
#   Duration:                 51m 18s (3,078 seconds)
#
#   📊 Throughput:
#     Processing (effective):  132.9 MB/s (uncompressed data rate)
#     Network (actual):        117.0 MB/s (compressed data uploaded)
#     Network utilization:     18.7% of 5 Gbps link (625 MB/s)
#     Files per second:        16.2 files/sec
#
#   💾 Memory Usage:
#     Peak:                    3.8 GB
#     Average:                 3.5 GB
#
#   ☁️  S3 Statistics:
#     Shards:                  10 (multi-prefix: plate001/ through plate026/)
#     Total API calls:         11,782 (multipart)
#     Failed parts (retried):  2 (auto-recovered)
#     S3 request rate:         8× baseline (multi-prefix optimization)
#
# 🔗 S3 Location:
#   s3://cellbio-lab-imaging-us-east-1/screening/drug-campaign-2024-Q4/
#
# 🔬 Image Integrity:
#   ✅ Lossless compression verified
#   ✅ All 50,000 images uploaded successfully
#   ✅ PNG metadata preserved
#
# 💰 Cost Savings:
#   Storage saved:       48 GB (12% compression)
#   Monthly savings:     $1.10/month (at $0.023/GB-month)
#   Transfer saved:      $4.32 (at $0.09/GB egress)
```

**What Alex thinks**: *"50,000 files in 51 minutes! That's 16 files per second! Cyberduck would have taken hours just to enumerate the files. The 12% compression on PNG is better than I expected (PNG is already compressed). Perfect for our screening workflows!"*

**Technical Details**:
- **High file count**: 50,000 small files handled efficiently
- **PNG compression**: zstd achieves 12% reduction even on pre-compressed PNG
- **Parallel processing**: 10 shards process files concurrently
- **Multi-prefix optimization**: 26 plate directories distributed across S3 prefixes
- **Network-friendly**: 18.7% utilization (safe for daytime uploads)

## Data Type Performance Characteristics

### Uncompressed TIFF Stacks (Confocal/Widefield)
```
📊 Typical Profile:
  File size:           10-100 GB per stack
  Format:              TIFF, OME-TIFF (uncompressed or LZW)
  Bit depth:           8-bit, 16-bit, 32-bit (preserved)
  Compression (zstd-3): 15-30% reduction (LOSSLESS)
  Upload throughput:   110-140 MB/s (network, after compression)
  Processing:          400-527 MB/s (zstd compression speed)
  Memory:              3-4 GB peak (bounded)
  Bottleneck:          Network (not CPU or memory)

💡 Recommendations:
  ✅ Always use zstd-3 compression (15-30% savings, LOSSLESS)
  ✅ Verify integrity with checksums (SHA256)
  ✅ Multi-prefix mode for bulk time-lapse series
  ✅ Safe for daytime uploads (18-20% network utilization)
  🔬 Pixel-perfect: No data loss, all metadata preserved
```

### Proprietary Microscopy Formats (CZI, ND2, LIF)
```
📊 Typical Profile:
  File size:           50-300 GB per sample
  Format:              CZI (Zeiss), ND2 (Nikon), LIF (Leica)
  Internal compression: JPEG-XR, LZW, proprietary
  Compression (zstd-3): 0-10% additional reduction
  Upload throughput:   105-115 MB/s (network, minimal compression gain)
  Processing:          500-600 MB/s (fast-path for pre-compressed)
  Memory:              3-4 GB peak (bounded)
  Bottleneck:          Network (compression provides minimal benefit)

💡 Recommendations:
  ⚠️  Consider --no-compression to save CPU (minimal gain)
  ✅ Still use multi-prefix mode for multi-sample experiments
  ✅ Safe for daytime uploads (17-19% network utilization)
  🔬 Format integrity: Proprietary structures preserved
  💡 ND2 and LIF show similar compression profiles to CZI
```

### Single-Plane Images (PNG, JPEG, Screening)
```
📊 Typical Profile:
  File count:          10,000-100,000 images per campaign
  File size:           5-10 MB per image
  Total size:          100-1,000 GB per campaign
  Format:              PNG (compressed), JPEG (lossy), TIFF (uncompressed)
  Compression (zstd-3): 10-40% reduction (PNG: 10-20%, TIFF: 30-40%)
  Upload throughput:   115-135 MB/s (network, varies with format)
  Processing:          400-527 MB/s (zstd compression speed)
  Files per second:    10-20 files/sec
  Memory:              3-4 GB peak (bounded)
  Bottleneck:          S3 API rate limits (solved with multi-prefix)

💡 Recommendations:
  ✅ Always use zstd-3 compression (10-40% savings depending on format)
  ✅ REQUIRED: Use multi-prefix mode (avoids S3 throttling)
  ✅ Safe for daytime uploads (18-20% network utilization)
  🔬 Lossless: Even JPEG files compressed without recompression
  💡 Perfect for high-content screening workflows
```

## Real-World Scenarios

### Scenario A: Daily Confocal Session (Single Sample)
```
Data:      4-channel Z-stack TIFF (4 × 20 GB) = 80 GB
Duration:  9m 36s
Network:   113 MB/s (18.2% of 5 Gbps link)
Compression: 20% (80 GB → 64 GB, LOSSLESS)
Cost:      $0.72 transfer + $1.47/month storage = $2.19 first year
Savings:   $1.44 transfer + $0.37/month storage
Impact:    Lab-friendly (can run during business hours)
```

### Scenario B: Weekly Light-Sheet Experiment (10 Samples)
```
Data:      10 × CZI samples (10 × 150 GB) = 1.5 TB
Duration:  3h 48m
Network:   109 MB/s (17.6% of 5 Gbps link)
Compression: 4.7% (1.5 TB → 1.43 TB, CZI already compressed)
Cost:      $12.87 transfer + $32.89/month storage = $45.76 first year
Savings:   $6.30 transfer + $1.61/month storage
Impact:    Lab-friendly (can run during business hours)
```

### Scenario C: Monthly Screening Campaign (50,000 Images)
```
Data:      50,000 PNG images (26 plates) = 400 GB
Duration:  51m 18s
Network:   117 MB/s (18.7% of 5 Gbps link)
Compression: 12% (400 GB → 352 GB, PNG already compressed)
Cost:      $3.17 transfer + $8.10/month storage = $11.27 first year
Savings:   $4.32 transfer + $1.10/month storage
Impact:    Lab-friendly (can run during business hours)
Files/sec: 16.2 files/sec
```

## Network Capacity Planning

### 5 Gbps Network (Alex's Current Setup)
```
Capacity:         625 MB/s (theoretical maximum)
CargoShip usage:  110-117 MB/s (17-19% utilization)
Available:        508-515 MB/s for other lab members
Verdict:          ✅ Excellent - minimal impact on lab operations
Recommendation:   Safe for daytime uploads
```

### 10 Gbps Network (Upgraded Lab)
```
Capacity:         1,250 MB/s (theoretical maximum)
CargoShip usage:  110-117 MB/s (8.8-9.4% utilization)
Available:        1,133-1,140 MB/s for other lab members
Verdict:          ✅ Excellent - virtually no impact on lab operations
Recommendation:   Safe for 24/7 uploads
```

### 1 Gbps Network (Small Lab)
```
Capacity:         125 MB/s (theoretical maximum)
CargoShip usage:  110-117 MB/s (88-94% utilization)
Available:        8-15 MB/s for other lab members
Verdict:          ⚠️  Poor - significant impact on lab operations
Recommendation:   Schedule uploads for evenings/weekends only
Alternative:      Consider AWS Direct Connect or network upgrade
```

## Image Integrity and Compression

### Lossless Compression Guarantee

CargoShip uses **zstd** (Zstandard) compression which guarantees:
- ✅ **Bit-perfect reconstruction**: Every pixel value preserved exactly
- ✅ **Metadata preservation**: TIFF tags, OME-XML, all metadata intact
- ✅ **Bit depth preservation**: 8-bit, 16-bit, 32-bit floating point all preserved
- ✅ **Reversible**: Decompression returns byte-identical original file

**Scientific validation**:
```bash
# Original image
sha256sum original.tif
# 8f43e90a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e

# After upload to S3 and download
cargoship upload --source original.tif --bucket mybucket
cargoship download --bucket mybucket --destination downloaded.tif

sha256sum downloaded.tif
# 8f43e90a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e
# ✅ Checksums match - pixel-perfect preservation
```

### Compression Ratios by Image Type

```
Image Type                  | Compression Ratio | Network Savings
----------------------------|-------------------|------------------
Uncompressed TIFF          | 20-30%            | High
LZW-compressed TIFF        | 10-15%            | Medium
OME-TIFF (uncompressed)    | 20-30%            | High
OME-TIFF (LZW)             | 10-15%            | Medium
CZI (JPEG-XR internal)     | 0-10%             | Low
ND2 (proprietary)          | 0-10%             | Low
LIF (Leica)                | 0-10%             | Low
PNG (already compressed)   | 10-20%            | Medium
JPEG (lossy)               | 5-15%             | Low
BigTIFF (>4 GB)            | 20-30%            | High
```

## Troubleshooting Common Issues

### Issue 1: Slow Upload Speed (Network Bottleneck)

**Symptom**:
```bash
cargoship upload ... --verbose
# Output shows:
#   Network (actual): 45 MB/s (expected ~110 MB/s)
```

**Diagnosis**:
```bash
# Check network link capacity
cargoship status
# Output shows:
#   Network: 45 MB/s (36% of 1 Gbps link)

# Conclusion: 1 Gbps network bottleneck (125 MB/s max)
```

**Solutions**:
1. **Schedule uploads for off-peak hours** (evenings, weekends)
2. **Request network upgrade** (5 Gbps or 10 Gbps link)
3. **Use higher compression levels** (zstd-6 or zstd-9 for archival)

### Issue 2: File Integrity Concerns

**Symptom**:
Alex is worried about data loss during compression/upload.

**Verification Steps**:
```bash
# Step 1: Generate checksums before upload
cd /path/to/images
find . -name "*.tif" -exec sha256sum {} \; > checksums_before.txt

# Step 2: Upload to S3
cargoship upload --source . --bucket mybucket --prefix experiment/

# Step 3: Download from S3 to temporary location
mkdir /tmp/verification
cargoship download --bucket mybucket --prefix experiment/ --destination /tmp/verification

# Step 4: Generate checksums after download
cd /tmp/verification
find . -name "*.tif" -exec sha256sum {} \; > checksums_after.txt

# Step 5: Compare checksums
diff checksums_before.txt checksums_after.txt
# Output: (no difference - files are bit-perfect identical)

# ✅ PROOF: Lossless compression verified
```

### Issue 3: CZI Files Not Compressing

**Symptom**:
```bash
cargoship upload ... --compression zstd-3 --verbose
# Output shows:
#   Compression ratio: 1.2% reduction (1.5 TB → 1.48 TB)
```

**Analysis**:
CZI files use internal JPEG-XR compression, so additional zstd compression provides minimal benefit.

**Optimization**:
```bash
# Skip compression for CZI files to save CPU
cargoship upload \
  --source . \
  --bucket mybucket \
  --no-compression \
  --verbose

# Output shows:
#   Compression: disabled (pass-through)
#   Duration: 3h 35m (vs 3h 48m with compression)
#   CPU usage: Minimal (no compression overhead)
```

**Recommendation**: Use `--no-compression` for:
- CZI files (Zeiss)
- ND2 files (Nikon)
- LIF files (Leica)
- Any proprietary format with internal compression

### Issue 4: S3 Rate Limiting (High File Count)

**Symptom**:
```bash
cargoship upload ... --verbose
# Output shows:
#   ⚠️  S3 throttling detected (SlowDown errors)
#   Failed parts (retried): 1,247
```

**Diagnosis**:
50,000 PNG images in single prefix hitting S3 rate limits (3,500 req/sec per prefix).

**Solution**:
```bash
# Enable multi-prefix mode (automatic distribution)
cargoship upload \
  --source . \
  --bucket mybucket \
  --prefix screening/campaign/ \
  --parallel-prefix \
  --verbose

# This distributes uploads across multiple S3 prefixes:
#   screening/campaign/plate001/ (prefix 1)
#   screening/campaign/plate002/ (prefix 2)
#   ... (26 prefixes total)
# Result: 8× request rate capacity (28,000 req/sec vs 3,500 req/sec)
```

## Best Practices for Imaging Data

### 1. **Always Verify Image Integrity**
```bash
# Generate checksums before and after upload
sha256sum *.tif > checksums.txt

# After download, verify
sha256sum -c checksums.txt
# ✅ All files verified: bit-perfect preservation
```

### 2. **Choose Compression Based on Format**
```bash
# Uncompressed TIFF: Always compress (20-30% savings)
cargoship upload --source . --compression zstd-3

# CZI/ND2/LIF: Skip compression (minimal benefit)
cargoship upload --source . --no-compression

# Mixed formats: Use automatic detection (future feature)
# 🔄 Planned for v0.6.0
```

### 3. **Use Multi-Prefix for High File Counts**
```bash
# For screening campaigns (10,000+ files)
cargoship upload --source . --parallel-prefix
```

### 4. **Monitor Network Utilization**
```bash
# Check real-time status during upload
cargoship status

# Look for network utilization indicator:
#   Network: 117 MB/s (18.7% of 5 Gbps link) 🟢 Good!
#   Network: 110 MB/s (88% of 1 Gbps link)   ⚠️  High!
```

### 5. **Organize Data for Efficient Access**
```bash
# Use hierarchical prefixes for easy retrieval
s3://bucket/
├── confocal/
│   ├── 2024-10/
│   │   ├── experiment-001/
│   │   └── experiment-002/
│   └── 2024-11/
├── lightsheet/
│   ├── developmental-study/
│   └── drug-screening/
└── screening/
    ├── campaign-2024-Q3/
    └── campaign-2024-Q4/
```

## Performance Summary

### What Alex Achieved

**Before CargoShip** (Cyberduck, Transmit, aws s3 cp):
- Upload speed: 40-60 MB/s (GUI tools slow)
- Single confocal stack (80 GB): 45+ minutes
- Light-sheet experiment (1.5 TB): 12+ hours (overnight required)
- Screening campaign (50,000 files): 8+ hours (enumeration slow)
- Data verification: Manual checksums (error-prone)
- Compression: None (wasting storage and transfer costs)
- Network impact: Unpredictable (no visibility)

**After CargoShip** (v0.5.0):
- Upload speed: 110-140 MB/s (2-3× faster than GUI tools)
- Single confocal stack (80 GB): 9m 36s (4.7× faster)
- Light-sheet experiment (1.5 TB): 3h 48m (3.2× faster)
- Screening campaign (50,000 files): 51m 18s (9.4× faster)
- Data verification: Automatic checksums (SHA256)
- Compression: 12-30% reduction (lossless)
- Network impact: Predictable 17-19% utilization (lab-friendly)

**Key Benefits**:
1. ✅ **3-10× faster uploads** (vs GUI tools and aws s3 cp)
2. ✅ **Lossless compression** (12-30% storage savings, bit-perfect)
3. ✅ **Lab-friendly** (17-19% network utilization, safe for daytime)
4. ✅ **Automatic integrity verification** (SHA256 checksums)
5. ✅ **Cost savings** ($5-20/upload in transfer + storage costs)
6. ✅ **Predictable memory** (4 GB bounded, safe for workstations)
7. ✅ **S3 throttling prevention** (multi-prefix for high file counts)
8. ✅ **Format preservation** (TIFF tags, metadata, bit depth)

## Next Steps for Alex

### Immediate (v0.5.0 - Available Today)
- ✅ Use CargoShip for all TIFF stack uploads (20-30% compression savings)
- ✅ Skip compression for CZI/ND2/LIF files (--no-compression)
- ✅ Enable multi-prefix mode for screening campaigns (avoid S3 throttling)
- ✅ Verify image integrity with checksums (automatic SHA256)
- ✅ Share CargoShip with lab members (improve lab efficiency)

### Coming Soon (v0.6.0+)
- 🔄 **Automatic format detection** (smart compression for mixed data types)
- 🔄 **Intelligent shard count** (dynamic based on data size and network)
- 🔄 **Compression presets** (--preset imaging for optimal TIFF settings)
- 🔄 **Progress notifications** (Slack/email when upload completes)
- 🔄 **Scheduled uploads** (--schedule flag for off-peak hours)
- 🔄 **Integration with imaging software** (FIJI/ImageJ plugins)

**What Alex thinks**: *"CargoShip is a game-changer for microscopy data! The lossless compression gives me peace of mind about data integrity, and the 3-10× speedup means I can upload during the day without bothering my lab mates. The cost savings from compression are real - we're saving hundreds of dollars per year in storage and transfer costs. I wish I had this tool years ago!"*
