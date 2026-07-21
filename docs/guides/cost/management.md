# Cost management & reporting

Once data is uploaded, CargoShip tracks what it actually cost and helps you
understand where the money goes. Everything is organized around **projects** —
each upload is tagged with a [project ID](/intro/concepts) (the manifest upload
ID, or whatever you pass to `--project`), and costs roll up per project.

The `cargoship cost` command tree covers reporting, forecasting, and burn-rate
analysis. For the exhaustive flag list, see the
[cost command reference](/reference/commands/cost).

::: tip Tag your uploads
Cost tracking is only as useful as your tagging. Pass `--project` on every
upload so spend attributes to something meaningful:

```bash
cargoship upload ./data s3://my-bucket/archives/ --project genomics-2026
```
:::

## List projects and their spend

```bash
cargoship cost projects
```

Lists every project that has cost records, with total cost per project. Add
`--json` for scripting. To drill into a single project:

```bash
cargoship cost project 20251206-abc123
```

This shows the cost breakdown by region and storage class, files processed,
data volume, compression savings, and a daily timeline.

## Generate a cost report

```bash
cargoship cost report --period month
```

Produces a detailed report for a time window (`today`, `week`, `month`,
`last_month`) with totals, breakdowns by service/region/storage class, trends,
and optimization recommendations. Save it to a file or emit JSON:

```bash
cargoship cost report --period month --output report.json --json
```

::: details Grant compliance reports (NSF / NIH)
`cost report` can also generate a data-management compliance report for a
specific project, suitable for federal grant reporting:

```bash
# NSF compliance report (JSON)
cargoship cost report --budget my-project-id --grant NSF-2024-12345 --format compliance

# NIH compliance report as human-readable text
cargoship cost report --budget my-project-id --grant R01-GM123456 \
  --format compliance --agency NIH --text
```

The report includes data provenance, reproducibility info, and the data
management plan. Use `--agency NSF` (default) or `--agency NIH`.
:::

## Forecast future spend

CargoShip fits your spending history to a model and projects forward at 7, 14,
30, 60, and 90 days, with confidence intervals and model-accuracy metrics
(R², MAE, RMSE).

```bash
# Forecast across all projects
cargoship cost forecast

# Forecast one project with a specific model
cargoship cost forecast 20251206-abc123 --model ensemble --days 60
```

Four models are available via `--model`:

| Model | Best for |
|-------|----------|
| `linear` (default) | Stable or steadily-trending spend |
| `exponential` | Accelerating / seasonal growth |
| `moving_average` | Smoothing out volatile day-to-day spend |
| `ensemble` | Combines all three; most robust for production |

::: tip Trust the fit, not just the number
Check the reported R² before acting on a forecast. Above 0.8 is a good fit,
above 0.9 is excellent. A low R² usually means you don't have enough history yet
(aim for at least 7 days) or a large one-off upload skewed the trend.
:::

## Analyze burn rate

Burn rate is how fast you're spending. `cost burnrate` reports current daily,
weekly, and monthly rates alongside historical stats (average, min, max,
volatility) and a trend direction with predicted future rates.

```bash
cargoship cost burnrate --days 90
cargoship cost burnrate 20251206-abc123 --days 60
```

## Predict budget exhaustion

Given a budget and current spend, predict the date it runs out:

```bash
cargoship cost exhaustion --budget 1000 --spent 450
cargoship cost exhaustion 20251206-abc123 --budget 500
```

Returns the exhaustion date, days remaining, and probability bounds. This is the
analytic behind budget projection alerts — to *enforce* a limit rather than just
predict it, set a budget (see below).

## From reporting to enforcement

Reporting tells you what happened; **budgets** stop overspending before it does.
The two share a data model — `cargoship cost budget …` and `cargoship budget …`
are the same subcommand tree. Once you understand your spend here, cap it in
[Budgets & volume quotas](/guides/cost/budgets) and get notified via
[Alerts & notifications](/guides/cost/alerts).

## Best practices

::: tip
- **Tag `--project` from day one** — retroactive attribution isn't possible.
- **Review `cost projects` weekly** to catch a runaway project early.
- **Forecast with `--model ensemble`** for production; check R² before trusting it.
- **Watch the burn-rate trend, not just today's number** — acceleration is the
  early warning.
- **Pipe `--json` into your monitoring** to chart spend over time.
- **Turn insight into limits**: once a project's steady-state cost is clear, set a
  budget so surprises get blocked, not just reported.
:::

## See also

- [Estimating costs](/guides/cost/estimate) — model cost before uploading.
- [Budgets & volume quotas](/guides/cost/budgets) — enforce spending and volume limits.
- [Alerts & notifications](/guides/cost/alerts) — get notified on thresholds.
- Reference: [Cost, budget & alerts commands](/reference/commands/cost).
