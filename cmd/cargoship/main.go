/*
Packaged main defines the top level CLI module
*/
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/scttfrdmn/cargoship/cmd/cargoship/cmd"
)

// Version information set during build
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// buildVersionInfo constructs version string with build information
func buildVersionInfo() string {
	versionInfo := version
	if commit != "unknown" {
		versionInfo += " (" + commit + ")"
	}
	if date != "unknown" {
		versionInfo += " built on " + date
	}
	return versionInfo
}

func main() {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	err := cmd.NewRootCmdWithVersion(os.Stdout, buildVersionInfo()).ExecuteContext(context.Background())
	if err != nil {
		slog.Error("error executing command, quitting", "error", err)
		os.Exit(3)
	}
}
