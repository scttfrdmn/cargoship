# Configuration schema

CargoShip stores settings in a YAML configuration file covering AWS, storage,
upload optimization, metrics, logging, and security. The file supplies defaults so
you don't have to repeat flags on every command; anything in it can still be
overridden by an environment variable or a command-line flag.

Generate an annotated example, then view or validate your active configuration:

```bash
cargoship config --generate           # write an example config
cargoship config --show                # show the resolved configuration
cargoship config --validate --file ~/.cargoship.yaml
```

Configuration files are searched in this order:

1. `~/.cargoship.yaml`
2. `~/.config/cargoship/.cargoship.yaml`
3. `./.cargoship.yaml`

The fastest way to create one is the [interactive setup wizard](/guides/config/setup)
(`cargoship setup`), which writes `~/.cargoship.yaml` for you.

::: warning Draft
This page is being expanded into a field-by-field schema reference. Until then,
`cargoship config --generate` produces a fully commented example that documents
every section inline.
:::

## See also

- [Interactive setup wizard](/guides/config/setup).
- [Config files & precedence](/guides/config/files).
- [Environment variables](/reference/environment-variables).
- Reference: [Configuration & context commands](/reference/commands/config).
