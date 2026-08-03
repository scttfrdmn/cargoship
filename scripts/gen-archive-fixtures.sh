#!/usr/bin/env bash
# Generate cross-version archive fixtures from ACTUAL RELEASED BINARIES (#322).
#
# The point of these fixtures is to test backward readability: an archive written
# by an older CargoShip must still restore under current code. That only means
# something if the archive was produced by the OLD code. Reconstructing one with
# today's writer would share today's bugs and pass vacuously — so this script
# downloads the real published binary for each version from GitHub Releases and
# has IT do the writing.
#
# What it produces, per version, under tests/e2e/testdata/archives/v<X.Y.Z>/:
#   direct/   — the direct-upload layout (files stored as individual objects)
#   chunked/  — the sharded tar.zst layout
# Each is a verbatim copy of the S3 object tree, so the readability test can
# serve it back from the emulator exactly as it was written.
#
# Usage:
#   bash scripts/gen-archive-fixtures.sh                 # all default versions
#   bash scripts/gen-archive-fixtures.sh v0.19.0         # just one (at release)
#
# Add a call for the new version at each release; the readability test picks up
# any directory it finds, so no test edit is needed.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT" || exit 1

# Versions to capture. v0.14.0 is the oldest kept: it is the first release with
# the current manifest v2.0 writer, so it is the oldest archive shape the format
# spec still promises to read.
DEFAULT_VERSIONS=(v0.14.0 v0.15.0 v0.16.1 v0.17.1 v0.18.0)

if [ "$#" -gt 0 ]; then
  VERSIONS=("$@")
else
  VERSIONS=("${DEFAULT_VERSIONS[@]}")
fi

command -v gh >/dev/null || { echo "gh CLI required" >&2; exit 1; }
command -v go >/dev/null || { echo "go required" >&2; exit 1; }

# Host platform, to pick the right release asset.
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=x86_64 ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

bindir="$(mktemp -d)"
trap 'rm -rf "$bindir"' EXIT

echo "Fetching released binaries for ${os}/${arch}..."
for v in "${VERSIONS[@]}"; do
  bare="${v#v}"
  asset="cargoship_${bare}_${os}_${arch}.tar.gz"
  echo "  $v → $asset"
  gh release download "$v" -p "$asset" -D "$bindir" --clobber
  mkdir -p "$bindir/$v"
  tar xzf "$bindir/$asset" -C "$bindir/$v"
  # Sanity-check: the binary must report the version we asked for. A silent
  # asset mismatch would produce fixtures mislabelled by version, which is worse
  # than having none.
  got="$("$bindir/$v/cargoship" --version 2>&1 | awk '{print $1}')"
  if [ "$got" != "$bare" ]; then
    echo "::error::$v binary reports version '$got', expected '$bare'" >&2
    exit 1
  fi
done

echo
echo "Driving each binary against the emulator to capture archives..."

# The capture itself needs an S3 endpoint and the SDK, so it lives in Go beside
# the emulator harness. CARGOSHIP_FIXTURE_BINDIR tells it where the binaries are.
CARGOSHIP_FIXTURE_BINDIR="$bindir" \
CARGOSHIP_FIXTURE_VERSIONS="${VERSIONS[*]}" \
  go test -tags e2e ./tests/e2e/ -run TestGenerateArchiveFixtures -v -timeout 15m

echo
echo "Fixtures written under tests/e2e/testdata/archives/. Commit them, then run:"
echo "  go test -tags e2e ./tests/e2e/ -run TestCrossVersionArchiveReadability -v"
