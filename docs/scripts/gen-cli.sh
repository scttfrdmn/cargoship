#!/usr/bin/env bash
# Generate the vendored CLI reference fragments in docs/gen/cli/ from the live
# cobra command tree, then sanitize them for VitePress + drift-checking.
#
# Why sanitize:
#   - cobra stamps a "Auto generated ... on <DATE>" footer that would change
#     every day and break the git-diff drift check;
#   - the "SEE ALSO" section links to sibling *.md files that don't resolve as
#     VitePress routes (the fragments are @included, not routed).
#
# The fragments are @included by the hand-written pages under reference/commands/.
# `cargoship mddocs` already omits hidden (man, mddocs) and unregistered
# (performance) commands. We additionally drop controller/webui — those are
# documented in the Distributed/Enterprise section, not the core reference.
#
# Usage: docs/scripts/gen-cli.sh [output-dir]   (default: docs/gen/cli)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${1:-$REPO_ROOT/docs/gen/cli}"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# Generate into a temp dir first, then sanitize into OUT_DIR.
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

( cd "$REPO_ROOT" && go run ./cmd/cargoship mddocs "$TMP_DIR" )

# Commands documented elsewhere (Enterprise section) — drop from core reference.
EXCLUDE=(cargoship_controller.md cargoship_webui.md)

for f in "$TMP_DIR"/*.md; do
  base="$(basename "$f")"
  for ex in "${EXCLUDE[@]}"; do
    [ "$base" = "$ex" ] && continue 2
  done
  # Sanitize for VitePress:
  #  - stop at the volatile SEE ALSO block (sibling-file links that don't resolve
  #    as VitePress routes) and drop the "Auto generated ... on <date>" footer
  #    (its date would break the drift check);
  #  - escape bare < and > that appear OUTSIDE fenced code blocks. Cobra Long
  #    descriptions put tokens like `<dir>`/`<file>` in indented prose, which
  #    Vue's markdown compiler otherwise reads as unclosed HTML tags and fails
  #    the build. Inside ``` fences we leave them alone so flag output is verbatim.
  awk '
    /^### SEE ALSO/ { exit }
    /^###### Auto generated/ { next }
    /^```/ { infence = !infence; print; next }
    {
      if (!infence) { gsub(/</, "\\&lt;"); gsub(/>/, "\\&gt;") }
      print
    }
  ' "$f" | sed -e 's/[[:space:]]*$//' > "$OUT_DIR/$base"
done

echo "Wrote $(ls "$OUT_DIR"/*.md | wc -l | tr -d ' ') CLI fragments to $OUT_DIR"
