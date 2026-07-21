# Tier-aware storage

Tier-aware storage lets CargoShip assign an S3 storage class per chunk based on
how recently the files in it were accessed, instead of writing everything to one
class. Cold data lands in cheaper, slower tiers (GLACIER, DEEP_ARCHIVE) while hot
data stays instantly readable — cutting storage cost on archives that are rarely
touched.

Enable it with `--auto-tier` on `cargoship upload` and choose a strategy:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --auto-tier --tier-strategy tier-aware --tier-max GLACIER
```

## Strategies

`--tier-strategy` controls how files are grouped into chunks before a class is
assigned (it requires `--auto-tier`):

| Strategy | Behavior | Use when |
|----------|----------|----------|
| `youngest-file` (default) | Conservative — a chunk takes the class implied by its *youngest* file, so nothing is tiered colder than any recent file it contains | You want savings without risking that a recently touched file lands in Glacier |
| `tier-aware` | Groups files by tier *before* chunking, so cold files cluster into cold chunks for the best savings | You have a clear hot/cold split and want maximum cost reduction |

## Age thresholds

The tier a chunk lands in is decided by how many days since a file was last
accessed:

- `--tier-hot-days` — up to this age stays hot / `STANDARD` (default `30`).
- `--tier-cold-days` — beyond this becomes cold / `GLACIER` (default `90`).
- `--tier-archive-days` — beyond this becomes archive / `DEEP_ARCHIVE` (default `180`).

Use `--tier-max` to cap how cold anything can go regardless of age: `STANDARD`,
`STANDARD_IA`, `GLACIER`, or `DEEP_ARCHIVE`. Setting `--tier-max STANDARD_IA`, for
example, prevents any chunk from being written to Glacier or Deep Archive.

## Cost confirmation

::: danger Cost implication
`--tier-strategy tier-aware` prompts for confirmation before it runs because
GLACIER and DEEP_ARCHIVE carry commitments that are easy to overlook:

- **GLACIER** — 90-day minimum storage, per-GB retrieval charges, multi-hour
  restore times.
- **DEEP_ARCHIVE** — 180-day minimum storage, higher per-GB retrieval, up to
  ~12-hour restore.
- **Early deletion penalties** apply if an object is removed before its minimum
  duration.

Best for long-term archives accessed less than once a year. Pass `--yes` to
accept the prompt in automation, and set `--tier-max` to bound how cold anything
can go.
:::

## See also

- [Multi-prefix sharding](/guides/features/sharding) — how chunks map to shards.
- [Restoring files: Glacier & Deep Archive](/guides/restoring#glacier).
- [Lifecycle & storage classes](/guides/cost/lifecycle).
- [Concepts: storage class & tier](/intro/concepts#storage-class-tier).
- Reference: [Compression format](/reference/format/compression).
- Reference: [Uploading & sync commands](/reference/commands/upload).
