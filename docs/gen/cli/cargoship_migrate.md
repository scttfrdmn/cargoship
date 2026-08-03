## cargoship migrate

Convert traditional archives to CargoHold sharded format

### Synopsis

Download a traditional tar.zst archive from S3 and re-upload
using CargoHold's intelligent sharding system.

The migrate command:
1. Downloads the traditional archive from S3
2. Extracts files to a temporary location
3. Re-uploads using CargoHold sharding with compression
4. Generates a manifest for selective extraction
5. Optionally deletes the original archive

Examples:
  # Migrate archive with default settings
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded

  # Migrate and delete original
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --delete-original

  # Dry run to estimate migration
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --dry-run

  # Custom temp directory and shard count
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded \
    --temp-dir /mnt/fast-ssd --shard-count 16

  # Keep temp files for debugging
  cargoship migrate s3://bucket/archive.tar.zst s3://bucket/dataset-sharded --keep-temp


```
cargoship migrate SOURCE_ARCHIVE DESTINATION [flags]
```

### Options

```
      --compression-level int   Zstd compression level (1-22) (default 3)
      --delete-original         Delete original archive after successful migration
      --dry-run                 Estimate migration without performing it
  -h, --help                    help for migrate
      --keep-temp               Keep temporary files after migration
      --quiet                   Disable progress display
  -r, --region string           AWS region (default "us-west-2")
      --shard-count int         Number of shards for CargoHold (1-100) (default 8)
      --skip-validation         Skip pre-flight validation checks
      --storage-class string    S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER_IR, DEEP_ARCHIVE) (default "STANDARD")
      --temp-dir string         Temporary directory for extraction (default: OS temp)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

