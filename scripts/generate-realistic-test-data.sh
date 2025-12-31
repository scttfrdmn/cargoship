#!/bin/bash
# Generate Realistic Test Data - Issue #166
# Creates domain-specific file distributions for benchmarking
#
# Usage:
#   ./scripts/generate-realistic-test-data.sh [OPTIONS]
#
# Options:
#   --domain DOMAIN         Domain type: software, media, database, scientific, all (default: all)
#   --size SIZE             Dataset size: small (10GB), medium (100GB), large (500GB) (default: small)
#   --output-dir DIR        Output directory (default: /tmp/realistic-benchmark-data)
#   --help                  Show this help message
#
# Examples:
#   # Generate all domains, small size
#   ./scripts/generate-realistic-test-data.sh
#
#   # Generate media production dataset, medium size
#   ./scripts/generate-realistic-test-data.sh --domain media --size medium
#
#   # Custom output directory
#   ./scripts/generate-realistic-test-data.sh --output-dir /Volumes/External/test-data

set -e

# Parse command-line arguments
DOMAIN="all"
SIZE="small"
OUTPUT_DIR="/tmp/realistic-benchmark-data"

while [[ $# -gt 0 ]]; do
    case $1 in
        --domain)
            DOMAIN="$2"
            shift 2
            ;;
        --size)
            SIZE="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
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

# Validate domain
case $DOMAIN in
    software|media|database|scientific|all) ;;
    *)
        echo "Error: Invalid domain '$DOMAIN'"
        echo "Valid domains: software, media, database, scientific, all"
        exit 1
        ;;
esac

# Validate size
case $SIZE in
    small|medium|large) ;;
    *)
        echo "Error: Invalid size '$SIZE'"
        echo "Valid sizes: small, medium, large"
        exit 1
        ;;
esac

# Size configuration (in MB)
case $SIZE in
    small)
        TOTAL_SIZE_MB=10240  # 10GB
        FILE_COUNT=10000
        ;;
    medium)
        TOTAL_SIZE_MB=102400  # 100GB
        FILE_COUNT=100000
        ;;
    large)
        TOTAL_SIZE_MB=512000  # 500GB
        FILE_COUNT=1000000
        ;;
esac

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
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

log_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
}

# Create output directory
mkdir -p "$OUTPUT_DIR"

log_section "Realistic Test Data Generation - Issue #166"
log_info "Domain: $DOMAIN"
log_info "Size: $SIZE ($TOTAL_SIZE_MB MB, $FILE_COUNT files)"
log_info "Output: $OUTPUT_DIR"

#
# DOMAIN 1: Software Engineering
# 45% code, 25% build artifacts, 15% media, 10% docs, 5% configs
#
generate_software_engineering() {
    local domain_dir="$OUTPUT_DIR/software-engineering"
    local domain_size=$1  # MB
    local domain_files=$2

    log_section "Generating Software Engineering Dataset"
    mkdir -p "$domain_dir"

    # Calculate file counts
    local code_files=$((domain_files * 45 / 100))
    local build_files=$((domain_files * 25 / 100))
    local media_files=$((domain_files * 15 / 100))
    local doc_files=$((domain_files * 10 / 100))
    local config_files=$((domain_files * 5 / 100))

    log_info "Generating code files (45%, $code_files files)..."
    mkdir -p "$domain_dir/src"
    for i in $(seq 1 $code_files); do
        local ext=$(( RANDOM % 10 ))
        case $ext in
            0|1|2) file_ext="py" ;;   # Python (30%)
            3|4) file_ext="go" ;;      # Go (20%)
            5|6) file_ext="js" ;;      # JavaScript (20%)
            7) file_ext="rs" ;;        # Rust (10%)
            8) file_ext="java" ;;      # Java (10%)
            9) file_ext="cpp" ;;       # C++ (10%)
        esac

        local size_kb=$((RANDOM % 50 + 5))  # 5-55KB per code file
        {
            echo "// Generated code file for benchmarking - Issue #166"
            echo "// This file simulates real source code patterns"
            echo ""
            for j in $(seq 1 $((size_kb * 20))); do
                echo "function example_${i}_${j}() { return \"test data\"; }"
            done
        } | head -c $((size_kb * 1024)) > "$domain_dir/src/module_${i}.$file_ext"

        if [ $((i % 1000)) -eq 0 ]; then
            log_info "  Generated $i / $code_files code files..."
        fi
    done
    log_success "Code files generated"

    log_info "Generating build artifacts (25%, $build_files files)..."
    mkdir -p "$domain_dir/build"
    for i in $(seq 1 $build_files); do
        local artifact_type=$((RANDOM % 4))
        case $artifact_type in
            0) ext="jar"; size_mb=$((RANDOM % 50 + 10)) ;;   # JAR: 10-60MB
            1) ext="whl"; size_mb=$((RANDOM % 20 + 5)) ;;    # Python wheel: 5-25MB
            2) ext="so"; size_mb=$((RANDOM % 10 + 1)) ;;     # Shared lib: 1-11MB
            3) ext="a"; size_mb=$((RANDOM % 5 + 1)) ;;       # Static lib: 1-6MB
        esac

        dd if=/dev/urandom of="$domain_dir/build/artifact_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 500)) -eq 0 ]; then
            log_info "  Generated $i / $build_files build artifacts..."
        fi
    done
    log_success "Build artifacts generated"

    log_info "Generating media files (15%, $media_files files)..."
    mkdir -p "$domain_dir/assets"
    for i in $(seq 1 $media_files); do
        local media_type=$((RANDOM % 3))
        case $media_type in
            0) ext="png"; size_kb=$((RANDOM % 200 + 50)) ;;  # PNG: 50-250KB
            1) ext="svg"; size_kb=$((RANDOM % 50 + 10)) ;;   # SVG: 10-60KB
            2) ext="jpg"; size_kb=$((RANDOM % 500 + 100)) ;; # JPEG: 100-600KB
        esac

        dd if=/dev/urandom of="$domain_dir/assets/image_${i}.$ext" bs=1024 count=$size_kb 2>/dev/null

        if [ $((i % 500)) -eq 0 ]; then
            log_info "  Generated $i / $media_files media files..."
        fi
    done
    log_success "Media files generated"

    log_info "Generating documentation (10%, $doc_files files)..."
    mkdir -p "$domain_dir/docs"
    for i in $(seq 1 $doc_files); do
        local doc_type=$((RANDOM % 3))
        case $doc_type in
            0) ext="md"; size_kb=$((RANDOM % 100 + 10)) ;;   # Markdown: 10-110KB
            1) ext="rst"; size_kb=$((RANDOM % 80 + 10)) ;;   # reStructuredText: 10-90KB
            2) ext="txt"; size_kb=$((RANDOM % 50 + 5)) ;;    # Plain text: 5-55KB
        esac

        {
            echo "# Documentation File $i - Issue #166"
            echo ""
            for j in $(seq 1 $((size_kb / 2))); do
                echo "This is a documentation line with realistic text content for benchmarking purposes."
            done
        } | head -c $((size_kb * 1024)) > "$domain_dir/docs/doc_${i}.$ext"

        if [ $((i % 500)) -eq 0 ]; then
            log_info "  Generated $i / $doc_files documentation files..."
        fi
    done
    log_success "Documentation generated"

    log_info "Generating configuration files (5%, $config_files files)..."
    mkdir -p "$domain_dir/config"
    for i in $(seq 1 $config_files); do
        local config_type=$((RANDOM % 4))
        case $config_type in
            0) ext="yaml"; size_kb=$((RANDOM % 20 + 1)) ;;   # YAML: 1-21KB
            1) ext="json"; size_kb=$((RANDOM % 30 + 2)) ;;   # JSON: 2-32KB
            2) ext="toml"; size_kb=$((RANDOM % 15 + 1)) ;;   # TOML: 1-16KB
            3) ext="xml"; size_kb=$((RANDOM % 50 + 5)) ;;    # XML: 5-55KB
        esac

        {
            echo "# Configuration File $i - Issue #166"
            for j in $(seq 1 $((size_kb / 2))); do
                echo "config_key_$j: value_$j"
            done
        } | head -c $((size_kb * 1024)) > "$domain_dir/config/config_${i}.$ext"
    done
    log_success "Configuration files generated"

    local actual_size=$(du -sh "$domain_dir" | cut -f1)
    log_success "Software Engineering dataset complete: $actual_size"
}

#
# DOMAIN 2: Media Production
# 70% video, 15% audio, 10% graphics, 5% metadata
#
generate_media_production() {
    local domain_dir="$OUTPUT_DIR/media-production"
    local domain_size=$1  # MB
    local domain_files=$2

    log_section "Generating Media Production Dataset"
    mkdir -p "$domain_dir"

    # Calculate file counts
    local video_files=$((domain_files * 70 / 100))
    local audio_files=$((domain_files * 15 / 100))
    local graphics_files=$((domain_files * 10 / 100))
    local metadata_files=$((domain_files * 5 / 100))

    log_info "Generating video files (70%, $video_files files)..."
    mkdir -p "$domain_dir/video"
    for i in $(seq 1 $video_files); do
        local video_type=$((RANDOM % 3))
        case $video_type in
            0) ext="mp4"; size_mb=$((RANDOM % 500 + 100)) ;;  # MP4: 100-600MB
            1) ext="mov"; size_mb=$((RANDOM % 1000 + 200)) ;; # MOV: 200-1200MB
            2) ext="avi"; size_mb=$((RANDOM % 800 + 150)) ;;  # AVI: 150-950MB
        esac

        dd if=/dev/urandom of="$domain_dir/video/clip_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 100)) -eq 0 ]; then
            log_info "  Generated $i / $video_files video files..."
        fi
    done
    log_success "Video files generated"

    log_info "Generating audio files (15%, $audio_files files)..."
    mkdir -p "$domain_dir/audio"
    for i in $(seq 1 $audio_files); do
        local audio_type=$((RANDOM % 3))
        case $audio_type in
            0) ext="mp3"; size_mb=$((RANDOM % 10 + 3)) ;;    # MP3: 3-13MB
            1) ext="wav"; size_mb=$((RANDOM % 50 + 10)) ;;   # WAV: 10-60MB
            2) ext="flac"; size_mb=$((RANDOM % 30 + 10)) ;;  # FLAC: 10-40MB
        esac

        dd if=/dev/urandom of="$domain_dir/audio/track_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 200)) -eq 0 ]; then
            log_info "  Generated $i / $audio_files audio files..."
        fi
    done
    log_success "Audio files generated"

    log_info "Generating graphics files (10%, $graphics_files files)..."
    mkdir -p "$domain_dir/graphics"
    for i in $(seq 1 $graphics_files); do
        local graphics_type=$((RANDOM % 4))
        case $graphics_type in
            0) ext="psd"; size_mb=$((RANDOM % 100 + 20)) ;;  # Photoshop: 20-120MB
            1) ext="ai"; size_mb=$((RANDOM % 50 + 10)) ;;    # Illustrator: 10-60MB
            2) ext="tiff"; size_mb=$((RANDOM % 200 + 50)) ;; # TIFF: 50-250MB
            3) ext="png"; size_mb=$((RANDOM % 20 + 5)) ;;    # PNG: 5-25MB
        esac

        dd if=/dev/urandom of="$domain_dir/graphics/graphic_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 100)) -eq 0 ]; then
            log_info "  Generated $i / $graphics_files graphics files..."
        fi
    done
    log_success "Graphics files generated"

    log_info "Generating metadata files (5%, $metadata_files files)..."
    mkdir -p "$domain_dir/metadata"
    for i in $(seq 1 $metadata_files); do
        local size_kb=$((RANDOM % 50 + 5))  # 5-55KB
        {
            echo "{"
            echo "  \"file_id\": \"media_${i}\","
            echo "  \"title\": \"Production Asset $i\","
            echo "  \"created\": \"2025-12-30T00:00:00Z\","
            echo "  \"format\": \"video/mp4\","
            echo "  \"duration\": $((RANDOM % 600 + 60)),"
            echo "  \"resolution\": \"1920x1080\","
            echo "  \"bitrate\": $((RANDOM % 5000 + 1000))"
            echo "}"
        } > "$domain_dir/metadata/meta_${i}.json"
    done
    log_success "Metadata files generated"

    local actual_size=$(du -sh "$domain_dir" | cut -f1)
    log_success "Media Production dataset complete: $actual_size"
}

#
# DOMAIN 3: Database Backup
# 60% dumps, 30% WAL logs, 10% configs
#
generate_database_backup() {
    local domain_dir="$OUTPUT_DIR/database-backup"
    local domain_size=$1  # MB
    local domain_files=$2

    log_section "Generating Database Backup Dataset"
    mkdir -p "$domain_dir"

    # Calculate file counts
    local dump_files=$((domain_files * 60 / 100))
    local wal_files=$((domain_files * 30 / 100))
    local config_files=$((domain_files * 10 / 100))

    log_info "Generating database dumps (60%, $dump_files files)..."
    mkdir -p "$domain_dir/dumps"
    for i in $(seq 1 $dump_files); do
        local dump_type=$((RANDOM % 3))
        case $dump_type in
            0) ext="sql"; size_mb=$((RANDOM % 500 + 50)) ;;   # SQL dump: 50-550MB
            1) ext="dump"; size_mb=$((RANDOM % 1000 + 100)) ;; # Binary dump: 100-1100MB
            2) ext="sql.gz"; size_mb=$((RANDOM % 200 + 20)) ;; # Compressed: 20-220MB
        esac

        # Generate SQL-like content for dumps
        {
            echo "-- Database Dump $i - Issue #166"
            echo "-- Generated: $(date)"
            echo ""
            for j in $(seq 1 $((size_mb * 100))); do
                echo "INSERT INTO table_$((RANDOM % 100)) VALUES ($j, 'data_$j', '$(date +%s)', $((RANDOM % 1000)));"
            done
        } | head -c $((size_mb * 1024 * 1024)) | gzip > "$domain_dir/dumps/backup_${i}.$ext" 2>/dev/null

        if [ $((i % 100)) -eq 0 ]; then
            log_info "  Generated $i / $dump_files dump files..."
        fi
    done
    log_success "Database dumps generated"

    log_info "Generating WAL logs (30%, $wal_files files)..."
    mkdir -p "$domain_dir/wal"
    for i in $(seq 1 $wal_files); do
        local size_mb=$((RANDOM % 16 + 1))  # PostgreSQL WAL: 1-17MB (typically 16MB)
        dd if=/dev/urandom of="$domain_dir/wal/$(printf "%08X" $i)" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 500)) -eq 0 ]; then
            log_info "  Generated $i / $wal_files WAL files..."
        fi
    done
    log_success "WAL logs generated"

    log_info "Generating configuration files (10%, $config_files files)..."
    mkdir -p "$domain_dir/config"
    for i in $(seq 1 $config_files); do
        {
            echo "# PostgreSQL Configuration - Issue #166"
            echo "max_connections = $((RANDOM % 500 + 100))"
            echo "shared_buffers = $(( (RANDOM % 8 + 1) * 128 ))MB"
            echo "effective_cache_size = $(( (RANDOM % 16 + 4) * 256 ))MB"
            echo "maintenance_work_mem = $(( (RANDOM % 8 + 1) * 64 ))MB"
            echo "checkpoint_completion_target = 0.9"
            echo "wal_buffers = 16MB"
            echo "default_statistics_target = 100"
            echo "random_page_cost = 1.1"
            echo "effective_io_concurrency = 200"
            echo "work_mem = $(( (RANDOM % 32 + 4) ))MB"
        } > "$domain_dir/config/postgresql_${i}.conf"
    done
    log_success "Configuration files generated"

    local actual_size=$(du -sh "$domain_dir" | cut -f1)
    log_success "Database Backup dataset complete: $actual_size"
}

#
# DOMAIN 4: Scientific Computing
# 40% data files, 35% images, 15% results, 10% docs
#
generate_scientific_computing() {
    local domain_dir="$OUTPUT_DIR/scientific-computing"
    local domain_size=$1  # MB
    local domain_files=$2

    log_section "Generating Scientific Computing Dataset"
    mkdir -p "$domain_dir"

    # Calculate file counts
    local data_files=$((domain_files * 40 / 100))
    local image_files=$((domain_files * 35 / 100))
    local result_files=$((domain_files * 15 / 100))
    local doc_files=$((domain_files * 10 / 100))

    log_info "Generating data files (40%, $data_files files)..."
    mkdir -p "$domain_dir/data"
    for i in $(seq 1 $data_files); do
        local data_type=$((RANDOM % 4))
        case $data_type in
            0) ext="hdf5"; size_mb=$((RANDOM % 500 + 50)) ;;   # HDF5: 50-550MB
            1) ext="nc"; size_mb=$((RANDOM % 300 + 30)) ;;     # NetCDF: 30-330MB
            2) ext="csv"; size_mb=$((RANDOM % 100 + 10)) ;;    # CSV: 10-110MB
            3) ext="parquet"; size_mb=$((RANDOM % 200 + 20)) ;; # Parquet: 20-220MB
        esac

        dd if=/dev/urandom of="$domain_dir/data/dataset_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 100)) -eq 0 ]; then
            log_info "  Generated $i / $data_files data files..."
        fi
    done
    log_success "Data files generated"

    log_info "Generating image files (35%, $image_files files)..."
    mkdir -p "$domain_dir/images"
    for i in $(seq 1 $image_files); do
        local image_type=$((RANDOM % 3))
        case $image_type in
            0) ext="tiff"; size_mb=$((RANDOM % 100 + 20)) ;;  # TIFF: 20-120MB
            1) ext="fits"; size_mb=$((RANDOM % 200 + 50)) ;;  # FITS: 50-250MB
            2) ext="png"; size_mb=$((RANDOM % 50 + 10)) ;;    # PNG: 10-60MB
        esac

        dd if=/dev/urandom of="$domain_dir/images/image_${i}.$ext" bs=1M count=$size_mb 2>/dev/null

        if [ $((i % 200)) -eq 0 ]; then
            log_info "  Generated $i / $image_files image files..."
        fi
    done
    log_success "Image files generated"

    log_info "Generating results files (15%, $result_files files)..."
    mkdir -p "$domain_dir/results"
    for i in $(seq 1 $result_files); do
        local result_type=$((RANDOM % 3))
        case $result_type in
            0) ext="csv"; size_mb=$((RANDOM % 50 + 5)) ;;     # CSV: 5-55MB
            1) ext="json"; size_mb=$((RANDOM % 30 + 3)) ;;    # JSON: 3-33MB
            2) ext="txt"; size_mb=$((RANDOM % 20 + 2)) ;;     # Text: 2-22MB
        esac

        {
            echo "# Scientific Results $i - Issue #166"
            echo "# Experiment: Benchmark_$(date +%s)"
            echo ""
            for j in $(seq 1 $((size_mb * 1000))); do
                echo "$j,$((RANDOM % 10000)).$((RANDOM % 100)),$((RANDOM % 10000)).$((RANDOM % 100))"
            done
        } | head -c $((size_mb * 1024 * 1024)) > "$domain_dir/results/result_${i}.$ext"

        if [ $((i % 200)) -eq 0 ]; then
            log_info "  Generated $i / $result_files result files..."
        fi
    done
    log_success "Results files generated"

    log_info "Generating documentation (10%, $doc_files files)..."
    mkdir -p "$domain_dir/docs"
    for i in $(seq 1 $doc_files); do
        {
            echo "# Scientific Documentation $i - Issue #166"
            echo ""
            echo "## Abstract"
            echo "This document describes the experimental methodology and results."
            echo ""
            echo "## Methods"
            for j in $(seq 1 100); do
                echo "Step $j: Perform measurement and record data points."
            done
            echo ""
            echo "## Results"
            echo "See accompanying data files for detailed results."
        } > "$domain_dir/docs/paper_${i}.md"
    done
    log_success "Documentation generated"

    local actual_size=$(du -sh "$domain_dir" | cut -f1)
    log_success "Scientific Computing dataset complete: $actual_size"
}

# Generate requested domains
START_TIME=$(date +%s)

case $DOMAIN in
    software)
        generate_software_engineering $TOTAL_SIZE_MB $FILE_COUNT
        ;;
    media)
        generate_media_production $TOTAL_SIZE_MB $FILE_COUNT
        ;;
    database)
        generate_database_backup $TOTAL_SIZE_MB $FILE_COUNT
        ;;
    scientific)
        generate_scientific_computing $TOTAL_SIZE_MB $FILE_COUNT
        ;;
    all)
        # Divide evenly across all domains
        domain_size=$((TOTAL_SIZE_MB / 4))
        domain_files=$((FILE_COUNT / 4))

        generate_software_engineering $domain_size $domain_files
        generate_media_production $domain_size $domain_files
        generate_database_backup $domain_size $domain_files
        generate_scientific_computing $domain_size $domain_files
        ;;
esac

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

# Generate summary
log_section "Generation Complete"
log_info "Total time: ${DURATION}s"
log_info "Output directory: $OUTPUT_DIR"

# Show disk usage
echo ""
log_info "Disk usage by domain:"
du -sh "$OUTPUT_DIR"/* 2>/dev/null | while read size path; do
    echo "  $size  $(basename "$path")"
done

echo ""
TOTAL_SIZE=$(du -sh "$OUTPUT_DIR" | cut -f1)
TOTAL_FILES=$(find "$OUTPUT_DIR" -type f | wc -l | tr -d ' ')
log_success "Generated $TOTAL_FILES files, $TOTAL_SIZE total"
log_success "Ready for benchmarking with CargoShip!"
