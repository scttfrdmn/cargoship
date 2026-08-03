## cargoship scuttle

🚨 NUCLEAR OPTION: Delete ALL CargoShip data from a bucket/prefix

### Synopsis

🚨 NUCLEAR OPTION: Delete ALL CargoShip uploads from a bucket or prefix.

The scuttle command is the nuclear option that deletes EVERYTHING:
- ALL uploads (all manifests, all chunks, all shards)
- ALL data under the specified prefix
- This operation is IRREVERSIBLE and PERMANENT

This command is named after "scuttling a ship" - deliberately sinking your own vessel.
Use this when:
- Decommissioning a test environment
- Cleaning up after development
- Starting fresh with a new bucket structure
- You're absolutely certain you want to delete EVERYTHING

⚠️  EXTREME DANGER WARNING ⚠️
This command requires TRIPLE confirmation unless --force is used.
All data will be permanently destroyed with no recovery option.

Safety features:
- Requires typing the full bucket name to confirm
- Requires typing "SCUTTLE" to confirm deletion
- Shows detailed preview of what will be deleted
- --dry-run to preview without deleting
- Rate-limited to prevent accidental mass deletion

Examples:
  # Scuttle all data in a bucket (with triple confirmation)
  cargoship scuttle --bucket my-test-bucket

  # Scuttle only a specific prefix
  cargoship scuttle --bucket my-bucket --prefix test-data/

  # Dry run to see what would be deleted
  cargoship scuttle --bucket my-bucket --dry-run

  # Force scuttle without confirmation (DANGEROUS)
  cargoship scuttle --bucket my-bucket --force


```
cargoship scuttle [flags]
```

### Options

```
  -b, --bucket string   S3 bucket name (required)
      --dry-run         Show what would be deleted without actually deleting
      --force           Skip ALL confirmations (EXTREMELY DANGEROUS)
  -h, --help            help for scuttle
  -p, --prefix string   S3 prefix to delete (default: entire bucket)
  -r, --region string   AWS region (default "us-west-2")
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

