# Benchmarking

`cargoship benchmark` measures how different compression algorithms and levels
perform on your data, so you can trade archive size against CPU time before
committing to a `--compression-level` for real uploads. It reports compression
ratio and throughput for gzip, zlib, zstd, lz4, and s2.

```bash
# Benchmark against generated data of a given size and type
cargoship benchmark --size 100MB --data-type text

# Benchmark against a real file (most representative)
cargoship benchmark --file /path/to/sample.tar

# Emit machine-readable results
cargoship benchmark --size 1GB --format json
```

## Options

- `--size` — size of generated test data, e.g. `1MB`, `10MB`, `1GB` (default
  `10MB`). Ignored when `--file` is set.
- `--data-type` — shape of generated data: `text`, `binary`, `mixed` (default), or
  `random`. Text is highly compressible; random is not, which brackets the range
  you'll see in practice.
- `--file` — benchmark a real file instead of generated data. Use this for the
  most representative result on your workload.
- `--format` — `table` (default, human-readable) or `json` for scripting.

## Reading the results

Table output lists each algorithm and level with the ratio it achieved and its
throughput. A run on compressible text data looks roughly like this:

```
Algorithm  Level  Ratio   Compress MB/s   Decompress MB/s
gzip       6      3.1x    95              420
zlib       6      3.1x    98              430
zstd       3      3.4x    480             1250
zstd       19     3.9x    24              1180
lz4        -      2.4x    720             2600
s2         -      2.6x    900             2400
```

Higher levels shrink the archive further but cost CPU: notice `zstd` at level 19
edges out level 3 on ratio while running an order of magnitude slower to compress.
`lz4` and `s2` prioritize speed over ratio. Your numbers will vary with data type
and hardware — always benchmark your own `--file` before picking a level for large
jobs. CargoShip uploads use Zstandard; the benchmark's cross-algorithm view helps
you judge how much a higher `--compression-level` is worth for your content.

## See also

- [Compression & content-aware](/guides/features/compression).
- [Performance tuning](/guides/features/optimization).
- Reference: [Diagnostics & utilities](/reference/commands/diagnostics).
