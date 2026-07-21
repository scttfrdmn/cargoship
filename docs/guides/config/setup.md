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

After setup, that config file supplies defaults for later commands, so a plain
`cargoship upload ./my-data s3://my-bucket/archives/` just works. You can edit the
file by hand at any time — see the [Configuration schema](/reference/configuration).

## See also

- [Config files & precedence](/guides/config/files) — how settings are resolved.
- [AWS setup & credentials](/start/aws-setup).
- [Quick Start](/start/quickstart).
- Reference: [Configuration & context commands](/reference/commands/config).
