## cargoship setup

Interactive setup wizard for CargoShip configuration

### Synopsis

Interactive setup wizard that guides you through CargoShip configuration.

This wizard will help you:
  • Configure AWS credentials and region
  • Verify S3 bucket access
  • Set optimal upload parameters based on your use case
  • Test your configuration

The wizard writes ~/.cargoship.yaml. If that file exists, its current values
are the defaults, setup asks before changing it (or needs --force), only the
settings it asks about are updated, and the previous file is kept as .bak.

Examples:
  # Run interactive setup
  cargoship setup

  # Accept every default without prompting (needs --force to update an existing file)
  cargoship setup --non-interactive

  # Save configuration to custom location
  cargoship setup --output /path/to/config.yaml

```
cargoship setup [flags]
```

### Options

```
      --force             Update an existing configuration file without asking
  -h, --help              help for setup
      --non-interactive   Accept every default without prompting (the existing file's values, if any)
      --output string     Custom configuration file path (default: ~/.cargoship.yaml)
```

### Options inherited from parent commands

```
      --allow-public-observability   Permit the pprof/Prometheus endpoints to bind a non-loopback (public) interface; they are unauthenticated, so this exposes them to the network
      --context string               Override execution context (local, repl)
      --memory-limit string          Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                        Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string            Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile                      Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                        Enable trace messages in output
  -v, --verbose                      Enable verbose output
```

