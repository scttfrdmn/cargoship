## cargoship cost exhaustion

Predict when budget will be exhausted

### Synopsis

Predict when a budget will be exhausted based on current spending patterns.

Calculates:
- Exact date when budget will run out
- Days until exhaustion
- Probability of exhaustion (based on confidence intervals)
- Budget usage forecast with confidence bounds

Handles edge cases:
- Budget already exhausted (today)
- Budget never exhausts within 90 days
- High/low confidence scenarios

Examples:
  # Predict exhaustion for $1000 budget (global)
  cargoship cost exhaustion --budget 1000

  # Predict for specific project
  cargoship cost exhaustion 20251206-abc123 --budget 500

  # Include current spending
  cargoship cost exhaustion --budget 1000 --spent 400

  # Exhaustion prediction as JSON
  cargoship cost exhaustion --budget 1000 --json


```
cargoship cost exhaustion [PROJECT_ID] --budget AMOUNT [flags]
```

### Options

```
      --budget float   Total budget amount (required)
  -h, --help           help for exhaustion
      --spent float    Amount already spent (default: calculated from cost records)
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

