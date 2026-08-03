## cargoship cost estimate

Estimate cost for a planned upload

### Synopsis

Estimate storage costs before uploading.

Calculates monthly storage costs based on:
- Data size (uncompressed)
- Storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.)
- Region-specific pricing

Examples:
  # Estimate cost for 100GB upload
  cargoship cost estimate --size 100GB

  # Estimate with specific storage class
  cargoship cost estimate --size 500GB --storage-class GLACIER

  # Estimate for different region
  cargoship cost estimate --size 1TB --region eu-west-1


```
cargoship cost estimate [flags]
```

### Options

```
  -h, --help                   help for estimate
      --operation string       Operation type (upload, download) (default "upload")
      --size string            Data size (e.g., 100GB, 500MB, 1TB) (required)
      --storage-class string   Storage class (STANDARD, STANDARD_IA, GLACIER, etc.) (default "STANDARD")
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

