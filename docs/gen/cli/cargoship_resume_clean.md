## cargoship resume clean

Clean up old or completed upload states

### Synopsis

Remove old state files to free up disk space.

By default, cleans up state files older than 24 hours.
Use --completed to remove only fully completed uploads.
Use --older-than to specify a custom age threshold.

Examples:
  # Clean states older than 24 hours (default)
  cargoship resume clean

  # Clean states older than 1 week
  cargoship resume clean --older-than 168h

  # Clean only completed uploads
  cargoship resume clean --completed

State files are stored in ~/.cargoship/state/
Each file is typically 10-50 KB.


```
cargoship resume clean [flags]
```

### Options

```
      --completed           Clean only completed uploads
  -h, --help                help for clean
      --older-than string   Clean states older than duration (e.g., 24h, 7d, 168h)
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

