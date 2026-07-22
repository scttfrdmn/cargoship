# Command reference

Every user-facing `cargoship` command, grouped by task. The flag tables on these
pages are **generated directly from the CLI**, so they always match the version
you have installed.

::: tip Generated reference
The synopsis, examples, and flag list for each command are produced by
`cargoship mddocs` from the live command tree and vendored into the docs. A CI
drift check fails if they fall out of sync, so what you read here is what the
binary actually accepts.
:::

## By task

| Page | Commands |
|------|----------|
| [Uploading & sync](/reference/commands/upload) | `upload`, `create upload`, `sync`, `migrate`, `resume` |
| [Inspection & retrieval](/reference/commands/inspect) | `list`, `info`, `verify`, `balance`, `download`, `restore`, `browse`, `shell` |
| [Cost, budget & alerts](/reference/commands/cost) | `estimate`, `analyze`, `cost …`, `budget …`, `alerts …`, `lifecycle` |
| [DVC](/reference/commands/dvc) | `dvc stages`, `dvc status`, `dvc export` |
| [Configuration & context](/reference/commands/config) | `config`, `setup`, `context …` |
| [Destructive operations](/reference/commands/destructive) | `delete`, `scuttle` |
| [Diagnostics & utilities](/reference/commands/diagnostics) | `benchmark`, `dashboard`, `profile …`, `create keys`, `metrics` |
| [Global flags](/reference/commands/global-flags) | flags available on every command |

## Other references

- [Environment variables](/reference/environment-variables)
- [Configuration schema](/reference/configuration)
- [CargoShip vs. other tools](/reference/comparison)
- [Recovery & operations runbook](/reference/recovery)
- [Troubleshooting](/reference/troubleshooting) · [FAQ](/reference/faq)
- [Glossary](/reference/glossary) · [Cheat sheet](/reference/cheatsheet)

Looking for the on-disk format instead of commands? See the
[Archive & Manifest Format Spec](/reference/format/).
