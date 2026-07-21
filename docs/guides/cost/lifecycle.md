# Lifecycle & storage classes

`cargoship lifecycle` manages S3 lifecycle policies so objects transition to
cheaper storage classes automatically over time. Rather than hand-writing JSON,
you apply one of CargoShip's predefined policy templates tuned for common
archival patterns, or import and export your own. The full flag list lives in the
[command reference](/reference/commands/cost).

```bash
cargoship lifecycle --bucket BUCKET --template TEMPLATE_ID
```

## Predefined templates

List the built-in templates and what each one does:

```bash
cargoship lifecycle --list-templates
```

CargoShip ships four templates, each targeting a prefix under `archives/`:

| Template | Transitions | Expiration | Est. monthly savings |
|----------|-------------|------------|----------------------|
| `archive-optimization` | Standard-IA at 30d → Glacier at 90d → Deep Archive at 365d | none | ~60% |
| `intelligent-tiering` | Intelligent-Tiering immediately (day 0) | none | ~25% |
| `compliance-retention` | Standard-IA at 30d → Glacier at 90d → Deep Archive at 365d | deletes at 2,555d (7 years) | ~65% |
| `fast-access` | Standard-IA at 90d → Glacier at 365d | none | ~30% |

- **archive-optimization** — aggressive cost reduction for long-term archival
  data you rarely read (prefix `archives/`).
- **intelligent-tiering** — hands-off; S3 moves objects between access tiers
  automatically based on usage (prefix `archives/`).
- **compliance-retention** — the archive-optimization ladder plus a hard 7-year
  expiration for regulatory retention (prefix `archives/compliance/`).
- **fast-access** — keeps data in warmer classes longer for datasets that still
  need quick retrieval (prefix `archives/frequent/`).

All four also abort incomplete multipart uploads (after 1–7 days depending on the
template) to stop orphaned parts from accruing charges.

## Applying a policy

```bash
cargoship lifecycle --bucket my-bucket --template archive-optimization
```

```
📋 Applying lifecycle policy: Archive Optimization
   Aggressive cost optimization for long-term archival data
✅ Lifecycle policy applied successfully!
```

## Estimating savings first

Add `--estimate-size` (data size in GB) to see the projected monthly and annual
savings before or when you apply a template:

```bash
cargoship lifecycle --bucket my-bucket --template intelligent-tiering \
  --estimate-size 100
```

```
💰 Savings Estimate (100.0 GB):
   Current monthly cost: $2.30
   Optimized monthly cost: $1.73
   Monthly savings: $0.57 (24.8%)
   Annual savings: $6.84
```

## Exporting, importing & removing

Review or version-control the current policy, apply a policy from a file, or
remove lifecycle management entirely:

```bash
# Export the bucket's current policy to a file
cargoship lifecycle --bucket my-bucket --export policy.json

# Apply a policy from a file (validated on import)
cargoship lifecycle --bucket my-bucket --import policy.json

# Remove the lifecycle policy
cargoship lifecycle --bucket my-bucket --remove
```

`--region` selects the region (default `us-east-1`), and `--profile` picks an AWS
credentials profile.

::: warning --remove clears all lifecycle rules
`--remove` deletes the bucket's entire lifecycle configuration, so objects stop
transitioning and any expiration rules stop firing. Export first if you might
want the policy back.
:::

## Lifecycle vs. upload-time storage class

Lifecycle policies are the "set it and forget it" complement to choosing storage
at upload time:

- **`upload --storage-class`** picks the class objects land in immediately.
- **[Tier-aware storage](/guides/features/tiering)** assigns classes per chunk by
  file age at upload time.
- **`lifecycle`** transitions objects between classes automatically as they age,
  regardless of how they were uploaded.

Common classes: `STANDARD`, `INTELLIGENT_TIERING`, `STANDARD_IA`, `GLACIER` /
`GLACIER_IR`, `DEEP_ARCHIVE`.

## Best practices

::: tip
- **Estimate before you apply** with `--estimate-size` so the savings match the
  retrieval trade-off you're accepting.
- **Match the template to access frequency** — `fast-access` for hot data,
  `archive-optimization` for cold, `compliance-retention` when you must delete on
  a schedule.
- **Export policies into version control** so lifecycle rules are reviewable and
  reproducible.
- **Remember retrieval cost** — colder classes cut storage cost but make
  [restores](/guides/restoring#glacier) slower and pricier.
:::

## See also

- [Tier-aware storage](/guides/features/tiering) — assign classes per chunk by file age.
- [Analyzing existing S3 spend](/guides/cost/analyze).
- [Concepts: storage class & tier](/intro/concepts#storage-class-tier).
- Reference: [Cost, budget & alerts](/reference/commands/cost).
