#!/usr/bin/env bash
# Anti-theater guard: fail if production Go code contains high-signal markers of
# fake/stubbed behaviour presented as real. Born from the 2026-09 "advertised vs
# actually-wired" audit, which found stub functions that only log, hardcoded
# "improvement" metrics (4.6x / 78.3% / 25%), and mock data returned as if live.
#
# This is a RATCHET, not a big-bang cleaner: every currently-known instance is
# listed in theater-allowlist.txt with a tracking reference, so the gate passes
# today while forbidding NEW theater anywhere else. When a subsystem is wired for
# real or deleted, remove its allowlist line — that is the "done" signal.
#
# Usage:
#   scripts/ci/check-no-theater.sh            # gate: allowlisted debt tolerated
#   scripts/ci/check-no-theater.sh --all      # report every match, ignore allowlist
#
# Exit: 0 = clean (modulo allowlist), 1 = new theater found, 2 = usage.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

ALLOWLIST="scripts/ci/theater-allowlist.txt"
show_all=0
[ "${1:-}" = "--all" ] && show_all=1

# High-signal lie-markers. Kept deliberately narrow to avoid false positives on
# honest simulation (benchmarks, estimators, test data legitimately "simulate").
#  1. stub bodies that only describe what they'd do
#  2. placeholder-for-real-implementation comments
#  3. mock data returned on a production path
#  4. hardcoded POSITIVE "improvement" metrics (a real metric comes from a
#     variable; ": 0" is an honest default and is not matched)
PATTERNS=(
  'In full implementation'
  'would be replaced with (a |an )?actual'
  'For now, return mock'
  '(OptimizationRatio|BandwidthSavings|LatencyReduction):[[:space:]]*[1-9]'
)

# Production Go only: no tests, examples, nested example modules, or benchmarks
# (those are honestly allowed to simulate).
mapfile -t files < <(git ls-files '*.go' ':!:*_test.go' ':!:examples/*' ':!:tests/*' ':!:benchmarks/*')

# Load allowlist globs (lines: "<path-glob>  # reason (issue)"; blanks/comments skipped).
allow=()
if [ -f "$ALLOWLIST" ]; then
  while IFS= read -r line; do
    line="${line%%#*}"
    line="$(printf '%s' "$line" | xargs || true)"
    [ -n "$line" ] && allow+=("$line")
  done < "$ALLOWLIST"
fi

is_allowlisted() {
  local f="$1"
  ((show_all)) && return 1
  local g
  for g in "${allow[@]}"; do
    # shellcheck disable=SC2053
    [[ "$f" == $g ]] && return 0
  done
  return 1
}

violations=0
allowed_hits=0
for f in "${files[@]}"; do
  for pat in "${PATTERNS[@]}"; do
    while IFS= read -r hit; do
      [ -z "$hit" ] && continue
      if is_allowlisted "$f"; then
        allowed_hits=$((allowed_hits + 1))
      else
        echo "::error::theater marker in $f:$hit"
        violations=$((violations + 1))
      fi
    done < <(grep -nE "$pat" "$f" 2>/dev/null)
  done
done

if ((show_all)); then
  echo "reported all matches (allowlist ignored)"
  exit 0
fi

if [ "$violations" -gt 0 ]; then
  echo
  echo "::error::$violations new theater marker(s) found in production code."
  echo "Wire the behaviour for real, or — if this is known, tracked debt — add the"
  echo "file to $ALLOWLIST with an issue reference."
  exit 1
fi

echo "no new theater markers ($allowed_hits allowlisted instance(s) still pending burn-down)"
exit 0
