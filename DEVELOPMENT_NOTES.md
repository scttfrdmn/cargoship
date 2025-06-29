# Development Notes

## Project Status (2025-06-28)

### Recent Changes
- **Project renamed**: `cargoship-cli` → `cargoship` for consistency
- **Module path updated**: All import paths now use `github.com/scttfrdmn/cargoship`
- **Git remote fixed**: Origin now points to correct `cargoship` repository

### Known Issues to Address

#### Test Failures
1. **pkg/errors**: Compilation errors due to undefined AWS types
   - `aws.GenericAPIError` and `types.AccessDenied` not found
   - Missing `ctx` variable in some functions
   - Type mismatch in retry configuration

2. **pkg/inventory**: Stack overflow during tests
   - Infinite recursion or excessive goroutine stack usage
   - May be related to tar archive processing

3. **pkg/rclone**: Error message format changed
   - Expected: `"didn't find section in config file"`
   - Actual: `"didn't find section in config file (\"never-exists\")"`

### Current State
- ✅ Project builds successfully
- ✅ Dependencies cleaned up with `go mod tidy`
- ✅ Most tests pass (cmd, pkg, aws/costs, gpg, suitcase packages)
- ❌ 3 packages have failing tests that need investigation
- ✅ Git repository ready with committed changes

### Next Steps for Development Resume
1. Fix AWS SDK import issues in `pkg/errors/handler.go`
2. Investigate stack overflow in inventory tests
3. Update rclone test expectations
4. Run full test suite to ensure all fixes work
5. Consider running linting tools (golangci-lint, gofmt)

### Build Status
- **Go version**: 1.24.3
- **Main binary**: Builds without errors
- **Test coverage**: Partial (some tests failing)