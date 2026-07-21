# Controller

The controller is the central coordination hub for a fleet of CargoShip
[launch agents](/enterprise/launch-agent) and [ghost ships](/enterprise/ghost-ship).
Agents connect to it over an authenticated, TLS-encrypted WebSocket; the
controller tracks their health, assigns archival jobs, and gives you a single
place to monitor distributed operations. Agents keep archiving autonomously even
when the controller is briefly unreachable.

Agents point at a controller with `CARGOSHIP_CONTROLLER_URL` and authenticate with
`CARGOSHIP_AUTH_TOKEN`:

```bash
CARGOSHIP_CONTROLLER_URL=wss://controller.example.com:8080
CARGOSHIP_AUTH_TOKEN=your-secure-token
```

On the CargoShip side, the `controller` [execution context](/guides/config/contexts)
enables controller operations and agent coordination.

::: warning Draft
This page is being expanded with controller setup, configuration, and the
management API. In the meantime see the pages below.
:::

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [Launch agents](/enterprise/launch-agent).
- [Web UI](/enterprise/webui).
- [Execution contexts](/guides/config/contexts).
