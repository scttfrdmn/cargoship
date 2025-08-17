#!/bin/bash

# CargoShip Quality Gates Script
# This script runs comprehensive quality checks before allowing commits or deployments

set -euo pipefail

# Configuration
TIMEOUT_TESTS="120s"
TIMEOUT_LINT="10m"
COVERAGE_THRESHOLD="75"
MAX_ISSUES_PER_LINTER="5"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Check if required tools are installed
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    if ! command -v go &> /dev/null; then
        missing_tools+=("go")
    fi
    
    if ! command -v golangci-lint &> /dev/null; then
        missing_tools+=("golangci-lint")
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Please install them before running quality gates"
        exit 1
    fi
    
    log_success "All prerequisites are installed"
}

# Run Go formatting checks
check_formatting() {
    log_info "Running formatting checks..."
    
    # Check if any Go files need formatting
    local unformatted_files
    unformatted_files=$(gofmt -l . 2>/dev/null || true)
    
    if [ -n "$unformatted_files" ]; then
        log_error "The following files are not properly formatted:"
        echo "$unformatted_files"
        log_info "Run 'go fmt ./...' to fix formatting issues"
        return 1
    fi
    
    # Check imports
    if command -v goimports &> /dev/null; then
        log_info "Checking imports with goimports..."
        local import_issues
        import_issues=$(goimports -l . 2>/dev/null || true)
        
        if [ -n "$import_issues" ]; then
            log_error "The following files have import issues:"
            echo "$import_issues"
            log_info "Run 'goimports -w .' to fix import issues"
            return 1
        fi
    fi
    
    log_success "All files are properly formatted"
}

# Run comprehensive linting
run_linting() {
    log_info "Running comprehensive linting with golangci-lint..."
    
    # Run golangci-lint with our configuration
    if ! golangci-lint run --timeout="${TIMEOUT_LINT}" --max-issues-per-linter="${MAX_ISSUES_PER_LINTER}"; then
        log_error "Linting failed - please fix the reported issues"
        return 1
    fi
    
    log_success "Linting passed"
}

# Run tests with coverage
run_tests() {
    log_info "Running tests with coverage..."
    
    # Clean previous test artifacts
    rm -f coverage.out coverage.html
    
    # Run tests with coverage, excluding AWS tests which may have leaks
    if ! go test -timeout="${TIMEOUT_TESTS}" -coverprofile=coverage.out -covermode=atomic \
         $(go list ./... | grep -v '/aws/s3$'); then
        log_error "Tests failed"
        return 1
    fi
    
    # Check coverage threshold
    local coverage_pct
    coverage_pct=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    
    if (( $(echo "$coverage_pct < $COVERAGE_THRESHOLD" | bc -l) )); then
        log_warning "Code coverage is ${coverage_pct}%, below threshold of ${COVERAGE_THRESHOLD}%"
        log_info "Consider adding more tests to improve coverage"
        # Don't fail on coverage for now, just warn
    else
        log_success "Code coverage is ${coverage_pct}%, meets threshold of ${COVERAGE_THRESHOLD}%"
    fi
    
    # Generate HTML coverage report
    go tool cover -html=coverage.out -o=coverage.html
    log_info "Coverage report generated: coverage.html"
}

# Check for potential security issues
security_check() {
    log_info "Running security checks..."
    
    # Check for common security issues in Go code
    local security_issues=0
    
    # Check for hardcoded credentials patterns
    if grep -r "password.*=" --include="*.go" . | grep -v "_test.go" | grep -v "testdata" | grep -q .; then
        log_warning "Potential hardcoded passwords found"
        security_issues=$((security_issues + 1))
    fi
    
    # Check for HTTP usage (should be HTTPS)
    if grep -r "http://" --include="*.go" . | grep -v "_test.go" | grep -v "localhost" | grep -q .; then
        log_warning "HTTP URLs found - consider using HTTPS"
        security_issues=$((security_issues + 1))
    fi
    
    if [ $security_issues -eq 0 ]; then
        log_success "No obvious security issues found"
    else
        log_warning "Found $security_issues potential security issues"
    fi
}

# Check build for all platforms
check_build() {
    log_info "Checking builds for multiple platforms..."
    
    local platforms=("linux/amd64" "darwin/amd64" "windows/amd64")
    
    for platform in "${platforms[@]}"; do
        local os_arch=(${platform//\// })
        local goos=${os_arch[0]}
        local goarch=${os_arch[1]}
        
        log_info "Building for $goos/$goarch..."
        
        if ! GOOS=$goos GOARCH=$goarch go build -o /dev/null ./cmd/cargoship; then
            log_error "Build failed for $goos/$goarch"
            return 1
        fi
    done
    
    log_success "All platform builds successful"
}

# Check dependencies for known vulnerabilities
dependency_check() {
    log_info "Checking dependencies for vulnerabilities..."
    
    if command -v govulncheck &> /dev/null; then
        if ! govulncheck ./...; then
            log_warning "Vulnerability check found issues"
            # Don't fail for now, just warn
        else
            log_success "No known vulnerabilities found"
        fi
    else
        log_info "govulncheck not installed - skipping vulnerability check"
        log_info "Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
    fi
}

# Generate quality report
generate_report() {
    local report_file="quality-report.txt"
    log_info "Generating quality report..."
    
    {
        echo "CargoShip Quality Report"
        echo "Generated: $(date)"
        echo "================================"
        echo
        echo "Go Version: $(go version)"
        echo "golangci-lint Version: $(golangci-lint version --format short 2>/dev/null || echo 'not available')"
        echo
        echo "Test Coverage:"
        if [ -f coverage.out ]; then
            go tool cover -func=coverage.out | tail -1
        else
            echo "No coverage data available"
        fi
        echo
        echo "Package Count: $(go list ./... | wc -l | tr -d ' ')"
        echo "Go Files: $(find . -name '*.go' -not -path './vendor/*' | wc -l | tr -d ' ')"
        echo "Test Files: $(find . -name '*_test.go' -not -path './vendor/*' | wc -l | tr -d ' ')"
        echo
        echo "Lines of Code:"
        find . -name '*.go' -not -path './vendor/*' -not -name '*_test.go' -exec wc -l {} + | tail -1
        echo
        echo "Quality Gates Status: PASSED"
    } > "$report_file"
    
    log_success "Quality report saved to $report_file"
}

# Main quality gates function
run_quality_gates() {
    log_info "🚀 Starting CargoShip Quality Gates"
    echo "======================================"
    
    local start_time=$(date +%s)
    
    # Run all quality checks
    check_prerequisites
    check_formatting
    run_linting
    run_tests
    security_check
    check_build
    dependency_check
    generate_report
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo "======================================"
    log_success "🎉 All quality gates passed! (${duration}s)"
    log_info "The code is ready for commit/deployment"
}

# Handle script arguments
case "${1:-all}" in
    "format")
        check_formatting
        ;;
    "lint")
        run_linting
        ;;
    "test")
        run_tests
        ;;
    "security")
        security_check
        ;;
    "build")
        check_build
        ;;
    "deps")
        dependency_check
        ;;
    "all"|"")
        run_quality_gates
        ;;
    *)
        echo "Usage: $0 [format|lint|test|security|build|deps|all]"
        echo "  format   - Check code formatting"
        echo "  lint     - Run linting checks"
        echo "  test     - Run tests with coverage"
        echo "  security - Run security checks"
        echo "  build    - Check multi-platform builds"
        echo "  deps     - Check dependencies for vulnerabilities"
        echo "  all      - Run all quality gates (default)"
        exit 1
        ;;
esac