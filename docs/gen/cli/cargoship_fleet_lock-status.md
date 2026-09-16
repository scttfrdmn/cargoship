## cargoship fleet lock-status

Audit a fleet bucket's immutability posture (versioning, Object Lock, lifecycle hygiene)

### Synopsis

Report whether a fleet bucket can resist a stolen delete-capable credential and
whether failed-upload hygiene is in place — read-only, it never changes the bucket.

Because the ghostship agent is delete-free by design, the ransomware backstop lives in
the bucket itself: S3 Versioning + Object Lock protect history even if a delete-capable
credential is compromised, and a lifecycle AbortIncompleteMultipartUpload rule cleans up
failed uploads the agent won't. This command checks all three and suggests remediation.

Object Lock, versioning, and lifecycle are bucket-level, so any prefix in the S3 URL is
ignored — the posture is reported for the whole bucket.

Examples:
  cargoship fleet lock-status s3://backups/nas
  cargoship fleet lock-status s3://backups/nas --json

```
cargoship fleet lock-status S3_URL [flags]
```

### Options

```
  -h, --help             help for lock-status
      --json             Emit the posture report as JSON
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

