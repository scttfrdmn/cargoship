# Lifecycle & storage classes

`cargoship lifecycle` manages S3 lifecycle policies so objects transition to
cheaper storage classes automatically over time. Rather than hand-writing JSON,
you apply one of CargoShip's predefined policy templates tuned for common archival
patterns, or import/export your own.

```bash
cargoship lifecycle --list-templates
cargoship lifecycle --bucket my-bucket --template archive-optimization
```

Estimate what a policy would save before applying it, or export the current policy
for review:

```bash
cargoship lifecycle --bucket my-bucket --template intelligent-tiering --estimate-size 100
cargoship lifecycle --bucket my-bucket --export policy.json
```

Lifecycle policies are the "set it and forget it" complement to picking a storage
class at upload time (`--storage-class`) or per-chunk with
[tier-aware storage](/guides/features/tiering). Common classes: `STANDARD`,
`INTELLIGENT_TIERING`, `STANDARD_IA`, `GLACIER` / `GLACIER_IR`, `DEEP_ARCHIVE`.

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Cost, budget & alerts command reference](/reference/commands/cost).
:::

## See also

- [Tier-aware storage](/guides/features/tiering) — assign classes per chunk by file age.
- [Analyzing existing S3 spend](/guides/cost/analyze).
- [Concepts: storage class & tier](/intro/concepts#storage-class-tier).
- Reference: [Cost, budget & alerts](/reference/commands/cost).
