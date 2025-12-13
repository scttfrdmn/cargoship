#!/bin/bash
# CargoHold Performance Benchmark Runner

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 CargoHold Performance Benchmark Suite${NC}"
echo "=========================================="
echo ""

# Configuration
SCENARIO="${1:-small}"
BUCKET="${CARGOSHIP_BENCHMARK_BUCKET:-}"
TOOLS="${2:-cargohold,s5cmd,mc,tar}"
DATA_DIR="./test-data"
RESULTS_DIR="./results"

# Check if bucket is configured
if [ -z "$BUCKET" ]; then
    echo -e "${RED}Error: S3 bucket not configured${NC}"
    echo "Set CARGOSHIP_BENCHMARK_BUCKET environment variable:"
    echo "  export CARGOSHIP_BENCHMARK_BUCKET=my-test-bucket"
    exit 1
fi

echo -e "${YELLOW}Configuration:${NC}"
echo "  Scenario: $SCENARIO"
echo "  Bucket:   $BUCKET"
echo "  Tools:    $TOOLS"
echo ""

# Build benchmark suite
echo -e "${YELLOW}Building benchmark suite...${NC}"
go build -o cargohold-benchmark .

# Check required tools
echo -e "${YELLOW}Checking dependencies...${NC}"
for tool in cargoship aws; do
    if ! command -v $tool &> /dev/null; then
        echo -e "${RED}Error: $tool not found${NC}"
        exit 1
    fi
    echo "  ✅ $tool"
done

# Optional tools
for tool in s5cmd mc; do
    if [[ "$TOOLS" == *"$tool"* ]]; then
        if ! command -v $tool &> /dev/null; then
            echo -e "${YELLOW}  ⚠️  $tool not found (will be skipped)${NC}"
            TOOLS=$(echo "$TOOLS" | sed "s/$tool,*//g" | sed 's/,$//')
        else
            echo "  ✅ $tool"
        fi
    fi
done

echo ""

# Run benchmark
echo -e "${GREEN}Running benchmark...${NC}"
./cargohold-benchmark \
    -scenario "$SCENARIO" \
    -bucket "$BUCKET" \
    -tools "$TOOLS" \
    -data-dir "$DATA_DIR" \
    -results-dir "$RESULTS_DIR"

echo ""
echo -e "${GREEN}✅ Benchmark complete!${NC}"
echo ""
echo "Results saved to: $RESULTS_DIR"
echo ""
echo "View reports:"
echo "  Text:  $RESULTS_DIR/${SCENARIO}_report.txt"
echo "  HTML:  $RESULTS_DIR/${SCENARIO}_report.html"
echo ""

# Open HTML report if possible
if command -v open &> /dev/null; then
    echo "Opening HTML report..."
    open "$RESULTS_DIR/${SCENARIO}_report.html"
elif command -v xdg-open &> /dev/null; then
    echo "Opening HTML report..."
    xdg-open "$RESULTS_DIR/${SCENARIO}_report.html"
fi
