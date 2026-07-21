# Principal investigator

**You are:** a PI running several grants at once. You don't touch individual
uploads — your lab manager does that. You need **budget visibility**, **early
warning before an overrun**, and **sponsor-ready reports** at renewal time, with
minimal time spent per week.

This tutorial shows the PI-facing slice of CargoShip: portfolio-level cost views,
forecasting, and compliance reporting. The day-to-day setup lives in the
[Lab data manager](/tutorials/lab-manager) tutorial.

## The prerequisite: consistent project tags

Everything below depends on uploads being tagged with a stable `--project` ID per
grant. If your lab manager has followed the
[lab-manager tutorial](/tutorials/lab-manager), this is already true and you can
just read the numbers out.

## Step 1 — See the whole portfolio

Roll spend up across every project, then drill into any one grant:

```bash
cargoship cost projects              # every grant's spend, side by side
cargoship cost project nih-r01       # one grant in detail
cargoship cost report --period month # this month across the portfolio
```

Periods accept `today`, `week`, `month`, `last_month`. See
[Cost management & reporting](/guides/cost/management).

## Step 2 — Forecast before the money's gone

The point of forecasting is to catch a grant trending over budget while there's
still time to defer or rebalance uploads:

```bash
# Where is NSF headed this period?
cargoship cost forecast nsf-grant --model ensemble --days 90

# How fast is it burning?
cargoship cost burnrate nsf-grant

# When does the budget run out at the current rate?
cargoship cost exhaustion nsf-grant --budget 90000
```

Four forecasting models are available — `linear`, `exponential`,
`moving_average`, and `ensemble`. `ensemble` blends them and is the safe default
for a portfolio view.

::: tip The PI workflow
Skim `cargoship cost projects` weekly. Only drill into `forecast` / `exhaustion`
for a grant your lab manager flags. That's the whole time commitment — the
detailed management stays delegated.
:::

## Step 3 — Keep budgets enforced, not just observed

Forecasting tells you where things are heading; a budget with an alert threshold
makes the system tell *you*. Your lab manager sets these per grant:

```bash
cargoship budget set nsf-grant --cost 90000 --volume 20000 --cost-alert 0.8
cargoship budget status nsf-grant
```

At 80% of the cost ceiling, alerts fire. Wire them to email or Slack so a
threshold breach reaches you without anyone checking a dashboard — see
[Alerts & notifications](/guides/cost/alerts) and
[Budgets & volume quotas](/guides/cost/budgets).

## Step 4 — Sponsor-ready reports at renewal

When a grant comes up for renewal, sponsors want a defensible accounting of cloud
spend tied to the award. Generate a compliance report:

```bash
cargoship cost report \
  --grant NSF-DBI-2024-12345 \
  --agency NSF \
  --format compliance \
  --text \
  --output nsf-dbi-renewal-2026.txt
```

- `--grant` stamps the award/grant number onto the report.
- `--agency` selects the funding-agency format (`NSF` or `NIH`).
- `--format compliance` switches on compliance mode; add `--text` for a
  human-readable file (omit it for JSON you can feed to another tool).

This turns what used to be days of parsing AWS bills into one command whose
output you can attach to the renewal. See
[Cost management & reporting](/guides/cost/management).

## Step 5 — Rebalancing across grants

There's no automatic cross-grant transfer command — budgets are per project by
design, so reallocation is a decision you make, not a button. The tooling that
supports that decision is what you've already seen:

1. `cargoship cost projects` shows which grants are under- and over-spending.
2. `cargoship cost forecast <id>` confirms the trend is real, not a one-off spike.
3. Adjust the ceilings with `cargoship budget set <id> --cost <new>` on the
   affected projects.

## Recap

- Your visibility rests on consistent `--project` tags (lab manager's job).
- `cost projects` weekly; `forecast` / `burnrate` / `exhaustion` for flagged grants.
- Budgets with `--cost-alert` plus [alerts](/guides/cost/alerts) push warnings to you.
- `cost report --grant --agency --format compliance` produces renewal-ready output.
- Reallocation is a manual `budget set` decision, informed by the cost views.

## Next steps

- [Cost management & reporting](/guides/cost/management) · [Budgets & quotas](/guides/cost/budgets) · [Alerts](/guides/cost/alerts).
- [Lab data manager](/tutorials/lab-manager) — the day-to-day setup behind these numbers.
- [`cost` reference](/reference/commands/cost).
