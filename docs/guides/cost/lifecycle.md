# Lifecycle & storage classes

`cargoship lifecycle` manages S3 lifecycle policies so objects transition to
cheaper storage classes automatically over time. Rather than hand-writing JSON,
you apply one of CargoShip's predefined policy templates tuned for common
archival patterns, or import and export your own. The full flag list lives in the
[command reference](/reference/commands/cost).

```bash
cargoship lifecycle --bucket BUCKET --prefix PREFIX --template TEMPLATE_ID
```

## Predefined templates

List the built-in templates and what each one does:

```bash
cargoship lifecycle --list-templates
```

CargoShip ships four templates. Each one applies to the prefix you upload to,
which you pass with `--prefix`:

| Template | Chunk transitions | Expiration | Est. monthly savings |
|----------|-------------------|------------|----------------------|
| `archive-optimization` | Standard-IA at 30d → Glacier at 90d → Deep Archive at 365d | none | ~60% |
| `intelligent-tiering` | Intelligent-Tiering immediately (day 0) | none | ~25% |
| `compliance-retention` | Standard-IA at 30d → Glacier at 90d → Deep Archive at 365d | **deletes every object under the prefix at 2,555d (7 years)** | ~65% |
| `fast-access` | Standard-IA at 90d → Glacier at 365d | none | ~30% |

- **archive-optimization**: aggressive cost reduction for long-term archival
  data you rarely read.
- **intelligent-tiering**: hands-off. S3 moves objects between access tiers
  automatically based on usage.
- **compliance-retention**: the archive-optimization ladder plus a hard 7-year
  expiration for regulatory retention.
- **fast-access**: keeps data in warmer classes longer for datasets that still
  need quick retrieval.

All four also abort incomplete multipart uploads under the prefix (after 1–7
days depending on the template), so orphaned parts don't keep accruing charges.

### What moves, and what stays

Only archive **chunks** are transitioned. A transition rule matches objects
under the prefix that carry the object tag `cargoship-object=chunk`.

**Manifests** (`uploads/<id>/manifest.json.gz`, plus the encrypted and partial
variants) never carry that tag, so they stay in the storage class they were
uploaded in. `sync`, `list`, `info`, `verify` and `restore` all read the
manifest first. If it were in Glacier or Deep Archive, each of those commands
would need a thaw before it could start.

S3 lifecycle filters can match a prefix, tags, or an object-size range, but not
a file-name suffix. Manifests live under the same prefix as their chunks, so a
prefix-only rule can't tell them apart. A size threshold doesn't work either: a
manifest for millions of files can be larger than a small dataset's last chunk.
Tagging the chunks is the only filter that is exact.

::: warning Untagged chunks don't transition
Chunks without the `cargoship-object=chunk` tag, such as ones uploaded by
earlier CargoShip versions, are left in their storage class. To include them,
add the tag to those chunk objects (keys ending in `.tar` or `.tar.zst` under
`uploads/<id>/shard-*/`). Don't tag manifests.
:::

### Expiration (compliance-retention)

`compliance-retention` **deletes** every object under the prefix, chunks and
manifests alike, 2,555 days (about 7 years) after each object was written. An
upload's manifests expire together with its chunks, so no upload is left
half-deleted. Because the template deletes data, it needs a non-empty
`--prefix` and won't apply to a whole bucket.

## Applying a policy

```bash
# Uploads to s3://my-bucket/backup
cargoship lifecycle --bucket my-bucket --prefix backup --template archive-optimization
```

`--prefix` is required with `--template`. Pass `--prefix ""` to cover the whole
bucket (not allowed for `compliance-retention`).

```
📋 Applying lifecycle policy: Archive Optimization
   Aggressive cost optimization for long-term archival data
   Scope: s3://my-bucket/backup/ (transitions apply to objects tagged cargoship-object=chunk; manifests stay put)
✅ Lifecycle policy applied successfully!
```

## Estimating savings first

Add `--estimate-size` (data size in GB) to see the projected monthly and annual
savings before or when you apply a template:

```bash
cargoship lifecycle --bucket my-bucket --prefix backup --template intelligent-tiering \
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

::: warning --remove clears CargoShip's rules
`--remove` deletes every lifecycle rule whose ID starts with `cargoship-`, so
CargoShip's transitions and expirations stop firing. Rules with other IDs stay
in place. Applying a template likewise replaces CargoShip's existing rules and
keeps everyone else's, so a bucket carries one CargoShip template, for one
prefix, at a time.
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
