## cargoship ghostship sign-config

Sign a ghostship config with an ed25519 private key (#614)

### Synopsis

Produce a detached signature over a ghostship config so agents can verify it
came from the operator. The config is validated first — a config with validation
errors is refused, so you never sign a broken config.

Writes a signature sidecar (default CONFIG_FILE.sig) that travels with the config.
Bump the config's 'version' field before signing so each rollout is identifiable in
writer heartbeats.

Example:
  cargoship ghostship sign-config ghost_ship.yaml --key config-signing-private.pem

```
cargoship ghostship sign-config CONFIG_FILE [flags]
```

### Options

```
  -h, --help            help for sign-config
      --key string      Path to the ed25519 private key PEM (required)
  -o, --output string   Write the signature to this path (default CONFIG_FILE.sig)
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

