## cargoship cost burnrate

Analyze burn rate with trend detection

### Synopsis

Analyze current and historical burn rates with trend detection.

Provides detailed burn rate metrics:
- Current rates (daily, weekly, monthly)
- Historical statistics (average, min, max, std dev, volatility)
- Trend detection (increasing/decreasing/stable) with strength
- Acceleration rate (change in burn rate per day)
- Predicted future burn rates (30/60/90 days)
- Confidence intervals for predictions

Examples:
  # Analyze burn rate for all projects (global)
  cargoship cost burnrate

  # Analyze burn rate for specific project
  cargoship cost burnrate 20251206-abc123

  # Analyze last 60 days
  cargoship cost burnrate --days 60

  # Burn rate as JSON
  cargoship cost burnrate --json


```
cargoship cost burnrate [PROJECT_ID] [flags]
```

### Options

```
      --days int   Number of days of historical data to analyze (7-365) (default 90)
  -h, --help       help for burnrate
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

