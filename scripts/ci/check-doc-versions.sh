#!/usr/bin/env bash
# Doc-consistency guard: keep repo docs in lockstep with the canonical version.
#
# Three independent checks, all intentionally SURGICAL to avoid false positives
# on legitimate version mentions (semver examples like "v0.4 → v0.5", historical
# roadmap entries, benchmark-environment notes):
#
#   1. Doc version declarations must match the single source of truth,
#      internal/version/version.txt (NOT the latest git tag — version.txt may
#      legitimately lead the tag during release prep; the tag==version.txt
#      guarantee is enforced at release time by the tag step, not here). Each
#      file below carries one authoritative version string; those must equal
#      version.txt. Every OTHER version string in the repo is left alone.
#
#   2. Denylisted tokens must not appear anywhere in the tracked docs. These are
#      removed commands / fictional settings that are always wrong if present
#      (not version numbers). Historical prose that must reference them can be
#      added to the allowlist below.
#
#   3. Local file links in root markdown must resolve.
#
# Usage: scripts/ci/check-doc-versions.sh
# Exit 0 = consistent; exit 1 = drift found (prints what and where).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

fail=0

# --- Check 1: doc version declarations match internal/version/version.txt ------

version_file="internal/version/version.txt"
if [ ! -f "$version_file" ]; then
  echo "::error::$version_file (canonical version source) not found"
  fail=1
else
  ver="$(tr -d '[:space:]' < "$version_file")" # e.g. 0.14.0
  vver="v${ver}"

  # Each entry: "<file>|<exact string that must be present>".
  # These are the authoritative version declarations the release process bumps.
  declarations=(
    "CLAUDE.md|**Current Version**: ${vver}"
    "ROADMAP.md|**Current Version**: ${vver}"
    "README.md|**${vver}**"
    "CHANGELOG.md|## [${ver}]"
  )
  for entry in "${declarations[@]}"; do
    f="${entry%%|*}"
    needle="${entry#*|}"
    [ -f "$f" ] || continue
    if ! grep -qF "$needle" "$f"; then
      echo "::error file=$f::missing current-version declaration; expected to find \"$needle\" (from $version_file = $ver)"
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

# --- Check 3: local file links in root markdown resolve ------------------------
#
# VitePress already fails its build on dead links inside the docs/ site. This
# covers the gap: relative file links in the repo-root markdown (README,
# CONTRIBUTING, SECURITY, ROADMAP) that point at files in the repo — e.g.
# [x](docs/foo.md), [x](pkg/manifest/README.md), [x](CHANGELOG.md). External
# (http) and in-page anchor links are skipped.

root_md=(README.md CONTRIBUTING.md SECURITY.md ROADMAP.md CLAUDE.md)
for f in "${root_md[@]}"; do
  [ -f "$f" ] || continue
  # Extract link targets: [text](target). Drop http(s):, mailto:, and #anchors.
  while IFS= read -r target; do
    [ -z "$target" ] && continue
    case "$target" in
      http://*|https://*|mailto:*|\#*) continue ;;
    esac
    # Strip any #fragment from the path before checking existence.
    path="${target%%#*}"
    [ -z "$path" ] && continue
    if [ ! -e "$path" ]; then
      echo "::error file=$f::broken local link → $target"
      fail=1
    fi
  done < <(grep -oE '\]\([^)]+\)' "$f" | sed -E 's/^\]\(//; s/\)$//')
done

# --- Check 4: the published-reports table covers every released version --------
#
# docs/project/verification-reports.md hand-lists each release's report with a
# direct asset URL. That table is the evidence the maturity and integrity pages
# point at, so a release that lands without a row silently weakens the claim.
# Every CHANGELOG release from FIRST_REPORT_VERSION onward must appear.
#
# Only the *presence* of a row is checked, not the metrics — the numbers come
# from the release run and can't be validated offline.

reports_doc="docs/project/verification-reports.md"
# The release that introduced the real-AWS verification lane. Versions before
# this have no published report and are documented as such.
first_report_version="0.16.0"

if [ -f "$reports_doc" ] && [ -f CHANGELOG.md ]; then
  # Released versions, newest first, excluding [Unreleased].
  mapfile -t released < <(grep -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md \
    | sed -E 's/^## \[//; s/\]$//')

  for v in "${released[@]}"; do
    # Skip anything older than the first report (string-safe numeric compare).
    lowest="$(printf '%s\n%s\n' "$v" "$first_report_version" | sort -V | head -1)"
    [ "$lowest" = "$first_report_version" ] || continue

    if ! grep -qE "^\| v${v//./\\.} " "$reports_doc"; then
      echo "::error file=$reports_doc::no published-reports row for v$v; add it with the report's direct asset URL (see the table in that file)"
      fail=1
    fi
  done
fi

if [ "$fail" -eq 0 ]; then
  echo "✅ doc-consistency: version declarations match ${version_file:+$(tr -d '[:space:]' < "$version_file")}; no denylisted tokens; local links resolve; verification-report table covers all releases."
fi
exit $fail
