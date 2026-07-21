## cargoship dvc stages

List DVC pipeline stages and their file counts from a manifest

```
cargoship dvc stages S3_URL [flags]
```

### Options

```
  -h, --help            help for stages
      --json            Output as JSON
      --region string   AWS region (default "us-east-1")
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

