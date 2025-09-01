# Disabled GitHub Actions Workflows

These GitHub Actions workflows have been temporarily disabled for solo development workflow optimization.

## Rationale

As a solo development project, we've moved to relying on pre-commit hooks for:
- Code quality checks (linting, formatting)
- Security scanning (gosec, govulncheck)
- Test execution
- Dependency validation

This approach provides:
- **Faster feedback** - Pre-commit hooks run instantly before commit
- **Local validation** - Catch issues before pushing to remote
- **Simplified CI/CD** - No complex GitHub Actions debugging
- **Resource efficiency** - No GitHub Actions minutes consumption

## Pre-Commit Hook Coverage

The `.githooks/pre-commit` script provides equivalent functionality:
- Go module consistency checks
- Security vulnerability scanning (govulncheck)
- Linting with golangci-lint (zero violations policy)
- Comprehensive test suite with coverage analysis
- Quality gates and reporting

## Re-enabling Workflows

If needed, workflows can be re-enabled by moving them back:
```bash
mv .github/workflows-disabled/*.yml .github/workflows/
```

## Disabled Workflows

- **test.yml** - Test suite, coverage, benchmarks, cross-platform testing
- **security.yml** - Security scanning, vulnerability checks, SARIF uploads
- **docs.yml** - Documentation building and deployment
- **release.yml** - Automated release creation and asset publishing

All functionality remains available through local pre-commit hooks and manual processes.