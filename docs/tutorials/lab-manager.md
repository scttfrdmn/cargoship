# Lab data manager

**You are:** the person who keeps a multi-project lab's data organized and its
cloud spend attributable. Several grants, many students, one recurring headache
— *who uploaded what, to which budget, and why is storage creeping up?*

This tutorial sets up project-tagged uploads, per-project budgets with alert
thresholds, deduplication for the redundant reference files students keep
re-uploading, and monthly reporting.

## The model: one project ID per grant

CargoShip attributes cost by **project**. Give each grant a stable project ID and
have everyone pass it on every upload — that single convention is what makes
budgets, reports, and forecasts work.

| Grant | Project ID | Bucket |
|-------|-----------|--------|
| NIH R01 | `nih-r01` | `chen-lab-nih-r01` |
| NSF | `nsf-grant` | `chen-lab-nsf` |
| Industry | `industry` | `chen-lab-industry` |

## Step 1 — Set a budget per project

```bash
cargoship budget set nih-r01 --cost 2000 --volume 500
cargoship budget set nsf-grant --cost 1200 --volume 300
cargoship budget set industry --cost 800 --volume 200
```

`--cost` is a USD ceiling, `--volume` a GB ceiling — a budget can enforce either
or both. Alert thresholds default to 80% (cost) and 75% (volume); tune with
`--cost-alert` / `--volume-alert`. See [Budgets & volume quotas](/guides/cost/budgets).

Check any project at any time:

```bash
cargoship budget status nih-r01
cargoship budget list
```

## Step 2 — A house style for uploads

Publish one command shape for the lab. Every upload carries the project ID
(`--project`) plus free-form `--tag`s for the uploader and experiment, so cost
rolls up by grant *and* slices by person:

```bash
cargoship upload ./rnaseq-liver \
  s3://chen-lab-nih-r01/james-kim/liver-rnaseq-2026-05/ \
  --project nih-r01 \
  --tag user=james-kim --tag assay=rnaseq --tag tissue=liver
```

`--tag key=value` is repeatable. Because the tags live in the manifest and on the
objects, they show up in cost reporting later without any spreadsheet.

::: tip Train the lab on two rules
1. **Always pass `--project <grant>`** — it's what makes spend attributable.
2. **Always `--tag user=<you>`** — it's what makes per-person breakdowns possible.
:::

## Step 3 — Estimate large uploads first

Before a student pushes a 400 GB screening set against a tight budget, have them
preview it:

```bash
cargoship estimate ./drug-screen-q2 --show-comparison
```

Then check headroom with `cargoship budget status industry` before running the
upload. See [Estimating costs](/guides/cost/estimate).

## Step 4 — Stop paying for duplicate reference files

The classic lab money leak: five students each upload their own copy of a 32 GB
reference genome. CargoShip can hash files during upload and store identical
content once:

```bash
cargoship upload ./project-with-shared-refs \
  s3://chen-lab-nih-r01/shared/ \
  --project nih-r01 \
  --enable-dedup
```

`--enable-dedup` adds a hashing pass and skips storing identical files more than
once — 10–30% savings on datasets with redundancy. Better still, keep genuinely
shared references (genomes, annotations, compound libraries) in **one** prefix
everyone reads from, instead of copying them into each project. See
[Deduplication](/guides/uploading#deduplication).

To audit what already exists in a bucket and find waste, analyze current spend:

```bash
cargoship analyze s3://chen-lab-nih-r01/
```

See [Analyzing existing S3 spend](/guides/cost/analyze).

## Step 5 — Monthly reporting for grant accounting

At month-end, produce a per-project cost report:

```bash
cargoship cost report --period last_month
cargoship cost projects            # spend rolled up by project
cargoship cost project nih-r01     # one project's detail
```

For sponsor-facing compliance output tied to a grant/award number:

```bash
cargoship cost report --grant R01-GM123456 --agency NIH \
  --format compliance --text --output nih-r01-2026-05.txt
```

See [Cost management & reporting](/guides/cost/management).

## Step 6 — See spend before it becomes a surprise

Rather than discovering an overrun at month-end, watch the trajectory:

```bash
cargoship cost forecast nsf-grant --model ensemble
cargoship cost burnrate nsf-grant
cargoship cost exhaustion nsf-grant --budget 1200
```

`forecast` projects spend forward, `burnrate` shows the daily rate, and
`exhaustion` estimates when a budget runs out at the current pace — the early
warning that lets you rebalance uploads across the month.

## Recap

- One stable **project ID per grant**; everyone passes `--project` and `--tag user=`.
- Set `budget set <id> --cost --volume` with alert thresholds.
- `estimate` big uploads and check `budget status` before running them.
- `--enable-dedup` plus a single shared-reference prefix kills duplicate-file waste.
- `cost report` / `cost projects` for accounting; `forecast` / `burnrate` /
  `exhaustion` to steer before overruns.

## Next steps

- [Budgets & volume quotas](/guides/cost/budgets) · [Cost management](/guides/cost/management) · [Analyzing spend](/guides/cost/analyze).
- [Alerts & notifications](/guides/cost/alerts) — email/Slack on threshold breach.
- Overseeing budgets at the grant-portfolio level? See [Principal investigator](/tutorials/principal-investigator).
- [`cost` reference](/reference/commands/cost).
