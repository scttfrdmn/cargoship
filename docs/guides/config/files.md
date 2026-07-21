# Config files & precedence

Most of the time you drive CargoShip with flags. For settings you'd otherwise
repeat on every run — your region, storage class, budget limits, alert channels —
put them in a YAML config file instead. The `cargoship config` command generates,
inspects, and validates it. See the
[config command reference](/reference/commands/config) for the full flag list.

## Precedence

CargoShip resolves each setting from the first source that provides it, in this
order:

1. **Command-line flags** — highest priority; always win.
2. **Environment variables** — `CARGOSHIP_*`.
3. **Configuration file** — the YAML file (see locations below).
4. **Built-in defaults** — lowest priority.

So a `--region` flag overrides `CARGOSHIP_REGION`, which overrides `region:` in
the file, which overrides the built-in default. This lets you keep a stable base
config and override per-run without editing files.

::: tip Env vars for secrets and CI
Environment variables sit above the file, which makes `CARGOSHIP_*` ideal for
CI and for anything you don't want on disk. See
[Environment variables](/reference/environment-variables) for the full list.
:::

## File locations

CargoShip searches these paths in order and uses the first that exists:

1. `~/.cargoship.yaml`
2. `~/.config/cargoship/.cargoship.yaml`
3. `./.cargoship.yaml` (current directory)

A project-local `./.cargoship.yaml` is handy for per-dataset defaults committed
alongside the data. Point at an explicit file anywhere with `--file`.

## Generate a starter config

```bash
cargoship config --generate
```

Writes an example configuration with the common sections filled in and
commented. It covers AWS (region, profile), storage/upload optimization,
metrics, logging, security, and cost control. A trimmed example:

```yaml
aws:
  region: us-west-2
  profile: default

cost_control:
  enabled: true
  max_budget: 10000.0
  alert_threshold: 0.80
  max_volume_gb: 5000.0
  volume_alert_threshold: 0.75
```

Cost-control settings here are the **global** budget limits — see
[Budgets & volume quotas](/guides/cost/budgets) for how they interact with
per-project budgets.

## Show the active configuration

```bash
cargoship config --show
cargoship config --show --format json
```

Prints the fully-resolved configuration — after precedence is applied — so you
can confirm what CargoShip will actually use. Invaluable when a flag or env var
isn't taking effect as expected. Default output is YAML; `--format json` for
machine parsing.

## Validate before you rely on it

```bash
# Syntax and schema check
cargoship config --validate --file ~/.cargoship.yaml

# Also verify AWS connectivity and bucket access
cargoship config --validate-detailed
```

`--validate` checks the file parses and matches the schema. `--validate-detailed`
goes further and confirms credentials work and target buckets are reachable —
worth running once after editing before a large batch job.

## Editing

```bash
cargoship config --edit
```

Opens the config file in your default editor (`$EDITOR`).

## Best practices

::: tip
- **Generate, don't hand-write** the first config — `--generate` gets the schema right.
- **Keep machine-wide defaults in `~/.cargoship.yaml`**, per-project overrides in
  `./.cargoship.yaml`.
- **Put secrets in env vars**, not the file — `CARGOSHIP_*` overrides the file anyway.
- **Run `--show` when a setting seems ignored** — it reveals exactly what won after
  precedence.
- **Run `--validate-detailed` after edits** and before big jobs to catch bad
  credentials or bucket names early.
:::

## See also

- [Environment variables](/reference/environment-variables) — the `CARGOSHIP_*` list.
- [Configuration schema](/reference/configuration) — every field explained.
- [Interactive setup wizard](/guides/config/setup) — guided first-time setup.
- [Budgets & volume quotas](/guides/cost/budgets) — global cost-control settings.
- Reference: [Configuration & context commands](/reference/commands/config).
