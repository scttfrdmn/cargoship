## cargoship lifecycle

Manage S3 lifecycle policies for cost optimization

### Synopsis

Manage S3 lifecycle policies to automatically optimize storage costs.

CargoShip provides predefined lifecycle policy templates optimized for different
use cases, or you can create custom policies based on your access patterns.

Examples:
  # List available policy templates
  cargoship lifecycle --list-templates

  # Apply archive optimization policy
  cargoship lifecycle --bucket my-bucket --template archive-optimization

  # Estimate savings for a policy
  cargoship lifecycle --bucket my-bucket --template intelligent-tiering --estimate-size 100

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
      --region string         AWS region (default "us-east-1")
      --remove                Remove existing lifecycle policy
      --template string       Lifecycle policy template to apply
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

