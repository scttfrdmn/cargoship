## cargoship benchmark

Benchmark compression algorithms

### Synopsis

Benchmark different compression algorithms to find the optimal one for your data.

This command tests various compression algorithms (gzip, zlib, zstd, lz4, s2)
with different compression levels to help you choose the best algorithm based
on your performance and size requirements.

Examples:
  # Benchmark with random data
  cargoship benchmark --size 10MB

  # Benchmark with specific data type simulation
  cargoship benchmark --size 50MB --data-type text

  # Benchmark using a real file
  cargoship benchmark --file /path/to/data.tar

  # Output results in JSON format
  cargoship benchmark --size 1GB --format json

```
cargoship benchmark [flags]
```

### Options

```
      --data-type string   Type of data to simulate (text, binary, mixed, random) (default "mixed")
      --file string        Use real file instead of generated data
      --format string      Output format (table, json) (default "table")
  -h, --help               help for benchmark
      --size string        Size of test data to generate (e.g., 1MB, 10MB, 1GB) (default "10MB")
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

