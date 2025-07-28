# CargoShip Development and Testing Workflow

## Quick Start

### 1. Local Development Setup
```bash
# Clone and navigate to project
cd cargoship

# Start local development environment
cd docker/development
docker-compose up -d

# Wait for services to be healthy
./scripts/test-suite.sh local

# View dashboards
open http://localhost:3000  # Grafana (admin/admin123)
open http://localhost:9093  # Prometheus
```

### 2. Development Cycle
```bash
# Make code changes
vim pkg/launch/ghost_ship.go

# Rebuild and test
docker-compose build cargoship-controller ghost-ship-1
docker-compose up -d cargoship-controller ghost-ship-1

# Run tests
./scripts/test-suite.sh --generate-data integration

# Check logs
docker logs -f cargoship-ghost-ship-1
```

### 3. astrapi.local Testing
```bash
# Deploy to astrapi
./scripts/launch-ghost-ship.sh \
  --target astrapi.local \
  --config configs/astrapi/ghost-ship-production.yaml \
  launch

# Run astrapi tests
./scripts/test-suite.sh astrapi

# Monitor on astrapi
ssh admin@astrapi.local 'docker logs -f cargoship-ghost-astrapi-production-ghost'
```

## Testing Environments

### Local Docker Environment
- **Purpose**: Development and unit testing
- **Components**: Controller + 2 Ghost Ships + LocalStack S3
- **Performance**: Simulated network conditions
- **Data**: Generated test files

### astrapi.local Environment  
- **Purpose**: Real-world validation
- **Components**: Production ghost ship on QNAP NAS
- **Performance**: Real 10Gbps + 5Gbps network
- **Data**: Actual NAS files with real AWS S3

## Test Suite Usage

```bash
# Run specific test suites
./scripts/test-suite.sh local       # Local Docker tests
./scripts/test-suite.sh integration # Integration tests
./scripts/test-suite.sh performance # Performance tests
./scripts/test-suite.sh astrapi     # astrapi.local tests
./scripts/test-suite.sh all         # All tests

# Options
./scripts/test-suite.sh --generate-data local  # Generate test data
./scripts/test-suite.sh --cleanup all          # Clean up after tests
./scripts/test-suite.sh --verbose astrapi      # Verbose output
```

## Development Best Practices

### 1. Code Changes
- Always test locally first
- Run integration tests before astrapi deployment
- Use feature branches for significant changes
- Test both autonomous and controller-coordinated modes

### 2. Configuration Testing
- Validate configs with different storage classes
- Test various file patterns and rules
- Verify authentication and TLS settings
- Test error handling and recovery scenarios

### 3. Performance Validation
- Monitor memory usage and CPU utilization
- Test with realistic file sizes and counts
- Validate network bandwidth utilization
- Check S3 optimization effectiveness

## Monitoring and Debugging

### Local Development
```bash
# View logs
docker logs cargoship-controller-dev
docker logs cargoship-ghost-ship-1

# Check metrics
curl http://localhost:8080/metrics
curl http://localhost:9091/metrics

# Inspect containers
docker exec -it cargoship-ghost-ship-1 /bin/bash
```

### astrapi.local Production
```bash
# View ghost ship status
./scripts/launch-ghost-ship.sh --target astrapi.local status

# View logs
./scripts/launch-ghost-ship.sh --target astrapi.local logs

# SSH to astrapi for debugging
ssh admin@astrapi.local
docker exec -it cargoship-ghost-astrapi-production-ghost /bin/bash
```

## Common Issues and Solutions

### Issue: Ghost ship not connecting to controller
```bash
# Check controller accessibility
curl http://localhost:8080/health

# Verify WebSocket connectivity
wscat -c ws://localhost:8080/api/v1/agents/connect \
  -H "Authorization: Bearer your-token"

# Check network connectivity from ghost ship
docker exec cargoship-ghost-ship-1 ping cargoship-controller
```

### Issue: Files not being archived
```bash
# Check file discovery
docker exec cargoship-ghost-ship-1 ls -la /data/public/

# Verify archival rules
docker exec cargoship-ghost-ship-1 cat /etc/cargoship/ghost_ship.yaml

# Check S3 connectivity
docker exec cargoship-ghost-ship-1 aws s3 ls
```

### Issue: Performance problems
```bash
# Monitor resource usage
docker stats --no-stream

# Check S3 optimization
curl http://localhost:9091/metrics | grep throughput

# Analyze network utilization
docker exec cargoship-ghost-ship-1 iftop -t -s 10
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: CargoShip Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Start services
        run: |
          cd docker/development
          docker-compose up -d
      - name: Run tests
        run: ./scripts/test-suite.sh --generate-data all
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: test-results
          path: test-results/
```

## Production Deployment Checklist

- [ ] Local tests passing
- [ ] Integration tests passing
- [ ] astrapi.local tests passing
- [ ] Performance benchmarks met
- [ ] Security validations complete
- [ ] Configuration reviewed
- [ ] Monitoring configured
- [ ] Backup and recovery tested
- [ ] Documentation updated

This workflow ensures reliable development and deployment of the CargoShip launch/ghost ship architecture.