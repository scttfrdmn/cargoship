# Interactive setup wizard

`cargoship setup` is an interactive wizard that walks you through configuring
CargoShip for the first time. It's the easiest way to get from a fresh install to
a working upload without reading through every flag: it configures your AWS
credentials and region, verifies S3 bucket access, suggests sensible upload
parameters for your use case, and tests the result — then writes it all to a
`.cargoship.yaml` file in your home directory.

```bash
cargoship setup
```

Save the configuration somewhere other than the default, or run without prompts
using defaults:

```bash
cargoship setup --output /path/to/config.yaml
cargoship setup --non-interactive
```

## Running setup again

If the config file already exists, setup treats its current values as the
defaults, so pressing Enter at a prompt keeps what you have. Before changing
anything it asks you to type `yes` (or pass `--force`), and `--non-interactive`
refuses to touch an existing file without `--force`. Setup updates only the
settings it asks about (the `aws`, `storage`, `upload`, `metrics` and `logging`
sections). Everything else in the file, such as `cost_control` budgets, keys it
doesn't know and your comments, is kept, and the previous file is saved next to
it as `.cargoship.yaml.bak`.

```bash
cargoship setup --non-interactive --force   # re-apply, keeping current values
```

After setup, that config file supplies defaults for later commands, so a plain
`cargoship upload ./my-data s3://my-bucket/archives/` just works. You can edit the
file by hand at any time — see the [Configuration schema](/reference/configuration).

## See also

- [Config files & precedence](/guides/config/files) — how settings are resolved.
- [AWS setup & credentials](/start/aws-setup).
- [Quick Start](/start/quickstart).
- Reference: [Configuration & context commands](/reference/commands/config).
