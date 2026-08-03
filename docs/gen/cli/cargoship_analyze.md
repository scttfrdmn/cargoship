## cargoship analyze

Analyze existing S3 storage and show potential CargoShip savings

### Synopsis

Analyze an existing S3 bucket and calculate how much CargoShip could save.

This command scans your existing S3 storage, calculates current costs, and shows
potential savings from re-grating to CargoShip's chunked format.

Supports AWS S3 and S3-compatible providers (Wasabi, Backblaze B2, MinIO).

Examples:
  # AWS S3
  cargoship analyze s3://my-bucket
  cargoship analyze s3://my-bucket/data --show-savings
  cargoship analyze s3://my-bucket --format json
  cargoship analyze s3://my-bucket --sampling --sample-size 10000
  cargoship analyze s3://my-bucket --region us-west-2

  # Wasabi
  cargoship analyze s3://my-bucket --provider wasabi --endpoint-url https://s3.wasabisys.com

  # Backblaze B2
  cargoship analyze s3://my-bucket --provider b2 --endpoint-url https://s3.us-west-002.backblazeb2.com

  # MinIO (self-hosted)
  cargoship analyze s3://my-bucket --provider minio --endpoint-url https://minio.example.com

```
cargoship analyze <s3://bucket[/prefix]> [flags]
```

### Options

```
      --endpoint-url string   S3-compatible endpoint URL (for Wasabi, B2, MinIO, etc.)
  -f, --format string         Output format (table, json) (default "table")
  -h, --help                  help for analyze
      --profile string        AWS profile to use
      --progress              Show progress during bucket scan (default true)
      --provider string       Storage provider (aws, wasabi, b2, minio, custom) (default "aws")
      --region string         AWS region (auto-detected from bucket if not specified)
      --sample-size int       Sample size for quick estimates (when --sampling enabled) (default 10000)
      --sampling              Use sampling mode for quick estimates on large buckets
      --show-savings          Show savings comparison with CargoShip re-gration (default true)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

