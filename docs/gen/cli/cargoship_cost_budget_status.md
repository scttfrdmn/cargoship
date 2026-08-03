## cargoship cost budget status

Show budget status for a project

### Synopsis

Display detailed budget status for a specific project.

Shows:
- Maximum cost budget and current spending
- Maximum volume quota and current usage
- Remaining budget/quota
- Usage percentages
- Daily burn rates
- Projected end-of-period usage
- Alert status

Examples:
  # Show budget status
  cargoship budget status project1

  # Status as JSON
  cargoship budget status project1 --json

  # Show the org/team-wide budget status
  cargoship budget status --global


```
cargoship cost budget status <project-id> [flags]
```

### Options

```
      --global   Show the org/team-wide budget status instead of a project's
  -h, --help     help for status
      --json     Output as JSON
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
      --store string          Budget store location: local (default) or s3://bucket/prefix for a shared, durable store
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

