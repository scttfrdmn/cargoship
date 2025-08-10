# CargoShip Testing Guide

## 🎯 Overview

This guide provides practical instructions for writing high-quality tests in CargoShip following our comprehensive testing architecture. It addresses common pitfalls and provides concrete examples for proper test implementation.

## 🏗️ Test Structure and Categories

### Test File Naming and Build Tags

```go
// Unit Test (fast, no external dependencies)
// File: calculator_test.go
package calculator

import (
    "testing"
    "github.com/scttfrdmn/cargoship/internal/testutil"
)

// Integration Test (requires external services)
// File: s3_integration_test.go
//go:build integration
// +build integration

package s3

// Performance Test (benchmarks, stress tests)
// File: upload_performance_test.go  
//go:build performance
// +build performance

package s3
```

## 🔧 Using Test Utilities

### Goroutine Leak Detection

```go
// Option 1: Simple leak detection
func TestSimpleFunction(t *testing.T) {
    testutil.RequireNoGoroutineLeak(t)
    
    // Your test logic here
    result := simpleFunction()
    assert.Equal(t, expected, result)
}

// Option 2: Advanced leak detection with custom options
func TestComplexFunction(t *testing.T) {
    opts := testutil.DefaultLeakCheckOptions()
    opts.Timeout = 10 * time.Second
    
    testutil.WithLeakCheck(t, opts, func(t *testing.T) {
        // Test logic that may start goroutines
        complexFunction()
    })
}

// Option 3: Testing functions that spawn goroutines
func TestBackgroundProcess(t *testing.T) {
    testutil.TestWithGoroutine(t, func(ctx context.Context) {
        backgroundWorker(ctx) // Your goroutine function
    })
}
```

### Proper Test Skipping

```go
func TestLongRunningOperation(t *testing.T) {
    testutil.SkipIfShort(t, "requires network simulation")
    
    // Long-running test logic
    simulateNetworkConditions()
}
```

## 🚀 Test Categories and Commands

### Running Different Test Types

```bash
# Unit tests only (fast, for development)
make test-unit

# Integration tests (requires Docker/LocalStack)
make test-integration

# Performance tests (benchmarks, stress tests) 
make test-performance

# End-to-end tests (full system validation)
make test-e2e

# All test categories
make test-all

# Test quality checks
make test-quality

# Goroutine leak detection
make test-leak-check
```

### Writing Tests for Each Category

#### Unit Tests (Default)
```go
func TestChunkPredictor(t *testing.T) {
    predictor := NewChunkPredictor()
    
    // Test pure logic, no external dependencies
    result := predictor.PredictOptimalSize(1024)
    assert.Greater(t, result, 0)
}
```

#### Integration Tests
```go
//go:build integration
// +build integration

func TestS3Upload(t *testing.T) {
    testutil.SkipIfShort(t, "requires LocalStack")
    
    testutil.WithLeakCheck(t, testutil.DefaultLeakCheckOptions(), func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        
        // Setup LocalStack
        client := createLocalStackS3Client(t)
        
        // Test S3 operations
        err := uploadToS3(ctx, client, testData)
        require.NoError(t, err)
        
        // Cleanup
        cancel()
        time.Sleep(50 * time.Millisecond)
    })
}
```

#### Performance Tests
```go
//go:build performance
// +build performance

func BenchmarkUploadSpeed(b *testing.B) {
    testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Benchmark upload operation
            uploadData(testPayload)
        }
    })
}

func TestStressUpload(t *testing.T) {
    testutil.SkipIfShort(t, "stress test takes several minutes")
    
    // Test with high concurrency
    const numUploads = 200
    var wg sync.WaitGroup
    
    for i := 0; i < numUploads; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Individual upload
        }()
    }
    
    wg.Wait()
}
```

## 🛡️ Common Patterns and Best Practices

### Proper Goroutine Management

```go
// ✅ CORRECT: Proper cleanup pattern
func TestBackgroundService(t *testing.T) {
    testutil.WithLeakCheck(t, testutil.DefaultLeakCheckOptions(), func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        
        service := NewBackgroundService()
        
        done := make(chan struct{})
        go func() {
            defer close(done)
            service.Run(ctx)
        }()
        
        // Test operations
        time.Sleep(100 * time.Millisecond)
        
        // Cleanup
        cancel()
        
        // Wait for completion
        select {
        case <-done:
            // Success
        case <-time.After(5 * time.Second):
            t.Fatal("Service did not stop within timeout")
        }
    })
}

// ❌ INCORRECT: No cleanup
func TestBackgroundServiceBad(t *testing.T) {
    service := NewBackgroundService()
    go service.Run(context.Background()) // Goroutine leak!
    
    time.Sleep(100 * time.Millisecond)
    // Test ends without cleanup
}
```

### Resource Management

```go
// ✅ CORRECT: Resource cleanup
func TestDatabaseOperations(t *testing.T) {
    db := setupTestDB(t)
    defer func() {
        if err := db.Close(); err != nil {
            t.Errorf("Database cleanup failed: %v", err)
        }
    }()
    
    // Test database operations
    result := db.Query("SELECT * FROM test")
    assert.NotNil(t, result)
}

// ✅ CORRECT: Complex resource cleanup
func TestMultipleResources(t *testing.T) {
    resources := setupResources(t)
    defer func() {
        for _, resource := range resources {
            if err := resource.Cleanup(); err != nil {
                t.Logf("Resource cleanup warning: %v", err)
            }
        }
    }()
    
    // Test logic using resources
}
```

### Context and Timeout Management

```go
// ✅ CORRECT: Appropriate timeouts
func TestNetworkOperation(t *testing.T) {
    testutil.SkipIfShort(t, "requires network operation")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    result, err := performNetworkOperation(ctx)
    require.NoError(t, err)
    assert.NotNil(t, result)
}

// ❌ INCORRECT: Too short timeout
func TestNetworkOperationBad(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()
    
    // This will likely fail due to timeout
    result, err := performNetworkOperation(ctx)
}
```

## 🔍 Test Quality Standards

### Checklist for Test Quality

- [ ] **Goroutine Management**: All goroutines have proper cleanup
- [ ] **Resource Cleanup**: All resources (files, connections, etc.) are cleaned up
- [ ] **Context Handling**: Appropriate timeouts and cancellation
- [ ] **Build Tags**: Performance and integration tests have proper tags  
- [ ] **Short Mode**: Long-running tests skip in short mode
- [ ] **Error Handling**: All method calls check errors appropriately
- [ ] **Deterministic**: Tests produce consistent results
- [ ] **Isolated**: Tests don't depend on external state or order

### Running Quality Checks

```bash
# Check all test files for quality issues
make test-quality

# The quality checker will report issues like:
# - Missing build tags on performance tests
# - Goroutines without cleanup patterns  
# - Integration tests without proper tags
# - Missing error checking
# - Hardcoded sleeps without justification
```

## 🚨 Troubleshooting Common Issues

### Goroutine Leaks
```
Error: goleak: found goroutine:
goroutine 123 [select]:
your.package.backgroundWorker()
```

**Solution**: Add proper context cancellation:
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
// ... start goroutines with ctx
cancel() // Explicit cancellation before test ends
time.Sleep(50 * time.Millisecond) // Allow cleanup
```

### Test Timeouts
```
Error: test timed out after 60s
```

**Solutions**:
1. Add `testutil.SkipIfShort()` for long tests
2. Use appropriate build tags (`//go:build performance`)
3. Reduce test scope or use mocks
4. Ensure proper cleanup to prevent hanging

### Flaky Tests
**Common causes and solutions**:
- **Race conditions**: Use proper synchronization (channels, mutexes)
- **Timing dependencies**: Avoid `time.Sleep`, use events/channels
- **Shared state**: Ensure test isolation
- **Resource contention**: Use unique test resources

### Integration Test Failures
```
Error: connection refused to localhost:4566
```

**Solutions**:
1. Check LocalStack is running: `docker ps | grep localstack`
2. Use proper build tags: `//go:build integration`
3. Run with integration flag: `make test-integration`
4. Check skip conditions work: `testutil.SkipIfShort()`

## 📊 Test Metrics and Coverage

### Coverage Requirements by Package
- Core logic packages: **85%** minimum, **95%** target
- Business logic packages: **70%** minimum, **90%** target  
- Utilities: **60%** minimum, **85%** target
- CLI commands: **50%** minimum, **75%** target
- UI/TUI packages: **40%** minimum, **70%** target

### Monitoring Test Health
```bash
# Check coverage by package
go test -cover ./...

# Generate detailed coverage report
make test-coverage
open coverage.html

# Check for flaky tests (run multiple times)
for i in {1..10}; do make test-unit || break; done
```

## 🎯 Migration Guide

### Updating Existing Tests

1. **Add leak detection**:
   ```go
   // Add to existing tests
   func TestExisting(t *testing.T) {
       testutil.RequireNoGoroutineLeak(t)
       // existing test code
   }
   ```

2. **Add build tags**:
   ```go
   // For performance tests
   //go:build performance
   // +build performance
   ```

3. **Add skip conditions**:
   ```go
   // For long-running tests
   func TestLongRunning(t *testing.T) {
       testutil.SkipIfShort(t, "description")
       // test code
   }
   ```

4. **Fix goroutine cleanup**:
   ```go
   // Replace context.Background() with cancellable context
   ctx, cancel := context.WithCancel(context.Background())
   defer cancel()
   ```

### Testing the Migration

```bash
# Test that quality checks pass
make test-quality

# Test that unit tests run quickly
time make test-unit

# Test that categorization works
make test-integration  # Should only run tagged tests
make test-performance  # Should only run performance tests
```

This comprehensive testing guide ensures CargoShip maintains high code quality while providing excellent developer experience and reliable test execution.