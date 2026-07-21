## cargoship setup

Interactive setup wizard for CargoShip configuration

### Synopsis

Interactive setup wizard that guides you through CargoShip configuration.

This wizard will help you:
  • Configure AWS credentials and region
  • Verify S3 bucket access
  • Set optimal upload parameters based on your use case
  • Test your configuration

The wizard will create a .cargoship.yaml file in your home directory.

Examples:
  # Run interactive setup
  cargoship setup

  # Save configuration to custom location
  cargoship setup --output /path/to/config.yaml

```
cargoship setup [flags]
```

### Options

```
  -h, --help              help for setup
      --non-interactive   Run in non-interactive mode with defaults
      --output string     Custom configuration file path (default: ~/.cargoship.yaml)
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

