## cargoship restore jobs clean

Remove completed and failed restore jobs

### Synopsis

Delete completed and failed restore jobs older than the given duration (default: 24h).

```
cargoship restore jobs clean [flags]
```

### Options

```
  -h, --help                help for clean
      --older-than string   Remove jobs older than this duration (e.g. 72h, 7d) (default "24h")
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

