# Overview

[![Go Reference](https://pkg.go.dev/badge/gitlab.oit.duke.edu/devil-ops/suitcasectl.svg)](https://pkg.go.dev/gitlab.oit.duke.edu/devil-ops/suitcasectl)
[![pipeline status](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/pipeline.svg)](https://gitlab.oit.duke.edu/devil-ops/data-suitcase/-/commits/main)
[![coverage report](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/coverage.svg)](https://gitlab.oit.duke.edu/devil-ops/data-suitcase/-/commits/main)
[![Latest Release](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/badges/release.svg)](https://gitlab.oit.duke.edu/devil-ops/data-suitcase/-/releases)

SuitcaseCTL is a tool for packaging up research data in a standardized format (tar, tar.gz) with the following additional features:

* Optional GPG encryption, either on the archive itself, or the files within the archive
* Splitting a single directory in to multiple tar files. This allows for smaller files to be transported to the cloud, and allows for faster archive creation since the multiple archives are created in parallel
* “Inventory” file which contains hashes and locations of all files in the archives, along with optional metadata
* CLI metadata file that contains the options the archive was created with

## Installation

### Prebuilt Binaries

Prebuilt binaries are the preferred and easiest way to get suitcasectl on your
host. If there is no available prebuilt option for your OS, please [create a new
issue](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/issues/new) and we'll
get it in there!

Download a package from the
[releases](https://gitlab.oit.duke.edu/devil-ops/data-suitcase/-/releases) page,
or use the [devil-ops
package](https://gitlab.oit.duke.edu/devil-ops/installing-devil-ops-packages)
for homebrew, yum, etc.

### Local builds

You can also use `go install` to download and build the latest commits to `main` (Or any other branch/tag)

```bash
$ go install gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl@main
...
```

## Global Flags

`-v` or `--verbose` : Enable debug logs.

`-t` or `--trace` : Enable trace logs. This will include the file/line of the
code that generated a given message.

`--log-format` : Either `console` or `json` (Default: `console`).

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
❯ suitcasectl create suitcase ~/Desktop/example-suitcase/ -o /tmp --suitcase-format ".tar.gpg" --max-suitcase-size="3.5Mb" --user=foo
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
