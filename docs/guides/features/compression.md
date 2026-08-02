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
CPU on incompressible data**. The `--compression-level` flag below overrides that
per-chunk choice.

## Overriding the level

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --compression-level 9
```

`--compression-level` is an **override**, not a ceiling: passing it pins *every*
chunk to that level and switches content-aware selection off entirely, including
the back-off on already-compressed data. Omit the flag to keep automatic
per-chunk selection, which is the recommended default.

It accepts zstd levels 1–22 (1–19 recommended). Higher levels compress more but
cost more CPU, and past a point the extra compression is slower to produce than
the bandwidth it saves.

::: warning Levels map to four bands
Go's zstd implementation exposes four internal levels, so the 1–22 range
collapses into four distinct behaviors: **1–2**, **3–5**, **6–9**, and **10+**.
Level 12 and level 19 produce identical output. CargoShip prints the effective
setting at upload start so you can see which band you landed in.
:::

| Level | Speed | Ratio | Good for |
|-------|-------|-------|----------|
| 1–2 | Fastest | ~2–2.5:1 | Network-bound uploads, mostly-compressed data |
| 3–5 | Fast | ~2.5–3:1 | Streaming-friendly throughput |
| 6–9 | Slower | ~3–4:1 | Balanced general use, text-heavy data |
| 10+ | Slowest | ~4:1+ | Cold archives you write once |

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
cargoship upload ./video-library s3://my-bucket/media/ --compression-level 1
```

Content-aware selection already backs off on recognized incompressible types, so
this mainly matters when your whole dataset is pre-compressed *and* detection
can't tell — otherwise omitting the flag gets you the same result without pinning
the level for everything else in the tree.

## Best practices

::: tip
- **Omit `--compression-level`** — content-aware selection already picks a high
  level for text and code and backs off on incompressible files. Passing the flag
  turns that off for the whole upload.
- **Override to 9+ only for uniformly text-heavy archives** (logs, code,
  CSV/JSON), where every chunk wants the same high level anyway.
- **Override to 1 for media and pre-compressed data** — you won't shrink it.
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
