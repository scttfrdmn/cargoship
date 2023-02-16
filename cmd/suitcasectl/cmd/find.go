package cmd

import (
	"github.com/drewstinnett/gout/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

// NewFindCmd creates a new 'find' command
func NewFindCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "find PATTERN INVENTORY.yaml",
		Short:   "Find where a file or directory lives from an inventory file",
		Aliases: []string{"search"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]
			searchD, err := cmd.Flags().GetStringArray("inventory-directory")
			checkErr(err, "")
			collection, err := inventory.CollectionWithDirs(searchD)
			checkErr(err, "")
			for inventoryF, i := range *collection {
				log.Info().Str("pattern", pattern).Str("inventory", inventoryF).Msg("find running")
				results := i.Search(pattern)
				gout.MustPrint(results)
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringArray("inventory-directory", []string{"."}, "Directory containing inventories to search. Can be specified multiple times for multiple directories.")
	return cmd
}
