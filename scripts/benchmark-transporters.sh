#!/bin/bash
# Transporter Performance Benchmark (Issue #161)
# Compares all 4 CargoShip transporter types: basic, staging, adaptive, optimized
# Tests with identical dataset to measure performance differences

set -e

# Configuration
AWS_PROFILE="${AWS_PROFILE:-aws}"
AWS_REGION="${AWS_REGION:-us-west-2}"
BENCHMARK_BUCKET="${CARGOSHIP_BENCHMARK_BUCKET:-cargoship-transporter-benchmark-$(date +%s)}"
RESULTS_DIR="/tmp/transporter-benchmark-results-$(date +%Y%m%d-%H%M%S)"
CARGOSHIP_BIN="${CARGOSHIP_BIN:-./cargoship}"

# Export AWS credentials for CargoShip to use
export AWS_ACCESS_KEY_ID=$(aws configure get aws_access_key_id --profile $AWS_PROFILE)
export AWS_SECRET_ACCESS_KEY=$(aws configure get aws_secret_access_key --profile $AWS_PROFILE)
export AWS_REGION="$AWS_REGION"

# Test data location - defaults to external NVMe for large-scale benchmarks
# Override with TEST_DATA_DIR environment variable
TEST_DATA_DIR="${TEST_DATA_DIR:-/Volumes/External HD/benchmark-data/mixed-workload}"

# Available test datasets on external NVMe:
# - competitive-test-10k: 39MB (10,000 small files)
# - small-files: 185MB
# - compressible-data: 1GB
# - deduplication-data: 1GB
# - mixed-workload: 6.2GB (DEFAULT - realistic mix of file sizes/types)
# - large-files: 56GB (for stress testing)

# Test data generation config (only used if TEST_DATA_DIR doesn't exist)
TEST_FILES="${TEST_FILES:-1000}"        # Number of files to generate
TEST_FILE_SIZE="${TEST_FILE_SIZE:-1048576}"  # 1MB per file = 1GB total if generated

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

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

log_metric() {
    echo -e "${CYAN}[METRIC]${NC} $1"
}

# Create results directory
mkdir -p "$RESULTS_DIR"

log_info "================================================================"
log_info "CargoShip Transporter Performance Benchmark (Issue #161)"
log_info "================================================================"
log_info "Results will be saved to: $RESULTS_DIR"
log_info "Test data directory: $TEST_DATA_DIR"
log_info ""

# Check if cargoship binary exists
if [ ! -f "$CARGOSHIP_BIN" ]; then
    log_error "CargoShip binary not found: $CARGOSHIP_BIN"
    log_info "Building CargoShip..."
    go build -o "$CARGOSHIP_BIN" ./cmd/cargoship
    log_success "Built: $CARGOSHIP_BIN"
fi

# Check test data
if [ ! -d "$TEST_DATA_DIR" ]; then
    log_info "Test data directory not found, creating: $TEST_DATA_DIR"
    log_info "Generating $TEST_FILES files @ $(numfmt --to=iec $TEST_FILE_SIZE) each"
    mkdir -p "$TEST_DATA_DIR"
    for i in $(seq 1 $TEST_FILES); do
        dd if=/dev/urandom of="$TEST_DATA_DIR/file-$(printf "%06d" $i).dat" bs=$TEST_FILE_SIZE count=1 2>/dev/null
        if [ $((i % 100)) -eq 0 ]; then
            echo -ne "\rCreated $i/$TEST_FILES files..."
        fi
    done
    echo ""
    log_success "Test data created: $(du -sh "$TEST_DATA_DIR" | cut -f1)"
else
    # Get file count and size of existing data
    file_count=$(find "$TEST_DATA_DIR" -type f | wc -l | tr -d ' ')
    total_size=$(du -sh "$TEST_DATA_DIR" | cut -f1)
    log_info "Using existing test data: $total_size ($file_count files)"
fi

# Create S3 bucket
log_info "Creating benchmark bucket: s3://$BENCHMARK_BUCKET"
AWS_PROFILE=$AWS_PROFILE aws s3 mb "s3://$BENCHMARK_BUCKET" --region "$AWS_REGION" 2>&1 || log_warn "Bucket may already exist"

# Function to cleanup S3 prefix
cleanup_s3_prefix() {
    local prefix=$1
    log_info "Cleaning up S3 prefix: $prefix"
    AWS_PROFILE=$AWS_PROFILE aws s3 rm "s3://$BENCHMARK_BUCKET/$prefix" --recursive --quiet 2>/dev/null || true
}

# Function to measure memory usage (macOS)
get_memory_usage() {
    local pid=$1
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: Use ps to get RSS in KB
        ps -o rss= -p $pid 2>/dev/null | awk '{print $1}'
    else
        # Linux: Use /proc
        awk '/VmRSS/ {print $2}' /proc/$pid/status 2>/dev/null
    fi
}

# Function to benchmark a transporter
benchmark_transporter() {
    local transporter=$1
    local optimization=${2:-"true"}
    local congestion_control=${3:-"auto"}

    log_info "========================================"
    log_info "Benchmarking: $transporter"
    log_info "========================================"

    local prefix="transporter-$transporter-$(date +%s)"
    local log_file="$RESULTS_DIR/$transporter.log"

    # Build command (use s3:// URL format with bucket/prefix)
    local cmd="$CARGOSHIP_BIN upload '$TEST_DATA_DIR' s3://'$BENCHMARK_BUCKET'/'$prefix' \
        --region '$AWS_REGION' \
        --transporter '$transporter' \
        --optimization=$optimization \
        --congestion-control '$congestion_control' \
        --quiet"

    log_info "Command: $cmd"

    # Start timing (macOS-compatible)
    local start_time=$(python3 -c 'import time; print(int(time.time() * 1000))')
    local start_mem=$(ps aux | grep -E 'cargoship|[c]argoship' | awk '{sum+=$6} END {print sum}')

    # Run upload in background to capture PID for memory monitoring
    eval "$cmd" > "$log_file" 2>&1 &
    local pid=$!

    # Monitor memory usage
    local peak_mem=0
    while kill -0 $pid 2>/dev/null; do
        local current_mem=$(get_memory_usage $pid)
        if [ ! -z "$current_mem" ] && [ "$current_mem" -gt "$peak_mem" ]; then
            peak_mem=$current_mem
        fi
        sleep 0.5
    done

    # Wait for completion and get exit code
    wait $pid
    local exit_code=$?

    # End timing (macOS-compatible)
    local end_time=$(python3 -c 'import time; print(int(time.time() * 1000))')
    local duration=$((end_time - start_time))

    # Calculate metrics (get actual data size)
    local total_bytes=$(du -sb "$TEST_DATA_DIR" 2>/dev/null | cut -f1)
    if [ -z "$total_bytes" ]; then
        total_bytes=$((TEST_FILES * TEST_FILE_SIZE))
    fi
    local duration_sec=$(echo "scale=3; $duration / 1000" | bc)
    local throughput_mbps=$(echo "scale=2; ($total_bytes * 8) / ($duration_sec * 1000000)" | bc)
    local throughput_mbs=$(echo "scale=2; $total_bytes / ($duration_sec * 1048576)" | bc)
    local peak_mem_mb=$(echo "scale=2; $peak_mem / 1024" | bc)

    # Check if upload succeeded
    if [ $exit_code -eq 0 ]; then
        log_success "$transporter completed successfully"

        # Log metrics
        log_metric "Duration:    ${duration}ms (${duration_sec}s)"
        log_metric "Throughput:  ${throughput_mbs} MB/s (${throughput_mbps} Mbps)"
        log_metric "Peak Memory: ${peak_mem_mb} MB"

        # Save to CSV
        echo "$transporter,$duration,$duration_sec,$throughput_mbs,$throughput_mbps,$peak_mem_mb,success" >> "$RESULTS_DIR/results.csv"

        # Cleanup S3 data
        cleanup_s3_prefix "$prefix"
    else
        log_error "$transporter failed with exit code $exit_code"
        log_error "Check log: $log_file"

        # Save failure to CSV
        echo "$transporter,$duration,$duration_sec,0,0,$peak_mem_mb,failed" >> "$RESULTS_DIR/results.csv"
    fi

    echo ""
    sleep 2
}

# Initialize results CSV
echo "transporter,duration_ms,duration_sec,throughput_mbs,throughput_mbps,peak_memory_mb,status" > "$RESULTS_DIR/results.csv"

# Run benchmarks for all 4 transporter types
log_info "Starting benchmarks..."
echo ""

# 1. Basic transporter (no optimization)
benchmark_transporter "basic" "false" "none"

# 2. Staging transporter (default, with optimization)
benchmark_transporter "staging" "true" "auto"

# 3. Adaptive transporter (with optimization)
benchmark_transporter "adaptive" "true" "bbr"

# 4. Optimized transporter (with optimization) - FIXED (Issue #162)
benchmark_transporter "optimized" "true" "cubic"

# Generate summary report
log_info "========================================"
log_info "Benchmark Results Summary"
log_info "========================================"

# Read results and generate report
{
    # Get actual test data info
    report_file_count=$(find "$TEST_DATA_DIR" -type f 2>/dev/null | wc -l | tr -d ' ')
    report_total_size=$(du -sh "$TEST_DATA_DIR" 2>/dev/null | cut -f1)

    echo ""
    echo "# CargoShip Transporter Performance Benchmark"
    echo "**Date**: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "**Test Data**: $report_total_size ($report_file_count files)"
    echo "**Location**: $TEST_DATA_DIR"
    echo "**Region**: $AWS_REGION"
    echo ""
    echo "## Results"
    echo ""
    echo "| Transporter | Duration (s) | Throughput (MB/s) | Throughput (Mbps) | Peak Memory (MB) | Status |"
    echo "|-------------|--------------|-------------------|-------------------|------------------|--------|"

    tail -n +2 "$RESULTS_DIR/results.csv" | while IFS=',' read -r transporter duration_ms duration_sec throughput_mbs throughput_mbps peak_mem status; do
        echo "| $transporter | $duration_sec | $throughput_mbs | $throughput_mbps | $peak_mem | $status |"
    done

    echo ""
    echo "## Analysis"
    echo ""

    # Find fastest transporter
    local fastest=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k3 -n | head -1 | cut -d',' -f1)
    local fastest_time=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k3 -n | head -1 | cut -d',' -f3)

    # Find highest throughput
    local highest_throughput=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k4 -n -r | head -1 | cut -d',' -f1)
    local highest_throughput_value=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k4 -n -r | head -1 | cut -d',' -f4)

    # Find lowest memory
    local lowest_mem=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k6 -n | head -1 | cut -d',' -f1)
    local lowest_mem_value=$(tail -n +2 "$RESULTS_DIR/results.csv" | grep success | sort -t',' -k6 -n | head -1 | cut -d',' -f6)

    echo "- **Fastest**: $fastest (${fastest_time}s)"
    echo "- **Highest Throughput**: $highest_throughput (${highest_throughput_value} MB/s)"
    echo "- **Lowest Memory**: $lowest_mem (${lowest_mem_value} MB)"
    echo ""
    echo "## Recommendations"
    echo ""
    echo "- **basic**: Simple, predictable performance. Use when optimization overhead not needed."
    echo "- **staging**: Default choice. Predictive staging with content-aware chunking."
    echo "- **adaptive**: Real-time network adaptation. Best for variable network conditions."
    echo "- **optimized**: BBR/CUBIC congestion control. Best for high-latency or congested networks."
    echo ""

} > "$RESULTS_DIR/REPORT.md"

# Display report
cat "$RESULTS_DIR/REPORT.md"

log_success "Benchmark complete!"
log_info "Full results: $RESULTS_DIR/results.csv"
log_info "Report: $RESULTS_DIR/REPORT.md"
log_info "Logs: $RESULTS_DIR/*.log"

# Cleanup bucket (optional - comment out to keep data)
log_info "Cleaning up benchmark bucket..."
AWS_PROFILE=$AWS_PROFILE aws s3 rb "s3://$BENCHMARK_BUCKET" --force 2>&1 || log_warn "Bucket cleanup failed"

log_success "All done!"
