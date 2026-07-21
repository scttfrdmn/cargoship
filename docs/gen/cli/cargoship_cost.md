## cargoship cost

Cost management and budget tracking

### Synopsis

Cost management and budget tracking for CargoShip uploads.

The cost command provides cost estimation, budget tracking, and pricing information:
- Estimate costs for planned uploads
- View budget status and spending
- Get current AWS pricing for your region

This replaces the standalone 'cargoship-cost' tool with integrated functionality.

Examples:
  # Estimate cost for uploading 100GB
  cargoship cost estimate --size 100GB --region us-west-2

  # Show budget status
  cargoship cost budget

  # Get current pricing
  cargoship cost pricing --region us-west-2

  # Generate cost report
  cargoship cost report --period month


```
cargoship cost [flags]
```

### Options

```
  -h, --help            help for cost
      --json            Output as JSON
  -r, --region string   AWS region (default "us-west-2")
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

