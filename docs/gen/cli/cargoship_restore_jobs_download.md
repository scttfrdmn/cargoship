## cargoship restore jobs download

Download files from a ready restore job

### Synopsis

Download the files for a restore job whose Glacier restore has completed.
The job must be in 'ready' status (run 'restore jobs check' first if unsure).

```
cargoship restore jobs download <job-id> [flags]
```

### Options

```
      --cache-gb int   LRU chunk cache size in GB (default 10)
  -h, --help           help for download
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

