#!/bin/sh
set -e
rm -rf manpages
mkdir manpages
go run ./cmd/suitcasectl man | gzip -c -9 >manpages/suitcasectl.1.gz
