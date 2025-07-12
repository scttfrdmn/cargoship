# Security Policy

## Overview

CargoShip takes security seriously. This document outlines our security practices, vulnerability reporting process, and supported versions for security updates.

## Supported Versions

We provide security updates for the following versions of CargoShip:

| Version | Supported          |
| ------- | ------------------ |
| 0.3.x   | :white_check_mark: |
| 0.2.x   | :x:                |
| < 0.2   | :x:                |

## Security Scanning and Monitoring

CargoShip implements comprehensive security measures:

### Automated Security Scanning
- **Daily vulnerability scans** using `govulncheck`
- **Static security analysis** with `gosec` 
- **Dependency vulnerability monitoring** with Nancy and Trivy
- **CodeQL security analysis** for advanced threat detection
- **Semgrep security patterns** for additional vulnerability detection
- **License compliance checking** with go-licenses
- **Supply chain security** with SBOM generation and scanning

### Pre-commit Security Validation
- **Vulnerability scanning** before every commit
- **Security linting** with gosec rules
- **Zero-tolerance policy** for known vulnerabilities
- **Dependency verification** and consistency checks

### Dependency Management
- **Automated dependency updates** via Dependabot
- **Weekly security updates** for all dependencies
- **Grouped security patches** for immediate deployment
- **Vulnerability alerts** for critical issues

## Security Features

### Data Protection
- **End-to-end encryption** using OpenPGP (ProtonMail/gopenpgp)
- **Secure key management** with gpg integration
- **TLS encryption** for all network communications
- **Secure credential handling** with AWS SDK best practices

### Access Controls
- **Bearer token authentication** for agent connections
- **Role-based access control** in multi-agent environments
- **Secure WebSocket connections** with authentication
- **Environment-based configuration** to prevent credential exposure

### Audit and Monitoring
- **Comprehensive logging** of all security events
- **Activity tracking** for agent connections and operations
- **Error reporting** without sensitive data exposure
- **Performance monitoring** with security metrics

## Reporting a Vulnerability

If you discover a security vulnerability in CargoShip, please follow these steps:

### 1. Do Not Create Public Issues
**Please do not report security vulnerabilities through public GitHub issues.**

### 2. Contact Information
Send detailed vulnerability reports to: **security@cargoship.dev**

### 3. Required Information
Please include the following information:
- **Description** of the vulnerability
- **Steps to reproduce** the issue
- **Potential impact** assessment
- **Suggested remediation** (if available)
- **Your contact information** for follow-up

### 4. Response Timeline
- **Initial acknowledgment**: Within 24 hours
- **Initial assessment**: Within 72 hours
- **Regular updates**: Every 5 business days
- **Resolution target**: Based on severity (see below)

## Vulnerability Severity and Response Times

| Severity | Description | Response Time | Patch Timeline |
|----------|-------------|---------------|----------------|
| **Critical** | Remote code execution, privilege escalation | 24 hours | 7 days |
| **High** | Data exposure, authentication bypass | 72 hours | 14 days |
| **Medium** | Limited data exposure, DoS vulnerabilities | 5 days | 30 days |
| **Low** | Information disclosure, minor security issues | 10 days | 60 days |

## Security Best Practices for Users

### Installation and Updates
- Always use the latest supported version
- Enable automatic security updates where possible
- Verify package signatures and checksums
- Download only from official sources

### Configuration Security
- Use strong, unique authentication tokens
- Enable TLS for all network communications
- Regularly rotate credentials and keys
- Follow principle of least privilege for permissions

### Operational Security
- Monitor logs for suspicious activity
- Implement network segmentation
- Use dedicated service accounts
- Regular security assessments

### Data Protection
- Encrypt sensitive data at rest
- Use secure backup practices
- Implement proper data retention policies
- Regular access reviews

## Security Hardening

### Environment Setup
```bash
# Enable comprehensive security scanning
export CARGOSHIP_SECURITY_MODE=strict

# Use TLS for all connections
export CARGOSHIP_TLS_REQUIRED=true

# Enable audit logging
export CARGOSHIP_AUDIT_LOG=enabled
```

### Agent Configuration
```yaml
# Secure agent configuration
security:
  tls_enabled: true
  auth_required: true
  audit_logging: true
  strict_mode: true
```

## Third-Party Security Tools

CargoShip integrates with industry-standard security tools:

- **govulncheck**: Go vulnerability database scanning
- **gosec**: Static security analysis for Go
- **Nancy**: Dependency vulnerability scanning
- **Trivy**: Container and filesystem vulnerability scanning
- **CodeQL**: Semantic code analysis
- **Semgrep**: Static analysis security patterns

## Compliance and Standards

CargoShip follows security best practices including:

- **OWASP Top 10** mitigation strategies
- **CIS Security Benchmarks** where applicable
- **NIST Cybersecurity Framework** principles
- **Supply Chain Security** best practices

## Security Resources

- [Go Security Guidelines](https://golang.org/security/)
- [AWS Security Best Practices](https://aws.amazon.com/architecture/security-identity-compliance/)
- [OWASP Go Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Go_SCP_Cheat_Sheet.html)

## Contact

For general security questions or concerns:
- **Email**: security@cargoship.dev
- **GitHub**: Create a private security advisory
- **Documentation**: Check our security documentation

---

**Last Updated**: December 2024
**Next Review**: Quarterly