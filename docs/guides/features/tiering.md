# Tier-aware storage

Tier-aware storage lets CargoShip assign an S3 storage class per chunk based on
how recently the files in it were accessed, instead of writing everything to one
class. Cold data lands in cheaper, slower tiers (GLACIER, DEEP_ARCHIVE) while hot
data stays instantly readable — cutting storage cost on archives that are rarely
touched.

Enable it with `--auto-tier` and choose a strategy:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --auto-tier --tier-strategy tier-aware --tier-max GLACIER
```

- `--tier-strategy` — `youngest-file` (conservative, tiers by the youngest file in
  a chunk, default) or `tier-aware` (groups files by tier before chunking for the
  best savings).
- `--tier-max` — cap how cold anything goes (`STANDARD`, `STANDARD_IA`, `GLACIER`,
  `DEEP_ARCHIVE`).
- `--tier-hot-days` / `--tier-cold-days` / `--tier-archive-days` — the access-age
  thresholds for hot (STANDARD), cold (GLACIER), and archive (DEEP_ARCHIVE).

::: danger Cost implication
`--tier-strategy tier-aware` prompts for confirmation because GLACIER and
DEEP_ARCHIVE carry minimum-storage-duration commitments (90 and 180 days),
per-GB retrieval charges, and slow thaw times — early deletion incurs penalties.
Pass `--yes` to accept in automation, and set `--tier-max` to bound the risk.
:::

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Uploading & sync command reference](/reference/commands/upload).
:::

## See also

- [Restoring files: Glacier & Deep Archive](/guides/restoring#glacier).
- [Lifecycle & storage classes](/guides/cost/lifecycle).
- [Concepts: storage class & tier](/intro/concepts#storage-class-tier).
- Reference: [Uploading & sync commands](/reference/commands/upload).
