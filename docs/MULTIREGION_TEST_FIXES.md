# Multi-Region Test Fixes - November 2025

## Summary

Fixed critical deadlock in multi-region coordinator's Shutdown() method that was causing tests to timeout after 300+ seconds. Tests now complete much faster, but one performance test still needs fixing.

## Issue #1: Coordinator Shutdown() Deadlock ✅ FIXED

### Problem
The `DefaultCoordinator.Shutdown()` method was causing a classic deadlock:

1. `Shutdown()` acquires write lock `c.mu.Lock()` at line 193
2. `Shutdown()` cancels context to signal background goroutines to stop
3. `Shutdown()` waits for `c.wg.Wait()` **while still holding the lock**
4. Background goroutines (healthCheckService, metricsCollectionService, failoverDetectionService) try to acquire read lock `c.mu.RLock()` to complete their work
5. Deadlock: Shutdown waits for goroutines, goroutines wait for lock

### Symptoms
- Tests timeout after 300-600 seconds
- Goroutine stack traces show:
  - Goroutines stuck in `performHealthChecks()` at `c.mu.RLock()`
  - Shutdown goroutine stuck in `c.wg.Wait()`
  - Message: "goroutine stuck for 8 minutes in sync.Mutex.Lock"

### Root Cause
File: `pkg/multiregion/coordinator.go:191-224`

**Before (Deadlock):**
```go
func (c *DefaultCoordinator) Shutdown(ctx context.Context) error {
	c.mu.Lock()              // ← Acquire write lock
	defer c.mu.Unlock()      // ← Hold lock entire function

	// ... cancel context ...

	// Wait for goroutines while holding lock - DEADLOCK!
	go func() {
		c.wg.Wait()          // ← Wait for goroutines
		close(done)
	}()
	// ...
}
```

### Solution
**Release lock before waiting for goroutines to finish:**

```go
func (c *DefaultCoordinator) Shutdown(ctx context.Context) error {
	// Check if initialized and cancel context with lock held
	c.mu.Lock()
	if !c.initialized {
		c.mu.Unlock()
		return fmt.Errorf("coordinator not initialized")
	}

	c.logger.Info("Shutting down multi-region coordinator")

	// Cancel coordinator context
	if c.cancel != nil {
		c.cancel()
	}

	// CRITICAL: Release lock BEFORE waiting for goroutines to finish
	// Background goroutines need to acquire the lock to complete their work
	c.mu.Unlock()

	// Wait for background services to shutdown
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("Multi-region coordinator shutdown completed")
	case <-ctx.Done():
		c.logger.Warn("Multi-region coordinator shutdown timed out")
		return ctx.Err()
	}

	// Reacquire lock to update initialized state
	c.mu.Lock()
	c.initialized = false
	c.mu.Unlock()

	return nil
}
```

### Impact
- ✅ Background service tests now pass in 0.2s (previously timeout at 300s)
- ✅ Failover tests now pass in 3.3s (previously timeout)
- ✅ Coordinator properly shuts down and logs "shutdown completed"

### Commit
- File: `pkg/multiregion/coordinator.go`
- Lines changed: 191-232 (42 lines)
- Change type: Fix critical deadlock bug

---

## Issue #2: TestFailoverResourceUsage Timeout ✅ RESOLVED - Not a Bug

### Problem
Performance test `TestFailoverResourceUsage` appeared to timeout after 60 seconds.

### Root Cause
**This was NOT a bug** - the test is intentionally slow because it's a performance test:
- Creates 10 failover managers
- Each manager performs 3 graceful failover operations (30 total)
- Each graceful failover takes ~500ms (drain period)
- Test also sleeps for 2 seconds at the end
- **Total legitimate duration: 30 × 0.5s + 2s = 17 seconds**

### Test Results
```
✅ PASS: TestFailoverResourceUsage (17.55s)
    Goroutine usage: started=50, ended=50, increase=0
    ✅ Zero goroutine leaks confirmed
```

### Symptoms (Explained)
- Test was timing out in full suite runs due to accumulated test duration
- When run individually with adequate timeout (30s), test passes perfectly
- The "hanging" at line 436 was actually just waiting for the 500ms drain timer to expire (normal behavior)

### Why It Appeared to Hang
When running with a short timeout (10s), the test only completes 19 out of 30 failovers before hitting the timeout. The goroutine stack trace showed it "stuck" in the select statement, but it was actually just waiting for the drain timer - normal operation.

### Resolution
✅ **NO FIX NEEDED** - Test is working correctly. The test suite just needs adequate timeout for performance tests.

Recommendation: Run full test suite with `-timeout=5m` to accommodate all performance tests.

---

## Test Results Summary

### Before Fixes
```
FAIL  github.com/scttfrdmn/cargoship/pkg/multiregion  300.204s
- Tests timeout after 5 minutes
- Goroutine leaks in Shutdown()
- Multiple tests hanging
```

### After Coordinator Fix
```
✅ TestDefaultCoordinator_backgroundServices  0.216s (was: timeout)
✅ TestComprehensiveFailoverScenarios          3.267s (was: timeout)
✅ Many coordinator tests now pass quickly

⚠️  TestFailoverResourceUsage                 60s timeout (still failing)
⚠️  Full test suite                           90-104s timeout
```

### Target
```
✅ All tests pass in < 30 seconds
✅ Zero goroutine leaks
✅ Race detector clean
✅ 100% pass rate
```

---

## Issue #3: Remaining Test Configuration Failures ⚠️ TODO

### Failing Tests
When running the full test suite, the following tests fail quickly (<1s each) due to configuration issues:

1. **TestFailoverIntegrationScenarios** (0.80s)
2. **TestFailoverRealtimeMonitoring** (0.90s)
3. **TestFailoverConfigurationScenarios** (2.52s)
4. **TestHealthCheckImplementation** (0.00s)
5. **TestHealthCheckTypes** (0.00s)

### Example Error
```
TestHealthCheckTypes:
  Error: invalid configuration: invalid region 'test-region':
         health check interval must be positive
```

### Analysis
These are **test configuration errors**, not deadlocks or goroutine leaks. The tests are failing because:
- Test configurations don't have valid health check intervals set
- Missing required configuration parameters
- These appear to be pre-existing test issues, not related to the Shutdown() deadlock fix

### Status
⚠️ **TODO** - These tests need their configurations fixed, but they're not blocking the main deadlock issue.

**Priority**: Medium - Can be addressed separately from the critical deadlock fix.

---

## Next Steps

1. ✅ **DONE**: Fix coordinator Shutdown() deadlock
2. ✅ **DONE**: Investigate TestFailoverResourceUsage (not a bug - working correctly)
3. ⏳ **TODO**: Fix remaining 5 test configuration issues
4. ⏳ **TODO**: Verify all tests pass with race detector (3 runs)
5. ⏳ **TODO**: Run full test suite and confirm all passing
6. ⏳ **TODO**: Update CLAUDE.md with resolution details

---

## Files Modified

### pkg/multiregion/coordinator.go
- Function: `Shutdown()` (lines 191-232)
- Change: Release mutex before waiting for goroutines
- Impact: Critical deadlock fix - enables proper shutdown

### pkg/multiregion/failover_performance_test.go (pending)
- Function: `TestFailoverResourceUsage()` (lines 420-455)
- Change: TBD - need to identify root cause
- Impact: Performance test reliability

---

## Lessons Learned

### Deadlock Pattern
**Classic pattern**: Holding a lock while waiting for goroutines that need that lock

**Detection**:
- Look for `defer mu.Unlock()` at function start
- Look for goroutine wait (`wg.Wait()`) in same function
- Check if goroutines being waited for need the lock

**Prevention**:
- Release locks before waiting for goroutines
- Use fine-grained locking
- Document lock ordering requirements

### Context Usage
**Always use timeout contexts in tests:**
```go
// ❌ BAD: Never times out
ctx := context.Background()

// ✅ GOOD: Has timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

---

**Date**: November 3, 2025
**Author**: Claude Code
**Status**: Partial fix complete, one test remaining
