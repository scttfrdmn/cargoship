# Benchmarks & methodology

CargoShip ships a repeatable benchmark suite so that any performance claim can be
reproduced on your own hardware. This page documents **how** the numbers are
produced — the methodology, the scenarios, and the provenance recorded with every
run — so results are never quoted without the context that makes them meaningful.

::: info No headline numbers here
Throughput depends heavily on your network path to S3, instance type, file mix,
and region. Rather than publish a single "X MB/s" figure that won't match your
environment, this page tells you how to measure it yourself. Run the suite and
read the provenance header for the machine, commit, and settings behind each
number.
:::

## Running the suite

From the repository root:

```bash
# Full run: 3s per benchmark, 5 iterations, with CPU + memory profiles
scripts/run-benchmarks.sh

# Quick run for iteration (1s, 3 iterations)
scripts/run-benchmarks.sh --short

# Long run for publishable numbers (10s, 10 iterations)
scripts/run-benchmarks.sh --long

# Skip profiling (faster, smaller output)
scripts/run-benchmarks.sh --no-profile
```

Each run writes a timestamped report to `benchmark-reports/benchmark-<timestamp>.txt`
and, unless `--no-profile` is set, CPU and memory profiles to `profiles/`.

You can tune the run with environment variables:

| Variable | Default | Meaning |
|---|---|---|
| `BENCH_TIME` | `3s` | Wall-clock duration per benchmark |
| `BENCH_COUNT` | `5` | Iterations per benchmark (for `benchstat`) |
| `AWS_REGION` | `us-west-2` | Region for S3 integration benchmarks |
| `BENCHMARK_BUCKET` | `cargoship-benchmark-test` | Bucket for S3 integration benchmarks |

## Provenance: every report is reproducible

The runner prepends a provenance header to each report recording exactly what
produced the numbers:

```
# CargoShip benchmark report
#
# Generated:      20260722-1030
# Git commit:     a4b8872b081ca9a16a76c28ec5de330594250535
# Go version:     go1.26.5
# OS / arch:      Darwin / arm64
# CPU:            Apple M4 Pro (12 cores)
# Memory:         49152 MB
# Bench time:     3s × 5 iterations
# Benchmark dir:  ./pkg/benchmarks/scenarios
#
# Reproduce:      BENCH_TIME=3s BENCH_COUNT=5 scripts/run-benchmarks.sh
# Methodology:    docs/reference/benchmarks.md
```

If the working tree has uncommitted changes, the commit line is marked
`(dirty — uncommitted changes present)` — a signal that the result does not
correspond to any published commit and should not be quoted as a release number.

**When publishing a benchmark result, always include this header.** A throughput
figure without its machine, commit, and settings is not reproducible and should
be treated as anecdote, not measurement.

## What the suite measures

The upload benchmarks live in `pkg/benchmarks/scenarios/`:

| Benchmark | File sizes | Focus |
|---|---|---|
| `BenchmarkSmallFileUpload` | 1–10 MB | Request overhead and latency (single-part) |
| `BenchmarkMediumFileUpload` | 10–100 MB | Multipart efficiency at low concurrency |
| `BenchmarkLargeFileUpload` | 100 MB–1 GB | Throughput at high concurrency |
| `BenchmarkXLFileUpload` | > 1 GB | Sustained multipart throughput |
| `BenchmarkMemoryEfficiency` | — | Bounded memory under streaming |
| `BenchmarkConcurrencyScaling` | — | Throughput vs. worker count |
| `BenchmarkChunkSizeImpact` | — | Throughput vs. chunk size |

Each is run with `-benchmem`, so the report includes allocations and bytes per
operation alongside wall-clock time — memory behavior is a first-class result, not
an afterthought.

## Comparing runs and detecting regressions

Because runs use `-count`, results are compatible with
[`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat):

```bash
# Save a baseline, make a change, then compare
scripts/run-benchmarks.sh --long   # → benchmark-reports/benchmark-<A>.txt
# ... make your change ...
scripts/run-benchmarks.sh --long   # → benchmark-reports/benchmark-<B>.txt

benchstat benchmark-reports/benchmark-<A>.txt benchmark-reports/benchmark-<B>.txt
```

`benchstat` reports the mean, variation, and whether the delta is statistically
significant — the right way to judge a change rather than eyeballing two raw
numbers. Compare only runs whose provenance headers show the **same machine, Go
version, and settings**; a faster laptop is not a faster CargoShip.

## Reading a result honestly

- **Local S3 emulator vs. real S3.** The default suite exercises the upload code
  path; it is not a measurement of your network throughput to AWS. For real-world
  numbers, run against a real bucket in your target region with credentials
  configured, and note the region in the report.
- **Warm vs. cold.** The first iteration pays setup costs (connection pools, TLS
  handshakes). `-count` and `benchstat` smooth this out; a single run does not.
- **File mix matters.** Compression ratio and per-file overhead depend entirely on
  content. A tree of already-compressed media behaves nothing like a tree of
  source code or CSV. Benchmark with a sample that resembles your real data.

## See also

- [Performance tuning](/guides/features/optimization) — the throughput knobs
  (concurrency, chunk size, sharding) these benchmarks exercise
- [CargoShip vs. other tools](/reference/comparison) — when CargoShip is and isn't
  the right choice
- [Sharding](/guides/features/sharding) — how multi-prefix parallelism affects
  request-rate ceilings
