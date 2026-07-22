# Millions of small files: the request-cost problem

**You are:** archiving a huge tree of tiny files — ML training shards, per-read
genomics output, sensor readings, JSON records, web-scrape results. Individually
they're bytes to kilobytes; together there are hundreds of thousands or millions
of them. A naive `aws s3 cp --recursive` or `aws s3 sync` "works," but the bill
and the wall-clock time are both dominated by something that has nothing to do
with how much data you have.

This tutorial explains **why** that happens, shows the cost difference with real
`cargoship` output, and walks a runnable demo you can try in a minute.

## The problem isn't your bytes — it's your requests

S3 charges for **PUT requests**, not just storage. Every individual object you
upload is one PUT. At the STANDARD price of **$0.005 per 1,000 PUT requests**,
the request charge scales with your *file count*, independent of file size:

```
     1,000,000 files ÷ 1,000 × $0.005  =  $5.00
     5,000,000 files ÷ 1,000 × $0.005  = $25.00
    50,000,000 files ÷ 1,000 × $0.005  = $250.00
```

That's **just the upload requests** — before a byte of storage. Fifty million
100-byte files hold ~5 GB of data (a few cents/month to store) but cost **$250
to upload** naively. The data is trivial; the request count is the bill.

Throughput suffers for the same reason: a naive tool issues one round-trip PUT
per file. Millions of sequential small requests spend almost all their time in
per-request overhead, not moving data.

::: info Why CargoShip helps
CargoShip **packs many files into a few compressed chunk objects** before
uploading. A million files become a few hundred `tar.zst` chunks — so you pay a
few hundred PUTs instead of a million, stream them with high concurrency, and
compress on the way. Selective restore still pulls a single file back out of its
chunk. See [How it works](/intro/how-it-works) and
[Sharding](/guides/features/sharding).
:::

## The numbers: naive vs CargoShip

CargoShip can compute this for you with `cargoship cost benchmark-compare` — no
upload, no AWS calls required. Take **5,000,000 files totalling 50 GB**.

**Naive per-file upload** (`aws s3 cp`, `aws s3 sync`, `rclone`, `s5cmd` — all
one PUT per file):

```bash
cargoship cost benchmark-compare --tool aws-cli --size-gb 50 --files 5000000
```

```json
{
  "tool": "aws-cli",
  "put_request_cost": 25,
  "storage_cost_monthly": 1.15,
  "total_upload_cost": 25,
  "annual_tco": 38.8,
  "currency": "USD"
}
```

`put_request_cost` is **$25.00** — exactly the `5,000,000 ÷ 1,000 × $0.005`
from above. That's the one-time cost of the upload's requests alone.

**CargoShip** (same data, packed into chunks, 3:1 compression):

```bash
cargoship cost benchmark-compare --tool cargoship --size-gb 50 --files 5000000 \
  --compression-ratio 3.0
```

```json
{
  "tool": "cargoship",
  "put_request_cost": 0.00085,
  "annual_tco": 4.60,
  "cargoship_advantages": {
    "request_reduction": 4999830,
    "savings_percentage": 88.1,
    "competitor_comparison_cost": 38.8
  }
}
```

CargoShip packs the 5,000,000 files into ~170 chunk objects, so the PUT cost
collapses from **$25.00 to under a tenth of a cent** — `request_reduction` of
**4,999,830** fewer requests. Combined with compression on the stored bytes, the
**first-year total cost drops ~88%** (`$38.80 → $4.60`).

| | Naive (`aws s3 cp`) | CargoShip |
|---|---|---|
| Objects PUT | 5,000,000 | ~170 |
| Upload request cost | **$25.00** | **$0.0009** |
| First-year total (TCO) | $38.80 | $4.60 |

::: tip These are your prices, not ours
`benchmark-compare` uses the same pricing engine as `cargoship estimate`. With
AWS credentials configured it can pull **live, region-specific** rates; otherwise
it uses current published STANDARD pricing. Nothing here is a marketing number —
re-run the commands above and you'll get the same result. See
[Benchmarks & methodology](/reference/benchmarks).
:::

## Try it: a 10,000-file demo

Generate a tree of tiny files and let CargoShip show the comparison on **your**
data. This touches no S3 and costs nothing.

```bash
# 10,000 tiny JSON records across 10 subdirectories
mkdir -p /tmp/smallfiles-demo
python3 - <<'PY'
import os
for i in range(10000):
    d = f"/tmp/smallfiles-demo/shard{i//1000:02d}"
    os.makedirs(d, exist_ok=True)
    with open(f"{d}/rec{i:05d}.json", "w") as f:
        f.write('{"id": %d, "value": "%s"}\n' % (i, "x" * 150))
PY

cargoship estimate /tmp/smallfiles-demo --show-comparison
```

The `--show-comparison` block reports:

```
📊 File Statistics:
   Total Files:        10,000
   Estimated Chunks:   100

💵 Savings Breakdown:
   PUT Request Cost Savings:  $0.05 (one-time, 9,900 requests reduced)

🎯 Total Monthly Savings:  $0.07 (99.0% reduction)

💡 Key Benefits:
   🎉 Excellent! CargoShip chunking provides >75% cost savings for this workload
   💡 Small files (<40 KB average) - chunking eliminates 4x minimum size penalty
      on archive tiers
```

10,000 files → ~100 chunks: **9,900 fewer PUT requests** and a 99% cost
reduction for this workload. The same ratio holds as you scale to millions —
that's the whole point.

::: tip Bonus: the minimum-size penalty
Archive tiers (Glacier, Deep Archive) bill a **minimum billable object size**
(e.g. 128 KB) per object. A tree of sub-KB files on Glacier is charged as if each
were 128 KB — a 100×+ storage penalty on tiny files. Packing them into large
chunks eliminates it, on top of the request savings above.
:::

## Do the real upload

When the estimate convinces you, the upload itself is the ordinary command:

```bash
cargoship upload /tmp/smallfiles-demo s3://my-bucket/small-files/ \
  --region us-west-2 --project small-files-demo
```

CargoShip scans, chunks, compresses, and streams — no per-file PUT storm, no
local scratch space. You get an [upload ID](/intro/concepts#upload-id), a
verifiable [manifest](/reference/format/manifest), and single-file
[selective restore](/guides/restoring) from within a chunk.

## When this applies (and when it doesn't)

- **Great fit:** large counts of small-to-medium files you're **archiving or
  backing up** — you read them back occasionally or in bulk, not individually at
  high frequency.
- **Not the goal:** if every file must stay an **individually addressable,
  frequently-served S3 object** (e.g. a public web asset bucket), keep them as
  separate objects; the packing that saves you requests also means a restore
  reads a chunk. See the honest [tool comparison](/reference/comparison).

## Next steps

- [Estimating costs](/guides/cost/estimate) · [Benchmarks & methodology](/reference/benchmarks).
- [How it works](/intro/how-it-works) · [Sharding](/guides/features/sharding) · [Compression](/guides/features/compression).
- [`cost` reference](/reference/commands/cost) · [`upload` reference](/reference/commands/upload).
