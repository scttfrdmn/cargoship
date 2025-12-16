#!/bin/bash
# Feature Matrix Benchmark - Granular Performance Analysis
# Issue #34: Shows performance impact of each CargoShip feature
#
# Tests CargoShip with different feature combinations:
# 1. Baseline: Minimal features (archive + upload only)
# 2. Compression: +zstd compression
# 3. Magika: +AI file type detection
# 4. Deduplication: +content-aware dedup
# 5. Adaptive: +adaptive chunk sizing
# 6. All Features: Default configuration
# 7. Recommended: Compression + Adaptive (no Magika overhead)
#
# Usage:
#   ./scripts/feature-matrix-benchmark.sh [OPTIONS]
#
# Options:
#   --profile PROFILE       AWS profile to use (default: aws)
#   --region REGION         AWS region to use (default: us-west-2)
#   --test-data-dir DIR     Directory for test data (default: /tmp/benchmark-data/scenario1-small-files)
#   --results-dir DIR       Directory for results (default: /tmp/feature-matrix-TIMESTAMP)
#   --iterations N          Number of iterations per config (default: 3)
#   --help                  Show this help message

set -e

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --profile)
            CLI_AWS_PROFILE="$2"
            shift 2
            ;;
        --region)
            CLI_AWS_REGION="$2"
            shift 2
            ;;
        --test-data-dir)
            CLI_TEST_DATA_DIR="$2"
            shift 2
            ;;
        --results-dir)
            CLI_RESULTS_DIR="$2"
            shift 2
            ;;
        --iterations)
            CLI_ITERATIONS="$2"
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

# Configuration
AWS_PROFILE="${CLI_AWS_PROFILE:-${AWS_PROFILE:-aws}}"
AWS_REGION="${CLI_AWS_REGION:-${AWS_REGION:-us-west-2}}"
TEST_DATA_DIR="${CLI_TEST_DATA_DIR:-${TEST_DATA_DIR:-/tmp/benchmark-data/scenario1-small-files}}"
RESULTS_DIR="${CLI_RESULTS_DIR:-${RESULTS_DIR:-/tmp/feature-matrix-$(date +%Y%m%d-%H%M%S)}}"
ITERATIONS="${CLI_ITERATIONS:-${ITERATIONS:-3}}"
BENCHMARK_BUCKET="cargoship-feature-matrix-$(date +%s)"

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

# Create results directory
mkdir -p "$RESULTS_DIR"

log_section "CargoShip Feature Matrix Benchmark"
log_info "Testing feature combinations to show performance impact"
log_info "Results: $RESULTS_DIR"
log_info "Iterations per config: $ITERATIONS"

# Verify test data exists
if [ ! -d "$TEST_DATA_DIR" ]; then
    log_error "Test data not found: $TEST_DATA_DIR"
    log_info "Run: ./scripts/competitive-benchmark.sh (Scenario 1 only) to generate data"
    exit 1
fi

FILE_COUNT=$(find "$TEST_DATA_DIR" -type f | wc -l | tr -d ' ')
TOTAL_SIZE_MB=$(du -sm "$TEST_DATA_DIR" | cut -f1)

log_info "Test data: $FILE_COUNT files, ${TOTAL_SIZE_MB}MB"

# Build cargoship if needed
if [ ! -f "./cargoship" ]; then
    log_info "Building cargoship..."
    go build -o ./cargoship ./cmd/cargoship
fi

# Check for gdate (macOS)
if command -v gdate > /dev/null 2>&1; then
    DATE_CMD="gdate"
else
    DATE_CMD="date"
fi

# Create S3 bucket
log_info "Creating benchmark bucket: $BENCHMARK_BUCKET"
AWS_PROFILE=$AWS_PROFILE aws s3 mb "s3://$BENCHMARK_BUCKET" --region "$AWS_REGION" 2>&1 || log_warn "Bucket may already exist"

# Initialize results CSV
echo "config,iteration,duration_ms,throughput_mbps,features" > "$RESULTS_DIR/results.csv"

# Temporarily move user's config file aside (CargoShip loads ~/.cargoship.yaml first)
USER_CONFIG="$HOME/.cargoship.yaml"
USER_CONFIG_BACKUP=""
if [ -f "$USER_CONFIG" ]; then
    USER_CONFIG_BACKUP="$USER_CONFIG.benchmark-backup"
    log_info "Temporarily moving user config: $USER_CONFIG → $USER_CONFIG_BACKUP"
    mv "$USER_CONFIG" "$USER_CONFIG_BACKUP"
fi

# Restore user config on exit
restore_user_config() {
    if [ -n "$USER_CONFIG_BACKUP" ] && [ -f "$USER_CONFIG_BACKUP" ]; then
        log_info "Restoring user config: $USER_CONFIG_BACKUP → $USER_CONFIG"
        mv "$USER_CONFIG_BACKUP" "$USER_CONFIG"
    fi
    # Clean up temporary config
    rm -f ./.cargoship.yaml
}

trap restore_user_config EXIT INT TERM

# Feature configuration functions (bash 3.2 compatible)
create_feature_config() {
    local config=$1
    local config_file="$RESULTS_DIR/${config}-config.yaml"

    case "$config" in
        baseline)
            # Minimal: Just archive and upload, no compression, no dedup
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: false

staging:
  enable_compression: false
  enable_deduplication: false
EOF
            ;;
        compression)
            # Add compression only
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: false

staging:
  enable_compression: true
  compression_algorithm: zstd
  compression_level: 3
  enable_deduplication: false
EOF
            ;;
        magika)
            # Add Magika file type detection
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: false

staging:
  enable_compression: false
  enable_deduplication: false

magika:
  enabled: true
  batch_size: 100
  enable_cache: true
EOF
            ;;
        dedup)
            # Add deduplication only
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: false

staging:
  enable_compression: false
  enable_deduplication: true
EOF
            ;;
        adaptive)
            # Add adaptive sizing only
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: true

staging:
  enable_compression: false
  enable_deduplication: false
EOF
            ;;
        recommended)
            # Compression + Adaptive (recommended config)
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: true

staging:
  enable_compression: true
  compression_algorithm: zstd
  compression_level: 3
  enable_deduplication: false
EOF
            ;;
        all-features)
            # Everything enabled (default CargoShip behavior)
            cat > "$config_file" <<EOF
upload:
  bucket: $BENCHMARK_BUCKET
  region: $AWS_REGION

chunking:
  enable_adaptive_sizing: true

staging:
  enable_compression: true
  compression_algorithm: zstd
  compression_level: 3
  enable_deduplication: true

magika:
  enabled: true
  batch_size: 100
  enable_cache: true
EOF
            ;;
    esac

    echo "$config_file"
}

get_feature_description() {
    case "$1" in
        baseline)
            echo "Archive + Upload only (minimal)"
            ;;
        compression)
            echo "+ Zstd Compression"
            ;;
        magika)
            echo "+ Magika AI File Detection"
            ;;
        dedup)
            echo "+ Content-Aware Deduplication"
            ;;
        adaptive)
            echo "+ Adaptive Chunk Sizing"
            ;;
        recommended)
            echo "Compression + Adaptive (recommended)"
            ;;
        all-features)
            echo "All Features Enabled (default)"
            ;;
    esac
}

# Run benchmarks for each configuration
for config in baseline compression magika dedup adaptive recommended all-features; do
    log_section "Config: $config"
    log_info "$(get_feature_description $config)"

    for iter in $(seq 1 $ITERATIONS); do
        PREFIX="feature-matrix/$config/iter-$iter"

        log_info "Iteration $iter/$ITERATIONS..."

        # Clean up prefix
        AWS_PROFILE=$AWS_PROFILE aws s3 rm "s3://$BENCHMARK_BUCKET/$PREFIX" --recursive --quiet 2>/dev/null || true

        # Create config file for this feature set
        CONFIG_FILE=$(create_feature_config $config)

        # Copy config to working directory (CargoShip looks for ./.cargoship.yaml)
        cp "$CONFIG_FILE" ./.cargoship.yaml

        # Run upload (bucket flag is required even with config file)
        START=$($DATE_CMD +%s%3N)

        AWS_PROFILE=$AWS_PROFILE ./cargoship create upload "$TEST_DATA_DIR" \
            --bucket "$BENCHMARK_BUCKET" \
            --prefix "$PREFIX" \
            --region "$AWS_REGION" \
            --quiet > "$RESULTS_DIR/${config}-iter${iter}.log" 2>&1

        END=$($DATE_CMD +%s%3N)

        # Calculate metrics
        DURATION=$((END - START))
        THROUGHPUT=$(echo "scale=2; ($TOTAL_SIZE_MB * 8 * 1000) / $DURATION" | bc)

        # Save to CSV
        echo "$config,$iter,$DURATION,$THROUGHPUT,$(get_feature_description $config)" >> "$RESULTS_DIR/results.csv"

        log_success "Completed in ${DURATION}ms (${THROUGHPUT} Mbps)"

        sleep 2
    done
done

# Generate analysis report
log_section "Generating Analysis Report"

cat > "$RESULTS_DIR/report.md" <<EOF
# CargoShip Feature Matrix Benchmark Report

**Date:** $(date)
**Test Data:** $FILE_COUNT files, ${TOTAL_SIZE_MB}MB
**Iterations:** $ITERATIONS per configuration
**Region:** $AWS_REGION

---

## Executive Summary

This benchmark shows the performance impact of each CargoShip feature, allowing users to make informed decisions about which features to enable based on their priorities.

## Feature Configurations Tested

| Config | Description | Features |
|--------|-------------|----------|
| **baseline** | Minimal | Archive + Upload only |
| **compression** | +Compression | + Zstd compression |
| **magika** | +AI Detection | + Magika file type detection |
| **dedup** | +Deduplication | + Content-aware deduplication |
| **adaptive** | +Adaptive Sizing | + Adaptive chunk sizing |
| **recommended** | Recommended | Compression + Adaptive sizing |
| **all-features** | Everything | All features enabled (default) |

---

## Performance Results

EOF

# Calculate averages for each config
for config in baseline compression magika dedup adaptive recommended all-features; do
    # Get all durations for this config
    DURATIONS=$(grep "^$config," "$RESULTS_DIR/results.csv" | cut -d',' -f3)

    if [ -n "$DURATIONS" ]; then
        # Calculate average duration
        AVG_DURATION=$(echo "$DURATIONS" | awk '{sum+=$1; count++} END {printf "%.0f", sum/count}')

        # Calculate average throughput
        AVG_THROUGHPUT=$(echo "scale=2; ($TOTAL_SIZE_MB * 8 * 1000) / $AVG_DURATION" | bc)

        # Get baseline for relative speed
        if [ "$config" = "baseline" ]; then
            BASELINE_DURATION=$AVG_DURATION
        fi

        # Calculate relative speed
        if [ -n "$BASELINE_DURATION" ] && [ "$BASELINE_DURATION" != "0" ]; then
            RELATIVE=$(echo "scale=2; $AVG_DURATION / $BASELINE_DURATION" | bc)
            RELATIVE_TEXT="${RELATIVE}x"
        else
            RELATIVE_TEXT="-"
        fi

        # Write to report
        cat >> "$RESULTS_DIR/report.md" <<EOF
### $config: ${FEATURE_DESCRIPTIONS[$config]}

- **Average Duration:** ${AVG_DURATION}ms
- **Average Throughput:** ${AVG_THROUGHPUT} Mbps
- **Relative to Baseline:** ${RELATIVE_TEXT}

EOF
    fi
done

cat >> "$RESULTS_DIR/report.md" <<EOF

---

## Feature Impact Analysis

This table shows the **overhead** of each feature relative to the baseline:

| Feature | Overhead | Throughput Impact | Value Proposition |
|---------|----------|-------------------|-------------------|
EOF

# Calculate overhead for each feature
for config in compression magika dedup adaptive; do
    DURATIONS=$(grep "^$config," "$RESULTS_DIR/results.csv" | cut -d',' -f3)
    if [ -n "$DURATIONS" ]; then
        AVG_DURATION=$(echo "$DURATIONS" | awk '{sum+=$1; count++} END {printf "%.0f", sum/count}')
        OVERHEAD=$((AVG_DURATION - BASELINE_DURATION))
        OVERHEAD_PCT=$(echo "scale=1; ($OVERHEAD / $BASELINE_DURATION) * 100" | bc)
        THROUGHPUT_IMPACT=$(echo "scale=2; ($BASELINE_DURATION / $AVG_DURATION) * 100 - 100" | bc)

        # Value proposition
        case $config in
            compression)
                VALUE="Reduces storage costs, faster transfers on slow networks"
                ;;
            magika)
                VALUE="Optimal compression per file type, better compression ratios"
                ;;
            dedup)
                VALUE="Eliminates duplicate data, massive savings on redundant datasets"
                ;;
            adaptive)
                VALUE="Optimal chunk sizes, better parallelism, improved throughput"
                ;;
        esac

        echo "| $config | +${OVERHEAD}ms (${OVERHEAD_PCT}%) | ${THROUGHPUT_IMPACT}% | $VALUE |" >> "$RESULTS_DIR/report.md"
    fi
done

cat >> "$RESULTS_DIR/report.md" <<EOF

---

## Recommendations

### For Maximum Performance
Use **baseline** or **compression** configuration:
- Fastest upload times
- Minimal overhead
- Good for time-sensitive uploads

### For Balanced Performance (Recommended)
Use **recommended** configuration:
- Compression + Adaptive sizing
- Good performance with useful features
- Best for most use cases

### For Maximum Cost Savings
Use **all-features** configuration:
- Deduplication saves on redundant data
- Compression reduces storage costs
- Magika optimizes compression per file type
- Best for large datasets with redundancy

### Feature Selection Guide

Choose features based on your priorities:

| Priority | Enable These Features |
|----------|-----------------------|
| **Speed** | None (baseline) or Compression only |
| **Balanced** | Compression + Adaptive (recommended) |
| **Cost** | All features (compression + dedup + magika) |
| **Mixed Content** | Compression + Magika (optimal per file type) |
| **Redundant Data** | Compression + Dedup (eliminate duplicates) |

---

## Raw Data

See \`results.csv\` for detailed per-iteration results.

**Test Configuration:**
- Platform: $(uname -sm)
- Test Data: $FILE_COUNT files, ${TOTAL_SIZE_MB}MB
- Iterations: $ITERATIONS per config
- Region: $AWS_REGION

---

**Generated by:** CargoShip Feature Matrix Benchmark (Issue #34)
**Repository:** https://github.com/scttfrdmn/cargoship
EOF

log_success "Report generated: $RESULTS_DIR/report.md"

# Display summary
log_section "Benchmark Complete!"

echo ""
log_info "Results saved to: $RESULTS_DIR"
log_info "  - results.csv: Raw data"
log_info "  - report.md: Analysis report"
echo ""

# Show quick summary
log_info "Quick Summary (Average Duration):"
for config in baseline compression magika dedup adaptive recommended all-features; do
    DURATIONS=$(grep "^$config," "$RESULTS_DIR/results.csv" | cut -d',' -f3)
    if [ -n "$DURATIONS" ]; then
        AVG=$(echo "$DURATIONS" | awk '{sum+=$1; count++} END {printf "%.0f", sum/count}')
        printf "  %-15s %6dms\n" "$config:" "$AVG"
    fi
done

echo ""
log_success "View full report: cat $RESULTS_DIR/report.md"

# Cleanup bucket
log_info "Cleaning up benchmark bucket..."
AWS_PROFILE=$AWS_PROFILE aws s3 rb "s3://$BENCHMARK_BUCKET" --force 2>&1 || log_warn "Cleanup may have failed"

log_success "Feature matrix benchmark complete!"
