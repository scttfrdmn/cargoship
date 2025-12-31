#!/bin/bash
# Download AWS Open Data Registry Datasets - Issue #166
# Downloads real-world datasets for realistic benchmarking
#
# Usage:
#   ./scripts/download-aws-open-data.sh [OPTIONS]
#
# Options:
#   --dataset DATASET       Dataset to download: sentinel, genomes, goes16, goes17, nasa, spacenet, all
#   --sample-size SIZE      Sample size: small (1GB), medium (10GB), large (100GB) (default: small)
#   --output-dir DIR        Output directory (default: /tmp/aws-open-data)
#   --region REGION         AWS region (default: us-west-2)
#   --profile PROFILE       AWS profile (default: aws)
#   --help                  Show this help message
#
# Examples:
#   # Download small Sentinel-2 sample
#   ./scripts/download-aws-open-data.sh --dataset sentinel --sample-size small
#
#   # Download multiple datasets
#   ./scripts/download-aws-open-data.sh --dataset sentinel,genomes --sample-size medium
#
#   # Custom output directory
#   ./scripts/download-aws-open-data.sh --dataset goes16 --output-dir /Volumes/External/datasets
#
# Note: All datasets use free AWS egress within the same region (no-sign-on buckets)
#
# Reference: https://registry.opendata.aws

set -e

# Parse command-line arguments
DATASET="sentinel"
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
# DATASET 1: Sentinel-2 - Satellite Imagery (JPEG2000)
# Bucket: s3://sentinel-s2-l2a (no-sign-request)
# https://registry.opendata.aws/sentinel-2/
#
download_sentinel() {
    log_section "Downloading Sentinel-2 Satellite Imagery"
    local dataset_dir="$OUTPUT_DIR/sentinel-2"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample Sentinel-2 scenes (JPEG2000 format)..."
    log_info "Source: s3://sentinel-s2-l2a (public bucket, no credentials required)"

    # Download from recent tiles (Europe - tile 32TPS, Paris area)
    # Path format: tiles/{UTM_ZONE}/{LATITUDE_BAND}/{GRID_SQUARE}/
    local tile_path="tiles/32/T/PS/2024/12"

    log_info "Sampling files from tile: $tile_path"

    # List and download sample files (limit to prevent excessive downloads)
    aws s3 ls "s3://sentinel-s2-l2a/$tile_path/" --no-sign-request --region eu-central-1 --recursive | \
        grep -E '\.(jp2|TIF)$' | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                local basename=$(basename "$file")
                log_info "  Downloading: $basename"
                aws s3 cp "s3://sentinel-s2-l2a/$file" \
                    "$dataset_dir/$basename" \
                    --no-sign-request \
                    --region eu-central-1 \
                    --quiet || log_warn "Failed to download $basename"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" 2>/dev/null | cut -f1)
    local file_count=$(find "$dataset_dir" -type f 2>/dev/null | wc -l | tr -d ' ')
    log_success "Sentinel-2 dataset complete: $file_count files, $actual_size"
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
# DATASET 3: NOAA GOES-16 - Weather Satellite Imagery
# Bucket: s3://noaa-goes16 (no-sign-request)
# https://registry.opendata.aws/noaa-goes/
#
download_goes16() {
    log_section "Downloading NOAA GOES-16 Weather Satellite Data"
    local dataset_dir="$OUTPUT_DIR/noaa-goes16"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample GOES-16 satellite imagery..."
    log_info "Source: s3://noaa-goes16 (public bucket, no credentials required)"

    # Download from ABI-L1b-RadC (CONUS radiance data)
    local year=$(date +%Y)
    local day=$(date +%j)  # Day of year
    local hour=$(date +%H)
    local data_path="ABI-L1b-RadC/$year/$day/$hour"

    log_info "Sampling files from $data_path..."

    aws s3 ls "s3://noaa-goes16/$data_path/" --no-sign-request --region us-east-1 | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                log_info "  Downloading: $file"
                aws s3 cp "s3://noaa-goes16/$data_path/$file" \
                    "$dataset_dir/$file" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $file"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" 2>/dev/null | cut -f1)
    local file_count=$(find "$dataset_dir" -type f 2>/dev/null | wc -l | tr -d ' ')
    log_success "NOAA GOES-16 dataset complete: $file_count files, $actual_size"
}

#
# DATASET 3B: NOAA GOES-17 - Weather Satellite Imagery
# Bucket: s3://noaa-goes17 (no-sign-request)
# https://registry.opendata.aws/noaa-goes/
#
download_goes17() {
    log_section "Downloading NOAA GOES-17 Weather Satellite Data"
    local dataset_dir="$OUTPUT_DIR/noaa-goes17"
    mkdir -p "$dataset_dir"

    log_info "Downloading sample GOES-17 satellite imagery..."
    log_info "Source: s3://noaa-goes17 (public bucket, no credentials required)"

    # Download from ABI-L1b-RadC (CONUS radiance data)
    local year=$(date +%Y)
    local day=$(date +%j)  # Day of year
    local hour=$(date +%H)
    local data_path="ABI-L1b-RadC/$year/$day/$hour"

    log_info "Sampling files from $data_path..."

    aws s3 ls "s3://noaa-goes17/$data_path/" --no-sign-request --region us-east-1 | \
        head -n $FILE_LIMIT | \
        awk '{print $4}' | \
        while read -r file; do
            if [ -n "$file" ]; then
                log_info "  Downloading: $file"
                aws s3 cp "s3://noaa-goes17/$data_path/$file" \
                    "$dataset_dir/$file" \
                    --no-sign-request \
                    --region us-east-1 \
                    --quiet || log_warn "Failed to download $file"
            fi
        done

    local actual_size=$(du -sh "$dataset_dir" 2>/dev/null | cut -f1)
    local file_count=$(find "$dataset_dir" -type f 2>/dev/null | wc -l | tr -d ' ')
    log_success "NOAA GOES-17 dataset complete: $file_count files, $actual_size"
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
# DATASET 5: SpaceNet - Satellite Imagery (GeoTIFF)
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
        sentinel)
            download_sentinel
            ;;
        genomes)
            download_genomes
            ;;
        goes16)
            download_goes16
            ;;
        goes17)
            download_goes17
            ;;
        nasa)
            download_nasa
            ;;
        spacenet)
            download_spacenet
            ;;
        all)
            download_sentinel
            download_genomes
            download_goes16
            download_goes17
            download_nasa
            download_spacenet
            ;;
        *)
            log_error "Unknown dataset: $ds"
            log_info "Valid datasets: sentinel, genomes, goes16, goes17, nasa, spacenet, all"
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
