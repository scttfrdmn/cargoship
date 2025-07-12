# Changelog

All notable changes to CargoShip will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Launch agent architecture for containerized deployment on NAS boxes and lab servers
- Research-focused documentation and examples
- Smart dataset detection for automatic archival workflows
- Budget-friendly cost optimization for academic research

### Changed
- Refocused project from enterprise to research use cases
- Simplified documentation structure for academic users
- Updated README with research-focused value proposition

## [0.3.0] - 2024-07-11

### Added
- Multi-region pipeline distribution with intelligent failover
- Advanced failover optimization with circuit breakers and predictive monitoring
- Comprehensive performance benchmarking command
- Production-grade security scanning pipeline
- Complete code signing infrastructure with GPG key management
- Extensive user documentation and security guides

### Changed
- Improved multiregion package coverage from 80% to 85.9%
- Enhanced GPG package test coverage to 88.8%
- Optimized rclone package performance and reliability

### Fixed
- Multiregion coordinator initialization and background services
- Test failures in coordinator validation and health checks
- Memory leaks in connection pooling and health monitoring

## [0.2.0] - 2024-07-10

### Added
- Predictive chunk staging with content analysis
- Network adaptation for optimal transfer performance
- Enhanced staging system with memory-efficient buffering
- Comprehensive test suite with 85%+ coverage
- Advanced compression algorithms (zstd, lz4)
- Multi-threaded upload optimization

### Changed
- Improved staging package coverage from 71.1% to 81.8%
- Enhanced compression package reliability and performance
- Optimized memory usage in chunk staging operations

### Fixed
- Buffer overflow issues in staging operations
- Race conditions in concurrent upload scenarios
- Memory leaks in compression and staging pipelines

## [0.1.0] - 2024-07-09

### Added
- Core AWS S3 integration with native SDK support
- Intelligent cost optimization and storage class selection
- Basic multi-region support and coordination
- Comprehensive AWS configuration and credential management
- Cost estimation and budget tracking
- CloudWatch metrics integration
- Basic CLI interface with survey, estimate, and ship commands

### Changed
- Migrated from rclone to native AWS SDK for improved performance
- Implemented intelligent storage class selection algorithms
- Added comprehensive error handling and retry logic

### Fixed
- S3 multipart upload reliability issues
- AWS credential handling and region selection
- Cost calculation accuracy for different storage classes

---

## Version Schema

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** version when you make incompatible API changes
- **MINOR** version when you add functionality in a backwards compatible manner  
- **PATCH** version when you make backwards compatible bug fixes

### Pre-1.0.0 Development

During pre-1.0.0 development:
- **0.MINOR.PATCH** where MINOR may include breaking changes
- **0.x.0** releases may contain significant new features
- **0.x.y** releases contain bug fixes and small improvements

### Release Types

- **Alpha** (`0.x.0-alpha.1`): Early development, unstable API
- **Beta** (`0.x.0-beta.1`): Feature complete, API stabilizing
- **Release Candidate** (`0.x.0-rc.1`): Production ready candidate
- **Stable** (`0.x.0`): Production ready release