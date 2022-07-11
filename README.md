# Overview

[![pipeline status](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/badges/main/pipeline.svg)](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/-/commits/main)
[![coverage report](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/badges/main/coverage.svg)](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/-/commits/main)
[![Latest Release](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/-/badges/release.svg)](https://gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/-/releases)

## Installation

Download a package from the [releases](https://gitlab.oit.duke.edu/devil-ops/data-suitcase/-/releases) page, or use the [devil-ops package](https://gitlab.oit.duke.edu/devil-ops/installing-devil-ops-packages) for homebrew, yum, etc.

## Global Flags

`-v` or `--verbose` : Enable debug logs.

`-t` or `--trace` : Enable trace logs. This will include the file/line of the
code that generated a given message.

`--log-format` : Either `console` or `json` (Default: `console`).

## Inventory

Inventory is a yaml file that contains a list of files and hashes.

Inventory creation is done with the `create inventory` subcommand. Some useful
arguments are:

`--max-suitcase-size` : This will limit the maximum size of a suitcase. The size
is calculated based on the un-compressed and un-encrypted file sizes, so the
actual file sizes will have some variance. It may be useful to buffer this to
something larger or smaller than what you actually want, based on results you
are getting. If no unit is specified, defaults to bytes. Any unit supported by
[go-humanize](https://github.com/dustin/go-humanize) is supported here.

`--hash-inner` : This option will take a sha256 hash of every file inside the
suitcase, and store it in the inventory. This will allow for future integrity
checking, but will increase the time that it takes to generate the inventory.

`--encrypt-inner` : This option will tell the inventory to encrypt all of the
files at suitcase generation time.

A couple reasons for setting this:

* Cloud storage may have a maximum file size limit. This is a good way to limit
  the size of a suitcase.

* Multiple suitcases will be created in parallel, so this may speed up the total
  time needed to generate them.

### Metadata

Arbitrary metadata can be included in the inventory file as well. This allows
for data owners to add some special text describing the data. By default,
anything in one of the top level directories that matches the glob
`suitcase-meta*` will be included in the metadata. This is configurable with the
`--internal-metadata-glob 'new-glob*'` flag.

You can also include metadata outside of the target directories with
`--external-metadata-file /tmp/some-file.txt`. This argument can be used
multiple times.

### Generic Example

```bash
❯ suitcasectl create inventory ~/Desktop/example-suitcase/ --max-suitcase-size=3.5Mb -v  > /tmp/inventory.yaml
12:58PM INF walking directory dir=/Users/drews/Desktop/example-suitcase/
12:58PM INF index is full, adding new index numCases=1 path=/Users/drews/Desktop/example-suitcase/20220221_100626.jpeg size=225122
12:58PM INF Indexed inventory count=2
12:58PM INF Completed end="2022-07-05 08:58:05.952764 -0400 EDT m=+0.023972543" runtime=21.483957ms start="2022-07-05 08:58:05.93128 -0400 EDT m=+0.002488586"
```

## Suitcase

Suitcase is the giant blob (or blobs) of all the data

Suitcase creation is done with the `create suitcase` subcommand. Some useful arguments are:

`--format` : The format of the suitcase. This is a string that is used to
determine what sort of compression should be used, along with if it should be
encrypted. Valid formats are currently: `tar`, `tar.gz`, `tar.pgp` and
`tar.gz.pgp`. Default is `tar.gz`

`--concurrency` : How many archive files to write at a time.

`--hash-outer` : This will do a final sha256 hash of the suitcase, and store it
in a ${suitcase}.sha256 file.

`--exclude-systems-pubkeys` : Don't include the systems team public keys when
doing encryption.

`-p` or `--public-key` : Armored public pgp files to use for encryption. Can be
specified multiple times.

```bash
❯ suitcasectl create suitcase -i /tmp/inventory.yaml /tmp/  --concurrency=10 --format=.tar
1:13PM INF Filling suitcase destination=/tmp/suitcase-1.tar encryptInner=false format=tar index=1
1:13PM INF Filling suitcase destination=/tmp/suitcase-2.tar encryptInner=false format=tar index=2
1:13PM INF Created file file=/tmp/suitcase-2.tar
1:13PM INF Created file file=/tmp/suitcase-1.tar
1:13PM INF Completed end="2022-07-05 09:13:02.518938 -0400 EDT m=+0.065861238" runtime=63.473358ms start="2022-07-05 09:13:02.455463 -0400 EDT m=+0.002387880"
```

Encryption is optional. Encryption and compression will be based on the target file extension.

Help available on all the command with `-h` or `--help`
