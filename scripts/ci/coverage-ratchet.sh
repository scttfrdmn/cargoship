#!/usr/bin/env bash
#
# coverage-ratchet.sh — per-package coverage ratchet for CargoShip (#238 Phase 6).
#
# Replaces the single flat project-wide floor with monotonic per-package floors
# recorded in .coverage-baseline. A package may never regress below its recorded
# floor (minus a small tolerance for run-to-run noise); when coverage improves,
# run --update to raise the floor so the gain can't silently erode later.
#
# Usage:
#   coverage-ratchet.sh [--check]     Verify coverage against the baseline (default).
#   coverage-ratchet.sh --update      Raise floors to current coverage; add new packages.
#   coverage-ratchet.sh --profile P   Use an existing coverage profile instead of re-running tests.
#
# Exit codes: 0 = ok, 1 = a package regressed / project floor breached, 2 = usage.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

BASELINE_FILE=".coverage-baseline"
MODULE_PREFIX="github.com/scttfrdmn/cargoship/"

# Project-wide hard floor (matches the historical pre-commit gate).
PROJECT_FLOOR=56
# Per-package tolerance: coverage may dip this many points below a recorded floor
# before it's treated as a regression (absorbs nondeterministic timing tests).
TOLERANCE=1

MODE="check"
PROFILE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --check) MODE="check" ;;
    --update) MODE="update" ;;
    --profile) PROFILE="${2:-}"; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done

# Produce a coverage profile unless one was supplied.
if [ -z "$PROFILE" ]; then
  PROFILE="$(mktemp -t cargoship-cov.XXXXXX)"
  trap 'rm -f "$PROFILE"' EXIT
  echo "Running tests with coverage (this can take a couple of minutes)..." >&2
  # -short + exclude examples: mirror the pre-commit / CI coverage job exactly.
  # shellcheck disable=SC2046 # intentional word-splitting to pass package args
  go test -short -timeout=300s -coverprofile="$PROFILE" -covermode=atomic \
    $(go list ./... | grep -v '/examples/') >/dev/null 2>&1 || true
fi

if [ ! -s "$PROFILE" ]; then
  echo "ERROR: coverage profile is empty ($PROFILE)" >&2
  exit 1
fi

# Per-package covered/total statement counts, computed directly from the profile
# lines: "<file>:<startline>.<col>,<endline>.<col> <numstmts> <count>". A single
# awk pass aggregates by package directory (the profile has ~20k lines, so a
# bash while-loop with per-line subprocesses is far too slow). Example/demo
# packages are excluded to match the pre-commit and baseline convention.
#
# Output lines: "<pkg> <pct>" (integer percent, floored).
declare -A cur_pct
while read -r pkg pct; do
  [ -z "$pkg" ] && continue
  cur_pct["$pkg"]="$pct"
done < <(awk -v prefix="$MODULE_PREFIX" '
  NR == 1 && $0 ~ /^mode:/ { next }
  {
    # Field layout: last field = exec count, second-last = num statements,
    # first field up to ":" = file path.
    n = $(NF-1); c = $NF
    colon = index($1, ":")
    file = substr($1, 1, colon - 1)
    # package dir = file path without the trailing "/<basename>"
    slash = 0
    for (i = length(file); i > 0; i--) { if (substr(file, i, 1) == "/") { slash = i; break } }
    pkg = substr(file, 1, slash - 1)
    sub("^" prefix, "", pkg)
    if (pkg ~ /(^|\/)examples(\/|$)/) next
    stmts[pkg] += n
    tot_stmts += n
    if (c != "0") { covered[pkg] += n; tot_covered += n }
  }
  END {
    for (p in stmts) if (stmts[p] > 0) printf "%s %d\n", p, covered[p] * 100 / stmts[p]
    if (tot_stmts > 0) printf "__TOTAL__ %d\n", tot_covered * 100 / tot_stmts
  }
' "$PROFILE")

# Split the project total out of the per-package map.
project_pct="${cur_pct[__TOTAL__]:-0}"
unset 'cur_pct[__TOTAL__]'

# Load recorded floors.
declare -A floor
if [ -f "$BASELINE_FILE" ]; then
  while read -r pkg pct; do
    [ -z "$pkg" ] && continue
    case "$pkg" in \#*) continue ;; esac
    floor["$pkg"]="$pct"
  done < "$BASELINE_FILE"
fi

if [ "$MODE" = "update" ]; then
  # Raise floors to current coverage; add newly-covered packages; keep existing
  # floor when current dipped (monotonic — never auto-lower).
  {
    echo "# CargoShip per-package coverage ratchet baseline."
    echo "# Format: <package> <min-percent>. Coverage must not drop below the floor."
    echo "# Monotonic: raise a floor when coverage improves; never lower it without cause."
    echo "# Regenerate/raise with: scripts/ci/coverage-ratchet.sh --update"
    echo "# Project-wide floor is PROJECT_FLOOR in the script (currently ${PROJECT_FLOOR})."
    for pkg in $(printf '%s\n' "${!cur_pct[@]}" "${!floor[@]}" | sort -u); do
      c="${cur_pct[$pkg]:-}"
      f="${floor[$pkg]:-0}"
      if [ -n "$c" ] && [ "$c" -gt "$f" ]; then
        printf '%s %d\n' "$pkg" "$c"
      else
        printf '%s %d\n' "$pkg" "$f"
      fi
    done
  } > "$BASELINE_FILE"
  echo "Updated $BASELINE_FILE."
  exit 0
fi

# --check
fail=0
echo "Project coverage: ${project_pct}% (floor: ${PROJECT_FLOOR}%, tolerance: ${TOLERANCE})"
# Apply the per-package tolerance to the project total too: measured coverage
# varies ~±1% run-to-run (concurrency/timing-sensitive tests), so gate on
# PROJECT_FLOOR - TOLERANCE to avoid spurious CI failures while keeping the
# documented target at PROJECT_FLOOR.
if [ "$project_pct" -lt $(( PROJECT_FLOOR - TOLERANCE )) ]; then
  echo "FAIL: project coverage ${project_pct}% below floor ${PROJECT_FLOOR}% (tolerance ${TOLERANCE})" >&2
  fail=1
fi

for pkg in $(printf '%s\n' "${!floor[@]}" | sort); do
  f="${floor[$pkg]}"
  c="${cur_pct[$pkg]:-}"
  if [ -z "$c" ]; then
    # Package in baseline but absent from this run (e.g. no test files now).
    echo "WARN: $pkg in baseline but not measured this run" >&2
    continue
  fi
  if [ "$c" -lt $(( f - TOLERANCE )) ]; then
    echo "FAIL: $pkg coverage ${c}% dropped below floor ${f}% (tolerance ${TOLERANCE})" >&2
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "Coverage ratchet OK: no package regressed below its floor."
fi
exit "$fail"
