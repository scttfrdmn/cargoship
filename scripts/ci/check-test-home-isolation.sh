#!/usr/bin/env bash
# Fail if a test package can reach the developer's home directory without
# isolating it first (#1048).
#
# Why this exists: `go test ./...` used to write ~/.cargoship.yaml,
# ~/.cargoship-context and ~/.cargoship/state on the machine running it — a
# developer's real config, budgets included, could be overwritten by the test
# suite (the #891 data-loss class, reached through tests). internal/testenv
# isolates HOME, the XDG dirs and the AWS config; every test package that links
# home-reading code must call it from an init in a _test.go file:
#
#     func init() { testenv.Isolate() }
#
# "Home-reading code" is found, not listed: any non-test package in this module
# that calls os.UserHomeDir, os.UserConfigDir, os.UserCacheDir or homedir.Dir,
# plus the AWS SDK config loader (it reads ~/.aws and AWS_PROFILE). A test
# package needs isolation when its test binary links one of those packages.
#
# Usage: scripts/ci/check-test-home-isolation.sh
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
module=$(go list -m)

# Directories of non-test Go files that look up a home/config/cache directory.
home_pkgs=$(
  grep -rlE 'os\.User(Home|Config|Cache)Dir\(|homedir\.Dir\(' --include='*.go' cmd pkg internal 2>/dev/null |
    grep -v '_test\.go$' | grep -v '^internal/testenv/' |
    xargs -n1 dirname | sed "s|^|$module/|"
  # The AWS SDK's config loader reads ~/.aws and AWS_PROFILE: a test binary
  # linking it can reach the developer's real AWS profile.
  echo "github.com/aws/aws-sdk-go-v2/config"
)
home_pkgs=$(sort -u <<<"$home_pkgs")
if [ -z "$home_pkgs" ]; then
  echo "check-test-home-isolation: no home-reading packages found (unexpected)" >&2
  exit 1
fi

missing=()
# One `go list -test` call: for each test binary (ImportPath ending .test), its
# directory and every linked package. Variant suffixes like " [x.test]" are
# stripped so a package's own test variant counts as linking it.
while IFS='|' read -r ip dir deps; do
  [[ "$ip" == *.test ]] || continue
  linked=$(tr ' ' '\n' <<<"$deps" | sed 's/ \[.*\]$//' | sort -u)
  if ! comm -12 <(echo "$home_pkgs") <(echo "$linked") | grep -q .; then
    continue
  fi
  if ! grep -lq 'testenv\.Isolate()' "$dir"/*_test.go 2>/dev/null; then
    missing+=("${dir#"$PWD"/}")
  fi
done < <(go list -test -f '{{.ImportPath}}|{{.Dir}}|{{join .Deps " "}}' ./cmd/... ./pkg/... ./internal/...)

if [ ${#missing[@]} -gt 0 ]; then
  echo "check-test-home-isolation: these test packages link home-reading code but never isolate HOME:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  echo "Add a _test.go file with:  func init() { testenv.Isolate() }  (internal/testenv, #1048)" >&2
  exit 1
fi
echo "check-test-home-isolation: OK"
