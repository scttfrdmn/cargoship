package cmd

import (
	"github.com/drewstinnett/gout/v2"
	"github.com/spf13/cobra"
)

// NewWizardCmd creates a new 'find' command
func NewWizardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "wizard",
		Short:   "Run a console wizard to do the creation",
		Long:    `This is for users who want a simple command to do some basic stuff. For advanced usage, use 'create suitcase'`,
		Aliases: []string{"wiz", "easybutton"},
		RunE: func(cmd *cobra.Command, args []string) error {
			gout.SetWriter(cmd.OutOrStdout())

			p, err := porterTravelAgentWithForm()
			if err != nil {
				return err
			}
			_ = p

			return nil
		},
	}

	cmd.PersistentFlags().StringArray("inventory-directory", []string{"."}, "Directory containing inventories to search. Can be specified multiple times for multiple directories.")
	return cmd
}
