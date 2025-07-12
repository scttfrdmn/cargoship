# CargoShip Security Guide

## Table of Contents

1. [Security Overview](#security-overview)
2. [Threat Model](#threat-model)
3. [Encryption](#encryption)
4. [Authentication & Authorization](#authentication--authorization)
5. [Network Security](#network-security)
6. [Data Protection](#data-protection)
7. [Audit & Compliance](#audit--compliance)
8. [Security Best Practices](#security-best-practices)
9. [Incident Response](#incident-response)
10. [Security Testing](#security-testing)

## Security Overview

CargoShip implements defense-in-depth security principles to protect data at rest, in transit, and during processing. The security model addresses:

- **Data Encryption**: End-to-end encryption using GPG and AWS encryption
- **Access Control**: Fine-grained IAM permissions and authentication
- **Network Security**: TLS encryption and secure communication
- **Audit Logging**: Comprehensive logging for security monitoring
- **Key Management**: Secure key generation, storage, and rotation

## Threat Model

### Assets Protected
- **Primary Data**: Files and directories being archived
- **Metadata**: File system metadata and archive inventories
- **Encryption Keys**: GPG keys and AWS encryption keys
- **Configuration**: CargoShip configuration and credentials
- **Audit Logs**: Security and operational logs

### Threat Actors
1. **External Attackers**: Unauthorized access to data in transit or at rest
2. **Malicious Insiders**: Unauthorized access by users with legitimate access
3. **Cloud Provider Compromise**: Potential compromise of AWS infrastructure
4. **Supply Chain Attacks**: Compromise of dependencies or build process

### Attack Vectors
- Network interception and man-in-the-middle attacks
- Unauthorized access to AWS credentials
- Compromise of GPG private keys
- Exploitation of software vulnerabilities
- Social engineering and phishing

## Encryption

### End-to-End Encryption Architecture

```
[Source Data] → [GPG Encryption] → [Compression] → [TLS Transport] → [AWS S3 Server-Side Encryption]
```

### GPG Encryption

CargoShip uses GPG (GNU Privacy Guard) for client-side encryption:

**Supported Algorithms:**
- **Symmetric**: AES-256, AES-192, AES-128
- **Asymmetric**: RSA (2048-4096 bits), ECC (Curve25519)
- **Hashing**: SHA-256, SHA-512

**Key Generation:**
```bash
# Generate production-grade keys
cargoship create-keys \
  --name "Production Archive Key" \
  --email archives@company.com \
  --key-size 4096 \
  --cipher-algo AES256 \
  --digest-algo SHA256 \
  --expires 2y
```

**Security Features:**
- Keys generated with cryptographically secure random numbers
- Support for key expiration and rotation
- Passphrase protection for private keys
- Hardware security module (HSM) integration

### AWS Server-Side Encryption

**Encryption Options:**
1. **SSE-S3**: Amazon S3 managed keys
2. **SSE-KMS**: AWS Key Management Service
3. **SSE-C**: Customer-provided keys

**Configuration:**
```yaml
aws:
  encryption:
    type: SSE-KMS
    kms_key_id: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
    bucket_key_enabled: true
```

### Encryption at Rest

**Local Temporary Files:**
- All temporary files are encrypted before writing to disk
- Secure deletion of temporary files after processing
- Memory-mapped files use encrypted storage when available

**Configuration Example:**
```yaml
security:
  temp_file_encryption: true
  secure_deletion: true
  memory_protection: true
```

## Authentication & Authorization

### AWS IAM Integration

**Principle of Least Privilege:**
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "CargoShipMinimalAccess",
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject"
            ],
            "Resource": "arn:aws:s3:::archive-bucket-*/*",
            "Condition": {
                "StringEquals": {
                    "s3:x-amz-server-side-encryption": "AES256"
                }
            }
        },
        {
            "Sid": "CargoShipBucketAccess",
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketLocation"
            ],
            "Resource": "arn:aws:s3:::archive-bucket-*"
        }
    ]
}
```

**Multi-Region IAM Policy:**
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "MultiRegionAccess",
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:GetObjectVersion"
            ],
            "Resource": [
                "arn:aws:s3:::archive-bucket-us-east-1/*",
                "arn:aws:s3:::archive-bucket-us-west-2/*",
                "arn:aws:s3:::archive-bucket-eu-west-1/*"
            ]
        },
        {
            "Sid": "CrossRegionReplication",
            "Effect": "Allow",
            "Action": [
                "s3:ReplicateObject",
                "s3:ReplicateDelete"
            ],
            "Resource": [
                "arn:aws:s3:::archive-bucket-*/*"
            ]
        }
    ]
}
```

### Credential Management

**AWS Credentials:**
- Support for IAM roles and instance profiles
- AWS SSO integration
- MFA support for sensitive operations
- Credential rotation and expiration

**GPG Key Security:**
- Private keys stored with passphrase protection
- Support for hardware security modules (HSMs)
- Key escrow for business continuity
- Regular key rotation policies

**Configuration Security:**
```yaml
security:
  credentials:
    use_iam_roles: true
    require_mfa: true
    session_duration: 3600
    rotation_interval: 24h
    
  gpg:
    require_passphrase: true
    hsm_integration: true
    key_escrow_enabled: true
    rotation_policy: 365d
```

## Network Security

### TLS/SSL Configuration

**Transport Security:**
- TLS 1.2+ for all AWS API communications
- Certificate pinning for AWS endpoints
- HSTS (HTTP Strict Transport Security)
- Perfect Forward Secrecy (PFS)

**Configuration:**
```yaml
network:
  tls:
    min_version: "1.2"
    cipher_suites:
      - "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
      - "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
    certificate_pinning: true
    verify_peer: true
```

### Network Monitoring

**Security Monitoring:**
- Network traffic analysis
- Anomaly detection for data transfer patterns
- Bandwidth monitoring and alerting
- Connection security validation

**Implementation:**
```yaml
monitoring:
  network:
    enabled: true
    anomaly_detection: true
    bandwidth_alerting: true
    connection_logging: true
    
  security:
    failed_auth_threshold: 5
    unusual_access_patterns: true
    geo_location_monitoring: true
```

## Data Protection

### Data Classification

**Classification Levels:**
1. **Public**: Non-sensitive data
2. **Internal**: Company internal data
3. **Confidential**: Sensitive business data
4. **Restricted**: Highly sensitive data (PII, financial)

**Handling Requirements:**
```yaml
data_classification:
  public:
    encryption_required: false
    compression: true
    retention_days: 2555  # 7 years
    
  internal:
    encryption_required: true
    compression: true
    retention_days: 2555
    access_logging: true
    
  confidential:
    encryption_required: true
    strong_encryption: true
    compression: true
    retention_days: 2555
    access_logging: true
    approval_required: true
    
  restricted:
    encryption_required: true
    strong_encryption: true
    compression: false  # Avoid compression side-channels
    retention_days: 2555
    access_logging: true
    approval_required: true
    dual_person_control: true
```

### Data Loss Prevention (DLP)

**Content Scanning:**
- Pattern recognition for sensitive data
- PII detection and masking
- Credit card and SSN detection
- Custom pattern definitions

**Configuration:**
```yaml
dlp:
  enabled: true
  scan_content: true
  patterns:
    - name: "SSN"
      regex: '\b\d{3}-\d{2}-\d{4}\b'
      action: "block"
    - name: "Credit Card"
      regex: '\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b'
      action: "mask"
    - name: "Email"
      regex: '\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b'
      action: "log"
      
  actions:
    block: "Prevent archiving"
    mask: "Replace with asterisks"
    log: "Log occurrence"
    encrypt: "Force encryption"
```

### Data Integrity

**Integrity Verification:**
- SHA-256 checksums for all archived data
- Merkle tree verification for large archives
- Periodic integrity checks
- Corruption detection and recovery

**Implementation:**
```yaml
integrity:
  checksums:
    algorithm: "SHA-256"
    verify_on_upload: true
    verify_on_download: true
    store_checksums: true
    
  verification:
    periodic_checks: true
    check_interval: "30d"
    corruption_handling: "alert_and_quarantine"
    
  recovery:
    redundant_copies: 2
    cross_region_backup: true
    point_in_time_recovery: true
```

## Audit & Compliance

### Audit Logging

**Comprehensive Logging:**
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "event_type": "archive_created",
  "user_id": "john.doe@company.com",
  "source_ip": "192.168.1.100",
  "source_path": "/data/financial/2024/q1",
  "destination": "s3://archive-bucket/financial/2024-q1.tar.gz.gpg",
  "file_size": 1073741824,
  "compression_ratio": 0.65,
  "encryption_key_id": "archives@company.com",
  "checksum": "sha256:a1b2c3d4e5f6...",
  "duration_ms": 45000,
  "success": true,
  "metadata": {
    "classification": "confidential",
    "retention_policy": "7years",
    "approved_by": "jane.manager@company.com"
  }
}
```

**Security Events:**
- Authentication attempts (success/failure)
- Authorization decisions
- Encryption/decryption operations
- Key usage and rotation
- Configuration changes
- Anomalous activities

### Compliance Frameworks

**SOX (Sarbanes-Oxley) Compliance:**
- Immutable audit trails
- Segregation of duties
- Data retention policies
- Financial data controls

**GDPR Compliance:**
- Data subject rights (access, deletion)
- Lawful basis for processing
- Data minimization
- Privacy by design

**HIPAA Compliance:**
- PHI protection controls
- Access logging and monitoring
- Encryption requirements
- Breach notification procedures

**Configuration:**
```yaml
compliance:
  frameworks:
    - sox
    - gdpr
    - hipaa
    
  controls:
    immutable_logs: true
    segregation_of_duties: true
    data_retention_enforcement: true
    privacy_by_design: true
    breach_detection: true
    
  reporting:
    automated_reports: true
    compliance_dashboard: true
    audit_trail_export: true
```

## Security Best Practices

### Deployment Security

**Secure Configuration:**
```yaml
security:
  hardening:
    disable_debug_logs_in_production: true
    use_secure_defaults: true
    validate_all_inputs: true
    sanitize_error_messages: true
    
  access_control:
    principle_of_least_privilege: true
    role_based_access: true
    time_limited_sessions: true
    require_approval_for_sensitive_ops: true
    
  monitoring:
    security_event_logging: true
    anomaly_detection: true
    automated_incident_response: true
    integration_with_siem: true
```

### Key Management Best Practices

1. **Key Generation:**
   - Use cryptographically secure random number generators
   - Generate keys in secure environments
   - Use appropriate key sizes (RSA 4096+, ECC 256+)

2. **Key Storage:**
   - Store private keys in hardware security modules when possible
   - Use strong passphrases for key protection
   - Implement key escrow for business continuity

3. **Key Rotation:**
   - Regular rotation schedules (annually for production keys)
   - Automated rotation where possible
   - Maintain old keys for decryption of existing archives

4. **Key Distribution:**
   - Secure channels for public key distribution
   - Certificate authorities for key validation
   - Out-of-band verification of key fingerprints

### Operational Security

**Secure Operations:**
```bash
# Use dedicated service accounts
sudo -u cargoship-service cargoship create /data --bucket secure-bucket

# Enable audit logging
export CARGOSHIP_AUDIT_LOG=/var/log/cargoship/audit.log

# Use configuration validation
cargoship config validate --security-check

# Monitor for security events
tail -f /var/log/cargoship/security.log | grep -E "(FAILED|ERROR|UNAUTHORIZED)"
```

**Environment Separation:**
```yaml
environments:
  development:
    encryption_required: false
    audit_logging: minimal
    test_data_only: true
    
  staging:
    encryption_required: true
    audit_logging: full
    production_like_security: true
    
  production:
    encryption_required: true
    audit_logging: full
    security_hardening: maximum
    compliance_controls: enabled
```

## Incident Response

### Security Incident Types

1. **Data Breach**: Unauthorized access to archived data
2. **Key Compromise**: Private encryption keys compromised
3. **System Compromise**: CargoShip system compromised
4. **Credential Theft**: AWS or GPG credentials stolen

### Response Procedures

**Immediate Response (0-1 hours):**
1. Identify and contain the incident
2. Assess the scope and impact
3. Notify security team and stakeholders
4. Preserve evidence and logs

**Short-term Response (1-24 hours):**
1. Investigate root cause
2. Implement additional safeguards
3. Revoke compromised credentials
4. Rotate encryption keys if necessary

**Long-term Response (1-30 days):**
1. Conduct thorough security review
2. Implement preventive measures
3. Update security policies and procedures
4. Provide security awareness training

### Incident Response Playbooks

**Data Breach Response:**
```bash
# 1. Immediately revoke access
aws iam put-user-policy --user-name compromised-user --policy-name DenyAll --policy-document file://deny-all.json

# 2. Enable CloudTrail logging if not already enabled
aws cloudtrail create-trail --name incident-response-trail --s3-bucket-name incident-logs

# 3. Generate access report
cargoship analyze access --bucket affected-bucket --since "2024-01-01" --format json > breach-analysis.json

# 4. Verify data integrity
cargoship verify --bucket affected-bucket --checksum-validation --report integrity-check.json
```

**Key Compromise Response:**
```bash
# 1. Generate new encryption keys
cargoship create-keys --name "Emergency Key $(date +%Y%m%d)" --key-size 4096

# 2. Re-encrypt affected archives with new key
cargoship re-encrypt --bucket affected-bucket --old-key compromised@company.com --new-key emergency@company.com

# 3. Revoke compromised key
gpg --edit-key compromised@company.com
# > revkey
# > save

# 4. Update configuration
cargoship config update --gpg-key emergency@company.com
```

## Security Testing

### Static Code Analysis

**Tools and Configuration:**
```yaml
security_testing:
  static_analysis:
    tools:
      - gosec
      - semgrep
      - codecov
    rules:
      - hardcoded_credentials
      - sql_injection
      - path_traversal
      - crypto_misuse
    fail_on: "medium"
    
  dependency_scanning:
    tools:
      - snyk
      - nancy
      - trivy
    vulnerability_threshold: "high"
    license_compliance: true
```

### Dynamic Security Testing

**Penetration Testing:**
```bash
# Network security testing
nmap -sV -sC cargoship-server.company.com

# Application security testing
./security-tests/run-pentest.sh --target cargoship --depth full

# Encryption testing
./security-tests/crypto-analysis.sh --test-gpg --test-tls
```

### Security Automation

**CI/CD Security Pipeline:**
```yaml
name: Security Pipeline
on: [push, pull_request]

jobs:
  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Run Gosec Security Scanner
        uses: securecodewarrior/github-action-gosec@master
        
      - name: Run Semgrep
        uses: returntocorp/semgrep-action@v1
        
      - name: Dependency Check
        uses: dependency-check/Dependency-Check_Action@main
        
      - name: Upload Security Report
        uses: github/codeql-action/upload-sarif@v1
        with:
          sarif_file: security-report.sarif
```

### Compliance Testing

**Automated Compliance Checks:**
```bash
# Run compliance validation
cargoship compliance check --framework sox,gdpr,hipaa

# Generate compliance report
cargoship compliance report --output compliance-status.json

# Validate security controls
cargoship security validate --controls encryption,access,logging
```

---

## Security Contact

For security issues, vulnerabilities, or questions:

- **Security Email**: security@cargoship.dev
- **GPG Key**: Available at https://keybase.io/cargoship
- **Security Advisory**: Check GitHub Security Advisories

## Security Changelog

- **v1.0.0**: Initial security implementation
- **v1.1.0**: Added multi-region security controls
- **v1.2.0**: Enhanced audit logging and compliance features
- **v1.3.0**: Improved key management and HSM integration

---

*This security guide is regularly updated. Last review: 2024-01-15*