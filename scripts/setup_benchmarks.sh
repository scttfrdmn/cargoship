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