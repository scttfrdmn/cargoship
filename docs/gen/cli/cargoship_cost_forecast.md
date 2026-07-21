## cargoship cost forecast

Generate cost forecasts with ML-based projections

### Synopsis

Generate cost forecasts using multiple forecasting models.

Predicts future costs based on historical spending patterns using:
- Linear regression (best for stable/trending patterns)
- Exponential smoothing (for seasonal patterns)
- Moving average (for smoothing volatility)
- Ensemble model (combines all models)

Displays:
- Predicted costs at 7, 14, 30, 60, 90 days
- Confidence intervals (90%, 95%, 99%)
- Model accuracy metrics (R², MAE, RMSE)
- Daily cost forecasts

Examples:
  # Generate forecast for all projects (global)
  cargoship cost forecast

  # Generate forecast for specific project
  cargoship cost forecast 20251206-abc123

  # Forecast with specific model
  cargoship cost forecast --model linear

  # Forecast as JSON
  cargoship cost forecast --json


```
cargoship cost forecast [PROJECT_ID] [flags]
```

### Options

```
      --days int       Number of days to forecast (7-90) (default 90)
  -h, --help           help for forecast
      --model string   Forecasting model (linear, exponential, moving_average, ensemble) (default "linear")
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, controller, repl)
      --json                  Output as JSON
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -r, --region string         AWS region (default "us-west-2")
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

