## cargoship restore jobs check

Check Glacier restore status for pending jobs

### Synopsis

Poll S3 for each pending job and mark jobs as 'ready' when all their chunks
are accessible. If a job ID is given, only that job is checked.

```
cargoship restore jobs check [job-id] [flags]
```

### Options

```
  -h, --help            help for check
      --job-id string   Check only this specific job ID
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

