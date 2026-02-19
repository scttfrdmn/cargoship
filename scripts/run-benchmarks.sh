#!/bin/bash
# CargoShip Comprehensive Benchmark Runner
# Runs benchmark suite with profiling, regression detection, and detailed analysis

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
BENCHMARK_DIR="./pkg/benchmarks/scenarios"
PROFILE_DIR="./profiles"
BASELINE_FILE="./pkg/benchmarks/baseline.json"
REPORT_DIR="./benchmark-reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_FILE="${REPORT_DIR}/benchmark-${TIMESTAMP}.txt"

# Benchmark parameters
BENCH_TIME="${BENCH_TIME:-3s}"  # Duration per benchmark
BENCH_COUNT="${BENCH_COUNT:-5}"  # Number of iterations

# Profiling options
ENABLE_CPU_PROFILE="${ENABLE_CPU_PROFILE:-true}"
ENABLE_MEM_PROFILE="${ENABLE_MEM_PROFILE:-true}"
ENABLE_TRACE="${ENABLE_TRACE:-false}"

# AWS configuration
export AWS_REGION="${AWS_REGION:-us-west-2}"
export BENCHMARK_BUCKET="${BENCHMARK_BUCKET:-cargoship-benchmark-test}"

# Functions
print_header() {
    echo ""
    echo -e "${CYAN}============================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}============================================${NC}"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_requirements() {
    print_header "Checking Requirements"

    # Check if running from repo root
    if [ ! -f "go.mod" ]; then
        log_error "Please run this script from the repository root directory"
        exit 1
    fi

    # Check Go installation
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        exit 1
    fi
    log_success "Go $(go version | awk '{print $3}')"

    # Check AWS credentials (optional - benchmarks run without AWS)
    if ! command -v aws &> /dev/null; then
        log_warning "AWS CLI not installed - S3 integration benchmarks will be skipped"
    else
        if aws sts get-caller-identity &> /dev/null 2>&1; then
            log_success "AWS credentials configured"
        else
            log_warning "AWS credentials not configured - S3 integration benchmarks will be skipped"
        fi
    fi
}

setup_directories() {
    print_header "Setting Up Directories"

    mkdir -p "${PROFILE_DIR}"
    mkdir -p "${REPORT_DIR}"

    log_success "Created ${PROFILE_DIR}"
    log_success "Created ${REPORT_DIR}"
}

print_config() {
    print_header "Benchmark Configuration"

    echo "Timestamp:        ${TIMESTAMP}"
    echo "Benchmark Time:   ${BENCH_TIME}"
    echo "Iterations:       ${BENCH_COUNT}"
    echo "AWS Region:       ${AWS_REGION}"
    echo "S3 Bucket:        ${BENCHMARK_BUCKET}"
    echo "Profile Dir:      ${PROFILE_DIR}"
    echo "Report File:      ${REPORT_FILE}"
    echo "Baseline File:    ${BASELINE_FILE}"
    echo ""
    echo "Profiling:"
    echo "  CPU Profile:    ${ENABLE_CPU_PROFILE}"
    echo "  Memory Profile: ${ENABLE_MEM_PROFILE}"
    echo "  Trace:          ${ENABLE_TRACE}"
}

run_benchmarks() {
    print_header "Running Benchmarks"

    # Build benchmark flags
    BENCH_FLAGS="-bench=. -benchmem -benchtime=${BENCH_TIME} -count=${BENCH_COUNT}"

    # Add profiling flags
    if [ "$ENABLE_CPU_PROFILE" = "true" ]; then
        BENCH_FLAGS="${BENCH_FLAGS} -cpuprofile=${PROFILE_DIR}/cpu-${TIMESTAMP}.prof"
    fi

    if [ "$ENABLE_MEM_PROFILE" = "true" ]; then
        BENCH_FLAGS="${BENCH_FLAGS} -memprofile=${PROFILE_DIR}/mem-${TIMESTAMP}.prof"
    fi

    if [ "$ENABLE_TRACE" = "true" ]; then
        BENCH_FLAGS="${BENCH_FLAGS} -trace=${PROFILE_DIR}/trace-${TIMESTAMP}.out"
    fi

    log_info "Running: go test ${BENCH_FLAGS} ${BENCHMARK_DIR}"
    echo ""

    # Run benchmarks and capture output
    if go test ${BENCH_FLAGS} ${BENCHMARK_DIR} 2>&1 | tee "${REPORT_FILE}"; then
        log_success "Benchmarks completed successfully"
    else
        log_error "Benchmarks failed"
        exit 1
    fi
}

analyze_profiles() {
    print_header "Profile Analysis"

    if [ "$ENABLE_CPU_PROFILE" = "true" ]; then
        CPU_PROF="${PROFILE_DIR}/cpu-${TIMESTAMP}.prof"
        if [ -f "$CPU_PROF" ]; then
            log_info "CPU Profile: ${CPU_PROF}"
            echo "  Top 10 functions by CPU time:"
            go tool pprof -top -nodecount=10 "$CPU_PROF" 2>/dev/null | grep -A 10 "Showing nodes" || log_warning "Unable to analyze CPU profile"
            echo ""
            log_info "To analyze interactively: go tool pprof ${CPU_PROF}"
        fi
    fi

    if [ "$ENABLE_MEM_PROFILE" = "true" ]; then
        MEM_PROF="${PROFILE_DIR}/mem-${TIMESTAMP}.prof"
        if [ -f "$MEM_PROF" ]; then
            log_info "Memory Profile: ${MEM_PROF}"
            echo "  Top 10 functions by allocations:"
            go tool pprof -top -nodecount=10 -alloc_space "$MEM_PROF" 2>/dev/null | grep -A 10 "Showing nodes" || log_warning "Unable to analyze memory profile"
            echo ""
            log_info "To analyze interactively: go tool pprof ${MEM_PROF}"
        fi
    fi

    if [ "$ENABLE_TRACE" = "true" ]; then
        TRACE_FILE="${PROFILE_DIR}/trace-${TIMESTAMP}.out"
        if [ -f "$TRACE_FILE" ]; then
            log_info "Trace File: ${TRACE_FILE}"
            log_info "To view: go tool trace ${TRACE_FILE}"
        fi
    fi
}

compare_baseline() {
    print_header "Baseline Comparison"

    if [ ! -f "$BASELINE_FILE" ]; then
        log_warning "No baseline file found at ${BASELINE_FILE}"
        log_info "To establish baseline, save current results as baseline"
        return
    fi

    log_info "Comparing against baseline: ${BASELINE_FILE}"

    # Extract key metrics from report
    if grep -q "FAIL" "${REPORT_FILE}"; then
        log_error "Some benchmarks failed - cannot perform comparison"
        return
    fi

    # Show baseline timestamp if available
    if command -v jq &> /dev/null && [ -f "$BASELINE_FILE" ]; then
        BASELINE_VERSION=$(jq -r '.Version // "unknown"' "$BASELINE_FILE" 2>/dev/null)
        BASELINE_TIME=$(jq -r '.Timestamp // "unknown"' "$BASELINE_FILE" 2>/dev/null)
        log_info "Baseline: ${BASELINE_VERSION} (${BASELINE_TIME})"
    fi

    log_info "Manual comparison required:"
    echo "  1. Review ${REPORT_FILE}"
    echo "  2. Compare metrics against ${BASELINE_FILE}"
    echo "  3. Look for significant changes in:"
    echo "     - Throughput (MB/s)"
    echo "     - Latency (ms/op, p50_ms, p95_ms, p99_ms)"
    echo "     - Memory (allocs/op, B/op)"
}

generate_summary() {
    print_header "Benchmark Summary"

    # Extract summary statistics from report
    if [ -f "${REPORT_FILE}" ]; then
        echo "Report Location: ${REPORT_FILE}"
        echo ""

        # Count benchmark results
        TOTAL_BENCHMARKS=$(grep -c "^Benchmark" "${REPORT_FILE}" 2>/dev/null || echo "0")
        PASSED_BENCHMARKS=$(grep "^ok" "${REPORT_FILE}" | wc -l || echo "0")

        echo "Benchmarks Run:   ${TOTAL_BENCHMARKS}"
        echo "Packages Tested:  ${PASSED_BENCHMARKS}"
        echo ""

        # Show profile locations
        if [ "$ENABLE_CPU_PROFILE" = "true" ] && [ -f "${PROFILE_DIR}/cpu-${TIMESTAMP}.prof" ]; then
            echo "CPU Profile:      ${PROFILE_DIR}/cpu-${TIMESTAMP}.prof"
        fi
        if [ "$ENABLE_MEM_PROFILE" = "true" ] && [ -f "${PROFILE_DIR}/mem-${TIMESTAMP}.prof" ]; then
            echo "Memory Profile:   ${PROFILE_DIR}/mem-${TIMESTAMP}.prof"
        fi
        if [ "$ENABLE_TRACE" = "true" ] && [ -f "${PROFILE_DIR}/trace-${TIMESTAMP}.out" ]; then
            echo "Trace File:       ${PROFILE_DIR}/trace-${TIMESTAMP}.out"
        fi
    fi
}

cleanup_old_reports() {
    print_header "Cleanup"

    # Keep only last 10 reports
    REPORT_COUNT=$(ls -1 "${REPORT_DIR}"/benchmark-*.txt 2>/dev/null | wc -l)
    if [ "$REPORT_COUNT" -gt 10 ]; then
        log_info "Removing old reports (keeping last 10)..."
        ls -1t "${REPORT_DIR}"/benchmark-*.txt | tail -n +11 | xargs rm -f
        log_success "Cleanup complete"
    else
        log_info "No cleanup needed (${REPORT_COUNT} reports)"
    fi

    # Keep only last 10 profiles
    PROFILE_COUNT=$(ls -1 "${PROFILE_DIR}"/*.prof 2>/dev/null | wc -l)
    if [ "$PROFILE_COUNT" -gt 20 ]; then
        log_info "Removing old profiles (keeping last 20)..."
        ls -1t "${PROFILE_DIR}"/*.prof | tail -n +21 | xargs rm -f
        log_success "Profile cleanup complete"
    fi
}

# Main execution
main() {
    print_header "CargoShip Benchmark Runner"
    echo "Starting benchmark suite at $(date)"

    check_requirements
    setup_directories
    print_config
    run_benchmarks
    analyze_profiles
    compare_baseline
    generate_summary
    cleanup_old_reports

    print_header "Benchmark Complete"
    log_success "Results saved to ${REPORT_FILE}"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --short)
            BENCH_TIME="1s"
            BENCH_COUNT="3"
            shift
            ;;
        --long)
            BENCH_TIME="10s"
            BENCH_COUNT="10"
            shift
            ;;
        --no-profile)
            ENABLE_CPU_PROFILE="false"
            ENABLE_MEM_PROFILE="false"
            ENABLE_TRACE="false"
            shift
            ;;
        --trace)
            ENABLE_TRACE="true"
            shift
            ;;
        --bucket)
            BENCHMARK_BUCKET="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --short          Run short benchmarks (1s, 3 iterations)"
            echo "  --long           Run long benchmarks (10s, 10 iterations)"
            echo "  --no-profile     Disable all profiling"
            echo "  --trace          Enable execution tracing"
            echo "  --bucket NAME    Use specific S3 bucket"
            echo "  --help           Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  AWS_REGION       AWS region (default: us-west-2)"
            echo "  BENCH_TIME       Benchmark duration (default: 3s)"
            echo "  BENCH_COUNT      Iteration count (default: 5)"
            echo "  BENCHMARK_BUCKET S3 bucket name (default: cargoship-benchmark-test)"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Run main function
main
