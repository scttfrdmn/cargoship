#!/usr/bin/env bash
# Compile-check every custom build tag in the repository.
#
# Why this exists (#329): two tagged test files — pkg/aws/s3/stress_test.go
# (`performance`) and pkg/pipeline/benchmark_test.go (`benchmark`) — had been
# broken on main for an unknown length of time. One called a helper that lived
# behind a different, mutually exclusive tag; the other called a method on a
# field whose type had since widened to interface{}. Neither was noticed because
# no CI lane set either tag, and absence of a signal reads exactly like success.
#
# These suites need real AWS credentials to *run*, so this does not run them. It
# only proves they still COMPILE, which is the part that was regressing.
#
# The tag list is DERIVED from the tree rather than hardcoded. A new tag is
# therefore covered the moment it appears — a hardcoded list would rot the same
# way the files did.
#
# Usage: scripts/ci/check-build-tags.sh
# Exit 0 = every tag compiles; exit 1 = at least one does not.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

# Tags handled by the toolchain itself (GOOS/GOARCH and friends). Setting these
# via -tags is meaningless-to-harmful, and cross-platform files are already
# compile-checked by the normal build on their own platform.
is_platform_tag() {
  case "$1" in
    linux|darwin|windows|freebsd|netbsd|openbsd|dragonfly|solaris|aix|plan9|android|ios) return 0 ;;
    js|wasm|wasip1|unix|cgo|race|msan|asan|purego|gc|gccgo) return 0 ;;
    *) return 1 ;;
  esac
}

# Collect identifiers from every //go:build line in tracked Go source. The
# expression grammar allows && || ! and parens; splitting on those leaves bare
# tag names. Negated tags (e.g. `!integration`) yield the same identifier, which
# is what we want — a file excluded by a tag is built when the tag is unset, and
# the unset case is covered by the untagged build below.
mapfile -t tags < <(
  git grep -h '^//go:build ' -- '*.go' \
    | sed 's|^//go:build ||' \
    | tr '()!' '   ' \
    | sed 's/&&/ /g; s/||/ /g' \
    | tr -s '[:space:]' '\n' \
    | grep -E '^[a-z][a-z0-9_.]*$' \
    | sort -u
)

custom=()
for t in "${tags[@]}"; do
  is_platform_tag "$t" || custom+=("$t")
done

if [ "${#custom[@]}" -eq 0 ]; then
  echo "no custom build tags found — nothing to check"
  exit 0
fi

echo "custom build tags discovered: ${custom[*]}"
echo

fail=0

# Baseline: the default build. Cheap, and it means a failure below is
# unambiguously tag-specific rather than a broken tree.
echo "→ go vet ./...  (no tags)"
if ! out="$(go vet ./... 2>&1)"; then
  echo "::error::go vet failed with no build tags set"
  echo "$out"
  fail=1
fi

for t in "${custom[@]}"; do
  echo "→ go vet -tags $t ./..."
  if ! out="$(go vet -tags "$t" ./... 2>&1)"; then
    echo "::error::build tag '$t' does not compile; tagged files are not built by any"
    echo "::error::other lane, so this rot is invisible until something sets the tag."
    echo "$out"
    fail=1
  fi
done

echo
if [ "$fail" -eq 0 ]; then
  echo "✅ build tags: default build plus ${#custom[@]} custom tag(s) all compile."
fi
exit $fail
