package cmd

import (
	"strings"

	"github.com/rclone/rclone/fs/rc"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

func splitFsRemote(s string) (string, string) {
	fsPieces := strings.Split(s, ":")
	var fs, remote string
	fs = fsPieces[0] + ":"
	if len(fsPieces) > 1 {
		remote = fsPieces[1]
	}
	return fs, remote
}

// NewRetierCmd represents the retier command
func NewRetierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "retier",
		Short:   "Change the tier of an object in cloud storage",
		Args:    cobra.ExactArgs(2),
		Example: `$ suitcasectl retier Archive suitcasectl-azure:/test`,
		Run: func(cmd *cobra.Command, args []string) {
			fs, remote := splitFsRemote(args[1])
			err := rclone.APIOneShot("operations/settier", rc.Params{
				"fs":     fs,
				"remote": remote,
				"tier":   args[0],
			})

			checkErr(err, "could not set tier")
		},
	}
	return cmd
}
