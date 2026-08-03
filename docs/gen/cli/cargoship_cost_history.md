## cargoship cost history

Show the recorded per-upload outcome history

### Synopsis

Display the durable per-upload outcome history (metadata only).

This history is opt-in and OFF by default. Enable it with the
CARGOSHIP_UPLOAD_HISTORY environment variable (1, true, or a file path) or the
cost_control.upload_history_location config key. Each successful upload then
appends a metadata-only record — dataset shape, chosen parameters, and measured
outcomes (compression ratio, throughput, cost). No file content, names, or
paths are recorded.

Examples:
  # Show the recorded history
  cargoship cost history

  # As JSON, most recent first
  cargoship cost history --json

```
cargoship cost history [flags]
```

### Options

```
  -h, --help        help for history
      --limit int   Maximum number of records to show (0 = all) (default 20)
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

