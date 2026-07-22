#!/usr/bin/env bash
# Doc-consistency guard: keep repo docs from drifting behind releases.
#
# Two independent checks, both intentionally SURGICAL to avoid false positives on
# legitimate version mentions (semver examples like "v0.4 → v0.5", historical
# roadmap entries, benchmark-environment notes):
#
#   1. "Current Version" declarations must match the latest release. The files
#      below each carry one authoritative "Current Version: vX.Y.Z" line; those
#      must equal the newest semver git tag. Every OTHER version string in the
#      repo is left alone.
#
#   2. Denylisted tokens must not appear anywhere in the tracked docs. These are
#      removed commands / fictional settings that are always wrong if present
#      (not version numbers). Historical prose that must reference them can be
#      added to the allowlist below.
#
# Usage: scripts/ci/check-doc-versions.sh
# Exit 0 = consistent; exit 1 = drift found (prints what and where).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

fail=0

# --- Check 1: "Current Version" declarations track the latest release tag ------

# Newest vX.Y.Z tag (sorted by version). Fall back gracefully if no tags.
latest_tag="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1)"
if [ -z "$latest_tag" ]; then
  echo "::warning::no vX.Y.Z git tags found; skipping current-version check"
else
  # Files that declare the canonical current version.
  version_files=(CLAUDE.md ROADMAP.md)
  for f in "${version_files[@]}"; do
    [ -f "$f" ] || continue
    # Extract the version from a line like: **Current Version**: v0.13.2 (...)
    declared="$(grep -oE 'Current Version\*{0,2}:? *v[0-9]+\.[0-9]+\.[0-9]+' "$f" \
                  | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)"
    if [ -z "$declared" ]; then
      echo "::error file=$f::no 'Current Version: vX.Y.Z' declaration found"
      fail=1
    elif [ "$declared" != "$latest_tag" ]; then
      echo "::error file=$f::declares Current Version $declared but latest release is $latest_tag"
      fail=1
    fi
  done
fi

# --- Check 2: denylisted (removed/fictional) tokens must not appear ------------

# Tokens that are ALWAYS wrong if present in current docs (not version numbers).
denylist=(
  "create suitcase"          # removed command
  "CARGOSHIP_SECURITY_MODE"  # fictional env var
  "CARGOSHIP_TLS_REQUIRED"   # fictional env var
  "CARGOSHIP_AUDIT_LOG"      # fictional env var
)

# Files/dirs to police (tracked docs + governance). Excludes generated CLI docs,
# the VitePress build output, node_modules, and CHANGELOG (historical record).
mapfile -t doc_files < <(git ls-files \
  'README.md' 'SECURITY.md' 'CONTRIBUTING.md' 'ROADMAP.md' 'CLAUDE.md' 'docs/**/*.md' \
  | grep -vE '^docs/gen/|^docs/\.vitepress/dist/')

for token in "${denylist[@]}"; do
  hits="$(grep -rn -F "$token" "${doc_files[@]}" 2>/dev/null || true)"
  if [ -n "$hits" ]; then
    echo "::error::denylisted token '$token' found in docs:"
    echo "$hits"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "✅ doc-consistency: Current Version matches $latest_tag; no denylisted tokens."
fi
exit $fail
