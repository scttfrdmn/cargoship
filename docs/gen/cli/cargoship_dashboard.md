## cargoship dashboard

Launch the CargoShip TUI dashboard

### Synopsis

Launch the CargoShip terminal dashboard — a read-only view over your real
data. It fabricates nothing; when a source has no data it says so.

Views (local, always available):
- 🏠 Overview: this month's recorded spend, budget used, in-progress uploads
- 💰 Costs:    recorded spend this month by storage class, plus budget status
- 📦 Uploads:  in-progress / resumable uploads and their progress

Passing an S3 target adds two bucket-backed views:
- 🗂️  Inventory: completed uploads, read from the manifests under the prefix
- 🔎 Analyze:   an on-demand bucket cost/savings scan (press 'a'; not automatic)

Local data comes from the cost ledger (see 'cargoship cost') and upload state
(see 'cargoship resume'). To browse and restore archived data, use
'cargoship browse'.

Navigation:
  Tab / ← →   Switch view      1-5   Jump to a view
  ↑ ↓         Move in a table   a     Analyze (with an S3 target)
  R           Refresh now       Q / Ctrl+C   Quit

```
cargoship dashboard [s3://bucket[/prefix]] [flags]
```

### Examples

```
  cargoship dashboard
  cargoship dashboard --view costs
  cargoship dashboard s3://my-bucket/backups   # adds Inventory + Analyze
```

### Options

```
  -h, --help               help for dashboard
      --profile string     AWS profile for the S3 target
      --refresh duration   Data refresh interval (e.g. 5s, 1m; default 5s)
      --region string      AWS region for the S3 target (auto-detected if empty)
      --view string        Initial view (overview, costs, uploads, inventory, analyze) (default "overview")
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

