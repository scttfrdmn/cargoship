## cargoship ghostship run

Run an unattended, writer-isolated incremental backup on a schedule

### Synopsis

Continuously back up SOURCE_DIR to s3://BUCKET/PREFIX on an interval, using
CargoShip's real incremental sync engine (chunked archives, manifests, dataset
versioning) — the same engine as 'cargoship sync', just scheduled and headless.

Each cycle uploads only what changed since the previous manifest. With --writer-id
the objects are isolated under writers/&lt;id&gt;/ so a fleet sharing one bucket never
collides. Outbound-only: no inbound port, no daemon socket. Stops cleanly on
SIGINT/SIGTERM.

Examples:
  cargoship ghostship run /volume1/Documents s3://backups/nas --writer-id lab-nas-1
  cargoship ghostship run ./data s3://backups/dev --writer-id auto --interval 30m
  cargoship ghostship run ./data s3://backups/dev --once   # one cycle then exit (cron)

```
cargoship ghostship run SOURCE_DIR S3_URL [flags]
```

### Options

```
      --compression-level int   Fixed zstd level (1-22); 0 = content-aware per-chunk selection
  -h, --help                    help for run
      --interval duration       How often to run a backup cycle (default 1h0m0s)
      --once                    Run a single cycle and exit (for cron / testing)
  -r, --region string           AWS region (default "us-west-2")
      --shard-count int         Number of shards for parallel uploads (1-100) (default 10)
      --shard-strategy string   Shard distribution strategy (round-robin, hash, size, type, directory) (default "round-robin")
      --storage-class string    S3 storage class (STANDARD, GLACIER_IR, DEEP_ARCHIVE) (default "STANDARD")
      --track-deletes           Record files deleted since the last backup in the manifest
      --writer-id string        Writer identity for fleet isolation (writers/<id>/). 'auto' derives a stable per-host id
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

