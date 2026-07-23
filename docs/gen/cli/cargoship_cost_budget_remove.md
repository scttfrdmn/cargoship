## cargoship cost budget remove

Remove budget for a project

### Synopsis

Remove cost budget and volume quota for a specific project.

After removal, the project will use the global budget and quota settings.

Examples:
  # Remove project budget
  cargoship budget remove project1


```
cargoship cost budget remove <project-id> [flags]
```

### Options

```
  -h, --help   help for remove
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
      --store string          Budget store location: local (default) or s3://bucket/prefix for a shared, durable store
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

