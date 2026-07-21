# Analyzing existing S3 spend

`cargoship analyze` scans an S3 bucket you already have, calculates what it costs
today, and estimates how much re-archiving it in CargoShip's chunked, compressed
format could save. It works against AWS S3 and S3-compatible providers (Wasabi,
Backblaze B2, MinIO). The full flag list lives in the
[command reference](/reference/commands/cost).

```bash
cargoship analyze s3://BUCKET[/PREFIX]
```

## Basic analysis

```bash
cargoship analyze s3://my-bucket
cargoship analyze s3://my-bucket/data
```

CargoShip walks the bucket, tallies object counts, sizes, and storage-class
distribution, computes the current monthly cost, and projects the cost after
re-archiving:

```
═══════════════════════════════════════════════════════════
📊 S3 COST ANALYSIS
═══════════════════════════════════════════════════════════

Bucket: s3://my-bucket
Region: us-west-2

📦 Bucket Statistics:
   Total Objects:        1,284,905
   Total Size:           4.7 TB
   Average Object Size:  3.8 MB

📏 Size Distribution:
   Small (<1 MB):        902,110
   Medium (1-100 MB):    378,220
   Large (100MB-1GB):    4,401
   Huge (>1 GB):         174

═══════════════════════════════════════════════════════════
💰 CURRENT MONTHLY COSTS
═══════════════════════════════════════════════════════════
   Storage Cost:         $110.79
   Request Costs:        $6.42  (estimated)

═══════════════════════════════════════════════════════════
🎯 CARGOSHIP SAVINGS POTENTIAL
═══════════════════════════════════════════════════════════
📦 After CargoShip Re-archiving (Estimated):
├─ Chunks:                18,300 (from 1,284,905 objects)
├─ Total Size:            2.9 TB (compressed)
└─ Projected Monthly Cost: $71.04
```

The savings comparison is shown by default; pass `--show-savings=false` to
suppress it and see only the current-cost breakdown.

## Sampling large buckets

Scanning millions of objects is slow. `--sampling` estimates the totals from a
subset instead of listing every object; tune the sample with `--sample-size`
(default 10,000):

```bash
cargoship analyze s3://my-bucket --sampling
cargoship analyze s3://my-bucket --sampling --sample-size 50000
```

Sampled runs label their figures as estimates and note the sample size. Use a
full scan for billing-grade numbers and sampling for a quick order-of-magnitude
read on a huge bucket.

## Non-AWS providers

Point `analyze` at an S3-compatible provider with `--provider` plus
`--endpoint-url`:

```bash
# Wasabi
cargoship analyze s3://my-bucket --provider wasabi \
  --endpoint-url https://s3.wasabisys.com

# Backblaze B2
cargoship analyze s3://my-bucket --provider b2 \
  --endpoint-url https://s3.us-west-002.backblazeb2.com

# MinIO (self-hosted)
cargoship analyze s3://my-bucket --provider minio \
  --endpoint-url https://minio.example.com
```

Supported `--provider` values are `aws` (default), `wasabi`, `b2`, `minio`, and
`custom`. Each applies that provider's pricing model to the cost estimate.

## Scripting

Emit machine-readable output with `--format json` and disable the live scan
progress with `--progress=false` (handy in CI logs):

```bash
cargoship analyze s3://my-bucket --format json --progress=false
```

Other flags: `--region` (auto-detected from the bucket if omitted) and
`--profile` to select an AWS credentials profile.

## Best practices

::: tip
- **Start with `--sampling`** on huge buckets to get a fast estimate, then run a
  full scan on the ones worth migrating.
- **Analyze before you migrate** — the projected cost tells you whether
  re-archiving a bucket is worth the effort.
- **Use `--format json`** to track spend over time or feed a dashboard.
- **Set `--provider`/`--endpoint-url`** for non-AWS storage so the pricing model
  matches your actual bill.
:::

## See also

- [Estimating costs](/guides/cost/estimate) — estimate an upload before you run it.
- [Cost management & reporting](/guides/cost/management).
- [Lifecycle & storage classes](/guides/cost/lifecycle) — automate transitions to cut spend.
- Reference: [Cost, budget & alerts](/reference/commands/cost).
