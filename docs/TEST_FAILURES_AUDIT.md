# Test Failures Audit for v0.5.0 Technical Debt Resolution

**Date**: 2025-10-16
**Status**: In Progress
**Goal**: Achieve 100% test pass rate for v0.5.0 release

## Executive Summary

**Total Failing Tests Identified**: ~20 tests across 2 packages
**Critical Issues**: 3 categories of failures
**Estimated Effort**: Medium (2-3 days)

## Test Failure Categories

### 1. cmd/cargoship/cmd Package Failures

#### 1.1 Integration Test Failures (2 tests)

**Test**: `TestV042DataDiscoveryWorkflow/Phase3_Selective_Extraction`
**File**: `cmd/cargoship/cmd/integration_test.go:178-180`
**Status**: ❌ FAILING
**Error**:
```
Extract command failed as expected in test environment:
failed to determine files for extraction: specific file not found in archive: /results/summary.json

Error: "failed to determine files for extraction: specific file not found in archive: /results/summary.json"
does not contain "index"
```

**Root Cause**: Test expects error message to contain "index" but gets "specific file not found"
**Impact**: Low - Test assertion mismatch
**Priority**: P2 - Medium
**Fix Approach**: Update test assertion to match actual error message OR fix error message generation

---

**Test**: `TestV042BackwardsCompatibility/Existing_Find_Command_Still_Works`
**File**: `cmd/cargoship/cmd/integration_test.go:428-438`
**Status**: ❌ FAILING
**Error**:
```
1. Expected value not to be nil (line 428)
2. Error message does not contain "Find where a file" (line 438)
```

**Root Cause**: Find command help text has changed, backward compatibility test is outdated
**Impact**: Medium - Indicates potential backward compatibility issue
**Priority**: P1 - High
**Fix Approach**:
- Option A: Update test to match new help text
- Option B: Restore original help text for backward compatibility
- Recommended: Review if help text change is intentional, update test accordingly

#### 1.2 Goroutine Leak Failures (14 tests in restore_test.go)

**Tests**:
1. `TestNewRestoreCmd`
2. `TestParseRestoreOptions`
3. `TestParseRestoreFilter`
4. `TestParseRestoreFilterInvalidDates`
5. `TestParseRestoreFilterInvalidSizes`
6. `TestCalculateTotalSize`
7. `TestEstimateRestoreTime`
8. `TestCalculateRequiredSpace`
9. `TestCalculateStorageCosts`
10. `TestCalculateTransferCosts`
11. `TestRestoreStructures`
12. `TestRestoreEngine`
13. `TestRestoreCommandIntegration`
14. *(and more)*

**File**: `cmd/cargoship/cmd/restore_test.go` (multiple lines)
**Status**: ❌ FAILING
**Error**:
```
found unexpected goroutines:
[Goroutine 438 in state chan receive,
with github.com/rclone/rclone/fs/accounting.(*tokenBucket).startSignalHandler.func1 on top of the stack]
```

**Root Cause**: rclone library starts a signal handler goroutine that is not cleaned up in tests
**Impact**: High - Affects 14+ tests, indicates resource leak
**Priority**: P1 - High
**Fix Approach**:
1. Add test cleanup to properly shutdown rclone accounting
2. Use `goleak.IgnoreTopFunction()` to ignore known safe rclone goroutines
3. Wrap rclone initialization in tests with proper teardown

**Example Fix**:
```go
import "go.uber.org/goleak"

func TestNewRestoreCmd(t *testing.T) {
    defer goleak.VerifyNone(t,
        goleak.IgnoreTopFunction("github.com/rclone/rclone/fs/accounting.(*tokenBucket).startSignalHandler.func1"),
    )
    // test code...
}
```

### 2. multiregion Package Status

**Test Suite Status**: ✅ ALL PASSING
**Tests Run**: 29 tests
**Pass Rate**: 100%
**Notable**: Previous goroutine leak issues have been resolved

### 3. AWS S3 Integration Tests (Background Processes)

**Status**: Still Running
**Tests**:
- `TestCargoShipTransporterIntegration` (multiple shells)
- `TestTransporterUploadIntegration`
- `TestMultiRegionCoordinatorIntegration`

**Action**: Wait for completion to assess results

## Detailed Failure Analysis

### Goroutine Leak Root Cause Deep Dive

**Library**: `github.com/rclone/rclone@v1.70.2`
**Function**: `fs/accounting/accounting_unix.go:24`
**Issue**: Signal handler goroutine started but never stopped

**Code Location**:
```go
// rclone/fs/accounting/accounting_unix.go:24
func (tb *tokenBucket) startSignalHandler.func1() {
    // Listens on signal channel indefinitely
}
```

**Impact**:
- Every test that initializes rclone creates a persistent goroutine
- Goroutines accumulate across test suite
- Not a real leak in production, but fails goleak checks

**Solutions**:
1. **Immediate Fix** (Recommended): Add goleak ignore patterns
2. **Long-term Fix**: Contribute PR to rclone for proper cleanup
3. **Alternative**: Mock rclone components in tests

## Prioritized Fix Plan

### Phase 1: Quick Wins (Day 1) - ✅ Ready to Execute
1. ✅ Fix integration test assertions (2 tests)
   - Update `TestV042DataDiscoveryWorkflow` error assertion
   - Fix `TestV042BackwardsCompatibility` help text check
2. ✅ Add goleak ignores for rclone goroutines (14 tests)
   - Single global ignore pattern can fix all restore tests

### Phase 2: Integration Tests (Day 2) - Depends on background tests
1. ⏳ Analyze AWS S3 integration test results
2. ⏳ Fix any S3-related failures
3. ⏳ Ensure multiregion integration tests pass

### Phase 3: Verification & Documentation (Day 3)
1. Run full test suite with `-race` flag
2. Verify zero failures across all packages
3. Update CLAUDE.md with resolution details
4. Create regression test suite

## Test Execution Commands

```bash
# Run specific failing tests
go test ./cmd/cargoship/cmd -run TestV042 -v
go test ./cmd/cargoship/cmd -run TestRestore -v

# Run with race detector
go test ./cmd/cargoship/cmd -race -short

# Run full suite excluding integration
go test ./... -short -v

# Check for goroutine leaks
go test ./cmd/cargoship/cmd -run TestNewRestoreCmd -v
```

## Success Criteria for v0.5.0

- [ ] Zero test failures in `go test ./... -short`
- [ ] Zero goroutine leaks detected
- [ ] All integration tests pass with real AWS credentials
- [ ] Race detector shows no issues (`-race`)
- [ ] Test coverage maintained or improved
- [ ] All fixes documented

## Risk Assessment

**Low Risk Fixes**:
- Integration test assertions (simple string comparison fixes)

**Medium Risk Fixes**:
- Goroutine leak ignores (need to verify rclone goroutines are truly safe)

**High Risk Areas**:
- AWS S3 integration tests (external dependency, may have intermittent failures)
- Backward compatibility fixes (may reveal actual breaking changes)

## Next Steps

1. **Immediate**: Fix the 2 integration test assertion failures
2. **Next**: Add goleak ignores for restore_test.go
3. **Then**: Wait for background AWS tests and address failures
4. **Finally**: Run comprehensive verification suite

## Notes

- All v0.4.6 new tests are passing (setup_test.go: 8/8)
- Multiregion package is now healthy after previous leak fixes
- Main focus should be on cmd package test stability
- Consider adding pre-commit hook to catch goroutine leaks early
