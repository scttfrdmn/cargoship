package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewCreateCmd creates a new 'create' command
func NewCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create something!",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return errors.New(cmd.UsageString())
		},
	}
	bindCreateKeys(cmd)
	return cmd
}
