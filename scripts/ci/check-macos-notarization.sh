#!/usr/bin/env bash
# Assert that a released darwin binary is Developer-ID signed AND notarized, and
# that it actually runs under quarantine (#381).
#
# Why this is not redundant with "the release step said it signed": goreleaser's
# `notarize:` pipe is gated on a secret being present, so a missing or renamed
# secret makes it SKIP and report success. And notarization is a round-trip
# through Apple — a stapling or submission problem produces a binary that looks
# signed locally and is still killed on a user's machine. The only claim worth
# publishing is the one tested the way a user experiences it: download, quarantine,
# execute.
#
# What "not notarized" costs, measured on the published v0.22.0 darwin_arm64
# binary: under com.apple.quarantine, Gatekeeper does not warn — it SIGKILLs the
# process. rc=137, no output, no diagnostic.
#
# MUST run on a macOS runner: codesign, spctl and the quarantine bit are all
# macOS-only. There is no Linux equivalent, so this cannot be folded into the
# ubuntu release job.
#
# Usage: scripts/ci/check-macos-notarization.sh <path-to-binary>
set -uo pipefail

bin="${1:-}"
if [ -z "$bin" ] || [ ! -f "$bin" ]; then
  echo "::error::usage: $0 <path-to-binary> (got '${bin}')"
  exit 1
fi

if [ "$(uname -s)" != "Darwin" ]; then
  echo "::error::this check requires macOS (codesign/spctl/quarantine are macOS-only)"
  exit 1
fi

fail=0
note() { echo "  $*"; }

echo "== Checking $bin"

# 1. Developer ID authority. An adhoc/linker-signed binary has NO Authority line
#    at all, which is exactly what our releases looked like before #381.
authority="$(codesign -dvvv "$bin" 2>&1 | grep -E '^Authority=' | head -1 | sed 's/^Authority=//')"
if [ -z "$authority" ]; then
  echo "::error::$bin has no signing authority — it is adhoc/linker-signed, not Developer ID signed"
  note "codesign flags: $(codesign -dvvv "$bin" 2>&1 | grep -oE 'flags=0x[0-9a-f]+\([^)]*\)' | head -1)"
  fail=1
elif ! printf '%s' "$authority" | grep -q '^Developer ID Application:'; then
  echo "::error::$bin is signed by '$authority', not a Developer ID Application certificate"
  fail=1
else
  note "authority: $authority"
fi

# 2. Hardened runtime. Apple refuses to notarize without it, so this should be
#    implied by step 3 — but assert it directly, because a check that only
#    verifies a downstream consequence tells you less when it breaks.
flags="$(codesign -dvvv "$bin" 2>&1 | grep -oE 'flags=0x[0-9a-f]+\([^)]*\)' | head -1)"
if printf '%s' "$flags" | grep -q 'runtime'; then
  note "hardened runtime: yes ($flags)"
else
  echo "::error::$bin lacks the hardened runtime flag ($flags); Apple will not notarize it"
  fail=1
fi

# 3. Notarization, per Gatekeeper itself. This is THE discriminating check:
#    `codesign --verify --strict` returns 0 for an adhoc-signed binary too
#    (verified on both a Developer-ID binary and one of ours), so it cannot tell
#    signed from unsigned and is not used here.
#
#    A notarized binary reports `source=Notarized Developer ID`; ours reported
#    `rejected`.
assessment="$(spctl -a -vvv -t install "$bin" 2>&1)"
if printf '%s' "$assessment" | grep -q 'source=Notarized Developer ID'; then
  note "gatekeeper: $(printf '%s' "$assessment" | grep -o 'source=[^ ]*.*' | head -1)"
else
  echo "::error::$bin is not notarized — Gatekeeper says: $(printf '%s' "$assessment" | tr '\n' ' ')"
  fail=1
fi

# 4. The end-to-end claim: does it RUN when quarantined? Steps 1-3 inspect
#    metadata; only this executes the thing a user downloads.
#
#    Off by default, because a REJECTED probe raises a "'probe' Not Opened —
#    Apple could not verify..." notification on the machine running it. On a CI
#    runner nobody sees that; on a developer's Mac it is an alarming popup caused
#    by a script they ran for an unrelated reason, and it happened to this repo's
#    owner twice. CI sets QUARANTINE_PROBE=1 (see release.yml); locally you opt in
#    knowing what it does.
probed=no
if [ "${QUARANTINE_PROBE:-0}" = "1" ]; then
  probed=yes
  # Run from a copy so the xattr cannot leak into the caller's file.
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  cp "$bin" "$tmp/probe"
  xattr -w com.apple.quarantine "0083;00000000;Safari;" "$tmp/probe"
  note "quarantine set: $(xattr -p com.apple.quarantine "$tmp/probe" 2>&1)"

  # Capture the status directly. Piping into `head` reports HEAD's exit code,
  # which is how a SIGKILL first read as a success here.
  out="$("$tmp/probe" --version 2>&1)"
  rc=$?
  if [ "$rc" -eq 0 ]; then
    note "quarantined run: ok (rc=0) — $out"
  elif [ "$rc" -eq 137 ]; then
    echo "::error::quarantined binary was SIGKILLed by Gatekeeper (rc=137) — this is the unnotarized failure mode"
    fail=1
  else
    echo "::error::quarantined binary exited $rc: $out"
    fail=1
  fi
else
  note "quarantined run: SKIPPED (set QUARANTINE_PROBE=1 to enable; it raises a"
  note "  Gatekeeper notification when the binary is not notarized)"
fi

if [ "$fail" -ne 0 ]; then
  echo
  echo "::error::$bin is NOT properly signed and notarized"
  exit 1
fi

echo
# Don't claim the part that was skipped. A summary line that overstates what ran
# is how a weakened check goes unnoticed.
if [ "$probed" = "yes" ]; then
  echo "$bin is Developer ID signed, notarized, and runs under quarantine"
else
  echo "$bin is Developer ID signed and notarized (quarantined-run probe skipped)"
fi
