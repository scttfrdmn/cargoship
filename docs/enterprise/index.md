# Distributed / Enterprise

Most of CargoShip is a single command you run on one machine. The distributed
mode is for a different problem: archiving research data that lives on NAS boxes,
file servers, and compute nodes across a lab or department, without pulling it all
back to one host first. It layers a few coordinated pieces on top of the same
upload engine.

## The pieces

- **Launch agents** — lightweight, containerized CargoShip agents that run on your
  own infrastructure, watch directories, and archive completed datasets directly to
  S3. They operate headlessly and connect to a controller over TLS. See
  [Launch agents](/enterprise/launch-agent).

- **Ghost ships** — autonomous agents deployed to remote NAS devices (QNAP,
  Synology) that continuously monitor local files and upload them straight to the
  cloud — no data round-trip — while staying coordinated centrally. See
  [ghost-ship](/enterprise/ghost-ship).

- **Controller** — the central coordination hub. Agents connect to it over an
  authenticated WebSocket; it tracks health, assigns jobs, and gives you one place
  to monitor the fleet. See [Controller](/enterprise/controller).

- **Web UI** — a browser dashboard for monitoring and managing agents, served by
  `cargoship webui`. See [Web UI](/enterprise/webui).

## When you need it

Reach for distributed mode when data is spread across multiple machines and you
want continuous, rule-based archival with central visibility. For a one-off or
scripted upload from a single host, plain [`cargoship upload`](/guides/uploading)
is all you need.

## Getting started

Agents connect to a controller with two environment variables:

```bash
CARGOSHIP_CONTROLLER_URL=wss://controller.example.com:8080
CARGOSHIP_AUTH_TOKEN=your-secure-token
CARGOSHIP_DESTINATION=s3://research-archive
```

Communication is TLS-encrypted (WSS) and token-authenticated; agents keep
archiving autonomously even when the controller is temporarily offline.

## See also

- [Launch agents](/enterprise/launch-agent).
- [ghost-ship](/enterprise/ghost-ship).
- [Controller](/enterprise/controller) · [Web UI](/enterprise/webui).
- [Deployment guide](/enterprise/deployment) · [QNAP / NAS deployment](/enterprise/qnap).
- [Execution contexts](/guides/config/contexts).
