package cmd

import (
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

// NewRetierCmd represents the retier command
func NewRetierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retier",
		Short: "Change the tier of an object in cloud storage",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			err := rclone.Command("settier", []string{args[0], args[1]}, nil)
			checkErr(err, "could not set teir")
		},
	}
	return cmd
}
