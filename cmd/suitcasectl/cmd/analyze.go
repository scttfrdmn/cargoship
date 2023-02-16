package cmd

import (
	"github.com/drewstinnett/gout/v2"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

// NewAnalyzeCmd creates a new 'find' command
func NewAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "analyze DIRECTORY",
		Short:   "Analyze a directory to provide some useful runtime information",
		Aliases: []string{"a"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			i, err := inventory.NewDirectoryInventory(inventory.NewOptions(inventory.WithDirectories(args)))
			checkErr(err, "")
			gout.MustPrint(i.Analyze())
			return nil
		},
	}

	return cmd
}
