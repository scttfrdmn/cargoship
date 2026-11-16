# Performance tuning

CargoShip's defaults are tuned to be fast on most workloads — adaptive
[sharding](/guides/features/sharding), streaming [compression](/guides/features/compression),
and adaptive staging all work without configuration. This guide covers the knobs
worth reaching for when you're pushing a large or unusual workload, and how to
tell which one to turn.

::: tip Measure before you tune
Change one thing at a time and time it. The right settings depend on your link
speed, CPU, and file mix — a knob that helps a TB of video hurts a directory of
tiny logs. See [Benchmarking](/guides/features/benchmarking) for a repeatable
harness.
:::

## Find the bottleneck first

Every upload is limited by one of three things. Identify which before tuning:

- **Network-bound** — link is saturated, CPU idle. Compress *harder* to send
  less; more shards won't help.
- **CPU-bound** — cores pegged, link underused. Compress *lighter* so the
  pipeline can feed the network.
- **Request/latency-bound** — many tiny files, low throughput despite spare CPU
  and bandwidth. Let CargoShip pack files into chunks (it does this by default)
  and consider direct-upload mode for small batches.

Run with `--verbose` to see per-stage progress and spot which stage is starving
the others.

## Compression vs. bandwidth

The single biggest lever. `--compression-level` (zstd 1–22, default 3) trades CPU
for smaller uploads:

```bash
# Network-bound: send less
cargoship upload ./logs s3://my-bucket/archives/ --compression-level 12

# CPU-bound or already-compressed data: keep the pipeline fast
cargoship upload ./media s3://my-bucket/archives/ --compression-level 3
```

Content-aware selection already avoids wasting CPU on incompressible files, so
this mostly matters for uniformly text-heavy or uniformly pre-compressed
datasets. Full detail in [Compression](/guides/features/compression).

## Parallelism via shards

Upload parallelism comes from [multi-prefix sharding](/guides/features/sharding).
The count is adaptive (4–32) based on file count, data size, CPU, and memory —
leave it on auto unless benchmarking:

```bash
# Pin more shards for a large dataset on a fast link
cargoship upload ./big-dataset s3://my-bucket/archives/ --shard-count 24
```

More shards multiply S3 request headroom; they don't create bandwidth. If you're
already network-bound, adding shards does nothing.

## Direct-upload mode for small files

For batches of small files, archiving-and-compressing adds overhead that can cost
more than it saves. Direct-upload mode bypasses the archive/compression pipeline
and uploads objects directly with high worker concurrency. It kicks in
automatically under a size threshold:

```bash
# Tune the auto threshold (default 500 MB total)
cargoship upload ./many-small-files s3://my-bucket/data/ \
  --direct-upload-threshold-mb 1000

# Force it on regardless of size (benchmarking)
cargoship upload ./data s3://my-bucket/data/ --force-direct-upload
```

- `--direct-upload` — enable direct mode.
- `--direct-upload-threshold-mb` — max total size for *automatic* direct upload (default 500).
- `--direct-upload-workers` — worker count for direct mode (default 256).
- `--force-direct-upload` — force it on regardless of thresholds.

## Transporter and staging

The `--transporter` flag selects the upload engine; the default `staging` uses
adaptive staging to smooth throughput. On very memory-constrained machines you
can disable staging to trade throughput for a smaller footprint:

```bash
# Reduce memory use
cargoship upload ./data s3://my-bucket/archives/ --disable-staging

# Cap total memory (slower, avoids OOM on tight hosts)
cargoship upload ./data s3://my-bucket/archives/ --memory-limit 2GB
```

`--transporter` accepts `basic`, `staging` (default), `adaptive`, `optimized`, or
`none`. The high-level `--optimization` flag (on by default) enables the bundle
of adaptive behaviors; leave it on unless you're isolating a variable in a
benchmark.

## TCP congestion control on long or lossy links

`--congestion-control` sets the kernel's TCP congestion-control algorithm on
every S3 connection. This works on **Linux** only; on other platforms the OS
default applies and an explicit value prints a warning.

| Value | Effect |
|---|---|
| `auto` (default) | `bbr` if the kernel has it and this process may use it, otherwise the kernel default |
| `bbr` | BBR on every S3 socket, or a warning that names the reason and the fix |
| `cubic` | CUBIC on every S3 socket |

`upload` prints the algorithm in effect, for example
`TCP congestion control: bbr (requested auto)`.

**It matters most on a NAS or other host uploading over a long or lossy WAN.**
On a clean path the algorithm barely matters. With loss, CUBIC collapses: on a
+100 ms path with 0.1% packet loss, the sister project aws-elastic-gcs measured
**0.16 Gbps with CUBIC against 0.97 Gbps with BBR**.

To make BBR available to an unprivileged process, once per boot (or
persistently in `/etc/sysctl.d/`):

```bash
sudo modprobe tcp_bbr
sudo sysctl -w net.ipv4.tcp_allowed_congestion_control="reno cubic bbr"
# Optional host-wide tuning (ESnet fasterdata): BBR as default, fq pacing,
# and TCP buffers that can grow to 32 MB.
sudo sysctl -w net.core.default_qdisc=fq net.ipv4.tcp_congestion_control=bbr
sudo sysctl -w net.core.rmem_max=33554432 net.core.wmem_max=33554432
sudo sysctl -w net.ipv4.tcp_rmem="4096 87380 33554432" net.ipv4.tcp_wmem="4096 65536 33554432"
```

A container uses its host's kernel, so for Docker or a NAS app this is set on
the host. Within those limits, cargoship sets the algorithm per socket itself.

::: info Separate from upload pacing
`--optimization` (on by default) also runs an application-level pacer that
backs off when S3 returns `503 SlowDown`. That pacer is independent of the TCP
algorithm above. Before v0.41.0, `--congestion-control` only reached the pacer
and never changed TCP, despite what these docs said (#881).
:::

## Storage class is a cost knob, not a speed knob

Choosing `GLACIER_IR` or `DEEP_ARCHIVE` cuts storage cost but doesn't change
upload speed. Pick storage class for access pattern and cost — see
[Lifecycle & storage classes](/guides/cost/lifecycle) — and tune speed with the
levers above.

## Best practices

::: tip
- **Identify the bottleneck** (network / CPU / requests) before changing anything.
- **Network-bound? Compress harder. CPU-bound? Compress lighter.** Shards don't add
  bandwidth.
- **Leave shard count on auto** unless a benchmark says otherwise.
- **Use direct-upload for small-file batches** where archive overhead dominates.
- **`--disable-staging` / `--memory-limit`** are for tight hosts — they trade speed
  for safety, so only use them when memory is the constraint.
- **Change one variable at a time** and time each run; keep the settings that
  actually moved the number.
:::

## See also

- [Multi-prefix sharding](/guides/features/sharding) — how parallelism scales.
- [Compression](/guides/features/compression) — the main CPU-vs-bandwidth tradeoff.
- [Benchmarking](/guides/features/benchmarking) — measure the effect of a change.
- [Observability & tracing](/guides/features/observability) — find bottlenecks with metrics.
- Reference: [Uploading & sync commands](/reference/commands/upload).
