## cargoship ghostship config-keygen

Generate an ed25519 keypair for signing fleet configs (#614)

### Synopsis

Generate an ed25519 signing keypair for the config-over-S3 trust path. The
operator keeps the private key and signs configs with 'ghostship sign-config'; only
the public key is baked into an agent at deploy, and the agent refuses any config that
doesn't verify against it.

Writes config-signing-private.pem (0600) and config-signing-public.pem; neither is
overwritten if it already exists. This keypair signs configs — it is separate from the
GPG keys 'cargoship create keys' makes for file encryption.

Example:
  cargoship ghostship config-keygen --out ./fleet-keys

```
cargoship ghostship config-keygen [flags]
```

### Options

```
  -h, --help         help for config-keygen
      --out string   Directory to write the keypair into (default current directory)
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

