## cargoship ghostship iam-policy

Emit a least-privilege, delete-free IAM policy for a fleet writer

### Synopsis

Emit the least-privilege AWS IAM policy for one ghostship writer backing up to
s3://BUCKET/PREFIX. Attach the emitted policy to the IAM user or role that your
writer runs as — a ghostship agent, or a cron'd 'cargoship sync --writer-id'.

The policy grants only what a writer needs: PutObject/GetObject/AbortMultipartUpload
scoped to writers/&lt;id&gt;/, plus ListBucket gated to that prefix. It is deliberately
delete-free and cannot reach another writer's data — a compromised writer can only
append to its own subtree, never delete or overwrite backups. With --kms-key-arn it
also grants kms:GenerateDataKey and kms:Decrypt on that one key (AWS requires both to
multipart-upload SSE-KMS objects).

Examples:
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id lab-nas-1
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id auto \
    --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd-1234 -o policy.json

```
cargoship ghostship iam-policy S3_URL [flags]
```

### Options

```
  -h, --help                 help for iam-policy
      --kms-key-arn string   KMS key ARN for encrypted uploads; adds kms:GenerateDataKey (+kms:Decrypt unless --write-only) scoped to that key
  -o, --output string        Write the policy JSON to this file instead of stdout
      --write-only           Emit the strict write-only policy: PutObject-only on data (no GetObject/Decrypt); incremental sync degrades to full re-scan each cycle
      --writer-id string     Writer identity to scope the policy to (writers/<id>/). 'auto' derives a stable per-host id; empty scopes to the base prefix
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

