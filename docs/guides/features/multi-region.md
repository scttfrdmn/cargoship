# Multi-region

CargoShip ships an internal `multiregion` library for distributing work across
several AWS regions with load balancing, health checking, and failover. This page
describes what that library provides and, importantly, how it is and isn't wired
into the CLI today.

::: info Status
The `multiregion` package is a **library capability, not a `cargoship upload`
workflow**. The `upload` command targets a single region via `--region` (or
`AWS_REGION`); it does not distribute a single upload across regions or fail over
between them. The multiregion library is currently exercised only by CargoShip's
internal performance/benchmark tooling. If you need cross-region resilience today,
run separate uploads per region and rely on
[Multi-prefix sharding](/guides/features/sharding) for parallelism within a region.
:::

## What the library provides

The `multiregion` package implements the building blocks for region-aware
transfers:

- **Coordinator** — orchestrates region selection and dispatch across a set of
  configured regions.
- **Region selector** — chooses a target region from candidates.
- **Load balancer** — distributes traffic using strategies such as round-robin,
  weighted, latency-based, geographic, adaptive, and least-connections.
- **Health checks** — periodically probe each region and track latency and
  success so unhealthy regions can be avoided.
- **Failover** — immediate, graceful, or manual strategies for shifting away from
  a degraded region.

These pieces exist and are tested, but they are not surfaced as upload flags.

## Working with regions today

Set the region for an upload explicitly, or let it come from the environment:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --region us-east-1
```

```bash
export AWS_REGION=us-east-1
cargoship upload ./my-data s3://my-bucket/archives/
```

For throughput, the lever that is fully wired into `upload` is multi-prefix
sharding, which parallelizes a single upload across many S3 prefixes in one
region.

## See also

- [Multi-prefix sharding](/guides/features/sharding) — in-region parallelism (the throughput lever today).
- [Performance tuning](/guides/features/optimization).
- Reference: [Environment variables](/reference/environment-variables) (`AWS_REGION`).
- Reference: [Uploading & sync commands](/reference/commands/upload) (`--region`).
