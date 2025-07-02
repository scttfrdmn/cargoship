# LocalStack Integration for CargoShip Testing

## Overview

This document describes the LocalStack integration implemented for testing CargoShip's AWS CloudWatch metrics functionality. LocalStack provides a fully functional local AWS cloud stack for development and testing.

## LocalStack Setup

### Prerequisites
- Docker installed and running
- LocalStack container available

### Starting LocalStack
```bash
# Start existing LocalStack container
docker start localstack

# Or create new LocalStack container
docker run --rm -d -p 4566:4566 localstack/localstack
```

### Verification
```bash
# Check LocalStack health
curl -s "http://localhost:4566/_localstack/health"

# Verify CloudWatch service is available
curl -s "http://localhost:4566/_localstack/health" | grep cloudwatch
```

## Integration Tests

### Test Structure
- **File**: `cmd/cargoship/cmd/metrics_integration_test.go`
- **Build Tag**: `// +build integration`
- **Endpoint**: `http://localhost:4566` (LocalStack default)

### Key Features

#### 1. Automatic LocalStack Detection
```go
func isLocalStackAvailable() bool {
    client := getTestCloudWatchClient()
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    _, err := client.ListMetrics(ctx, &cloudwatch.ListMetricsInput{})
    return err == nil
}
```

#### 2. AWS Client Configuration
```go
func getTestCloudWatchClient() *cloudwatch.Client {
    cfg := aws.Config{
        Region:      testRegion,
        Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
        EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(
            func(service, region string, options ...interface{}) (aws.Endpoint, error) {
                return aws.Endpoint{
                    URL:           localStackEndpoint,
                    SigningRegion: testRegion,
                }, nil
            },
        ),
    }
    return cloudwatch.NewFromConfig(cfg)
}
```

#### 3. Comprehensive Metrics Testing
The integration tests cover all CargoShip metric types:
- **Upload Metrics**: Duration, throughput, errors, storage classes
- **Cost Metrics**: Savings calculations, optimization types
- **Network Metrics**: Bandwidth, latency, optimization
- **Operational Metrics**: Active uploads, memory, CPU usage
- **Lifecycle Metrics**: Policy management, transitions

### Test Execution

#### Running Integration Tests
```bash
# Run all integration tests
go test -tags=integration ./cmd/cargoship/cmd/ -v

# Run specific LocalStack tests
go test -tags=integration -run TestRunMetricsIntegrationWithLocalStack ./cmd/cargoship/cmd/ -v
```

#### Running Without LocalStack
Tests gracefully skip when LocalStack is unavailable:
```
Skipping integration tests - LocalStack not available
To run integration tests:
  docker run --rm -d -p 4566:4566 localstack/localstack
```

## Coverage Improvements

### Metrics Function Coverage
- **runMetrics**: 4.4% → 33.3% (+28.9%)
- **Total cmd package**: 71.3% → 73.2% (+1.9%)

### Test Scenarios Covered
1. **Basic metrics publishing** with LocalStack CloudWatch
2. **Custom namespace and region** configuration
3. **AWS configuration error handling**
4. **Metrics validation and structure testing**
5. **CloudWatch API interaction patterns**

## Benefits of LocalStack Integration

### 1. Realistic AWS Testing
- Tests actual AWS SDK interactions
- Validates CloudWatch API calls
- Tests metric data formatting and publishing

### 2. No External Dependencies
- No AWS credentials required
- No actual AWS costs
- Isolated test environment

### 3. CI/CD Friendly
- Consistent test results
- Fast execution
- Easy setup and teardown

### 4. Comprehensive Coverage
- Tests complete metrics publishing workflow
- Validates error handling paths
- Tests configuration variations

## Best Practices

### 1. Test Isolation
```go
func TestMain(m *testing.M) {
    // Check LocalStack availability
    if !isLocalStackAvailable() {
        fmt.Println("Skipping integration tests - LocalStack not available")
        os.Exit(0)
    }
    
    // Run tests
    code := m.Run()
    os.Exit(code)
}
```

### 2. Graceful Degradation
Tests skip automatically when LocalStack is unavailable rather than failing.

### 3. Realistic Data
Use production-like metric data in tests to validate real-world scenarios.

### 4. Error Path Testing
Test both success and failure scenarios including AWS configuration errors.

## Future Enhancements

### 1. Additional AWS Services
- S3 integration testing
- IAM policy testing
- SNS/SQS testing

### 2. Performance Testing
- Metrics throughput testing
- Concurrent publishing tests
- Large dataset handling

### 3. Advanced Scenarios
- Network failure simulation
- Authentication error testing
- Rate limiting scenarios

## Troubleshooting

### Common Issues

#### LocalStack Not Starting
```bash
# Check Docker status
docker ps

# Check LocalStack logs
docker logs localstack
```

#### Connection Refused
```bash
# Verify LocalStack port
netstat -an | grep 4566

# Check LocalStack health
curl http://localhost:4566/_localstack/health
```

#### Test Timeouts
- Increase timeout values in test configuration
- Check LocalStack resource limits
- Verify Docker has sufficient memory

## Documentation References

- [LocalStack Documentation](https://docs.localstack.cloud/)
- [AWS SDK Go v2](https://aws.github.io/aws-sdk-go-v2/)
- [CargoShip Metrics](./pkg/aws/metrics/)