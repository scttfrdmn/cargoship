## cargoship alerts config

Show current alert configuration

### Synopsis

Display the current alert notification configuration.

Shows:
- Enabled notification channels
- Channel-specific configuration
- Alert thresholds and cooldown periods
- Last alert timestamps

Examples:
  # Show configuration
  cargoship alerts config

  # Show as JSON
  cargoship alerts config --json


```
cargoship alerts config [flags]
```

### Options

```
  -h, --help   help for config
      --json   Output as JSON
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

