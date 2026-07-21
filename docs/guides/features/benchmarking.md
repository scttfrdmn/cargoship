# Benchmarking

`cargoship benchmark` measures how different compression algorithms and levels
perform on your data, so you can trade off archive size against CPU time before
committing to a `--compression-level` for real uploads. It reports throughput and
resulting size for algorithms such as gzip, zlib, zstd, lz4, and s2.

```bash
cargoship benchmark --size 100MB --data-type text
cargoship benchmark --file /path/to/sample.tar --format json
```

Run it against generated data of a given `--size` and `--data-type` (text, binary,
mixed, random), or against a real `--file` for the most representative result.

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Diagnostics & utilities command reference](/reference/commands/diagnostics).
:::

## See also

- [Compression & content-aware](/guides/features/compression).
- [Performance tuning](/guides/features/optimization).
- Reference: [Diagnostics & utilities](/reference/commands/diagnostics).
