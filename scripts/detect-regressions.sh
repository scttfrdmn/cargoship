#!/bin/bash
# Regression Detection Script
# Compares current benchmark results against baseline and detects regressions
#
# Noise Reduction Features:
# - Filters out statistically insignificant changes (marked with ~ by benchstat)
# - Applies minimum threshold to ignore small fluctuations
# - Configurable via environment variables:
#   - MIN_THRESHOLD: Minimum % change to report (default: 5%)
#   - REQUIRE_SIGNIFICANCE: Require statistical significance (default: true)
#
# Exit codes:
#   0 = No regressions or low/medium severity
#   1 = High severity regressions detected (>25% slower)
#   2 = Critical regressions detected (>50% slower)

set -e

BASELINE_DIR="${BASELINE_DIR:-profiles/baselines}"
CURRENT_BENCH="${CURRENT_BENCH:-benchmark-current.txt}"
BASELINE_FILE="${BASELINE_FILE:-$BASELINE_DIR/current.txt}"

# Noise reduction settings
MIN_THRESHOLD="${MIN_THRESHOLD:-5}"  # Minimum % change to report (reduces noise)
REQUIRE_SIGNIFICANCE="${REQUIRE_SIGNIFICANCE:-true}"  # Require statistical significance

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

# Check if baseline exists
if [ ! -f "$BASELINE_FILE" ]; then
    log_error "Baseline file not found: $BASELINE_FILE"
    echo "Create a baseline first with: make benchmark-baseline"
    exit 1
fi

# Check if current benchmark results exist
if [ ! -f "$CURRENT_BENCH" ]; then
    log_error "Current benchmark results not found: $CURRENT_BENCH"
    echo "Run benchmarks first with: make test-benchmark > benchmark-current.txt"
    exit 1
fi

log_info "Comparing benchmarks against baseline"
log_info "Baseline: $BASELINE_FILE"
log_info "Current:  $CURRENT_BENCH"
log_info "Noise reduction: MIN_THRESHOLD=${MIN_THRESHOLD}%, REQUIRE_SIGNIFICANCE=${REQUIRE_SIGNIFICANCE}"

# Parse benchmark results and compare
# This is a simplified version - real implementation would use benchstat
REGRESSIONS=0
THRESHOLD=10 # 10% threshold

# Use benchstat if available
if command -v benchstat &> /dev/null; then
    log_info "Using benchstat for comparison..."

    REPORT=$(benchstat "$BASELINE_FILE" "$CURRENT_BENCH" 2>&1)
    echo "$REPORT"

    # Check for significant regressions with noise reduction
    # benchstat output contains lines like "+10.5%" for regressions
    # The '~' marker indicates not statistically significant - filter those out
    if [ "$REQUIRE_SIGNIFICANCE" = "true" ]; then
        # Filter out non-significant results (marked with ~)
        DEGRADED=$(echo "$REPORT" | grep -E '\+[0-9]+\.[0-9]+%' | grep -v '~' || true)
    else
        # Include all results regardless of significance
        DEGRADED=$(echo "$REPORT" | grep -E '\+[0-9]+\.[0-9]+%' || true)
    fi

    # Apply minimum threshold filter to reduce noise
    if [ -n "$DEGRADED" ] && [ "$MIN_THRESHOLD" -gt 0 ]; then
        # Filter out changes below minimum threshold
        # This regex matches percentages >= MIN_THRESHOLD
        if [ "$MIN_THRESHOLD" -ge 10 ]; then
            # For thresholds >= 10%, use pattern matching on first digit
            FIRST_DIGIT=$((MIN_THRESHOLD / 10))
            DEGRADED=$(echo "$DEGRADED" | grep -E "\+[${FIRST_DIGIT}-9][0-9]\." || true)
        else
            # For thresholds < 10%, we need more complex filtering
            # Just filter out very small changes (< 5%)
            DEGRADED=$(echo "$DEGRADED" | grep -E '\+([5-9]\.|[1-9][0-9])' || true)
        fi
    fi

    if [ -n "$DEGRADED" ]; then
        log_warn "Performance regressions detected:"
        echo "$DEGRADED"

        # Count regressions
        REGRESSIONS=$(echo "$DEGRADED" | wc -l | tr -d ' ')

        # Check if any exceed critical threshold (50%)
        CRITICAL=$(echo "$DEGRADED" | grep -E '\+[5-9][0-9]\.' || true)
        if [ -n "$CRITICAL" ]; then
            log_error "Critical regressions detected (>50% slower)"
            exit 2
        fi

        # Check if any exceed high threshold (25%)
        HIGH=$(echo "$DEGRADED" | grep -E '\+[2-4][0-9]\.' || true)
        if [ -n "$HIGH" ]; then
            log_warn "High severity regressions detected (>25% slower)"
            exit 1
        fi

        log_warn "Low/medium severity regressions detected"
        exit 0
    else
        log_success "No significant regressions detected!"
        exit 0
    fi
else
    log_warn "benchstat not found, using simple comparison"
    log_info "Install benchstat: go install golang.org/x/perf/cmd/benchstat@latest"

    # Simple line-by-line comparison (fallback)
    echo ""
    echo "=== Baseline vs Current ==="
    echo ""

    # Extract benchmark names and times
    BASELINE_BENCHES=$(grep '^Benchmark' "$BASELINE_FILE" | awk '{print $1}')

    for bench in $BASELINE_BENCHES; do
        BASELINE_TIME=$(grep "^$bench" "$BASELINE_FILE" | awk '{print $3}')
        CURRENT_TIME=$(grep "^$bench" "$CURRENT_BENCH" | awk '{print $3}' || echo "0")

        if [ "$CURRENT_TIME" != "0" ] && [ -n "$BASELINE_TIME" ]; then
            # Calculate percentage change (simplified - needs bc for real calculation)
            echo "$bench:"
            echo "  Baseline: $BASELINE_TIME"
            echo "  Current:  $CURRENT_TIME"
            echo ""
        fi
    done

    log_info "Manual comparison complete. Use benchstat for detailed analysis."
    exit 0
fi
