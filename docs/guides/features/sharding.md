# Multi-prefix sharding

CargoShip spreads a single upload across multiple S3 **prefixes** — shards — and
uploads them in parallel. This is the main reason it's faster than uploading
files one at a time: S3 scales request throughput *per prefix*, so more prefixes
means proportionally more aggregate throughput. The shard count is adaptive by
default, so most people never touch it.

For where shards land in the archive layout, see the
[archive layout spec](/reference/format/archive-layout).

## Why prefixes scale throughput

S3 limits requests per prefix, not per bucket: roughly **3,500 PUT/s and 5,500
GET/s per prefix**. A single-prefix uploader hits that ceiling and stops
scaling. By writing chunks across N prefixes, CargoShip multiplies the effective
limit:

```
aggregate PUT/s ≈ shard_count × 3,500
```

Eight shards give ~28,000 PUT/s of headroom — enough that your network, not S3,
becomes the bottleneck. Each shard is an independent S3 prefix uploaded
concurrently.

## Adaptive shard count (the default)

By default (`--shard-count 0`) CargoShip picks the count for you, balancing four
inputs:

- **File count** — more files, more shards.
- **Total compressed size** — more data, more shards.
- **Available CPU cores** — matches parallelism to the machine.
- **Available memory** — caps shards so memory stays bounded.

The result is clamped to a **4–32** range: never fewer than 4 (so even small jobs
get real parallelism) and never more than 32 (past which coordination overhead
outweighs the gain). It also targets a minimum number of chunks per shard so
shards stay balanced rather than lopsided.

```bash
# Adaptive — recommended
cargoship upload ./my-data s3://my-bucket/archives/
```

## Overriding the count

Pin a specific count when benchmarking or when you know your workload:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --shard-count 16
```

Manual values are honored within the same 4–32 range. Raise it for very large
datasets on a fast link; there's rarely a reason to lower it below the adaptive
choice.

::: tip Leave it on auto
The adaptive count is tuned to your actual file count, data size, and hardware.
Override it to benchmark or to satisfy a specific constraint — otherwise auto
almost always matches or beats a hand-picked number.
:::

## Distribution strategies

`--shard-strategy` controls *which* files go to *which* shard. The default,
`hash`, keeps shards evenly sized; the others suit specific access patterns.

| Strategy | Behavior | Use when |
|----------|----------|----------|
| `hash` (default) | Even, balanced distribution | General purpose — start here |
| `size` | Large files isolated into their own shards | Mixed file sizes; keeps big files from unbalancing shards |
| `type` | Groups files by extension / content type | You'll later download by kind (all `.log`, all `.csv`) |
| `directory` | Keeps directory trees together in a shard | You'll download or restore by subtree |

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --shard-count 16 --shard-strategy size
```

Strategy affects [selective download](/guides/downloading) too: grouping by type
or directory means retrieving a subset touches fewer chunks.

## Best practices

::: tip
- **Leave `--shard-count` on auto** unless you're benchmarking — it's tuned to your
  workload and hardware.
- **Keep `hash`** for general uploads; reach for `size` only when a few huge files
  are unbalancing shards.
- **Match strategy to retrieval**: `type` or `directory` if you'll download
  subsets by kind or subtree later.
- **Scale up (24–32) for TB-scale jobs on fast links**; the ceiling exists because
  more shards eventually add coordination overhead, not throughput.
- **If uploads stall or S3 throttles**, that's usually the network — fewer shards
  won't help; see [Performance tuning](/guides/features/optimization).
:::

## See also

- [Archive layout & shard keys](/reference/format/archive-layout) — how shards map to S3 keys.
- [Compression](/guides/features/compression) — per-chunk compression, the other throughput lever.
- [Downloading & extracting](/guides/downloading) — selective retrieval by shard.
- [Performance tuning](/guides/features/optimization) — workers, chunk size, and network knobs.
- Reference: [Uploading & sync commands](/reference/commands/upload).
