# Execution contexts

A CargoShip **context** determines which commands are available and how the tool
operates. Most users stay in `local` and never think about it; contexts matter
when you run CargoShip as part of the distributed / enterprise setup (launch
agents and a controller). The current context is cached in `~/.cargoship-context`
and persists between sessions.

The four contexts:

- `local` — local filesystem operations and archive creation (the default).
- `agent` — launch-agent monitoring and management.
- `controller` — controller operations and agent coordination.
- `repl` — interactive shell mode with command discovery.

```bash
cargoship context                    # show current context
cargoship context list               # list available contexts
cargoship context switch controller  # switch context
cargoship context show               # show context with details
cargoship context reset              # back to default (local)
```

You can also override the context for a single command with the global
`--context` flag (e.g. `--context controller`) without changing the cached
default.

::: warning Draft
This page is being expanded. For the complete command list, see the
[Configuration & context command reference](/reference/commands/config).
:::

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [Launch agents](/enterprise/launch-agent).
- [Controller](/enterprise/controller).
- Reference: [Configuration & context commands](/reference/commands/config).
