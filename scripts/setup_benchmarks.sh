#!/bin/bash
set -e
set -x

if [[ ! -d benchmark_data ]]; then
  mkdir benchmark_data/
fi
cd benchmark_data/
if [[ ! -d American-Gut ]]; then
  git clone --depth=1  https://github.com/biocore/American-Gut
fi

if [[ ! -d BBBC005_v1_images ]]; then
  wget -q https://data.broadinstitute.org/bbbc/BBBC005/BBBC005_v1_images.zip
  unzip -q BBBC005_v1_images.zip
  rm -f BBBC005_v1_images.zip
fi