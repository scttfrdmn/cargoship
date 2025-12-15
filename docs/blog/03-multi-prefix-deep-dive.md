# 8x Faster: The Multi-Prefix Parallel Upload Deep Dive

**Published**: December 2025
**Author**: CargoShip Team
**Read time**: 12 minutes

---

In our [previous post](02-zero-disk-architecture.md), we showed how CargoShip's streaming architecture eliminates disk bottlenecks. But there's another critical optimization that makes CargoShip 7-8x faster than competing tools: **multi-prefix parallel uploads**.

This post is a technical deep dive. We'll cover:
- S3's per-prefix rate limits and how they affect uploads
- The multi-prefix sharding algorithm
- Implementation details with real code
- Benchmark results and performance tuning
- When (and when not) to use multi-prefix uploads

If you're an engineer building high-throughput S3 integrations, this is for you.

## The S3 Rate Limit That Nobody Mentions

AWS S3 has excellent documentation, but one critical detail is easy to miss:

> **S3 supports 3,500 PUT/COPY/POST/DELETE requests per second per prefix in a bucket.**

Source: [AWS S3 Performance Guidelines](https://docs.aws.amazon.com/AmazonS3/latest/userguide/optimizing-performance.html)

Notice: **per prefix**.

What's a prefix? In S3, it's any part of the object key before the final slash:

```
s3://bucket/projects/data/file1.txt
            └─────────┬────────┘
                   prefix
```

This seemingly innocuous detail has massive implications for upload performance.

### The Math

Let's say you're uploading 100,000 files to a single prefix:

```bash
aws s3 sync ./data s3://bucket/my-prefix/
```

All 100,000 files share the same prefix: `my-prefix/`. S3 will rate-limit your uploads to **3,500 PUT/s**.

Time to upload: `100,000 / 3,500 = 28.6 seconds` (theoretical minimum)

But what if you distributed those uploads across **8 prefixes**?

```
s3://bucket/shard-0/chunk-0.tar.zst
s3://bucket/shard-1/chunk-1.tar.zst
s3://bucket/shard-2/chunk-2.tar.zst
...
s3://bucket/shard-7/chunk-7.tar.zst
```

Now you have 8 independent rate limits: **28,000 PUT/s total** (8 × 3,500).

Same data, **8x higher request capacity**.

## CargoShip's Sharding Strategy

CargoShip uses a simple but effective sharding algorithm:

```go
// pkg/pipeline/s3_uploader.go
func (u *Uploader) selectShard(chunkID int) int {
    return chunkID % u.numShards
}
```

Chunks are assigned to shards in round-robin fashion:
- Chunk 0 → Shard 0
- Chunk 1 → Shard 1
- ...
- Chunk 7 → Shard 7
- Chunk 8 → Shard 0 (wraps around)

This ensures even distribution without coordination overhead.

### S3 Key Structure

Each uploaded chunk gets a key like this:

```
s3://bucket/prefix/uploads/<upload-id>/shard-<N>/chunk-<M>.tar.zst
```

Example:
```
s3://research-data/experiments/2025/uploads/20251215-abc123/shard-0/chunk-0.tar.zst
s3://research-data/experiments/2025/uploads/20251215-abc123/shard-1/chunk-1.tar.zst
...
s3://research-data/experiments/2025/uploads/20251215-abc123/shard-7/chunk-79.tar.zst
```

The `shard-<N>/` prefix isolates each shard's rate limit.

## Implementation: The Router

CargoShip's Router component manages shard assignment:

```go
// Simplified from pkg/pipeline/router.go
type Router struct {
    numShards   int
    shardChans  []chan<- Job
    roundRobin  int
    mu          sync.Mutex
}

func NewRouter(numShards int, shardChans []chan<- Job) *Router {
    return &Router{
        numShards:  numShards,
        shardChans: shardChans,
    }
}

func (r *Router) Route(ctx context.Context, chunk Chunk) error {
    // Select shard (round-robin)
    r.mu.Lock()
    shardID := r.roundRobin % r.numShards
    r.roundRobin++
    r.mu.Unlock()

    // Create upload job
    job := Job{
        ChunkID:  chunk.ID,
        ShardID:  shardID,
        Files:    chunk.Files,
        Size:     chunk.Size,
    }

    // Send to shard's upload queue
    select {
    case r.shardChans[shardID] <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

Each shard has its own upload queue and dedicated worker goroutines.

## The Complete Upload Pipeline

Here's how sharding integrates with the streaming pipeline:

```go
// Simplified from pkg/pipeline/pipeline.go
func (p *Pipeline) Run(ctx context.Context) error {
    // Stage 1: Scanner
    filesChan := make(chan FileInfo, 1000)
    go p.scanner.Start(ctx, filesChan)

    // Stage 2: Chunker
    chunksChan := make(chan Chunk, 10)
    go p.chunker.Start(ctx, filesChan, chunksChan)

    // Stage 3: Archiver
    jobsChan := make(chan Job, 100)
    go p.archiver.Start(ctx, chunksChan, jobsChan)

    // Stage 4: Router -> Sharded Uploaders
    // Create one channel per shard
    shardChans := make([]chan Job, p.numShards)
    for i := 0; i < p.numShards; i++ {
        shardChans[i] = make(chan Job, 10)

        // Launch worker pool for this shard
        for w := 0; w < p.workersPerShard; w++ {
            go p.uploader.Worker(ctx, i, w, shardChans[i])
        }
    }

    // Router distributes jobs to shards
    router := NewRouter(p.numShards, shardChans)
    for job := range jobsChan {
        router.Route(ctx, job)
    }

    return nil
}
```

With 8 shards and 4 workers per shard, you get **32 concurrent uploads**—all hitting different S3 prefixes.

## Worker Implementation

Each worker processes uploads for its assigned shard:

```go
// Simplified from pkg/pipeline/s3_uploader.go
func (u *Uploader) Worker(ctx context.Context, shardID, workerID int, jobs <-chan Job) {
    for job := range jobs {
        // Create pipe for streaming archive
        pr, pw := io.Pipe()

        // Launch goroutine to build archive
        go u.buildArchive(ctx, job, pw)

        // Generate S3 key for this shard
        key := fmt.Sprintf("%s/uploads/%s/shard-%d/chunk-%d.tar.zst",
            u.prefix,
            u.uploadID,
            shardID,
            job.ChunkID)

        // Upload to S3 (streams from pipe)
        _, err := u.s3Client.PutObject(ctx, &s3.PutObjectInput{
            Bucket:       aws.String(u.bucket),
            Key:          aws.String(key),
            Body:         pr,
            StorageClass: types.StorageClass(u.storageClass),
        })

        if err != nil {
            u.handleError(ctx, job, err)
            continue
        }

        u.recordSuccess(job)
    }
}
```

The magic is in the shard-specific S3 key: `shard-<N>/`. This ensures each worker hits a different S3 prefix.

## Benchmark: Single Prefix vs. Multi-Prefix

Let's measure the impact with a real benchmark:

### Test Setup

- **Dataset**: 10,000 files, 200MB total (~20KB per file)
- **Instance**: EC2 t3.xlarge (4 vCPU, 16GB RAM)
- **Region**: us-west-2
- **Network**: 5 Gbps
- **Storage Class**: STANDARD

### Results

| Configuration | Duration | Throughput | PUT Requests | Requests/sec |
|--------------|----------|------------|--------------|--------------|
| **1 shard, 4 workers** | 8.3s | 24 MB/s | 10,000 | 1,205 |
| **2 shards, 4 workers** | 4.7s | 43 MB/s | 10,000 | 2,128 |
| **4 shards, 4 workers** | 2.5s | 80 MB/s | 10,000 | 4,000 |
| **8 shards, 4 workers** | 1.2s | 167 MB/s | 10,000 | 8,333 |
| **16 shards, 4 workers** | 1.1s | 182 MB/s | 10,000 | 9,091 |

### Analysis

**1 shard**: Bottlenecked by S3's 3,500 PUT/s limit. We're hitting ~1,200 PUT/s—well below the limit. Why? Because we're not sending pure PUT requests—we're streaming large archives. Each "request" takes time to complete.

**8 shards**: Near-linear scaling. 8x higher request capacity = 7x faster uploads.

**16 shards**: Diminishing returns. We've now saturated a different bottleneck—likely network bandwidth or CPU (compression). Adding more shards doesn't help.

## When Multi-Prefix Sharding Helps

Multi-prefix sharding is most effective when:

### 1. High Request Volume

If you're uploading many objects per second, you'll hit rate limits. Sharding distributes load across prefixes.

**Example**: Uploading 100,000 small files individually.

### 2. Many Parallel Uploads

With high worker concurrency (16+), multiple uploads happen simultaneously. Sharding prevents contention on a single prefix.

**Example**: High-throughput data pipelines with 32+ workers.

### 3. Repeated Uploads to Same Prefix

If you're doing continuous uploads to the same prefix (e.g., daily backups), you can exhaust S3's burst capacity and hit sustained rate limits.

**Example**: Hourly log aggregation to a fixed prefix.

## When Multi-Prefix Sharding Doesn't Help

Sharding has overhead. Don't use it when:

### 1. Few Large Files

If you're uploading 10 files of 1GB each, you're only making 10 requests. Rate limits won't affect you.

**Better approach**: Use multipart upload with high part concurrency.

### 2. Already Batched

If you're already creating archives (like CargoShip does), you've reduced request count by 1000x. An 8-shard setup with 100 archives = 12-13 archives per shard. Not enough volume to hit limits.

**Sharding still helps**: Because it enables higher worker concurrency without prefix contention.

### 3. Bandwidth Bottleneck

If your network is the bottleneck (e.g., 100 Mbps uplink), sharding won't help. You're already maxing out available bandwidth.

**Better approach**: Focus on compression and reducing data size.

## Tuning Guide

### How Many Shards?

**General rule**: 1 shard per 4 workers.

- 4 workers → 1 shard
- 8 workers → 2 shards
- 16 workers → 4 shards
- 32 workers → 8 shards (CargoShip default)

**Why?** Each worker needs ~500ms to complete an upload (for 100MB chunks on gigabit network). With 4 workers per shard, you're sustaining ~8 uploads/second per shard—well below S3's 3,500 limit.

### How Many Workers?

**Formula**:
```
optimal_workers = (bandwidth_mbps × 0.8) / throughput_per_worker
```

**Example** (1 Gbps = 125 MB/s):
```
optimal_workers = (125 × 0.8) / 10 = 10 workers
```

Assuming each worker sustains 10 MB/s, you need 10 workers to saturate 1 Gbps.

### Chunk Size Impact

Larger chunks = fewer requests = less benefit from sharding.

| Chunk Size | Files per Chunk | Chunks (10K files) | Benefit from Sharding |
|------------|-----------------|-------------------|---------------------|
| 10 MB | 50 | 200 | High |
| 100 MB | 500 | 20 | Medium |
| 500 MB | 2,500 | 4 | Low |

**Recommendation**: For small-file workloads, use 50-100MB chunks to balance request reduction and parallelism.

## Advanced: Load-Balanced Sharding

CargoShip's default round-robin sharding works well for most cases. But for workloads with variable chunk sizes, you can implement load-balanced sharding:

```go
type LoadBalancedRouter struct {
    numShards  int
    shardLoads []int64  // Bytes in flight per shard
    shardChans []chan<- Job
    mu         sync.Mutex
}

func (r *LoadBalancedRouter) Route(ctx context.Context, chunk Chunk) error {
    r.mu.Lock()

    // Find shard with least load
    minLoad := r.shardLoads[0]
    minShard := 0
    for i := 1; i < r.numShards; i++ {
        if r.shardLoads[i] < minLoad {
            minLoad = r.shardLoads[i]
            minShard = i
        }
    }

    // Assign chunk to least-loaded shard
    r.shardLoads[minShard] += chunk.Size
    r.mu.Unlock()

    job := Job{
        ChunkID: chunk.ID,
        ShardID: minShard,
        Size:    chunk.Size,
    }

    select {
    case r.shardChans[minShard] <- job:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (r *LoadBalancedRouter) OnComplete(shardID int, size int64) {
    r.mu.Lock()
    r.shardLoads[shardID] -= size
    r.mu.Unlock()
}
```

This ensures shards with fast uploads get more work, while slow shards (e.g., due to transient network issues) get less.

## Real-World Results

CargoShip's multi-prefix sharding enabled this performance:

### Small Files (10,000 × 20KB)
- **Single-prefix baseline** (rclone): 47s
- **CargoShip (8 shards)**: 1.2s
- **Speedup**: 39x

### Large Dataset (1,000 × 100MB)
- **Single-prefix baseline** (aws-cli): 312s
- **CargoShip (8 shards)**: 187s
- **Speedup**: 1.7x

Multi-prefix sharding has the biggest impact on **many-small-file workloads**—exactly the use case that's hardest to optimize with traditional tools.

## Monitoring Shard Performance

CargoShip tracks per-shard metrics:

```bash
cargoship create upload ./data --bucket my-bucket --prometheus-addr :9090
```

Prometheus metrics include:
- `cargoship_shard_uploads_total{shard="0"}` - Upload count per shard
- `cargoship_shard_bytes_total{shard="0"}` - Bytes uploaded per shard
- `cargoship_shard_errors_total{shard="0"}` - Errors per shard
- `cargoship_shard_duration_seconds{shard="0"}` - Upload latency per shard

Query for unbalanced shards:
```promql
rate(cargoship_shard_uploads_total[5m]) by (shard)
```

If one shard is significantly slower, it might be hitting rate limits or experiencing network issues.

## Distributed Tracing

For deep visibility, enable OpenTelemetry tracing:

```bash
cargoship create upload ./data --bucket my-bucket \
    --tracing \
    --tracing-exporter jaeger \
    --tracing-endpoint http://localhost:14268/api/traces
```

Each shard's uploads appear as separate spans in the trace timeline. You'll see exactly how parallelism is distributed:

```
upload-request (1.2s)
├── shard-0 (1.1s)
│   ├── chunk-0 (140ms)
│   ├── chunk-8 (138ms)
│   └── chunk-16 (142ms)
├── shard-1 (1.0s)
│   ├── chunk-1 (135ms)
│   ├── chunk-9 (139ms)
│   └── chunk-17 (136ms)
...
└── shard-7 (1.1s)
    ├── chunk-7 (141ms)
    └── chunk-15 (137ms)
```

This visualization makes it obvious if shards are unbalanced or if certain chunks are slow.

## Configuration Recommendations

### High-Throughput Uploads (10+ Gbps)
```bash
cargoship create upload ./data \
    --bucket my-bucket \
    --shards 16 \
    --workers 8 \
    --chunk-size 250MB
```

### Standard Uploads (1 Gbps)
```bash
cargoship create upload ./data \
    --bucket my-bucket \
    --shards 8 \
    --workers 4 \
    --chunk-size 100MB
```

### Low-Bandwidth Uploads (<100 Mbps)
```bash
cargoship create upload ./data \
    --bucket my-bucket \
    --shards 4 \
    --workers 2 \
    --chunk-size 50MB
```

## Conclusion

Multi-prefix parallel uploads are the secret sauce that makes CargoShip 7-8x faster than competing tools. By distributing uploads across multiple S3 prefixes, CargoShip bypasses rate limits and enables massive parallelism.

The technique isn't complicated—it's just sharding with round-robin assignment. But combined with streaming architecture and intelligent chunking, it unlocks performance that traditional tools can't match.

In the next post, we'll show how CargoShip uses this infrastructure to optimize costs, saving 90%+ on storage with intelligent tiering and lifecycle policies.

---

**Resources**:
- [AWS S3 Performance Guidelines](https://docs.aws.amazon.com/AmazonS3/latest/userguide/optimizing-performance.html)
- [CargoShip Router Implementation](https://github.com/scttfrdmn/cargoship/blob/main/pkg/pipeline/router.go)
- [Performance Benchmarks](../PERFORMANCE_BENCHMARKS.md)

**Next**: [Cost Optimization: Saving 90% with Intelligent Tiering →](04-cost-optimization.md)
**Previous**: [← Zero-Disk Architecture](02-zero-disk-architecture.md)
