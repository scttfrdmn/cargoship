# Multi-region

CargoShip can distribute uploads across multiple AWS regions with load balancing,
health checking, and automatic failover. When a region's endpoint degrades or
becomes unreachable, traffic shifts to healthy regions so an upload keeps making
progress instead of stalling — useful for geographically distributed teams and for
resilience against a single-region disruption.

::: warning Draft
This page is being expanded with configuration and endpoint-selection details.
For per-command flags, see the [command reference](/reference/commands/upload); for
region defaults, see [Environment variables](/reference/environment-variables)
(`AWS_REGION`).
:::

## See also

- [Multi-prefix sharding](/guides/features/sharding) — parallelism within a region.
- [Performance tuning](/guides/features/optimization).
- Reference: [Environment variables](/reference/environment-variables).
