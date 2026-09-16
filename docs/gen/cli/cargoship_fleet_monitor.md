## cargoship fleet monitor

Watch a fleet and alert when a writer goes stale (#630)

### Synopsis

Periodically read every writer's heartbeat under a fleet prefix and fire a
stale-writer alert for any writer that has not checked in within the freshness
threshold — a writer that is down, stuck, or offline can't report on itself, so this
runs on the control side. Each pass also evaluates budgets, so both fleet-freshness
and budget alerts go out through the same configured channels (see 'cargoship alerts').

Alerts are de-duplicated per writer by the alert cooldown; a writer alerts at most
once per cooldown window. With no alert channel configured, this still logs the fleet
state on every pass. Runs on an interval; --once does a single pass (for cron).

Examples:
  cargoship fleet monitor s3://backups/nas --threshold 2h --interval 15m
  cargoship fleet monitor s3://backups/nas --once

```
cargoship fleet monitor S3_URL [flags]
```

### Options

```
  -h, --help                 help for monitor
      --interval duration    How often to check the fleet (default 15m0s)
      --once                 Run a single check and exit (for cron / testing)
      --profile string       AWS profile for the S3 target
      --region string        AWS region for the S3 target (auto-detected if empty)
      --threshold duration   Age since last check-in after which a writer is considered stale (default 2h0m0s)
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

