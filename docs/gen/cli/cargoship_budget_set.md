## cargoship budget set

Set budget and quota for a project

### Synopsis

Set cost budget and/or volume quota for a specific project.

Budget values:
- Cost budget in USD (e.g., 1000 = $1000)
- Volume quota in GB (e.g., 500 = 500GB)
- Alert thresholds as percentages (0.0-1.0, e.g., 0.8 = 80%)
- Use 0 for unlimited

Examples:
  # Set cost budget only
  cargoship budget set project1 --cost 1000

  # Set volume quota only
  cargoship budget set project1 --volume 500

  # Set both cost and volume limits
  cargoship budget set project1 --cost 1000 --volume 500

  # Set with custom alert thresholds
  cargoship budget set project1 --cost 1000 --volume 500 \\
    --cost-alert 0.85 --volume-alert 0.75

  # Set unlimited
  cargoship budget set project1 --cost 0 --volume 0

  # Set the org/team-wide budget ceiling (across all projects)
  cargoship budget set --global --cost 10000 --volume 5000


```
cargoship budget set <project-id> [flags]
```

### Options

```
      --cost float           Maximum cost budget in USD (0 = unlimited)
      --cost-alert float     Cost alert threshold (0.0-1.0, default 0.8) (default 0.8)
      --global               Set the org/team-wide budget ceiling (across all projects) instead of a per-project budget
  -h, --help                 help for set
      --volume float         Maximum volume quota in GB (0 = unlimited)
      --volume-alert float   Volume alert threshold (0.0-1.0, default 0.75) (default 0.75)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
      --store string          Budget store location: local (default) or s3://bucket/prefix for a shared, durable store
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

