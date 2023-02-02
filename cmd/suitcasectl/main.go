package main

import (
	"os"

	"github.com/rs/zerolog/log"

	"gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmd"
)

func main() {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	err := cmd.NewRootCmd(os.Stdout).Execute()
	if err != nil {
		log.Fatal().Err(err).Msg("Error executing command")
	}
}
