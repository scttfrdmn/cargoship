#!/bin/bash

# CargoShip Ghost Ship Launcher
# Deploys autonomous ghost ship archival agents to remote NAS systems

set -euo pipefail

# Configuration
TARGET_HOST="${TARGET_HOST:-astrapi.local}"
TARGET_USER="${TARGET_USER:-admin}"
GHOST_SHIP_ID="${GHOST_SHIP_ID:-$(hostname)-ghost-$(date +%s)}"
CONFIG_FILE="${CONFIG_FILE:-examples/launch/ghost_ship_config.yaml}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
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

log_launch() {
    echo -e "${PURPLE}[LAUNCH]${NC} $1"
}

# Show usage
show_usage() {
    cat << EOF
CargoShip Ghost Ship Launcher

Usage: $0 [OPTIONS] COMMAND

Commands:
    launch      Deploy and launch ghost ship on target NAS
    status      Check ghost ship status
    stop        Stop ghost ship on target NAS
    logs        View ghost ship logs
    config      Show ghost ship configuration
    remove      Remove ghost ship from target NAS

Options:
    -h, --help              Show this help message
    -t, --target HOST       Target NAS host (default: astrapi.local)
    -u, --user USER         SSH user for target NAS (default: admin)
    -i, --id ID             Ghost ship ID (default: auto-generated)
    -f, --config FILE       Configuration file (default: examples/launch/ghost_ship_config.yaml)
    -v, --verbose           Enable verbose output

Environment Variables:
    TARGET_HOST             Target NAS hostname
    TARGET_USER             SSH user for target NAS
    GHOST_SHIP_ID           Ghost ship identifier

Examples:
    $0 launch                                    # Launch on astrapi.local
    $0 --target mynas.local launch               # Launch on custom NAS
    $0 --id production-ghost launch              # Launch with specific ID
    $0 status                                    # Check status
    $0 logs                                      # View logs
EOF
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if ssh is available
    if ! command -v ssh &> /dev/null; then
        log_error "SSH is not available"
        exit 1
    fi
    
    # Check if scp is available
    if ! command -v scp &> /dev/null; then
        log_error "SCP is not available"
        exit 1
    fi
    
    # Test connection to target
    if ! ping -c 1 "$TARGET_HOST" &> /dev/null; then
        log_error "Cannot reach target host at $TARGET_HOST"
        exit 1
    fi
    
    # Check if configuration file exists
    if [[ ! -f "$CONFIG_FILE" ]]; then
        log_error "Configuration file not found: $CONFIG_FILE"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Build ghost ship configuration
build_config() {
    log_info "Building ghost ship configuration..."
    
    local temp_config="/tmp/ghost_ship_${GHOST_SHIP_ID}.yaml"
    
    # Copy base configuration
    cp "$CONFIG_FILE" "$temp_config"
    
    # Update configuration with runtime values
    sed -i.bak \
        -e "s/id: .*/id: \"${GHOST_SHIP_ID}\"/" \
        "$temp_config"

    echo "$temp_config"
}

# Deploy ghost ship
deploy_ghost_ship() {
    log_launch "🚢 Deploying CargoShip Ghost Ship to $TARGET_HOST"
    
    local config_file
    config_file=$(build_config)
    
    # Create deployment directory on target
    ssh "$TARGET_USER@$TARGET_HOST" \
        "mkdir -p /volume1/docker/cargoship-ghost/{config,logs,data}"
    
    # Copy configuration
    log_info "Copying ghost ship configuration..."
    scp "$config_file" "$TARGET_USER@$TARGET_HOST:/volume1/docker/cargoship-ghost/config/ghost_ship.yaml"
    
    # Copy AWS credentials if they exist
    if [[ -f "$HOME/.aws/credentials" ]]; then
        log_info "Copying AWS credentials..."
        ssh "$TARGET_USER@$TARGET_HOST" "mkdir -p /volume1/homes/admin/.aws"
        scp "$HOME/.aws/credentials" "$TARGET_USER@$TARGET_HOST:/volume1/homes/admin/.aws/"
        scp "$HOME/.aws/config" "$TARGET_USER@$TARGET_HOST:/volume1/homes/admin/.aws/"
    fi
    
    # Create docker-compose file for ghost ship
    create_docker_compose
    
    # Copy docker-compose file
    scp "/tmp/ghost-ship-compose.yml" \
        "$TARGET_USER@$TARGET_HOST:/volume1/docker/cargoship-ghost/docker-compose.yml"
    
    # Deploy and start ghost ship
    log_info "Starting ghost ship container..."
    ssh "$TARGET_USER@$TARGET_HOST" \
        "cd /volume1/docker/cargoship-ghost && docker-compose up -d"
    
    # Cleanup temporary files
    rm -f "$config_file" "/tmp/ghost-ship-compose.yml"
    
    log_success "Ghost ship deployed successfully!"
    log_launch "👻 Ghost Ship ID: $GHOST_SHIP_ID"
    log_launch "🎯 Target: $TARGET_HOST"
}

# Create docker-compose file
create_docker_compose() {
    cat > "/tmp/ghost-ship-compose.yml" << EOF
version: '3.8'

services:
  cargoship-ghost-ship:
    image: cargoship:astrapi-latest
    container_name: cargoship-ghost-${GHOST_SHIP_ID}
    restart: unless-stopped
    network_mode: host
    
    volumes:
      # Data access (read-only)
      - /volume1/Public:/data/public:ro
      - /volume1/homes:/data/homes:ro
      
      # Configuration
      - ./config/ghost_ship.yaml:/etc/cargoship/ghost_ship.yaml:ro
      
      # AWS credentials
      - /volume1/homes/admin/.aws:/root/.aws:ro
      
      # Logs
      - ./logs:/var/log/cargoship
      
      # Temp/work directory
      - ./data:/tmp/cargoship
    
    environment:
      - CARGOSHIP_LOG_LEVEL=info
      - CARGOSHIP_METRICS_ENABLED=true
      - CARGOSHIP_CONFIG_FILE=/etc/cargoship/ghost_ship.yaml
      - GOMAXPROCS=0
    
    command: >
      ghost-ship 
      --config /etc/cargoship/ghost_ship.yaml
      --log-level info
      --metrics-port 9090
    
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
    
    labels:
      - "cargoship.ghost.id=${GHOST_SHIP_ID}"
      - "cargoship.service=ghost-ship"

  # Optional: Prometheus metrics collector
  prometheus:
    image: prom/prometheus:latest
    container_name: cargoship-prometheus-${GHOST_SHIP_ID}
    restart: unless-stopped
    ports:
      - "9091:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
    profiles:
      - monitoring

volumes:
  prometheus-data:
EOF
}

# Check ghost ship status
check_status() {
    log_info "Checking ghost ship status on $TARGET_HOST..."
    
    # Check if container is running
    if ssh "$TARGET_USER@$TARGET_HOST" \
       "docker ps | grep -q 'cargoship-ghost-${GHOST_SHIP_ID}'"; then
        log_success "Ghost ship is running"
        
        # Show container details
        ssh "$TARGET_USER@$TARGET_HOST" \
            "docker ps --filter name=cargoship-ghost-${GHOST_SHIP_ID} --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"
        
        # Show health status
        log_info "Health status:"
        ssh "$TARGET_USER@$TARGET_HOST" \
            "docker inspect cargoship-ghost-${GHOST_SHIP_ID} --format='{{.State.Health.Status}}' 2>/dev/null || echo 'No health check configured'"
            
    else
        log_error "Ghost ship is not running"
        
        # Check if container exists but is stopped
        if ssh "$TARGET_USER@$TARGET_HOST" \
           "docker ps -a | grep -q 'cargoship-ghost-${GHOST_SHIP_ID}'"; then
            log_warning "Ghost ship container exists but is stopped"
            ssh "$TARGET_USER@$TARGET_HOST" \
                "docker ps -a --filter name=cargoship-ghost-${GHOST_SHIP_ID} --format 'table {{.Names}}\t{{.Status}}'"
        fi
    fi
}

# View ghost ship logs
view_logs() {
    log_info "Viewing ghost ship logs on $TARGET_HOST..."
    
    ssh "$TARGET_USER@$TARGET_HOST" \
        "docker logs -f cargoship-ghost-${GHOST_SHIP_ID}"
}

# Stop ghost ship
stop_ghost_ship() {
    log_info "Stopping ghost ship on $TARGET_HOST..."
    
    ssh "$TARGET_USER@$TARGET_HOST" \
        "cd /volume1/docker/cargoship-ghost && docker-compose down"
    
    log_success "Ghost ship stopped"
}

# Remove ghost ship
remove_ghost_ship() {
    log_warning "Removing ghost ship from $TARGET_HOST..."
    
    # Stop and remove containers
    ssh "$TARGET_USER@$TARGET_HOST" \
        "cd /volume1/docker/cargoship-ghost && docker-compose down --rmi all -v"
    
    # Remove deployment directory
    ssh "$TARGET_USER@$TARGET_HOST" \
        "rm -rf /volume1/docker/cargoship-ghost"
    
    log_success "Ghost ship removed completely"
}

# Show configuration
show_config() {
    log_info "Ghost ship configuration on $TARGET_HOST:"
    
    ssh "$TARGET_USER@$TARGET_HOST" \
        "cat /volume1/docker/cargoship-ghost/config/ghost_ship.yaml"
}

# Parse command line arguments
COMMAND="launch"
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -t|--target)
            TARGET_HOST="$2"
            shift 2
            ;;
        -u|--user)
            TARGET_USER="$2"
            shift 2
            ;;
        -i|--id)
            GHOST_SHIP_ID="$2"
            shift 2
            ;;
        -f|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        launch|status|stop|logs|config|remove)
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

# Show launch banner
echo -e "${PURPLE}"
echo "🚢 CargoShip Ghost Ship Launcher"
echo "================================="
echo -e "${NC}"

# Execute command
case "$COMMAND" in
    launch)
        log_launch "Launching ghost ship: $GHOST_SHIP_ID"
        log_launch "Target: $TARGET_HOST"
        echo
        check_prerequisites
        deploy_ghost_ship
        echo
        log_launch "🎉 Ghost ship launch sequence completed!"
        log_info "Use '$0 status' to check status"
        log_info "Use '$0 logs' to view logs"
        ;;
    status)
        check_status
        ;;
    stop)
        stop_ghost_ship
        ;;
    logs)
        view_logs
        ;;
    config)
        show_config
        ;;
    remove)
        remove_ghost_ship
        ;;
esac