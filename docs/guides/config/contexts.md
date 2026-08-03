# Execution contexts

A CargoShip **context** determines which commands are available and how the tool
operates. Most users stay in `local` and never think about it; contexts matter
when you run CargoShip as part of the [distributed setup](/enterprise/). The
current context is cached in `~/.cargoship-context` and persists between
sessions.

## The three contexts

| Context | Purpose |
|---------|---------|
| `local` | Local filesystem operations and archive creation (the default). |
| `agent` | Distributed-agent monitoring and management. |
| `repl` | Interactive shell mode with command discovery. |

The `controller` context was removed in v0.20.0 along with the controller itself
([#340](https://github.com/scttfrdmn/cargoship/issues/340)). If it is still
cached in your `~/.cargoship-context`, run `cargoship context reset`.

## Managing the context

```bash
cargoship context                    # show current context
cargoship context list               # list available contexts
cargoship context switch agent       # switch and cache a new context
cargoship context show               # show current context with details
cargoship context reset              # back to the default (local)
```

`context switch` writes the choice to `~/.cargoship-context`, so it sticks across
sessions until you switch again or `reset`.

## Overriding for a single command

Use the global `--context` flag to override the cached context for one invocation
without changing the stored default:

```bash
cargoship --context agent <command>
```

## Automatic detection

When no context is cached, CargoShip infers one from environment variables:

| Variable | Resulting context |
|----------|-------------------|
| `CARGOSHIP_AGENT_MODE` set | `agent` |
| `CARGOSHIP_REPL_MODE` set | `repl` |
| *(none set)* | `local` |

This makes contexts work naturally inside agent deployments, where those
variables are already present. See
[Environment variables](/reference/environment-variables) for the full list.

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [ghost-ship](/enterprise/ghost-ship).
- Reference: [Configuration & context commands](/reference/commands/config).
- Reference: [Environment variables](/reference/environment-variables).
