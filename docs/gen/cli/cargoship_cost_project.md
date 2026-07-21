## cargoship cost project

Show costs for a specific project

### Synopsis

Show detailed cost information for a specific project (manifest upload ID).

Displays:
- Total costs and savings for the project
- Total files and data size uploaded
- Cost breakdown by region and storage class
- First and last upload timestamps
- Average cost per GB

Examples:
  # Show costs for specific project
  cargoship cost project 20251206-abc123

  # Show project costs for specific period
  cargoship cost project 20251206-abc123 --period month

  # Project costs as JSON
  cargoship cost project 20251206-abc123 --json


```
cargoship cost project PROJECT_ID [flags]
```

### Options

```
  -h, --help            help for project
      --period string   Report period (all, today, week, month, last_month) (default "all")
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --json                  Output as JSON
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

