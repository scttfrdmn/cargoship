# CargoShip Release Process

This document describes how to create and manage releases for the CargoShip project.

## Overview

CargoShip uses automated GitHub Actions to build and release cross-platform binaries. The release process supports:

- **Linux**: x86_64 and ARM64
- **macOS**: x86_64 (Intel) and ARM64 (Apple Silicon)  
- **Windows**: x86_64

All binaries are statically compiled with no external dependencies.

## Supported Platforms

| Platform | Architecture | Filename | Format |
|----------|-------------|----------|---------|
| Linux | x86_64 | `cargoship-linux-amd64.tar.gz` | tar.gz |
| Linux | ARM64 | `cargoship-linux-arm64.tar.gz` | tar.gz |
| macOS | x86_64 | `cargoship-darwin-amd64.tar.gz` | tar.gz |
| macOS | ARM64 | `cargoship-darwin-arm64.tar.gz` | tar.gz |
| Windows | x86_64 | `cargoship-windows-amd64.zip` | zip |

## Automated Release Process

### 1. Create a Release Tag

```bash
# Tag the current commit with a semantic version
git tag v1.0.0

# Push the tag to trigger the release
git push origin v1.0.0
```

### 2. GitHub Actions Workflow

The release workflow (`.github/workflows/release.yml`) automatically:

1. **Builds** binaries for all supported platforms
2. **Tests** the code before building
3. **Creates** compressed archives (tar.gz for Unix, zip for Windows)
4. **Uploads** binaries to GitHub Releases
5. **Generates** SHA256 checksums for all artifacts
6. **Updates** documentation with the new version

### 3. Manual Release (if needed)

You can also trigger a release manually:

```bash
# Using GitHub CLI
gh workflow run release.yml -f tag=v1.0.0

# Or use the GitHub web interface:
# Go to Actions → Release Binaries → Run workflow
```

## Local Testing

### Build All Platforms Locally

Use the provided build script to test releases locally:

```bash
# Build with version tag
./scripts/build-release.sh v1.0.0

# Build development version
./scripts/build-release.sh dev
```

### Test Local Build

```bash
# Extract and test (example for macOS ARM64)
tar -xzf dist/cargoship-darwin-arm64.tar.gz
./cargoship-darwin-arm64 --version

# Should output something like:
# cargoship version v1.0.0 (abcd1234) built on 2024-06-30T12:00:00Z
```

## Version Information

Binaries include build information:

- **Version**: Git tag (e.g., `v1.0.0`)
- **Commit**: Short commit hash (e.g., `abcd1234`)
- **Build Date**: ISO 8601 timestamp (e.g., `2024-06-30T12:00:00Z`)

Access version info:

```bash
cargoship --version
cargoship version
```

## Release Checklist

Before creating a release:

- [ ] All tests pass: `go test ./...`
- [ ] Linting passes: `golangci-lint run`
- [ ] Documentation is updated
- [ ] CHANGELOG.md is updated (if exists)
- [ ] Version follows semantic versioning (e.g., v1.2.3)

## Troubleshooting

### Release Failed

Check the GitHub Actions logs:
1. Go to the [Actions tab](https://github.com/scttfrdmn/cargoship/actions)
2. Click on the failed "Release Binaries" workflow
3. Review the build logs for errors

### Binary Not Working

Verify the binary:

```bash
# Check if binary is executable
chmod +x cargoship-*

# Verify it's not corrupted
./cargoship-* --version

# Check dependencies (should show no external dependencies)
ldd cargoship-* # Linux
otool -L cargoship-* # macOS
```

### Checksums Don't Match

Download the `checksums.txt` file from the release and verify:

```bash
# Download binary and checksums
curl -LO https://github.com/scttfrdmn/cargoship/releases/download/v1.0.0/cargoship-linux-amd64.tar.gz
curl -LO https://github.com/scttfrdmn/cargoship/releases/download/v1.0.0/checksums.txt

# Verify checksum
sha256sum cargoship-linux-amd64.tar.gz
grep cargoship-linux-amd64.tar.gz checksums.txt
```

## Installation Scripts

The release process automatically updates installation documentation with download URLs. Users can install using:

### Using curl (Unix/Linux/macOS)

```bash
# Automated installer (when available)
curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/install.sh | sh

# Manual download
curl -LO https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64.tar.gz
tar -xzf cargoship-linux-amd64.tar.gz
sudo mv cargoship-linux-amd64 /usr/local/bin/cargoship
```

### Using PowerShell (Windows)

```powershell
# Download and extract
Invoke-WebRequest -Uri "https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-windows-amd64.zip" -OutFile "cargoship.zip"
Expand-Archive -Path "cargoship.zip" -DestinationPath "C:\Program Files\CargoShip"
# Add to PATH
```

## Security

- All binaries are built in GitHub's secure environment
- Checksums are provided for integrity verification
- Binaries are statically compiled to reduce supply chain risks
- No external dependencies or dynamic linking

## Support

For release-related issues:

- **GitHub Issues**: [Report problems](https://github.com/scttfrdmn/cargoship/issues)
- **Documentation**: [Full documentation](https://cargoship.app)
- **Discussions**: [Community support](https://github.com/scttfrdmn/cargoship/discussions)

---

**Note**: This release process is designed for maintainers. End users should use the installation methods described in [docs/install.md](docs/install.md).