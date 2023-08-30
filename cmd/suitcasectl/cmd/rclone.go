package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

// NewRcloneCmd returns a new cobra.Command for Rclone
func NewRcloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rclone",
		Short:   "Execute an rclone (sync) from src to dct",
		Aliases: []string{"r"},
		// Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			src, _ := cmd.Flags().GetString("source")
			destination, _ := cmd.Flags().GetString("destination")
			fmt.Println("rclone called")
			rclone.Clone(src, destination)
		},
	}

	return cmd
}
