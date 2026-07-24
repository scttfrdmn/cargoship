## cargoship upload

Upload directory to S3 with CargoHold sharding

### Synopsis

Upload a directory to S3 using CargoHold's intelligent sharding system.

CargoHold divides large datasets into multiple shards for parallel uploads,
providing:
- Intelligent shard distribution (hash, size, type, or directory-based)
- Per-shard compression with configurable levels (zstd)
- Parallel uploads for maximum throughput
- Automatic manifest generation for easy restore
- Progress tracking with per-shard visibility

Shard Strategies:
  hash      - Hash-based distribution (balanced, default)
  size      - Size-based distribution (large files in separate shards)
  type      - File type distribution (group by extension)
  directory - Directory-based distribution (keep directories together)

Examples:
  # Upload with default settings (10 shards, hash strategy, compression level 3)
  cargoship upload /data s3://my-bucket/dataset

  # Upload with custom shard count and strategy
  cargoship upload /data s3://my-bucket/dataset --shard-count 20 --shard-strategy size

  # Upload with maximum compression
  cargoship upload /data s3://my-bucket/dataset --compression-level 19

  # Quiet mode (no progress display)
  cargoship upload /data s3://my-bucket/dataset --quiet


```
cargoship upload SOURCE_DIR DESTINATION [flags]
```

### Options

```
      --auto-tier                        Enable automatic storage tier selection based on file access time
  -b, --bucket string                    S3 bucket name (or use s3:// URL in DESTINATION)
      --compression-level int            Zstd compression level (1-22, recommended 1-19) (default 3)
      --congestion-control string        Congestion control algorithm: bbr, cubic, auto (default "auto")
      --direct-upload                    Enable direct upload mode (bypasses archiving/compression for small files)
      --direct-upload-threshold-mb int   Max total size in MB for auto direct upload (default: 500) (default 500)
      --direct-upload-workers int        Worker count for direct upload (default: 256) (default 256)
      --disable-staging                  Disable adaptive staging (reduces memory usage)
      --dvc-auto                         Auto-discover DVC stages from dvc.yaml and annotate each file entry with its stage name
      --dvc-cache-dir string             Local DVC cache directory (recorded in manifest; default: .dvc/cache) (default ".dvc/cache")
      --dvc-output-dir string            Directory to write .dvc files (default: source directory)
      --dvc-stage string                 DVC pipeline stage name to extract provenance from (reads dvc.yaml + dvc.lock)
      --enable-dedup                     Enable cross-shard file deduplication (10-30% space savings for redundant datasets)
      --encrypt-manifest                 Encrypt manifest with KMS envelope encryption (requires --kms-key-id)
      --force-direct-upload              Force direct upload regardless of thresholds (for benchmarking)
      --force-restart                    Ignore saved state and start fresh upload (bypasses resume detection)
      --generate-dvc-files               Generate DVC sidecar .dvc files after upload
      --git-metadata                     Embed Git repository metadata (commit, branch, tag, remote) in the manifest
  -h, --help                             help for upload
      --incremental                      Enable incremental sync: only upload new or changed files
      --interactive                      Enable interactive TUI mode with per-shard progress (Issue #112)
      --kms-key-id string                AWS KMS key ID or ARN for encryption (data chunks encrypted with SSE-KMS)
      --no-file-checksums                Disable per-file content checksums (faster uploads, but 'verify --deep' can't confirm per-file integrity)
      --optimization                     Enable optimization features (BBR/CUBIC, adaptive staging, BDP) (default true)
      --prev-manifest string             Path to previous manifest JSON (or .json.gz) for incremental sync
      --project string                   Project ID for cost tracking (e.g. 'dvc_cache' for DVC remotes)
      --prometheus-addr string           Prometheus metrics HTTP address (e.g., :9090)
      --quiet                            Disable progress display
  -r, --region string                    AWS region (default "us-west-2")
      --shard-count int                  Number of shards for parallel uploads (0=auto, 4-32=manual, default: 0)
      --shard-strategy string            Shard distribution strategy (hash, size, type, directory) (default "hash")
      --storage-class string             S3 storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.) (default "STANDARD")
      --tag stringArray                  Custom tag in key=value format, repeatable (e.g. --tag dvc_cache=true --tag env=prod)
      --tier-archive-days int            Days since access to consider 'archive' (DEEP_ARCHIVE) (default 180)
      --tier-cold-days int               Days since access to consider 'cold' (GLACIER) (default 90)
      --tier-hot-days int                Days since access to consider 'hot' (STANDARD) (default 30)
      --tier-max string                  Maximum storage tier (STANDARD, STANDARD_IA, GLACIER, DEEP_ARCHIVE) - prevents automatic selection of more restrictive tiers
      --tier-strategy string             Tier chunking strategy (requires --auto-tier):
                                           youngest-file: Conservative strategy - assigns tier based on youngest file per chunk (default)
                                           tier-aware:    Optimal cost - groups files by tier before chunking (30-60% savings)

                                         ⚠️  WARNING: tier-aware uses GLACIER/DEEP_ARCHIVE with cost implications:
                                           • GLACIER: 90-day minimum storage ($0.004/GB-month, $0.01/GB retrieval, 3-5hr access)
                                           • DEEP_ARCHIVE: 180-day minimum ($0.00099/GB-month, $0.02/GB retrieval, 12hr access)
                                           • Early deletion penalties apply if removed before minimum duration
                                           • Best for long-term archives accessed <1x per year

                                           See: https://github.com/scttfrdmn/cargoship/issues/168 (default "youngest-file")
      --tracing                          Enable distributed tracing
      --tracing-endpoint string          Tracing endpoint URL (required for jaeger/otlp exporters)
      --tracing-exporter string          Tracing exporter: stdout, jaeger, otlp, none (default "stdout")
      --tracing-sample-rate float        Trace sampling rate (0.0-1.0, default: 1.0 = 100%) (default 1)
      --transporter string               S3 transporter type: basic, staging, adaptive, optimized, none (default "staging")
  -y, --yes                              Skip confirmation prompts (auto-accept warnings)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

