# Cloud Storage with CargoShip ☁️

## Current Status: S3-Native Architecture

**As of v0.6.0, CargoShip has transitioned to a native S3 architecture** and no longer uses rclone for cloud transfers. This change provides:

✅ **Better Performance**: Native AWS SDK with multipart uploads and parallel prefix optimization
✅ **Enhanced Reliability**: Direct S3 integration with automatic retries and error handling
✅ **Cost Optimization**: Intelligent storage class selection (STANDARD, GLACIER_IR, DEEP_ARCHIVE)
✅ **Advanced Features**: Manifest tracking, incremental sync, streaming compression

## S3 Direct Upload (Current)

CargoShip now uses direct S3 uploads with advanced optimization:

```bash
# Upload with streaming pipeline
cargoship create upload ~/my-data/ --bucket my-bucket --prefix backups --region us-west-2

# Incremental sync (only upload changed files)
cargoship sync ~/my-data/ s3://my-bucket/backups

# Cost estimation before upload
cargoship estimate ~/my-data/ --storage-class GLACIER_IR
```

### Key Features

**Multi-Prefix Sharding**: Parallel uploads across 10 S3 prefixes for maximum throughput
```bash
cargoship create upload ~/data/ --bucket my-bucket --shard-count 10
```

**Storage Class Optimization**: Choose the right tier for your access patterns
- `STANDARD`: Frequent access, highest cost
- `GLACIER_IR`: Infrequent access, medium cost (instant retrieval)
- `DEEP_ARCHIVE`: Long-term archival, lowest cost

**Incremental Sync**: Upload only new or modified files
```bash
# First sync: uploads everything
cargoship sync ~/photos s3://my-bucket/backups

# Second sync: only uploads changed photos
cargoship sync ~/photos s3://my-bucket/backups
```

**Compression**: Automatic zstd compression for space savings
```bash
cargoship create upload ~/data/ --bucket my-bucket --compression-level 3
```

## Migration from Rclone

If you were previously using rclone with CargoShip:

### For S3 Destinations
✅ **Use native CargoShip commands** (shown above) for better performance and features

### For Non-S3 Cloud Providers
❌ **Not supported** - CargoShip is now S3-focused
✅ **Alternative**: Use [rclone](https://rclone.org/) directly as a separate tool

### Breaking Changes

The following have been removed in v0.6.0:
- `--cloud-destination` flag
- `cargoship rclone` command
- rclone configuration sections
- Cloud transporter plugin

See [CHANGELOG.md](../../CHANGELOG.md) for migration details.

## Advanced Configuration

### AWS Credentials
CargoShip uses standard AWS credential chain:
```bash
# Option 1: AWS CLI configuration
aws configure

# Option 2: Environment variables
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret
export AWS_REGION=us-west-2

# Option 3: AWS profiles
cargoship create upload ~/data/ --bucket my-bucket --profile production
```

### Multi-Region Uploads
```bash
# Upload with automatic failover
cargoship create upload ~/data/ --bucket my-bucket --multi-region --regions us-west-2,us-east-1
```

### Budget Controls
```bash
# Set cost limits
cargoship budget set --max-budget 1000 --max-volume-gb 500

# Estimate before upload
cargoship estimate ~/large-dataset/ --real-time-pricing
```

## Documentation

- [Architecture Overview](../architecture/TRANSFER_ARCHITECTURE.md)
- [CLI Reference](../CLI_REFERENCE.md)
- [Cost Management](../budget/COST_TRACKING.md)
- [Incremental Sync](../features/INCREMENTAL_SYNC.md)

## Support

For questions or issues:
- GitHub Issues: https://github.com/scttfrdmn/cargoship/issues
- Documentation: https://github.com/scttfrdmn/cargoship/tree/main/docs
