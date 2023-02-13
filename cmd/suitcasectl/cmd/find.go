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
		Use:   "find PATTERN INVENTORY.yaml",
		Short: "Find where a file or directory lives from an inventory file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]
			inventoryF := args[1]
			log.Info().Str("pattern", pattern).Str("inventory", inventoryF).Msg("find running")

			i, err := inventory.NewInventoryWithFilename(inventoryF)
			if err != nil {
				return err
			}

			results := i.Search(pattern)

			gout.MustPrint(results)
			return nil
		},
	}
	return cmd
}
