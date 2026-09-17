## cargoship ghostship init

Scaffold a deployable, write-only fleet-writer bundle (#613)

### Synopsis

Generate everything needed to deploy one ghostship writer against
s3://BUCKET/BASE: a strict write-only IAM policy, a config skeleton, a compose file
that wires credentials via a mounted file (not env), and the operator's config-signing
public key baked in.

This emits artifacts only — it does not call AWS. Attach the emitted policy to a new
IAM identity yourself (or use your own tooling), then fill in and sign the config. The
writer runs in signed config-over-S3 (pull) mode.

The emitted IAM policy is write-only and delete-free: PutObject on the writer's own
prefix, ListBucket scoped to it, read-only on its config, and kms:GenerateDataKey (no
Decrypt). A compromised writer can neither read back, decrypt, nor delete any data.

Example:
  cargoship ghostship config-keygen --out ./keys
  cargoship ghostship init s3://backups/nas --writer-id lab-nas-1 \
    --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd \
    --public-key ./keys/config-signing-public.pem --out ./lab-nas-1

```
cargoship ghostship init S3_URL [flags]
```

### Options

```
  -h, --help                 help for init
      --image string         Container image to run in the emitted compose file (default "cargoship:latest")
      --kms-key-arn string   Fleet KMS key ARN; adds kms:GenerateDataKey (write-only, no Decrypt) scoped to that key
      --out string           Directory to write the bundle into (default ./<writer-id>)
      --public-key string    Path to the config-signing public key (from 'ghostship config-keygen') to bake into the bundle (required)
  -r, --region string        AWS region for the emitted compose file (default "us-west-2")
      --writer-id string     Writer identity for this bundle (writers/<id>/). 'auto' derives a stable per-host id
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

