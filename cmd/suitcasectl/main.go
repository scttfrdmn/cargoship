package main

import (
	"context"
	"fmt"
	"os"

	"gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmd"
)

func main() {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	err := cmd.NewRootCmd(os.Stdout).ExecuteContext(context.Background())
	if err != nil {
		// log.Error("error executing command", "error", err)
		fmt.Fprintf(os.Stderr, "Error executing command, quitting: %v", err)
		os.Exit(3)
	}
}
