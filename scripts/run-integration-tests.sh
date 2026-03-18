#!/bin/bash

# CargoShip Integration Test Runner — uses in-process Substrate emulator.
# No Docker required; Substrate starts inside the test process.

set -e

TEST_TIMEOUT="300s"

echo "CargoShip Integration Test Runner"
echo "=================================="

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

cd "$(dirname "$0")/.."

# Run integration tests (Substrate starts in-process automatically)
if go test -tags=integration -timeout=$TEST_TIMEOUT -v ./pkg/aws/s3/... -run Integration "$@"; then
    echo ""
    echo "All integration tests passed!"

    # Run benchmarks if requested
    if [ "$1" = "--bench" ]; then
        echo ""
        echo "Running benchmarks..."
        go test -tags=integration -timeout=$TEST_TIMEOUT -bench=BenchmarkIntegration -benchmem ./pkg/aws/s3/...
    fi

    exit 0
else
    echo ""
    echo "Integration tests failed!"
    exit 1
fi
