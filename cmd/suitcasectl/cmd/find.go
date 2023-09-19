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
		Use:   "find PATTERN",
		Short: "Find where a file or directory lives",
		Long: `By default, we'll search your current directory for yaml
files, and treat them as inventory. This can be changed using
the --inventory-directory flag.`,
		Example: `$ suitcasectl find SOME_PATTERN
files:
    - path: /Users/drews/Desktop/Almost Garbage/godoc/src/runtime/cgo/libcgo_windows.h?m=text
      destination: godoc/src/runtime/cgo/libcgo_windows.h
      name: libcgo_windows.h
      size: 258
      suitcase_index: 5
      suitcase_name: suitcase-drews-05-of-05.tar.zst
...
directories:
    - directory: godoc/lib
      totalsize: 139186
      totalsizehr: 139 kB
      suitcases:
	- suitcase-drews-04-of-05.tar.zst
	- suitcase-drews-05-of-05.tar.zst
...`,
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
