# CargoShip Development Rules

## Core Principles

### 1. Problem Resolution Policy - NO BYPASSING
**All problems must be fixed - never bypassed, worked around, cheated, or faked.**

- ❌ **FORBIDDEN**: Skipping tests, commenting out failing code, or temporary workarounds
- ❌ **FORBIDDEN**: Adding `// TODO: fix this later` without immediate action
- ❌ **FORBIDDEN**: Mocking failures or returning fake success responses
- ✅ **REQUIRED**: Root cause analysis and proper fixes for all issues
- ✅ **REQUIRED**: If a dependency has bugs, either fix it, replace it, or contribute upstream

### 2. Code Quality Standards

#### Linting
- **ALL code must pass linting** with zero warnings
- Use `golangci-lint` for Go code with strict configuration
- Fix all linting issues before committing - no exceptions
- Linting failures block all PRs and commits

#### Test Coverage Requirements
- **Project-wide coverage: minimum 85%**
- **Individual file coverage: minimum 80%**
- Coverage measured by `go test -cover` and enforced in CI
- New code cannot reduce overall coverage percentage
- Untestable code must be explicitly justified and documented

#### Test Quality
- Tests must be meaningful, not just coverage padding
- All error paths must be tested
- Edge cases and boundary conditions must be covered
- Integration tests required for complex workflows
- No flaky tests - all tests must be deterministic

### 3. Code Standards

#### Documentation
- All public functions, types, and packages must have documentation
- Code comments explain **why**, not **what**
- Complex algorithms require detailed explanations
- API changes require documentation updates

#### Error Handling
- All errors must be properly handled - no ignored errors
- Use structured error types with context
- Implement proper error wrapping and unwrapping
- Log errors at appropriate levels with sufficient context

#### Security
- No hardcoded secrets, passwords, or API keys
- Input validation for all external data
- Proper sanitization of user inputs
- Secure defaults for all configurations

### 4. Git and Version Control

#### Commits
- Descriptive commit messages following conventional commit format
- Each commit must build and pass all tests
- No WIP commits on main branch
- Squash feature branches before merging

#### Branching
- Feature branches for all non-trivial changes
- Main branch always deployable
- No direct commits to main branch
- All merges require PR review and approval

### 5. Dependency Management

#### Third-Party Libraries
- Evaluate security, maintenance status, and license compatibility
- Pin to specific versions, avoid floating dependencies
- Regular security audits with `go mod audit`
- Document rationale for each major dependency

#### Updates
- Test thoroughly before updating dependencies
- Never update dependencies just to update - have a reason
- Backward compatibility must be maintained
- Breaking changes require major version bumps

### 6. Performance Requirements

#### Benchmarks
- Performance-critical code must have benchmarks
- Regression testing for performance-sensitive operations
- Memory usage optimization for large data processing
- Profiling integration for production monitoring

### 7. Enforcement

#### Automated Checks
- CI pipeline enforces all rules automatically
- **Pre-commit hooks prevent rule violations and are mandatory**
- Coverage reports generated and tracked over time
- Automated security scanning
- Git hooks validate quality before each commit

#### Manual Review
- All code changes require peer review
- Reviewers must verify rule compliance
- Architecture decisions require team consensus
- Regular code quality audits

### 8. Consequences

#### For Rule Violations
- Immediate rejection of PRs that violate rules
- Required remediation before any new feature work
- Documentation of violations and resolution in commit messages
- Escalation for repeated violations

## Implementation Checklist

Before any commit or PR:
- [ ] All tests pass (`go test ./...`)
- [ ] Linting passes (`golangci-lint run`)
- [ ] Coverage meets requirements (`go test -cover ./...`)
- [ ] No security vulnerabilities (`go mod audit`)
- [ ] Documentation updated for public APIs
- [ ] Error handling properly implemented
- [ ] Performance impact assessed
- [ ] **Pre-commit hooks installed and passing** (`.githooks/pre-commit`)

### Pre-Commit Hook Setup

**MANDATORY**: All developers must install and use pre-commit hooks.

```bash
# Install hooks (run once after cloning)
./scripts/setup-hooks.sh

# Verify installation
.githooks/pre-commit

# Test manually before committing
git add . && .githooks/pre-commit
```

The pre-commit hook automatically:
- ✅ Runs `golangci-lint` with zero tolerance for warnings
- ✅ Executes full test suite with coverage validation
- ✅ Verifies 85%+ project coverage and 80%+ individual file coverage
- ✅ Checks Go module consistency (`go mod tidy -diff`)
- ✅ Validates documentation for exported functions
- ✅ Scans for code quality issues (TODO/FIXME/debugging statements)
- ✅ Ensures development rules compliance

**Cannot bypass**: Hooks enforce the "NO BYPASSING" policy automatically.

## Tooling Requirements

### Required Tools
- `golangci-lint` for code linting
- `go test -race -cover` for testing
- `go mod audit` for security scanning
- `go tool cover` for coverage analysis
- `go tool pprof` for performance profiling

### CI/CD Integration
- Automated testing on all PRs
- Coverage reporting and trending
- Security scanning integration
- Performance regression detection
- Automatic dependency vulnerability scanning

---

**These rules are non-negotiable. Quality and reliability are paramount to CargoShip's mission of enterprise-grade data archiving.**