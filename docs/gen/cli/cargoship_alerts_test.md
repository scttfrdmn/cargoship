## cargoship alerts test

Test alert notification delivery

### Synopsis

Send a test alert to verify notification configuration.

Tests all enabled channels or a specific channel if specified.
The test alert uses sample budget data to demonstrate formatting.

Examples:
  # Test all enabled channels
  cargoship alerts test

  # Test specific channel
  cargoship alerts test --channel email

  # Test with specific severity
  cargoship alerts test --severity critical


```
cargoship alerts test [flags]
```

### Options

```
      --channel string    Test specific channel (email, slack, webhook, cloudwatch)
  -h, --help              help for test
      --severity string   Alert severity (info, warning, critical) (default "warning")
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

