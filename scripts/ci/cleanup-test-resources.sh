#!/bin/bash
# scripts/ci/cleanup-test-resources.sh
# Cleans up old test buckets to prevent resource leaks

set -euo pipefail

# Default values
DRY_RUN=false
MAX_AGE_HOURS=24
VERBOSE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Usage function
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Clean up old CargoShip test buckets from AWS S3.

OPTIONS:
    --dry-run               Show what would be deleted without deleting
    --max-age-hours HOURS   Maximum age in hours (default: 24)
    --verbose               Enable verbose output
    -h, --help              Show this help message

EXAMPLES:
    # Dry run to see what would be deleted
    $0 --dry-run

    # Delete buckets older than 48 hours
    $0 --max-age-hours 48

    # Delete buckets older than 24 hours (default)
    $0
EOF
    exit 0
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --max-age-hours)
            MAX_AGE_HOURS="$2"
            shift 2
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_verbose() {
    if [ "${VERBOSE}" == "true" ]; then
        echo -e "[DEBUG] $*"
    fi
}

# Find old test buckets
find_old_buckets() {
    local cutoff_timestamp=$(($(date +%s) - (MAX_AGE_HOURS * 3600)))

    log_info "Searching for test buckets older than ${MAX_AGE_HOURS} hours..."
    log_verbose "Cutoff timestamp: ${cutoff_timestamp}"

    # List all buckets with cargoship-ci-integration prefix
    aws s3api list-buckets --query 'Buckets[?starts_with(Name, `cargoship-ci-integration-`)].Name' \
        --output text | tr '\t' '\n' | while read -r bucket; do

        if [ -z "${bucket}" ]; then
            continue
        fi

        log_verbose "Checking bucket: ${bucket}"

        # Get bucket tags
        timestamp=$(aws s3api get-bucket-tagging --bucket "${bucket}" \
            --query 'TagSet[?Key==`Timestamp`].Value' \
            --output text 2>/dev/null || echo "0")

        log_verbose "  Bucket timestamp: ${timestamp}"

        if [ "${timestamp}" == "None" ] || [ "${timestamp}" == "" ]; then
            # No timestamp tag, try to parse from bucket name
            # Format: cargoship-ci-integration-{timestamp}
            timestamp=$(echo "${bucket}" | grep -oP 'cargoship-ci-integration-\K\d+' || echo "0")
            log_verbose "  Parsed timestamp from name: ${timestamp}"
        fi

        if [ "${timestamp}" -lt "${cutoff_timestamp}" ] && [ "${timestamp}" != "0" ]; then
            age_hours=$(( ($(date +%s) - timestamp) / 3600 ))
            log_verbose "  Age: ${age_hours} hours (WILL DELETE)"
            echo "${bucket}:${timestamp}:${age_hours}"
        else
            log_verbose "  Too recent, skipping"
        fi
    done
}

# Delete bucket and all contents
delete_bucket() {
    local bucket=$1
    local timestamp=$2
    local age_hours=$3

    log_info "Deleting bucket: ${bucket} (age: ${age_hours} hours)"

    if [ "${DRY_RUN}" == "true" ]; then
        log_warn "[DRY RUN] Would delete bucket: ${bucket}"
        return 0
    fi

    # Delete all objects
    log_verbose "  Deleting all objects..."
    if ! aws s3 rm "s3://${bucket}" --recursive 2>&1; then
        log_error "  Failed to delete objects from ${bucket}"
        return 1
    fi

    # Check for versioning and delete versions if needed
    local versioning
    versioning=$(aws s3api get-bucket-versioning --bucket "${bucket}" \
        --query 'Status' --output text 2>/dev/null || echo "Disabled")

    if [ "${versioning}" == "Enabled" ]; then
        log_verbose "  Deleting object versions..."
        aws s3api list-object-versions --bucket "${bucket}" --output json | \
            jq -r '.Versions[]? | "--key '\''\(.Key)'\'' --version-id '\''\(.VersionId)'\''"' | \
            xargs -I {} -n 4 aws s3api delete-object --bucket "${bucket}" {} 2>/dev/null || true
    fi

    # Delete bucket
    log_verbose "  Deleting bucket..."
    if ! aws s3 rb "s3://${bucket}" --force 2>&1; then
        log_error "  Failed to delete bucket: ${bucket}"
        return 1
    fi

    log_info "  ✅ Deleted: ${bucket}"
    return 0
}

# Main function
main() {
    log_info "🧹 CargoShip Test Resource Cleanup"
    log_info "===================================="
    log_info "Mode: ${DRY_RUN:+DRY RUN}${DRY_RUN:-LIVE}"
    log_info "Max age: ${MAX_AGE_HOURS} hours"
    log_info ""

    # Check AWS CLI is installed
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI is not installed"
        exit 1
    fi

    # Check AWS credentials are configured
    if ! aws sts get-caller-identity &> /dev/null; then
        log_error "AWS credentials are not configured"
        exit 1
    fi

    local deleted_count=0
    local failed_count=0

    # Find and delete old buckets
    while IFS=':' read -r bucket timestamp age_hours; do
        if [ -z "${bucket}" ]; then
            continue
        fi

        if delete_bucket "${bucket}" "${timestamp}" "${age_hours}"; then
            ((deleted_count++))
        else
            ((failed_count++))
        fi
    done < <(find_old_buckets)

    log_info ""
    log_info "===================================="
    log_info "Cleanup Summary:"
    if [ "${DRY_RUN}" == "true" ]; then
        log_info "  Would delete: ${deleted_count} bucket(s)"
    else
        log_info "  Deleted: ${deleted_count} bucket(s)"
    fi

    if [ "${failed_count}" -gt 0 ]; then
        log_warn "  Failed: ${failed_count} bucket(s)"
        exit 1
    fi

    log_info "✅ Cleanup complete"
}

main "$@"
