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

## Round-trip speed + cost harness (`cargoship-bench`)

The Go microbenchmarks above measure the upload code path in isolation. The
`cargoship-bench` harness measures the **end-to-end** picture on real S3: it
plants a reproducible corpus, uploads it through the real pipeline, restores it,
verifies the round-trip is **byte-identical**, and reports throughput plus the
full S3 **dollar** cost computed from **exact, middleware-counted** requests —
not estimates.

```bash
# All reproducible profiles against a real bucket (writes objects; clean up after)
cargoship-bench --bucket my-bucket --region us-west-2

# One profile, forcing the upload path (auto|direct|packed), JSON output
cargoship-bench --bucket my-bucket --region us-west-2 --profile mixed --mode packed --json

# Parametric corpus: N uniform files of a fixed size (the crossover sweep)
cargoship-bench --bucket my-bucket --region us-west-2 --files 1000 --file-size 65536 --mode direct
```

It leaves its objects in place (delete them yourself, as with the real-AWS
torture procedure). Point `--endpoint` at an emulator for a free correctness run;
emulator numbers validate the round-trip but are meaningless for throughput.

### Reproducible corpora

Each named profile seeds its own RNG from a fixed seed, so it plants
byte-identical content on every run and machine — the property that lets a
published number be independently reproduced:

| Profile | Shape | Exercises |
|---|---|---|
| `many-tiny` | 5000 files, ~2 KB each | request/small-object overhead |
| `few-large` | 4 files, 6–20 MiB | multipart uploads |
| `mixed` | compressible + already-compressed | both chunk kinds in one upload |
| `hostile` | awkward names/sizes/nesting | edge cases, direct-upload path |

### What the numbers mean

- **Effective throughput** = source bytes moved per second (`source_bytes ÷
  wall-clock`), crediting compression — the figure comparable to a raw 1:1 mover,
  not wire bytes. Reported for both the upload and restore legs.
- **Request counts** are captured by an aws-sdk middleware that tallies each
  logical S3 operation (including every multipart `UploadPart`), so the request
  bill is measured, not derived from the manifest.
- **Cost** is modeled at the STANDARD class: **upload data transfer is free** (S3
  ingress), monthly storage is billed on the stored (compressed) bytes, and
  restore is GET-tier requests + egress ($0.09/GB) on the bytes that physically
  leave S3.

### Recorded baseline (2026-09-10, macOS → `us-west-2`, STANDARD)

Provenance matters as much here as for the microbenchmarks — these are one
machine's numbers on one network path, recorded for reference, not a portable
"CargoShip does X MB/s" claim:

| Profile | Files / size | Upload MB/s | Restore MB/s |
|---|---|---|---|
| `few-large` | 4 / 46 MiB | 38.4 | 66.0 |
| `mixed` | 4 / 24 MiB | 24.8 | 46.0 |

### Direct vs. packed: the crossover sweep (#466)

CargoShip auto-selects a direct-upload fast path (one S3 object per file) for
small datasets and packs everything else into chunks. Sweeping file count at a
fixed 64 KB file size, direct vs. forced-packed on real S3:

| Files | Upload MB/s (direct / packed) | Restore MB/s (direct / packed) | Upload requests (direct / packed) |
|---|---|---|---|
| 50 | 8.7 / 6.3 | 1.2 / 12.2 | 51 / 2 |
| 500 | 61.2 / 30.2 | 1.2 / 57.2 | 501 / 2 |
| 1000 | 71.2 / 45.0 | 1.2 / 68.5 | 1001 / 2 |
| 2000 | 72.6 / 44.9 | 1.1 / 67.3 | 2001 / 3 |

There is no single-axis crossover: **direct uploads faster** for medium-small
files (parallel PUTs, no archive CPU), but **packing restores far faster and
costs far fewer requests** at every count. Direct restore is a flat ~1.2 MB/s
because [`BatchRestore` downloads serially](https://github.com/scttfrdmn/cargoship/issues/472);
packing amortizes that across a few chunk GETs. CargoShip therefore packs once a
dataset exceeds a conservative file-count cap (`DirectUploadMaxFiles`, default
1000), reserving direct upload for genuinely small sets — see
[#466](https://github.com/scttfrdmn/cargoship/issues/466).

::: warning "Fastest" and "cheapest" are goals, not verified claims
These are CargoShip's own numbers. Head-to-head comparisons against `aws s3 cp`,
`s5cmd`, and `rclone` on the same corpus/bucket/hardware are future work; until
that harness exists, the [capability matrix](/project/verification) marks
performance and cost **Aspirational**.
:::

## See also

- [Performance tuning](/guides/features/optimization) — the throughput knobs
  (concurrency, chunk size, sharding) these benchmarks exercise
- [CargoShip vs. other tools](/reference/comparison) — when CargoShip is and isn't
  the right choice
- [Sharding](/guides/features/sharding) — how multi-prefix parallelism affects
  request-rate ceilings
