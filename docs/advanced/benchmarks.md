# Benchmarking and Profiling

Benchmarking can be done in a number of ways. You can run our built in Go
benchmarks using commands like this:

```bash
❯ BENCHMARK_DATA_DIR=/Users/joeuser/src/gitlab.oit.duke.edu/devil-ops/suitcasectl/benchmark_data/ /usr/local/bin/go test -benchmem -run=^$ -bench ^BenchmarkCalculateHashes$ gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmd
goos: darwin
goarch: amd64
pkg: gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmd
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
BenchmarkCalculateHashes/suitcase_calculate_hashes_md5-16                      1        1881289818 ns/op         1436832 B/op       3368 allocs/op
BenchmarkCalculateHashes/suitcase_calculate_hashes_sha1-16                     1        1465278938 ns/op         1453968 B/op       3368 allocs/op
BenchmarkCalculateHashes/suitcase_calculate_hashes_sha256-16                   1        3053716076 ns/op         1469664 B/op       3350 allocs/op
BenchmarkCalculateHashes/suitcase_calculate_hashes_sha512-16                   1        2208864876 ns/op         1549440 B/op       3368 allocs/op
PASS
ok      gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmd   8.835s
```

You may also generate a CPU Profile using the `--profile` option of suitcasectl.
This will generate a new profile to a temp directory, that you can then run `go
tool pprof FILE` to analyze.
