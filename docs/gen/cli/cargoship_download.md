## cargoship download

Download and extract files from a CargoShip upload

### Synopsis

Download and selectively extract files from a CargoShip upload using the manifest.

The download command provides efficient selective extraction by:
1. Downloading the lightweight manifest first (~30KB)
2. Identifying which chunks contain the requested files
3. Only downloading and extracting necessary chunks (10x faster than full download)

Selective extraction options:
  --pattern    : Glob pattern matching (e.g., "*.log", "data/*.csv")
  --files      : Comma-separated list of exact file paths
  --shard-ids  : Only download specific shard IDs (0-7 by default)

Examples:
  # Download all files from an upload
  cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./restored

  # Download files matching a pattern
  cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./logs \
    --pattern "*.log"

  # Download specific files
  cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./reports \
    --files "data/report.csv,data/summary.csv"

  # Download specific shards only
  cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./restored \
    --shard-ids 0,2,4

  # Dry run to see what would be downloaded
  cargoship download s3://my-bucket/uploads/20231208-123456-abcd1234 ./restored \
    --pattern "*.csv" --dry-run


```
cargoship download S3_URL OUTPUT_DIR [flags]
```

### Options

```
      --dry-run          Show what would be downloaded without actually downloading
      --files strings    Comma-separated list of exact file paths to download
  -h, --help             help for download
      --pattern string   Filter files by glob pattern (e.g., '*.log')
  -r, --region string    AWS region (default "us-west-2")
      --shard-ids ints   Comma-separated list of shard IDs to download (0-7)
      --verbose          Show verbose output (list each file as extracted)
      --workers int      Number of parallel download workers (future use) (default 4)
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

