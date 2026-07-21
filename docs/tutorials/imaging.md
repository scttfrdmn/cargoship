# Imaging / microscopy data

**You are:** a microscopy researcher archiving large image data — confocal and
light-sheet stacks, proprietary formats (CZI, ND2, LIF), and high-content
screening campaigns with tens of thousands of small images. Two worries drive
everything: **do not lose a single pixel**, and **do not let storage costs run
away**.

This tutorial covers all three shapes of imaging data and how to prove, byte for
byte, that nothing changed.

## Lossless by design

CargoShip compresses with Zstandard, which is **lossless** — decompression
returns the byte-identical original, every pixel and every metadata tag intact.
Compression only ever changes how the bytes are stored in S3, never their
content. You can prove this yourself with checksums (Step 3).

## Your data profile

| Type | Typical size | Compresses further? |
|------|--------------|---------------------|
| Uncompressed / OME-TIFF stacks | 20–100 GB/stack | Yes, meaningfully — raw pixel data |
| CZI / ND2 / LIF (proprietary) | 50–300 GB/sample | Little — internally compressed already |
| Screening PNG/JPEG (many files) | 5–10 MB each, 10k–50k files | Modest — already compressed formats |

## Step 1 — A single confocal stack

A 4-channel Z-stack is a handful of large TIFFs. Uncompressed TIFF is the case
where lossless compression genuinely earns its keep:

```bash
cargoship upload /data/confocal/experiment-2026-05-15 \
  s3://cellbio-imaging/confocal/experiment-2026-05-15/ \
  --region us-east-1 \
  --project confocal-2026
```

::: tip Point CargoShip at the right AWS account
CargoShip uses the standard AWS credential chain. To use a named profile, set it
in the environment rather than a flag:

```bash
AWS_PROFILE=cellbio-lab cargoship upload ./experiment s3://cellbio-imaging/...
```
:::

## Step 2 — Proprietary formats: skip the wasted CPU

CZI/ND2/LIF files are already internally compressed (e.g. JPEG-XR), so a high
Zstandard level buys almost nothing while burning CPU. Drop to the fastest level
so the upload is I/O-bound, not compute-bound:

```bash
cargoship upload /data/lightsheet/dev-study-2026 \
  s3://cellbio-imaging/lightsheet/dev-study-2026/ \
  --compression-level 1 \
  --project lightsheet-2026
```

CargoShip's content-aware selection already avoids re-compressing data it detects
as packed — level 1 just makes the intent explicit for a whole tree of known-
compressed files. See [Compression](/guides/features/compression).

## Step 3 — Prove integrity (bit-perfect round trip)

For precious data, don't take it on faith. Checksum before, restore, checksum
after:

```bash
# Before upload
cd /data/confocal/experiment-2026-05-15
sha256sum *.tif > /tmp/before.sha256

# Validate the archive against its manifest
cargoship verify s3://cellbio-imaging/confocal/experiment-2026-05-15/uploads/<id>

# Restore and re-checksum
cargoship restore s3://cellbio-imaging/confocal/experiment-2026-05-15/uploads/<id> \
  /tmp/restore
cd /tmp/restore && sha256sum -c /tmp/before.sha256
```

Matching checksums are your proof of lossless preservation. See
[Verifying integrity](/guides/verifying) and [Restoring files](/guides/restoring).

## Step 4 — High-content screening: many small files

A screening campaign can be 50,000 PNGs across dozens of plate directories. The
challenge here isn't size, it's **file count** — naive per-file uploads hammer S3
request limits. CargoShip groups files into chunks and spreads them across
prefixes automatically:

```bash
cargoship upload /data/screening/drug-campaign-2026-Q2 \
  s3://cellbio-imaging/screening/drug-campaign-2026-Q2/ \
  --project screening-2026
```

The [adaptive shard count](/guides/features/sharding) rises with file count, and
grouping tens of thousands of small images into a handful of compressed chunks
collapses what would be tens of thousands of PUT requests into a few — which is
also where the cost savings come from (fewer requests, smaller objects).

## Step 5 — Keep storage costs down

Finished experiments that you must keep but rarely reopen belong in a colder
class:

```bash
cargoship upload ./old-campaigns s3://cellbio-imaging/archive/ \
  --storage-class INTELLIGENT_TIERING
```

`INTELLIGENT_TIERING` lets S3 move cold objects down automatically with no
retrieval penalty for the occasional access — a good default for imaging you
might revisit. For age-based per-chunk placement into Glacier, use
`--auto-tier --tier-strategy tier-aware` (see [Tiering](/guides/features/tiering)).

::: warning
Deep-archive classes are cheapest to store but slowest and priciest to retrieve.
Don't send data you cite in an active paper to `DEEP_ARCHIVE`. See
[Costs & safety](/intro/costs-and-safety).
:::

## Recap

- Zstandard is lossless — pixels and metadata survive exactly. Prove it with
  `sha256sum` around a `restore`.
- Compress raw TIFF; use `--compression-level 1` for CZI/ND2/LIF to save CPU.
- Many-small-file screening sets are handled by automatic chunking + sharding.
- Cold campaigns → `INTELLIGENT_TIERING` or age-based tiering.

## Next steps

- [Compression](/guides/features/compression) · [Sharding](/guides/features/sharding) · [Verifying integrity](/guides/verifying).
- [Restoring files](/guides/restoring) · [Tier-aware storage](/guides/features/tiering).
- [`upload` reference](/reference/commands/upload).
