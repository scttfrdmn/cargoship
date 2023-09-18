/*
Package porter is the superset of things that can operate the suitcases and
such

This package came about because I found myself having these giant functions
that had to pass in tons of individual items to get everything it needed. This
object is an attempt to simplify all those in to one cohesive bit
*/
package porter

import (
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

// Porter holds all the pieces of the suitcases together and such
type Porter struct {
	Cmd         *cobra.Command
	Args        []string
	TravelAgent *travelagent.TravelAgent
	Inventory   *inventory.Inventory
	Logger      *zerolog.Logger
}

// SendUpdate sends an update to the travel agent if it exists
func (p Porter) SendUpdate(u travelagent.StatusUpdate) error {
	if p.TravelAgent == nil {
		return nil
	}
	log := *p.Logger

	resp, err := p.TravelAgent.Update(u)
	if err != nil {
		return err
	}
	if p.Logger != nil {
		if u.ComponentName != "" {
			log = log.With().Str("component", u.ComponentName).Logger()
		}
		for _, msg := range resp.Messages {
			if strings.TrimSpace(msg) != "updated fields:" {
				log.Info().Msg(msg)
			}
		}
	}

	return nil
}
