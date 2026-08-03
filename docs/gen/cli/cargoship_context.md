## cargoship context

Manage CargoShip execution context

### Synopsis

Manage CargoShip execution context to control which commands are available.

Context determines the operational mode:
- local: Local filesystem operations and archive creation
- agent: Launch agent monitoring and management
- repl:  Interactive shell mode with command discovery

The current context is cached in ~/.cargoship-context and persists between sessions.

```
cargoship context [flags]
```

### Examples

```
  # Show current context
  cargoship context

  # Switch to agent context
  cargoship context switch agent

  # List available contexts
  cargoship context list

  # Reset to default (local) context
  cargoship context reset

  # Show context with details
  cargoship context show
```

### Options

```
  -h, --help   help for context
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

