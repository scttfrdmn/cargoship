## cargoship ghostship run

Run an unattended, writer-isolated incremental backup on a schedule

### Synopsis

Continuously back up on an interval using CargoShip's real incremental sync engine
(chunked archives, manifests, dataset versioning) — the same engine as 'cargoship
sync', just scheduled and headless. Each cycle uploads only what changed since the
previous manifest.

Three forms:
  Single source (flags):  cargoship ghostship run SOURCE_DIR s3://BUCKET/PREFIX --writer-id ID
  Config file (fleet):    cargoship ghostship run --config box.yaml
  Signed config (pull):   cargoship ghostship run --config-url s3://BUCKET/BASE \
                            --public-key pub.pem --writer-id ID

With a config file, each entry in 'watch_paths' is backed up as its own source under
one writer prefix (writers/&lt;id&gt;/). 'archival_rules' are ignored in sync mode (they
belong to the legacy per-file model).

With --config-url, the config is PULLED from BASE/fleet/&lt;id&gt;/config.yaml and must
verify against the ed25519 --public-key (the agent bakes only the public key). An
unsigned, wrong-key, or tampered config is refused. Outbound-only: no inbound port.
Stops cleanly on SIGINT/SIGTERM; --once runs a single cycle (for cron).

Examples:
  cargoship ghostship run /volume1/Documents s3://backups/nas --writer-id lab-nas-1
  cargoship ghostship run --config /etc/cargoship/box.yaml --interval 30m
  cargoship ghostship run --config-url s3://backups/nas --public-key /etc/cargoship/fleet.pub --writer-id lab-nas-1

```
cargoship ghostship run [SOURCE_DIR S3_URL] [flags]
```

### Options

```
      --compression-level int   Fixed zstd level (1-22); 0 = content-aware per-chunk selection
      --config string           Ghostship config file (fleet mode); backs up each watch_paths entry
      --config-url string       Pull a signed config from S3 (s3://BUCKET/BASE); reads BASE/fleet/<id>/config.yaml, verified against --public-key
  -h, --help                    help for run
      --ignore-budget           Skip the per-writer budget/volume cap check (#629)
      --interval duration       How often to run a backup cycle (overrides config scan_interval) (default 1h0m0s)
      --once                    Run a single cycle and exit (for cron / testing)
      --public-key string       PEM ed25519 public key the pulled config must verify against (required with --config-url)
  -r, --region string           AWS region (default "us-west-2")
      --shard-count int         Number of shards for parallel uploads (1-100) (default 10)
      --shard-strategy string   Shard distribution strategy (round-robin, hash, size, type, directory) (default "round-robin")
      --storage-class string    Default S3 storage class (per-source storage_class in config overrides) (default "STANDARD")
      --track-deletes           Record files deleted since the last backup in the manifest
      --writer-id string        Writer identity for fleet isolation (writers/<id>/). 'auto' derives a stable per-host id; overrides the config
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

