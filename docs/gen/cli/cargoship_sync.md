## cargoship sync

Incrementally sync directory to S3 (only upload new/changed files)

### Synopsis

Incrementally sync a local directory to S3 by uploading only new or modified files.

The sync command provides efficient incremental backups by:
1. Downloading the latest manifest for the source path (if exists)
2. Comparing local filesystem state against the manifest
3. Uploading only files that are new or have changed
4. Creating a new manifest that references the previous one

First sync uploads everything (like 'upload' command).
Subsequent syncs only upload changed files, saving time and bandwidth.

Change detection (default: fast mode):
  - Size change: File size differs from manifest
  - Time change: Modification time is newer than manifest

Use --checksum for guaranteed accuracy (slower, computes SHA256).

Examples:
  # First sync: uploads all files
  cargoship sync /home/photos s3://my-bucket/backups

  # Second sync: only uploads new/changed photos
  cargoship sync /home/photos s3://my-bucket/backups

  # Dry run to see what would be synced
  cargoship sync /home/photos s3://my-bucket/backups --dry-run

  # Use checksum comparison (slower but accurate)
  cargoship sync /data s3://my-bucket/backups --checksum

  # Force full sync (ignore previous manifest)
  cargoship sync /data s3://my-bucket/backups --force


```
cargoship sync SOURCE_DIR S3_URL [flags]
```

### Options

```
      --checksum                Use SHA256 checksum comparison (slower but accurate)
      --compression-level int   Fixed zstd compression level (1-22), overriding per-chunk content-aware selection. Unset = content-aware (default 3)
      --dry-run                 Show what would be synced without uploading
      --force                   Force full sync (ignore previous manifest)
  -h, --help                    help for sync
  -q, --quiet                   Quiet mode (minimal output)
  -r, --region string           AWS region (default "us-west-2")
      --shard-count int         Number of shards for parallel uploads (1-100) (default 10)
      --shard-strategy string   Shard distribution strategy (round-robin, hash, size, type, directory) (default "round-robin")
      --storage-class string    S3 storage class (STANDARD, GLACIER_IR, DEEP_ARCHIVE) (default "STANDARD")
      --track-deletes           Track deleted files in manifest
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

