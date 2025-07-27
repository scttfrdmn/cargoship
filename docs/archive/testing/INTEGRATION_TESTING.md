# Integration Testing with LocalStack

CargoShip uses [LocalStack](https://localstack.cloud/) to provide comprehensive integration testing for AWS services without requiring actual AWS resources or incurring costs.

## Overview

The integration tests use LocalStack's **free Community Edition** to emulate:
- **S3** - Object storage operations (uploads, downloads, metadata)
- **CloudWatch** - Metrics and monitoring (future)

## Prerequisites

1. **Docker** - Must be installed and running
2. **LocalStack** - Automatically pulled as Docker image

## Running Integration Tests

### Automated Script (Recommended)

The easiest way to run integration tests is using the provided script:

```bash
# Run integration tests
./scripts/run-integration-tests.sh

# Run integration tests with benchmarks
./scripts/run-integration-tests.sh --bench
```

The script automatically:
- ✅ Starts LocalStack container
- ✅ Waits for services to be ready  
- ✅ Runs integration tests
- ✅ Cleans up containers
- ✅ Provides detailed output

### Manual Testing

If you prefer manual control:

```bash
# 1. Start LocalStack
docker run --rm -d \
    --name cargoship-localstack \
    -p 4566:4566 \
    -e SERVICES=s3,cloudwatch \
    localstack/localstack:latest

# 2. Wait for LocalStack to be ready
curl -s http://localhost:4566/_localstack/health

# 3. Run integration tests
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test  
export AWS_DEFAULT_REGION=us-east-1
go test -tags=integration -v ./pkg/aws/s3/...

# 4. Cleanup
docker stop cargoship-localstack
```

## Test Coverage

### Without Integration Tests
- **Unit Tests Only**: 74.0% coverage
- Missing: AWS SDK interactions, actual upload/download workflows

### With Integration Tests  
- **Unit + Integration Tests**: 92.2% coverage ⭐
- **Total Improvement**: +60.7 percentage points from original 31.5%
- **Covers**: Real AWS API calls, end-to-end workflows, error scenarios

## Test Structure

### Integration Test Files

```
pkg/aws/s3/
├── s3_test.go              # Unit tests (74.0% coverage)
├── integration_test.go     # Integration tests (+18.2% coverage)
└── adaptive_test.go        # Adaptive algorithm tests
```

### Test Categories

#### 🔄 **Transporter Integration Tests**
- `TestTransporterUploadIntegration` - Real S3 uploads with metadata
- `TestTransporterExistsIntegration` - Object existence checking
- `TestTransporterGetObjectInfoIntegration` - Object metadata retrieval

#### 🚀 **Parallel Upload Tests**
- `TestParallelUploaderIntegration` - Multi-prefix parallel uploads
- `TestParallelUploaderEmptyInput` - Edge case handling

#### 🎯 **Storage Optimization Tests**  
- `TestUploadStorageClassOptimization` - Intelligent storage class selection
- Tests Deep Archive, Glacier, Standard-IA, Intelligent Tiering

#### ⚡ **Performance Benchmarks**
- `BenchmarkUploadIntegration` - Single upload performance
- `BenchmarkParallelUploadIntegration` - Parallel upload performance

## LocalStack Configuration

### Services Used
- **S3**: ✅ Full support in free version
- **CloudWatch**: ✅ Available in free version  
- **Pricing API**: ❌ Not available in free version (uses unit tests only)

### LocalStack Features Tested
- Bucket creation and management
- Object upload/download operations
- Object metadata and storage classes
- Multipart uploads (for large files)
- Object existence checking
- Error handling and edge cases

## Sample Test Output

```bash
✅ LocalStack is ready!
📊 LocalStack Status: s3=available, cloudwatch=available

🧪 Running integration tests...

=== RUN   TestTransporterUploadIntegration
=== RUN   TestTransporterUploadIntegration/simple_upload
=== RUN   TestTransporterUploadIntegration/large_file_upload  
=== RUN   TestTransporterUploadIntegration/upload_with_intelligent_tiering
--- PASS: TestTransporterUploadIntegration (0.28s)

=== RUN   TestParallelUploaderIntegration
INFO starting parallel upload archives=4 max_prefixes=2
INFO parallel upload completed total_uploaded=4 total_errors=0 duration=6.374167ms
--- PASS: TestParallelUploaderIntegration (0.01s)

PASS
coverage: 92.2% of statements
ok  	github.com/scttfrdmn/cargoship/pkg/aws/s3	1.727s

✅ All integration tests passed!
```

## Benefits of LocalStack Integration

### ✅ **Real AWS API Testing**
- Tests actual AWS SDK calls without mocking
- Validates request/response handling
- Catches integration issues early

### ✅ **Cost-Free Testing**
- No AWS charges for development/testing
- Unlimited test runs without cost concerns
- Safe for CI/CD pipelines

### ✅ **Comprehensive Coverage**
- End-to-end workflow testing
- Error scenario validation
- Performance benchmarking

### ✅ **Developer Experience**
- Fast test execution (local)
- Reliable and repeatable
- Easy setup and teardown

## Troubleshooting

### LocalStack Not Starting
```bash
# Check Docker is running
docker info

# Check port availability
lsof -i :4566

# View LocalStack logs
docker logs cargoship-localstack
```

### Test Failures
```bash
# Verify LocalStack health
curl http://localhost:4566/_localstack/health

# Check S3 service status
curl http://localhost:4566/_localstack/health | jq '.services.s3'

# Review test output for specific failures
go test -tags=integration -v ./pkg/aws/s3/... | grep FAIL
```

### Network Issues
```bash
# Test LocalStack connectivity
curl -v http://localhost:4566/_localstack/health

# Check AWS client configuration
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1
```

## Future Enhancements

### Potential Additions
- **CloudWatch integration tests** for metrics collection
- **Error injection testing** with LocalStack Pro features
- **Multi-region testing** scenarios
- **Large file upload tests** (>100MB)
- **Concurrent upload stress tests**

### CI/CD Integration
The integration tests can be easily added to CI/CD pipelines:

```yaml
# Example GitHub Actions integration
- name: Start LocalStack
  run: docker run --rm -d -p 4566:4566 --name localstack localstack/localstack
  
- name: Run Integration Tests
  run: ./scripts/run-integration-tests.sh
```

## Conclusion

The LocalStack integration testing provides **enterprise-grade test coverage** at **zero AWS cost**, making it an ideal solution for CargoShip's development and quality assurance needs. With **92.2% test coverage**, the S3 package now has comprehensive validation of both business logic and AWS integration functionality.