#!/bin/bash
# Competitive Benchmark Script - Sequential Execution
# Compares CargoShip against s5cmd, mc, and aws-cli
# IMPORTANT: Runs ONE tool at a time to avoid resource contention

set -e

# Configuration
AWS_PROFILE="${AWS_PROFILE:-aws}"
AWS_REGION="${AWS_REGION:-us-west-2}"
BENCHMARK_BUCKET="cargoship-competitive-benchmark-$(date +%s)"
TEST_DATA_DIR="/Volumes/External HD/benchmark-data/competitive-test-10k"
RESULTS_DIR="/tmp/competitive-benchmark-results-$(date +%Y%m%d-%H%M%S)"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
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

# Create results directory
mkdir -p "$RESULTS_DIR"

log_info "Competitive Benchmark - Sequential Execution"
log_info "Results will be saved to: $RESULTS_DIR"

# Create test data if needed
if [ ! -d "$TEST_DATA_DIR" ]; then
    log_info "Creating test data: 10,000 files @ ~20MB total"
    mkdir -p "$TEST_DATA_DIR"
    for i in {1..10000}; do
        dd if=/dev/zero of="$TEST_DATA_DIR/file-$i.dat" bs=2048 count=1 2>/dev/null
    done
    log_success "Test data created: $(du -sh "$TEST_DATA_DIR" | cut -f1)"
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

# Function to measure execution time
measure_time() {
    local tool=$1
    local command=$2

    log_info "Running: $tool"
    log_info "Command: $command"

    local start=$(date +%s%3N)
    eval "$command" 2>&1 | tee "$RESULTS_DIR/$tool.log"
    local end=$(date +%s%3N)

    local duration=$((end - start))
    echo "$tool,$duration" >> "$RESULTS_DIR/results.csv"

    log_success "$tool completed in ${duration}ms"
}

# Initialize results CSV
echo "tool,duration_ms" > "$RESULTS_DIR/results.csv"

#
# 1. s5cmd - HIGH PERFORMANCE S3 CLI
#
log_info "========================"
log_info "Benchmark 1/4: s5cmd"
log_info "========================"
cleanup_s3_prefix "s5cmd-test"
measure_time "s5cmd" "s5cmd --profile $AWS_PROFILE cp '$TEST_DATA_DIR/*' s3://$BENCHMARK_BUCKET/s5cmd-test/"
sleep 5

#
# 2. MinIO mc - CLOUD STORAGE CLIENT
#
log_info "========================"
log_info "Benchmark 2/4: mc"
log_info "========================"
cleanup_s3_prefix "mc-test"
# Configure mc alias if not exists
mc alias set aws-benchmark https://s3.$AWS_REGION.amazonaws.com $(aws configure get aws_access_key_id --profile $AWS_PROFILE) $(aws configure get aws_secret_access_key --profile $AWS_PROFILE) 2>/dev/null || true
measure_time "mc" "mc cp --recursive '$TEST_DATA_DIR/' aws-benchmark/$BENCHMARK_BUCKET/mc-test/"
sleep 5

#
# 3. aws-cli - OFFICIAL AWS CLI
#
log_info "========================"
log_info "Benchmark 3/4: aws-cli"
log_info "========================"
cleanup_s3_prefix "aws-cli-test"
measure_time "aws-cli" "AWS_PROFILE=$AWS_PROFILE aws s3 cp '$TEST_DATA_DIR' s3://$BENCHMARK_BUCKET/aws-cli-test/ --recursive"
sleep 5

#
# 4. cargoship - OUR TOOL
#
log_info "========================"
log_info "Benchmark 4/4: cargoship"
log_info "========================"
cleanup_s3_prefix "cargoship-test"
# Build cargoship if needed
if [ ! -f "./cargoship" ]; then
    log_info "Building cargoship..."
    go build -o ./cargoship ./cmd/cargoship
fi
measure_time "cargoship" "AWS_PROFILE=$AWS_PROFILE ./cargoship create upload '$TEST_DATA_DIR' --bucket $BENCHMARK_BUCKET --prefix cargoship-test --region $AWS_REGION --quiet"

# Generate report
log_info "Generating comparison report..."
cat > "$RESULTS_DIR/report.md" <<EOF
# Competitive Benchmark Results

**Test Workload:** 10,000 files @ ~20MB total
**Date:** $(date)
**Platform:** $(uname -a)

## Results

| Tool | Duration (ms) | Duration (s) | Relative Speed |
|------|--------------|--------------|----------------|
EOF

# Calculate results
baseline=$(grep "s5cmd" "$RESULTS_DIR/results.csv" | cut -d',' -f2)
while IFS=, read -r tool duration; do
    if [ "$tool" = "tool" ]; then continue; fi  # Skip header

    duration_s=$(echo "scale=2; $duration / 1000" | bc)
    if [ -n "$baseline" ] && [ "$baseline" != "0" ]; then
        relative=$(echo "scale=2; $duration / $baseline" | bc)
        echo "| $tool | $duration | $duration_s | ${relative}x |" >> "$RESULTS_DIR/report.md"
    else
        echo "| $tool | $duration | $duration_s | - |" >> "$RESULTS_DIR/report.md"
    fi
done < "$RESULTS_DIR/results.csv"

cat >> "$RESULTS_DIR/report.md" <<EOF

## Analysis

**Fastest Tool:** s5cmd (baseline)
**CargoShip Performance:** See table above for relative comparison

## Raw Data

See \`results.csv\` for raw timing data.

EOF

log_success "Report generated: $RESULTS_DIR/report.md"

# Cleanup bucket
log_info "Cleaning up benchmark bucket..."
AWS_PROFILE=$AWS_PROFILE aws s3 rb "s3://$BENCHMARK_BUCKET" --force 2>&1 || log_warn "Cleanup may have failed"

log_success "Competitive benchmark complete!"
log_success "Results: $RESULTS_DIR"
