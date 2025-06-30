#!/bin/bash

# CargoShip Integration Test Runner with LocalStack
# This script starts LocalStack and runs integration tests

set -e

LOCALSTACK_CONTAINER="cargoship-localstack"
LOCALSTACK_PORT="4566"
TEST_TIMEOUT="300s"

echo "🚢 CargoShip Integration Test Runner"
echo "======================================"

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    docker stop $LOCALSTACK_CONTAINER 2>/dev/null || true
    docker rm $LOCALSTACK_CONTAINER 2>/dev/null || true
}

# Set trap to cleanup on script exit
trap cleanup EXIT

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if LocalStack container is already running
if docker ps | grep -q $LOCALSTACK_CONTAINER; then
    echo "🔄 Stopping existing LocalStack container..."
    docker stop $LOCALSTACK_CONTAINER
    docker rm $LOCALSTACK_CONTAINER
fi

echo "🐳 Starting LocalStack container..."
docker run --rm -d \
    --name $LOCALSTACK_CONTAINER \
    -p $LOCALSTACK_PORT:4566 \
    -e DEBUG=1 \
    -e SERVICES=s3,cloudwatch \
    -e DATA_DIR=/tmp/localstack/data \
    -e DOCKER_HOST=unix:///var/run/docker.sock \
    -v /var/run/docker.sock:/var/run/docker.sock \
    localstack/localstack:latest

echo "⏳ Waiting for LocalStack to be ready..."

# Wait for LocalStack to be ready
MAX_WAIT=60
WAIT_COUNT=0
while ! curl -s http://localhost:$LOCALSTACK_PORT/_localstack/health >/dev/null; do
    if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
        echo "❌ LocalStack failed to start within ${MAX_WAIT} seconds"
        exit 1
    fi
    echo "   Waiting... ($((WAIT_COUNT + 1))/${MAX_WAIT})"
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

echo "✅ LocalStack is ready!"

# Show LocalStack status
echo ""
echo "📊 LocalStack Status:"
curl -s http://localhost:$LOCALSTACK_PORT/_localstack/health | jq . || echo "Status check failed"

echo ""
echo "🧪 Running integration tests..."
echo "   Test timeout: $TEST_TIMEOUT"
echo ""

# Run integration tests
cd "$(dirname "$0")/.."

# Set environment variables for tests
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

# Run tests with integration tag
if go test -tags=integration -timeout=$TEST_TIMEOUT -v ./pkg/aws/s3/... -run Integration; then
    echo ""
    echo "✅ All integration tests passed!"
    
    # Run benchmarks if requested
    if [ "$1" = "--bench" ]; then
        echo ""
        echo "🏃 Running benchmarks..."
        go test -tags=integration -timeout=$TEST_TIMEOUT -bench=BenchmarkIntegration -benchmem ./pkg/aws/s3/...
    fi
    
    exit 0
else
    echo ""
    echo "❌ Integration tests failed!"
    exit 1
fi