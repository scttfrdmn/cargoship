## cargoship delete

Delete a CargoShip upload from S3

### Synopsis

Delete a complete CargoShip upload including all chunks, shards, and manifest.

The delete command removes an entire upload from S3:
1. Downloads manifest to identify all S3 objects
2. Deletes all chunk archives across all shards
3. Deletes the manifest file
4. Shows summary of deleted objects and saved costs

WARNING: This operation is IRREVERSIBLE. All data will be permanently deleted.

Use --dry-run to preview what would be deleted without actually deleting.
Use --force to skip confirmation prompt (dangerous for automation).

Examples:
  # Delete an upload (with confirmation)
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # Dry run to see what would be deleted
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234 --dry-run

  # Force delete without confirmation (automation)
  cargoship delete --bucket my-bucket --upload-id 20231208-123456-abcd1234 --force


```
cargoship delete [flags]
```

### Options

```
  -b, --bucket string      S3 bucket name (required)
      --dry-run            Show what would be deleted without actually deleting
      --force              Skip confirmation prompt (dangerous)
  -h, --help               help for delete
  -p, --prefix string      S3 prefix for upload (default: empty)
  -r, --region string      AWS region (default "us-west-2")
  -u, --upload-id string   Upload ID to delete (required)
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

