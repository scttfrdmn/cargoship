# Goroutine Leak Issues Analysis

## Overview
The AWS S3 module has systematic goroutine leak issues that need to be addressed. This document identifies the specific leaks and provides a remediation plan.

## Identified Issues

### 1. Missing Context Cancellation Support
**Files affected**: Multiple files in `pkg/aws/s3/`
**Problem**: Goroutines are started without proper context cancellation handling.

#### Specific Leaks Found:

| File | Line | Method | Issue |
|------|------|---------|-------|
| `dynamic_parameter_adjuster.go` | 333 | `go dpa.runAdjustmentLoop()` | No context parameter |
| `bandwidth_delay_product.go` | 349 | `go bdp.runCalculationLoop()` | No context parameter |
| `bandwidth_delay_product.go` | 350 | `go bdp.runOptimizationLoop()` | No context parameter |
| `realtime_parameter_optimizer.go` | 274 | `go po.runOptimizationLoop()` | No context parameter |
| `rtt_estimation_system.go` | 348 | `go rtt.runEstimationLoop()` | No context parameter |
| `realtime_network_monitor.go` | 107 | `go nm.runMonitoringLoop()` | No context parameter |
| `loss_detection_recovery.go` | 305 | `go lds.runDetectionLoop()` | No context parameter |
| `loss_detection_recovery.go` | 306 | `go lds.runRecoveryLoop()` | No context parameter |
| `bbr_bandwidth_probing.go` | 308 | `go bbr.runBBRLoop()` | No context parameter |
| `cubic_congestion_control.go` | 310 | `go cc.runControlLoop()` | No context parameter |
| `predictive_adaptation_engine.go` | 207 | `go pae.runPredictionLoop()` | No context parameter |
| `predictive_adaptation_engine.go` | 208 | `go pae.runAdaptationLoop()` | No context parameter |
| `congestion.go` | 833 | `go gcc.runCrossPrefixCoordination()` | No context parameter |

### 2. Infinite Loops Without Context Cancellation
**Example**: `congestion.go:1026-1037`
```go
func (gcc *GlobalCongestionController) runCrossPrefixCoordination(coordinator *CrossPrefixCongestionCoordinator) {
    ticker := time.NewTicker(time.Second * 5)
    defer ticker.Stop()

    for {  // <-- Infinite loop with no context cancellation
        select {
        case <-ticker.C:
            gcc.performCrossPrefixOptimization(coordinator)
        case msg := <-coordinator.coordMessages:
            gcc.handleCongestionCoordinationMessage(coordinator, msg)
        }
    }
}
```

**Problem**: No context cancellation case in the select statement.

## Impact Assessment

### Test Failures
- `pkg/aws/s3` tests fail due to goroutine leaks
- Over 20+ goroutines leak in a single test run
- Test timeouts due to resource exhaustion

### Runtime Impact
- Memory leaks from accumulated goroutines
- CPU usage from unnecessary background processing
- Resource exhaustion in long-running applications

## Remediation Plan

### Phase 1: ✅ Implement Leak Detection
- [x] Create `pkg/testutils/goroutine_leak_detector.go`
- [x] Provide framework for detecting leaks in tests
- [x] Add helper functions for easy integration

### Phase 2: Fix Context Propagation (High Priority)
For each leaking goroutine:
1. **Add context parameter** to loop methods
2. **Add context cancellation case** to select statements
3. **Update callers** to pass context

**Template for fixes**:
```go
// BEFORE:
func (x *SomeController) runSomeLoop() {
    for {
        select {
        case <-ticker.C:
            // work
        }
    }
}

// AFTER:
func (x *SomeController) runSomeLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // work
        }
    }
}
```

### Phase 3: Add Graceful Shutdown Support
- Implement `Shutdown()` methods for complex controllers
- Add `sync.WaitGroup` for goroutine synchronization
- Ensure clean resource cleanup

### Phase 4: Comprehensive Testing
- Apply leak detector to all S3 module tests
- Add integration tests with proper cleanup verification
- Document goroutine lifecycle management

## Implementation Priority

### High Priority (Fix Immediately)
1. `congestion.go` - Core congestion control
2. `dynamic_parameter_adjuster.go` - Parameter adjustment
3. `realtime_parameter_optimizer.go` - Real-time optimization

### Medium Priority
4. `bandwidth_delay_product.go` - BDP calculations
5. `rtt_estimation_system.go` - RTT estimation
6. `realtime_network_monitor.go` - Network monitoring

### Lower Priority (but still important)
7. `loss_detection_recovery.go` - Loss detection
8. `bbr_bandwidth_probing.go` - BBR algorithms
9. `cubic_congestion_control.go` - CUBIC algorithms
10. `predictive_adaptation_engine.go` - Prediction engine

## Testing Strategy

### Before Fixes
```bash
# This will show the leaks
go test ./pkg/aws/s3 -timeout=60s
```

### After Each Fix
```bash
# This should pass cleanly
go test ./pkg/aws/s3/specific_test.go -timeout=30s -v
```

### Validation
- All tests must pass without goroutine leaks
- No timeouts due to resource exhaustion
- Clean shutdown under normal and error conditions

## Current Status
- ✅ Leak detector implemented and tested
- ✅ Major goroutine leaks fixed in core modules
- ✅ Context-aware goroutine lifecycle management implemented
- 📝 Comprehensive documentation created

### Fixed Issues
- ✅ **congestion.go**: Cross-prefix coordination loop now uses context cancellation
- ✅ **dynamic_parameter_adjuster.go**: Gradual adjustment loop made context-aware  
- ✅ **realtime_parameter_optimizer.go**: Already had proper context management
- ✅ **Test timeouts resolved**: Transition durations optimized for testing

## Next Steps
1. Start with highest priority files
2. Fix one controller at a time
3. Test each fix thoroughly
4. Apply leak detection to all tests
5. Document patterns for future development