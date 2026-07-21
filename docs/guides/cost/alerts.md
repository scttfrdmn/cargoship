# Alerts & notifications

When a [budget](/guides/cost/budgets) crosses its alert threshold, its hard
limit, or its projected exhaustion, CargoShip can notify you over four channels:
email (SMTP), Slack, custom webhooks, and AWS CloudWatch. Alerts are delivered
independently — one channel failing doesn't block the others.

::: warning Opt-in, disabled by default
Every alert channel is **off until you configure and enable it**. This is a
deliberate security default: CargoShip never sends anything outbound (or handles
your SMTP password / webhook URL) until you explicitly turn a channel on. See
[Security best practices](#security) below.
:::

For the full flag list, see the [cost command reference](/reference/commands/cost).

## Alert types and severities

Alerts fire on these conditions:

| Type | Fires when |
|------|-----------|
| `cost_threshold` | Spend crosses the cost alert threshold (e.g. 80%) |
| `volume_threshold` | Volume crosses the volume alert threshold (e.g. 75%) |
| `cost_over_budget` | Spend exceeds the maximum budget |
| `volume_over_quota` | Volume exceeds the maximum quota |
| `budget_projection` | Forecast shows the budget will be exhausted |
| `volume_projection` | Forecast shows the quota will be exhausted |

Each carries a severity: `info` (below threshold), `warning` (past threshold,
under max), or `critical` (over max / action required). Slack messages are
color-coded by severity.

## Email (SMTP)

```bash
cargoship alerts configure email \
  --smtp-host smtp.gmail.com \
  --smtp-port 587 \
  --smtp-username alerts@example.com \
  --smtp-password "your-app-password" \
  --smtp-from "cargoship@example.com" \
  --recipients admin@example.com,finance@example.com
```

| Flag | Notes |
|------|-------|
| `--smtp-host` / `--smtp-port` | Server and port. `587` for TLS (STARTTLS), `465` for SSL |
| `--smtp-username` / `--smtp-password` | Auth credentials |
| `--smtp-from` | From address |
| `--recipients` | Comma-separated recipient list |
| `--smtp-use-tls` | TLS on by default (`true`) |

::: details Provider-specific setup (Gmail, Office 365, AWS SES)
**Gmail** requires an App Password, not your account password. Enable 2-Step
Verification, then generate an App Password for "Mail" and use it as
`--smtp-password`.

**Office 365**: `--smtp-host smtp.office365.com --smtp-port 587`.

**AWS SES**: `--smtp-host email-smtp.<region>.amazonaws.com --smtp-port 587`,
using SES SMTP credentials. Verify the sender address/domain and move out of the
SES sandbox for production.
:::

## Slack

```bash
cargoship alerts configure slack \
  --webhook-url "https://hooks.slack.com/services/T00/B00/abc123" \
  --channel "#cargoship-alerts" \
  --username "CargoShip Monitor"
```

Create the webhook URL in your Slack workspace under **Incoming Webhooks**
(`https://api.slack.com/apps` → create app → activate Incoming Webhooks → add to
a channel). `--channel` and `--username` are optional overrides.

## Custom webhook

Send alerts as an HTTP POST with a JSON body to any endpoint:

```bash
cargoship alerts configure webhook \
  --webhook-url "https://your-server.com/api/alerts" \
  --headers "Authorization: Bearer token123,X-API-Key: key456"
```

The payload includes the alert type, severity, project ID, cost and volume
metrics, a recommendation, and an `action_required` flag:

```json
{
  "id": "alert-1733745000",
  "timestamp": "2026-07-21T10:30:00Z",
  "type": "cost_over_budget",
  "severity": "critical",
  "project_id": "genomics-2026",
  "max_budget": 1000.00,
  "current_spend": 1050.00,
  "budget_used_percent": 1.05,
  "recommendation": "Halt operations or increase budget",
  "action_required": true
}
```

## CloudWatch

```bash
cargoship alerts configure cloudwatch \
  --namespace "CargoShip/Production" \
  --metric-name "BudgetAlert"
```

Each alert publishes a metric (dimensioned by project, alert type, and severity)
that you can alarm on with `aws cloudwatch put-metric-alarm`. Requires
`cloudwatch:PutMetricData` in your IAM policy.

## Test, enable, disable

Configuring a channel stores its settings; you still enable it, and you should
test it first. Testing works even before a channel is enabled, so you can verify
credentials safely.

```bash
# Test all configured channels
cargoship alerts test

# Test one channel at a specific severity
cargoship alerts test --channel email --severity critical

# Enable / disable a channel
cargoship alerts enable email
cargoship alerts disable slack

# Review current configuration (secrets masked)
cargoship alerts config
```

A failed test prints the underlying error (e.g. SMTP `535 Authentication
failed`) with troubleshooting hints — much faster than waiting for a real alert
that never arrives.

::: tip Cooldown prevents spam
Alerts have a cooldown period (24h by default) so a project sitting just over a
threshold doesn't notify on every upload. Shorten it for critical projects,
lengthen it for stable ones.
:::

## Troubleshooting

- **Email auth fails** — for Gmail confirm you're using an App Password; check the
  port (587 TLS vs 465 SSL) and that outbound SMTP isn't firewalled.
- **Slack test passes but no message** — verify the webhook URL and that the
  channel name includes `#`; regenerate the webhook if it was revoked.
- **Webhook 401/403** — add auth via `--headers`.
- **CloudWatch metrics missing** — confirm `cloudwatch:PutMetricData` permission
  and that `AWS_REGION` matches where you're looking.

## Security

::: tip
- **Keep channels disabled until tested** — the opt-in default is a feature; don't
  bypass it in automation.
- **Never commit credentials.** SMTP passwords and webhook URLs are secrets — pass
  them from a secrets manager or environment, not a checked-in config.
- **Use a dedicated sender** (`cargoship-alerts@…`) and a dedicated Slack channel,
  not a personal account or a busy general channel.
- **Keep TLS on** for email and HTTPS for webhooks; authenticate webhook
  endpoints and rotate credentials periodically.
- **Layer channels**: email for the record, Slack for team visibility, CloudWatch
  for monitoring integration.
:::

## See also

- [Budgets & volume quotas](/guides/cost/budgets) — the limits alerts fire on.
- [Cost management & reporting](/guides/cost/management) — forecasts behind projection alerts.
- Reference: [Cost, budget & alerts commands](/reference/commands/cost).
