## cargoship dataset

Inspect dataset versions (incremental sync history)

### Synopsis

Inspect and manage the version history that incremental sync builds under an S3 prefix.

A "dataset" is a chain of manifests linked by their previous-manifest reference;
each 'cargoship sync' adds a version. 'list', 'versions', and 'diff' are
read-only; 'prune' performs destructive, irreversible garbage collection.

### Options

```
  -h, --help   help for dataset
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

