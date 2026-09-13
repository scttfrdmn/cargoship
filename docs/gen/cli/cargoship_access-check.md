## cargoship access-check

Report the access-control posture of a target bucket

### Synopsis

Report whether the bucket CargoShip writes to (or reads from) is exposed:
public access, wildcard-principal grants in the bucket policy or ACL, object
ownership, and the scoping of the default SSE-KMS key policy.

This is a read-only posture report — it makes no changes and always exits 0 when
it can reach the bucket (findings are informational). It is deliberately scoped
to user/group access controls, not a general security audit.

Each check degrades gracefully: if the calling principal lacks permission for a
check, that check reports "unknown" with the IAM action it needs, and the rest
of the report still runs. Checks are bucket-level (a prefix is recorded for
context but S3 policy/ACL/BPA/ownership apply to the whole bucket).

Examples:
  # Report on a bucket/prefix
  cargoship access-check s3://my-bucket/archives

  # Using flags, with a specific profile and region
  cargoship access-check --bucket my-bucket --region us-west-2 --profile prod

  # Machine-readable output
  cargoship access-check s3://my-bucket/archives --format json

  # Inspect a specific KMS key's policy instead of the bucket default
  cargoship access-check s3://my-bucket/archives --kms-key-id alias/cargoship

Exit Codes:
  0 - Report produced (regardless of findings)
  1 - Could not reach the bucket (missing/invalid credentials, no such bucket)
  2 - Usage error


```
cargoship access-check [s3://bucket/prefix] [flags]
```

### Options

```
  -b, --bucket string       S3 bucket name
      --format string       output format: table or json (default "table")
  -h, --help                help for access-check
      --kms-key-id string   KMS key ID/ARN/alias to inspect (default: the bucket's SSE-KMS key)
  -p, --prefix string       S3 prefix (for context; checks are bucket-level)
      --profile string      AWS profile
  -r, --region string       AWS region (auto-detected if omitted)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

