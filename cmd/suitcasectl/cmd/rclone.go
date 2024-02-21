package cmd

import (
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/plugins/transporters"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

// NewRcloneCmd returns a new cobra.Command for Rclone
func NewRcloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rclone SOURCE DESTINATION",
		Short: "Execute an rclone (sync) from src to dst",
		Long: `rclone - Sync data out to the cloud (or elsewhere!) using an embedded version
of rclone.
		
SOURCE should be a local directory or file.

DESTINATION should be a valid rclone endpoint. This will be ready by your local
rclone config, but does not require the rclone binary to be present on your
host.`,
		Aliases: []string{"r"},
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return rclone.Copy(args[0], transporters.UniquifyDest(args[1]), nil)
		},
	}

	return cmd
}
