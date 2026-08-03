## cargoship browse

Interactively browse and restore files from a CargoShip archive

### Synopsis

Open an interactive terminal UI to browse manifest contents and select files for restore.

Navigation:
  ↑/↓          Navigate file list
  space        Toggle selection on highlighted file
  enter        Confirm restore of selected files
  /            Enter incremental search mode
  d            Cycle DVC stage filter
  g            Cycle git commit filter
  a            Select all visible files
  c            Clear selection
  q / ctrl+c   Quit without restoring

Glacier/Deep Archive:
  --tier       Retrieval tier if files are archived: expedited, standard (default), bulk
  --wait       Block until Glacier restoration completes before downloading
  --max-restore-cost  Abort restore if estimated retrieval cost exceeds this USD limit

Examples:
  # Open the interactive browser
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored

  # Use a larger cache for big datasets
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored --cache-gb 20

  # Restore from Glacier with standard tier, wait for completion
  cargoship browse s3://my-bucket/uploads/20240101-abc123 --tier standard --wait


```
cargoship browse S3_URL [OUTPUT_DIR] [flags]
```

### Options

```
      --cache-gb int             LRU chunk cache size in GB (default 10)
  -h, --help                     help for browse
      --max-restore-cost float   Abort if estimated retrieval cost exceeds this USD amount
  -r, --region string            AWS region (default "us-east-1")
      --restore-days int32       Days to keep Glacier restored copy available (default 7)
      --tier string              Glacier retrieval tier: expedited, standard (default), bulk
      --wait                     Block until Glacier restoration completes before downloading
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

