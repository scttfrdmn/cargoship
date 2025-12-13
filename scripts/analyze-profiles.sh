#!/bin/bash
# Profile Analysis Script
# Analyzes pprof profiles and generates bottleneck reports

set -e

PROFILE_DIR="${PROFILE_DIR:-profiles/benchmarks}"
REPORT_DIR="${REPORT_DIR:-profiles/reports}"

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

# Check if pprof is available
if ! command -v go &> /dev/null; then
    log_error "Go is not installed"
    exit 1
fi

# Create report directory
mkdir -p "$REPORT_DIR"

log_info "Analyzing profiles in $PROFILE_DIR"

# Find all profile files
CPU_PROFILES=$(find "$PROFILE_DIR" -name "*cpu*.prof" -type f 2>/dev/null | sort -r)
MEM_PROFILES=$(find "$PROFILE_DIR" -name "*memory*.prof" -type f 2>/dev/null | sort -r)
BLOCK_PROFILES=$(find "$PROFILE_DIR" -name "*block*.prof" -type f 2>/dev/null | sort -r)
MUTEX_PROFILES=$(find "$PROFILE_DIR" -name "*mutex*.prof" -type f 2>/dev/null | sort -r)

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_FILE="$REPORT_DIR/bottleneck-report-$TIMESTAMP.txt"

echo "CargoShip Performance Bottleneck Report" > "$REPORT_FILE"
echo "=======================================" >> "$REPORT_FILE"
echo "Generated: $(date)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Analyze CPU profiles
if [ -n "$CPU_PROFILES" ]; then
    log_info "Analyzing CPU profiles..."

    for profile in $CPU_PROFILES; do
        log_info "Processing: $(basename $profile)"

        echo "## CPU Profile: $(basename $profile)" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        # Top 10 functions by CPU time
        echo "### Top 10 CPU Hotspots" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        go tool pprof -text -top 10 "$profile" 2>/dev/null | head -20 >> "$REPORT_FILE" || echo "Failed to analyze" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        # Generate flame graph (if graphviz is available)
        if command -v dot &> /dev/null; then
            SVG_FILE="$REPORT_DIR/flamegraph-cpu-$(basename $profile .prof).svg"
            go tool pprof -svg "$profile" > "$SVG_FILE" 2>/dev/null || log_warn "Failed to generate SVG"
            if [ -f "$SVG_FILE" ]; then
                log_success "Flame graph: $SVG_FILE"
            fi
        fi
    done
else
    log_warn "No CPU profiles found"
fi

# Analyze memory profiles
if [ -n "$MEM_PROFILES" ]; then
    log_info "Analyzing memory profiles..."

    for profile in $MEM_PROFILES; do
        log_info "Processing: $(basename $profile)"

        echo "## Memory Profile: $(basename $profile)" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        # Top 10 allocation sites
        echo "### Top 10 Memory Allocation Sites" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        go tool pprof -text -top 10 "$profile" 2>/dev/null | head -20 >> "$REPORT_FILE" || echo "Failed to analyze" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        # Inuse space analysis
        echo "### Inuse Memory Analysis" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        go tool pprof -text -sample_index=inuse_space -top 10 "$profile" 2>/dev/null | head -20 >> "$REPORT_FILE" || echo "Failed to analyze" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    done
else
    log_warn "No memory profiles found"
fi

# Analyze block profiles
if [ -n "$BLOCK_PROFILES" ]; then
    log_info "Analyzing block profiles..."

    for profile in $BLOCK_PROFILES; do
        log_info "Processing: $(basename $profile)"

        echo "## Block Profile: $(basename $profile)" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        echo "### Top 10 Blocking Operations" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        go tool pprof -text -top 10 "$profile" 2>/dev/null | head -20 >> "$REPORT_FILE" || echo "Failed to analyze" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    done
else
    log_warn "No block profiles found"
fi

# Analyze mutex profiles
if [ -n "$MUTEX_PROFILES" ]; then
    log_info "Analyzing mutex profiles..."

    for profile in $MUTEX_PROFILES; do
        log_info "Processing: $(basename $profile)"

        echo "## Mutex Profile: $(basename $profile)" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"

        echo "### Top 10 Mutex Contentions" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        go tool pprof -text -top 10 "$profile" 2>/dev/null | head -20 >> "$REPORT_FILE" || echo "Failed to analyze" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    done
else
    log_warn "No mutex profiles found"
fi

# Summary
echo "" >> "$REPORT_FILE"
echo "## Summary" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "- CPU profiles analyzed: $(echo "$CPU_PROFILES" | wc -w | tr -d ' ')" >> "$REPORT_FILE"
echo "- Memory profiles analyzed: $(echo "$MEM_PROFILES" | wc -w | tr -d ' ')" >> "$REPORT_FILE"
echo "- Block profiles analyzed: $(echo "$BLOCK_PROFILES" | wc -w | tr -d ' ')" >> "$REPORT_FILE"
echo "- Mutex profiles analyzed: $(echo "$MUTEX_PROFILES" | wc -w | tr -d ' ')" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "## Next Steps" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "1. Review the top hotspots identified above" >> "$REPORT_FILE"
echo "2. Use \`go tool pprof -http=:8080 <profile>\` for interactive analysis" >> "$REPORT_FILE"
echo "3. Generate flame graphs with \`go tool pprof -svg <profile>\`" >> "$REPORT_FILE"
echo "4. Compare profiles across versions to track improvements" >> "$REPORT_FILE"

log_success "Report generated: $REPORT_FILE"

# Display report
cat "$REPORT_FILE"

exit 0
