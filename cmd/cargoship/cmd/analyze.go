package cmd

import (
	"github.com/drewstinnett/gout/v2"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/spf13/cobra"
)

// NewAnalyzeCmd creates a new 'find' command
func NewAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "analyze DIRECTORY",
		Short:   "Analyze a directory to provide some useful runtime information",
		Aliases: []string{"a"},
		Args:    cobra.ExactArgs(1),
		Example: `❯ suitcasectl analyze ~/Desktop/example-suitcase
largestfilesize: 3154432
largestfilesizehr: 3.2 MB
filecount: 17
averagefilesize: 599271
averagefilesizehr: 599 kB
totalfilesize: 10187619
totalfilesizehr: 10 MB`,
		RunE: func(cmd *cobra.Command, args []string) error {
			gout.SetWriter(cmd.OutOrStdout())
			opts, err := inventory.NewOptions(inventory.WithDirectories(args))
			if err != nil {
				return err
			}
			i, err := inventory.NewDirectoryInventory(opts)
			if err != nil {
				return err
			}
			gout.MustPrint(i.Analyze())
			return nil
		},
	}

	return cmd
}
