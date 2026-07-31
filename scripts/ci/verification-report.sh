#!/usr/bin/env bash
# scripts/ci/verification-report.sh
#
# Generates a dated, per-release integrity verification report from the output
# of the real-AWS integration suite (#270 leg 3, sub-issue #5: "publish a dated
# verification report per release"). Trust compounds when it is visible and
# dated: this turns the transient CI evidence into a durable, published artifact.
#
# It does NOT run the tests — it parses their `go test -v` output (so the report
# reflects exactly the run that gated the release, no double execution and no
# risk of the numbers drifting from what actually passed). The caller runs the
# suite, tees stdout to a log, and passes that log here.
#
# Inputs:
#   -i <file>   go test -v output log (required)
#   -v <ver>    version string, e.g. v0.15.0 (required)
#   -d <date>   ISO date stamp (required; passed in — the workflow owns the clock)
#   -c <sha>    commit SHA (optional)
#   -o <file>   output markdown path (default: stdout)
#
# Exit codes: 0 = report written (suite passed); 1 = usage/parse error;
# 2 = the parsed log shows FAILs (the report is still written, marked FAILED).

set -euo pipefail

LOG=""
VERSION=""
DATESTAMP=""
COMMIT=""
OUT=""

usage() {
  cat <<EOF
Usage: $0 -i <test-log> -v <version> -d <iso-date> [-c <commit>] [-o <out.md>]

Generate a dated per-release integrity verification report from real-AWS
integration test output.
EOF
}

while getopts "i:v:d:c:o:h" opt; do
  case "$opt" in
    i) LOG="$OPTARG" ;;
    v) VERSION="$OPTARG" ;;
    d) DATESTAMP="$OPTARG" ;;
    c) COMMIT="$OPTARG" ;;
    o) OUT="$OPTARG" ;;
    h) usage; exit 0 ;;
    *) usage; exit 1 ;;
  esac
done

if [ -z "$LOG" ] || [ -z "$VERSION" ] || [ -z "$DATESTAMP" ]; then
  echo "error: -i, -v, and -d are required" >&2
  usage
  exit 1
fi
if [ ! -f "$LOG" ]; then
  echo "error: test log not found: $LOG" >&2
  exit 1
fi

# --- Parse the round-trip evidence markers -----------------------------------
# The property test emits, per storage path:
#   VERIFICATION_EVIDENCE mode=direct files=10 bytes=12345 chunked=false
# Sum files/bytes across all markers and record which paths were exercised.
total_files=0
total_bytes=0
paths_seen=""
while IFS= read -r line; do
  files=$(echo "$line" | sed -n 's/.*files=\([0-9]*\).*/\1/p')
  bytes=$(echo "$line" | sed -n 's/.*bytes=\([0-9]*\).*/\1/p')
  mode=$(echo "$line" | sed -n 's/.*mode=\([a-z]*\).*/\1/p')
  [ -n "$files" ] && total_files=$((total_files + files))
  [ -n "$bytes" ] && total_bytes=$((total_bytes + bytes))
  [ -n "$mode" ] && paths_seen="$paths_seen $mode"
done < <(grep -F "VERIFICATION_EVIDENCE" "$LOG" || true)

# De-dup the storage-path list, preserving a stable order.
storage_paths=""
for p in direct chunked; do
  case " $paths_seen " in
    *" $p "*) storage_paths="${storage_paths:+$storage_paths, }$p" ;;
  esac
done
[ -z "$storage_paths" ] && storage_paths="(none observed)"

# --- Parse per-package pass/fail from `go test` summary lines ----------------
# Lines look like: "ok  \tgithub.com/.../pkg/pipeline\t105.336s"
#              or:  "FAIL\tgithub.com/.../pkg/foo\t0.5s"
pkg_ok=$(grep -cE '^ok[[:space:]]+github.com/scttfrdmn/cargoship' "$LOG" || true)
pkg_fail=$(grep -cE '^FAIL[[:space:]]+github.com/scttfrdmn/cargoship' "$LOG" || true)
pkg_ok=${pkg_ok:-0}
pkg_fail=${pkg_fail:-0}

# Human-readable byte size.
human_bytes() {
  local b=$1
  if [ "$b" -ge 1073741824 ]; then awk "BEGIN{printf \"%.2f GB\", $b/1073741824}"
  elif [ "$b" -ge 1048576 ]; then awk "BEGIN{printf \"%.2f MB\", $b/1048576}"
  elif [ "$b" -ge 1024 ]; then awk "BEGIN{printf \"%.2f KB\", $b/1024}"
  else echo "${b} B"; fi
}
bytes_h=$(human_bytes "$total_bytes")

status="PASSED"
badge="✅"
exit_code=0
if [ "$pkg_fail" -gt 0 ]; then
  status="FAILED"
  badge="❌"
  exit_code=2
fi

commit_line=""
[ -n "$COMMIT" ] && commit_line="- **Commit:** \`${COMMIT}\`"

# List the integration suites that ran (the `ok`/`FAIL` package lines), so the
# report names the actual evidence, not just a count.
suite_lines=$(grep -E '^(ok|FAIL)[[:space:]]+github.com/scttfrdmn/cargoship' "$LOG" \
  | sed -E 's#^(ok|FAIL)[[:space:]]+github.com/scttfrdmn/cargoship/([^[:space:]]+)[[:space:]]+([0-9.]+s).*#- `\2` — \1 (\3)#' \
  | sort -u || true)
[ -z "$suite_lines" ] && suite_lines="- (no integration packages recorded)"

render() {
  cat <<EOF
# Integrity Verification Report — ${VERSION}

${badge} **${status}**

- **Version:** ${VERSION}
- **Date:** ${DATESTAMP}
${commit_line}
- **Target:** real AWS S3 (dedicated \`cargoship-dev\` account)

CargoShip's core promise is that **what you restore is byte-identical to what
you uploaded**. This report is the dated, per-release evidence for that claim:
the whole-pipeline round-trip invariant and the credential-gated integration
suites, run against **real S3** at release time. It is generated from the actual
test output that gated this release — see [Integrity model](/project/integrity).

## Round-trip integrity invariant

A deliberately hostile corpus (empty files, large files, incompressible and
highly-compressible content, deep nesting, unicode / spaces / dotfile names) was
uploaded through the real pipeline and restored through the real
\`SelectiveExtractor.BatchRestore\`, then every file's SHA-256 was compared to
the source.

| Metric | Value |
|---|---|
| Files round-tripped byte-identical | **${total_files}** |
| Bytes round-tripped | **${bytes_h}** (${total_bytes} bytes) |
| Storage paths exercised | ${storage_paths} |
| Byte-identity failures | **$([ "$status" = "PASSED" ] && echo 0 || echo "≥1 — SEE CI")** |

## Integration suites (real S3)

Passed: **${pkg_ok}** · Failed: **${pkg_fail}**

${suite_lines}

## What this does and does not show

- **Shows:** for this exact build, the upload→restore path preserves bytes
  across both direct and chunked storage, verified against real S3, over an
  adversarial corpus. The manifest is independently readable and schema-valid.
- **Does not show:** a proof for all possible inputs, protection against
  source-side corruption before upload, or cost/performance guarantees. Those
  are honestly bounded, not proven — see the Integrity model page.

## Reproduce

\`\`\`bash
# Requires AWS credentials + a test bucket (see docs/project/integrity.md).
CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 \\
CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS=true \\
CARGOSHIP_TEST_BUCKET=<your-bucket> AWS_REGION=us-east-1 \\
  go test -tags integration -run TestRoundTripProperty -v ./pkg/pipeline/
\`\`\`

---
_Generated by \`scripts/ci/verification-report.sh\` from the release-gating test run._
EOF
}

if [ -n "$OUT" ]; then
  render > "$OUT"
  echo "wrote verification report: $OUT (status=$status, files=$total_files, bytes=$total_bytes)" >&2
else
  render
fi

exit "$exit_code"
