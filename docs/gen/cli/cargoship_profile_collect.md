## cargoship profile collect

Collect performance profiles

### Synopsis

Collect various performance profiles for analysis.

By default, profiles are saved to a temporary directory. Use --output-dir
to specify a custom location.

```
cargoship profile collect [flags]
```

### Options

```
      --allocs              Collect allocation profile
      --block               Collect blocking operations profile
      --cpu                 Collect CPU profile
  -d, --duration int        Duration in seconds for CPU profiling (default 30)
      --goroutine           Collect goroutine profile
  -h, --help                help for collect
      --memory              Collect memory profile
      --mutex               Collect mutex contention profile
  -o, --output-dir string   Output directory for profile files (default: temp dir)
      --trace               Collect execution trace
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -v, --verbose               Enable verbose output
```

