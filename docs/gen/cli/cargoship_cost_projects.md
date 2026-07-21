## cargoship cost projects

List all projects with cost records

### Synopsis

List all projects (manifest upload IDs) that have associated cost records.

Projects are identified by their manifest upload IDs (e.g., 20251206-abc123).
Each upload to S3 creates a unique project that can be tracked separately.

Examples:
  # List all projects
  cargoship cost projects

  # List projects as JSON
  cargoship cost projects --json


```
cargoship cost projects [flags]
```

### Options

```
  -h, --help   help for projects
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

