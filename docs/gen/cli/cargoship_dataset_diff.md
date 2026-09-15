## cargoship dataset diff

Compare two versions of a dataset (added/removed/modified files)

```
cargoship dataset diff S3_URL --dataset-id ID --from V --to V [flags]
```

### Options

```
      --dataset-id string   Dataset ID (from 'cargoship dataset list')
      --from string         Baseline version: a number (v2 → 2) or a date (YYYY-MM-DD)
  -h, --help                help for diff
      --json                Output as JSON
  -r, --region string       AWS region (default "us-west-2")
      --to string           Target version: a number or date; default HEAD (latest)
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

