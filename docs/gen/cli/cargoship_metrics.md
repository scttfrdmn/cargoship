## cargoship metrics

Test CloudWatch metrics integration

### Synopsis

Test CloudWatch metrics integration for CargoShip observability.

This command allows you to test the CloudWatch metrics publishing functionality
to ensure proper integration with AWS monitoring services.

Examples:
  # Test metrics publishing
  cargoship metrics --test

  # Test with custom namespace and region
  cargoship metrics --test --namespace "CargoShip/Prod" --region us-east-1

```
cargoship metrics [flags]
```

### Options

```
  -h, --help               help for metrics
      --namespace string   CloudWatch namespace for metrics (default "CargoShip/Test")
      --region string      AWS region for CloudWatch (default "us-west-2")
      --test               Send test metrics to CloudWatch
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

