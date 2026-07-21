# Genomics / sequencing data

**You are:** a researcher moving whole-genome sequencing data off an HPC cluster
to S3. Your files are large (FASTQ, BAM/CRAM, VCF), your network is shared with
the rest of the lab, and `aws s3 cp` both crawls and monopolizes bandwidth.

This tutorial walks a single genome, then a full cohort, and shows how to pick
compression per file type so you don't waste CPU on data that's already packed.

## Your data profile

| Type | Typical size | Compresses further? |
|------|--------------|---------------------|
| FASTQ (`.fastq.gz`) | 25–50 GB/file, paired R1/R2 | ~15% with zstd — already gzipped |
| BAM / CRAM | 20–40 GB/file | Barely — BGZF/reference-compressed |
| VCF (`.vcf`) | 100 MB–2 GB | 60–80% — plain text |
| VCF (`.vcf.gz`) | smaller | ~10–15% |

The lesson repeated below: **compression pays off on text (VCF), not on
already-compressed formats (BAM/CRAM, `.gz`).**

## Step 1 — Estimate before you move a terabyte

```bash
cargoship estimate /cluster/data/WGS/sample001 --show-comparison
```

`--show-comparison` contrasts CargoShip's chunked upload against a naive
per-file `aws s3 cp` — useful when justifying the switch to a PI. Nothing
touches S3. See [Estimating costs](/guides/cost/estimate).

## Step 2 — Upload a single genome

One WGS sample is two ~25 GB FASTQ files:

```bash
cargoship upload /cluster/data/WGS/sample001 \
  s3://genomics-lab-wgs/cohort-2026/sample001/ \
  --region us-west-2 \
  --project cohort-2026
```

CargoShip scans the two files, chunks and shards them, compresses each chunk
with Zstandard, and streams to S3 — nothing lands on local disk. It prints an
[upload ID](/intro/concepts#upload-id); save it.

::: tip Pro tip
Tag the upload with `--project` from the very first run. It costs nothing now
and makes [cost reports](/guides/cost/management) and
[budgets](/guides/cost/budgets) meaningful later — which you'll want once the
whole lab is uploading.
:::

FASTQ is already gzipped, so the default level is the right call. If you're
archiving plain-text VCFs and want to trade CPU for smaller objects, bump it:

```bash
cargoship upload ./variants s3://genomics-lab-wgs/cohort-2026/vcf/ \
  --compression-level 19
```

See [Compression & content-aware selection](/guides/features/compression) for
how CargoShip already skips re-compressing content it detects as packed.

## Step 3 — Upload a full cohort

A 25-genome cohort is ~1.25 TB across 50 FASTQ files. The same command scales —
just point at the cohort directory:

```bash
cargoship upload /cluster/data/WGS/cohort-2026 \
  s3://genomics-lab-wgs/cohort-2026/ \
  --project cohort-2026
```

The [shard count is adaptive](/guides/features/sharding) (4–32), tuned to the
file count and total size, and CargoShip spreads chunks across multiple S3
prefixes so you don't hit per-prefix request limits on a big cohort. Leave it on
auto unless you're benchmarking; override with `--shard-count` if you must.

::: tip Sharing the network
CargoShip uploads compressed chunks, so the bytes on the wire are already
reduced. On a 10 Gbps cluster link a sustained cohort upload uses a small
fraction of capacity — friendly enough to run during business hours rather than
scheduling a midnight job. Confirm your own numbers with a single-genome run
first.
:::

## Step 4 — Confirm it landed, then prove the round trip

```bash
# What's in this upload?
cargoship info  s3://genomics-lab-wgs/cohort-2026/uploads/<id>
cargoship list  s3://genomics-lab-wgs/cohort-2026/uploads/<id> --pattern '*.fastq.gz'

# Validate integrity against the manifest
cargoship verify s3://genomics-lab-wgs/cohort-2026/uploads/<id>
```

To pull one sample back out without downloading the whole cohort, restore a
single path:

```bash
cargoship restore s3://genomics-lab-wgs/cohort-2026/uploads/<id> ./restored \
  --file cohort-2026/sample001/sample001_R1.fastq.gz
```

See [Restoring files](/guides/restoring) — including the `--tier` options when
your data has aged into Glacier.

## Step 5 — Long-term archival tiers

Raw FASTQ you rarely re-read is a classic cold-storage candidate. Two ways:

```bash
# Whole upload to one class
cargoship upload ./cohort s3://genomics-lab-wgs/archive/ \
  --storage-class GLACIER_IR

# Or let CargoShip assign classes per chunk by file age
cargoship upload ./cohort s3://genomics-lab-wgs/archive/ \
  --auto-tier --tier-strategy tier-aware --tier-max GLACIER
```

::: warning
`--tier-strategy tier-aware` changes retrieval characteristics and prompts for
confirmation; add `--yes` in automation and `--tier-max` to cap how cold
anything goes. Glacier restores cost money and take time — see
[Tier-aware storage](/guides/features/tiering) and
[Costs & safety](/intro/costs-and-safety).
:::

## Recap

- Estimate first; it's free and makes the case for switching.
- Compress text (VCF) hard; let the default handle already-packed FASTQ/BAM.
- One command scales from a single genome to a full cohort — sharding is
  automatic.
- Always `verify`, and restore individual files by path.
- Push cold cohorts to Glacier tiers, mindful of retrieval cost.

## Next steps

- [Uploading data](/guides/uploading) — the full workflow guide.
- [Compression](/guides/features/compression) · [Sharding](/guides/features/sharding) · [Tiering](/guides/features/tiering).
- [`upload` reference](/reference/commands/upload) — every flag.
- Managing a whole lab's uploads? See [Lab data manager](/tutorials/lab-manager).
