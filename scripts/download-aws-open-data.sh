#!/bin/bash
# Download AWS Open Data Registry Datasets - Issue #166
# Downloads real-world datasets for realistic benchmarking
#
# Usage:
#   ./scripts/download-aws-open-data.sh [OPTIONS]
#
# Options:
#   --dataset DATASET       Dataset to download: landsat, genomes, noaa, nasa, crawl, brain, spacenet, all
#   --sample-size SIZE      Sample size: small (1GB), medium (10GB), large (100GB) (default: small)
#   --output-dir DIR        Output directory (default: /tmp/aws-open-data)
#   --region REGION         AWS region (default: us-west-2)
#   --profile PROFILE       AWS profile (default: aws)
#   --help                  Show this help message
#
# Examples:
#   # Download small Landsat sample
#   ./scripts/download-aws-open-data.sh --dataset landsat --sample-size small
#
#   # Download multiple datasets
#   ./scripts/download-aws-open-data.sh --dataset landsat,genomes --sample-size medium
#
#   # Custom output directory
#   ./scripts/download-aws-open-data.sh --dataset noaa --output-dir /Volumes/External/datasets
#
# Note: All datasets use free AWS egress within the same region (no-sign-on buckets)
#
# Reference: https://registry.opendata.aws

set -e

# Parse command-line arguments
DATASET="landsat"
SAMPLE_SIZE="small"
OUTPUT_DIR="/tmp/aws-open-data"
AWS_REGION="us-west-2"
AWS_PROFILE="aws"

while [[ $# -gt 0 ]]; do
    case $1 in
        --dataset)
            DATASET="$2"
            shift 2
            ;;
        --sample-size)
            SAMPLE_SIZE="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --region)
            AWS_REGION="$2"
            shift 2
            ;;
        --profile)
            AWS_PROFILE="$2"
            shift 2
            ;;
        --help|-h)
            sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# //' | sed 's/^#$//' | grep -v '^$'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate sample size
case $SAMPLE_SIZE in
    small|medium|large) ;;
    *)
        echo "Error: Invalid sample size '$SAMPLE_SIZE'"
        echo "Valid sizes: small, medium, large"
        exit 1
        ;;
esac

# Size configuration (approximate file count)
case $SAMPLE_SIZE in
    small)
        FILE_LIMIT=100
        SIZE_DESC="1GB"
        ;;
    medium)
        FILE_LIMIT=1000
        SIZE_DESC="10GB"
        ;;
    large)
        FILE_LIMIT=10000
        SIZE_DESC="100GB"
        ;;
esac

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
}

# Check AWS CLI
if ! command -v aws > /dev/null 2>&1; then
    log_error "AWS CLI not found. Please install: brew install awscli"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

log_section "AWS Open Data Registry Download - Issue #166"
log_info "Dataset: $DATASET"
log_info "Sample size: $SAMPLE_SIZE (~$SIZE_DESC, $FILE_LIMIT files)"
log_info "Output: $OUTPUT_DIR"
log_info "Region: $AWS_REGION"

#
# DATASET 1: Landsat 8 - Satellite Imagery (GeoTIFF)
# Bucket: s3://landsat-pds (no-sign-request)
# https://registry.opendata.aws/landsat-8/
#
download_landsat() {
    log_section "Downloading Landsat 8 Satellite Imagery"
    local dataset_dir="$OUTPUT_DIR/landsat-8"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample Landsat 8 scenes (GeoTIFF format)..."
    log_info "Source: s3://landsat-pds (public bucket, no credentials required)"

    # Download from a specific scene (WRS path/row)
    # Example: LC08_L1TP_042033_20170616_20170629_01_T1 (San Francisco area)
    local scene_path="c1/L8/042/033/LC08_L1TP_042033_20170616_20170629_01_T1"

    # Download up to FILE_LIMIT files from this scene
    log_info "Downloading scene: $scene_path"

    aws s3 ls "s3://landsat-pds/$scene_path/" --no-sign-request --region us-west-2 | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                log_info "  Downloading: $file"
                aws s3 cp "s3://landsat-pds/$scene_path/$file" \
                    "$dataset_dir/$file" \
                    --no-sign-request \
                    --region us-west-2 \
                    --quiet || log_warn "Failed to download $file"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "Landsat 8 dataset complete: $file_count files, $actual_size"
}

#
# DATASET 2: 1000 Genomes - Genomics Data (BAM/VCF)
# Bucket: s3://1000genomes (no-sign-request)
# https://registry.opendata.aws/1000-genomes/
#
download_genomes() {
    log_section "Downloading 1000 Genomes Genomics Data"
    local dataset_dir="$OUTPUT_DIR/1000-genomes"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample genomics data (BAM/VCF format)..."
    log_info "Source: s3://1000genomes (public bucket, no credentials required)"

    # Download from phase3 alignment data
    local data_path="phase3/data"

    log_info "Sampling files from $data_path..."

    # List and download sample files
    aws s3 ls "s3://1000genomes/$data_path/" --no-sign-request --region us-east-1 --recursive | \
        grep -E '\.(bam|vcf|vcf\.gz)$' | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                local basename=$(basename "$file")
                log_info "  Downloading: $basename"
                aws s3 cp "s3://1000genomes/$file" \
                    "$dataset_dir/$basename" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $basename"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "1000 Genomes dataset complete: $file_count files, $actual_size"
}

#
# DATASET 3: NOAA NEXRAD - Weather Radar Data (binary)
# Bucket: s3://noaa-nexrad-level2 (no-sign-request)
# https://registry.opendata.aws/noaa-nexrad/
#
download_noaa() {
    log_section "Downloading NOAA NEXRAD Weather Radar Data"
    local dataset_dir="$OUTPUT_DIR/noaa-nexrad"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample weather radar data (Level 2 format)..."
    log_info "Source: s3://noaa-nexrad-level2 (public bucket, no credentials required)"

    # Download recent data from a specific station (e.g., KDLH - Duluth, MN)
    local year=$(date +%Y)
    local month=$(date +%m)
    local day=$(date +%d)
    local station="KDLH"
    local data_path="$year/$month/$day/$station"

    log_info "Downloading station $station data from $data_path..."

    aws s3 ls "s3://noaa-nexrad-level2/$data_path/" --no-sign-request --region us-east-1 | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                log_info "  Downloading: $file"
                aws s3 cp "s3://noaa-nexrad-level2/$data_path/$file" \
                    "$dataset_dir/$file" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $file"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "NOAA NEXRAD dataset complete: $file_count files, $actual_size"
}

#
# DATASET 4: NASA NEX - Climate Model Data (NetCDF)
# Bucket: s3://nasanex (no-sign-request)
# https://registry.opendata.aws/nasanex/
#
download_nasa() {
    log_section "Downloading NASA NEX Climate Model Data"
    local dataset_dir="$OUTPUT_DIR/nasa-nex"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample climate model data (NetCDF format)..."
    log_info "Source: s3://nasanex (public bucket, no credentials required)"

    # Download from NEX-GDDP dataset
    local data_path="NEX-GDDP/BCSD/rcp45/day/atmos/tasmax/r1i1p1/v1.0"

    log_info "Sampling files from $data_path..."

    aws s3 ls "s3://nasanex/$data_path/" --no-sign-request --region us-west-2 --recursive | \
        grep '\.nc$' | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                local basename=$(basename "$file")
                log_info "  Downloading: $basename"
                aws s3 cp "s3://nasanex/$file" \
                    "$dataset_dir/$basename" \
                    --no-sign-request \
                    --region us-west-2 \
                    --quiet || log_warn "Failed to download $basename"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "NASA NEX dataset complete: $file_count files, $actual_size"
}

#
# DATASET 5: Common Crawl - Web Archive Data (WARC)
# Bucket: s3://commoncrawl (no-sign-request)
# https://registry.opendata.aws/commoncrawl/
#
download_crawl() {
    log_section "Downloading Common Crawl Web Archive Data"
    local dataset_dir="$OUTPUT_DIR/common-crawl"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample web archive data (WARC format)..."
    log_info "Source: s3://commoncrawl (public bucket, no credentials required)"

    # Download from recent crawl
    local crawl_id="CC-MAIN-2024-10"
    local data_path="crawl-data/$crawl_id/segments"

    log_info "Sampling WARC files from $crawl_id..."

    aws s3 ls "s3://commoncrawl/$data_path/" --no-sign-request --region us-east-1 --recursive | \
        grep '\.warc\.gz$' | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                local basename=$(basename "$file")
                log_info "  Downloading: $basename"
                aws s3 cp "s3://commoncrawl/$file" \
                    "$dataset_dir/$basename" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $basename"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "Common Crawl dataset complete: $file_count files, $actual_size"
}

#
# DATASET 6: Allen Brain Atlas - Neuroscience Data (NWB/HDF5)
# Bucket: s3://allen-brain-observatory (no-sign-request)
# https://registry.opendata.aws/allen-brain-observatory/
#
download_brain() {
    log_section "Downloading Allen Brain Observatory Data"
    local dataset_dir="$OUTPUT_DIR/allen-brain"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample neuroscience data (NWB format)..."
    log_info "Source: s3://allen-brain-observatory (public bucket, no credentials required)"

    log_info "Sampling NWB files from visual-coding-2p..."

    aws s3 ls "s3://allen-brain-observatory/visual-coding-2p/" --no-sign-request --region us-west-2 --recursive | \
        grep '\.nwb$' | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                local basename=$(basename "$file")
                log_info "  Downloading: $basename"
                aws s3 cp "s3://allen-brain-observatory/$file" \
                    "$dataset_dir/$basename" \
                    --no-sign-request \
                    --region us-west-2 \
                    --quiet || log_warn "Failed to download $basename"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "Allen Brain dataset complete: $file_count files, $actual_size"
}

#
# DATASET 7: SpaceNet - Satellite Imagery (GeoJSON)
# Bucket: s3://spacenet-dataset (no-sign-request)
# https://registry.opendata.aws/spacenet/
#
download_spacenet() {
    log_section "Downloading SpaceNet Satellite Imagery"
    local dataset_dir="$OUTPUT_DIR/spacenet"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample satellite imagery (GeoTIFF/GeoJSON format)..."
    log_info "Source: s3://spacenet-dataset (public bucket, no credentials required)"

    # Download from SpaceNet 1 (Rio de Janeiro)
    local data_path="spacenet/SN1_buildings/tarballs"

    log_info "Sampling files from SpaceNet 1..."

    aws s3 ls "s3://spacenet-dataset/$data_path/" --no-sign-request --region us-east-1 | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                log_info "  Downloading: $file"
                aws s3 cp "s3://spacenet-dataset/$data_path/$file" \
                    "$dataset_dir/$file" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $file"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" | cut -f1)
    local file_count=$(find "$dataset_dir" -type f | wc -l | tr -d ' ')
    log_success "SpaceNet dataset complete: $file_count files, $actual_size"
}

# Download requested datasets
START_TIME=$(date +%s)

# Parse comma-separated dataset list
IFS=',' read -ra DATASETS <<< "$DATASET"

for ds in "${DATASETS[@]}"; do
    ds=$(echo "$ds" | tr -d ' ')  # Trim whitespace
    case $ds in
        landsat)
            download_landsat
            ;;
        genomes)
            download_genomes
            ;;
        noaa)
            download_noaa
            ;;
        nasa)
            download_nasa
            ;;
        crawl)
            download_crawl
            ;;
        brain)
            download_brain
            ;;
        spacenet)
            download_spacenet
            ;;
        all)
            download_landsat
            download_genomes
            download_noaa
            download_nasa
            download_crawl
            download_brain
            download_spacenet
            ;;
        *)
            log_error "Unknown dataset: $ds"
            log_info "Valid datasets: landsat, genomes, noaa, nasa, crawl, brain, spacenet, all"
            exit 1
            ;;
    esac
done

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

# Generate summary
log_section "Download Complete"
log_info "Total time: ${DURATION}s"
log_info "Output directory: $OUTPUT_DIR"

# Show disk usage
echo ""
log_info "Disk usage by dataset:"
du -sh "$OUTPUT_DIR"/* 2>/dev/null | while read size path; do
    echo "  $size  $(basename "$path")"
done

echo ""
TOTAL_SIZE=$(du -sh "$OUTPUT_DIR" | cut -f1)
TOTAL_FILES=$(find "$OUTPUT_DIR" -type f 2>/dev/null | wc -l | tr -d ' ')
log_success "Downloaded $TOTAL_FILES files, $TOTAL_SIZE total"
log_success "Ready for benchmarking with CargoShip!"

# Generate metadata file
cat > "$OUTPUT_DIR/dataset-metadata.json" <<EOF
{
  "download_timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "datasets": "$(echo "${DATASETS[@]}" | tr ' ' ',')",
  "sample_size": "$SAMPLE_SIZE",
  "total_files": $TOTAL_FILES,
  "total_size": "$TOTAL_SIZE",
  "duration_seconds": $DURATION,
  "aws_region": "$AWS_REGION",
  "source": "AWS Open Data Registry",
  "issue": "#166"
}
EOF

log_success "Metadata saved: $OUTPUT_DIR/dataset-metadata.json"
