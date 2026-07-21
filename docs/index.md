---
layout: home

hero:
  name: CargoShip
  text: High-performance S3 data archiving
  tagline: Stream large datasets straight to S3 — sharded, compressed, verifiable, and cost-aware. Your data lands in an open, portable archive format you can read with or without CargoShip.
  image:
    src: /hero.svg
    alt: CargoShip
  actions:
    - theme: brand
      text: Quick Start
      link: /start/quickstart
    - theme: alt
      text: What is CargoShip?
      link: /intro/how-it-works
    - theme: alt
      text: View on GitHub
      link: https://github.com/scttfrdmn/cargoship

features:
  - icon: 🚢
    title: Zero-disk streaming pipeline
    details: Scanner → Chunker → Archiver → Uploader, connected over io.Pipe. Data streams directly to S3 without staging to local disk, so memory stays bounded regardless of dataset size.
    link: /intro/how-it-works
    linkText: How it works
  - icon: ⚡
    title: Multi-prefix parallel sharding
    details: CargoHold spreads chunks across multiple S3 prefixes with an adaptive shard count (4–32), parallelizing request throughput instead of funnelling through one key space.
    link: /guides/features/sharding
    linkText: Learn more
  - icon: 🗜️
    title: Content-aware compression
    details: Streaming tar + Zstandard with per-content-type levels, plus optional Magika AI file-type detection to compress smarter.
    link: /guides/features/compression
    linkText: Learn more
  - icon: 💰
    title: Cost & budget controls
    details: Estimate before you upload, track spend by project, set dual cost + volume quotas, and forecast burn-down with multiple models.
    link: /guides/cost/estimate
    linkText: Learn more
  - icon: 🔒
    title: Encryption & integrity
    details: SSE and KMS-envelope-encrypted manifests, optional GPG, and manifest-checksum verification that proves a round trip on restore.
    link: /guides/features/encryption
    linkText: Learn more
  - icon: 📖
    title: Open, portable format
    details: Plain tar.zst objects plus a documented JSON manifest (v2.0). Extract your data with standard tools even without CargoShip installed.
    link: /reference/format/
    linkText: Read the spec
---

## Get where you're going

CargoShip ships large research and backup datasets to Amazon S3 quickly, cheaply,
and reversibly. Pick the path that fits you:

- **New here?** Start with [How it works](/intro/how-it-works) for the mental model,
  then run the [Quick Start](/start/quickstart) to get a verified upload in a few minutes.
- **Know what you want?** Jump to a [use-case tutorial](/tutorials/) (genomics, imaging,
  ML/DVC, lab data) or the [command reference](/reference/).
- **Building tooling on top?** Read the [canonical archive & manifest format spec](/reference/format/).

### Install

::: code-group

```bash [Homebrew]
brew install scttfrdmn/tap/cargoship
```

```bash [Go]
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

```bash [Binary]
# Download the latest release for your platform from:
# https://github.com/scttfrdmn/cargoship/releases/latest
```

:::

Then send your first dataset to S3:

```bash
cargoship upload ./my-data s3://my-bucket/archives/
```

See [Install](/start/install) for all methods and [Your first upload](/start/first-upload)
for a line-by-line walkthrough.
