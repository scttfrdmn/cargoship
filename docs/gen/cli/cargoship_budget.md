## cargoship budget

Manage project budgets and volume quotas

### Synopsis

Manage project-specific budgets and volume quotas.

Project budgets allow you to set cost and volume limits per project (manifest upload).
This enables granular cost control where operations can be blocked if they would exceed
EITHER cost budgets OR volume quotas.

Features:
- Set cost budgets per project (e.g., $1000/month)
- Set volume quotas per project (e.g., 500GB/month)
- Separate alert thresholds for cost vs volume
- Hierarchical enforcement (project limits, then global limits)
- Real-time budget status with burn rate tracking

Examples:
  # Show budget status for a project
  cargoship budget status project1

  # Set project budget (cost only)
  cargoship budget set project1 --cost 1000

  # Set project with both cost and volume limits
  cargoship budget set project1 --cost 1000 --volume 500

  # Set project with custom alert thresholds
  cargoship budget set project1 --cost 1000 --volume 500 --cost-alert 0.85 --volume-alert 0.75

  # List all project budgets
  cargoship budget list

  # Remove project budget
  cargoship budget remove project1


```
cargoship budget [flags]
```

### Options

```
  -h, --help           help for budget
      --store string   Budget store location: local (default) or s3://bucket/prefix for a shared, durable store
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

