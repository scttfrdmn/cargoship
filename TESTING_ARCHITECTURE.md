# CargoShip Testing Architecture

## 🎯 Overview

This document defines the comprehensive testing architecture for CargoShip, addressing systemic issues identified in the current testing regime and establishing standards for maintainable, reliable tests.

## 🏷️ Test Categorization

### 1. Unit Tests (`*_test.go`)
**Scope:** Single functions/methods, no external dependencies
**Execution:** `make test-unit`
**Requirements:**
- Run in <1 second per test file
- No goroutine leaks  
- No external service dependencies
- 100% deterministic
- Thread-safe

### 2. Integration Tests (`*_integration_test.go`, build tag: `integration`)
**Scope:** Component interactions, external services (LocalStack, real AWS)
**Execution:** `make test-integration`
**Requirements:**
- Use build tag: `//go:build integration`
- Proper setup/teardown with Docker
- Resource cleanup guaranteed
- Maximum 5 minutes per test

### 3. Performance Tests (`*_performance_test.go`, build tag: `performance`)
**Scope:** Benchmarks, stress testing, resource usage validation
**Execution:** `make test-performance`
**Requirements:**
- Use build tag: `//go:build performance`
- Memory usage tracking
- Performance regression detection
- Maximum 10 minutes per test

### 4. End-to-End Tests (`*_e2e_test.go`, build tag: `e2e`)
**Scope:** Full workflow testing, CLI interaction, real system behavior
**Execution:** `make test-e2e`
**Requirements:**
- Use build tag: `//go:build e2e`
- Complete environment simulation
- User scenario validation
- Maximum 15 minutes per test

## 🔧 Test Quality Standards

### Goroutine Management Rules
```go
// ✅ CORRECT: Proper goroutine cleanup
func TestBackgroundProcess(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    done := make(chan struct{})
    go func() {
        defer close(done)
        backgroundProcess(ctx)
    }()
    
    // Test logic
    time.Sleep(100 * time.Millisecond)
    
    // Cleanup
    cancel()
    
    select {
    case <-done:
        // Good - goroutine stopped
    case <-time.After(time.Second):
        t.Fatal("Goroutine did not stop within timeout")
    }
}
```

### Context and Timeout Standards
- **Unit tests:** 5 seconds maximum total runtime
- **Integration tests:** 1 minute per logical operation
- **Always use cancellable contexts:** `context.WithCancel()` for background processes
- **Explicit cleanup ordering:** Stop() → Cancel() → Wait for completion

### Resource Management
```go
// ✅ CORRECT: Resource cleanup pattern
func TestResourceUsage(t *testing.T) {
    resource := setupResource(t)
    defer func() {
        if err := resource.Cleanup(); err != nil {
            t.Errorf("Resource cleanup failed: %v", err)
        }
    }()
    
    // Test logic using resource
}
```

## 🛠️ Testing Infrastructure

### 1. Goroutine Leak Detection
Use `go.uber.org/goleak` in all tests that start goroutines:

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}

func TestWithGoroutines(t *testing.T) {
    defer goleak.VerifyNone(t)
    
    // Test that starts goroutines
}
```

### 2. Test Categories and Build Tags
```makefile
# Unit tests - fast, no external dependencies
test-unit:
	go test -short -race -timeout=60s ./...

# Integration tests - LocalStack, Docker dependencies  
test-integration:
	go test -tags=integration -race -timeout=300s ./...

# Performance tests - benchmarks, stress tests
test-performance: 
	go test -tags=performance -bench=. -timeout=600s ./...

# End-to-end tests - full system validation
test-e2e:
	go test -tags=e2e -timeout=900s ./...
```

### 3. Test Quality Gates

#### Pre-commit Quality Checks
- Goroutine leak detection on changed files
- Test timeout validation (<5s for unit tests)
- Coverage requirements (60% minimum)
- No TODO/FIXME in test files

#### CI/CD Quality Checks  
- All test categories must pass
- Race condition detection
- Memory leak detection
- Performance regression alerts
- Flaky test detection (retry patterns)

### 4. Coverage Requirements by Package Type

| Package Type | Minimum Coverage | Target Coverage |
|--------------|------------------|----------------|
| Core Logic (`pkg/controller`, `pkg/core`) | 85% | 95% |
| Business Logic (`pkg/aws/*`, `pkg/staging`) | 70% | 90% |
| Utilities (`pkg/utils`, `pkg/config`) | 60% | 85% |
| CLI Commands (`cmd/*`) | 50% | 75% |
| TUI/Interface (`pkg/tui`, `pkg/repl`) | 40% | 70% |

## 🧪 Testing Patterns and Examples

### Property-Based Testing
Use `github.com/leanovate/gopter` for algorithmic correctness:

```go
func TestCompressionProperty(t *testing.T) {
    parameters := gopter.DefaultTestParameters()
    properties := gopter.NewProperties(parameters)
    
    properties.Property("compress then decompress equals original", 
        prop.ForAll(func(data []byte) bool {
            compressed := compress(data)
            decompressed := decompress(compressed)
            return bytes.Equal(data, decompressed)
        }, gen.SliceOfN(100, gen.UInt8())))
    
    properties.TestingRun(t)
}
```

### Fuzz Testing Integration
```go
func FuzzConfigParser(f *testing.F) {
    f.Add(`{"key": "value"}`) // Seed corpus
    
    f.Fuzz(func(t *testing.T, configData string) {
        _, err := parseConfig(configData)
        // Should not panic, errors are acceptable
    })
}
```

### Test Data Management
```go
// testdata/ directory structure
testdata/
├── configs/           # Test configuration files
├── archives/         # Sample archive files  
├── large-files/      # Large test files (git LFS)
└── golden/           # Golden master test files
```

## 📊 Test Metrics and Monitoring

### Flaky Test Detection
- Track test failure rates over time
- Retry failed tests max 2 times
- Alert on >5% failure rate for any test

### Performance Regression Detection  
- Benchmark key operations in CI
- Alert on >10% performance degradation
- Track memory usage trends

### Coverage Trend Analysis
- Track coverage changes per PR
- Require coverage increase for new features
- Generate coverage heat maps

## 🚨 Critical Issues to Address Immediately

### 1. Zero-Coverage Packages (CRITICAL)
- `cmd/cargoship-cost` - Cost management CLI
- `cmd/controller` - Central controller  
- `pkg/aws/cost` - Cost calculation module
- `pkg/tui` - Terminal UI functionality

### 2. Goroutine Leak Fixes (HIGH)
- All files in `pkg/aws/s3/*_test.go` with background processes
- `pkg/staging/network_adaptation_test.go` 
- `pkg/multiregion/*_test.go` coordination tests

### 3. Test Infrastructure (HIGH)
- Add `goleak` to all test packages
- Implement test categorization build tags
- Update Makefile with new test targets
- Enhance CI workflows

### 4. Test Quality Standards (MEDIUM)
- Audit all tests for proper cleanup patterns
- Fix race conditions in concurrent tests  
- Implement property-based testing for algorithms
- Add fuzz testing for parsers

## 📚 Implementation Roadmap

### Phase 1: Foundation (Week 1)
- [ ] Implement goroutine leak detection framework
- [ ] Fix critical zero-coverage packages
- [ ] Add test categorization build tags
- [ ] Update Makefile and CI workflows

### Phase 2: Quality (Week 2)  
- [ ] Fix all goroutine leak issues
- [ ] Implement test quality gates
- [ ] Add property-based testing framework
- [ ] Enhance pre-commit hooks

### Phase 3: Advanced (Week 3)
- [ ] Add fuzz testing pipeline
- [ ] Implement performance regression detection
- [ ] Add comprehensive security testing
- [ ] Create test quality dashboard

### Phase 4: Optimization (Week 4)
- [ ] Optimize test execution time
- [ ] Add parallel test execution
- [ ] Implement advanced coverage analysis
- [ ] Create test maintenance automation

This architecture addresses all identified issues systematically while establishing maintainable standards for future development.