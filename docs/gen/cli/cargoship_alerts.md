## cargoship alerts

Manage budget alert notifications

### Synopsis

Configure and test budget alert notifications.

Budget alerts support multiple notification channels:
- Webhooks (HTTP POST with JSON payload)
- CloudWatch (AWS CloudWatch metrics and alarms)
- Email (SMTP with TLS encryption)
- Slack (webhook integration with rich formatting)

Features:
- Configure multiple notification channels
- Test alert delivery before enabling
- View current alert configuration
- Enable/disable specific channels

Examples:
  # Show current alert configuration
  cargoship alerts config

  # Configure email notifications
  cargoship alerts configure email \\
    --smtp-host smtp.gmail.com \\
    --smtp-port 587 \\
    --smtp-username alerts@example.com \\
    --smtp-password "app-password" \\
    --smtp-from "cargoship@example.com" \\
    --recipients admin@example.com,ops@example.com

  # Configure Slack notifications
  cargoship alerts configure slack \\
    --webhook-url "https://hooks.slack.com/services/T00/B00/abc123" \\
    --channel "#cargoship-alerts" \\
    --username "CargoShip Monitor"

  # Test alert delivery
  cargoship alerts test

  # Enable/disable specific channels
  cargoship alerts enable email
  cargoship alerts disable slack


```
cargoship alerts [flags]
```

### Options

```
  -h, --help   help for alerts
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

