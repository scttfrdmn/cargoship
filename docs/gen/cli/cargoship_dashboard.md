## cargoship dashboard

Launch the CargoShip TUI dashboard

### Synopsis

Launch the CargoShip terminal dashboard — a read-only view over your real
local data. It fabricates nothing; when a source has no data it says so.

Views:
- 🏠 Overview: this month's recorded spend, budget used, in-progress uploads
- 💰 Costs:    recorded spend this month by storage class, plus budget status
- 📦 Uploads:  in-progress / resumable uploads and their progress

Data comes from the local cost ledger (see 'cargoship cost') and local upload
state (see 'cargoship resume'); the dashboard makes no live bucket scans. To
browse and restore archived data interactively, use 'cargoship browse'.

Navigation:
  Tab / ← →   Switch view      1-3   Jump to a view
  ↑ ↓         Move in a table   R     Refresh now      Q / Ctrl+C   Quit

```
cargoship dashboard [flags]
```

### Examples

```
  cargoship dashboard
  cargoship dashboard --view costs
  cargoship dashboard --refresh 10s
```

### Options

```
  -h, --help               help for dashboard
      --refresh duration   Data refresh interval (e.g. 5s, 1m; default 5s)
      --view string        Initial view (overview, costs, uploads) (default "overview")
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

