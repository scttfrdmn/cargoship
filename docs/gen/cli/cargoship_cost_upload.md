## cargoship cost upload

Show actual cost for a specific upload

### Synopsis

Display actual storage costs for a CargoShip upload.

Query the manifest to calculate real costs based on:
- Actual compressed size stored in S3
- Storage duration (from CreatedAt timestamp)
- Region and storage class
- Compression savings achieved

Examples:
  # Show cost for specific upload
  cargoship cost upload --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # Show cost with compression ROI details
  cargoship cost upload --bucket my-bucket --upload-id xxx --show-savings

  # JSON output
  cargoship cost upload --bucket my-bucket --upload-id xxx --json


```
cargoship cost upload [flags]
```

### Options

```
      --bucket string      S3 bucket name (required)
  -h, --help               help for upload
      --prefix string      S3 prefix (default "cargoship")
      --show-savings       Show compression savings (default true)
      --upload-id string   Upload ID (required)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --json                  Output as JSON
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

