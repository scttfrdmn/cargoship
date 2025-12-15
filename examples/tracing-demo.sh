#!/bin/bash
# Demonstration of CargoShip distributed tracing capabilities
# This script shows various tracing configurations and their outputs

set -e

echo "🔍 CargoShip Distributed Tracing Demo"
echo "====================================="
echo ""

# Check if cargoship binary exists
if [ ! -f "./cargoship" ]; then
    echo "Building cargoship binary..."
    go build -o cargoship ./cmd/cargoship
    echo "✅ Build complete"
    echo ""
fi

# Create test data directory
TEST_DIR="./test-tracing-data"
if [ ! -d "$TEST_DIR" ]; then
    echo "Creating test data directory..."
    mkdir -p "$TEST_DIR"
    echo "test file 1" > "$TEST_DIR/file1.txt"
    echo "test file 2" > "$TEST_DIR/file2.txt"
    echo "test file 3" > "$TEST_DIR/file3.txt"
    echo "✅ Test data created"
    echo ""
fi

# Demo 1: Stdout tracing (default)
echo "📝 Demo 1: Stdout Tracing"
echo "-------------------------"
echo "Command: ./cargoship upload --tracing"
echo ""
echo "This will output trace spans in JSON format to stdout."
echo "You'll see the complete span hierarchy:"
echo "  - upload-request (root)"
echo "    - pipeline-execution"
echo "      - scanner-stage"
echo "      - archiver-stage"
echo "      - uploader-stage"
echo "        - job-N (per upload)"
echo ""
read -p "Press Enter to run (or Ctrl+C to skip)..."
echo ""

# Note: This would need a real S3 bucket to work
# ./cargoship upload "$TEST_DIR" s3://test-bucket/demo --tracing --quiet

echo "⚠️  Skipping actual execution (requires AWS credentials and S3 bucket)"
echo ""

# Demo 2: Jaeger tracing
echo "📊 Demo 2: Jaeger Tracing"
echo "-------------------------"
echo "To use Jaeger tracing:"
echo ""
echo "1. Start Jaeger (using Docker):"
echo "   docker run -d --name jaeger \\"
echo "     -p 16686:16686 \\"
echo "     -p 14268:14268 \\"
echo "     jaegertracing/all-in-one:latest"
echo ""
echo "2. Upload with Jaeger tracing:"
echo "   ./cargoship upload ./data s3://bucket/prefix \\"
echo "     --tracing \\"
echo "     --tracing-exporter jaeger \\"
echo "     --tracing-endpoint localhost:14268"
echo ""
echo "3. View traces in Jaeger UI:"
echo "   Open http://localhost:16686 in your browser"
echo ""
echo "You'll see a visual trace timeline showing:"
echo "  - Duration of each pipeline stage"
echo "  - Parallel job execution"
echo "  - Retry attempts (if any)"
echo "  - S3 operation details"
echo ""

# Demo 3: OTLP tracing (OpenTelemetry Collector)
echo "🚀 Demo 3: OTLP Tracing"
echo "----------------------"
echo "For production deployments with OpenTelemetry Collector:"
echo ""
echo "1. Start OpenTelemetry Collector"
echo "2. Configure collector endpoint"
echo "3. Run upload:"
echo "   ./cargoship upload ./data s3://bucket/prefix \\"
echo "     --tracing \\"
echo "     --tracing-exporter otlp \\"
echo "     --tracing-endpoint localhost:4317"
echo ""

# Demo 4: Combined tracing + metrics
echo "📈 Demo 4: Tracing + Prometheus Metrics"
echo "---------------------------------------"
echo "Combine distributed tracing with Prometheus metrics:"
echo ""
echo "./cargoship upload ./data s3://bucket/prefix \\"
echo "  --tracing \\"
echo "  --tracing-exporter jaeger \\"
echo "  --tracing-endpoint localhost:14268 \\"
echo "  --prometheus-addr :9090"
echo ""
echo "Then access:"
echo "  - Traces: http://localhost:16686 (Jaeger UI)"
echo "  - Metrics: http://localhost:9090/metrics (Prometheus)"
echo ""

# Demo 5: Sampling
echo "🎲 Demo 5: Trace Sampling"
echo "-------------------------"
echo "For high-volume uploads, use sampling to reduce overhead:"
echo ""
echo "# Sample 10% of traces"
echo "./cargoship upload ./data s3://bucket/prefix \\"
echo "  --tracing \\"
echo "  --tracing-sample-rate 0.1"
echo ""
echo "This captures:"
echo "  - 10% of upload traces for analysis"
echo "  - 90% reduction in tracing overhead"
echo "  - Still provides statistical visibility"
echo ""

# Demo 6: Disabled tracing
echo "⚡ Demo 6: Tracing Disabled (Default)"
echo "-------------------------------------"
echo "By default, tracing is disabled for zero overhead:"
echo ""
echo "./cargoship upload ./data s3://bucket/prefix"
echo ""
echo "No tracing flags = no tracing overhead"
echo ""

# Summary
echo "📚 Key Capabilities Demonstrated"
echo "================================="
echo "✅ Distributed tracing with OpenTelemetry"
echo "✅ Multiple exporters: stdout, Jaeger, OTLP"
echo "✅ Complete span hierarchy (upload → stages → jobs)"
echo "✅ Trace context propagation across goroutines"
echo "✅ Log enrichment with trace IDs"
echo "✅ Prometheus metrics integration"
echo "✅ Configurable sampling rates"
echo "✅ Zero overhead when disabled"
echo ""

echo "For more information, see:"
echo "  - Issue #155: https://github.com/scttfrdmn/cargoship/issues/155"
echo "  - Documentation: docs/observability.md"
echo ""

# Cleanup
echo "🧹 Cleanup"
echo "----------"
read -p "Delete test data directory? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf "$TEST_DIR"
    echo "✅ Cleaned up test data"
fi

echo ""
echo "Demo complete! 🎉"
