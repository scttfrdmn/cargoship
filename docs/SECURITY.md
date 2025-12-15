# CargoShip Security Guide

## Overview

CargoShip provides enterprise-grade encryption for data at rest using AWS Key Management Service (KMS). This guide covers encryption features, setup, and best practices.

## Table of Contents

- [Encryption Features](#encryption-features)
- [Quick Start](#quick-start)
- [KMS Setup](#kms-setup)
- [Encryption Architecture](#encryption-architecture)
- [CLI Usage](#cli-usage)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Encryption Features

CargoShip supports two levels of encryption:

### 1. Data Chunk Encryption (SSE-KMS)
- **What**: Encrypts uploaded data chunks (tar.zst archives) at rest in S3
- **How**: Server-Side Encryption with AWS KMS (SSE-KMS)
- **When**: Automatic when `--kms-key-id` flag is provided
- **Location**: S3 bucket (managed by AWS)

### 2. Manifest Encryption (Envelope Encryption)
- **What**: Encrypts manifest metadata files
- **How**: Client-side envelope encryption with AES-256-GCM
- **When**: Enabled with `--encrypt-manifest` flag
- **Location**: Encrypted locally before upload to S3

**Combined Protection**: Use both features together for end-to-end encryption of all uploaded data.

---

## Quick Start

### Basic Encrypted Upload

```bash
# Upload with KMS encryption (data chunks only)
cargoship upload ./mydata s3://my-bucket/uploads \
  --kms-key-id arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012

# Upload with full encryption (data chunks + manifest)
cargoship upload ./mydata s3://my-bucket/uploads \
  --kms-key-id arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012 \
  --encrypt-manifest
```

### Download Encrypted Data

```bash
# Download commands automatically detect and decrypt encrypted manifests
cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./restored

# List encrypted manifest contents
cargoship list s3://my-bucket/uploads/20231208-123456-abcd1234

# View encrypted manifest metadata
cargoship info s3://my-bucket/uploads/20231208-123456-abcd1234
```

---

## KMS Setup

### Prerequisites

- AWS Account with KMS access
- IAM permissions for KMS operations
- CargoShip v0.6.2 or later

### Step 1: Create KMS Key

**Option A: AWS Console**
1. Go to AWS KMS Console
2. Click "Create key"
3. Key type: Symmetric
4. Key usage: Encrypt and decrypt
5. Key spec: SYMMETRIC_DEFAULT
6. Alias: `cargoship-encryption-key`
7. Set key administrators and users

**Option B: AWS CLI**
```bash
# Create KMS key
aws kms create-key \
  --description "CargoShip data encryption key" \
  --key-usage ENCRYPT_DECRYPT \
  --origin AWS_KMS

# Create alias (optional but recommended)
aws kms create-alias \
  --alias-name alias/cargoship-encryption \
  --target-key-id <key-id>
```

### Step 2: Configure IAM Permissions

Attach this policy to your IAM user/role:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CargoShipKMSUpload",
      "Effect": "Allow",
      "Action": [
        "kms:GenerateDataKey",
        "kms:Decrypt",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:us-west-2:123456789012:key/*"
    }
  ]
}
```

**Minimum Permissions**:
- **Upload**: `kms:GenerateDataKey`, `kms:DescribeKey`
- **Download**: `kms:Decrypt`, `kms:DescribeKey`

### Step 3: Configure S3 Bucket Policy (Optional)

Enforce encryption for all uploads:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DenyUnencryptedObjectUploads",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::my-bucket/*",
      "Condition": {
        "StringNotEquals": {
          "s3:x-amz-server-side-encryption": "aws:kms"
        }
      }
    }
  ]
}
```

---

## Encryption Architecture

### SSE-KMS (Data Chunks)

**Flow**:
1. CargoShip creates compressed tar.zst archive (data chunk)
2. S3 client uploads with `ServerSideEncryption: aws:kms` header
3. AWS KMS generates Data Encryption Key (DEK)
4. S3 encrypts chunk with DEK
5. S3 stores encrypted chunk + encrypted DEK

**Storage**:
```
s3://bucket/prefix/shard-0/chunk-001.tar.zst  [encrypted by AWS]
├─ Data: AES-256 encrypted with DEK
└─ Metadata: SSEKMSKeyId, ServerSideEncryption=aws:kms
```

**Benefits**:
- Zero performance overhead (handled by AWS)
- No client-side key management
- Integrated with AWS CloudTrail for audit logs

### Envelope Encryption (Manifests)

**Encryption Flow**:
```
1. Generate DEK using KMS.GenerateDataKey()
   └─ Returns: Plaintext DEK (32 bytes) + Encrypted DEK (ciphertext)

2. Encrypt manifest JSON with DEK using AES-256-GCM
   └─ Produces: Encrypted data + IV (nonce)

3. Upload to S3:
   {
     "algorithm": "AES-256-GCM",
     "kms_key_id": "arn:aws:kms:...",
     "encrypted_dek": "<base64>",  // Encrypted by KMS
     "iv": "<base64>",
     "encrypted_data": "<base64>"
   }
```

**Decryption Flow**:
```
1. Download encrypted manifest from S3

2. Decrypt DEK using KMS.Decrypt()
   └─ Returns: Plaintext DEK

3. Decrypt manifest data using DEK with AES-256-GCM
   └─ Returns: Plaintext manifest JSON
```

**Storage**:
```
s3://bucket/prefix/uploads/{uploadID}/
├─ manifest.encrypted.json.gz  [envelope encrypted]
├─ manifest.json.gz             [fallback - not encrypted]
└─ chunks/...                   [SSE-KMS encrypted]
```

**Benefits**:
- Client-side encryption (data never stored unencrypted)
- AES-256-GCM provides authenticated encryption (integrity + confidentiality)
- DEK never stored in plaintext (encrypted by KMS CMK)

---

## CLI Usage

### Upload Commands

```bash
# Data chunk encryption only
cargoship upload ./data s3://bucket/prefix \
  --kms-key-id arn:aws:kms:us-west-2:123456789012:key/abcd1234

# Full encryption (data + manifest)
cargoship upload ./data s3://bucket/prefix \
  --kms-key-id arn:aws:kms:us-west-2:123456789012:key/abcd1234 \
  --encrypt-manifest

# Using KMS key alias
cargoship upload ./data s3://bucket/prefix \
  --kms-key-id alias/cargoship-encryption \
  --encrypt-manifest
```

### Download Commands

```bash
# Download with automatic decryption
cargoship download s3://bucket/prefix/uploads/20231208-123456-abcd1234 ./restored

# Selective download (pattern matching)
cargoship download s3://bucket/prefix/uploads/20231208-123456-abcd1234 ./logs \
  --pattern "*.log"

# Specific files
cargoship download s3://bucket/prefix/uploads/20231208-123456-abcd1234 ./reports \
  --files "data/report.csv,data/summary.csv"
```

### Query Commands

```bash
# List files (reads encrypted manifest)
cargoship list s3://bucket/prefix/uploads/20231208-123456-abcd1234

# View metadata (reads encrypted manifest)
cargoship info s3://bucket/prefix/uploads/20231208-123456-abcd1234

# JSON output for scripting
cargoship info s3://bucket/prefix/uploads/20231208-123456-abcd1234 --json
```

---

## Best Practices

### Key Management

1. **Use Separate Keys for Different Environments**
   ```bash
   # Production
   --kms-key-id alias/cargoship-prod

   # Staging
   --kms-key-id alias/cargoship-staging
   ```

2. **Enable Key Rotation**
   ```bash
   aws kms enable-key-rotation --key-id <key-id>
   ```

3. **Use Key Aliases** (easier to rotate keys)
   ```bash
   # Good: Using alias
   --kms-key-id alias/cargoship-encryption

   # Avoid: Hardcoding key ID
   --kms-key-id arn:aws:kms:us-west-2:123456789012:key/abcd1234
   ```

### IAM Permissions

1. **Follow Least Privilege Principle**
   - Upload users: `kms:GenerateDataKey` only
   - Download users: `kms:Decrypt` only
   - Admins: Full KMS permissions

2. **Use Resource-Based Policies**
   ```json
   {
     "Condition": {
       "StringEquals": {
         "kms:ViaService": "s3.us-west-2.amazonaws.com"
       }
     }
   }
   ```

3. **Enable CloudTrail Logging**
   ```bash
   aws cloudtrail create-trail \
     --name cargoship-kms-audit \
     --s3-bucket-name my-audit-bucket
   ```

### Encryption Best Practices

1. **Always Encrypt Manifests in Production**
   ```bash
   # Manifests contain file metadata (paths, sizes, timestamps)
   cargoship upload ./data s3://bucket/prefix \
     --kms-key-id $KMS_KEY_ID \
     --encrypt-manifest  # Always use this in production
   ```

2. **Test Decryption Before Production**
   ```bash
   # Upload test data
   cargoship upload ./test s3://bucket/test \
     --kms-key-id $KMS_KEY_ID --encrypt-manifest

   # Verify download works
   cargoship download s3://bucket/test/uploads/<upload-id> ./restored
   ```

3. **Monitor KMS API Calls**
   ```bash
   # Check KMS usage in CloudTrail
   aws cloudtrail lookup-events \
     --lookup-attributes AttributeKey=EventName,AttributeValue=GenerateDataKey
   ```

### Cost Optimization

**KMS Pricing** (as of 2024):
- Customer Master Key (CMK): $1/month
- API Requests: $0.03 per 10,000 requests

**Cost Example**:
```
Upload 1 TB with 8 shards, 64MB chunks:
- Chunks: ~16,384 chunks = ~16,384 GenerateDataKey calls
- Manifest: 1 GenerateDataKey + 1 Encrypt call
- Total KMS requests: ~16,385
- Cost: $0.05 (plus $1/month for CMK)
```

**Optimization Tips**:
1. Use larger chunk sizes to reduce KMS calls
2. Batch uploads when possible
3. Use S3 lifecycle policies to transition old data to Glacier
4. Consider AWS KMS request caching (built-in, 5-minute cache)

---

## Troubleshooting

### Common Errors

#### 1. Access Denied (KMS)

**Error**:
```
failed to generate data key from KMS: AccessDeniedException
```

**Solution**:
```bash
# Check IAM permissions
aws kms get-key-policy --key-id <key-id> --policy-name default

# Verify your identity
aws sts get-caller-identity

# Test KMS access
aws kms describe-key --key-id <key-id>
```

#### 2. Key Not Found

**Error**:
```
failed to generate data key from KMS: NotFoundException
```

**Solution**:
```bash
# List available keys
aws kms list-keys

# Check key exists in correct region
aws kms describe-key --key-id <key-id> --region us-west-2
```

#### 3. Manifest Decryption Failed

**Error**:
```
failed to decrypt manifest data (authentication failed)
```

**Causes**:
- Corrupted manifest file
- Wrong KMS key used
- Manifest tampered with (GCM authentication fails)

**Solution**:
```bash
# Try downloading from different region
cargoship info s3://bucket/prefix/uploads/<upload-id> --region us-west-2

# Check S3 object integrity
aws s3api head-object --bucket bucket --key prefix/uploads/<upload-id>/manifest.encrypted.json.gz
```

#### 4. Invalid Ciphertext

**Error**:
```
failed to decrypt DEK using KMS: InvalidCiphertextException
```

**Solution**:
- Verify you're using the same KMS key used for encryption
- Check if key was deleted or disabled
- Ensure encrypted DEK wasn't corrupted

### Debug Mode

Enable verbose logging:
```bash
export AWS_SDK_LOG_LEVEL=debug
cargoship upload ./data s3://bucket/prefix --kms-key-id <key-id> --encrypt-manifest
```

### Verify Encryption

**Check S3 Object Encryption**:
```bash
aws s3api head-object \
  --bucket my-bucket \
  --key prefix/shard-0/chunk-001.tar.zst \
  | jq '.ServerSideEncryption, .SSEKMSKeyId'
```

**Verify Manifest Encryption**:
```bash
# Download encrypted manifest
aws s3 cp s3://bucket/prefix/uploads/<upload-id>/manifest.encrypted.json.gz - \
  | gunzip | jq '.algorithm, .kms_key_id'

# Should show:
# "AES-256-GCM"
# "arn:aws:kms:..."
```

---

## Compliance & Certifications

### Standards Supported

- **FIPS 140-2**: AWS KMS uses FIPS 140-2 validated HSMs
- **HIPAA**: KMS is HIPAA eligible
- **PCI DSS**: Supports PCI DSS Level 1 compliance
- **SOC 2**: AWS KMS is SOC 2 compliant

### Audit Logging

All KMS operations are logged to CloudTrail:
```bash
# View KMS operations
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=EventSource,AttributeValue=kms.amazonaws.com \
  --start-time 2024-01-01 \
  --end-time 2024-01-31
```

### Data Residency

- KMS keys are regional (data never leaves the region)
- S3 bucket must be in the same region as KMS key for SSE-KMS
- Client-side envelope encryption allows cross-region uploads

---

## FAQ

### Q: Can I use the same KMS key for multiple buckets?
**A**: Yes, one KMS key can encrypt data in multiple S3 buckets.

### Q: What happens if I lose access to the KMS key?
**A**: You cannot decrypt the data. Always maintain KMS key access and consider:
- Key aliases for rotation
- Cross-account key access
- Backup key policies

### Q: Can I migrate from unencrypted to encrypted uploads?
**A**: Yes, encryption is opt-in per upload. Mix encrypted and unencrypted uploads in the same bucket.

### Q: Is there a performance impact?
**A**: Minimal. SSE-KMS has no client-side overhead. Envelope encryption adds ~10ms per manifest operation.

### Q: Can I use customer-provided keys (SSE-C)?
**A**: Not currently. CargoShip uses SSE-KMS for AWS-managed encryption.

### Q: How do I rotate KMS keys?
**A**: AWS KMS supports automatic key rotation (yearly). Enable with:
```bash
aws kms enable-key-rotation --key-id <key-id>
```

### Q: Are manifest partial saves encrypted?
**A**: Yes, if `--encrypt-manifest` is used, partial manifests are also encrypted.

---

## Additional Resources

- [AWS KMS Developer Guide](https://docs.aws.amazon.com/kms/latest/developerguide/)
- [S3 Encryption Best Practices](https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingEncryption.html)
- [CargoShip Deployment Guide](./DEPLOYMENT_GUIDE.md)
- [Issue #163: KMS Encryption Implementation](https://github.com/scttfrdmn/cargoship/issues/163)

---

## Support

For security-related issues or questions:
- GitHub Issues: https://github.com/scttfrdmn/cargoship/issues
- Security: Report vulnerabilities via GitHub Security Advisories

**Version**: CargoShip v0.6.2+
**Last Updated**: 2024-12-15
