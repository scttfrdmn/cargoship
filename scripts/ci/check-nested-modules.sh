#!/usr/bin/env bash
# Verify every nested module still resolves against the root module.
#
# Why this exists: the modules under examples/library-usage/ and benchmarks/ each
# `replace github.com/scttfrdmn/cargoship => ../../..`, so they compile against
# the working tree rather than a published version. Their own `// indirect` pins
# must therefore stay consistent with the ROOT go.mod. Dependabot only watches
# `/` for gomod (.github/dependabot.yml), so a root bump moves the root and
# leaves the nested modules behind.
#
# The failure that motivated this: bumping klauspost/compress, prometheus/
# client_golang, or the aws-sdk group broke four example modules at once, and the
# only lane that noticed was the govulncheck step in security.yml — which reported
# it as
#
#     govulncheck: loading packages: err: exit status 1: stderr:
#     go: updates to go.mod needed; to update it: go mod tidy
#
# i.e. a *vulnerability check* failing for a reason that has nothing to do with
# vulnerabilities, on a module that contains no product code. That is an
# expensive thing to diagnose from a red X, and it fires on the dependency PRs
# where a red X is most likely to be waved through as noise.
#
# This checks the same property directly and says so in one line. It uses
# `go list` in the default read-only mode, which is exactly what CI does: the
# point is to fail when go.mod would need editing, not to edit it.
#
# Fix when this fails: `cd <dir> && go mod tidy`, for each reported directory.
#
# Usage: scripts/ci/check-nested-modules.sh
# Exit 0 = every nested module resolves; exit 1 = at least one is stale.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

# Derived from the tree, not hardcoded, so a new nested module is covered the
# moment it is added.
mapfile -t mods < <(find . -name go.mod -not -path '*/vendor/*' -not -path './go.mod' | sort)

if [ ${#mods[@]} -eq 0 ]; then
  echo "no nested modules found — nothing to check"
  exit 0
fi

stale=()
for mod in "${mods[@]}"; do
  dir="$(dirname "$mod")"
  # GOFLAGS is cleared deliberately: an ambient -mod=mod in the environment would
  # let the toolchain silently REWRITE go.mod and report success, turning this
  # check into a no-op. Read-only is the whole point.
  if ( cd "$dir" && GOFLAGS= go list ./... >/dev/null 2>&1 ); then
    echo "ok:     $dir"
  else
    echo "STALE:  $dir"
    stale+=("$dir")
  fi
done

if [ ${#stale[@]} -gt 0 ]; then
  echo
  echo "::error::${#stale[@]} nested module(s) are out of sync with the root go.mod"
  echo "These modules 'replace' the root, so a root dependency bump must be"
  echo "propagated into their indirect pins. To fix:"
  for dir in "${stale[@]}"; do
    echo "  (cd $dir && go mod tidy)"
  done
  exit 1
fi

echo
echo "all ${#mods[@]} nested module(s) resolve against the root module"
