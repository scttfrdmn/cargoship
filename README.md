# Overview

[![Go Reference](https://pkg.go.dev/badge/gitlab.oit.duke.edu/devil-ops/suitcasectl.svg)](https://pkg.go.dev/gitlab.oit.duke.edu/devil-ops/suitcasectl)
[![pipeline status](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/pipeline.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/commits/main)
[![coverage report](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/coverage.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/commits/main)
[![Latest Release](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/badges/release.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/releases)

SuitcaseCTL is a tool for packaging up research data in a standardized format (tar, tar.gz) with the following additional features:

* Optional GPG encryption, either on the archive itself, or the files within the archive
* Splitting a single directory in to multiple tar files. This allows for smaller files to be transported to the cloud, and allows for faster archive creation since the multiple archives are created in parallel
* “Inventory” file which contains hashes and locations of all files in the archives, along with optional metadata
* CLI metadata file that contains the options the archive was created with

Full documentation [here](https://devil-ops.pages.oit.duke.edu/suitcasectl/)

![demo](./vhs/demo.gif)

## Get Suitcasectl

* [Installation Instructions](https://devil-ops.pages.oit.duke.edu/suitcasectl/install/)

## Documentation

All documentation is hosted [here](https://devil-ops.pages.oit.duke.edu/suitcasectl/)

## Components

When creating a new suitcase, several components are generated. Each is describe
below.

### Inventory

Inventory is a yaml or json file that contains a list of files that will be
added to the suitcase, along with some additional metadata around them.

This will be a human readable file that can be used for future file retrieval.
It is also used to generate the actual suitcase archive files.

#### Inventory Metadata

Arbitrary metadata can be included in the inventory file as well. This allows
for data owners to add some special text describing the data. By default,
anything in one of the top level directories that matches the glob
`suitcase-meta*` will be included in the metadata. This is configurable with the
`--internal-metadata-glob 'new-glob*'` flag.

You can also include metadata outside of the target directories with
`--external-metadata-file /tmp/some-file.txt`. This argument can be used
multiple times.

### CLI Metadata

This is a yaml file that contains information about what was passed to the CLI.
It will be created as `cli-invocation-meta.yaml` in the output directory.

### CLI Output

The console output from the `suitcasectl` app will also be logged in json
format to the directory containing the suitcases. This will be in a file called
`suitcasectl.log`.

### Hash Files

If `--hash-outer` or `--hash-inner` are specified on the CLI, hash files will be
generated alongside everything else.

`--hash-outer` will generate hashes on the suitcases and the metadata files.

`--hash-inner` will generate hashes on the files inside of the suitcases.

### Suitcases

Suitcases are the giant blob (or blobs) of all the data

Suitcase creation is done with the `create suitcase` subcommand. Some useful
arguments are:

### Generic Example

```bash
❯ suitcasectl create suitcase ~/Desktop/example-suitcase/ -d /tmp --suitcase-format ".tar.gpg" --max-suitcase-size="3.5Mb" --user=foo
5:35PM INF No inventory file specified, we're going to go ahead and create one
5:35PM WRN Skipping file hashes. This will increase the speed of the inventory, but will not be able to verify the integrity of the files.
5:35PM INF walking directory dir=/Users/drews/Desktop/example-suitcase/
5:35PM INF Finished walking directory files=10
5:35PM INF index is full, adding new index numCases=1 path=/Users/drews/Desktop/example-suitcase/20220221_100626.jpeg size=225122
5:35PM INF Now that we have the inventory, sub in the real suitcase names
5:35PM INF Memory Usage in MB allocated=725352 allocated-percent=100 gc-count=0 system=8735760 total-allocated=725352
5:35PM INF Created inventory file file=/tmp/inventory.yaml
5:35PM INF Individual Suitcase Data file-count=8 file-size=3386722 file-size-human="3.4 MB" index=1
5:35PM INF Individual Suitcase Data file-count=2 file-size=225990 file-size-human="226 kB" index=2
5:35PM INF Total Suitcase Data file-count=10 file-size=3612712 file-size-human="3.6 MB"
5:35PM INF Cloned GPG keys from Git subdir=linux url=https://gitlab.oit.duke.edu/oit-ssi-systems/staff-public-keys.git
5:35PM INF Filling suitcase destination=/tmp/suitcase-foo-02-of-02.tar.gpg encryptInner=false format=tar.gpg index=2
5:35PM INF Filling suitcase destination=/tmp/suitcase-foo-01-of-02.tar.gpg encryptInner=false format=tar.gpg index=1
5:35PM INF Created file file=/tmp/suitcase-foo-02-of-02.tar.gpg
5:35PM INF Created file file=/tmp/suitcase-foo-01-of-02.tar.gpg
5:35PM INF Created CLI meta file meta-file=/tmp/cli-invocation-meta.yaml
5:35PM INF Completed end="2022-07-19 13:35:39.077167 -0400 EDT m=+0.473826130" runtime=471.334499ms start="2022-07-19 13:35:38.605818 -0400 EDT m=+0.002491631"
```

For more information on the available arguments, run `suitcasectl create suitcase --help`.

### Find where a suitcase lives

Given an inventory, you can search across files.

This will return both files that contain the pattern, and directory summaries containing the pattern, example:

```bash
$ suitcasectl find SOME_PATTERN
files:
    - path: /Users/drews/Desktop/Almost Garbage/godoc/src/runtime/cgo/libcgo_windows.h?m=text
      destination: godoc/src/runtime/cgo/libcgo_windows.h
      name: libcgo_windows.h
      size: 258
      suitcase_index: 5
      suitcase_name: suitcase-drews-05-of-05.tar.zst
...
directories:
    - directory: godoc/lib
      totalsize: 139186
      totalsizehr: 139 kB
      suitcases:
        - suitcase-drews-04-of-05.tar.zst
        - suitcase-drews-05-of-05.tar.zst
...
```

By default, suitcasectl will recursively search the current directory for yaml
inventories. To specify other directories, use the `--inventory-directory` flag.
This flag can be specified multiple times for multiple directories.

## Advanced

### Custom Suitcase Settings File

Users can include a `suitcasectl.[yaml|toml|json]` file in the root of their
directory which will automatically be used by suitcasectl for certain options.
This could be useful for things like ignore patterns or other settings.

Example:

```yaml
ignore-glob:
  - "*.out"
  - "*.swp"
```

Under the covers, the [viper](https://github.com/spf13/viper) library is doing
this. Their documentation will be useful when tracking down issues.

### Inventory Schema

We will try to keep the inventory as standardized as possible, with that said,
we are tracking schema changes inside of `pkg/static/schemas`. These files
represent the schema at a given time. This can be generated using a hidden
subcommand: `suitcasectl schema > pkg/static/schemas/YYYY-MM-DD.json`

### CLI Completion

If you are using either our homebrew or deb/rpm packages, command line
completion is enabled by default. If you aren't using one of these packages, you
can enable completion using `suitcasectl completion ...`. See `suitcasectl
completion $YOUR_SHELL -h` for help on your specific shell.

### Copy to the Cloud ☁️

Suitcasectl currently supports cloud destinations that are supported by the
[rclone](https://github.com/rclone/rclone) software library. While you don't
need to have Rclone installed, you will need to have your destination defined
in `~/.config/rclone/rclone.conf`, or using environment variables.

For example, if you have:

```shell
❯ cat ~/.config/rclone/rclone.conf
[suitcasectl-azure]
type = azureblob
account = suitcasectltesting
key = your-key
```

You can copy your files with:

```shell
❯ suitcasectl rclone ~/my-suitcases/ suitcasectl-azure:/test/
...
```

To pull the destination information from environment variables, use the following:

```shell
export RCLONE_CONFIG_MYCLOUD_TYPE=azureblob
export RCLONE_CONFIG_MYCLOUD_ACCOUNT=suitcasectltesting
export RCLONE_CONFIG_MYCLOUD_KEY=your-key
❯ suitcasectl rclone ~/my-suitcases/ my-cloud:/test/
...

```

Or you can copy them up as they are created with the `--cloud-destination`
flag:

```shell
❯ suitcasectl create suitcase ~/example-directory/ --cloud-destination suitcasectl-azure:/test/
...
```

To hopefully prevent later errors, the `--cloud-destination` option also checks
to ensure the destination already exists. If it does not, the command will fail
before anything is created. Using this option also uploads all relevant
metadata that is created with the suitcases.

### Shell Script Execution

Use `--shell-destination=$YOUR_SHELL_SCRIPT`

The file to copy will be accessible with the `$SUITCASECTL_FILE` variable, and can be used in a script like this:

```bash
#!/usr/bin/env bash

if [[ -z "${SUITCASECTL_FILE}" ]]; then
    echo "must set SUITCASECTL_FILE" before running 1>&2
    exit 2
fi

if [[ ! -e "${SUITCASECTL_FILE}" ]]; then
    echo "SUITCASECTL_FILE must be a file" 2>&2
    exit 3
fi

rsync -va "${SUITCASECTL_FILE}" foo:/bar/
```

### Performance and Benchmarking

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
