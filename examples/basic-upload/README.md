# CargoShip Basic Upload Example

This example demonstrates how to use CargoShip as a library for optimized S3 uploads in your own Go applications.

## What This Example Shows

- Loading AWS configuration
- Creating an S3 client
- Configuring CargoShip for optimal performance
- Uploading files with CargoShip optimization
- Verifying successful uploads
- Retrieving upload statistics (throughput, duration)

## Prerequisites

- Go 1.21 or later
- AWS credentials configured (`~/.aws/credentials` or environment variables)
- An S3 bucket with write permissions

## Installation

```bash
cd examples/basic-upload
go mod init basic-upload
go mod tidy
```

## Usage

### Basic Usage

```bash
go run main.go -bucket my-bucket-name
```

### With Custom Options

```bash
# Upload 10MB file with custom key
go run main.go -bucket my-bucket -key my-file.dat -size 10240

# Use specific AWS profile and region
go run main.go -bucket my-bucket -profile production -region us-east-1
```

### Command-Line Flags

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `-bucket` | S3 bucket name | - | **Yes** |
| `-key` | S3 object key | `test-upload.dat` | No |
| `-size` | File size in KB | `1024` | No |
| `-profile` | AWS profile | (default profile) | No |
| `-region` | AWS region | `us-west-2` | No |

## Example Output

```
CargoShip Basic Upload Example
==============================

Step 1: Loading AWS configuration...
✓ AWS config loaded (region: us-west-2)

Step 2: Creating S3 client...
✓ S3 client created

Step 3: Configuring CargoShip for optimal performance...
✓ CargoShip configured:
  - Storage Class: INTELLIGENT_TIERING
  - Multipart Threshold: 5.0 MB
  - Chunk Size: 10.0 MB
  - Concurrency: 4

Step 4: Creating CargoShip transporter...
✓ Transporter created with optimization enabled

Step 5: Preparing test data...
✓ Generated 1024 KB of test data

Step 6: Uploading with CargoShip optimization...
Uploading to s3://my-bucket/test-upload.dat...

==================================================
✓ Upload Complete!
==================================================
Location:      https://my-bucket.s3.amazonaws.com/test-upload.dat
ETag:          "d41d8cd98f00b204e9800998ecf8427e"
Storage Class: INTELLIGENT_TIERING
Duration:      1.234s
Throughput:    0.83 MB/s
Size:          1.00 MB

✓ Example completed successfully!
```

## Integration Patterns

### Pattern 1: Simple Upload Function

```go
func uploadFile(bucket, key string, data []byte) error {
    ctx := context.Background()

    // Load AWS config
    cfg, _ := config.LoadDefaultConfig(ctx)
    client := s3.NewFromConfig(cfg)

    // Configure CargoShip
    cargoConfig := cargoconfig.S3Config{
        Bucket:       bucket,
        StorageClass: cargoconfig.StorageClassStandard,
        Concurrency:  4,
    }

    // Create transporter and upload
    transporter := cargos3.NewTransporter(client, cargoConfig)
    archive := cargos3.Archive{
        Key:    key,
        Reader: bytes.NewReader(data),
        Size:   int64(len(data)),
    }

    _, err := transporter.Upload(ctx, archive)
    return err
}
```

### Pattern 2: Reusable Uploader

```go
type S3Uploader struct {
    transporter *cargos3.Transporter
    bucket      string
}

func NewS3Uploader(bucket string) (*S3Uploader, error) {
    ctx := context.Background()
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, err
    }

    client := s3.NewFromConfig(cfg)
    cargoConfig := cargoconfig.S3Config{
        Bucket:       bucket,
        StorageClass: cargoconfig.StorageClassIntelligentTiering,
        Concurrency:  4,
    }

    return &S3Uploader{
        transporter: cargos3.NewTransporter(client, cargoConfig),
        bucket:      bucket,
    }, nil
}

func (u *S3Uploader) Upload(ctx context.Context, key string, data []byte) error {
    archive := cargos3.Archive{
        Key:    key,
        Reader: bytes.NewReader(data),
        Size:   int64(len(data)),
    }
    _, err := u.transporter.Upload(ctx, archive)
    return err
}
```

### Pattern 3: With Progress Tracking

```go
func uploadWithProgress(bucket, key string, data []byte) error {
    ctx := context.Background()
    cfg, _ := config.LoadDefaultConfig(ctx)
    client := s3.NewFromConfig(cfg)

    cargoConfig := cargoconfig.S3Config{
        Bucket:       bucket,
        Concurrency:  4,
    }

    transporter := cargos3.NewTransporter(client, cargoConfig)

    // Start timer
    start := time.Now()

    archive := cargos3.Archive{
        Key:    key,
        Reader: bytes.NewReader(data),
        Size:   int64(len(data)),
    }

    result, err := transporter.Upload(ctx, archive)
    if err != nil {
        return err
    }

    // Log results
    log.Printf("Uploaded %s in %v (%.2f MB/s)",
        key, time.Since(start), result.Throughput)

    return nil
}
```

## Key Features Demonstrated

### 1. Intelligent Storage Class Selection

CargoShip automatically selects the optimal storage class based on access patterns:

```go
archive := cargos3.Archive{
    AccessPattern:   "infrequent", // or "archive", "rare", "unknown"
    RetentionDays:   90,            // Expected retention period
    // ...
}
```

### 2. Multipart Upload Optimization

For files larger than the threshold, CargoShip automatically uses multipart upload:

```go
cargoConfig := cargoconfig.S3Config{
    MultipartThreshold: 5 * 1024 * 1024,  // 5MB
    MultipartChunkSize: 10 * 1024 * 1024, // 10MB chunks
    Concurrency:        4,                 // 4 parallel chunks
}
```

### 3. Performance Metrics

Get detailed upload statistics:

```go
result, err := transporter.Upload(ctx, archive)
if err == nil {
    fmt.Printf("Throughput: %.2f MB/s\n", result.Throughput)
    fmt.Printf("Duration: %v\n", result.Duration)
}
```

### 4. Upload Verification

Verify uploads succeeded:

```go
exists, err := transporter.Exists(ctx, key)
if exists {
    info, _ := transporter.GetObjectInfo(ctx, key)
    fmt.Printf("Size: %d bytes\n", *info.ContentLength)
}
```

## Configuration Options

### Storage Classes

```go
cargoconfig.StorageClassStandard           // Frequent access
cargoconfig.StorageClassStandardIA         // Infrequent access
cargoconfig.StorageClassOneZoneIA          // Infrequent, single AZ
cargoconfig.StorageClassIntelligentTiering // Automatic tiering
cargoconfig.StorageClassGlacier            // Archival
cargoconfig.StorageClassDeepArchive        // Long-term archival
```

### Performance Tuning

```go
cargoConfig := cargoconfig.S3Config{
    // Multipart threshold - use multipart for files larger than this
    MultipartThreshold: 10 * 1024 * 1024, // 10MB

    // Chunk size for multipart uploads
    MultipartChunkSize: 10 * 1024 * 1024, // 10MB

    // Number of parallel uploads
    Concurrency: 8, // Higher = faster for large files

    // Optional: KMS encryption
    KMSKeyID: "arn:aws:kms:us-west-2:123456789:key/12345",

    // Optional: Transfer acceleration
    UseTransferAcceleration: true,
}
```

## Error Handling

```go
result, err := transporter.Upload(ctx, archive)
if err != nil {
    // Check for specific error types
    if strings.Contains(err.Error(), "NoSuchBucket") {
        log.Fatal("Bucket does not exist")
    } else if strings.Contains(err.Error(), "AccessDenied") {
        log.Fatal("Insufficient permissions")
    } else {
        log.Fatalf("Upload failed: %v", err)
    }
}
```

## Best Practices

1. **Reuse Transporter**: Create once, use for multiple uploads
   ```go
   transporter := cargos3.NewTransporter(client, config)
   // Use for multiple uploads
   ```

2. **Use Context**: Always pass context for timeout control
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
   defer cancel()
   ```

3. **Validate Config**: Check configuration before uploading
   ```go
   if err := awsConfig.Validate(); err != nil {
       log.Fatal(err)
   }
   ```

4. **Handle Large Files**: Adjust multipart settings for file size
   ```go
   // For 1GB+ files
   cargoConfig.MultipartChunkSize = 50 * 1024 * 1024 // 50MB
   cargoConfig.Concurrency = 8
   ```

## Troubleshooting

### "Bucket does not exist"
```bash
# Check bucket name
aws s3 ls s3://my-bucket

# Create bucket if needed
aws s3 mb s3://my-bucket
```

### "AccessDenied"
```bash
# Check AWS credentials
aws sts get-caller-identity

# Verify bucket permissions
aws s3api get-bucket-acl --bucket my-bucket
```

### Slow Uploads
- Increase `Concurrency` for multipart uploads
- Reduce `MultipartChunkSize` for better parallelism
- Enable `UseTransferAcceleration` for long distances

## Related Examples

- [ObjectFS Integration](../objectfs-integration/) - How ObjectFS uses CargoShip
- [Batch Upload](../batch-upload/) - Uploading multiple files
- [Advanced Configuration](../advanced-config/) - All configuration options

## Learn More

- [API Stability Guarantees](../../docs/api-stability.md)
- [Versioning Guidelines](../../docs/versioning.md)
- [CargoShip Documentation](../../README.md)

## License

This example is part of the CargoShip project and is licensed under the MIT License.
