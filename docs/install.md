# Installation Guide

## Quick Install

### Using Go Install (Recommended)

If you have Go installed, this is the fastest way to get CargoShip:

```bash
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

### Using Pre-built Binaries

Download the latest release for your platform:

=== "Linux"
    ```bash
    # Linux x86_64
    curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64.tar.gz | tar -xz
    sudo mv cargoship-linux-amd64 /usr/local/bin/cargoship
    
    # Linux ARM64
    curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-arm64.tar.gz | tar -xz
    sudo mv cargoship-linux-arm64 /usr/local/bin/cargoship
    ```

=== "macOS"
    ```bash
    # macOS (Intel)
    curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-amd64.tar.gz | tar -xz
    sudo mv cargoship-darwin-amd64 /usr/local/bin/cargoship
    
    # macOS (Apple Silicon)
    curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-arm64.tar.gz | tar -xz
    sudo mv cargoship-darwin-arm64 /usr/local/bin/cargoship
    ```

=== "Windows"
    ```powershell
    # Download and extract
    Invoke-WebRequest -Uri "https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-windows-amd64.zip" -OutFile "cargoship.zip"
    Expand-Archive -Path "cargoship.zip" -DestinationPath "C:\Program Files\CargoShip"
    # Rename binary and add to PATH
    Rename-Item "C:\Program Files\CargoShip\cargoship-windows-amd64.exe" "cargoship.exe"
    # Add C:\Program Files\CargoShip to your PATH environment variable
    ```

### Using Docker

Run CargoShip in a container without installing:

```bash
# Survey data
docker run --rm -v $(pwd):/data scttfrdmn/cargoship:latest survey /data

# Ship to AWS (requires AWS credentials)
docker run --rm \
  -v $(pwd):/data \
  -v ~/.aws:/root/.aws \
  scttfrdmn/cargoship:latest ship /data \
  --destination s3://my-bucket/archives
```

## Requirements

### System Requirements

- **OS**: Linux, macOS, or Windows
- **Architecture**: x86_64 or ARM64
- **Memory**: 512MB minimum, 2GB recommended for large archives
- **Disk Space**: Temporary space equal to largest archive being created

### AWS Requirements

- **AWS Account** with appropriate S3 permissions
- **AWS CLI** configured or environment variables set
- **IAM Permissions** for S3, CloudWatch, and optionally KMS

## AWS Setup

### 1. Install AWS CLI

=== "Linux/macOS"
    ```bash
    curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
    unzip awscliv2.zip
    sudo ./aws/install
    ```

=== "Windows"
    ```powershell
    # Download and run the AWS CLI MSI installer from:
    # https://awscli.amazonaws.com/AWSCLIV2.msi
    ```

### 2. Configure AWS Credentials

```bash
aws configure
```

Or set environment variables:

```bash
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_DEFAULT_REGION=us-east-1
```

### 3. Required IAM Permissions

Create an IAM policy with these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket",
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:GetBucketLocation",
        "s3:ListAllMyBuckets"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name",
        "arn:aws:s3:::your-bucket-name/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "cloudwatch:PutMetricData",
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:*:*:key/*"
    }
  ]
}
```

## Verification

Test your installation:

```bash
# Check version
cargoship --version

# Test AWS connectivity
cargoship survey /tmp --dry-run

# Verify configuration
cargoship config show
```

## Next Steps

1. **[Run the Setup Wizard](wizard.md)** - Interactive configuration
2. **[Basic Usage](index.md#basic-usage)** - Common commands
3. **[Configuration](advanced/defaults_overrides.md)** - Customize settings

## Troubleshooting

### Command Not Found

If `cargoship` command is not found:

1. **Go Install**: Ensure `$GOPATH/bin` is in your `$PATH`
2. **Binary Install**: Ensure the binary location is in your `$PATH`
3. **Docker**: Use the full docker command instead

### AWS Permission Errors

If you get AWS permission errors:

1. Check your AWS credentials: `aws sts get-caller-identity`
2. Verify IAM permissions match the policy above
3. Ensure the S3 bucket exists and you have access

### Performance Issues

For large archives or slow uploads:

1. Increase concurrency: `--max-concurrency 16`
2. Adjust chunk size: `--chunk-size 32MB`
3. Enable transfer acceleration: `--use-transfer-acceleration`
4. Consider using faster storage class temporarily

## Getting Help

- **Documentation**: Browse the full docs at [cargoship.app](https://cargoship.app)
- **Issues**: Report bugs at [GitHub Issues](https://github.com/scttfrdmn/cargoship/issues)
- **Discussions**: Ask questions at [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)