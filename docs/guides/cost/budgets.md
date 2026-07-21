# Budgets & volume quotas

Budgets turn cost tracking into cost *control*. CargoShip enforces two
independent limits per project — a **cost budget** in USD and a **volume quota**
in GB — and blocks an upload before it starts if it would exceed either one. No
data is transferred and no charges are incurred for a blocked upload.

This is the canonical budget page. For the analytics behind budgets (forecasts,
burn rate, exhaustion prediction) see [Cost management](/guides/cost/management);
for getting notified when limits approach, see [Alerts](/guides/cost/alerts).

::: tip Two commands, one system
`cargoship budget …` and `cargoship cost budget …` are the same subcommand tree
exposed under two names. Use whichever reads better in your workflow.
:::

## Quick start

```bash
# Set a $5000 cost budget and a 2000 GB volume quota for a project
cargoship budget set genomics-2026 --cost 5000 --volume 2000

# Check where it stands
cargoship budget status genomics-2026

# Upload against it — the budget is checked before the upload runs
cargoship upload ./dataset s3://research-data/genomics/ --project genomics-2026
```

If the upload would push the project over either limit, it's blocked with an
error showing current spend, the operation's estimated cost, the projected
total, and the overage.

## The dual-control model

CargoShip enforces budgets with two independent controls. An operation is
blocked if it would exceed **either** one.

**Cost budget (USD)** — a maximum spend. Tracks actual S3 costs (storage,
requests, transfer). Alerts by default at 80% of the limit.

**Volume quota (GB)** — a maximum data volume. Tracks total uploaded size.
Alerts by default at 75% of the limit.

Set either or both. Use `0` for unlimited on either axis:

```bash
cargoship budget set proj --cost 1000            # cost only, volume unlimited
cargoship budget set proj --volume 500           # volume only, cost unlimited
cargoship budget set proj --cost 1000 --volume 500  # both enforced
```

### Projects vs. global budgets

A **project** is a logical grouping of uploads — a grant, a cost center, an
experiment. The project ID is typically the manifest upload ID but can be any
identifier you pass to `--project`.

Enforcement is hierarchical:

1. **Project budget** — if the upload's `--project` has a budget set, that's checked.
2. **Global budget** — otherwise, the global limits from your
   [config file](/guides/config/files) apply.

Set global limits in `~/.cargoship.yaml`:

```yaml
cost_control:
  enabled: true
  max_budget: 10000.0          # global cost budget (USD)
  alert_threshold: 0.80        # alert at 80%
  max_volume_gb: 5000.0        # global volume quota (GB)
  volume_alert_threshold: 0.75 # alert at 75%
```

::: warning Enforcement must be enabled
Global budgets only take effect when `cost_control.enabled: true` is set in your
config. If uploads proceed past a limit you thought you set, check this first,
then confirm the `--project` on the upload matches the project you budgeted.
:::

## Setting and adjusting budgets

```bash
cargoship budget set <project-id> [flags]
```

| Flag | Meaning | Default |
|------|---------|---------|
| `--cost` | Max cost budget in USD (0 = unlimited) | 0 |
| `--volume` | Max volume quota in GB (0 = unlimited) | 0 |
| `--cost-alert` | Cost alert threshold, 0.0–1.0 | 0.8 |
| `--volume-alert` | Volume alert threshold, 0.0–1.0 | 0.75 |

```bash
# Custom alert thresholds — warn earlier on a risky grant
cargoship budget set clinical-2026 --cost 25000 --volume 15000 \
  --cost-alert 0.90 --volume-alert 0.85

# Raise a budget mid-period when funds are approved
cargoship budget set clinical-2026 --cost 30000

# Remove all limits (make unlimited)
cargoship budget set clinical-2026 --cost 0 --volume 0
```

Alert thresholds decide *when you're warned*, not when uploads are blocked —
blocking always happens at 100% of the limit. Lower thresholds for high-risk
projects, higher for stable ones.

## Checking status

```bash
cargoship budget status genomics-2026
```

```
Budget Status for Project: genomics-2026

=== Cost Budget ===
  Maximum Budget:    $5000.00
  Current Spending:  $1250.50
  Remaining:         $3749.50
  Usage:             25.0%
  Daily Burn Rate:   $125.05/day
  Projected Total:   $3751.50
  Status:            OK

=== Volume Quota ===
  Maximum Volume:    2000.00 GB
  Current Volume:    480.25 GB
  Remaining:         1519.75 GB
  Usage:             24.0%
  Daily Rate:        48.02 GB/day
  Projected Total:   1440.60 GB
  Status:            OK
```

The status includes a **daily burn rate** and a **projected end-of-period total**
for both cost and volume — so you're warned when the *trajectory* will exceed a
limit even if today's usage is fine. A status can read `WILL EXCEED` (projection
over limit), `ALERT TRIGGERED` (past the alert threshold), or `OVER BUDGET`
(already over).

Add `--json` for programmatic use:

```bash
cargoship budget status genomics-2026 --json
```

## Listing and removing

```bash
# List every configured project budget
cargoship budget list

# Remove a project's budget — it falls back to global limits
cargoship budget remove genomics-2026
```

## Common patterns

**Per-grant tracking.** Give each grant its own project and budget, then check
status for quarterly reporting:

```bash
cargoship budget set nih-r01-genomics --cost 10000 --volume 5000
cargoship budget set nih-r01-imaging  --cost 5000  --volume 2000
cargoship budget status nih-r01-genomics --json | jq '.budget_used'
```

**Cost-center allocation.** One project per business unit, scripted monthly
review:

```bash
for cc in engineering rnd datascience; do
  cargoship budget status costcenter-$cc --json
done
```

**Strict overrun prevention.** Low alert thresholds plus a hard limit means the
project warns early and blocks before any overage:

```bash
cargoship budget set clinical-trial --cost 25000 --cost-alert 0.90
```

When an upload is blocked, your options are to raise the budget (if funds
allow), remove some data, or defer to the next period.

## Exporting for external monitoring

`--json` makes budget data easy to push into CloudWatch, dashboards, or custom
alerting:

```bash
BUDGET_USED=$(cargoship budget status proj1 --json | jq '.budget_used')
aws cloudwatch put-metric-data --namespace CargoShip/Budget \
  --metric-name BudgetUsed --value "$BUDGET_USED" --dimensions Project=proj1
```

For built-in notification channels (email, Slack, webhook, CloudWatch), use
[Alerts & notifications](/guides/cost/alerts) instead of rolling your own.

::: details How enforcement works under the hood
Every upload runs a check sequence before transferring data:

1. **Estimate** the operation's cost and volume from data size, storage class,
   and region.
2. **Check the cost budget** — is projected spend within the limit?
3. **Check the volume quota** — is projected volume within the limit?
4. **Proceed or block.**

Costs are estimated up front and reconciled with actuals after the upload
completes. Budget tracking starts when you set a budget — uploads that happened
*before* a budget existed aren't retroactively counted. Programmatic access to
the same checks (`CheckProjectBudget`, `CheckProjectVolumeQuota`,
`SetProjectBudget`, `GetProjectBudgetStatus`) lives in the
`github.com/scttfrdmn/cargoship/pkg/aws/cost` package.
:::

## Best practices

::: tip
- **Budget per project, not globally**, once you have more than one workstream —
  it's the only way to attribute and cap spend independently.
- **Set both cost and volume limits** for grants; volume is often the harder
  constraint on shared storage allocations.
- **Alert early (75–80%), block at 100%** — the defaults are sensible; tighten
  alerts for high-risk projects.
- **Enable `cost_control` globally** so untagged uploads still hit a ceiling.
- **Watch `Projected Total`, not just current usage** — the burn-rate projection
  catches trouble before you hit the wall.
- **Wire `--json` status into your monitoring** for a single pane of glass.
:::

## See also

- [Estimating costs](/guides/cost/estimate) — size a budget before setting it.
- [Cost management & reporting](/guides/cost/management) — forecasts and burn rate.
- [Alerts & notifications](/guides/cost/alerts) — get told when limits approach.
- [Config files & precedence](/guides/config/files) — where global limits live.
- Reference: [Cost, budget & alerts commands](/reference/commands/cost).
