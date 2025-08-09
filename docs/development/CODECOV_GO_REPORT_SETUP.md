# Codecov & Go Report Card Setup Guide

This guide explains how to set up Codecov and Go Report Card for the CargoShip project to get professional code quality badges and coverage reporting.

## 🎯 Codecov Setup

### 1. Sign Up for Codecov

1. Go to [codecov.io](https://codecov.io)
2. Sign up with your GitHub account
3. Grant access to the `scttfrdmn/cargoship` repository
4. Get your repository's upload token

### 2. Configure GitHub Repository

1. Go to your repository settings on GitHub
2. Navigate to **Settings > Secrets and variables > Actions**
3. Add a new repository secret:
   - Name: `CODECOV_TOKEN`
   - Value: Your Codecov upload token from step 1

**✅ Note: The CODECOV_TOKEN secret has already been configured for this repository.**

### 3. Verify GitHub Actions Workflow

The test workflow is already configured in `.github/workflows/test.yml`:

```yaml
- name: Run tests with coverage
  run: |
    go test -race -coverprofile=coverage.txt -coverpkg=./... ./...

- name: Upload coverage reports to Codecov
  uses: codecov/codecov-action@v5
  with:
    token: ${{ secrets.CODECOV_TOKEN }}
```

### 4. Local Testing

Test coverage locally before pushing:

```bash
# Run tests with coverage
make codecov-test

# Upload to Codecov (requires CODECOV_TOKEN env var)
export CODECOV_TOKEN=273eda7b-ebbb-4c12-a268-9e296b1406a4
make codecov-upload

# Or generate local HTML report
make test-coverage
open coverage.html
```

### 5. Codecov Configuration

The `codecov.yml` file is already configured with:
- **Component-specific coverage targets**
- **Ignore patterns** for generated files
- **Pull request comments** with coverage diffs
- **Coverage status checks** for CI

## 📊 Go Report Card Setup

### 1. Automatic Registration

Go Report Card automatically scans public Go repositories on GitHub. No registration needed!

### 2. Trigger Initial Scan

1. Go to [goreportcard.com](https://goreportcard.com)
2. Enter your repository URL: `github.com/scttfrdmn/cargoship`
3. Click "Generate Report" to trigger the first scan
4. The report will be available at: `https://goreportcard.com/report/github.com/scttfrdmn/cargoship`

### 3. Improve Your Score

Go Report Card checks these areas:

#### ✅ **gofmt** (100%)
Files are properly formatted:
```bash
gofmt -w .
```

#### ✅ **go vet** (100%) 
No suspicious constructs:
```bash
go vet ./...
```

#### ✅ **golint** (100%)
Code follows Go conventions:
```bash
golangci-lint run
```

#### ✅ **gocyclo** (100%)
Cyclomatic complexity is low:
```bash
# Avoid deeply nested functions
# Refactor complex functions
```

#### ✅ **ineffassign** (100%)
No ineffectual assignments:
```bash
# Remove unused variable assignments
```

#### ✅ **misspell** (100%)
No common misspellings:
```bash
# Fix typos in comments and strings
```

### 4. Optimize for Go Report Card

Add these tools to your development workflow:

```bash
# Install linting tools
make install-tools

# Run all quality checks
make lint

# Fix auto-fixable issues
make lint-fix
```

## 🚀 Badge Integration

### Current Badges in README.md

```markdown
[![Go Reference](https://pkg.go.dev/badge/github.com/scttfrdmn/cargoship.svg)](https://pkg.go.dev/github.com/scttfrdmn/cargoship)
[![Go Report Card](https://goreportcard.com/badge/github.com/scttfrdmn/cargoship)](https://goreportcard.com/report/github.com/scttfrdmn/cargoship)
[![codecov](https://codecov.io/gh/scttfrdmn/cargoship/branch/main/graph/badge.svg)](https://codecov.io/gh/scttfrdmn/cargoship)
[![Go Version](https://img.shields.io/github/go-mod/go-version/scttfrdmn/cargoship)](https://github.com/scttfrdmn/cargoship/blob/main/go.mod)
[![Build Status](https://github.com/scttfrdmn/cargoship/actions/workflows/test.yml/badge.svg)](https://github.com/scttfrdmn/cargoship/actions)
[![GitHub Release](https://img.shields.io/github/v/release/scttfrdmn/cargoship?include_prereleases)](https://github.com/scttfrdmn/cargoship/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/scttfrdmn/cargoship)](https://hub.docker.com/r/scttfrdmn/cargoship)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
```

### Badge Status Updates

- **Go Report Card**: Updates automatically when you push to GitHub
- **Codecov**: Updates on every CI run with test workflow
- **Build Status**: Updates on every push/PR
- **Go Version**: Updates automatically from go.mod

## 🔧 Development Workflow

### Pre-Commit Quality Checks

1. **Run tests with coverage**:
   ```bash
   make codecov-test
   ```

2. **Check Go Report Card factors**:
   ```bash
   make lint
   ```

3. **Security scanning**:
   ```bash
   make security
   ```

4. **Full development cycle**:
   ```bash
   make dev  # lint + test
   ```

### Continuous Integration

The GitHub Actions workflow automatically:
- ✅ Runs tests with race detection
- ✅ Generates coverage reports
- ✅ Uploads coverage to Codecov
- ✅ Runs linting (affects Go Report Card)
- ✅ Performs security scanning
- ✅ Tests on multiple Go versions and OS

## 📈 Monitoring & Maintenance

### Weekly Tasks

1. **Check Codecov Dashboard**:
   - Review coverage trends
   - Check for coverage drops
   - Review pull request coverage

2. **Check Go Report Card**:
   - Ensure A+ rating maintained
   - Fix any new issues flagged

3. **Dependency Updates**:
   ```bash
   make mod-update
   ```

### Coverage Goals by Component

Set in `codecov.yml`:
- **Compression**: 95% (critical performance code)
- **S3 Optimization**: 80% (complex algorithms)
- **Suitcase Archives**: 90% (core functionality)
- **Terminal UI**: 70% (UI testing challenges)
- **Controller**: 60% (integration heavy)

## 🎯 Expected Results

After setup completion:

### Codecov
- ✅ Coverage badge shows current percentage
- ✅ PR comments with coverage diffs
- ✅ Coverage trending over time
- ✅ Component-specific coverage tracking

### Go Report Card
- ✅ A+ rating badge
- ✅ Automatic quality scanning
- ✅ Detailed quality breakdown
- ✅ Best practices compliance

### GitHub Actions
- ✅ Automated testing on push/PR
- ✅ Multi-platform testing
- ✅ Coverage reporting
- ✅ Quality checks

## 🆘 Troubleshooting

### Codecov Issues

**Coverage not uploading**:
```bash
# Check token is set
echo $CODECOV_TOKEN

# Test local upload
make codecov-upload

# Check GitHub Actions logs
```

**Low coverage warnings**:
```bash
# Run coverage locally
make test-coverage
open coverage.html

# Check codecov.yml configuration
```

### Go Report Card Issues

**Poor rating**:
```bash
# Check each factor
make lint          # golint, gofmt, go vet
go test ./...      # tests passing
```

**Stale report**:
- Go to goreportcard.com
- Enter your repo URL to trigger refresh
- Wait a few minutes for update

## 📚 Additional Resources

- **Codecov Documentation**: [docs.codecov.io](https://docs.codecov.io)
- **Go Report Card**: [goreportcard.com](https://goreportcard.com)
- **GitHub Actions**: [docs.github.com/actions](https://docs.github.com/actions)
- **golangci-lint**: [golangci-lint.run](https://golangci-lint.run)

---

*This setup provides professional code quality indicators that build trust with users and contributors.*