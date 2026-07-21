# Web UI

The CargoShip web UI is a browser-based dashboard for monitoring and managing
distributed [launch agents](/enterprise/launch-agent) and their archival activity
from one place — a friendlier front end than reading agent logs over SSH. It is
served by `cargoship webui`, shares the [controller](/enterprise/controller)'s
server, and is protected by a required auth token.

## Starting the web UI

An auth token is **required** — the command will not start without `--auth-token`:

```bash
cargoship webui --auth-token your-secure-token
```

On startup it logs the dashboard URL. From the dashboard you can see connected
agents, job status, and real-time throughput updates streamed over WebSocket.

### Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--addr` | `:8081` | Address to bind the web server. |
| `--auth-token` | *(required)* | Token for API access. The command errors if it is not set. |
| `--tls` | `false` | Enable TLS. |
| `--tls-cert` | | TLS certificate file (used with `--tls`). |
| `--tls-key` | | TLS private key file (used with `--tls`). |

## Security guidance

::: warning Exposes fleet management
The web UI can view and manage your entire agent fleet, so treat it as a
privileged endpoint:

- **Use a strong, unique `--auth-token`** — it is the only thing standing between
  the internet and your fleet controls. Never reuse a token you have logged or
  committed.
- **Enable `--tls`** with a real certificate whenever the UI is reachable beyond
  localhost, so the token and dashboard traffic are encrypted (`https://` rather
  than `http://`).
- **Restrict exposure** — bind to a private interface or put it behind a VPN /
  reverse proxy rather than publishing `:8081` to the open internet.
:::

## See also

- [Controller](/enterprise/controller).
- [Distributed / Enterprise overview](/enterprise/).
- [Launch agents](/enterprise/launch-agent).
