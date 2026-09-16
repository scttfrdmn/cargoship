## cargoship fleet status

Show the last-known status of every writer in a fleet

### Synopsis

List every writer that has reported under the given fleet prefix, with how
long ago it last checked in, whether its most recent cycle was healthy, and its
per-source counts.

S3_URL is the fleet's base prefix (the same destination the agents back up to, not a
writers/&lt;id&gt;/ sub-prefix). Writers with no readable heartbeat are omitted.

Examples:
  cargoship fleet status s3://backups/nas
  cargoship fleet status s3://backups/nas --json

```
cargoship fleet status S3_URL [flags]
```

### Options

```
  -h, --help             help for status
      --json             Emit the fleet status as JSON
      --profile string   AWS profile for the S3 target
      --region string    AWS region for the S3 target (auto-detected if empty)
```

### Options inherited from parent commands

```
      --allow-public-observability   Permit the pprof/Prometheus endpoints to bind a non-loopback (public) interface; they are unauthenticated, so this exposes them to the network
      --context string               Override execution context (local, repl)
      --memory-limit string          Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                        Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string            Address for runtime profiling HTTP endpoint (default "localhost:6060")
  -t, --trace                        Enable trace messages in output
  -v, --verbose                      Enable verbose output
```

