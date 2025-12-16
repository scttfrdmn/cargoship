#!/bin/bash
# scripts/ci/post-test-summary.sh
# Aggregates test results from multiple workflows and posts comprehensive summary to PRs

set -euo pipefail

# Default values
PR_NUMBER=""
UNIT_RESULTS=""
INTEGRATION_RESULTS=""
BENCHMARK_RESULTS=""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Usage function
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Aggregate test results and post summary to GitHub PR.

OPTIONS:
    --pr NUMBER               PR number (required)
    --unit-results FILE       Path to unit test JSON results
    --integration-results FILE Path to integration test JSON results
    --benchmark-results FILE   Path to benchmark comparison results
    -h, --help                Show this help message

EXAMPLES:
    $0 --pr 123 --unit-results unit.json --integration-results integration.json

ENVIRONMENT:
    GITHUB_TOKEN              Required for posting PR comments
EOF
    exit 0
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --pr)
            PR_NUMBER="$2"
            shift 2
            ;;
        --unit-results)
            UNIT_RESULTS="$2"
            shift 2
            ;;
        --integration-results)
            INTEGRATION_RESULTS="$2"
            shift 2
            ;;
        --benchmark-results)
            BENCHMARK_RESULTS="$2"
            shift 2
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

# Validate required arguments
if [ -z "${PR_NUMBER}" ]; then
    echo "Error: --pr is required"
    usage
fi

if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "Error: GITHUB_TOKEN environment variable is required"
    exit 1
fi

# Parse test results from JSON
parse_test_results() {
    local test_file=$1

    if [ ! -f "${test_file}" ]; then
        echo "0:0:0:0"
        return
    fi

    # Extract test counts
    local total
    local passed
    local failed
    local skipped

    total=$(jq -r 'select(.Action=="pass" or .Action=="fail" or .Action=="skip") | select(.Test != null) | .Test' "${test_file}" | sort -u | wc -l | tr -d ' ')
    passed=$(jq -r 'select(.Action=="pass") | select(.Test != null) | .Test' "${test_file}" | sort -u | wc -l | tr -d ' ')
    failed=$(jq -r 'select(.Action=="fail") | select(.Test != null) | .Test' "${test_file}" | sort -u | wc -l | tr -d ' ')
    skipped=$(jq -r 'select(.Action=="skip") | select(.Test != null) | .Test' "${test_file}" | sort -u | wc -l | tr -d ' ')

    echo "${total}:${passed}:${failed}:${skipped}"
}

# Generate status icon
status_icon() {
    local passed=$1
    local failed=$2

    if [ "${failed}" -eq 0 ]; then
        echo "✅"
    else
        echo "❌"
    fi
}

# Generate markdown summary
generate_summary() {
    local summary="## 🧪 CargoShip Test Results\n\n"

    # Unit tests
    if [ -n "${UNIT_RESULTS}" ] && [ -f "${UNIT_RESULTS}" ]; then
        IFS=':' read -r total passed failed skipped <<< "$(parse_test_results "${UNIT_RESULTS}")"
        local icon=$(status_icon "${passed}" "${failed}")

        summary+="### ${icon} Unit Tests\n"
        summary+="- **Total**: ${total}\n"
        summary+="- **Passed**: ${passed}\n"
        summary+="- **Failed**: ${failed}\n"
        summary+="- **Skipped**: ${skipped}\n"
        summary+="\n"
    fi

    # Integration tests
    if [ -n "${INTEGRATION_RESULTS}" ] && [ -f "${INTEGRATION_RESULTS}" ]; then
        IFS=':' read -r total passed failed skipped <<< "$(parse_test_results "${INTEGRATION_RESULTS}")"
        local icon=$(status_icon "${passed}" "${failed}")

        summary+="### ${icon} Integration Tests (LocalStack)\n"
        summary+="- **Total**: ${total}\n"
        summary+="- **Passed**: ${passed}\n"
        summary+="- **Failed**: ${failed}\n"
        summary+="- **Skipped**: ${skipped}\n"
        summary+="\n"
    fi

    # Benchmarks
    if [ -n "${BENCHMARK_RESULTS}" ] && [ -f "${BENCHMARK_RESULTS}" ]; then
        summary+="### 📊 Benchmarks\n"

        # Check for regressions
        if grep -q "slower" "${BENCHMARK_RESULTS}"; then
            summary+="⚠️ **Performance changes detected**\n\n"
            summary+="<details>\n"
            summary+="<summary>View Benchmark Comparison</summary>\n\n"
            summary+="\`\`\`\n"
            summary+="$(cat "${BENCHMARK_RESULTS}")\n"
            summary+="\`\`\`\n\n"
            summary+="</details>\n"
        else
            summary+="✅ **No significant performance regression**\n"
        fi
        summary+="\n"
    fi

    summary+="---\n"
    summary+="📊 [View detailed results](${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID})\n"

    echo -e "${summary}"
}

# Post to PR using GitHub CLI
post_to_pr() {
    local summary=$1
    local pr_number=$2

    echo "Posting test summary to PR #${pr_number}..."

    # Create or update comment using gh CLI
    gh pr comment "${pr_number}" --body "${summary}" || {
        echo "Warning: Failed to post comment to PR"
        return 1
    }

    echo "✅ Test summary posted successfully"
}

# Main function
main() {
    echo "Aggregating test results for PR #${PR_NUMBER}..."

    # Generate summary
    local summary
    summary=$(generate_summary)

    # Post to PR
    if ! post_to_pr "${summary}" "${PR_NUMBER}"; then
        echo "Failed to post test summary"
        exit 1
    fi

    echo "Done"
}

main "$@"
