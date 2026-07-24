## cargoship verify

Verify dataset integrity using manifest checksums

### Synopsis

Verify the integrity of a CargoShip upload by validating the manifest and checksums.

The verify command performs comprehensive integrity checks:
- Downloads and validates manifest structure
- Verifies manifest consistency (shard counts, file counts, size totals)
- Checks for missing or corrupted metadata
- Validates checksum coverage (if present)

This is useful for:
- Ensuring upload completed successfully
- Detecting corrupted or incomplete uploads
- Validating data integrity before restore
- Compliance and audit requirements

Examples:
  # Verify using S3 URL
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234

  # Verify using flags
  cargoship verify --bucket my-bucket --prefix cargoship --upload-id 20231208-123456-abcd1234

  # Quick validation (metadata only, fast)
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --quick

  # Verbose output with detailed errors
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --verbose

Exit Codes:
  0 - All checks passed
  1 - Verification failed (errors found)


```
cargoship verify [s3://bucket/prefix/uploads/upload-id] [flags]
```

### Options

```
  -b, --bucket string      S3 bucket name (or provide S3 URL as argument)
      --deep               Deep verification: re-download stored objects and recompute checksums against the manifest (data-level integrity)
  -h, --help               help for verify
  -p, --prefix string      S3 prefix for upload (default: empty)
      --quick              Quick validation (metadata only, fast)
  -r, --region string      AWS region (default "us-west-2")
  -u, --upload-id string   Upload ID to verify (or provide S3 URL as argument)
      --verbose            Show detailed error and warning information
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
```

