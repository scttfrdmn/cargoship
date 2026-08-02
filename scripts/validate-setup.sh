#!/bin/bash

# CargoShip Setup Validation Script
# Validates that all components are ready for testing

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${BLUE}"
echo "🚢 CargoShip Setup Validation"
echo "============================="
echo -e "${NC}"

# Check Docker
check_docker() {
    log_info "Checking Docker..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        return 1
    fi
    
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        return 1
    fi
    
    log_success "Docker is available and running"
}

# Check Docker Compose
check_docker_compose() {
    log_info "Checking Docker Compose..."
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed"
        return 1
    fi
    
    log_success "Docker Compose is available"
}

# Check project structure
check_project_structure() {
    log_info "Checking project structure..."
    
    local required_files=(
        "pkg/launch/central_controller.go"
        "pkg/launch/ghost_ship.go"
        "pkg/launch/agent.go"
        "cmd/controller/main.go"
        "cmd/ghost-ship/main.go"
        "cmd/cargoship-test/main.go"
        "docker/Dockerfile.controller"
        "docker/Dockerfile.ghost-ship"
        "docker/Dockerfile.astrapi"
        "docker/development/docker-compose.yml"
        "scripts/launch-ghost-ship.sh"
        "scripts/test-suite.sh"
    )
    
    local missing_files=()
    
    for file in "${required_files[@]}"; do
        if [[ ! -f "$PROJECT_ROOT/$file" ]]; then
            missing_files+=("$file")
        fi
    done
    
    if [[ ${#missing_files[@]} -gt 0 ]]; then
        log_error "Missing required files:"
        for file in "${missing_files[@]}"; do
            echo "  - $file"
        done
        return 1
    fi
    
    log_success "All required files are present"
}

# Check Go module
check_go_module() {
    log_info "Checking Go module..."
    
    if [[ ! -f "$PROJECT_ROOT/go.mod" ]]; then
        log_error "go.mod file not found"
        return 1
    fi
    
    if ! command -v go &> /dev/null; then
        log_warning "Go is not installed, but Docker builds should work"
        return 0
    fi
    
    cd "$PROJECT_ROOT"
    if ! go mod tidy -v &> /dev/null; then
        log_error "Go module validation failed"
        return 1
    fi
    
    log_success "Go module is valid"
}

# Validate Docker Compose configuration
check_docker_compose_config() {
    log_info "Validating Docker Compose configuration..."
    
    cd "$PROJECT_ROOT/docker/development"
    
    if ! docker-compose config --quiet; then
        log_error "Docker Compose configuration is invalid"
        return 1
    fi
    
    log_success "Docker Compose configuration is valid"
}

# Check required directories
check_directories() {
    log_info "Checking required directories..."
    
    local required_dirs=(
        "test-results"
        "docker/development/test-data/nas-1/documents"
        "docker/development/test-data/nas-1/images" 
        "docker/development/test-data/nas-2/backups"
        "docker/development/test-data/nas-2/videos"
    )
    
    for dir in "${required_dirs[@]}"; do
        mkdir -p "$PROJECT_ROOT/$dir"
    done
    
    log_success "Required directories created"
}

# Generate test data
generate_test_data() {
    log_info "Generating test data..."
    
    local test_data_dir="$PROJECT_ROOT/docker/development/test-data"
    
    # Generate documents
    for i in {1..5}; do
        echo "Sample document $i - $(date)" > "$test_data_dir/nas-1/documents/doc_$i.txt"
        echo "%PDF-1.4 Sample PDF $i" > "$test_data_dir/nas-1/documents/sample_$i.pdf"
    done
    
    # Generate small binary files as images
    for i in {1..3}; do
        dd if=/dev/zero of="$test_data_dir/nas-1/images/photo_$i.jpg" bs=1K count=50 2>/dev/null
    done
    
    # Generate backup files
    for i in {1..2}; do
        echo "backup content $i" | gzip > "$test_data_dir/nas-2/backups/backup_$i.tar.gz"
    done
    
    log_success "Test data generated"
}

# Check AWS CLI (optional)
check_aws_cli() {
    log_info "Checking AWS CLI (optional)..."
    
    if ! command -v aws &> /dev/null; then
        log_warning "AWS CLI not found - real AWS testing will not be available"
        return 0
    fi
    
    log_success "AWS CLI is available"
}

# Check network connectivity
check_network() {
    log_info "Checking network connectivity..."
    
    if ! ping -c 1 8.8.8.8 &> /dev/null; then
        log_warning "No internet connectivity - offline mode (emulator-backed tests only; real-AWS tests will skip)"
        return 0
    fi
    
    log_success "Network connectivity available"
}

# Run all checks
main() {
    local failed=0
    
    check_docker || ((failed++))
    check_docker_compose || ((failed++))
    check_project_structure || ((failed++))
    check_go_module || ((failed++))
    check_docker_compose_config || ((failed++))
    check_directories || ((failed++))
    generate_test_data || ((failed++))
    check_aws_cli
    check_network
    
    echo
    if [[ $failed -eq 0 ]]; then
        log_success "🎉 All checks passed! Setup is ready for testing."
        echo
        log_info "Next steps:"
        echo "  1. Start development environment:"
        echo "     cd docker/development && docker-compose up -d"
        echo "  2. Run tests:"
        echo "     ./scripts/test-suite.sh --generate-data local"
        echo "  3. View dashboards:"
        echo "     - Grafana: http://localhost:3000 (admin/admin123)"
        echo "     - Controller: http://localhost:8080"
        echo "     - Prometheus: http://localhost:9093"
    else
        log_error "❌ $failed checks failed. Please fix the issues above."
        exit 1
    fi
}

main "$@"