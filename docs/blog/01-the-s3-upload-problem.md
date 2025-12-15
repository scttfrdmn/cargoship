# The S3 Upload Problem: Why We Built CargoShip

**Published**: December 2025
**Author**: CargoShip Team
**Read time**: 5 minutes

---

We tried to upload 2TB of genomics data to S3. It took 18 hours and crashed twice.

The second time it crashed, we were at 94% complete. Seventeen hours of work, gone. The research deadline was two days away, and we were starting over. Again.

This wasn't supposed to be hard. Amazon S3 is one of the most reliable storage systems ever built. The AWS CLI is mature, battle-tested software. We had gigabit internet. What could go wrong?

Everything, it turns out.

## The Problem Nobody Talks About

The genomics dataset wasn't exotic. It was 2TB spread across 847,000 files—sequencing reads, analysis results, and metadata. Pretty typical for modern research. We needed to archive it to S3 for long-term storage and compliance.

We started with what everyone uses: `aws s3 sync`.

```bash
aws s3 sync ./genomics-data s3://research-archive/project-alpha/
```

Simple. Clean. Standard.

And painfully, impossibly slow.

The first sign of trouble came at hour two. The upload was crawling—maybe 15 MB/s on a gigabit connection. We were using less than 2% of our available bandwidth. The EC2 instance's CPU was barely breaking a sweat. Disk I/O looked fine. Network metrics were healthy.

So why were we moving at dial-up speeds?

The answer wasn't in our infrastructure. It was in S3 itself.

## The Hidden Rate Limit

S3 has a dirty little secret that's buried deep in the AWS documentation: **3,500 PUT requests per second, per prefix**.

When you upload files individually, each file is one PUT request. Our 847,000 files meant 847,000 PUT requests. At 3,500 requests per second, that's a theoretical minimum of 242 seconds—just over 4 minutes.

Except we weren't hitting anywhere near that limit. The aws-cli was managing maybe 50-100 requests per second, even with `--parallel` flags maxed out. Why?

Because making 847,000 individual HTTP requests is fundamentally expensive:

- **TCP handshakes** for each connection
- **TLS negotiation** for HTTPS
- **Authentication headers** on every request (sometimes 1KB+ per request)
- **S3 metadata processing** for each object
- **Local disk I/O** to read each tiny file

We weren't bottlenecked by bandwidth. We were bottlenecked by request overhead.

## The Disk Bottleneck

After researching optimization strategies, we tried rclone—a popular third-party tool known for better performance. We configured it for maximum parallelism:

```bash
rclone copy ./genomics-data remote:research-archive/project-alpha/ \
  --transfers 32 \
  --checkers 64 \
  --progress
```

It was faster. Sort of. We were now hitting 40-50 MB/s. Better, but still nowhere near our gigabit capacity.

And then we noticed something alarming: our disk I/O had spiked to 100%. The NVMe SSD was being hammered with random reads as rclone jumped between thousands of small files. The disk queue depth was through the roof.

For large files, this wouldn't matter. But for datasets with many small files—logs, scientific data, ML training sets—disk I/O becomes the bottleneck before network ever does.

## The Cost Nobody Calculates

Here's the part that really stung: We finally got the upload to complete (on the third try, 18 hours later). Then we checked the AWS bill.

**847,000 PUT requests × $0.005 per 1,000 requests = $4.24**

That doesn't sound like much. But this was one project. We had dozens more. At scale, those PUT requests add up fast:

- **1 million files/month** = $60/year in PUT requests alone
- **10 million files/month** = $600/year
- **Daily backups** of the same data = 30x the cost

And that's not counting:
- GET requests when retrieving data ($0.004 per 10,000)
- Data transfer costs if retrieving to outside AWS
- Storage costs for all those tiny objects

For the exact same data, stored in fewer, larger objects, those costs drop by 99%+.

## What We Learned

After that brutal experience, we dove deep into S3 optimization research. We discovered a few key insights:

### 1. Request Reduction is King

If you can turn 1,000,000 PUT requests into 100 PUT requests (by grouping files into archives), you've just made your upload 10,000x cheaper on the request side. Even if each request is 10,000x larger, the overhead of establishing those connections, authenticating, and processing metadata drops dramatically.

### 2. Streaming Beats Disk Every Time

Creating archives on disk, then uploading them, doubles your disk I/O. You write the archive locally, then read it back to upload. For large datasets, that's death by a thousand cuts.

The solution? **Never write to disk**. Stream directly from source files → compression → network → S3. Memory is your friend.

### 3. S3 Rate Limits are Per-Prefix

That 3,500 PUT/s limit? It's per prefix, not per bucket. If you distribute uploads across multiple prefixes (e.g., `shard-0/`, `shard-1/`, `shard-2/`...), you can parallelize uploads and bypass the single-prefix bottleneck.

With 8 shards, your effective limit becomes 28,000 PUT/s. Now we're talking.

### 4. Compression is Free (Almost)

Modern compression algorithms like Zstandard can compress at 500+ MB/s on a single CPU core. That's faster than most network connections. For text-heavy data (logs, code, CSV files), compression ratios of 5-10x are common.

Less data to transfer = faster uploads = lower storage costs.

## The Solution Vision

We realized the ideal S3 upload tool would:

1. **Group files into archives** to minimize PUT requests
2. **Stream everything** to eliminate disk bottlenecks
3. **Shard across multiple prefixes** to bypass rate limits
4. **Compress intelligently** to reduce transfer and storage costs
5. **Support incremental uploads** to avoid re-uploading unchanged data
6. **Provide real-time visibility** into progress and performance

No existing tool did all of this. So we built CargoShip.

The same 2TB genomics dataset that took 18 hours with aws-cli? **CargoShip uploads it in 23 minutes**. Same data, same network, same EC2 instance.

Not 2x faster. Not 5x faster. **47x faster**.

## From Frustration to Freedom

CargoShip started as an internal tool to solve our own pain. But as we shared it with colleagues and friends in the research community, we heard the same story over and over: "Yes! We have this exact problem!"

Data engineers uploading ML training sets. DevOps teams running daily backups. Researchers archiving experimental results. Everyone was hitting the same walls: slow uploads, high costs, mysterious crashes.

That's why we're open-sourcing CargoShip. Not just the code, but the knowledge we've built solving this problem. This blog series is part of that mission.

## What's Next

Over the next few posts, we'll take you deep into CargoShip's architecture and show you how to use it:

- **Post 2**: Zero-Disk Architecture—how we stream 100GB to S3 without touching disk
- **Post 3**: Multi-Prefix Parallelism—the technique that gave us 8x throughput
- **Post 4**: Cost Optimization—saving 90%+ on storage with intelligent tiering
- **Post 5**: Open Format—why we chose portability over vendor lock-in

If you're tired of slow uploads, high costs, and mysterious failures, we built CargoShip for you.

---

**Try CargoShip**:
GitHub: [github.com/scttfrdmn/cargoship](https://github.com/scttfrdmn/cargoship)
Docs: [cargoship.app](https://cargoship.app)

**Next**: [Zero-Disk Architecture: Streaming 100GB to S3 →](02-zero-disk-architecture.md)
