# Distributed / Enterprise

Most of CargoShip is a single command you run on one machine. Distributed mode is
for a different problem: archiving research data that lives on NAS boxes and file
servers around a lab, without pulling it all back to one host first.

Today that means one thing — **ghost ships**: autonomous agents deployed to a
remote NAS (QNAP, Synology) that continuously monitor local paths, apply archival
rules, and upload straight to S3, with no data round-trip. Each runs
independently. See [ghost-ship](/enterprise/ghost-ship).

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

Reach for a ghost ship when data sits on a NAS that you want archiving itself,
continuously, on rules you set once. For a one-off or scripted upload from a
single host, plain [`cargoship upload`](/guides/uploading) is all you need.

## See also

- [ghost-ship](/enterprise/ghost-ship) — configuration and archival rules.
- [QNAP / NAS deployment](/enterprise/qnap) — step-by-step deployment.
- [Deployment guide](/enterprise/deployment) — production CargoShip on a server.
- [Execution contexts](/guides/config/contexts).
