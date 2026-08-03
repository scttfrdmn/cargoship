## cargoship dashboard

Launch comprehensive CargoShip TUI dashboard

### Synopsis

Launch the comprehensive CargoShip terminal user interface dashboard.

The TUI provides a full-featured interface with multiple views:
- 🏠 Overview: System status, storage usage, and quick stats
- 📦 Archive: Data archival operations, cost estimates, and survey results
- 📋 Inventory: Browse archived data, search, and restore operations
- 💰 Costs: Cost analysis, optimization suggestions, and budget tracking
- 🤖 Agents: Launch agent management and monitoring (when available)
- ⚙️ Config: Configuration management and profiles
- 📝 Logs: System logs and monitoring (coming soon)

Navigation:
  Tab/→ ←       - Switch between dashboard views
  1-7           - Quick jump to specific view
  ↑↓            - Focus different sections within view
  Enter         - Select/activate focused item
  R             - Force refresh data
  Q/Ctrl+C      - Exit dashboard

The dashboard adapts based on your current context and available features.

```
cargoship dashboard [flags]
```

### Examples

```
  # Launch dashboard with overview
  cargoship dashboard

  # Launch with specific view
  cargoship dashboard --view archive
  cargoship dashboard --view costs

  # Launch with agent context
  cargoship --context=agent dashboard
```

### Options

```
  -h, --help               help for dashboard
      --mock-data          Use mock data for testing (development only)
      --refresh duration   Override refresh interval (e.g., 5s, 1m)
      --view string        Initial dashboard view (overview, archive, inventory, costs, agents, config, logs) (default "overview")
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

