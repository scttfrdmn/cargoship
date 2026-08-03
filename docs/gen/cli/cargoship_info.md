## cargoship info

Display metadata and statistics for a CargoShip upload

### Synopsis

Display comprehensive metadata for a CargoShip upload without downloading data.

The info command downloads only the lightweight manifest (~30KB) and displays:
- Upload identification (ID, timestamp, source)
- File statistics (count, size, compression ratio)
- Shard distribution (per-shard statistics)
- Compression settings (algorithm, level, ratio achieved)
- Storage location (bucket, prefix, region)

This is useful for:
- Inspecting upload metadata before downloading
- Verifying upload completed successfully
- Planning selective extractions
- Auditing archived datasets

Examples:
  # Using S3 URL (Issue #98)
  cargoship info s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234

  # Using flags (backward compatibility)
  cargoship info --bucket my-bucket --prefix cargoship --upload-id 20231208-123456-abcd1234

  # Show detailed per-shard statistics
  cargoship info s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --verbose

  # Output as JSON for scripting
  cargoship info s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --json


```
cargoship info [s3://bucket/prefix/uploads/upload-id] [flags]
```

### Options

```
  -b, --bucket string      S3 bucket name (or provide S3 URL as argument)
  -h, --help               help for info
      --json               Output manifest as JSON for scripting
  -p, --prefix string      S3 prefix for upload (default: empty)
  -r, --region string      AWS region (default "us-west-2")
  -u, --upload-id string   Upload ID to inspect (or provide S3 URL as argument)
      --verbose            Show detailed per-shard statistics
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
```

