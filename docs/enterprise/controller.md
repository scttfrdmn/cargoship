# Controller

The controller is the central coordination hub for a fleet of CargoShip
[launch agents](/enterprise/launch-agent) and [ghost ships](/enterprise/ghost-ship).
Agents connect to it over an authenticated WebSocket; the controller tracks their
health, assigns archival jobs, and gives you a single place to monitor distributed
operations. Agents keep archiving autonomously even when the controller is briefly
unreachable.

## Running a controller

```bash
# Listen on the default :8080 with an auto-generated auth token
cargoship controller

# Pin a port and a known token
cargoship controller --listen :8080 --auth-token your-secure-token

# Run with TLS (recommended for anything beyond localhost)
cargoship controller --tls --cert-file server.crt --key-file server.key
```

On startup the controller prints its connection URL and auth token, plus the
environment variables to configure agents with.

### Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--listen` | `:8080` | Address to listen on for agent connections. |
| `--auth-token` | *(auto-generated)* | Token agents must present. If omitted, a secure random token is generated and logged at startup. |
| `--tls` | `false` | Enable TLS for encrypted connections. |
| `--cert-file` | | TLS certificate file (**required** with `--tls`). |
| `--key-file` | | TLS private key file (**required** with `--tls`). |
| `--log-level` | `info` | Log verbosity: `debug`, `info`, `warn`, `error`. |

::: tip Secure the connection
Set an explicit `--auth-token` (rather than relying on the generated one) and
enable `--tls` with a real certificate whenever the controller is reachable beyond
localhost. Without TLS the agent WebSocket is unencrypted; enabling TLS upgrades it
to a secure (TLS-wrapped) WebSocket.
:::

## Connecting agents

Point each agent at the controller and give it the matching token via environment
variables:

```bash
export CARGOSHIP_CONTROLLER_URL=wss://controller.example.com:8080
export CARGOSHIP_AUTH_TOKEN=your-secure-token
```

On the CargoShip side, the `controller`
[execution context](/guides/config/contexts) enables controller operations and
agent coordination.

## Management API

The controller exposes a small HTTP API alongside the WebSocket server:

- `GET /api/v1/agents` — list connected agents.
- `GET /api/v1/agents/{id}` — details for one agent.
- `GET /health` — health check.

The [Web UI](/enterprise/webui) is a browser front end over this same controller.

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [Launch agents](/enterprise/launch-agent).
- [Web UI](/enterprise/webui).
- [Execution contexts](/guides/config/contexts).
