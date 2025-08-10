#!/bin/bash
# CargoShip Test Quality Enforcement Script
# Checks test files for common anti-patterns and quality issues

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
ISSUES_FOUND=0
FILES_CHECKED=0

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

# Check for goroutine leak issues
check_goroutine_leaks() {
    local file=$1
    local issues=0
    
    # Look for goroutines started without proper cleanup patterns
    if grep -n "go func" "$file" | grep -v "defer\|close\|cancel\|Done"; then
        log_error "$file: Found goroutines without cleanup patterns"
        issues=$((issues + 1))
    fi
    
    # Look for context.WithTimeout with very short timeouts in tests
    if grep -n "context.WithTimeout.*time.Millisecond\*[0-9][0-9])" "$file"; then
        log_warning "$file: Found very short timeouts that may cause flaky tests"
        issues=$((issues + 1))
    fi
    
    # Check for missing defer cancel() after context creation
    if grep -n "WithCancel\|WithTimeout" "$file" | grep -v "defer cancel"; then
        if ! grep -q "defer cancel()" "$file"; then
            log_error "$file: Found context creation without defer cancel()"
            issues=$((issues + 1))
        fi
    fi
    
    return $issues
}

# Check for proper test categorization
check_test_categories() {
    local file=$1
    local issues=0
    local filename=$(basename "$file")
    
    # Files with performance/stress testing should have performance build tags
    if [[ "$filename" =~ (stress|extreme|performance|benchmark)_test\.go ]]; then
        if ! head -5 "$file" | grep -q "//go:build performance"; then
            log_error "$file: Performance test missing '//go:build performance' tag"
            issues=$((issues + 1))
        fi
    fi
    
    # Files with LocalStack or external service dependencies should have integration tags
    if grep -q "LocalStack\|localstack\|4566\|docker" "$file"; then
        if ! head -5 "$file" | grep -q "//go:build integration"; then
            log_warning "$file: Integration test missing '//go:build integration' tag"
            issues=$((issues + 1))
        fi
    fi
    
    # Check for proper short mode skipping in long-running tests
    if grep -q "time.Sleep.*Second\|time.Sleep.*Minute" "$file"; then
        if ! grep -q "testing.Short()\|testutil.SkipIfShort" "$file"; then
            log_warning "$file: Long-running test should skip in short mode"
            issues=$((issues + 1))
        fi
    fi
    
    return $issues
}

# Check for test quality anti-patterns
check_test_patterns() {
    local file=$1
    local issues=0
    
    # Check for TODO/FIXME in test files
    if grep -n "TODO\|FIXME\|XXX\|HACK" "$file"; then
        log_warning "$file: Contains TODO/FIXME comments that should be addressed"
        issues=$((issues + 1))
    fi
    
    # Check for hardcoded sleeps without justification
    if grep -n "time.Sleep" "$file" | grep -v "cleanup\|goroutine\|background"; then
        log_warning "$file: Contains time.Sleep without clear justification"
        issues=$((issues + 1))
    fi
    
    # Check for tests without error checking
    if grep -n "= .*\.Start()\|= .*\.Stop()" "$file" | grep -v "require.NoError\|assert.NoError\|if err"; then
        log_warning "$file: Found method calls without error checking"
        issues=$((issues + 1))
    fi
    
    # Check for missing test helpers
    if grep -n "func Test.*" "$file" | wc -l | xargs test 10 -lt; then
        if ! grep -q "t.Helper()" "$file" && grep -q "func.*testing.T" "$file"; then
            log_info "$file: Consider using t.Helper() in test helper functions"
        fi
    fi
    
    return $issues
}

# Check for proper resource cleanup
check_resource_cleanup() {
    local file=$1
    local issues=0
    
    # Files that create resources should have cleanup
    resource_creators=("New.*Controller\|New.*Manager\|New.*Client\|New.*Server")
    cleanup_patterns=("defer.*Stop\|defer.*Close\|defer.*Cleanup\|defer func")
    
    for pattern in "${resource_creators[@]}"; do
        if grep -q "$pattern" "$file"; then
            has_cleanup=false
            for cleanup in "${cleanup_patterns[@]}"; do
                if grep -q "$cleanup" "$file"; then
                    has_cleanup=true
                    break
                fi
            done
            
            if [ "$has_cleanup" = false ]; then
                log_warning "$file: Creates resources but missing cleanup patterns"
                issues=$((issues + 1))
            fi
            break
        fi
    done
    
    return $issues
}

# Main function to check a single test file
check_test_file() {
    local file=$1
    local file_issues=0
    
    FILES_CHECKED=$((FILES_CHECKED + 1))
    
    log_info "Checking $file..."
    
    # Run all checks
    check_goroutine_leaks "$file" || file_issues=$((file_issues + $?))
    check_test_categories "$file" || file_issues=$((file_issues + $?))
    check_test_patterns "$file" || file_issues=$((file_issues + $?))
    check_resource_cleanup "$file" || file_issues=$((file_issues + $?))
    
    if [ $file_issues -eq 0 ]; then
        log_success "$file: No issues found"
    else
        log_error "$file: Found $file_issues issues"
        ISSUES_FOUND=$((ISSUES_FOUND + file_issues))
    fi
    
    return $file_issues
}

# Main execution
main() {
    echo "🔍 CargoShip Test Quality Check"
    echo "==============================="
    
    # Find all test files
    test_files=$(find . -name "*_test.go" -not -path "./vendor/*" | sort)
    
    if [ -z "$test_files" ]; then
        log_error "No test files found!"
        exit 1
    fi
    
    # Process each test file
    for file in $test_files; do
        check_test_file "$file"
    done
    
    echo
    echo "==============================="
    echo "📊 Test Quality Summary"
    echo "Files checked: $FILES_CHECKED"
    echo "Issues found: $ISSUES_FOUND"
    
    if [ $ISSUES_FOUND -eq 0 ]; then
        log_success "🎉 All test files passed quality checks!"
        exit 0
    else
        log_error "❌ Found $ISSUES_FOUND quality issues across $FILES_CHECKED files"
        echo
        echo "💡 To fix these issues:"
        echo "  1. Add proper goroutine cleanup with defer cancel()"
        echo "  2. Use build tags for integration/performance tests"
        echo "  3. Add testutil.SkipIfShort() for long-running tests"
        echo "  4. Ensure proper resource cleanup with defer patterns"
        echo "  5. Add error checking for all method calls"
        echo
        exit 1
    fi
}

# Run main function
main "$@"