#!/bin/bash

# CargoShip Astrapi Deployment Script
# Deploys CargoShip test infrastructure to astrapi NAS

set -euo pipefail

# Configuration
ASTRAPI_HOST="${ASTRAPI_HOST:-astrapi.local}"
ASTRAPI_USER="${ASTRAPI_USER:-admin}"
DOCKER_REGISTRY="${DOCKER_REGISTRY:-cargoship}"
IMAGE_TAG="${IMAGE_TAG:-astrapi-latest}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if docker is available
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    # Check if docker-compose is available
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed or not in PATH"
        exit 1
    fi
    
    # Check if ssh is available
    if ! command -v ssh &> /dev/null; then
        log_error "SSH is not available"
        exit 1
    fi
    
    # Test connection to astrapi
    if ! ping -c 1 "$ASTRAPI_HOST" &> /dev/null; then
        log_error "Cannot reach astrapi at $ASTRAPI_HOST"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Build CargoShip container image
build_image() {
    log_info "Building CargoShip container image..."
    
    cd "$(dirname "$0")/.."
    
    # Build the image
    docker build -f docker/Dockerfile.astrapi -t "$DOCKER_REGISTRY:$IMAGE_TAG" .
    
    # Tag for astrapi registry if different
    if [[ "$DOCKER_REGISTRY" != "cargoship" ]]; then
        docker tag "cargoship:$IMAGE_TAG" "$DOCKER_REGISTRY:$IMAGE_TAG"
    fi
    
    log_success "Container image built successfully"
}

# Deploy to astrapi
deploy_to_astrapi() {
    log_info "Deploying to astrapi NAS..."
    
    # Create deployment directory on astrapi
    ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "mkdir -p /volume1/docker/cargoship/results"

    # Copy configuration files. astrapi-config.yaml lands beside the compose
    # file because the compose file bind-mounts it as ./astrapi-config.yaml.
    log_info "Copying configuration files..."
    scp docker/astrapi-config.yaml "$ASTRAPI_USER@$ASTRAPI_HOST:/volume1/docker/cargoship/"
    scp docker/docker-compose.astrapi.yml "$ASTRAPI_USER@$ASTRAPI_HOST:/volume1/docker/cargoship/docker-compose.yml"

    # Save and transfer the Docker image
    log_info "Transferring Docker image to astrapi..."
    docker save "$DOCKER_REGISTRY:$IMAGE_TAG" | ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker load"
    
    # Deploy using docker-compose
    log_info "Starting CargoShip services on astrapi..."
    ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "cd /volume1/docker/cargoship && docker-compose up -d"
    
    log_success "Deployment completed successfully"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."
    
    # Wait for services to start
    sleep 10
    
    # Check if services are running
    if ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml ps | grep -q 'Up'"; then
        log_success "Services are running"
    else
        log_error "Services failed to start"
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml logs"
        exit 1
    fi
    
    # No endpoint to probe: a ghost ship binds no port. The Launch API on :8080
    # was removed in #340 and the :9090 metrics endpoint never existed (#348), so
    # the container log is the health signal.
    log_info "Recent ghost ship log:"
    ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker logs --tail 20 cargoship-astrapi-ghost-ship"

    # Show service status
    log_info "Service status:"
    ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml ps"
}

# Setup AWS credentials
setup_aws_credentials() {
    log_info "Setting up AWS credentials on astrapi..."
    
    # Check if AWS credentials exist locally
    if [[ ! -f "$HOME/.aws/credentials" ]] || [[ ! -f "$HOME/.aws/config" ]]; then
        log_warning "AWS credentials not found locally. Please ensure they exist on astrapi."
        return
    fi
    
    # Create AWS directory on astrapi
    ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "mkdir -p /volume1/homes/admin/.aws"
    
    # Copy AWS credentials (if they don't already exist)
    if ! ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "test -f /volume1/homes/admin/.aws/credentials"; then
        log_info "Copying AWS credentials..."
        scp "$HOME/.aws/credentials" "$ASTRAPI_USER@$ASTRAPI_HOST:/volume1/homes/admin/.aws/"
        scp "$HOME/.aws/config" "$ASTRAPI_USER@$ASTRAPI_HOST:/volume1/homes/admin/.aws/"
        log_success "AWS credentials copied"
    else
        log_info "AWS credentials already exist on astrapi"
    fi
}

# Create test runner script
create_test_runner() {
    log_info "Creating test runner script on astrapi..."
    
    cat << 'EOF' | ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "cat > /volume1/docker/cargoship/run-test.sh && chmod +x /volume1/docker/cargoship/run-test.sh"
#!/bin/bash

# CargoShip Test Runner for Astrapi
# Usage: ./run-test.sh [test-type] [options]

set -euo pipefail

CONTAINER_NAME="cargoship-astrapi-ghost-ship"
TEST_TYPE="${1:-performance}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Check if container is running
if ! docker ps | grep -q "$CONTAINER_NAME"; then
    log_info "Starting CargoShip container..."
    cd /volume1/docker/cargoship
    docker-compose up -d
    sleep 5
fi

log_info "Running CargoShip test: $TEST_TYPE"

# Execute test
docker exec -it "$CONTAINER_NAME" /usr/local/bin/cargoship-test \
    --test-type="$TEST_TYPE" \
    --s3-bucket="cargoship-astrapi-test" \
    --aws-profile="aws" \
    --aws-region="us-west-2" \
    --enable-optimization \
    --enable-bbr \
    --enable-cubic \
    --verbose \
    --output-format="json" \
    "${@:2}"

log_success "Test completed"
EOF
    
    log_success "Test runner script created"
}

# Show usage information
show_usage() {
    cat << EOF
CargoShip Astrapi Deployment Script

Usage: $0 [OPTIONS] [COMMAND]

Commands:
    deploy      Full deployment (default)
    build       Build container image only
    push        Push image to astrapi only
    verify      Verify deployment only
    status      Show service status
    logs        Show service logs
    stop        Stop services
    restart     Restart services
    clean       Remove all containers and images

Options:
    -h, --help          Show this help message
    -v, --verbose       Enable verbose output
    --host HOST         Astrapi hostname (default: astrapi.local)
    --user USER         Astrapi SSH user (default: admin)
    --registry REGISTRY Docker registry/namespace (default: cargoship)
    --tag TAG           Image tag (default: astrapi-latest)

Environment Variables:
    ASTRAPI_HOST        Astrapi hostname
    ASTRAPI_USER        Astrapi SSH user
    DOCKER_REGISTRY     Docker registry/namespace
    IMAGE_TAG           Docker image tag

Examples:
    $0 deploy                           # Full deployment
    $0 build                            # Build image only
    $0 --host mynas.local deploy        # Deploy to custom host
    $0 status                           # Check service status
    $0 logs                             # View service logs
EOF
}

# Parse command line arguments
COMMAND="deploy"
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        --host)
            ASTRAPI_HOST="$2"
            shift 2
            ;;
        --user)
            ASTRAPI_USER="$2"
            shift 2
            ;;
        --registry)
            DOCKER_REGISTRY="$2"
            shift 2
            ;;
        --tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        deploy|build|push|verify|status|logs|stop|restart|clean)
            COMMAND="$1"
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

# Execute command
case "$COMMAND" in
    deploy)
        log_info "Starting full deployment to $ASTRAPI_HOST"
        check_prerequisites
        build_image
        setup_aws_credentials
        deploy_to_astrapi
        create_test_runner
        verify_deployment
        log_success "Deployment completed successfully!"
        log_info "Follow the ghost ship with: ssh $ASTRAPI_USER@$ASTRAPI_HOST 'docker logs -f cargoship-astrapi-ghost-ship'"
        log_info "Run tests with: ssh $ASTRAPI_USER@$ASTRAPI_HOST '/volume1/docker/cargoship/run-test.sh'"
        ;;
    build)
        log_info "Building container image"
        build_image
        ;;
    push)
        log_info "Pushing image to astrapi"
        docker save "$DOCKER_REGISTRY:$IMAGE_TAG" | ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker load"
        ;;
    verify)
        verify_deployment
        ;;
    status)
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml ps"
        ;;
    logs)
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml logs -f"
        ;;
    stop)
        log_info "Stopping services"
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml down"
        ;;
    restart)
        log_info "Restarting services"
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml restart"
        ;;
    clean)
        log_info "Cleaning up containers and images"
        ssh "$ASTRAPI_USER@$ASTRAPI_HOST" "docker-compose -f /volume1/docker/cargoship/docker-compose.yml down --rmi all -v"
        ;;
esac