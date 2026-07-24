## cargoship restore

Restore specific files from a CargoShip archive using hash, path, commit, or DVC stage

### Synopsis

Restore targeted files from a CargoShip archive without downloading the whole dataset.

Restoration modes (pick one or combine --file with others):
  --hash        : Restore a single file by its MD5 content hash
  --file        : Restore one or more exact file paths
  --git-commit  : Restore all files from a specific git commit
  --dvc-stage   : Restore all files produced by a DVC pipeline stage

Glacier/Deep Archive support:
  --tier        : Retrieval tier: expedited (1-5 min), standard (3-5 h), bulk (5-12 h)
  --wait        : Block until Glacier restoration completes before downloading
  --dry-run     : Show what would be restored (size, cost) without downloading

Budget controls:
  --max-restore-cost : Abort if estimated retrieval cost exceeds this USD limit

Examples:
  # Restore a file by its MD5 hash
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --hash d8e8fca2dc0f896fd7cb4cb0031ba249

  # Restore specific files by path
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --file data/train.csv --file models/model.pkl

  # Restore all files from a DVC pipeline stage
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage preprocess

  # Restore from Glacier with standard retrieval tier, wait for completion
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage train --tier standard --wait

  # Dry-run: show estimated cost without restoring
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage train --dry-run


```
cargoship restore S3_URL OUTPUT_DIR [flags]
```

### Options

```
      --cache-gb int             LRU chunk cache size in GB (0 = default 10 GB) (default 10)
      --dry-run                  Show what would be restored without downloading
      --dvc-stage string         Restore all files produced by this DVC pipeline stage
      --file stringArray         Exact file path(s) to restore (repeatable)
      --git-commit string        Restore all files from this git commit SHA
      --hash string              MD5 content hash of the file to restore
  -h, --help                     help for restore
      --json                     Output restore statistics as JSON
      --max-restore-cost float   Abort if estimated retrieval cost exceeds this USD amount
      --no-verify                Skip restore-time checksum verification (faster, but won't detect corrupted stored data)
  -r, --region string            AWS region (default "us-east-1")
      --restore-days int32       Days to keep Glacier restored copy available (default 7)
      --tier string              Glacier retrieval tier: expedited, standard (default), bulk
      --wait                     Block until Glacier restoration completes before downloading
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

