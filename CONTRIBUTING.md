# Contributing to CargoShip

<div align="center">
  <img src="../assets/images/logo.png" alt="CargoShip Logo" width="150" height="150">
</div>

Thank you for your interest in contributing to CargoShip! This project builds upon the foundation of Duke University's excellent [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl) with enterprise enhancements for AWS and modern cloud infrastructure.

## 🚀 Getting Started

### Prerequisites

- **Go 1.26+** - CargoShip is written in Go (see `go.mod`)
- **AWS CLI** (optional) - Only for opt-in real-AWS testing
- **Pre-commit hooks** - Automatic code quality/security checks

> Integration tests run against an **in-process Substrate emulator** — no Docker
> or LocalStack required (this replaced the old Docker/LocalStack setup in
> v0.13.1).

### Development Setup

1. **Fork and Clone**
   ```bash
   git clone https://github.com/your-username/cargoship.git
   cd cargoship
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Set Up Pre-commit Hooks**
   ```bash
   ./scripts/setup-hooks.sh
   ```

4. **Run Tests**
   ```bash
   make test
   go test -race ./...
   ```
   Integration tests use an in-process emulator, so no external services are
   needed:
   ```bash
   go test -tags integration ./...
   ```

## 📋 Development Process

### Code Standards

CargoShip enforces these standards via the pre-commit hook and CI:

- **Zero linting violations** - `golangci-lint` must pass
- **Security scanning** - `govulncheck` (zero known vulnerabilities), gitleaks, Trivy, Semgrep
- **Test coverage** - New code requires tests
- **Go module consistency** - All modules must be verified

### Commit Guidelines

We use conventional commits for clear change tracking:

```
feat: add BBR congestion control algorithm
fix: resolve S3 multipart upload memory leak
docs: update AWS integration guide
test: add coverage for cost optimization
```

### Pre-commit Checks

Our pre-commit hook automatically runs:
- **Dependency verification** - Ensures all tools are available
- **Go module consistency** - Validates module integrity
- **Security scanning** - Checks for vulnerabilities
- **Linting** - Zero violations required
- **Test execution** - Full test suite with coverage

## 🧪 Testing

### Test Categories

1. **Unit Tests** - Fast, isolated component testing
   ```bash
   go test -short ./...
   ```

2. **Integration Tests** - in-process Substrate S3 emulator (no Docker)
   ```bash
   go test -tags integration ./...
   ```

3. **Benchmarks** - Performance validation
   ```bash
   go test -bench=. ./...
   ```

4. **Real AWS Tests** - Live AWS environment testing
   ```bash
   go test -tags realaws ./...
   ```

### Coverage Requirements

- **New code**: Must include comprehensive tests
- **Bug fixes**: Must include regression tests
- **Performance features**: Must include benchmarks

The pre-commit hook enforces a project-wide coverage floor (currently 56%) and a
code-quality score; check the current number with `make test-coverage`.

### Test Patterns

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    Input
        expected Expected
        wantErr  bool
    }{
        {"valid input", validInput, expectedOutput, false},
        {"error case", invalidInput, nil, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## 🏗️ Architecture

### Core Principles

1. **Modularity** - Clear separation of concerns
2. **Testability** - Easy to test components
3. **Performance** - Optimized for speed and efficiency
4. **Security** - Built-in security from the ground up
5. **Observability** - Comprehensive metrics and logging

### Key Modules

- **`pkg/pipeline/`** - Streaming pipeline (Scanner → Chunker → Archiver → Uploader)
- **`pkg/chunking/`** - File grouping into chunks and shard routing
- **`pkg/compression/`** - Compression (zstd and others)
- **`pkg/manifest/`** - Manifest read/write, query, and extraction
- **`pkg/aws/s3/`** - S3 upload transporters and Glacier restore
- **`pkg/launch/`** - Ghost-ship agents (autonomous NAS-side archival)
- **`pkg/tui/`** - Terminal user interface

### Adding New Features

1. **Design Document** - For significant features
2. **Interface Definition** - Clear API boundaries
3. **Implementation** - Follow existing patterns
4. **Testing** - Comprehensive test coverage
5. **Documentation** - Update relevant docs

## 📝 Documentation

### Documentation Types

- **User Documentation** - `docs/` folder for end users
- **API Documentation** - Go doc comments
- **Architecture Documentation** - Design decisions and rationale
- **Development Documentation** - Contributing and development guides

### Writing Guidelines

- Use clear, concise language
- Include code examples
- Keep documentation up to date with code changes
- Follow markdown best practices

## 🐛 Bug Reports

### Before Submitting

1. **Search existing issues** - Avoid duplicates
2. **Use latest version** - Ensure bug still exists
3. **Minimal reproduction** - Simplest case that shows the bug
4. **Environment details** - OS, Go version, CargoShip version

### Bug Report Template

```markdown
**CargoShip Version**: (output of `cargoship --version`)
**Go Version**: (output of `go version`)
**OS**: e.g. Ubuntu 22.04

**Description**
Clear description of the bug.

**Steps to Reproduce**
1. Run command `cargoship upload ...`
2. See error

**Expected Behavior**
What should have happened.

**Actual Behavior**
What actually happened.

**Logs/Output**
```
[Include relevant logs]
```
```

## 🌟 Feature Requests

### Feature Request Process

1. **Check roadmap** - See [ROADMAP.md](ROADMAP.md)
2. **Open discussion** - Use GitHub Discussions
3. **Gather feedback** - Community input welcomed
4. **Design proposal** - For complex features
5. **Implementation** - Follow development process

### Enhancement Guidelines

- Align with project goals
- Consider AWS optimization opportunities
- Maintain backward compatibility
- Include comprehensive testing

## 🎯 Priority Areas

We particularly welcome contributions aligned with the current
[roadmap](ROADMAP.md):

- **Cost & budget** - cost-analysis accuracy, reporting, budget/quota features
- **Reliability** - upload/restore robustness, resume, verification
- **Performance** - upload throughput and memory efficiency
- **DVC & workflows** - graduating the DVC integration from beta toward stable
- **Documentation & tests** - guides, examples, and edge-case coverage

Speculative directions (multi-cloud, ML-based optimization, a REST API) are
listed as **not committed** on the [roadmap](ROADMAP.md) — discuss in an issue
before investing significant effort.

## 🔄 Pull Request Process

### Before Submitting

1. **Fork from main branch**
2. **Create feature branch**: `git checkout -b feature/amazing-feature`
3. **Make changes** with tests
4. **Run pre-commit checks**
5. **Update documentation** if needed

### Pull Request Requirements

- **Clear description** - What and why
- **Breaking changes** - Clearly identified
- **Tests pass** - All checks green
- **Documentation updated** - If applicable
- **Conventional commits** - Clean commit history

### Review Process

1. **Automated checks** - CI must pass
2. **Code review** - At least one approval
3. **Testing validation** - Comprehensive testing
4. **Documentation review** - If docs changed
5. **Final approval** - Maintainer approval

## 🤝 Community

### Code of Conduct

CargoShip is committed to providing a welcoming and inclusive environment for all contributors. Please:

- Be respectful and constructive
- Help others learn and grow
- Focus on what's best for the community
- Show empathy and kindness

### Getting Help

- **GitHub Discussions** - General questions and ideas
- **GitHub Issues** - Bug reports and feature requests
- **Documentation** - [cargoship.app](https://cargoship.app)
- **Code Review** - Learn from PR feedback

### Recognition

Contributors are recognized in:
- **CHANGELOG.md** - Feature and fix credits
- **Repository contributors** - GitHub contributors page
- **Release notes** - Significant contribution highlights

## 📜 License

By contributing to CargoShip, you agree that your contributions will be licensed under the MIT License.

---

## Attribution

CargoShip builds upon the excellent work of Duke University's SuitcaseCTL project. We gratefully acknowledge their innovation in research data management and continue their spirit of open collaboration.

**Ready to contribute?** Start by exploring our [good first issues](https://github.com/scttfrdmn/cargoship/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) or join the discussion in [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions).

---

*Ship your contributions with confidence. Ship them with CargoShip.* 🚢