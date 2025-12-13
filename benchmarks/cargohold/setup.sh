#!/bin/bash
# Setup script for CargoHold benchmark suite

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}CargoHold Benchmark Suite Setup${NC}"
echo "=================================="
echo ""

# Check Go installation
echo -e "${YELLOW}Checking Go installation...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Please install Go 1.23+ from https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "  ✅ Go $GO_VERSION"

# Check AWS CLI
echo -e "${YELLOW}Checking AWS CLI...${NC}"
if ! command -v aws &> /dev/null; then
    echo -e "${RED}Error: AWS CLI is not installed${NC}"
    echo "Please install from https://aws.amazon.com/cli/"
    exit 1
fi
echo "  ✅ AWS CLI $(aws --version | awk '{print $1}')"

# Check cargoship
echo -e "${YELLOW}Checking cargoship...${NC}"
if ! command -v cargoship &> /dev/null; then
    echo -e "${YELLOW}  ⚠️  cargoship not found in PATH${NC}"
    echo "  Building cargoship..."
    (cd ../.. && go build -o cargoship ./cmd/cargoship)
    export PATH="$PATH:$(pwd)/../.."
    echo "  ✅ Built cargoship"
else
    echo "  ✅ cargoship $(cargoship --version 2>/dev/null || echo 'found')"
fi

# Optional tools
echo -e "${YELLOW}Checking optional tools...${NC}"

# s5cmd
if command -v s5cmd &> /dev/null; then
    echo "  ✅ s5cmd $(s5cmd version 2>&1 | grep -o 'v[0-9.]*' || echo 'found')"
else
    echo "  ℹ️  s5cmd not found (optional - for competitor benchmarks)"
    echo "     Install: https://github.com/peak/s5cmd"
fi

# MinIO mc
if command -v mc &> /dev/null; then
    echo "  ✅ mc $(mc --version 2>&1 | grep -o 'RELEASE[0-9T-]*' || echo 'found')"
else
    echo "  ℹ️  mc not found (optional - for competitor benchmarks)"
    echo "     Install: https://min.io/docs/minio/linux/reference/minio-mc.html"
fi

# Check AWS credentials
echo ""
echo -e "${YELLOW}Checking AWS credentials...${NC}"
if aws sts get-caller-identity &> /dev/null; then
    echo "  ✅ AWS credentials configured"
    aws sts get-caller-identity --query 'Account' --output text | xargs -I {} echo "     Account: {}"
else
    echo -e "${RED}  ❌ AWS credentials not configured${NC}"
    echo "     Run: aws configure"
    exit 1
fi

# Check S3 bucket configuration
echo ""
echo -e "${YELLOW}Checking S3 bucket configuration...${NC}"
if [ -z "$CARGOSHIP_BENCHMARK_BUCKET" ]; then
    echo -e "${YELLOW}  ⚠️  CARGOSHIP_BENCHMARK_BUCKET not set${NC}"
    echo "     Set it in your environment:"
    echo "     export CARGOSHIP_BENCHMARK_BUCKET=my-test-bucket"
    echo ""
    echo "     Or create .env file:"
    echo "     echo 'export CARGOSHIP_BENCHMARK_BUCKET=my-test-bucket' > .env"
else
    echo "  ✅ Bucket configured: $CARGOSHIP_BENCHMARK_BUCKET"

    # Verify bucket exists and is accessible
    if aws s3 ls "s3://$CARGOSHIP_BENCHMARK_BUCKET" &> /dev/null; then
        echo "  ✅ Bucket accessible"
    else
        echo -e "${RED}  ❌ Cannot access bucket${NC}"
        echo "     Make sure the bucket exists and you have permissions"
        exit 1
    fi
fi

# Build benchmark suite
echo ""
echo -e "${YELLOW}Building benchmark suite...${NC}"
go build -o cargohold-benchmark .
echo "  ✅ Built cargohold-benchmark"

# Create directories
echo ""
echo -e "${YELLOW}Creating directories...${NC}"
mkdir -p test-data results regression/baseline
echo "  ✅ Created: test-data, results, regression/baseline"

echo ""
echo -e "${GREEN}✅ Setup complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Set benchmark bucket: export CARGOSHIP_BENCHMARK_BUCKET=my-test-bucket"
echo "  2. Run small benchmark: make run-small"
echo "  3. View results: open results/*_report.html"
echo ""
echo "For more options: make help"
