## cargoship dataset prune

Delete old versions of a dataset, keeping the newest N (garbage collection)

### Synopsis

Reclaim storage by deleting old versions of a dataset, keeping the newest N.

Only objects no kept version still references are removed. The oldest kept
version is first rewritten self-contained (compaction) so the pruned versions'
manifests can be removed without breaking it; the original is backed up to a
".pre-compact.bak" object first, and the rewrite is verified before anything is
deleted.

WARNING: deletion is IRREVERSIBLE. Use --dry-run to preview. Encrypted-manifest
datasets and mixed direct/chunked chains are not yet supported.

```
cargoship dataset prune S3_URL --dataset-id ID --keep-last N [flags]
```

### Options

```
      --dataset-id string   Dataset ID to prune (from 'cargoship dataset list')
      --dry-run             Preview what would be pruned without deleting
      --force               Skip the confirmation prompt
  -h, --help                help for prune
      --keep-last int       Keep the newest N versions; delete older ones (required, >= 1)
  -r, --region string       AWS region (default "us-west-2")
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

