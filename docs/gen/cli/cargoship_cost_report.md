## cargoship cost report

Generate cost report

### Synopsis

Generate detailed cost report for a time period or compliance report.

Standard mode (default):
  Shows total costs, breakdown by service/region, trends, and recommendations.

Compliance mode (--format compliance):
  Generates an NSF or NIH data-management compliance report for a specific
  budget/project, including data provenance, reproducibility info, and DMP.

Examples:
  # Monthly report
  cargoship cost report --period month

  # Export report to file
  cargoship cost report --period month --output report.json

  # NSF compliance report
  cargoship cost report --budget my-project-id --grant NSF-2024-12345 --format compliance

  # NIH compliance report (text)
  cargoship cost report --budget my-project-id --grant R01-GM123456 --format compliance --agency NIH --text


```
cargoship cost report [flags]
```

### Options

```
      --agency string   Funding agency for compliance report (NSF or NIH) (default "NSF")
      --budget string   Budget/project ID for compliance report
      --format string   Output format: compliance (enables compliance report mode)
      --grant string    Grant/award number (e.g., NSF-2024-12345, R01-GM123456)
  -h, --help            help for report
      --output string   Output file path (default: stdout)
      --period string   Report period (today, week, month, last_month) (default "month")
      --text            Render compliance report as human-readable text (default: JSON)
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

