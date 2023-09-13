package cmd

import (
	"strings"

	"github.com/rclone/rclone/fs/rc"
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
			fsPieces := strings.Split(args[1], ":")
			var fs, remote string
			fs = fsPieces[0] + ":"
			if len(fsPieces) > 1 {
				remote = fsPieces[1]
			}
			params := rc.Params{
				"fs":     fs,
				"remote": remote,
				"tier":   args[0],
			}
			err := rclone.APIOneShot("operations/settier", params)

			checkErr(err, "could not set tier")
		},
	}
	return cmd
}
