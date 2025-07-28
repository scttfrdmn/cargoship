# CargoShip Development and Testing Plan

## Overview

This document outlines the comprehensive testing strategy for CargoShip's launch/ghost ship architecture, covering local development testing through Docker and real-world validation on astrapi.local (QNAP NAS).

## Testing Environments

### 1. Local Development Environment

**Purpose**: Rapid development, unit testing, and integration testing
**Infrastructure**: Docker Compose with simulated services

```bash
# Start full development environment
cd docker/development
docker-compose up -d

# Start with testing profile
docker-compose --profile testing up -d

# Start with load testing
docker-compose --profile load-testing up -d
```

**Components**:
- Central Controller (port 8080)
- 2 Ghost Ships (ports 9091, 9092)
- LocalStack (S3 simulation, port 4566)
- Prometheus (metrics, port 9093)
- Grafana (dashboards, port 3000)
- Test Data Generator
- Integration Test Suite

### 2. astrapi.local Testing Environment

**Purpose**: Real-world validation, performance testing, production readiness
**Infrastructure**: Actual QNAP NAS with real AWS S3

```bash
# Deploy to astrapi.local
./scripts/launch-ghost-ship.sh --target astrapi.local launch

# Deploy with production-like config
./scripts/launch-ghost-ship.sh \
  --target astrapi.local \
  --config configs/astrapi/ghost-ship-production.yaml \
  launch
```

## Testing Phases

### Phase 1: Component Testing (Local Docker)

#### 1.1 Controller Testing
```bash
# Test controller startup and health
curl http://localhost:8080/health

# Test authentication
curl -H "Authorization: Bearer dev-controller-token-123" \
  http://localhost:8080/api/v1/status

# Test WebSocket connectivity
wscat -c ws://localhost:8080/api/v1/agents/connect \
  -H "Authorization: Bearer dev-ghost-token-111"
```

#### 1.2 Ghost Ship Testing
```bash
# Check ghost ship health
curl http://localhost:9091/health  # Ghost Ship 1
curl http://localhost:9092/health  # Ghost Ship 2

# Verify S3 connectivity (LocalStack)
aws --endpoint-url=http://localhost:4566 s3 ls

# Test file discovery
docker exec cargoship-ghost-ship-1 ls -la /data/public/
```

#### 1.3 Integration Testing
```bash
# Run integration test suite
docker-compose --profile testing run --rm integration-tests

# Check test results
ls -la test-results/
cat test-results/integration-test-report.json
```

### Phase 2: End-to-End Testing (Local Docker)

#### 2.1 Autonomous Archival Flow
```bash
# Generate test files
docker-compose --profile testing up test-data-generator

# Monitor archival process
docker logs -f cargoship-ghost-ship-1
docker logs -f cargoship-ghost-ship-2

# Verify files in LocalStack S3
aws --endpoint-url=http://localhost:4566 s3 ls s3://cargoship-dev-bucket-1/
aws --endpoint-url=http://localhost:4566 s3 ls s3://cargoship-dev-bucket-2/
```

#### 2.2 Controller-Ghost Ship Communication
```bash
# Test job assignment via controller API
curl -X POST \
  -H "Authorization: Bearer dev-admin-token-456" \
  -H "Content-Type: application/json" \
  -d '{
    "job_id": "test-job-001",
    "type": "archive",
    "path": "/data/public/documents/*.pdf",
    "destination": "manual-jobs/",
    "storage_class": "STANDARD"
  }' \
  http://localhost:8080/api/v1/ghostships/dev-ghost-ship-1/assign

# Monitor job progress
curl -H "Authorization: Bearer dev-admin-token-456" \
  http://localhost:8080/api/v1/ghostships/dev-ghost-ship-1/jobs
```

### Phase 3: Performance Testing (Local Docker)

#### 3.1 Load Testing
```bash
# Run K6 load tests
docker-compose --profile load-testing run --rm load-tester

# Test concurrent file operations
docker exec cargoship-test-data-generator \
  /scripts/generate-load.sh --files=100 --concurrent=10
```

#### 3.2 Resource Monitoring
```bash
# View metrics in Grafana
open http://localhost:3000  # admin/admin123

# Query Prometheus directly  
curl 'http://localhost:9093/api/v1/query?query=cargoship_active_jobs'
curl 'http://localhost:9093/api/v1/query?query=cargoship_bytes_archived_total'
```

### Phase 4: astrapi.local Real-World Testing

#### 4.1 Environment Setup
```bash
# Deploy controller (if not running locally)
docker run -d \
  --name cargoship-controller \
  -p 8080:8080 \
  -v /etc/cargoship:/etc/cargoship:ro \
  cargoship:controller

# Deploy ghost ship to astrapi.local
./scripts/launch-ghost-ship.sh \
  --target astrapi.local \
  --controller ws://your-controller-host:8080 \
  --id astrapi-production-ghost \
  launch
```

#### 4.2 Real Data Testing
```bash
# Monitor real file archival
ssh admin@astrapi.local \
  'docker logs -f cargoship-ghost-astrapi-production-ghost'

# Check actual AWS S3 buckets
aws s3 ls s3://your-production-bucket/astrapi/

# Verify file integrity
aws s3 cp s3://your-production-bucket/astrapi/test-file.pdf /tmp/
sha256sum /tmp/test-file.pdf
```

#### 4.3 Performance Validation
```bash
# Test high-bandwidth scenarios
ssh admin@astrapi.local \
  'docker exec cargoship-ghost-ship \
   /usr/local/bin/cargoship-test \
   --test-type=bandwidth \
   --target-throughput=500mbps'

# Monitor system resources
ssh admin@astrapi.local 'docker stats cargoship-ghost-ship'
```

### Phase 5: Production Readiness Testing

#### 5.1 Security Testing
```bash
# Test TLS connectivity
openssl s_client -connect astrapi.local:8080 -servername astrapi.local

# Verify certificate validation
curl --cacert /etc/cargoship/ca.crt \
  https://astrapi.local:8080/health

# Test authentication
curl -H "Authorization: Bearer invalid-token" \
  https://astrapi.local:8080/api/v1/status
```

#### 5.2 Fault Tolerance Testing
```bash
# Test controller disconnection
docker stop cargoship-controller
# Ghost ship should continue autonomous operation

# Test network interruption
# Simulate network partition
sudo iptables -A INPUT -s astrapi.local -j DROP

# Test recovery
sudo iptables -D INPUT -s astrapi.local -j DROP
```

## Test Data Generation

### Development Environment
```bash
# Create test file structure
mkdir -p docker/development/test-data/{nas-1,nas-2}/{documents,images,backups,videos}

# Generate sample files
for i in {1..10}; do
  echo "Sample document $i" > docker/development/test-data/nas-1/documents/doc_$i.txt
  echo "Sample PDF content" > docker/development/test-data/nas-1/documents/sample_$i.pdf
done

# Generate binary test files
dd if=/dev/urandom of=docker/development/test-data/nas-1/images/photo_1.jpg bs=1M count=5
dd if=/dev/urandom of=docker/development/test-data/nas-2/videos/video_1.mp4 bs=1M count=50
```

### astrapi.local Environment
```bash
# Use existing files on astrapi
# /volume1/Public/... contains real data for testing

# Or create dedicated test directory
ssh admin@astrapi.local 'mkdir -p /volume1/Public/CargoShip-Test/{docs,media,backups}'
scp test-files/* admin@astrapi.local:/volume1/Public/CargoShip-Test/docs/
```

## Validation Criteria

### Functional Tests
- [ ] Controller starts and serves API
- [ ] Ghost ships connect to controller
- [ ] WebSocket communication works
- [ ] File discovery and pattern matching
- [ ] S3 uploads complete successfully
- [ ] Job assignment and progress tracking
- [ ] Health monitoring and reporting

### Performance Tests
- [ ] Achieves >100MB/s throughput on local network
- [ ] Handles 100+ concurrent files
- [ ] CPU usage <80% under normal load
- [ ] Memory usage <2GB per ghost ship
- [ ] Network utilization >80% of available bandwidth

### Integration Tests
- [ ] Multi-ghost ship coordination
- [ ] Controller failover handling
- [ ] S3 error recovery
- [ ] Configuration updates
- [ ] Log aggregation and monitoring

### Security Tests
- [ ] TLS encryption working
- [ ] Authentication required
- [ ] No credential exposure in logs
- [ ] Network traffic encrypted
- [ ] Proper certificate validation

## Monitoring and Debugging

### Log Locations
```bash
# Local development
docker logs cargoship-controller-dev
docker logs cargoship-ghost-ship-1
docker logs cargoship-ghost-ship-2

# astrapi.local
ssh admin@astrapi.local 'docker logs cargoship-ghost-ship'
ssh admin@astrapi.local 'cat /volume1/docker/cargoship-ghost/logs/ghost-ship.log'
```

### Metrics Endpoints
- Controller: http://localhost:8080/metrics
- Ghost Ship 1: http://localhost:9091/metrics  
- Ghost Ship 2: http://localhost:9092/metrics
- Prometheus: http://localhost:9093/metrics

### Debugging Commands
```bash
# Check WebSocket connections
ss -tulpn | grep :8080

# Verify S3 connectivity
aws s3 ls --endpoint-url=http://localhost:4566

# Monitor file system events
docker exec cargoship-ghost-ship-1 inotifywait -mr /data/public/

# Check container resources
docker stats --no-stream
```

## Continuous Integration

### Automated Testing Pipeline
```yaml
# .github/workflows/test.yml (example)
name: CargoShip Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build images
        run: docker-compose -f docker/development/docker-compose.yml build
      - name: Start services
        run: docker-compose -f docker/development/docker-compose.yml up -d
      - name: Wait for health checks
        run: ./scripts/wait-for-health.sh
      - name: Run tests
        run: docker-compose -f docker/development/docker-compose.yml --profile testing run --rm integration-tests
      - name: Collect logs
        if: failure()
        run: docker-compose -f docker/development/docker-compose.yml logs
```

## Development Workflow

1. **Local Development**: Make changes, test with Docker Compose
2. **Unit Testing**: Run individual component tests
3. **Integration Testing**: Test component interaction
4. **Local Validation**: Full end-to-end testing with LocalStack
5. **astrapi.local Testing**: Deploy to real NAS, test with real AWS
6. **Performance Validation**: Load testing and optimization
7. **Production Deployment**: Deploy to production environment

This comprehensive testing plan ensures reliable development and deployment of the CargoShip launch/ghost ship architecture.