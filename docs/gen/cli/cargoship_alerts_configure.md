## cargoship alerts configure

Configure alert notification channels

### Synopsis

Configure specific alert notification channels.

Supported channels:
- email:    SMTP email notifications with TLS
- slack:    Slack webhook notifications
- webhook:  Custom HTTP webhook
- cloudwatch: AWS CloudWatch integration

Examples:
  # Configure email
  cargoship alerts configure email \\
    --smtp-host smtp.gmail.com \\
    --smtp-port 587 \\
    --smtp-username alerts@example.com \\
    --smtp-password "app-password" \\
    --smtp-from "cargoship@example.com" \\
    --recipients admin@example.com,ops@example.com

  # Configure Slack
  cargoship alerts configure slack \\
    --webhook-url "https://hooks.slack.com/services/..." \\
    --channel "#cargoship-alerts"


```
cargoship alerts configure [channel] [flags]
```

### Options

```
      --channel string         Slack channel (e.g., #cargoship-alerts)
  -h, --help                   help for configure
      --namespace string       CloudWatch namespace (default "CargoShip/Budget")
      --recipients strings     Email recipients (comma-separated)
      --smtp-from string       From email address
      --smtp-host string       SMTP server hostname
      --smtp-password string   SMTP password
      --smtp-port int          SMTP server port (default 587)
      --smtp-use-tls           Use TLS encryption (default true)
      --smtp-username string   SMTP username
      --username string        Slack bot username (default "CargoShip Monitor")
      --webhook-url string     Slack webhook URL
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

