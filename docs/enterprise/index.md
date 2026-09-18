# Distributed / Enterprise

Most of CargoShip is a single command you run on one machine. Distributed mode is
for a different problem: archiving research data that lives on NAS boxes and file
servers around a lab, without pulling it all back to one host first.

Today that means a **ghostship fleet**: unattended, write-only agents deployed to
remote boxes (NAS, lab servers, workstations) that back up their own directories to S3
on a schedule, straight from the box with no data round-trip. Each agent is isolated to
its own `writers/<id>/` prefix, pulls a **signed** config from S3, and has a
**write-only, delete-free** IAM identity — a compromised agent can only append to its
own subtree. You watch the whole fleet from a control machine with `cargoship fleet
status`/`monitor` and the dashboard's 🚢 Fleet tab. See
[ghost-ship](/enterprise/ghost-ship) and the [fleet tutorial](/enterprise/fleet-tutorial).

::: warning Central coordination was removed in v0.20.0
Earlier versions also shipped a central controller, a `cargoship-launch` agent
binary, and a `cargoship webui` dashboard for managing a fleet from one place.
That subsystem was never finished — most of its request handlers were empty — and
a security audit found an authentication bypass in it, so it was removed rather
than hardened. See [issue #340](https://github.com/scttfrdmn/cargoship/issues/340).

If you used `cargoship controller`, `cargoship webui`, or `cargoship-launch`,
they no longer exist. Ghost ships are unaffected: they were built to archive
autonomously and never required a controller to function.
:::

## When you need it

Reach for a ghostship fleet when data sits on boxes you want backing themselves up on a
schedule, unattended, without a central host in the data path — especially many boxes
sharing one bucket. For a one-off or scripted upload from a single host, plain
[`cargoship upload`](/guides/uploading) is all you need.

## See also

- [ghost-ship](/enterprise/ghost-ship) — the fleet agent: config-over-S3 trust, write-only IAM, observability.
- [Fleet tutorial](/enterprise/fleet-tutorial) — end-to-end: provision a writer → deploy → watch → recover.
- [QNAP / NAS deployment](/enterprise/qnap) — step-by-step deployment.
- [Deployment guide](/enterprise/deployment) — production CargoShip on a server.
- [Execution contexts](/guides/config/contexts).
