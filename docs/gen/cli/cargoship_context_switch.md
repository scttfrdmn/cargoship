## cargoship context switch

Switch to a different execution context

### Synopsis

Switch to a different execution context.

Available contexts:
- local: Local filesystem operations and archive creation
- agent: Launch agent monitoring and management
- repl:  Interactive shell mode with command discovery

```
cargoship context switch <context> [flags]
```

### Examples

```
  cargoship context switch local
  cargoship context switch agent
  cargoship context switch repl
```

### Options

```
  -h, --help   help for switch
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

