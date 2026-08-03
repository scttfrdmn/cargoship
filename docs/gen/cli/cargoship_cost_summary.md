## cargoship cost summary

Summarize costs by DVC stage or git commit

### Synopsis

Aggregate recorded costs by DVC pipeline stage or git commit.

Requires cost records that were tagged with DVC provenance information
(populated automatically when --dvc-stage is passed to 'cargoship upload').

Examples:
  # Summarise costs for a specific DVC stage
  cargoship cost summary --by-dvc-stage preprocess

  # List all records for a git commit
  cargoship cost summary --git-commit abc1234


```
cargoship cost summary [flags]
```

### Options

```
      --by-dvc-stage string   Aggregate costs for this DVC pipeline stage
      --git-commit string     List costs tagged with this git commit SHA
  -h, --help                  help for summary
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

