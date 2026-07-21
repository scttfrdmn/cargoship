## cargoship create upload

Upload directories to S3 using streaming pipeline

### Synopsis

Upload directories to S3 using the high-performance streaming pipeline.

This command replaces the legacy suitcase/rclone system with a modern streaming
architecture that provides:
- Real-time progress tracking with beautiful TUI
- Multi-prefix S3 parallel uploads (8x throughput improvement)
- Zero local disk usage (streaming directly to S3)
- Automatic compression (zstd)
- Intelligent chunking and sharding

```
cargoship create upload SOURCE_DIR... [flags]
```

### Examples

```
  # Upload a directory with progress tracking
  cargoship create upload /path/to/data --bucket my-bucket

  # Upload with custom prefix
  cargoship create upload /path/to/data --bucket my-bucket --prefix backups/2025-12-05

  # Quiet mode (no progress display)
  cargoship create upload /path/to/data --bucket my-bucket --quiet

  # JSON progress output (for scripts)
  cargoship create upload /path/to/data --bucket my-bucket --progress-format json
```

### Options

```
      --bucket string                S3 bucket name (required)
      --chunk-size-mb int            Target chunk size in MB (0 = adaptive) (default 200)
      --cleanup-on-failure           Automatically delete partial uploads on error (default true)
  -h, --help                         help for upload
      --http2                        Enable HTTP/2 (default true)
      --http2-max-streams int        Max concurrent HTTP/2 streams per connection (default 250)
      --idle-conn-timeout duration   Idle connection timeout (default 5m0s)
      --max-idle-conns int           Max idle connections per host (default 100)
      --network-profile string       Network tuning profile: default, aggressive, conservative (default "default")
      --no-cleanup                   Disable automatic cleanup on failure (for debugging)
      --prefix string                S3 key prefix (optional)
      --progress-format string       Progress output format: tui, json, text (default "tui")
      --quiet                        Disable progress display
      --region string                AWS region (default "us-west-2")
      --resume                       Resume a previous incomplete upload
      --shards int                   Number of S3 prefix shards for parallel uploads (default 8)
      --skip-existing                Skip chunks that already exist in S3 (HeadObject check)
      --storage-class string         S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.) (default "STANDARD")
      --upload-id string             Upload ID to resume (auto-detect if not specified)
      --workers int                  Workers per stage (scanner, archiver, uploader) (default 4)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
  -d, --destination string    Directory to write files in to. If not specified, we'll use an auto generated temp dir
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

