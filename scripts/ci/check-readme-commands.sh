#!/usr/bin/env bash
#
# Guard README example commands against the drift class found in the v0.29.0
# review, which the Quick Start E2E (docs-site walkthrough) does not cover:
#
#   1. `cargoship info` / `cargoship verify` need the upload-id in their s3://
#      argument — `parseUploadPath` rejects a bare `s3://bucket/prefix`
#      ("upload-id is required"). The positional form must be
#      `s3://<bucket>/<prefix>/uploads/<upload-id>`.
#   2. `cargoship restore` takes `OUTPUT_DIR` positionally (Use:
#      "restore S3_URL OUTPUT_DIR", ExactArgs(2)) — there is NO `--output` flag.
#
# This is a targeted lint for those two shapes, not a full CLI validator. It
# joins backslash line-continuations so multi-line examples are checked as one
# command. Conservative by construction: it only inspects lines that use the
# positional `s3://` form, so flag-based invocations (--bucket/--upload-id) are
# left alone.
set -euo pipefail
cd "$(dirname "$0")/../.."

readme="README.md"
fail=0

# Join backslash-continued lines into one logical command line (portable awk).
joined="$(awk '/\\[[:space:]]*$/ { sub(/\\[[:space:]]*$/,""); buf=buf $0; next } { print buf $0; buf="" } END { if (buf != "") print buf }' "$readme")"

# 1. info / verify: the positional s3:// argument must carry /uploads/<id>.
while IFS= read -r line; do
  url="$(printf '%s\n' "$line" | grep -oE 's3://[^ ]+' | head -1 || true)"
  [ -z "$url" ] && continue
  case "$url" in
    */uploads/*) : ;;
    *)
      echo "::error file=$readme::info/verify need the upload-id — expected s3://<bucket>/<prefix>/uploads/<upload-id>, got: $line"
      fail=1
      ;;
  esac
done < <(printf '%s\n' "$joined" | grep -E 'cargoship (info|verify) s3://' || true)

# 2. restore: no --output flag (OUTPUT_DIR is positional).
while IFS= read -r line; do
  echo "::error file=$readme::'cargoship restore' has no --output flag; OUTPUT_DIR is positional (restore S3_URL OUTPUT_DIR): $line"
  fail=1
done < <(printf '%s\n' "$joined" | grep -E 'cargoship restore' | grep -- '--output' || true)

if [ "$fail" -ne 0 ]; then
  echo "❌ README command examples are invalid (see errors above)."
  exit 1
fi
echo "✅ README command examples valid: info/verify carry an upload-id; restore uses positional OUTPUT_DIR."
