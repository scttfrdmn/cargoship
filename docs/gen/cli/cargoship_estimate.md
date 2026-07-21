## cargoship estimate

Estimate AWS costs for archiving data

### Synopsis

Estimate the cost of archiving data to AWS S3.

This command analyzes the specified directory and provides detailed cost estimates
for different storage classes, including storage, transfer, and request costs.

Examples:
  cargoship estimate ./research-data
  cargoship estimate /data --storage-class glacier --format json
  cargoship estimate . --show-recommendations --region us-west-2

```
cargoship estimate [path] [flags]
```

### Options

```
      --bandwidth float            Network bandwidth in MB/s for optimization (0 = auto-detect)
  -f, --format string              Output format (table, json) (default "table")
  -h, --help                       help for estimate
      --max-prefixes int           Maximum prefixes for parallel upload analysis (0 = auto)
      --real-time-pricing          Use real-time AWS pricing (requires AWS credentials)
      --region string              AWS region for cost calculation (default "us-east-1")
      --show-comparison            Show cost comparison: naive upload vs CargoShip chunking (Issue #169)
      --show-parallel              Show parallel upload optimization recommendations (default true)
      --show-recommendations       Show cost optimization recommendations (default true)
      --show-upload-optimization   Show intelligent upload sizing recommendations (default true)
  -s, --storage-class string       Target storage class for estimation
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

