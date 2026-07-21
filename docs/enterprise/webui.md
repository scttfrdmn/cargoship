# Web UI

The CargoShip web UI is a browser-based dashboard for monitoring and managing
distributed [launch agents](/enterprise/launch-agent) and their archival activity
from one place — a friendlier front end than reading agent logs over SSH. It's
served by `cargoship webui` and protected by an auth token.

```bash
cargoship webui --auth-token your-secure-token
```

Point the UI at a running [controller](/enterprise/controller) to see connected
agents, job status, and throughput. Always run it behind TLS with a strong token,
since it exposes fleet management.

::: warning Draft
This page is being expanded with the full flag list, deployment notes, and
security guidance.
:::

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [Controller](/enterprise/controller).
- [Launch agents](/enterprise/launch-agent).
