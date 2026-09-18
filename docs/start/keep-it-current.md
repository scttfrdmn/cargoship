---
prev:
  text: Verify & restore it
  link: /start/verify-and-restore
next:
  text: Clean up
  link: /start/cleanup
---

# Keep it current

One upload is a snapshot. A backup is something that keeps up with your data. This
step turns the round trip you just proved into a repeatable one: **sync** only what
changed, then put it on a **schedule**.

Replace the paths and bucket URL with your own throughout.

## Sync only what changed

`cargoship sync` compares your directory against the previous upload's manifest and
uploads only new and modified files:

```bash
cargoship sync ./my-data s3://my-bucket/archives
```

The first run has nothing to compare against, so it uploads everything (a *full*
sync). Every run after that is *incremental* — it reads the last manifest, computes a
delta, and skips unchanged files. Each sync writes a new manifest that chains to the
previous one, so your history stays restorable version by version.

Look before you leap:

```bash
cargoship sync ./my-data s3://my-bucket/archives --dry-run
```

`--dry-run` prints what would be uploaded and exits without transferring anything.

Useful flags:

| Flag | Why |
| --- | --- |
| `--dry-run` | Preview the delta; upload nothing. |
| `--checksum` | Compare by SHA-256 instead of size+mtime — slower, catches same-size edits. |
| `--track-deletes` | Record files deleted locally, so restores reflect removals (opt-in; default is additive). |
| `--force` | Ignore the previous manifest and do a full sync. |
| `-q, --quiet` | Minimal output, for cron and scripts. |

Restoring works exactly as in the previous step, and understands the version chain: with
`--dataset-id <id>` you can add `--version N` or `--as-of YYYY-MM-DD` to restore an
older state. See [Incremental sync](/guides/sync) and
[dataset versioning](/guides/inspecting).

## Put it on a schedule

### Cron (simplest)

`sync` is exit-code correct and quiet-able, so it drops straight into cron. Nightly
at 02:00:

```bash
# crontab -e
0 2 * * * /usr/local/bin/cargoship sync /home/me/my-data s3://my-bucket/archives --quiet >> /var/log/cargoship.log 2>&1
```

Use an absolute binary path and make sure the AWS credentials the cron user sees are
the ones you configured in [AWS setup](/start/aws-setup) (cron has a minimal
environment — `AWS_PROFILE`/`AWS_REGION` may need setting in the crontab).

macOS launchd and systemd timers work equally well; there's nothing CargoShip-specific
about them.

### Unattended agents (a fleet)

If you're scheduling backups on machines you don't want to babysit — a NAS, lab
servers, a room full of workstations — use **ghostship** instead of hand-rolled cron.
It runs the same sync engine as a supervised, unattended agent and adds what a fleet
needs:

```bash
cargoship ghostship run ./my-data s3://my-bucket/archives \
  --writer-id my-box --interval 1h
```

Each agent is isolated to its own `writers/<id>/` prefix, can be given a
**write-only, delete-free** identity (it cannot read back, decrypt, or delete),
reports a heartbeat you read with `cargoship fleet status`, and can pull a **signed**
config from S3 instead of a local file. See the
[fleet tutorial](/enterprise/fleet-tutorial) for the full walkthrough, or
[ghost-ship](/enterprise/ghost-ship) for the reference.

::: tip Which do I want?
One machine you administer → **cron + `sync`**. Several machines backing themselves up
unattended, sharing one bucket → **a ghostship fleet**.
:::

## Watch the cost

Scheduled backups run without you watching, so set the guardrails once:

```bash
cargoship budget set my-project --cost 100 --volume 500   # $100 / 500 GB
cargoship budget status
```

CargoShip refuses a cycle that would exceed a configured quota *before* moving bytes,
and can alert you through email/Slack/webhook/CloudWatch (`cargoship alerts configure`).
See [Budgets & quotas](/guides/cost/budgets).

## Next

- [Clean up](/start/cleanup) — remove the test upload safely.
- [Incremental sync](/guides/sync) — the full sync reference.
- [Fleet tutorial](/enterprise/fleet-tutorial) — unattended agents, end to end.
