## cargoship config

Manage CargoShip configuration

### Synopsis

Manage CargoShip configuration files and settings.

CargoShip uses YAML configuration files to store settings for AWS, storage,
upload optimization, metrics, logging, and security. Configuration can be
loaded from multiple sources with the following precedence:

1. Command line flags (highest priority)
2. Environment variables (CARGOSHIP_*)
3. Configuration file
4. Built-in defaults (lowest priority)

Configuration file locations (searched in order):
- ~/.cargoship.yaml
- ~/.config/cargoship/.cargoship.yaml
- ./.cargoship.yaml

Examples:
  # Generate example configuration file
  cargoship config --generate

  # Show current configuration
  cargoship config --show

  # Validate configuration file
  cargoship config --validate --file ~/.cargoship.yaml

  # Show configuration in JSON format
  cargoship config --show --format json

```
cargoship config [flags]
```

### Options

```
      --edit                Edit configuration file with default editor
      --file string         Configuration file path
      --format string       Output format (yaml, json) (default "yaml")
      --generate            Generate example configuration file
  -h, --help                help for config
      --show                Show current configuration
      --validate            Validate configuration file
      --validate-detailed   Validate configuration with AWS connectivity and bucket access checks
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

