## cargoship resume list

List all resumable uploads

### Synopsis

List all resumable uploads with their current progress.

Displays:
- Upload ID and status
- Source directory and S3 destination
- Progress (files and bytes completed)
- Time since upload started
- Estimated completion (if applicable)

State files are read from ~/.cargoship/state/


```
cargoship resume list [flags]
```

### Options

```
  -h, --help   help for list
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

