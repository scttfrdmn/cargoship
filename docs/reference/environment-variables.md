# Environment variables

CargoShip reads two families of environment variables: the standard `AWS_*`
variables (via the AWS SDK credential chain) and its own `CARGOSHIP_*` settings.

::: info Precedence
Settings resolve highest-priority first: **command-line flags → environment
variables → configuration file → built-in defaults.** See
[Config files & precedence](/guides/config/files).
:::

## AWS & standard variables

CargoShip uses your standard AWS credential chain — the same one the AWS CLI uses.
These are consumed by the AWS SDK, not read directly:

| Variable | Purpose |
|----------|---------|
| `AWS_REGION` | Default region (override per command with `--region`). |
| `AWS_PROFILE` | Named profile from `~/.aws/config` / `~/.aws/credentials`. |
| `AWS_ACCESS_KEY_ID` | Access key (when not using a profile or role). |
| `AWS_SECRET_ACCESS_KEY` | Secret key. |
| `AWS_SESSION_TOKEN` | Session token for temporary credentials. |
| `AWS_ENDPOINT_URL` | S3-compatible endpoint override (Wasabi, B2, MinIO). |

CargoShip also honors a few standard tool/runtime variables:

| Variable | Purpose |
|----------|---------|
| `EDITOR` / `VISUAL` | Editor launched by `cargoship config --edit`. |
| `GOMEMLIMIT` / `GOGC` | Go runtime memory/GC tuning, respected if set. |
| `XDG_DATA_HOME` | Base directory for restore-job state (see [Restoring](/guides/restoring)). |

See [AWS setup & credentials](/start/aws-setup) for the minimal IAM policy.

## CargoShip variables

::: warning Few CARGOSHIP_* variables are read explicitly
For normal `cargoship upload` work, prefer flags or the
[configuration file](/reference/configuration) — see the caveat on config-key
variables below.

The `CARGOSHIP_AGENT_*`, `CARGOSHIP_WATCH_PATHS`, `CARGOSHIP_DESTINATION`, and
`CARGOSHIP_CONTROLLER_URL` variables previously documented here were read only by
the `cargoship-launch` binary, which was **removed in v0.20.0** along with the
controller ([#340](https://github.com/scttfrdmn/cargoship/issues/340)). Setting
them now has no effect.
:::

### Distributed agents

| Variable | Purpose |
|----------|---------|
| `CARGOSHIP_TLS_INSECURE` | Disable TLS verification (development only; logs a loud warning). |
| `AWS_PROFILE` | Credential profile a ghost ship uses for S3. |

### Execution-context detection

Force the [execution context](/guides/config/contexts) instead of auto-detecting:

| Variable | Effect |
|----------|--------|
| `CARGOSHIP_AGENT_MODE` | Run as an agent. |
| `CARGOSHIP_REPL_MODE` | Run in REPL context. |

### Config-key variables (limited)

CargoShip enables viper's `AutomaticEnv` with a `CARGOSHIP_` prefix, so some
config keys can be set as environment variables (e.g. `CARGOSHIP_LOG_LEVEL`).

::: warning Nested keys don't reliably bind
There is **no env-key replacer** configured, so nested config keys such as
`upload.chunk_size` do **not** map cleanly to `CARGOSHIP_UPLOAD_CHUNK_SIZE`. Treat
only the explicitly-listed variables above as guaranteed. For anything under a
config section, use the [configuration file](/reference/configuration) or a
command-line flag instead.
:::

## See also

- [Configuration schema](/reference/configuration)
- [Config files & precedence](/guides/config/files)
- [AWS setup & credentials](/start/aws-setup)
