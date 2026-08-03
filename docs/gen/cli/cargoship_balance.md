## cargoship balance

Analyze shard balance for an uploaded dataset

### Synopsis

Analyze and rebalance shard distribution for a CargoShip upload.

This command downloads the manifest for an upload and analyzes the distribution
of files across shards. It identifies imbalanced shards and can automatically
rebalance them by redistributing files.

An upload is considered imbalanced if the largest shard is more than 2x the average
shard size (configurable with --threshold).

Examples:
  # Analyze shard balance only (read-only)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123

  # Show what would be rebalanced without executing
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --dry-run

  # Execute rebalancing (redistributes files across shards)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --execute

  # Check with custom threshold (3x instead of 2x)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --threshold 3.0

  # Output as JSON
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --format json

```
cargoship balance <s3://bucket/prefix/uploads/upload-id> [flags]
```

### Options

```
      --dry-run           Show rebalancing plan without executing
      --execute           Execute rebalancing (modifies data)
  -f, --format string     Output format (table, json) (default "table")
  -h, --help              help for balance
      --profile string    AWS profile to use
      --region string     AWS region
      --threshold float   Imbalance threshold (max/avg ratio) (default 2)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

