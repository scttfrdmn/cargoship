#!/bin/bash

# CargoShip Test Suite
# Comprehensive testing script for launch/ghost ship architecture

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_RESULTS_DIR="$PROJECT_ROOT/test-results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# Test environment configuration
LOCAL_CONTROLLER_URL="http://localhost:8080"
LOCAL_GHOST_SHIP_1_URL="http://localhost:9091"
LOCAL_GHOST_SHIP_2_URL="http://localhost:9092"
ASTRAPI_HOST="${ASTRAPI_HOST:-astrapi.local}"
ASTRAPI_USER="${ASTRAPI_USER:-admin}"

# Authentication tokens
CONTROLLER_TOKEN="${CARGOSHIP_CONTROLLER_TOKEN:-dev-controller-token-123}"
ADMIN_TOKEN="${CARGOSHIP_ADMIN_TOKEN:-dev-admin-token-456}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m'

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_test() { echo -e "${PURPLE}[TEST]${NC} $1"; }

# Create test results directory
mkdir -p "$TEST_RESULTS_DIR"
TEST_REPORT="$TEST_RESULTS_DIR/test-report-$TIMESTAMP.json"

# Initialize test report
cat > "$TEST_REPORT" << EOF
{
  "test_run": {
    "timestamp": "$(date -Iseconds)",
    "environment": "unknown",
    "results": {}
  }
}
EOF

# Test result tracking
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Function to record test result
record_test_result() {
  local test_name="$1"
  local status="$2"
  local message="$3"
  local duration="${4:-0}"
  
  # Update counters
  case "$status" in
    "PASSED") ((TESTS_PASSED++)) ;;
    "FAILED") ((TESTS_FAILED++)) ;;
    "SKIPPED") ((TESTS_SKIPPED++)) ;;
  esac
  
  # Add to JSON report
  local temp_file=$(mktemp)
  jq --arg name "$test_name" \
     --arg status "$status" \
     --arg message "$message" \
     --arg duration "$duration" \
     '.test_run.results[$name] = {
       "status": $status,
       "message": $message,
       "duration": $duration,
       "timestamp": now | todate
     }' "$TEST_REPORT" > "$temp_file"
  mv "$temp_file" "$TEST_REPORT"
}

# Function to run a test with timeout and error handling
run_test() {
  local test_name="$1"
  local test_command="$2"
  local timeout_seconds="${3:-30}"
  
  log_test "Running: $test_name"
  
  local start_time=$(date +%s)
  
  if timeout "$timeout_seconds" bash -c "$test_command"; then
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    log_success "$test_name - PASSED (${duration}s)"
    record_test_result "$test_name" "PASSED" "Test completed successfully" "$duration"
    return 0
  else
    local exit_code=$?
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    if [ $exit_code -eq 124 ]; then
      log_error "$test_name - FAILED (TIMEOUT after ${timeout_seconds}s)"
      record_test_result "$test_name" "FAILED" "Test timed out after ${timeout_seconds} seconds" "$duration"
    else
      log_error "$test_name - FAILED (exit code: $exit_code)"
      record_test_result "$test_name" "FAILED" "Test failed with exit code $exit_code" "$duration"
    fi
    return 1
  fi
}

# Wait for service to be healthy
wait_for_service() {
  local service_name="$1"
  local health_url="$2"
  local max_attempts="${3:-30}"
  local sleep_interval="${4:-2}"
  
  log_info "Waiting for $service_name to be healthy..."
  
  for ((i=1; i<=max_attempts; i++)); do
    if curl -f -s "$health_url" > /dev/null 2>&1; then
      log_success "$service_name is healthy"
      return 0
    fi
    
    log_info "Attempt $i/$max_attempts - $service_name not ready, waiting ${sleep_interval}s..."
    sleep "$sleep_interval"
  done
  
  log_error "$service_name failed to become healthy after $max_attempts attempts"
  return 1
}

# Local development tests
test_local_environment() {
  log_info "🐳 Testing Local Development Environment"
  
  # Update test report environment
  local temp_file=$(mktemp)
  jq '.test_run.environment = "local_docker"' "$TEST_REPORT" > "$temp_file"
  mv "$temp_file" "$TEST_REPORT"
  
  # Test 1: Docker Compose services are running
  run_test "docker_services_running" '
    cd '"$PROJECT_ROOT"'/docker/development
    docker-compose ps | grep -q "Up"
  ' 10
  
  # Test 2: Controller health check
  run_test "controller_health" '
    wait_for_service "Controller" "'"$LOCAL_CONTROLLER_URL"'/health"
  ' 60
  
  # Test 3: Ghost Ship 1 health check  
  run_test "ghost_ship_1_health" '
    wait_for_service "Ghost Ship 1" "'"$LOCAL_GHOST_SHIP_1_URL"'/health"
  ' 60
  
  # Test 4: Ghost Ship 2 health check
  run_test "ghost_ship_2_health" '
    wait_for_service "Ghost Ship 2" "'"$LOCAL_GHOST_SHIP_2_URL"'/health"
  ' 60
  
  # Test 5: LocalStack S3 connectivity
  run_test "localstack_s3_connectivity" '
    aws --endpoint-url=http://localhost:4566 s3 ls > /dev/null 2>&1
  ' 15
  
  # Test 6: Controller API authentication
  run_test "controller_api_auth" '
    response=$(curl -s -w "%{http_code}" -H "Authorization: Bearer '"$CONTROLLER_TOKEN"'" "'"$LOCAL_CONTROLLER_URL"'/api/v1/status")
    echo "$response" | grep -q "200$"
  ' 10
  
  # Test 7: WebSocket connectivity
  run_test "websocket_connectivity" '
    timeout 10 bash -c "
      echo \"test\" | wscat -c \"ws://localhost:8080/api/v1/agents/connect\" \
        -H \"Authorization: Bearer '"$CONTROLLER_TOKEN"'\" --wait 5 > /dev/null 2>&1
    "
  ' 15
}

# Integration tests
test_integration() {
  log_info "🔗 Testing Integration Scenarios"
  
  # Test 1: Ghost ship registration with controller
  run_test "ghost_ship_registration" '
    response=$(curl -s -H "Authorization: Bearer '"$ADMIN_TOKEN"'" "'"$LOCAL_CONTROLLER_URL"'/api/v1/ghostships")
    echo "$response" | jq -e ".ghost_ships | length > 0" > /dev/null
  ' 15
  
  # Test 2: File archival workflow
  run_test "file_archival_workflow" '
    # Create test file
    mkdir -p '"$PROJECT_ROOT"'/docker/development/test-data/nas-1/documents
    echo "Test document for archival" > '"$PROJECT_ROOT"'/docker/development/test-data/nas-1/documents/test-$(date +%s).txt
    
    # Wait for archival (ghost ship scans every 30s in dev)
    sleep 35
    
    # Check if file appears in LocalStack S3
    aws --endpoint-url=http://localhost:4566 s3 ls s3://cargoship-dev-bucket-1/ | grep -q "test-"
  ' 120
  
  # Test 3: Job assignment via controller
  run_test "controller_job_assignment" '
    job_id="test-job-$(date +%s)"
    response=$(curl -s -X POST \
      -H "Authorization: Bearer '"$ADMIN_TOKEN"'" \
      -H "Content-Type: application/json" \
      -d "{
        \"job_id\": \"$job_id\",
        \"type\": \"archive\",
        \"path\": \"/data/public/documents/*.txt\",
        \"destination\": \"controller-jobs/\",
        \"storage_class\": \"STANDARD\"
      }" \
      "'"$LOCAL_CONTROLLER_URL"'/api/v1/ghostships/dev-ghost-ship-1/assign")
    
    echo "$response" | jq -e ".success == true" > /dev/null
  ' 20
  
  # Test 4: Metrics collection
  run_test "metrics_collection" '
    # Check controller metrics
    curl -s "'"$LOCAL_CONTROLLER_URL"'/metrics" | grep -q "cargoship_"
    
    # Check ghost ship metrics
    curl -s "'"$LOCAL_GHOST_SHIP_1_URL"'/metrics" | grep -q "cargoship_"
  ' 15
}

# Performance tests
test_performance() {
  log_info "⚡ Testing Performance Scenarios"
  
  # Test 1: Concurrent file processing
  run_test "concurrent_file_processing" '
    # Generate multiple test files
    for i in {1..20}; do
      echo "Concurrent test file $i" > '"$PROJECT_ROOT"'/docker/development/test-data/nas-1/documents/concurrent_$i.txt
    done
    
    # Wait for processing
    sleep 60
    
    # Check processing rate
    processed_count=$(aws --endpoint-url=http://localhost:4566 s3 ls s3://cargoship-dev-bucket-1/ | grep "concurrent_" | wc -l)
    test "$processed_count" -ge 15  # At least 75% processed
  ' 180
  
  # Test 2: Large file handling
  run_test "large_file_handling" '
    # Create a larger test file (10MB)
    dd if=/dev/zero of='"$PROJECT_ROOT"'/docker/development/test-data/nas-1/documents/large_file.bin bs=1M count=10 2>/dev/null
    
    # Wait for archival
    sleep 90
    
    # Verify upload
    aws --endpoint-url=http://localhost:4566 s3 ls s3://cargoship-dev-bucket-1/ | grep -q "large_file.bin"
  ' 300
  
  # Test 3: Memory usage under load
  run_test "memory_usage_check" '
    # Get memory usage of ghost ship containers
    memory_usage=$(docker stats --no-stream --format "{{.MemUsage}}" cargoship-ghost-ship-1 | cut -d"/" -f1 | sed "s/MiB//" | sed "s/GiB/*1000/" | bc 2>/dev/null || echo "0")
    
    # Check if memory usage is reasonable (less than 1GB)
    test "${memory_usage%.*}" -lt 1000
  ' 10
}

# astrapi.local tests
test_astrapi_environment() {
  log_info "🏠 Testing astrapi.local Environment"
  
  # Update test report environment
  local temp_file=$(mktemp)
  jq '.test_run.environment = "astrapi_production"' "$TEST_REPORT" > "$temp_file"
  mv "$temp_file" "$TEST_REPORT"
  
  # Test 1: astrapi connectivity
  run_test "astrapi_connectivity" '
    ping -c 3 '"$ASTRAPI_HOST"' > /dev/null 2>&1
  ' 15
  
  # Test 2: SSH connectivity
  run_test "astrapi_ssh_connectivity" '
    ssh -o ConnectTimeout=10 '"$ASTRAPI_USER"'@'"$ASTRAPI_HOST"' "echo \"SSH connected\""
  ' 20
  
  # Test 3: Ghost ship container status
  run_test "astrapi_ghost_ship_status" '
    ssh '"$ASTRAPI_USER"'@'"$ASTRAPI_HOST"' "docker ps | grep -q cargoship-ghost"
  ' 15
  
  # Test 4: Real S3 connectivity from astrapi
  run_test "astrapi_s3_connectivity" '
    ssh '"$ASTRAPI_USER"'@'"$ASTRAPI_HOST"' "
      docker exec \$(docker ps --filter name=cargoship-ghost --format \"{{.Names}}\" | head -1) \
        aws s3 ls > /dev/null 2>&1
    "
  ' 30
  
  # Test 5: File system access
  run_test "astrapi_filesystem_access" '
    ssh '"$ASTRAPI_USER"'@'"$ASTRAPI_HOST"' "
      docker exec \$(docker ps --filter name=cargoship-ghost --format \"{{.Names}}\" | head -1) \
        ls -la /data/public/ | grep -q \"drwx\"
    "
  ' 15
  
  # Test 6: Performance test - network throughput
  run_test "astrapi_network_performance" '
    ssh '"$ASTRAPI_USER"'@'"$ASTRAPI_HOST"' "
      docker exec \$(docker ps --filter name=cargoship-ghost --format \"{{.Names}}\" | head -1) \
        /usr/local/bin/cargoship-test --test-type=bandwidth --duration=30s --target-throughput=100mbps
    "
  ' 60
}

# Test data generation
generate_test_data() {
  log_info "📁 Generating Test Data"
  
  local test_data_dir="$PROJECT_ROOT/docker/development/test-data"
  
  # Create directory structure
  mkdir -p "$test_data_dir"/nas-{1,2}/{documents,images,backups,videos}
  
  # Generate document files
  for i in {1..5}; do
    echo "Sample document $i - $(date)" > "$test_data_dir/nas-1/documents/doc_$i.txt"
    echo "Sample PDF content $i" > "$test_data_dir/nas-1/documents/sample_$i.pdf"
  done
  
  # Generate image files (small binary files)
  for i in {1..3}; do
    dd if=/dev/urandom of="$test_data_dir/nas-1/images/photo_$i.jpg" bs=1K count=100 2>/dev/null
  done
  
  # Generate backup files for nas-2
  for i in {1..3}; do
    tar -czf "$test_data_dir/nas-2/backups/backup_$i.tar.gz" -C /tmp . 2>/dev/null || true
  done
  
  log_success "Test data generated successfully"
}

# Cleanup function
cleanup() {
  log_info "🧹 Cleaning up test environment"
  
  # Remove generated test files
  rm -f "$PROJECT_ROOT"/docker/development/test-data/nas-*/documents/test-*.txt
  rm -f "$PROJECT_ROOT"/docker/development/test-data/nas-*/documents/concurrent_*.txt
  rm -f "$PROJECT_ROOT"/docker/development/test-data/nas-*/documents/large_file.bin
}

# Generate final test report
generate_final_report() {
  local temp_file=$(mktemp)
  jq --arg passed "$TESTS_PASSED" \
     --arg failed "$TESTS_FAILED" \
     --arg skipped "$TESTS_SKIPPED" \
     --arg total "$((TESTS_PASSED + TESTS_FAILED + TESTS_SKIPPED))" \
     '.test_run.summary = {
       "total_tests": ($total | tonumber),
       "passed": ($passed | tonumber),
       "failed": ($failed | tonumber), 
       "skipped": ($skipped | tonumber),
       "success_rate": (($passed | tonumber) / ($total | tonumber) * 100 | floor)
     }' "$TEST_REPORT" > "$temp_file"
  mv "$temp_file" "$TEST_REPORT"
  
  log_info "📊 Test Summary:"
  log_info "  Total Tests: $((TESTS_PASSED + TESTS_FAILED + TESTS_SKIPPED))"
  log_success "  Passed: $TESTS_PASSED"
  log_error "  Failed: $TESTS_FAILED"
  log_warning "  Skipped: $TESTS_SKIPPED"
  
  if [ $TESTS_FAILED -eq 0 ]; then
    log_success "🎉 All tests passed!"
  else
    log_error "❌ Some tests failed. Check the detailed report: $TEST_REPORT"
  fi
  
  echo
  log_info "📋 Detailed test report: $TEST_REPORT"
  log_info "📊 View results: jq '.test_run.summary' $TEST_REPORT"
}

# Show usage
show_usage() {
  cat << EOF
CargoShip Test Suite

Usage: $0 [OPTIONS] [TEST_SUITE]

Test Suites:
    local       Run local development environment tests (default)
    integration Run integration tests
    performance Run performance tests  
    astrapi     Run astrapi.local environment tests
    all         Run all test suites

Options:
    -h, --help              Show this help message
    -c, --cleanup           Clean up test environment after running
    -g, --generate-data     Generate test data before running tests
    -r, --report-only       Only generate report from existing results
    -v, --verbose           Enable verbose output

Environment Variables:
    ASTRAPI_HOST            astrapi hostname (default: astrapi.local)
    ASTRAPI_USER            astrapi SSH user (default: admin)
    CARGOSHIP_CONTROLLER_TOKEN  Controller API token
    CARGOSHIP_ADMIN_TOKEN       Admin API token

Examples:
    $0 local                        # Run local tests
    $0 --generate-data local        # Generate test data and run local tests
    $0 astrapi                      # Run astrapi tests
    $0 --cleanup all                # Run all tests and cleanup
EOF
}

# Parse command line arguments
TEST_SUITE="local"
CLEANUP_AFTER=false
GENERATE_DATA=false
REPORT_ONLY=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
  case $1 in
    -h|--help)
      show_usage
      exit 0
      ;;
    -c|--cleanup)
      CLEANUP_AFTER=true
      shift
      ;;
    -g|--generate-data)
      GENERATE_DATA=true
      shift
      ;;
    -r|--report-only)
      REPORT_ONLY=true
      shift
      ;;
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    local|integration|performance|astrapi|all)
      TEST_SUITE="$1"
      shift
      ;;
    *)
      log_error "Unknown option: $1"
      show_usage
      exit 1
      ;;
  esac
done

# Enable verbose output if requested
if [[ "$VERBOSE" == "true" ]]; then
  set -x
fi

# Show test banner
echo -e "${PURPLE}"
echo "🚢 CargoShip Test Suite"
echo "======================"
echo -e "${NC}"
echo "Test Suite: $TEST_SUITE"
echo "Results Dir: $TEST_RESULTS_DIR"
echo "Report File: $TEST_REPORT"
echo

# Generate test data if requested
if [[ "$GENERATE_DATA" == "true" ]] && [[ "$REPORT_ONLY" == "false" ]]; then
  generate_test_data
fi

# Run tests unless report-only mode
if [[ "$REPORT_ONLY" == "false" ]]; then
  case "$TEST_SUITE" in
    local)
      test_local_environment
      ;;
    integration)
      test_integration
      ;;
    performance)
      test_performance
      ;;
    astrapi)
      test_astrapi_environment
      ;;
    all)
      test_local_environment
      test_integration
      test_performance
      test_astrapi_environment
      ;;
  esac
fi

# Cleanup if requested
if [[ "$CLEANUP_AFTER" == "true" ]]; then
  cleanup
fi

# Generate final report
generate_final_report

# Exit with appropriate code
if [ $TESTS_FAILED -eq 0 ]; then
  exit 0
else
  exit 1
fi