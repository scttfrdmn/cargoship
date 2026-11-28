## cargoship lifecycle

Manage S3 lifecycle policies for cost optimization

### Synopsis

Manage S3 lifecycle policies to automatically optimize storage costs.

CargoShip provides predefined lifecycle policy templates optimized for different
use cases, or you can create custom policies based on your access patterns.

A template applies to the prefix your uploads use (--prefix, the part after the
bucket in s3://bucket/prefix). Its transitions move only archive chunks, which
carry the object tag cargoship-object=chunk. Manifests are never transitioned,
so sync, list, info, verify and restore keep working without a thaw.

The compliance-retention template also DELETES every object under the prefix,
chunks and manifests, 2,555 days (about 7 years) after it was written. It needs
a non-empty --prefix.

Examples:
  # List available policy templates
  cargoship lifecycle --list-templates

  # Apply archive optimization to uploads under s3://my-bucket/backup
  cargoship lifecycle --bucket my-bucket --prefix backup --template archive-optimization

  # Apply a template to the whole bucket
  cargoship lifecycle --bucket my-bucket --prefix "" --template intelligent-tiering

  # Estimate savings for a policy
  cargoship lifecycle --bucket my-bucket --prefix backup --template intelligent-tiering --estimate-size 100

  # Export current policy
  cargoship lifecycle --bucket my-bucket --export policy.json

  # Remove lifecycle policy
  cargoship lifecycle --bucket my-bucket --remove

```
cargoship lifecycle [flags]
```

### Options

```
      --bucket string         S3 bucket name (required unless listing templates)
      --estimate-size float   Data size in GB for savings estimation
      --export string         Export current policy to file
  -h, --help                  help for lifecycle
      --import string         Import policy from file
      --list-templates        List available policy templates
      --prefix string         Upload prefix the template applies to (s3://bucket/PREFIX); required with --template, "" for the whole bucket
      --region string         AWS region (default "us-east-1")
      --remove                Remove CargoShip's lifecycle rules (rules with other IDs are kept)
      --template string       Lifecycle policy template to apply
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

