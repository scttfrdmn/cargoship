# Tutorials (by use-case)

The [Get Started](/start/quickstart) path teaches you the mechanics: install,
upload, verify, restore. These tutorials pick up where that leaves off — they
follow a **realistic persona** through an end-to-end workflow, with concrete
dataset sizes, storage-class choices, and cost trade-offs.

## Pick your use-case

| If you… | Start here |
|---------|-----------|
| Archive millions of tiny files without a huge S3 request bill | [Millions of small files](/tutorials/small-files) |
| Move large sequencing files (FASTQ / BAM / VCF) off an HPC cluster | [Genomics / sequencing data](/tutorials/genomics) |
| Archive microscopy stacks and screening images without losing pixels | [Imaging / microscopy data](/tutorials/imaging) |
| Version ML datasets and push them through DVC | [ML datasets with DVC](/tutorials/ml-dvc) |
| Run shared buckets for a whole lab and keep spend attributable | [Lab data manager](/tutorials/lab-manager) |
| Oversee grant budgets and produce sponsor-ready reports | [Principal investigator](/tutorials/principal-investigator) |
| Come from `rclone` or `aws s3 cp` and want the equivalent commands | [Migrating from rclone / aws cli](/tutorials/migrating) |

::: info How tutorials relate to the rest of the docs
A tutorial **owns the narrative** — the "why", the numbers, and the sequence of
steps for one kind of user. It does not re-document every flag. When a step uses
a feature, it links out to the canonical page:

- **Guides** explain the mechanism (e.g. [Compression](/guides/features/compression),
  [Multi-prefix sharding](/guides/features/sharding), [Tier-aware storage](/guides/features/tiering)).
- **Reference** lists every flag (e.g. [`upload`](/reference/commands/upload)).

If a tutorial and a guide ever seem to disagree on how a flag behaves, the guide
and reference are authoritative.
:::

## Before you start

Every tutorial assumes you've done the [Quick Start](/start/quickstart) once and
have working AWS credentials ([AWS setup](/start/aws-setup)). The canonical
upload command throughout is:

```bash
cargoship upload SOURCE_DIR s3://BUCKET/PREFIX/
```

New to the terms *upload ID*, *manifest*, *shard*, or *chunk*? See
[Concepts & terminology](/intro/concepts).
