## cargoship ghostship scuttle

Revoke a minted fleet-writer IAM identity (#613)

### Synopsis

Decommission one fleet writer by deleting the IAM identity that
'ghostship init --mint' created for it: its access keys, the attached write-only
policy, and the user. Idempotent — a writer that is already gone is a no-op, and
only the cargoship-managed policy is deleted (any other attached policy is detached
but left intact).

With --purge-data it ALSO deletes that writer's S3 data (writers/&lt;id&gt;/) and config
(&lt;base&gt;/fleet/&lt;id&gt;/) under the given bucket — irreversible, so it requires --yes.

This is the fleet-writer counterpart to 'cargoship scuttle' (which wipes bucket data).

Examples:
  cargoship ghostship scuttle lab-nas-1
  cargoship ghostship scuttle lab-nas-1 --purge-data s3://backups/nas --yes

```
cargoship ghostship scuttle WRITER_ID [flags]
```

### Options

```
  -h, --help                help for scuttle
      --profile string      AWS profile to use
      --purge-data string   Also delete this writer's S3 data + config under s3://BUCKET/BASE (irreversible; needs --yes)
  -r, --region string       AWS region (default "us-west-2")
      --yes                 Confirm the irreversible --purge-data deletion
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

