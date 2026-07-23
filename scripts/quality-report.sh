#!/bin/bash
# CargoShip Quality Report Script
# Modern alternative to goreportcard-cli using up-to-date tools
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Report variables
REPORT_DIR="reports"
TIMESTAMP=$(date +"%Y-%m-%d_%H-%M-%S")
REPORT_FILE="$REPORT_DIR/quality-report-$TIMESTAMP.json"
TOTAL_SCORE=0
MAX_SCORE=100

# Create reports directory
mkdir -p "$REPORT_DIR"

echo -e "${BLUE}🚢 CargoShip Quality Report${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo "Generated: $(date)"
echo "Project: $(basename $(pwd))"
echo ""

# Initialize JSON report
cat > "$REPORT_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "project": "$(basename $(pwd))",
  "version": "$(git describe --tags --always 2>/dev/null || echo 'unknown')",
  "commit": "$(git rev-parse HEAD 2>/dev/null || echo 'unknown')",
  "metrics": {
EOF

# Function to add metric to JSON report
add_metric() {
    local name="$1"
    local score="$2"
    local details="$3"
    local max_score="$4"
    
    cat >> "$REPORT_FILE" <<EOF
    "$name": {
      "score": $score,
      "max_score": $max_score,
      "percentage": $(echo "scale=2; $score * 100 / $max_score" | bc -l),
      "details": "$details"
    },
EOF
}

# Function to display metric
display_metric() {
    local name="$1"
    local score="$2"
    local max_score="$3"
    local details="$4"
    
    local percentage=$(echo "scale=1; $score * 100 / $max_score" | bc -l)
    local color=$RED
    
    if (( $(echo "$percentage >= 90" | bc -l) )); then
        color=$GREEN
    elif (( $(echo "$percentage >= 70" | bc -l) )); then
        color=$YELLOW
    fi
    
    printf "%-20s %s%6.1f%%%s %s\n" "$name" "$color" "$percentage" "$NC" "$details"
    TOTAL_SCORE=$(echo "$TOTAL_SCORE + $score" | bc -l)
}

# 1. Go Format Check (20 points)
echo -e "${BLUE}📝 Checking code formatting...${NC}"
gofmt_issues=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./pkg/mod/*" | xargs gofmt -l | wc -l)
gofmt_score=$(echo "20 - $gofmt_issues" | bc)
if [ $gofmt_score -lt 0 ]; then gofmt_score=0; fi
add_metric "gofmt" "$gofmt_score" "$gofmt_issues unformatted files" 20
display_metric "Go Format" "$gofmt_score" 20 "($gofmt_issues issues)"

# 2. Go Vet Check (20 points)
echo -e "${BLUE}🔍 Running go vet...${NC}"
if go vet ./... 2>/dev/null; then
    govet_score=20
    govet_issues=0
else
    govet_issues=$(go vet ./... 2>&1 | grep -c ":" || echo 0)
    govet_score=$(echo "20 - $govet_issues" | bc)
    if [ $govet_score -lt 0 ]; then govet_score=0; fi
fi
add_metric "go_vet" "$govet_score" "$govet_issues issues found" 20
display_metric "Go Vet" "$govet_score" 20 "($govet_issues issues)"

# 3. Golangci-lint Check (25 points)
echo -e "${BLUE}🔧 Running golangci-lint...${NC}"
if command -v golangci-lint >/dev/null 2>&1; then
    if golangci-lint run --timeout=2m ./... 2>/dev/null; then
        lint_score=25
        lint_issues=0
    else
        lint_issues=$(golangci-lint run --timeout=2m ./... 2>&1 | grep -c ":" || echo 0)
        lint_score=$(echo "25 - $lint_issues / 2" | bc)
        if [ $lint_score -lt 0 ]; then lint_score=0; fi
    fi
else
    echo "golangci-lint not found, skipping..."
    lint_score=0
    lint_issues="N/A"
fi
add_metric "linting" "$lint_score" "$lint_issues issues found" 25
display_metric "Linting" "$lint_score" 25 "($lint_issues issues)"

# 4. Test Coverage (20 points)
echo -e "${BLUE}🧪 Calculating test coverage...${NC}"
if go test -coverprofile=coverage.out ./... >/dev/null 2>&1; then
    coverage=$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $3}' | sed 's/%//')
    coverage_score=$(echo "scale=1; $coverage * 20 / 100" | bc -l)
    rm -f coverage.out
else
    coverage=0
    coverage_score=0
fi
add_metric "test_coverage" "$coverage_score" "${coverage}% coverage" 20
display_metric "Test Coverage" "$coverage_score" 20 "(${coverage}%)"

# 5. License Check (5 points)
echo -e "${BLUE}📄 Checking license...${NC}"
if [ -f LICENSE ] || [ -f LICENSE.md ] || [ -f LICENSE.txt ]; then
    license_score=5
    license_status="present"
else
    license_score=0
    license_status="missing"
fi
add_metric "license" "$license_score" "$license_status" 5
display_metric "License" "$license_score" 5 "($license_status)"

# 6. Go Modules Check (10 points)
echo -e "${BLUE}📦 Checking Go modules...${NC}"
if [ -f go.mod ] && go mod verify >/dev/null 2>&1; then
    modules_score=10
    modules_status="valid"
else
    modules_score=0
    modules_status="invalid/missing"
fi
add_metric "go_modules" "$modules_score" "$modules_status" 10
display_metric "Go Modules" "$modules_score" 10 "($modules_status)"

# Calculate total score and grade
MAX_SCORE=100
percentage=$(echo "scale=1; $TOTAL_SCORE * 100 / $MAX_SCORE" | bc -l)

# Determine grade
if (( $(echo "$percentage >= 90" | bc -l) )); then
    grade="A+"
    grade_color=$GREEN
elif (( $(echo "$percentage >= 80" | bc -l) )); then
    grade="A"
    grade_color=$GREEN
elif (( $(echo "$percentage >= 70" | bc -l) )); then
    grade="B"
    grade_color=$YELLOW
elif (( $(echo "$percentage >= 60" | bc -l) )); then
    grade="C"
    grade_color=$YELLOW
else
    grade="F"
    grade_color=$RED
fi

# Finish JSON report
cat >> "$REPORT_FILE" <<EOF
    "summary": {
      "total_score": $TOTAL_SCORE,
      "max_score": $MAX_SCORE,
      "percentage": $percentage,
      "grade": "$grade"
    }
  }
}
EOF

# Display summary
echo ""
echo -e "${BLUE}📊 Quality Report Summary${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
printf "Total Score: %s%.1f/%.0f (%.1f%%)%s\n" "$grade_color" "$TOTAL_SCORE" "$MAX_SCORE" "$percentage" "$NC"
printf "Grade: %s%s%s\n" "$grade_color" "$grade" "$NC"
echo ""
echo "📁 Detailed report saved to: $REPORT_FILE"

# Output for CI/automation (optional JSON format)
if [ "${1:-}" = "--json" ]; then
    cat "$REPORT_FILE"
fi

# Set exit code based on grade (optional threshold)
THRESHOLD=${QUALITY_THRESHOLD:-70}
if (( $(echo "$percentage < $THRESHOLD" | bc -l) )); then
    echo -e "${RED}⚠️  Quality score below threshold ($THRESHOLD%)${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Quality check passed!${NC}"