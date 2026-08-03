## cargoship resume

Manage resumable uploads

### Synopsis

Manage resumable uploads including listing, resuming, and cleaning up old states.

The resume command allows you to:
- List all resumable uploads with their progress
- Resume a specific interrupted upload by ID
- Clean up old state files to free disk space

Examples:
  # List all resumable uploads
  cargoship resume list

  # Resume a specific upload
  cargoship resume 20250115-143052-a3b4c5d6

  # Clean up state files older than 24 hours
  cargoship resume clean --older-than 24h

  # Clean up all completed uploads
  cargoship resume clean --completed

State files are stored in ~/.cargoship/state/
Each state file contains upload progress, configuration, and file hashes.


```
cargoship resume [upload-id] [flags]
```

### Options

```
  -h, --help   help for resume
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

