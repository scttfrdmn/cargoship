#!/usr/bin/env bash
# Capability-verification guard (trust priority): every evidence token cited in
# the capability matrix (docs/project/verification.md) must resolve to an
# artifact that really exists — so a "Verified" claim can never drift from its
# proof. Plus a surgical check that invocations of removed commands don't
# reappear in the user-facing docs (the class of bug that left a dead `agent`
# context documented after v0.20.0).
#
# Usage: scripts/ci/check-capabilities.sh
# Exit 0 = every citation resolves and no removed-command invocations; 1 = drift.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

MATRIX="docs/project/verification.md"
fail=0

[ -f "$MATRIX" ] || { echo "::error::$MATRIX (capability matrix) not found"; exit 1; }

# --- Check A: every evidence token resolves ----------------------------------
# Tokens are backtick-wrapped: `test:Name`, `make:target`, `script:path`,
# `cmd:name`, or `report`. Only parse table rows (lines starting with '|') so the
# prose legend's placeholder examples aren't treated as real citations.
mapfile -t tokens < <(grep '^|' "$MATRIX" | grep -oE '`(test|make|script|cmd):[^`]+`|`report`' | tr -d '`' | sort -u)

if [ "${#tokens[@]}" -eq 0 ]; then
  echo "::error file=$MATRIX::no evidence tokens found — the matrix must cite checkable evidence"
  fail=1
fi

for tok in "${tokens[@]}"; do
  kind="${tok%%:*}"
  val="${tok#*:}"
  case "$kind" in
    test)
      grep -rqE "func ${val}\b" --include='*_test.go' . \
        || { echo "::error file=$MATRIX::evidence 'test:${val}' — no such test function"; fail=1; } ;;
    make)
      grep -qE "^${val}:" Makefile \
        || { echo "::error file=$MATRIX::evidence 'make:${val}' — no such Makefile target"; fail=1; } ;;
    script)
      [ -f "${val}" ] \
        || { echo "::error file=$MATRIX::evidence 'script:${val}' — file not found"; fail=1; } ;;
    cmd)
      grep -rqE "Use:[[:space:]]+\"${val}" cmd/ \
        || { echo "::error file=$MATRIX::evidence 'cmd:${val}' — no such CLI command"; fail=1; } ;;
    report)
      grep -qE "Passed" docs/project/verification-reports.md 2>/dev/null \
        || { echo "::error file=$MATRIX::evidence 'report' — no passed row in verification-reports.md"; fail=1; } ;;
  esac
done

# --- Check B: removed-command invocations must not reappear in user docs ------
# Surgical: only the direct-invocation form "cargoship [flags] <removed>" is
# always wrong now. Prose that names these features historically lives in the
# roadmap / maturity / this matrix and is excluded below.
removed_cmds=(agent controller webui wizard travelagent schema launch-server)
# Excluded: generated CLI docs, the build output, and the pages whose PURPOSE is
# to document removed/deferred features honestly (roadmap, maturity, this matrix,
# the verification-report log, and the enterprise migration note that tells users
# the controller/webui/launch commands are gone).
mapfile -t doc_files < <(git ls-files 'README.md' 'docs/**/*.md' \
  | grep -vE '^docs/gen/|^docs/\.vitepress/dist/|^docs/project/roadmap\.md$|^docs/project/maturity\.md$|^docs/project/verification\.md$|^docs/project/verification-reports\.md$|^docs/enterprise/index\.md$')

if [ "${#doc_files[@]}" -gt 0 ]; then
  for c in "${removed_cmds[@]}"; do
    hits="$(grep -rnE "cargoship( --?[a-z-]+)* ${c}\b" "${doc_files[@]}" 2>/dev/null || true)"
    if [ -n "$hits" ]; then
      echo "::error::invocation of removed command 'cargoship … ${c}' found in user docs:"
      echo "$hits"
      fail=1
    fi
  done
fi

if [ "$fail" -ne 0 ]; then
  echo "capability-verification: FAILED"
  exit 1
fi
echo "capability-verification: OK (${#tokens[@]} evidence tokens resolve; no removed-command invocations in user docs)"
