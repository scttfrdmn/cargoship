// Package version is the single source of truth for CargoShip's version.
//
// The canonical version lives in version.txt (embedded below) and is the value
// every part of the project should reference:
//   - the binary's `--version` (via cmd/cargoship/main.go) for dev/`go run`
//     builds, where no linker flags are set;
//   - release builds, where goreleaser overrides it with the git tag via
//     `-X main.version` — the tag and version.txt are kept in lockstep by the
//     release process and the drift check (scripts/check-version.sh);
//   - documentation strings (CLAUDE.md, README.md, ROADMAP.md, CHANGELOG.md),
//     which the drift check verifies against this file.
//
// To bump the version, edit version.txt only.
package version

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var raw string

// Version is the canonical semantic version, without a leading "v"
// (e.g. "0.14.0").
var Version = strings.TrimSpace(raw)
