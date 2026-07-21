## cargoship list

List files from a CargoShip upload using the manifest

### Synopsis

Query and display files from a CargoShip upload without downloading archives.

The list command downloads the lightweight manifest.json.gz file (~30KB) from S3
and displays all uploaded files with their locations and metadata.

Examples:
  # List all files from an upload
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # List files matching a pattern
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234 --pattern "*.log"

  # Verbose output with chunk and shard information
  cargoship list --bucket my-bucket --upload-id 20231208-123456-abcd1234 --verbose


```
cargoship list [flags]
```

### Options

```
  -b, --bucket string      S3 bucket name (required)
  -h, --help               help for list
      --pattern string     Filter files by glob pattern (e.g., '*.log')
  -p, --prefix string      S3 prefix for upload (default: empty)
  -r, --region string      AWS region (default "us-west-2")
  -u, --upload-id string   Upload ID to query (required)
      --verbose            Show verbose output with full file details
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

