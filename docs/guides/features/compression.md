# Compression & content-aware levels

CargoShip compresses every chunk with [Zstandard](https://facebook.github.io/zstd/)
(zstd) as it streams to S3 — no separate compression pass, no temp files.
Compression is content-aware: CargoShip recognizes what a file *is* and picks a
level that's worth the CPU, so it doesn't waste cycles re-compressing data that's
already compressed. Smaller objects mean lower storage cost and faster uploads.

The on-disk details (frame format, how levels are recorded in the manifest) live
in the [compression format spec](/reference/format/compression); this guide is
about using it.

## How it works

Each `tar.zst` chunk is compressed independently as it's built, which is what
lets uploads stream with [bounded memory](/intro/how-it-works). The compression
ratio depends entirely on content: text and logs compress dramatically, while
media and archives barely move.

| Content | Typical ratio | Notes |
|---------|---------------|-------|
| Text, logs, source code | 3–5:1 | Highly compressible |
| Structured data (JSON, CSV) | 2.5–3:1 | Good |
| Images (JPEG, PNG) | ~1.1:1 | Already compressed |
| Video, audio | ~1:1 | Already compressed |
| Encrypted / random | ~1:1 | Incompressible |

## Content-aware level selection

Rather than apply one blunt level to everything, CargoShip detects each file's
content type and compresses accordingly — hard on compressible content, light or
not at all on content that won't shrink. That detection is sharpened when
[Magika AI file detection](/guides/features/magika) is enabled, which identifies
content types that file extensions miss (code in `.bin` files, misnamed or
extensionless files).

The practical upshot: **leave levels alone and CargoShip already avoids wasting
CPU on incompressible data**. The `--compression-level` flag below sets the
ceiling for content that *is* worth compressing.

## Choosing a level

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --compression-level 9
```

`--compression-level` accepts zstd levels 1–22 (1–19 recommended; the default is
3, tuned for fast, streaming-friendly throughput). Higher levels compress more
but cost more CPU, and past a point the extra compression is slower to produce
than the bandwidth it saves.

| Level | Speed | Ratio | Good for |
|-------|-------|-------|----------|
| 1–3 | Fastest | ~2–2.5:1 | Network-bound uploads, mostly-compressed data (default: 3) |
| 6–9 | Fast | ~3–4:1 | Balanced general use |
| 12–15 | Slower | ~4–5:1 | Text-heavy data, CPU to spare |
| 16–19 | Slowest | ~5:1+ | Cold archives you write once |

::: tip Total time, not just ratio
The fastest level isn't always the fastest *upload*. On a slow link a higher
level can win because there's less to send; on a fast link a lower level wins
because compression is the bottleneck. If you're optimizing a repeated large job,
benchmark a couple of levels rather than guessing — see
[Performance tuning](/guides/features/optimization).
:::

## When to drop the level

For data that's already compressed — media libraries, `.zip`/`.gz` archives,
encrypted blobs — a high level just burns CPU for no gain. A low level keeps the
pipeline fast:

```bash
cargoship upload ./video-library s3://my-bucket/media/ --compression-level 3
```

Content-aware selection already backs off on recognized incompressible types, so
this mainly matters when your whole dataset is pre-compressed.

## Best practices

::: tip
- **Start with the default** — it streams fast and content-aware selection already
  avoids wasted work on incompressible files.
- **Raise to 9–15 for text-heavy archives** (logs, code, CSV/JSON) where the ratio
  pays off.
- **Keep it low for media and pre-compressed data** — you won't shrink it.
- **Enable [Magika](/guides/features/magika)** for mixed or misnamed content so
  detection (and level choice) is accurate.
- **Benchmark for repeated big jobs** — the best level depends on your link speed
  and CPU, not just the data.
:::

## See also

- [Compression levels (format spec)](/reference/format/compression) — how it's stored and read back.
- [Magika AI file detection](/guides/features/magika) — sharpens content-type detection.
- [Multi-prefix sharding](/guides/features/sharding) — the other half of upload throughput.
- [Performance tuning](/guides/features/optimization) — balancing compression against bandwidth.
- Reference: [Uploading & sync commands](/reference/commands/upload).
