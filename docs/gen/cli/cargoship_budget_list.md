## cargoship budget list

List all project budgets

### Synopsis

List all configured project budgets and their current status.

Shows for each project:
- Project ID
- Cost budget and current spending
- Volume quota and current usage
- Alert status

Examples:
  # List all project budgets
  cargoship budget list

  # List as JSON
  cargoship budget list --json


```
cargoship budget list [flags]
```

### Options

```
  -h, --help   help for list
      --json   Output as JSON
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

