#!/usr/bin/env bash
# Validate .goreleaser.yml on every PR, so a config problem surfaces here rather
# than at tag-push time.
#
# Why this exists (#381): the release workflow pins `version: '~> v2'`, which
# tracks goreleaser minor releases as they land. Nothing in CI ran
# `goreleaser check`, so a deprecation could be removed upstream and we would
# learn about it from a FAILED RELEASE on a pushed tag — the one moment when the
# tree is frozen, the tag already public, and a fix means a retag. A `brews:`
# deprecation sat in the config across six releases, warning into a log nobody
# read, precisely because of this gap.
#
# The three states `goreleaser check` distinguishes, verified against 2.17.1 in a
# real repository (a scratch dir gives a misleading "configuration is invalid:
# scm releases: current folder is not a git repository" for every input, so this
# must run from the checkout):
#
#   exit 0  valid, no deprecations
#   exit 2  valid, but uses deprecated properties
#   exit 1  genuinely broken — unknown field, unparseable YAML
#
# The distinction is the useful part, and it is why this wraps `check` instead of
# calling it directly in the workflow. A deprecation is a scheduling problem: the
# release still works today and there may be a deliberate reason to wait (for
# `brews:` there is — see the KNOWN_DEPRECATIONS note below). A hard error is
# never acceptable. Collapsing both into one red X would mean either blocking
# every PR on a known-and-accepted deprecation, or ignoring the lane entirely.
#
# So: exit 1 is always a failure. Exit 2 fails only for deprecations that are NOT
# in the allowlist below, which makes accepting one an explicit, reviewable edit
# to this file rather than a silence.
#
# Usage: scripts/ci/check-goreleaser-config.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

# Deprecations we have looked at and are deliberately still carrying. Each entry
# needs an issue and a reason — the point is that the decision is written down,
# not that the warning is hidden.
#
#   brews  → #381. Migrating to `homebrew_casks:` is NOT a rename: `test:` and
#            `install:` do not exist on a cask, and casks propagate macOS
#            quarantine. Our darwin artifacts are adhoc/linker-signed only, not
#            notarized, and a quarantined v0.22.0 darwin_arm64 binary is
#            SIGKILLed by Gatekeeper (rc=137, measured). Migrating before we
#            notarize would break `brew install` on macOS, so `brews:` stays
#            until the signing question is settled.
KNOWN_DEPRECATIONS=(
  "brews"
)

if ! command -v goreleaser >/dev/null 2>&1; then
  echo "::error::goreleaser not installed — this check needs it on PATH"
  exit 1
fi

# NO_COLOR is set because goreleaser colourises when it detects CI, which puts
# ANSI escapes *inside* the deprecation line — between "DEPRECATED:" and the
# property name. Parsing that yielded an empty name, so the property matched
# nothing in the allowlist and the lane failed on its own known-good config.
# Belt and braces: the escapes are also stripped below, and an empty name is
# rejected explicitly, so this cannot silently degrade again if NO_COLOR stops
# being honoured.
output="$(NO_COLOR=1 goreleaser check 2>&1)"
status=$?
# shellcheck disable=SC2001 # a character-class sed is clearer here than ${//}
output="$(echo "$output" | sed 's/\x1b\[[0-9;]*m//g')"
echo "$output"
echo

# A hard configuration error. Never allowlisted: the release cannot run.
if [ "$status" -eq 1 ]; then
  echo "::error::.goreleaser.yml is invalid — a release would fail at tag time"
  echo "Run 'goreleaser check' locally to reproduce."
  exit 1
fi

if [ "$status" -eq 0 ]; then
  echo "goreleaser config is valid with no deprecations"
  exit 0
fi

if [ "$status" -ne 2 ]; then
  # An exit code this script has not characterised. Treat as a failure rather
  # than assume it is benign.
  echo "::error::'goreleaser check' exited $status, which this check does not recognise"
  echo "Inspect the output above and update scripts/ci/check-goreleaser-config.sh."
  exit 1
fi

# Exit 2: valid, but deprecated properties are in use. Extract which ones. The
# line looks like:
#   • DEPRECATED:  brews  should not be used anymore, check https://...
# `+` not `*`: a zero-length match would produce an empty property name that
# matches nothing in the allowlist, failing the lane with a blank name in the
# message — which is exactly what the ANSI escapes above caused.
mapfile -t found < <(
  echo "$output" \
    | grep -o 'DEPRECATED:[[:space:]]*[A-Za-z0-9_.]\{1,\}' \
    | sed 's/DEPRECATED:[[:space:]]*//' \
    | grep -v '^$' \
    | sort -u
)

if [ ${#found[@]} -eq 0 ]; then
  echo "::error::'goreleaser check' reported deprecations but none could be parsed"
  echo "The output format may have changed; update the parser in this script."
  exit 1
fi

unexpected=()
for dep in "${found[@]}"; do
  known=false
  for allowed in "${KNOWN_DEPRECATIONS[@]}"; do
    if [ "$dep" = "$allowed" ]; then
      known=true
      break
    fi
  done
  if $known; then
    echo "known deprecation (accepted, see this script's header): $dep"
  else
    unexpected+=("$dep")
  fi
done

if [ ${#unexpected[@]} -gt 0 ]; then
  echo
  echo "::error::.goreleaser.yml uses ${#unexpected[@]} deprecated property(ies) not yet triaged: ${unexpected[*]}"
  echo "goreleaser removes deprecated properties on its own schedule, and the"
  echo "release workflow pins '~> v2', so this WILL become a failed release at"
  echo "tag-push time. Either migrate the property now, or add it to"
  echo "KNOWN_DEPRECATIONS in scripts/ci/check-goreleaser-config.sh with an issue"
  echo "and a reason."
  exit 1
fi

echo
echo "goreleaser config is valid; ${#found[@]} deprecation(s), all triaged"
