# SuitcaseCTL

> Package and inventory your data, then send it to the cloud!

## Overview

[![Go Reference](https://pkg.go.dev/badge/gitlab.oit.duke.edu/devil-ops/suitcasectl.svg)](https://pkg.go.dev/gitlab.oit.duke.edu/devil-ops/suitcasectl)
[![pipeline status](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/pipeline.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/commits/main)
[![coverage report](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/badges/main/coverage.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/commits/main)
[![Latest Release](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/badges/release.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/releases)

SuitcaseCTL is a tool for packaging up research data in a standardized format (tar, tar.gz) with the following additional features:

* Optional [GPG encryption](https://devil-ops.pages.oit.duke.edu/suitcasectl/advanced/gpg_encryption/), either on the archive itself, or the files within the archive
* Splitting a single directory in to multiple tar files (known as [Suitcases](https://devil-ops.pages.oit.duke.edu/suitcasectl/components/suitcase/)). This allows for smaller files to be transported to the cloud, and allows for faster archive creation since the multiple archives are created in parallel
* [Inventory](https://devil-ops.pages.oit.duke.edu/suitcasectl/components/inventory/) file which contains hashes and locations of all files in the archives, along with optional metadata
* [CLI metadata](https://devil-ops.pages.oit.duke.edu/suitcasectl/components/cli_metadata/) file that contains the options the archive was created with

![demo](./vhs/demo.gif)

## Get Suitcasectl

* [Installation Instructions](https://devil-ops.pages.oit.duke.edu/suitcasectl/install/)

## QuickStart

```bash
$ suitcasectl create suitcase $YOUR_DATA --destination=/srv/cold-storage/ --max-suitcase-size=50G
...
```

## Documentation

Check out the [Official SuitcaseCTL Documentation](https://devil-ops.pages.oit.duke.edu/suitcasectl/)!
