package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
	"gopkg.in/yaml.v3"
)

func newTravelAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "travelagent CREDENTIAL_FILE",
		Args:  cobra.ExactArgs(1),
		Short: "Run a travel agent server. NOT FOR PRODUCTION USE",
		RunE: func(_ *cobra.Command, args []string) error {
			credB, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}

			var creds travelagent.StaticCredentials
			if err := yaml.Unmarshal(credB, &creds); err != nil {
				return err
			}
			if len(creds.Transfers) == 0 {
				return errors.New("could not find any transfers")
			}
			s := travelagent.NewServer(
				travelagent.WithAdminToken(creds.AdminToken),
				travelagent.WithStaticTransfers(creds.Transfers),
			)
			logger.Info("starting server", "address", s.Addr())
			// Get port with: s.Listener.Addr().(*net.TCPAddr).Port
			return s.Run()
		},
	}
	return cmd
}
