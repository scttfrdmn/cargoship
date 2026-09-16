## cargoship fleet

Observe a ghostship fleet from the control side (read-only)

### Synopsis

Inspect a fleet of unattended ghostship backup agents by reading the
heartbeats they write under writers/&lt;id&gt;/status.json.

These commands run on your control machine and only read the bucket — they do not
need any agent's write identity. See 'cargoship ghostship' for the agent side.

### Options

```
  -h, --help   help for fleet
```

### Options inherited from parent commands

```
      --allow-public-observability   Permit the pprof/Prometheus endpoints to bind a non-loopback (public) interface; they are unauthenticated, so this exposes them to the network
      --context string               Override execution context (local, repl)
      --memory-limit string          Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                        Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string            Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile                      Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                        Enable trace messages in output
  -v, --verbose                      Enable verbose output
```

