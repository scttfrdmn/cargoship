#!/bin/bash
# CargoShip Integration Test Script
# Tests the complete launch capability with LocalStack S3

set -euo pipefail

# Colors for output
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
TEST_RESULTS_DIR="$PROJECT_ROOT/test-results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="$TEST_RESULTS_DIR/integration-test-$TIMESTAMP.json"

# Test configuration
LOCALSTACK_URL="http://localhost:4566"
CONTROLLER_URL="http://localhost:8080"
S3_BUCKET="cargoship-test-$(date +%s)"
TEST_DATA_DIR="$PROJECT_ROOT/docker/development/test-data"

echo -e "${BLUE}"
echo "🚢 CargoShip Integration Test Suite"
echo "=================================="
echo "Timestamp: $(date)"
echo "LocalStack: $LOCALSTACK_URL"
echo "Controller: $CONTROLLER_URL"
echo "Test Bucket: $S3_BUCKET"
echo -e "${NC}"

# Create results directory
mkdir -p "$TEST_RESULTS_DIR"

# Initialize test report
cat > "$REPORT_FILE" << EOF
{
  "test_suite": "integration",
  "timestamp": "$(date -Iseconds)",
  "environment": "localstack",
  "bucket": "$S3_BUCKET",
  "tests": []
}
EOF

# Test counter
TEST_COUNT=0
PASSED_COUNT=0
FAILED_COUNT=0

# Function to record test result
record_test() {
  local test_name="$1"
  local status="$2"
  local duration="$3"
  local details="$4"
  
  TEST_COUNT=$((TEST_COUNT + 1))
  
  if [[ "$status" == "PASSED" ]]; then
    PASSED_COUNT=$((PASSED_COUNT + 1))
    log_success "$test_name - PASSED (${duration}s)"
  else
    FAILED_COUNT=$((FAILED_COUNT + 1))
    log_error "$test_name - FAILED (${duration}s)"
    echo "  Details: $details"
  fi
  
  # Update JSON report
  local temp_file=$(mktemp)
  jq --arg name "$test_name" --arg status "$status" --arg duration "$duration" --arg details "$details" \
    '.tests += [{name: $name, status: $status, duration: ($duration | tonumber), details: $details}]' \
    "$REPORT_FILE" > "$temp_file" && mv "$temp_file" "$REPORT_FILE"
}

# Test 1: LocalStack S3 Service
test_localstack_s3() {
  log_info "Testing LocalStack S3 service..."
  local start_time=$(date +%s)
  
  if curl -sf "$LOCALSTACK_URL/_localstack/health" | jq -e '.services.s3 == "running"' > /dev/null; then
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    record_test "localstack_s3_service" "PASSED" "$duration" "S3 service is running"
  else
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    record_test "localstack_s3_service" "FAILED" "$duration" "S3 service not running"
    return 1
  fi
}

# Test 2: Controller Health
test_controller_health() {
  log_info "Testing controller health..."
  local start_time=$(date +%s)
  
  if curl -sf "$CONTROLLER_URL/health" > /dev/null 2>&1; then
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    record_test "controller_health" "PASSED" "$duration" "Controller is responding"
  else
    local end_time=$(date +%s) 
    local duration=$((end_time - start_time))
    record_test "controller_health" "FAILED" "$duration" "Controller not responding"
    return 1
  fi
}

# Test 3: S3 Bucket Operations
test_s3_bucket_operations() {
  log_info "Testing S3 bucket operations..."
  local start_time=$(date +%s)
  
  # Configure AWS CLI for LocalStack
  export AWS_ACCESS_KEY_ID=test
  export AWS_SECRET_ACCESS_KEY=test
  export AWS_DEFAULT_REGION=us-west-2
  export AWS_ENDPOINT_URL=$LOCALSTACK_URL
  
  # Create test bucket
  if aws s3 mb "s3://$S3_BUCKET" --endpoint-url=$LOCALSTACK_URL 2>/dev/null; then
    # Test upload
    echo "Test file content" > /tmp/test-file.txt
    if aws s3 cp /tmp/test-file.txt "s3://$S3_BUCKET/test-file.txt" --endpoint-url=$LOCALSTACK_URL 2>/dev/null; then
      # Test download
      if aws s3 cp "s3://$S3_BUCKET/test-file.txt" /tmp/downloaded-file.txt --endpoint-url=$LOCALSTACK_URL 2>/dev/null; then
        if [[ "$(cat /tmp/downloaded-file.txt)" == "Test file content" ]]; then
          local end_time=$(date +%s)
          local duration=$((end_time - start_time))
          record_test "s3_bucket_operations" "PASSED" "$duration" "Bucket create/upload/download successful"
        else
          local end_time=$(date +%s)
          local duration=$((end_time - start_time))
          record_test "s3_bucket_operations" "FAILED" "$duration" "File content mismatch"
          return 1
        fi
      else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        record_test "s3_bucket_operations" "FAILED" "$duration" "Download failed"
        return 1
      fi
    else
      local end_time=$(date +%s)
      local duration=$((end_time - start_time))
      record_test "s3_bucket_operations" "FAILED" "$duration" "Upload failed"
      return 1
    fi
  else
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    record_test "s3_bucket_operations" "FAILED" "$duration" "Bucket creation failed"
    return 1
  fi
  
  # Cleanup
  rm -f /tmp/test-file.txt /tmp/downloaded-file.txt
}

# Test 4: Docker Services Status
test_docker_services() {
  log_info "Testing Docker services status..."
  local start_time=$(date +%s)
  
  cd "$PROJECT_ROOT/docker/development" 2>/dev/null || cd "$PROJECT_ROOT"
  
  local services=("cargoship-controller-dev" "cargoship-localstack" "cargoship-grafana")
  local all_running=true
  local status_details=""
  
  for service in "${services[@]}"; do
    if docker-compose ps "$service" 2>/dev/null | grep -q "Up"; then
      status_details="$status_details $service:UP"
    else
      status_details="$status_details $service:DOWN"
      all_running=false
    fi
  done
  
  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  
  if $all_running; then
    record_test "docker_services_status" "PASSED" "$duration" "All services running:$status_details"
  else
    record_test "docker_services_status" "FAILED" "$duration" "Some services down:$status_details"
    return 1
  fi
  
  cd "$PROJECT_ROOT"
}

# Test 5: Test Data Generation
test_data_generation() {
  log_info "Testing test data availability..."
  local start_time=$(date +%s)
  
  local required_files=(
    "$TEST_DATA_DIR/nas-1/documents/doc_1.txt"
    "$TEST_DATA_DIR/nas-1/images/photo_1.jpg"
    "$TEST_DATA_DIR/nas-2/backups/backup_1.tar.gz"
  )
  
  local missing_files=()
  for file in "${required_files[@]}"; do
    if [[ ! -f "$file" ]]; then
      missing_files+=("$file")
    fi
  done
  
  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  
  if [[ ${#missing_files[@]} -eq 0 ]]; then
    record_test "test_data_generation" "PASSED" "$duration" "All test files present"
  else
    record_test "test_data_generation" "FAILED" "$duration" "Missing files: ${missing_files[*]}"
    return 1
  fi
}

# Test 6: Monitoring Endpoints
test_monitoring_endpoints() {
  log_info "Testing monitoring endpoints..."
  local start_time=$(date +%s)
  
  local endpoints=(
    "http://localhost:3000:grafana"
    "http://localhost:9090:prometheus"
  )
  
  local failed_endpoints=()
  for endpoint_info in "${endpoints[@]}"; do
    local endpoint="${endpoint_info%:*}"
    local service="${endpoint_info##*:}"
    
    if curl -sf "$endpoint" > /dev/null 2>&1; then
      log_info "  ✅ $service endpoint accessible"
    else
      failed_endpoints+=("$service")
    fi
  done
  
  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  
  if [[ ${#failed_endpoints[@]} -eq 0 ]]; then
    record_test "monitoring_endpoints" "PASSED" "$duration" "All monitoring endpoints accessible"
  else
    record_test "monitoring_endpoints" "FAILED" "$duration" "Failed endpoints: ${failed_endpoints[*]}"
    return 1
  fi
}

# Run all tests
run_all_tests() {
  log_info "Starting integration test suite..."
  
  test_localstack_s3 || true
  test_controller_health || true
  test_s3_bucket_operations || true
  test_docker_services || true
  test_data_generation || true
  test_monitoring_endpoints || true
}

# Generate final report
generate_report() {
  log_info "Generating final test report..."
  
  # Update summary in JSON
  local temp_file=$(mktemp)
  jq --arg total "$TEST_COUNT" --arg passed "$PASSED_COUNT" --arg failed "$FAILED_COUNT" \
    '. + {summary: {total: ($total | tonumber), passed: ($passed | tonumber), failed: ($failed | tonumber)}}' \
    "$REPORT_FILE" > "$temp_file" && mv "$temp_file" "$REPORT_FILE"
  
  echo
  echo -e "${BLUE}📊 Integration Test Results${NC}"
  echo "=========================="
  echo "Total Tests: $TEST_COUNT"
  echo "Passed: $PASSED_COUNT"
  echo "Failed: $FAILED_COUNT"
  echo "Success Rate: $(( PASSED_COUNT * 100 / TEST_COUNT ))%"
  echo
  echo "Report saved to: $REPORT_FILE"
  
  if [[ $FAILED_COUNT -eq 0 ]]; then
    log_success "🎉 All integration tests passed!"
    echo
    log_info "Next steps:"
    echo "  1. Deploy to astrapi.local for real AWS testing"
    echo "  2. Run performance benchmarks"
    echo "  3. Test fault tolerance scenarios"
    return 0
  else
    log_error "❌ $FAILED_COUNT tests failed. Check the report for details."
    return 1
  fi
}

# Cleanup function
cleanup() {
  log_info "Cleaning up test resources..."
  
  if [[ -n "${S3_BUCKET:-}" ]]; then
    aws s3 rb "s3://$S3_BUCKET" --force --endpoint-url=$LOCALSTACK_URL 2>/dev/null || true
  fi
  
  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_DEFAULT_REGION AWS_ENDPOINT_URL
}

# Main execution
main() {
  trap cleanup EXIT
  
  run_all_tests
  generate_report
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi