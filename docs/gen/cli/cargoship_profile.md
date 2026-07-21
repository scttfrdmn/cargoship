## cargoship profile

Performance profiling and diagnostics tools

### Synopsis

Performance profiling and diagnostics tools for CargoShip.

This command provides comprehensive profiling capabilities to help diagnose
performance issues and optimize CargoShip operations.

Available profile types:
  • CPU: CPU usage profiling (--cpu)
  • Memory: Memory allocation profiling (--memory)
  • Goroutine: Goroutine stack traces (--goroutine)
  • Block: Blocking operations profiling (--block)
  • Mutex: Mutex contention profiling (--mutex)
  • Trace: Execution trace for detailed analysis (--trace)
  • Allocs: Memory allocation profiling (--allocs)

Examples:
  # Capture CPU profile for 30 seconds
  cargoship profile collect --cpu --duration 30

  # Capture memory profile
  cargoship profile collect --memory

  # Capture all profiles
  cargoship profile collect --cpu --memory --goroutine --duration 60

  # List available profile files
  cargoship profile list

  # Show current runtime statistics
  cargoship profile stats

### Options

```
  -h, --help   help for profile
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

