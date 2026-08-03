## cargoship cost benchmark-compare

Compare CargoShip costs vs competitors for benchmarking

### Synopsis

Calculate and compare costs for benchmark scenarios.

Shows CargoShip's cost advantages:
- Compression savings (20-70% data reduction)
- Intelligent chunking (50% fewer requests)
- Storage tier optimization (30-60% cost reduction)
- Deduplication (variable savings)

Output format is JSON for easy integration with benchmark scripts.

Examples:
  # Compare CargoShip (3:1 compression) vs competitor (no compression)
  cargoship cost benchmark-compare --size 100GB --files 10000 \
    --compression-ratio 3.0 --storage-class GLACIER

  # Compare with deduplication
  cargoship cost benchmark-compare --size 100GB --files 10000 \
    --compression-ratio 2.0 --dedup-ratio 2.0

  # Competitor cost only
  cargoship cost benchmark-compare --tool s5cmd --size 100GB --files 10000


```
cargoship cost benchmark-compare [flags]
```

### Options

```
      --chart                     Display ASCII cost comparison charts
      --compression-ratio float   Compression ratio (e.g., 3.0 for 3:1) (default 1)
      --dedup-ratio float         Deduplication ratio (e.g., 2.0 for 2:1) (default 1)
      --files int                 Number of files (required)
  -h, --help                      help for benchmark-compare
      --size-gb float             Data size in GB (required)
      --storage-class string      Storage class (default "STANDARD")
      --tool string               Tool name (s5cmd, rclone, aws-cli, cargoship)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --json                  Output as JSON
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

