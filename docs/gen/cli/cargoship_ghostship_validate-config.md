## cargoship ghostship validate-config

Validate a ghostship config, and optionally check it doesn't widen scope

### Synopsis

Validate a ghostship config file before deploying it. Reports errors (an
invalid config) and warnings (dangerous-but-permitted combinations, e.g.
delete_after_archive with a broad matcher).

With --baseline, also reports how CONFIG_FILE widens what a writer reads or
deletes relative to the currently-deployed config (new/broadened watch paths,
recursive flips, added includes, removed excludes, newly-enabled source deletion)
— the check the fleet's config-over-S3 pull will enforce.

Read-only: no AWS calls, no network.

Examples:
  cargoship ghostship validate-config ghost_ship.yaml
  cargoship ghostship validate-config new.yaml --baseline deployed.yaml --strict

```
cargoship ghostship validate-config CONFIG_FILE [flags]
```

### Options

```
      --baseline string     Path to the currently-deployed config; report how CONFIG_FILE widens watch scope vs it
  -h, --help                help for validate-config
      --public-key string   Verify the config's detached signature against this PEM ed25519 public key (fails if invalid)
      --signature string    Path to the signature sidecar (default CONFIG_FILE.sig)
      --strict              Treat warnings and scope-widenings as failures (non-zero exit)
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

