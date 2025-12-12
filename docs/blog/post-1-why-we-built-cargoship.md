# Why We Built CargoShip: Solving the S3 Upload Bottleneck

**Published**: December 10, 2025
**Author**: Scott Friedman
**Reading Time**: 5 minutes

---

We tried to upload 2TB of genomics data to S3. It took 18 hours and crashed twice. Traditional tools couldn't handle the scale.

This wasn't a one-time problem. It was happening repeatedly across research labs, media companies, and data teams trying to move large datasets to AWS. The tools we had—`aws s3 sync`, `rclone`, basic backup scripts—all hit the same walls.

## The Problem: When Standard Tools Break Down

If you've ever uploaded more than a terabyte to S3, you know the pain points:

**Request Rate Limits**: AWS S3 has a 3,500 PUT requests per second limit *per prefix*. Most tools upload everything to a single prefix, hitting this ceiling immediately. Your network has gigabit capacity, but you're throttled at the API level.

**Disk Bottleneck**: Traditional backup workflows look like this:
1. Scan your 2TB dataset
2. Compress everything to a staging directory (requires another 2TB of free space)
3. Upload the compressed archives
4. Clean up staging files

You need 4TB of local storage to upload 2TB of data. If you're working with external drives or limited server space, you're stuck before you even start.

**Single-Threaded Uploads**: Tools like `aws-cli` upload one file at a time. Even with `--parallel` flags, you're limited by the single-prefix bottleneck. Your 10 Gbps network connection sits idle while uploads crawl at 30-50 MB/s.

**Manual Cost Optimization**: S3 has eight storage classes with wildly different pricing. Choose Standard for archival data, and you're paying $276/month for 1.2TB. The right choice (Deep Archive) would cost $12/month—a 23x difference. But making that choice requires reading pricing docs, calculating access patterns, and hoping you got it right.

**No Visibility**: Traditional tools are black boxes. You start an 18-hour upload, walk away, and come back to find it failed 12 hours in. No progress tracking, no cost estimates, no insight into what's actually happening.

Here's what our initial attempt looked like:

```bash
$ aws s3 sync /data/genomics s3://research-data/2024-study/
# 18 hours later...
upload failed: Connection timeout
# Start over, lose 12 hours of progress
```

## What We Learned: S3 Has Hidden Performance Levers

After months of failed uploads, manual restarts, and wasted engineering time, we started digging into AWS documentation and experimenting with different approaches.

**Discovery 1: S3 Prefix Sharding**
That 3,500 PUT/s limit? It's *per prefix*. Create eight prefixes, and suddenly you have 28,000 PUT/s capacity. Most tools don't use this because it requires distributing chunks across multiple S3 keys and tracking which prefix each upload targets.

**Discovery 2: Streaming Architecture**
Unix solved the staging problem 50 years ago with pipes: `tar czf - /data | ssh remote "tar xzf -"`. No intermediate files, just data flowing from input to output. We could do the same for S3 uploads—scan files, compress on-the-fly, stream directly to S3. Zero local disk required.

**Discovery 3: Intelligent Chunking**
S3's multipart upload API is optimized for 200MB chunks. Break your data into 200MB archives, upload them in parallel across multiple prefixes, and you saturate your network bandwidth. Content-aware boundaries ensure related files stay together.

**Discovery 4: Storage Class Intelligence**
We built a cost estimator that analyzes your data and calculates exact storage costs across all eight S3 storage classes. For our genomics data (read once, archived for seven years), the answer was obvious: Deep Archive at $147/year instead of Standard at $3,318/year.

**Discovery 5: Open Format = No Lock-In**
We use standard tar+zstd archives. No proprietary formats, no vendor lock-in. You can extract your data in 2050 with standard Unix tools, even if CargoShip no longer exists.

## The Results: From 18 Hours to 2.5 Hours, Zero Disk

After implementing these discoveries, the same 2TB genomics upload transformed:

**Before CargoShip**:
- **Time**: 18 hours (with 2 crashes requiring restart)
- **Disk**: 4TB+ staging space required
- **Throughput**: ~31 MB/s
- **Cost**: $3,318/year (wrong storage class)
- **Visibility**: None (black box upload)

**After CargoShip**:
- **Time**: 2.5 hours (uninterrupted)
- **Disk**: 0 bytes (streaming pipeline)
- **Throughput**: ~227 MB/s
- **Cost**: $147/year (optimized storage class)
- **Visibility**: Real-time progress, cost tracking, metrics

For smaller workloads, the speedup is even more dramatic. We've benchmarked 10,000 small files uploading in 437 milliseconds compared to 311 seconds with traditional tools—a 71x improvement.

## The Solution: CargoShip's Design Principles

CargoShip isn't just faster—it's fundamentally different.

**Zero-Disk Streaming**: Data flows through an in-memory pipeline from scanner to S3 uploader. No staging directory, no temporary files, no "requires 2x storage space" limitation.

**Multi-Prefix Parallelism**: Automatically shard uploads across eight S3 prefixes for 8x request capacity. Your 10 Gbps network finally gets saturated.

**Intelligent Cost Optimization**: Estimate costs across all storage classes before uploading. Apply lifecycle policies automatically. Track spending per project with budget alerts.

**Open Format**: Standard tar+zstd archives. Extract without CargoShip using `zstd -d archive.tar.zst && tar -xf archive.tar`. Your data isn't locked in.

**Enterprise Observability**: Budget tracking, cost forecasting with ML models, multi-channel alerts (Email, Slack, CloudWatch), burn rate analysis—everything you need for production deployments.

## What's Different?

This isn't incremental improvement. It's rethinking the entire upload workflow:

- **Not just faster** — fundamentally different streaming architecture
- **Not just cheaper** — intelligent cost management system with forecasting
- **Not proprietary** — open source, open format, no lock-in
- **Not a black box** — comprehensive metrics, tracing, and budget tracking

## Try It Yourself

CargoShip is open source and available now:

```bash
# Install (requires Go 1.23+)
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest

# Estimate costs before uploading
cargoship estimate /your/data --storage-class INTELLIGENT_TIERING

# Upload with optimal settings
cargoship create upload /your/data \
  --bucket your-bucket \
  --storage-class INTELLIGENT_TIERING \
  --shards 8
```

## What's Next

In the next post, we'll dive deep into the zero-disk streaming architecture—how CargoShip uploads 100GB without writing a single byte to local storage, and why io.Pipe is the secret to high-performance data pipelines.

**Coming up in this series**:
- **Post 2**: Zero-Disk Streaming: How CargoShip Uploads 100GB Without Staging
- **Post 3**: 8x S3 Performance: The Multi-Prefix Sharding Deep Dive
- **Post 4**: Save 90% on S3 Costs with Intelligent Storage Class Selection
- **Post 5**: Open Format, Open Source: Building on CargoShip

---

**About the Author**: Scott Friedman is the creator of CargoShip. This project started from frustration with existing backup tools during large-scale genomics data uploads at Duke University.

**GitHub**: [github.com/scttfrdmn/cargoship](https://github.com/scttfrdmn/cargoship)
**Documentation**: See `/docs` folder in the repository
**Discussions**: [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)
