## cargoship cost pricing

Show current AWS pricing

### Synopsis

Display current AWS S3 pricing for your region.

Shows pricing for:
- Storage (per GB per month) for all storage classes
- Request costs (PUT, GET, etc.)
- Data transfer costs

Examples:
  # Show pricing for default region
  cargoship cost pricing

  # Show pricing for specific region
  cargoship cost pricing --region eu-west-1

  # Pricing as JSON
  cargoship cost pricing --json


```
cargoship cost pricing [flags]
```

### Options

```
  -h, --help   help for pricing
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --json                  Output as JSON
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

