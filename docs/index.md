# Overview

[![Go Reference](https://pkg.go.dev/badge/gitlab.oit.duke.edu/devil-ops/suitcasectl.svg)](https://pkg.go.dev/gitlab.oit.duke.edu/devil-ops/suitcasectl)
[![Latest Release](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/badges/release.svg)](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/releases)

SuitcaseCTL is a tool for packaging up research data in a standardized format (tar, tar.gz) with the following additional features:

* Optional GPG encryption, either on the archive itself, or the files within the archive
* Splitting a single directory in to multiple tar files. This allows for smaller files to be transported to the cloud, and allows for faster archive creation since the multiple archives are created in parallel
* “Inventory” file which contains hashes and locations of all files in the archives, along with optional metadata
* CLI metadata file that contains the options the archive was created wit

![demo](./vhs/demo.gif)
