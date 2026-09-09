#!/usr/bin/env bash
# Reachability guard: fail if the shipped CLI's dependency graph pulls in a
# package that is meant to be off the real path — simulated/experimental
# subsystems that must not be silently wired into what users run.
#
# The 2026-09 audit found large simulated subsystems (pkg/multiregion,
# pkg/monitoring) advertised or half-built but reached by nothing. This locks
# that in: if someone imports one from the CLI, they must make a deliberate
# decision (wire it for real + remove it from FORBIDDEN here), not do it by
# accident. It is the reachability counterpart to check-no-theater.sh.
#
# Usage: scripts/ci/check-no-dead-imports.sh
# Exit: 0 = clean, 1 = a forbidden package is reachable from the CLI, 2 = usage.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

# Packages that must NOT be reachable from ./cmd/cargoship. Each is either dead
# (pending deletion) or roadmap-only (must not ship wired until decided). Wiring
# one for real means removing it here in the same change.
FORBIDDEN=(
  "github.com/scttfrdmn/cargoship/pkg/multiregion"  # roadmap; simulated coordinator/health/failover (audit 2026-09)
  "github.com/scttfrdmn/cargoship/pkg/monitoring"   # dead; simulated CPU/mem/net, no prod consumer (audit 2026-09)
)

module="$(go list -m 2>/dev/null)"
if [ -z "$module" ]; then
  echo "::error::could not determine module path (go list -m failed)"
  exit 2
fi

# Full transitive import set of the CLI binary.
deps="$(GOFLAGS=-mod=mod go list -deps ./cmd/cargoship/... 2>/dev/null)"
if [ -z "$deps" ]; then
  echo "::error::go list -deps ./cmd/cargoship/... produced no output"
  exit 2
fi

violations=0
for pkg in "${FORBIDDEN[@]}"; do
  if printf '%s\n' "$deps" | grep -qxF "$pkg"; then
    echo "::error::$pkg is reachable from ./cmd/cargoship — it must stay off the shipped path"
    # Show one import chain for debugging.
    go mod why "$pkg" 2>/dev/null | sed 's/^/    /' | head -12
    violations=$((violations + 1))
  fi
done

if [ "$violations" -gt 0 ]; then
  echo
  echo "::error::$violations forbidden package(s) reachable from the CLI. Either revert"
  echo "the import, or — if you are intentionally shipping it — wire it for real and"
  echo "remove it from FORBIDDEN in scripts/ci/check-no-dead-imports.sh."
  exit 1
fi

echo "no forbidden simulated/roadmap packages are reachable from ./cmd/cargoship"
exit 0
