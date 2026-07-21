# Environment variables

CargoShip reads two families of environment variables: its own `CARGOSHIP_*`
settings, and the standard `AWS_*` variables used by the AWS SDK for credentials
and region.

::: info Precedence
Settings are resolved highest-priority first: **command-line flags → environment
variables (`CARGOSHIP_*`) → configuration file → built-in defaults.** A flag always
wins over an env var, which always wins over the config file. See
[Config files & precedence](/guides/config/files).
:::

## AWS variables

CargoShip uses your standard AWS credential chain — the same one the AWS CLI uses.

| Variable | Purpose |
|----------|---------|
| `AWS_REGION` | Default region for S3 operations (override per command with `--region`). |
| `AWS_PROFILE` | Named profile from `~/.aws/config` / `~/.aws/credentials`. |
| `AWS_ACCESS_KEY_ID` | Access key (when not using a profile or role). |
| `AWS_SECRET_ACCESS_KEY` | Secret key. |
| `AWS_SESSION_TOKEN` | Session token for temporary credentials. |

See [AWS setup & credentials](/start/aws-setup) for the minimal IAM policy.

## CargoShip variables

Common `CARGOSHIP_*` variables include:

| Variable | Purpose |
|----------|---------|
| `CARGOSHIP_DESTINATION` | Default S3 destination. |
| `CARGOSHIP_STORAGE_CLASS` | Default S3 storage class. |
| `CARGOSHIP_COMPRESSION` | Default compression level. |
| `CARGOSHIP_LOG_LEVEL` | Log verbosity. |
| `CARGOSHIP_AUTH_TOKEN` | Auth token for the web UI / distributed mode. |
| `CARGOSHIP_CONTROLLER_URL` | Controller endpoint for agents. |

::: warning Draft
This page is being expanded into a complete, categorized table. For the current
list and the exact settings each maps to, run `cargoship config --show` and see
the [Configuration schema](/reference/configuration).
:::

## See also

- [Configuration schema](/reference/configuration).
- [Config files & precedence](/guides/config/files).
- [AWS setup & credentials](/start/aws-setup).
