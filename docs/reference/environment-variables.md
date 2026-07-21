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

::: warning Most CARGOSHIP_* variables are for agents, not everyday uploads
The variables CargoShip reads explicitly are used almost entirely by the
distributed **launch agent** and **controller** (see
[Distributed / Enterprise](/enterprise/)). For normal `cargoship upload` work,
prefer flags or the [configuration file](/reference/configuration) — see the
caveat on config-key variables below.
:::

### Agent & controller

Read by the launch-agent binary and controller wiring:

| Variable | Purpose |
|----------|---------|
| `CARGOSHIP_AGENT_ID` | Agent identity. |
| `CARGOSHIP_AGENT_NAME` | Agent display name. |
| `CARGOSHIP_AGENT_DESCRIPTION` | Agent description. |
| `CARGOSHIP_CONTROLLER_URL` | Controller endpoint the agent connects to. |
| `CARGOSHIP_AUTH_TOKEN` | Auth token for agent ↔ controller / web UI. |
| `CARGOSHIP_WATCH_PATHS` | Paths the agent watches for new data. |
| `CARGOSHIP_PATTERNS` | Include patterns for watched files. |
| `CARGOSHIP_EXCLUDE_PATTERNS` | Exclude patterns for watched files. |
| `CARGOSHIP_CHECK_INTERVAL` | Agent poll interval. |
| `CARGOSHIP_MIN_AGE_DAYS` | Minimum file age before an agent archives it. |
| `CARGOSHIP_DESTINATION` | Default S3 destination for the agent. |
| `CARGOSHIP_STORAGE_CLASS` | Default storage class for the agent. |
| `CARGOSHIP_COMPRESSION` | Compression setting for the agent. |
| `CARGOSHIP_TLS_INSECURE` | Disable TLS verification (development only; logs a loud warning). |

### Execution-context detection

Force the [execution context](/guides/config/contexts) instead of auto-detecting:

| Variable | Effect |
|----------|--------|
| `CARGOSHIP_AGENT_MODE` | Run as an agent. |
| `CARGOSHIP_CONTROLLER_MODE` | Run as a controller. |
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
