## cargoship alerts enable

Enable alert notification channel

### Synopsis

Enable a specific alert notification channel.

Supported channels:
- email
- slack
- webhook
- cloudwatch

Examples:
  # Enable email notifications
  cargoship alerts enable email

  # Enable Slack notifications
  cargoship alerts enable slack


```
cargoship alerts enable [channel] [flags]
```

### Options

```
  -h, --help   help for enable
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

