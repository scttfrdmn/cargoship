#!/bin/bash

# CargoShip Benchmark Runner
# This script runs all benchmarks and performance tests separately from pre-commit hooks
# Run this periodically to monitor performance characteristics

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

main() {
    echo "🚀 CargoShip Benchmark Suite"
    echo "============================"
    
    log_info "Running compression algorithm benchmarks..."
    
    # Run compression benchmarks
    if ! go test -v -run=Benchmark ./pkg/compression -timeout=300s; then
        log_error "Compression benchmarks failed"
        exit 1
    fi
    
    log_info "Running S3 transporter benchmarks..."
    
    # Run S3 benchmarks
    if ! go test -v -run=Benchmark ./pkg/aws/s3 -timeout=300s; then
        log_error "S3 benchmarks failed"
        exit 1
    fi
    
    log_info "Running all Go benchmarks..."
    
    # Run all benchmarks with proper benchmark flag
    if ! go test -bench=. -benchmem ./... -timeout=600s; then
        log_error "Some benchmarks failed"
        exit 1
    fi
    
    log_success "All benchmarks completed successfully!"
    
    echo
    echo "📊 Performance monitoring complete."
    echo "   Use 'go test -bench=. -benchmem ./pkg/specific' for targeted benchmarks"
    echo "   Use 'go test -benchtime=10s -bench=.' for longer benchmark runs"
}

# Check if this is being run from the correct directory
if [ ! -f "go.mod" ]; then
    log_error "Please run this script from the repository root directory"
    exit 1
fi

main "$@"